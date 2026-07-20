package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
	versioncore "github.com/gitbagHero/EnvMason/internal/version"
)

// DefaultSetInput contains the local facts that bind one explicit NVM default
// change to the reviewed environment and control-file bytes.
type DefaultSetInput struct {
	Inventory             inventory.Inventory
	CreatedAt             time.Time
	TargetVersion         string
	ScriptDigest          string
	CurrentAliasDigest    string
	CurrentAlias          string
	CurrentDefaultVersion string
}

// DefaultRestoreInput binds a recovery Plan to both the source operation and
// the exact current/original alias states.
type DefaultRestoreInput struct {
	Inventory              inventory.Inventory
	CreatedAt              time.Time
	ScriptDigest           string
	CurrentAliasDigest     string
	CurrentAlias           string
	CurrentDefaultVersion  string
	OriginalAliasDigest    string
	OriginalAlias          string
	OriginalDefaultVersion string
	SourceOperationID      string
	SourcePlanID           string
}

func BuildDefaultSet(input DefaultSetInput) (Plan, error) {
	target, err := normalizeDefaultVersion(input.TargetVersion)
	if err != nil {
		return Plan{}, fmt.Errorf("build default set Plan: %w", err)
	}
	current, err := normalizeDefaultVersion(input.CurrentDefaultVersion)
	if err != nil {
		return Plan{}, fmt.Errorf("build default set Plan: current default: %w", err)
	}
	if err := validateDefaultInputs(input.CreatedAt, input.ScriptDigest, input.CurrentAliasDigest, input.CurrentAlias); err != nil {
		return Plan{}, fmt.Errorf("build default set Plan: %w", err)
	}
	environment, err := defaultEnvironment(input.Inventory, target)
	if err != nil {
		return Plan{}, fmt.Errorf("build default set Plan: %w", err)
	}
	if !nvmVersionInstalled(input.Inventory, current) {
		return Plan{}, errors.New("build default set Plan: current default Node.js version is not an executable NVM installation")
	}
	targetAlias := "v" + target
	return buildDefaultPlan(defaultPlanInput{
		CreatedAt: input.CreatedAt, Environment: environment, Operation: "set_default", ActionID: "set-node-default",
		TargetVersion: target, CurrentAlias: input.CurrentAlias, CurrentAliasDigest: input.CurrentAliasDigest,
		CurrentDefaultVersion: current, DesiredAlias: targetAlias, DesiredAliasDigest: digestAliasValue(targetAlias),
		DesiredDefaultVersion: target, ScriptDigest: input.ScriptDigest, RecoveryMode: "plan",
		RecoverySummary: fmt.Sprintf("Generate and explicitly confirm a new R3 recovery Plan to restore the original NVM default alias %q.", input.CurrentAlias),
	})
}

func BuildDefaultRestore(input DefaultRestoreInput) (Plan, error) {
	current, err := normalizeDefaultVersion(input.CurrentDefaultVersion)
	if err != nil {
		return Plan{}, fmt.Errorf("build default restore Plan: current default: %w", err)
	}
	original, err := normalizeDefaultVersion(input.OriginalDefaultVersion)
	if err != nil {
		return Plan{}, fmt.Errorf("build default restore Plan: original default: %w", err)
	}
	if err := validateDefaultInputs(input.CreatedAt, input.ScriptDigest, input.CurrentAliasDigest, input.CurrentAlias); err != nil {
		return Plan{}, fmt.Errorf("build default restore Plan: %w", err)
	}
	if !digestPattern.MatchString(input.OriginalAliasDigest) || !validAliasValue(input.OriginalAlias) ||
		input.OriginalAliasDigest != digestAliasValue(input.OriginalAlias) {
		return Plan{}, errors.New("build default restore Plan: original alias value and digest are invalid")
	}
	if !operationIDPattern.MatchString(input.SourceOperationID) || !digestPattern.MatchString(input.SourcePlanID) {
		return Plan{}, errors.New("build default restore Plan: valid source operation and Plan IDs are required")
	}
	environment, err := defaultEnvironment(input.Inventory, original)
	if err != nil {
		return Plan{}, fmt.Errorf("build default restore Plan: %w", err)
	}
	return buildDefaultPlan(defaultPlanInput{
		CreatedAt: input.CreatedAt, Environment: environment, Operation: "restore_default", ActionID: "restore-node-default",
		TargetVersion: original, CurrentAlias: input.CurrentAlias, CurrentAliasDigest: input.CurrentAliasDigest,
		CurrentDefaultVersion: current, DesiredAlias: input.OriginalAlias, DesiredAliasDigest: input.OriginalAliasDigest,
		DesiredDefaultVersion: original, ScriptDigest: input.ScriptDigest, RecoveryMode: "manual",
		RecoverySummary:   "If this recovery is no longer desired, generate and explicitly confirm a new set-default R3 Plan.",
		SourceOperationID: input.SourceOperationID, SourcePlanID: input.SourcePlanID,
	})
}

type defaultPlanInput struct {
	CreatedAt                           time.Time
	Environment                         EnvironmentSummary
	Operation, ActionID, TargetVersion  string
	CurrentAlias, CurrentAliasDigest    string
	CurrentDefaultVersion               string
	DesiredAlias, DesiredAliasDigest    string
	DesiredDefaultVersion, ScriptDigest string
	RecoveryMode, RecoverySummary       string
	SourceOperationID, SourcePlanID     string
}

func buildDefaultPlan(input defaultPlanInput) (Plan, error) {
	environmentDigest, err := digestValue(input.Environment)
	if err != nil {
		return Plan{}, fmt.Errorf("build default Plan: digest environment: %w", err)
	}
	policyDigest, err := digestValue(struct {
		Operation          string `json:"operation"`
		TargetVersion      string `json:"target_version"`
		CurrentAlias       string `json:"current_alias"`
		CurrentAliasDigest string `json:"current_alias_digest"`
		DesiredAlias       string `json:"desired_alias"`
		DesiredAliasDigest string `json:"desired_alias_digest"`
		SourceOperationID  string `json:"source_operation_id,omitempty"`
		SourcePlanID       string `json:"source_plan_id,omitempty"`
	}{
		Operation: input.Operation, TargetVersion: input.TargetVersion,
		CurrentAlias: input.CurrentAlias, CurrentAliasDigest: input.CurrentAliasDigest,
		DesiredAlias: input.DesiredAlias, DesiredAliasDigest: input.DesiredAliasDigest,
		SourceOperationID: input.SourceOperationID, SourcePlanID: input.SourcePlanID,
	})
	if err != nil {
		return Plan{}, fmt.Errorf("build default Plan: digest policy: %w", err)
	}
	createdAt := input.CreatedAt.UTC()
	preconditions := []Check{
		{Kind: "inventory_digest_matches", Subject: "runtime.node", Expected: environmentDigest},
		{Kind: "adapter_script_digest_matches", Subject: "nvm.sh", Expected: input.ScriptDigest},
		{Kind: "default_alias_digest_matches", Subject: "nvm/default", Expected: input.CurrentAliasDigest},
		{Kind: "default_alias_value_matches", Subject: "nvm/default", Expected: input.CurrentAlias},
		{Kind: "default_version_matches", Subject: "nvm/default", Expected: "v" + input.CurrentDefaultVersion},
		{Kind: "current_default_version_installed", Subject: "runtime.node", Expected: "v" + input.CurrentDefaultVersion},
		{Kind: "target_version_installed", Subject: "runtime.node", Expected: "v" + input.TargetVersion},
	}
	if input.SourceOperationID != "" {
		preconditions = append(preconditions,
			Check{Kind: "source_operation_matches", Subject: input.SourceOperationID, Expected: input.SourcePlanID},
			Check{Kind: "restore_alias_digest_matches", Subject: input.DesiredAlias, Expected: input.DesiredAliasDigest},
		)
	}
	value := Plan{
		SchemaVersion: HighRiskExecutableSchemaVersion, CreatedAt: createdAt, ExpiresAt: createdAt.Add(DefaultTTL), Executable: true,
		Summary: fmt.Sprintf("Change the NVM default alias from %q (v%s) to %q (v%s) without changing the current Shell or deleting Node versions.",
			input.CurrentAlias, input.CurrentDefaultVersion, input.DesiredAlias, input.DesiredDefaultVersion),
		EnvironmentDigest: environmentDigest, PolicyDigest: policyDigest, Environment: input.Environment,
		Actions: []Action{{
			ID: input.ActionID, ToolID: "runtime.node", Operation: input.Operation, Adapter: "nvm", TargetVersion: input.TargetVersion,
			Risk: RiskR3, Dependencies: []string{}, Confirmation: Confirmation{Required: true, Scope: "plan"},
			ElevationRequired: false, RestartRequired: false, Download: Download{State: "known", Bytes: int64Pointer(0)},
			Preconditions: preconditions,
			Verifications: []Check{
				{Kind: "default_alias_value_matches", Subject: "nvm/default", Expected: input.DesiredAlias},
				{Kind: "default_alias_digest_matches", Subject: "nvm/default", Expected: input.DesiredAliasDigest},
				{Kind: "default_alias_resolves_to", Subject: "nvm/default", Expected: "v" + input.DesiredDefaultVersion},
				{Kind: "new_shell_default_version_matches", Subject: "runtime.node", Expected: "v" + input.DesiredDefaultVersion},
				{Kind: "active_shell_version_unchanged", Subject: input.Environment.ActiveInstallationID, Expected: input.Environment.ActiveVersion},
			},
			Recovery: Recovery{Mode: input.RecoveryMode, Summary: input.RecoverySummary},
		}},
	}
	value.ID, err = planID(value)
	if err != nil {
		return Plan{}, fmt.Errorf("build default Plan: calculate ID: %w", err)
	}
	if err := Validate(value); err != nil {
		return Plan{}, err
	}
	return value, nil
}

func nvmVersionInstalled(value inventory.Inventory, target string) bool {
	for _, tool := range value.Tools {
		if tool.ID != "runtime.node" {
			continue
		}
		for _, item := range tool.Installations {
			if item.Manager != "nvm" {
				continue
			}
			if normalized, err := normalizeDefaultVersion(item.Version); err == nil && normalized == target {
				return true
			}
		}
	}
	return false
}

func defaultEnvironment(value inventory.Inventory, target string) (EnvironmentSummary, error) {
	for _, tool := range value.Tools {
		if tool.ID != "runtime.node" {
			continue
		}
		result := EnvironmentSummary{OS: string(value.System.OS), OSVersion: value.System.OSVersion, Architecture: string(value.System.Architecture), ToolID: "runtime.node"}
		targetFound := false
		for _, item := range tool.Installations {
			result.Installations = append(result.Installations, InstallationSummary{ID: item.ID, Version: item.Version, Path: item.Path, Manager: item.Manager, ActiveState: string(item.ActiveState), DefaultState: string(item.DefaultState)})
			if item.ActiveState == inventory.ActiveStateActive {
				if result.ActiveInstallationID != "" {
					return EnvironmentSummary{}, errors.New("multiple active Node.js installations are ambiguous")
				}
				result.ActiveInstallationID, result.ActiveVersion, result.ActiveManager = item.ID, item.Version, item.Manager
			}
			if item.Manager == "nvm" {
				if normalized, err := normalizeDefaultVersion(item.Version); err == nil && normalized == target {
					targetFound = true
				}
			}
		}
		if result.ActiveInstallationID == "" {
			return EnvironmentSummary{}, errors.New("an active Node.js installation is required")
		}
		if !targetFound {
			return EnvironmentSummary{}, errors.New("target Node.js version is not installed by NVM")
		}
		sort.Slice(result.Installations, func(i, j int) bool { return result.Installations[i].ID < result.Installations[j].ID })
		return result, nil
	}
	return EnvironmentSummary{}, errors.New("runtime.node inventory is missing")
}

func validateDefaultInputs(createdAt time.Time, scriptDigest, aliasDigest, alias string) error {
	if createdAt.IsZero() || !digestPattern.MatchString(scriptDigest) || !digestPattern.MatchString(aliasDigest) {
		return errors.New("created_at and valid NVM control-file digests are required")
	}
	if !validAliasValue(alias) || aliasDigest != digestAliasValue(alias) {
		return errors.New("default alias must be a canonical one-line NVM value matching its digest")
	}
	return nil
}

func normalizeDefaultVersion(value string) (string, error) {
	if strings.TrimSpace(value) != value {
		return "", errors.New("an exact stable semantic version is required")
	}
	parsed := versioncore.ParseSemVer(value)
	if !parsed.Comparable || strings.Contains(parsed.Normalized, "-") || strings.Contains(parsed.Normalized, "+") {
		return "", errors.New("an exact stable semantic version is required")
	}
	return parsed.Normalized, nil
}

func validAliasValue(value string) bool {
	if value == "" || len(value) > 4095 || strings.TrimSpace(value) != value || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
		return false
	}
	for _, character := range value {
		switch {
		case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z', character >= '0' && character <= '9':
		case strings.ContainsRune("*._-/", character):
		default:
			return false
		}
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func digestAliasValue(value string) string {
	digest := sha256.Sum256([]byte(value + "\n"))
	return "sha256:" + hex.EncodeToString(digest[:])
}
