package baseapply

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/baseinstall"
	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/lockfile"
	"github.com/gitbagHero/EnvMason/internal/profile"
)

const (
	liveBundleSchemaVersion = "0.1.0"
	liveDisposableAck       = "I_UNDERSTAND_THIS_DISPOSABLE_VM_WILL_CHANGE"
	liveCatalogURL          = "https://formulae.brew.sh/api/formula.json"
	liveMaximumCatalogBytes = 64 << 20
)

var liveDigestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// liveBundle is private test evidence, not a product or public persistence
// contract. It deliberately contains no confirmation receipt or registry
// token. The live build-tagged test writes it only outside the repository.
type liveBundle struct {
	SchemaVersion string                       `json:"schema_version"`
	ID            string                       `json:"id"`
	CreatedAt     time.Time                    `json:"created_at"`
	Executable    bool                         `json:"executable"`
	Confirmable   bool                         `json:"confirmable"`
	Profile       profile.Profile              `json:"profile"`
	Prepared      Prepared                     `json:"prepared"`
	CatalogJSON   []byte                       `json:"catalog_json"`
	BottleTag     string                       `json:"bottle_tag"`
	Artifacts     []baseinstall.BottleArtifact `json:"artifacts"`
}

type liveGateInput struct {
	GOOS         string
	Mode         string
	Disposable   string
	BundlePath   string
	Repository   string
	Confirmation string
	Now          time.Time
	Bundle       *liveBundle
}

type liveReviewOutput struct {
	SchemaVersion string                         `json:"schema_version"`
	PlanID        string                         `json:"plan_id"`
	PlanSchema    string                         `json:"plan_schema_version"`
	ReviewID      string                         `json:"review_id"`
	LockID        string                         `json:"lock_id"`
	CreatedAt     time.Time                      `json:"created_at"`
	ExpiresAt     time.Time                      `json:"expires_at"`
	Target        lockfile.Target                `json:"target"`
	Summary       baseinstall.TransactionSummary `json:"summary"`
	Actions       []liveReviewAction             `json:"actions"`
	Confirmation  string                         `json:"confirmation_token"`
}

type liveReviewAction struct {
	ActionID           string `json:"action_id"`
	Formula            string `json:"formula"`
	Version            string `json:"version"`
	Risk               string `json:"risk"`
	DependencyCount    int    `json:"dependency_count"`
	TotalFormulae      int    `json:"total_formulae"`
	TotalDownloadBytes int64  `json:"total_download_bytes"`
}

func validateLiveGate(input liveGateInput) error {
	if input.GOOS != "darwin" {
		return errors.New("I21 live acceptance requires macOS")
	}
	if input.Mode != "prepare" && input.Mode != "apply" {
		return errors.New("I21 live acceptance mode must be prepare or apply")
	}
	if input.Disposable != liveDisposableAck {
		return errors.New("I21 live acceptance requires the exact disposable-VM acknowledgement")
	}
	if !cleanAbsolutePOSIX(input.BundlePath) || !strings.HasSuffix(input.BundlePath, ".json") ||
		!cleanAbsolutePOSIX(input.Repository) || withinPOSIX(input.BundlePath, input.Repository) {
		return errors.New("I21 live acceptance bundle must be an absolute JSON path outside the repository")
	}
	if input.Now.IsZero() {
		return errors.New("I21 live acceptance requires the current time")
	}
	if input.Mode == "prepare" {
		if input.Confirmation != "" || input.Bundle != nil {
			return errors.New("I21 live prepare cannot accept confirmation or an existing bundle")
		}
		return nil
	}
	if input.Bundle == nil || validateLiveBundle(*input.Bundle) != nil {
		return errors.New("I21 live apply requires one valid private bundle")
	}
	if input.Now.Before(input.Bundle.Prepared.Review.PreparedAt) ||
		!input.Now.Before(input.Bundle.Prepared.Plan.ExpiresAt) {
		return errors.New("I21 live apply bundle is not active")
	}
	if input.Confirmation != liveConfirmationToken(input.Bundle.Prepared) {
		return errors.New("I21 live apply confirmation does not bind the final Plan and Review")
	}
	return nil
}

func cleanAbsolutePOSIX(value string) bool {
	return value != "" && path.IsAbs(value) && path.Clean(value) == value && !strings.ContainsAny(value, "\x00\r\n")
}

func withinPOSIX(candidate, root string) bool {
	return candidate == root || strings.HasPrefix(candidate, root+"/")
}

func liveConfirmationToken(prepared Prepared) string {
	return "I21-R2 plan=" + prepared.Plan.ID + " review=" + prepared.Review.ID
}

func buildLiveBundle(
	createdAt time.Time,
	profileValue profile.Profile,
	prepared Prepared,
	catalogJSON []byte,
	bottleTag string,
	artifacts []baseinstall.BottleArtifact,
) (liveBundle, error) {
	value := liveBundle{
		SchemaVersion: liveBundleSchemaVersion,
		CreatedAt:     createdAt.UTC(),
		Executable:    false,
		Confirmable:   false,
		Profile:       profileValue,
		Prepared:      prepared,
		CatalogJSON:   append([]byte{}, catalogJSON...),
		BottleTag:     bottleTag,
		Artifacts:     append([]baseinstall.BottleArtifact{}, artifacts...),
	}
	sort.Slice(value.Artifacts, func(left, right int) bool {
		return value.Artifacts[left].Formula < value.Artifacts[right].Formula
	})
	var err error
	value.ID, err = calculateLiveBundleID(value)
	if err != nil {
		return liveBundle{}, err
	}
	if err := validateLiveBundle(value); err != nil {
		return liveBundle{}, err
	}
	return value, nil
}

func validateLiveBundle(value liveBundle) error {
	if value.SchemaVersion != liveBundleSchemaVersion || !liveDigestPattern.MatchString(value.ID) ||
		value.CreatedAt.IsZero() || value.Executable || value.Confirmable {
		return errors.New("validate I21 live bundle: fixed identity is invalid")
	}
	normalized, err := profile.Normalize(value.Profile)
	if err != nil || !reflect.DeepEqual(normalized, value.Profile) {
		return errors.New("validate I21 live bundle: Profile is not normalized")
	}
	if err := validatePreparedOutcome(value.Prepared); err != nil ||
		!value.CreatedAt.Equal(value.Prepared.Review.PreparedAt) {
		return errors.New("validate I21 live bundle: Prepared evidence is invalid")
	}
	rebuilt, err := lockfile.Build(lockfile.BuildInput{
		GeneratedAt: value.Prepared.Lock.GeneratedAt,
		Profile:     value.Profile,
		Target:      value.Prepared.Lock.Target,
		Sources:     value.Prepared.Lock.Sources,
		Items:       value.Prepared.Lock.Items,
	})
	if err != nil || !reflect.DeepEqual(rebuilt, value.Prepared.Lock) {
		return errors.New("validate I21 live bundle: Profile does not bind the Lock")
	}
	if len(value.CatalogJSON) == 0 || len(value.CatalogJSON) > liveMaximumCatalogBytes {
		return errors.New("validate I21 live bundle: catalog snapshot is empty or oversized")
	}
	source, found := liveCatalogSource(value.Prepared.Lock)
	if !found || source.URI != liveCatalogURL || source.Digest != liveDigest(value.CatalogJSON) ||
		value.Prepared.Review.Baseline.CatalogDigest != source.Digest {
		return errors.New("validate I21 live bundle: catalog snapshot is not bound")
	}
	wantTag, err := liveBottleTag(value.Prepared.Lock.Target)
	if err != nil || value.BottleTag != wantTag ||
		len(value.Artifacts) != value.Prepared.Review.Summary.InstallRequired {
		return errors.New("validate I21 live bundle: bottle evidence is incomplete")
	}
	seen := make(map[string]struct{}, len(value.Artifacts))
	for index, artifact := range value.Artifacts {
		if artifact.Formula == "" || artifact.Version == "" || artifact.BottleTag != value.BottleTag ||
			!liveDigestPattern.MatchString(artifact.SHA256) || artifact.DownloadBytes <= 0 ||
			(index > 0 && value.Artifacts[index-1].Formula >= artifact.Formula) {
			return errors.New("validate I21 live bundle: bottle artifact is invalid or unordered")
		}
		if _, duplicate := seen[artifact.Formula]; duplicate {
			return errors.New("validate I21 live bundle: bottle artifact is duplicated")
		}
		seen[artifact.Formula] = struct{}{}
	}
	wantID, err := calculateLiveBundleID(value)
	if err != nil || wantID != value.ID {
		return errors.New("validate I21 live bundle: content-derived ID does not match")
	}
	return nil
}

func liveCatalogSource(value lockfile.Lock) (lockfile.Source, bool) {
	for _, source := range value.Sources {
		if source.ID == "homebrew-core" {
			return source, true
		}
	}
	return lockfile.Source{}, false
}

func calculateLiveBundleID(value liveBundle) (string, error) {
	value.ID = ""
	data, err := json.Marshal(value)
	if err != nil {
		return "", errors.New("calculate I21 live bundle ID: encode content")
	}
	return liveDigest(data), nil
}

func liveDigest(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func marshalLiveBundle(value liveBundle) ([]byte, error) {
	if err := validateLiveBundle(value); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode I21 live bundle: %w", err)
	}
	return append(data, '\n'), nil
}

func decodeLiveBundle(data []byte) (liveBundle, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value liveBundle
	if err := decoder.Decode(&value); err != nil {
		return liveBundle{}, fmt.Errorf("decode I21 live bundle: %w", err)
	}
	if _, err := decoder.Token(); err == nil || !errors.Is(err, io.EOF) {
		return liveBundle{}, errors.New("decode I21 live bundle: trailing JSON value")
	}
	if err := validateLiveBundle(value); err != nil {
		return liveBundle{}, err
	}
	return value, nil
}

func validateLiveEmptyBase(value inventory.Inventory) error {
	if _, err := inventory.Marshal(value); err != nil || value.SchemaVersion != inventory.SchemaVersion ||
		value.System.OS != inventory.OSMacOS ||
		(value.System.Architecture != inventory.ArchitectureARM64 &&
			value.System.Architecture != inventory.ArchitectureAMD64) {
		return errors.New("I21 live acceptance requires one valid supported macOS Inventory")
	}
	for _, tool := range value.Tools {
		if tool.ID != "homebrew.formula.git" && tool.ID != "homebrew.formula.cmake" {
			continue
		}
		for _, installation := range tool.Installations {
			architectureMatches := installation.Architecture == "" ||
				installation.Architecture == inventory.ArchitectureUnknown ||
				installation.Architecture == value.System.Architecture
			if installation.Manager == "homebrew" && architectureMatches {
				return errors.New("I21 live acceptance requires Git and CMake Homebrew formulae to be absent")
			}
		}
	}
	return nil
}

func liveReview(prepared Prepared) liveReviewOutput {
	result := liveReviewOutput{
		SchemaVersion: "0.1.0",
		PlanID:        prepared.Plan.ID, PlanSchema: prepared.Plan.SchemaVersion,
		ReviewID: prepared.Review.ID, LockID: prepared.Lock.ID,
		CreatedAt: prepared.Plan.CreatedAt, ExpiresAt: prepared.Plan.ExpiresAt,
		Target: prepared.Lock.Target, Summary: prepared.Review.Summary,
		Actions: []liveReviewAction{}, Confirmation: liveConfirmationToken(prepared),
	}
	for _, action := range prepared.Review.Actions {
		totalBytes := action.Root.DownloadBytes
		for _, dependency := range action.Dependencies {
			totalBytes += dependency.DownloadBytes
		}
		result.Actions = append(result.Actions, liveReviewAction{
			ActionID: action.ActionID, Formula: action.Root.Name, Version: action.Root.Version,
			Risk: "R2", DependencyCount: len(action.Dependencies),
			TotalFormulae: 1 + len(action.Dependencies), TotalDownloadBytes: totalBytes,
		})
	}
	return result
}

func liveBottleTag(target lockfile.Target) (string, error) {
	major, _, _ := strings.Cut(target.OSVersion, ".")
	name := map[string]string{"12": "monterey", "13": "ventura", "14": "sonoma", "15": "sequoia", "26": "tahoe"}[major]
	if target.OS != inventory.OSMacOS || name == "" {
		return "", errors.New("unsupported live macOS target")
	}
	switch target.Architecture {
	case inventory.ArchitectureARM64:
		return "arm64_" + name, nil
	case inventory.ArchitectureAMD64:
		return name, nil
	default:
		return "", errors.New("unsupported live macOS architecture")
	}
}

func TestLiveGateRequiresDisposableTwoPhaseBinding(t *testing.T) {
	bundle := validLiveBundle(t)
	valid := liveGateInput{
		GOOS: "darwin", Mode: "apply", Disposable: liveDisposableAck,
		BundlePath: "/private/tmp/envmason-i21/bundle.json", Repository: "/Users/test/EnvMason",
		Confirmation: liveConfirmationToken(bundle.Prepared), Now: bundle.CreatedAt.Add(time.Second), Bundle: &bundle,
	}
	tests := []struct {
		name   string
		mutate func(*liveGateInput)
	}{
		{"non macOS", func(value *liveGateInput) { value.GOOS = "linux" }},
		{"unknown mode", func(value *liveGateInput) { value.Mode = "run" }},
		{"missing disposable acknowledgement", func(value *liveGateInput) { value.Disposable = "yes" }},
		{"relative bundle", func(value *liveGateInput) { value.BundlePath = "bundle.json" }},
		{"bundle inside repository", func(value *liveGateInput) { value.BundlePath = value.Repository + "/bundle.json" }},
		{"wrong confirmation", func(value *liveGateInput) { value.Confirmation += "x" }},
		{"expired Plan", func(value *liveGateInput) { value.Now = bundle.Prepared.Plan.ExpiresAt }},
		{"missing bundle", func(value *liveGateInput) { value.Bundle = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid
			test.mutate(&input)
			if err := validateLiveGate(input); err == nil {
				t.Fatal("unsafe live gate was accepted")
			}
		})
	}
	if err := validateLiveGate(valid); err != nil {
		t.Fatalf("valid apply gate error = %v", err)
	}
	prepare := valid
	prepare.Mode, prepare.Confirmation, prepare.Bundle = "prepare", "", nil
	if err := validateLiveGate(prepare); err != nil {
		t.Fatalf("valid prepare gate error = %v", err)
	}
	prepare.Confirmation = valid.Confirmation
	if err := validateLiveGate(prepare); err == nil {
		t.Fatal("prepare accepted an execution confirmation")
	}
}

func TestLiveBundleStrictlyBindsPrivatePreparedEvidence(t *testing.T) {
	value := validLiveBundle(t)
	data, err := marshalLiveBundle(value)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeLiveBundle(data)
	if err != nil || !reflect.DeepEqual(decoded, value) {
		t.Fatalf("bundle round trip = %#v, %v", decoded, err)
	}
	unknown := append([]byte{}, data[:len(data)-2]...)
	unknown = append(unknown, []byte(",\n  \"command\": \"brew install\"\n}\n")...)
	if _, err := decodeLiveBundle(unknown); err == nil {
		t.Fatal("bundle accepted an executable unknown field")
	}
	trailing := append(append([]byte{}, data...), []byte("{}")...)
	if _, err := decodeLiveBundle(trailing); err == nil {
		t.Fatal("bundle accepted trailing JSON")
	}

	tests := []struct {
		name   string
		mutate func(*liveBundle)
	}{
		{"identity", func(item *liveBundle) { item.ID = applyDigest('f') }},
		{"executable", func(item *liveBundle) { item.Executable = true }},
		{"Profile", func(item *liveBundle) { item.Profile.Name = "changed" }},
		{"Prepared", func(item *liveBundle) { item.Prepared.Review.ID = applyDigest('f') }},
		{"catalog", func(item *liveBundle) { item.CatalogJSON = append(item.CatalogJSON, '\n') }},
		{"bottle tag", func(item *liveBundle) { item.BottleTag = "sonoma" }},
		{"artifact order", func(item *liveBundle) { item.Artifacts[0], item.Artifacts[1] = item.Artifacts[1], item.Artifacts[0] }},
		{"artifact size", func(item *liveBundle) { item.Artifacts[0].DownloadBytes++ }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := value
			changed.CatalogJSON = append([]byte{}, value.CatalogJSON...)
			changed.Artifacts = append([]baseinstall.BottleArtifact{}, value.Artifacts...)
			test.mutate(&changed)
			if err := validateLiveBundle(changed); err == nil {
				t.Fatal("tampered live bundle was accepted")
			}
		})
	}
}

func TestLiveSafetyRejectsNonEmptyBaseAndKeepsReviewRedacted(t *testing.T) {
	bundle := validLiveBundle(t)
	current := applyInventory(bundle.CreatedAt, true)
	if err := validateLiveEmptyBase(current); err != nil {
		t.Fatalf("empty Base Inventory error = %v", err)
	}
	current.Tools = append(current.Tools, outcomeFormulaTool("git", "2.51.0", bundle.CreatedAt))
	if err := validateLiveEmptyBase(current); err == nil {
		t.Fatal("live safety accepted an existing target-architecture Git formula")
	}
	current.Tools[len(current.Tools)-1].Installations[0].Architecture = inventory.ArchitectureAMD64
	if err := validateLiveEmptyBase(current); err != nil {
		t.Fatalf("other-architecture formula incorrectly blocked target VM: %v", err)
	}

	reviewJSON, err := json.Marshal(liveReview(bundle.Prepared))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"/opt/homebrew", "/Users/envmason", "/private/tmp/envmason",
		"HOMEBREW_", "formulae.brew.sh", "catalog_json", "inventory", "stdout", "stderr",
	} {
		if strings.Contains(strings.ToLower(string(reviewJSON)), strings.ToLower(forbidden)) {
			t.Fatalf("live review leaked %q: %s", forbidden, reviewJSON)
		}
	}
}

func validLiveBundle(t *testing.T) liveBundle {
	t.Helper()
	fixture := newApplyFixtureWithCMake(t)
	normalized, err := profile.Normalize(profile.Profile{
		SchemaVersion: profile.SchemaVersion, Name: "base",
		Modules: []profile.Module{{ID: profile.ModuleBase}},
	})
	if err != nil {
		t.Fatal(err)
	}
	value, err := buildLiveBundle(
		fixture.prepared.Review.PreparedAt, normalized, fixture.prepared,
		fixture.snapshots.CatalogJSON, fixture.snapshots.BottleTag, fixture.snapshots.Artifacts,
	)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
