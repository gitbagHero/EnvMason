// Package baseinstall binds reviewed macOS Base Plans to sanitized Homebrew
// transaction previews without exposing execution capability.
package baseinstall

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/lockfile"
	"github.com/gitbagHero/EnvMason/internal/plan"
)

const TransactionReviewSchemaVersion = "0.1.0"

var (
	transactionDigestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	formulaNamePattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9@+._/-]{0,127}$`)
	formulaVersionPattern    = regexp.MustCompile(`^[0-9][0-9A-Za-z.+_-]{0,127}$`)
	configurationKeyPattern  = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,127}$`)
	baselineIdentityPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9._:-]{0,127}$`)
)

type FormulaState string

const (
	FormulaSatisfied       FormulaState = "satisfied"
	FormulaInstallRequired FormulaState = "install_required"
)

// FormulaPreview is a sanitized package-manager fact. DownloadBytes is the
// reviewed artifact size, not a runtime network-transfer measurement.
type FormulaPreview struct {
	Name          string       `json:"name"`
	Version       string       `json:"version"`
	State         FormulaState `json:"state"`
	DownloadBytes int64        `json:"download_bytes"`
}

type ActionPreview struct {
	ActionID     string           `json:"action_id"`
	Root         FormulaPreview   `json:"root"`
	Dependencies []FormulaPreview `json:"dependencies"`
}

// TransactionBaseline contains only stable identities and digests. Paths,
// environment values and Homebrew configuration contents stay outside the
// review so it is safe to persist in a later increment.
type TransactionBaseline struct {
	ObservedAt              time.Time `json:"observed_at"`
	InstallationID          string    `json:"installation_id"`
	HomebrewVersion         string    `json:"homebrew_version"`
	ExecutableDigest        string    `json:"executable_digest"`
	ConfigurationDigest     string    `json:"configuration_digest"`
	ConfigurationSafe       bool      `json:"configuration_safe"`
	UnsafeConfigurationKeys []string  `json:"-"`
	CatalogSourceID         string    `json:"catalog_source_id"`
	CatalogDigest           string    `json:"catalog_digest"`
	CatalogSnapshotAt       time.Time `json:"catalog_snapshot_at"`
}

type TransactionReviewInput struct {
	PreparedAt time.Time
	Plan       plan.Plan
	Lock       lockfile.Lock
	Baseline   TransactionBaseline
	Actions    []ActionPreview
}

type TransactionSummary struct {
	Formulae           int   `json:"formulae"`
	Satisfied          int   `json:"satisfied"`
	InstallRequired    int   `json:"install_required"`
	TotalDownloadBytes int64 `json:"total_download_bytes"`
}

// TransactionReview is deliberately neither executable nor confirmable. It
// proves that every reviewed Base Plan action has a complete, exact and safe
// Homebrew formula closure before a write adapter can be considered.
type TransactionReview struct {
	SchemaVersion string              `json:"schema_version"`
	ID            string              `json:"id"`
	PreparedAt    time.Time           `json:"prepared_at"`
	Executable    bool                `json:"executable"`
	Confirmable   bool                `json:"confirmable"`
	PlanID        string              `json:"plan_id"`
	LockID        string              `json:"lock_id"`
	Target        lockfile.Target     `json:"target"`
	Baseline      TransactionBaseline `json:"baseline"`
	Actions       []ActionPreview     `json:"actions"`
	Summary       TransactionSummary  `json:"summary"`
}

func PrepareTransactionReview(input TransactionReviewInput) (TransactionReview, error) {
	if err := validateTransactionInput(input); err != nil {
		return TransactionReview{}, err
	}
	value := TransactionReview{
		SchemaVersion: TransactionReviewSchemaVersion,
		PreparedAt:    input.PreparedAt.UTC(),
		Executable:    false,
		Confirmable:   false,
		PlanID:        input.Plan.ID,
		LockID:        input.Lock.ID,
		Target:        input.Lock.Target,
		Baseline:      cloneTransactionBaseline(input.Baseline),
		Actions:       cloneActionPreviews(input.Actions),
	}
	canonicalizeTransactionReview(&value)
	summary, err := summarizeTransactions(value.Actions)
	if err != nil {
		return TransactionReview{}, fmt.Errorf("prepare Homebrew transaction review: %w", err)
	}
	value.Summary = summary
	value.ID, err = transactionReviewID(value)
	if err != nil {
		return TransactionReview{}, errors.New("prepare Homebrew transaction review: calculate ID")
	}
	if err := ValidateTransactionReview(value); err != nil {
		return TransactionReview{}, err
	}
	return value, nil
}

func ValidateTransactionReview(value TransactionReview) error {
	if value.SchemaVersion != TransactionReviewSchemaVersion || !transactionDigestPattern.MatchString(value.ID) ||
		value.PreparedAt.IsZero() || value.Executable || value.Confirmable ||
		!transactionDigestPattern.MatchString(value.PlanID) || !transactionDigestPattern.MatchString(value.LockID) {
		return errors.New("validate Homebrew transaction review: fixed identity, time or read-only flags are invalid")
	}
	if value.Target.OS != inventory.OSMacOS ||
		(value.Target.Architecture != inventory.ArchitectureARM64 && value.Target.Architecture != inventory.ArchitectureAMD64) ||
		!formulaVersionPattern.MatchString(value.Target.OSVersion) {
		return errors.New("validate Homebrew transaction review: target must be supported macOS")
	}
	if err := validateBaseline(value.Baseline, time.Time{}, value.PreparedAt); err != nil {
		return fmt.Errorf("validate Homebrew transaction review: %w", err)
	}
	if len(value.Actions) == 0 {
		return errors.New("validate Homebrew transaction review: actions are required")
	}
	seenActions := make(map[string]struct{}, len(value.Actions))
	for _, action := range value.Actions {
		if err := validatePreview(action); err != nil {
			return fmt.Errorf("validate Homebrew transaction review: %w", err)
		}
		if _, duplicate := seenActions[action.ActionID]; duplicate {
			return fmt.Errorf("validate Homebrew transaction review: duplicate action %q", action.ActionID)
		}
		seenActions[action.ActionID] = struct{}{}
	}
	expected := cloneTransactionReview(value)
	canonicalizeTransactionReview(&expected)
	summary, err := summarizeTransactions(expected.Actions)
	if err != nil {
		return fmt.Errorf("validate Homebrew transaction review: %w", err)
	}
	expected.Summary = summary
	if !reflect.DeepEqual(value, expected) {
		return errors.New("validate Homebrew transaction review: content is not canonical or summary is invalid")
	}
	expectedID, err := transactionReviewID(value)
	if err != nil || expectedID != value.ID {
		return errors.New("validate Homebrew transaction review: content-derived ID does not match")
	}
	return nil
}

func validateTransactionInput(input TransactionReviewInput) error {
	prefix := "prepare Homebrew transaction review: "
	if input.PreparedAt.IsZero() {
		return errors.New(prefix + "prepared_at is required")
	}
	preparedAt := input.PreparedAt.UTC()
	if err := plan.Validate(input.Plan); err != nil || input.Plan.SchemaVersion != plan.ExecutableSchemaVersion || !input.Plan.Executable || input.Plan.Continuation != nil {
		return errors.New(prefix + "a valid executable Base Plan 0.2.0 is required")
	}
	if preparedAt.Before(input.Plan.CreatedAt) || !preparedAt.Before(input.Plan.ExpiresAt) {
		return errors.New(prefix + "Plan is not active at prepared_at")
	}
	if err := lockfile.Validate(input.Lock); err != nil {
		return fmt.Errorf(prefix+"invalid Lock: %w", err)
	}
	if input.Lock.GeneratedAt.After(input.Plan.CreatedAt) || input.Plan.PolicyDigest != input.Lock.Profile.Digest ||
		input.Lock.Target.OS != inventory.OSMacOS ||
		(input.Lock.Target.Architecture != inventory.ArchitectureARM64 && input.Lock.Target.Architecture != inventory.ArchitectureAMD64) ||
		input.Plan.Environment.OS != string(input.Lock.Target.OS) || input.Plan.Environment.OSVersion != input.Lock.Target.OSVersion ||
		input.Plan.Environment.Architecture != string(input.Lock.Target.Architecture) || input.Plan.Environment.ToolID != "profile.base" ||
		input.Plan.Environment.ActiveManager != "homebrew" {
		return errors.New(prefix + "Plan, Lock and target are not bound")
	}
	if err := validateBaseline(input.Baseline, input.Plan.CreatedAt, preparedAt); err != nil {
		return fmt.Errorf(prefix+"%w", err)
	}
	if input.Baseline.InstallationID != input.Plan.Environment.ActiveInstallationID ||
		input.Baseline.HomebrewVersion != input.Plan.Environment.ActiveVersion {
		return errors.New(prefix + "Homebrew identity changed after Plan creation")
	}
	source, ok := transactionSource(input.Lock, input.Baseline.CatalogSourceID)
	if !ok || source.Digest != input.Baseline.CatalogDigest || !source.SnapshotAt.Equal(input.Baseline.CatalogSnapshotAt) {
		return errors.New(prefix + "Homebrew catalog does not match the locked source")
	}
	if len(input.Actions) != len(input.Plan.Actions) {
		return errors.New(prefix + "transaction previews must exactly cover Plan actions")
	}
	previews := make(map[string]ActionPreview, len(input.Actions))
	for _, preview := range input.Actions {
		if err := validatePreview(preview); err != nil {
			return fmt.Errorf(prefix+"%w", err)
		}
		if _, duplicate := previews[preview.ActionID]; duplicate {
			return fmt.Errorf(prefix+"duplicate action preview %q", preview.ActionID)
		}
		previews[preview.ActionID] = preview
	}
	lockItems := baseLockItems(input.Lock)
	for _, action := range input.Plan.Actions {
		preview, exists := previews[action.ID]
		if !exists {
			return fmt.Errorf(prefix+"action %q has no transaction preview", action.ID)
		}
		packageID := strings.TrimPrefix(action.ToolID, "homebrew.formula.")
		item, exists := lockItems[packageID]
		if !exists || item.Implementation == nil {
			return fmt.Errorf(prefix+"action %q is not backed by a supported Lock item", action.ID)
		}
		implementation := item.Implementation
		if action.ID != "install-base-"+packageID || (packageID != "git" && packageID != "cmake") ||
			action.Operation != "install" || action.Adapter != "homebrew" || action.TargetVersion != implementation.Version ||
			action.Risk != plan.RiskR2 || len(action.Dependencies) != 0 || action.Download.State != "unknown" || action.Download.Bytes != nil ||
			item.State != lockfile.StateInstallRequired || implementation.Manager != "homebrew" ||
			implementation.PackageKind != "formula" || implementation.PackageID != packageID || implementation.SourceID != source.ID ||
			preview.Root.Name != packageID || preview.Root.Version != implementation.Version || preview.Root.State != FormulaInstallRequired {
			return fmt.Errorf(prefix+"action %q does not match its locked Homebrew formula", action.ID)
		}
		if len(action.Preconditions) != 4 || !hasPlanCheck(action.Preconditions, "lock_id_matches", "profile.base", input.Lock.ID) ||
			!hasPlanCheck(action.Preconditions, "lock_source_matches", source.ID, source.Digest) ||
			!hasPlanCheck(action.Preconditions, "package_state_matches", item.Capability, string(lockfile.StateInstallRequired)) ||
			!hasPlanCheck(action.Preconditions, "manager_available", "homebrew", "true") || len(action.Verifications) != 2 ||
			!hasPlanCheck(action.Verifications, "formula_version_installed", packageID, implementation.Version) ||
			!hasPlanCheck(action.Verifications, "lock_target_matches", item.Capability, string(input.Lock.Target.OS)+"/"+string(input.Lock.Target.Architecture)) ||
			!action.Confirmation.Required || action.Confirmation.Scope != "plan" || action.ElevationRequired || action.RestartRequired || action.Recovery.Mode != "manual" {
			return fmt.Errorf(prefix+"action %q safety checks do not match the Base contract", action.ID)
		}
	}
	if _, err := summarizeTransactions(input.Actions); err != nil {
		return fmt.Errorf(prefix+"%w", err)
	}
	return nil
}

func validateBaseline(value TransactionBaseline, earliest, latest time.Time) error {
	if value.ObservedAt.IsZero() || (!earliest.IsZero() && value.ObservedAt.Before(earliest)) || value.ObservedAt.After(latest) ||
		!baselineIdentityPattern.MatchString(value.InstallationID) || !formulaVersionPattern.MatchString(value.HomebrewVersion) ||
		!transactionDigestPattern.MatchString(value.ExecutableDigest) || !transactionDigestPattern.MatchString(value.ConfigurationDigest) ||
		!baselineIdentityPattern.MatchString(value.CatalogSourceID) || !transactionDigestPattern.MatchString(value.CatalogDigest) ||
		value.CatalogSnapshotAt.IsZero() || value.CatalogSnapshotAt.After(value.ObservedAt) {
		return errors.New("Homebrew transaction baseline is invalid or stale")
	}
	for _, key := range value.UnsafeConfigurationKeys {
		if !configurationKeyPattern.MatchString(key) {
			return errors.New("Homebrew configuration assessment contains an invalid key name")
		}
	}
	if !value.ConfigurationSafe || len(value.UnsafeConfigurationKeys) != 0 {
		return errors.New("Homebrew configuration is not safe for a controlled transaction")
	}
	return nil
}

func validatePreview(value ActionPreview) error {
	packageID := strings.TrimPrefix(value.ActionID, "install-base-")
	if (packageID != "git" && packageID != "cmake") || value.ActionID != "install-base-"+packageID || value.Root.Name != packageID {
		return errors.New("transaction action and root formula identity are unsupported")
	}
	if err := validateFormula(value.Root, true); err != nil {
		return fmt.Errorf("action %q root: %w", value.ActionID, err)
	}
	if len(value.Dependencies) > 256 {
		return fmt.Errorf("action %q contains too many dependencies", value.ActionID)
	}
	seen := map[string]struct{}{value.Root.Name: {}}
	for _, dependency := range value.Dependencies {
		if err := validateFormula(dependency, false); err != nil {
			return fmt.Errorf("action %q dependency: %w", value.ActionID, err)
		}
		if _, duplicate := seen[dependency.Name]; duplicate {
			return fmt.Errorf("action %q contains duplicate formula %q", value.ActionID, dependency.Name)
		}
		seen[dependency.Name] = struct{}{}
	}
	return nil
}

func validateFormula(value FormulaPreview, root bool) error {
	if !formulaNamePattern.MatchString(value.Name) || unsafeFormulaName(value.Name) ||
		!formulaVersionPattern.MatchString(value.Version) {
		return errors.New("formula identity or exact version is invalid")
	}
	if value.State != FormulaSatisfied && value.State != FormulaInstallRequired {
		return errors.New("formula state is unsupported")
	}
	if root && value.State != FormulaInstallRequired {
		return errors.New("root formula must require installation")
	}
	if value.State == FormulaSatisfied && value.DownloadBytes != 0 {
		return errors.New("satisfied formula must not contribute download bytes")
	}
	if value.State == FormulaInstallRequired && value.DownloadBytes <= 0 {
		return errors.New("install-required formula needs a known non-zero download size")
	}
	return nil
}

func summarizeTransactions(actions []ActionPreview) (TransactionSummary, error) {
	unique := make(map[string]FormulaPreview)
	for _, action := range actions {
		formulae := append([]FormulaPreview{action.Root}, action.Dependencies...)
		for _, formula := range formulae {
			if prior, exists := unique[formula.Name]; exists && prior != formula {
				return TransactionSummary{}, fmt.Errorf("formula %q has conflicting transaction facts", formula.Name)
			}
			unique[formula.Name] = formula
		}
	}
	result := TransactionSummary{Formulae: len(unique)}
	for _, formula := range unique {
		switch formula.State {
		case FormulaSatisfied:
			result.Satisfied++
		case FormulaInstallRequired:
			result.InstallRequired++
			if result.TotalDownloadBytes > math.MaxInt64-formula.DownloadBytes {
				return TransactionSummary{}, errors.New("formula download size total overflows")
			}
			result.TotalDownloadBytes += formula.DownloadBytes
		}
	}
	return result, nil
}

func transactionSource(value lockfile.Lock, id string) (lockfile.Source, bool) {
	for _, source := range value.Sources {
		if source.ID == id && source.Kind == lockfile.SourcePackageCatalog {
			return source, true
		}
	}
	return lockfile.Source{}, false
}

func baseLockItems(value lockfile.Lock) map[string]lockfile.Item {
	result := make(map[string]lockfile.Item)
	for _, item := range value.Items {
		if item.Module == "base" && item.Implementation != nil && item.Capability == "base."+item.Implementation.PackageID &&
			(item.Implementation.PackageID == "git" || item.Implementation.PackageID == "cmake") &&
			item.Implementation.ToolID == "homebrew.formula."+item.Implementation.PackageID {
			result[item.Implementation.PackageID] = item
		}
	}
	return result
}

func hasPlanCheck(values []plan.Check, kind, subject, expected string) bool {
	for _, value := range values {
		if value.Kind == kind && value.Subject == subject && value.Expected == expected {
			return true
		}
	}
	return false
}

func unsafeFormulaName(value string) bool {
	if strings.Contains(value, "//") {
		return true
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "." || segment == ".." {
			return true
		}
	}
	return false
}

func canonicalizeTransactionReview(value *TransactionReview) {
	value.PreparedAt = value.PreparedAt.UTC()
	value.Baseline.ObservedAt = value.Baseline.ObservedAt.UTC()
	value.Baseline.CatalogSnapshotAt = value.Baseline.CatalogSnapshotAt.UTC()
	value.Baseline.ConfigurationSafe = true
	value.Baseline.UnsafeConfigurationKeys = nil
	for index := range value.Actions {
		sort.Slice(value.Actions[index].Dependencies, func(left, right int) bool {
			return value.Actions[index].Dependencies[left].Name < value.Actions[index].Dependencies[right].Name
		})
	}
	sort.Slice(value.Actions, func(left, right int) bool { return value.Actions[left].ActionID < value.Actions[right].ActionID })
}

func transactionReviewID(value TransactionReview) (string, error) {
	copyValue := cloneTransactionReview(value)
	copyValue.ID = ""
	data, err := json.Marshal(copyValue)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func cloneTransactionBaseline(value TransactionBaseline) TransactionBaseline {
	value.UnsafeConfigurationKeys = append([]string{}, value.UnsafeConfigurationKeys...)
	return value
}

func cloneActionPreviews(values []ActionPreview) []ActionPreview {
	result := make([]ActionPreview, len(values))
	for index, value := range values {
		result[index] = value
		result[index].Dependencies = append([]FormulaPreview{}, value.Dependencies...)
	}
	return result
}

func cloneTransactionReview(value TransactionReview) TransactionReview {
	value.Baseline = cloneTransactionBaseline(value.Baseline)
	value.Actions = cloneActionPreviews(value.Actions)
	return value
}
