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

func Validate(value Plan) error {
	if value.SchemaVersion != SchemaVersion {
		return fmt.Errorf("validate plan: unsupported schema_version %q", value.SchemaVersion)
	}
	if !digestPattern.MatchString(value.ID) || !digestPattern.MatchString(value.EnvironmentDigest) || !digestPattern.MatchString(value.PolicyDigest) {
		return errors.New("validate plan: ID and digests are required")
	}
	if value.CreatedAt.IsZero() || value.ExpiresAt.Sub(value.CreatedAt) != DefaultTTL {
		return fmt.Errorf("validate plan: expiration must be exactly %s after creation", DefaultTTL)
	}
	if value.Executable {
		return errors.New("validate plan: I13 plans must be non-executable")
	}
	if strings.TrimSpace(value.Summary) == "" || len(value.Actions) == 0 {
		return errors.New("validate plan: summary and actions are required")
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
		if err := validateAction(action); err != nil {
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
	if value.OS == "" || value.OSVersion == "" || value.Architecture == "" || value.ToolID != "runtime.node" ||
		value.ActiveInstallationID == "" || value.ActiveVersion == "" || value.ActiveManager == "" || len(value.Installations) == 0 {
		return errors.New("validate plan: environment summary is incomplete")
	}
	seen := map[string]bool{}
	activeMatches := 0
	for _, item := range value.Installations {
		if item.ID == "" || item.Version == "" || item.Path == "" || item.Manager == "" || item.ActiveState == "" || item.DefaultState == "" || seen[item.ID] {
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

func validateAction(action Action) error {
	if !actionIDPattern.MatchString(action.ID) || action.ToolID != "runtime.node" || action.Operation != "install_version" || action.Adapter != "nvm" || !versioncore.ParseSemVer(action.TargetVersion).Comparable {
		return errors.New("unsupported tool, operation, adapter or target")
	}
	if !validRisk(action.Risk) {
		return fmt.Errorf("unknown risk %q", action.Risk)
	}
	if riskRank(action.Risk) < riskRank(RiskR2) {
		return errors.New("install_version risk cannot be lower than R2")
	}
	if !action.Confirmation.Required || action.Confirmation.Scope != "plan" {
		return errors.New("R2 action requires plan-level confirmation")
	}
	if action.ElevationRequired {
		return errors.New("I13 preview cannot require elevation")
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
		if !validCheckKind(check.Kind) || check.Subject == "" || check.Expected == "" {
			return errors.New("invalid check metadata")
		}
	}
	if action.Recovery.Mode != "manual" || strings.TrimSpace(action.Recovery.Summary) == "" {
		return errors.New("manual recovery metadata is required")
	}
	return nil
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

func validCheckKind(value string) bool {
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
