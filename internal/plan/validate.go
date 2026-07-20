package plan

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	versioncore "github.com/gitbagHero/EnvMason/internal/version"
)

var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var actionIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)
var operationIDPattern = regexp.MustCompile(`^op-[a-f0-9]{32}$`)

func Validate(value Plan) error {
	if value.SchemaVersion != SchemaVersion && value.SchemaVersion != ExecutableSchemaVersion && value.SchemaVersion != HighRiskExecutableSchemaVersion {
		return fmt.Errorf("validate plan: unsupported schema_version %q", value.SchemaVersion)
	}
	if !digestPattern.MatchString(value.ID) || !digestPattern.MatchString(value.EnvironmentDigest) || !digestPattern.MatchString(value.PolicyDigest) {
		return errors.New("validate plan: ID and digests are required")
	}
	if value.CreatedAt.IsZero() || value.ExpiresAt.Sub(value.CreatedAt) != DefaultTTL {
		return fmt.Errorf("validate plan: expiration must be exactly %s after creation", DefaultTTL)
	}
	if value.SchemaVersion == SchemaVersion && value.Executable {
		return errors.New("validate plan: Plan 0.1.0 must be non-executable")
	}
	if value.SchemaVersion == ExecutableSchemaVersion && !value.Executable {
		return errors.New("validate plan: Plan 0.2.0 must be executable")
	}
	if value.SchemaVersion == HighRiskExecutableSchemaVersion && !value.Executable {
		return errors.New("validate plan: Plan 0.3.0 must be executable")
	}
	if strings.TrimSpace(value.Summary) == "" || len(value.Actions) == 0 {
		return errors.New("validate plan: summary and actions are required")
	}
	if value.SchemaVersion == HighRiskExecutableSchemaVersion && len(value.Actions) != 1 {
		return errors.New("validate plan: Plan 0.3.0 requires exactly one R3 action")
	}
	if err := validateEnvironment(value.Environment, value.EnvironmentDigest); err != nil {
		return err
	}

	actions := make(map[string]Action, len(value.Actions))
	for _, action := range value.Actions {
		if action.ID == "" {
			return errors.New("validate plan: action ID is required")
		}
		if _, duplicate := actions[action.ID]; duplicate {
			return fmt.Errorf("validate plan: duplicate action ID %q", action.ID)
		}
		actions[action.ID] = action
		if err := validateAction(value.SchemaVersion, action); err != nil {
			return fmt.Errorf("validate plan: action %q: %w", action.ID, err)
		}
	}
	for _, action := range value.Actions {
		for _, dependency := range action.Dependencies {
			if dependency == action.ID {
				return fmt.Errorf("validate plan: action %q depends on itself", action.ID)
			}
			if _, exists := actions[dependency]; !exists {
				return fmt.Errorf("validate plan: action %q has unknown dependency %q", action.ID, dependency)
			}
		}
	}
	if hasCycle(actions) {
		return errors.New("validate plan: action dependency cycle")
	}
	expectedID, err := planID(value)
	if err != nil {
		return fmt.Errorf("validate plan: calculate ID: %w", err)
	}
	if value.ID != expectedID {
		return errors.New("validate plan: content does not match Plan ID")
	}
	return nil
}

func validateEnvironment(value EnvironmentSummary, digest string) error {
	if value.OS == "" || value.OSVersion == "" || value.Architecture == "" || !identifierPattern.MatchString(value.ToolID) ||
		value.ActiveInstallationID == "" || value.ActiveVersion == "" || !identifierPattern.MatchString(value.ActiveManager) || len(value.Installations) == 0 {
		return errors.New("validate plan: environment summary is incomplete")
	}
	seen := map[string]bool{}
	activeMatches := 0
	for _, item := range value.Installations {
		if item.ID == "" || item.Version == "" || item.Path == "" || !identifierPattern.MatchString(item.Manager) || item.ActiveState == "" || item.DefaultState == "" || seen[item.ID] {
			return errors.New("validate plan: environment installation is incomplete or duplicated")
		}
		if item.ActiveState != "active" && item.ActiveState != "inactive" && item.ActiveState != "unknown" ||
			item.DefaultState != "default" && item.DefaultState != "non_default" && item.DefaultState != "unknown" {
			return errors.New("validate plan: environment installation state is invalid")
		}
		if item.ID == value.ActiveInstallationID && item.Version == value.ActiveVersion && item.Manager == value.ActiveManager && item.ActiveState == "active" {
			activeMatches++
		}
		seen[item.ID] = true
	}
	if activeMatches != 1 {
		return errors.New("validate plan: active environment summary does not match an installation")
	}
	want, err := digestValue(value)
	if err != nil {
		return fmt.Errorf("validate plan: digest environment: %w", err)
	}
	if digest != want {
		return errors.New("validate plan: environment does not match its digest")
	}
	return nil
}

func validateAction(schemaVersion string, action Action) error {
	if !actionIDPattern.MatchString(action.ID) {
		return errors.New("invalid action ID")
	}
	if schemaVersion == SchemaVersion {
		if action.ToolID != "runtime.node" || action.Operation != "install_version" || action.Adapter != "nvm" || !versioncore.ParseSemVer(action.TargetVersion).Comparable {
			return errors.New("unsupported tool, operation, adapter or target")
		}
	} else if !identifierPattern.MatchString(action.ToolID) || !identifierPattern.MatchString(action.Operation) ||
		!identifierPattern.MatchString(action.Adapter) || strings.TrimSpace(action.TargetVersion) == "" {
		return errors.New("invalid declarative action identity or target")
	}
	if schemaVersion == HighRiskExecutableSchemaVersion {
		if action.ToolID != "runtime.node" || action.Adapter != "nvm" ||
			(action.Operation != "set_default" && action.Operation != "restore_default") || !exactStableVersion(action.TargetVersion) {
			return errors.New("Plan 0.3.0 only permits exact Node/NVM default actions")
		}
	}
	if !validRisk(action.Risk) {
		return fmt.Errorf("unknown risk %q", action.Risk)
	}
	if schemaVersion == SchemaVersion && riskRank(action.Risk) < riskRank(RiskR2) {
		return errors.New("install_version risk cannot be lower than R2")
	}
	if schemaVersion == ExecutableSchemaVersion && action.Risk != RiskR1 && action.Risk != RiskR2 {
		return errors.New("Plan 0.2.0 only permits R1 and R2 actions")
	}
	if schemaVersion == HighRiskExecutableSchemaVersion && action.Risk != RiskR3 {
		return errors.New("Plan 0.3.0 requires R3 risk")
	}
	if !action.Confirmation.Required || action.Confirmation.Scope != "plan" {
		return errors.New("action requires Plan-bound confirmation")
	}
	if action.ElevationRequired {
		return errors.New("supported Plan versions cannot require elevation")
	}
	if action.Download.State != "known" && action.Download.State != "unknown" {
		return errors.New("download state is unknown")
	}
	if action.Download.State == "known" && (action.Download.Bytes == nil || *action.Download.Bytes < 0) {
		return errors.New("known download requires non-negative bytes")
	}
	if action.Download.State == "unknown" && action.Download.Bytes != nil {
		return errors.New("unknown download cannot include bytes")
	}
	if schemaVersion == HighRiskExecutableSchemaVersion &&
		(action.Download.State != "known" || action.Download.Bytes == nil || *action.Download.Bytes != 0 || len(action.Dependencies) != 0) {
		return errors.New("Plan 0.3.0 default actions cannot download or depend on another action")
	}
	if len(action.Preconditions) == 0 {
		return errors.New("preconditions are required")
	}
	if len(action.Verifications) == 0 {
		return errors.New("verifications are required")
	}
	dependencies := map[string]bool{}
	for _, dependency := range action.Dependencies {
		if dependency == "" || dependencies[dependency] {
			return errors.New("dependencies must be non-empty and unique")
		}
		dependencies[dependency] = true
	}
	for _, check := range append(append([]Check{}, action.Preconditions...), action.Verifications...) {
		if !validCheckKind(schemaVersion, check.Kind) || check.Subject == "" || check.Expected == "" {
			return errors.New("invalid check metadata")
		}
	}
	if strings.TrimSpace(action.Recovery.Summary) == "" {
		return errors.New("recovery metadata is required")
	}
	if schemaVersion == HighRiskExecutableSchemaVersion {
		if action.Operation == "set_default" && action.Recovery.Mode != "plan" {
			return errors.New("set_default requires recovery Plan metadata")
		}
		if action.Operation == "restore_default" && action.Recovery.Mode != "manual" {
			return errors.New("restore_default requires manual recovery metadata")
		}
	} else if action.Recovery.Mode != "manual" {
		return errors.New("manual recovery metadata is required")
	}
	return nil
}

func exactStableVersion(value string) bool {
	if strings.TrimSpace(value) != value {
		return false
	}
	parsed := versioncore.ParseSemVer(value)
	return parsed.Comparable && parsed.Normalized == strings.TrimPrefix(value, "v") &&
		!strings.Contains(parsed.Normalized, "+") && !strings.Contains(parsed.Normalized, "-")
}

func validRisk(value Risk) bool {
	return value == RiskR0 || value == RiskR1 || value == RiskR2 || value == RiskR3 || value == RiskR4
}

func riskRank(value Risk) int {
	switch value {
	case RiskR0:
		return 0
	case RiskR1:
		return 1
	case RiskR2:
		return 2
	case RiskR3:
		return 3
	case RiskR4:
		return 4
	default:
		return -1
	}
}

func validCheckKind(schemaVersion, value string) bool {
	if schemaVersion == ExecutableSchemaVersion || schemaVersion == HighRiskExecutableSchemaVersion {
		return identifierPattern.MatchString(value)
	}
	switch value {
	case "inventory_digest_matches", "policy_digest_matches", "manager_available", "current_version_matches", "target_version_allowed",
		"version_installed", "existing_versions_retained", "active_version_unchanged":
		return true
	default:
		return false
	}
}

func hasCycle(actions map[string]Action) bool {
	state := make(map[string]uint8, len(actions))
	var visit func(string) bool
	visit = func(id string) bool {
		if state[id] == 1 {
			return true
		}
		if state[id] == 2 {
			return false
		}
		state[id] = 1
		for _, dependency := range actions[id].Dependencies {
			if visit(dependency) {
				return true
			}
		}
		state[id] = 2
		return false
	}
	for id := range actions {
		if visit(id) {
			return true
		}
	}
	return false
}
