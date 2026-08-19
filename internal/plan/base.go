package plan

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/lockfile"
)

const basePlanSummary = "Install the reviewed macOS Base Homebrew formulae while leaving satisfied and out-of-scope capabilities unchanged."

type BaseInstallInput struct {
	Lock      lockfile.Lock
	Inventory inventory.Inventory
	CreatedAt time.Time
}

// BuildBaseInstall builds the candidate Plan for the first macOS Base
// formulae. It cannot be executed by I21-D until a current transaction review
// is bound into a distinct final Plan ID.
func BuildBaseInstall(input BaseInstallInput) (Plan, error) {
	if input.CreatedAt.IsZero() {
		return Plan{}, errors.New("build Base Plan: created_at is required")
	}
	if err := lockfile.Validate(input.Lock); err != nil {
		return Plan{}, fmt.Errorf("build Base Plan: invalid Lock: %w", err)
	}
	createdAt := input.CreatedAt.UTC()
	if input.Lock.GeneratedAt.After(createdAt) {
		return Plan{}, errors.New("build Base Plan: Lock was generated after the Plan creation time")
	}
	if err := validateBaseTarget(input.Lock, input.Inventory, createdAt); err != nil {
		return Plan{}, err
	}
	environment, err := baseEnvironment(input.Inventory)
	if err != nil {
		return Plan{}, err
	}
	environmentDigest, err := digestValue(environment)
	if err != nil {
		return Plan{}, errors.New("build Base Plan: digest environment")
	}
	actions, err := baseActions(input.Lock)
	if err != nil {
		return Plan{}, err
	}
	if len(actions) == 0 {
		return Plan{}, errors.New("build Base Plan: all supported Base formulae are already satisfied")
	}
	value := Plan{
		SchemaVersion:     ExecutableSchemaVersion,
		CreatedAt:         createdAt,
		ExpiresAt:         createdAt.Add(DefaultTTL),
		Executable:        true,
		Summary:           basePlanSummary,
		EnvironmentDigest: environmentDigest,
		PolicyDigest:      input.Lock.Profile.Digest,
		Environment:       environment,
		Actions:           actions,
	}
	value.ID, err = planID(value)
	if err != nil {
		return Plan{}, errors.New("build Base Plan: calculate Plan ID")
	}
	if err := Validate(value); err != nil {
		return Plan{}, err
	}
	return value, nil
}

func validateBaseTarget(value lockfile.Lock, current inventory.Inventory, createdAt time.Time) error {
	if value.Target.OS != inventory.OSMacOS ||
		(value.Target.Architecture != inventory.ArchitectureARM64 && value.Target.Architecture != inventory.ArchitectureAMD64) {
		return errors.New("build Base Plan: Lock target must be supported macOS")
	}
	if current.SchemaVersion != inventory.SchemaVersion || current.GeneratedAt.IsZero() || current.GeneratedAt.After(createdAt) {
		return errors.New("build Base Plan: current Inventory is required")
	}
	if current.System.OS != value.Target.OS || current.System.OSVersion != value.Target.OSVersion ||
		current.System.Architecture != value.Target.Architecture {
		return errors.New("build Base Plan: Inventory system does not match the Lock target")
	}
	return nil
}

func baseEnvironment(value inventory.Inventory) (EnvironmentSummary, error) {
	var result EnvironmentSummary
	active := 0
	for _, tool := range value.Tools {
		if tool.ID != "manager.homebrew" {
			continue
		}
		result = EnvironmentSummary{
			OS: string(value.System.OS), OSVersion: value.System.OSVersion,
			Architecture: string(value.System.Architecture), ToolID: "profile.base",
			Installations: make([]InstallationSummary, 0, len(tool.Installations)),
		}
		for _, item := range tool.Installations {
			if item.ID == "" || item.Version == "" || item.Path == "" || item.Manager != "homebrew" {
				return EnvironmentSummary{}, errors.New("build Base Plan: Homebrew Inventory metadata is incomplete")
			}
			summary := InstallationSummary{
				ID: item.ID, Version: item.Version, Path: item.Path, Manager: item.Manager,
				ActiveState: string(item.ActiveState), DefaultState: string(item.DefaultState),
			}
			result.Installations = append(result.Installations, summary)
			if item.ActiveState == inventory.ActiveStateActive {
				active++
				result.ActiveInstallationID = item.ID
				result.ActiveVersion = item.Version
				result.ActiveManager = item.Manager
			}
		}
	}
	if active != 1 || len(result.Installations) == 0 {
		return EnvironmentSummary{}, errors.New("build Base Plan: one active existing Homebrew installation is required; bootstrap is not supported")
	}
	sort.Slice(result.Installations, func(left, right int) bool {
		return result.Installations[left].ID < result.Installations[right].ID
	})
	return result, nil
}

func baseActions(value lockfile.Lock) ([]Action, error) {
	sources := make(map[string]lockfile.Source, len(value.Sources))
	for _, source := range value.Sources {
		sources[source.ID] = source
	}
	actions := []Action{}
	baseFound := false
	for _, item := range value.Items {
		packageID, supported := map[string]string{
			"base.git":   "git",
			"base.cmake": "cmake",
		}[item.Capability]
		if item.Capability == "base.git" && item.Module == "base" {
			baseFound = true
		}
		if !supported || item.Module != "base" {
			continue
		}
		switch item.State {
		case lockfile.StateSatisfied:
			continue
		case lockfile.StateConflict:
			return nil, fmt.Errorf("build Base Plan: capability %q has an installation conflict", item.Capability)
		case lockfile.StateUnresolved:
			return nil, fmt.Errorf("build Base Plan: capability %q is unresolved", item.Capability)
		case lockfile.StateInstallRequired:
		default:
			return nil, fmt.Errorf("build Base Plan: capability %q has unsupported state %q", item.Capability, item.State)
		}
		if item.Implementation == nil || item.Implementation.Manager != "homebrew" ||
			item.Implementation.PackageKind != "formula" || item.Implementation.PackageID != packageID ||
			item.Implementation.ToolID != "homebrew.formula."+packageID {
			return nil, fmt.Errorf("build Base Plan: capability %q does not use the supported Homebrew formula", item.Capability)
		}
		source, exists := sources[item.Implementation.SourceID]
		if !exists {
			return nil, fmt.Errorf("build Base Plan: capability %q source is unavailable", item.Capability)
		}
		actionID := "install-base-" + packageID
		actions = append(actions, Action{
			ID: actionID, ToolID: item.Implementation.ToolID,
			Operation: "install", Adapter: "homebrew", TargetVersion: item.Implementation.Version,
			Risk: RiskR2, Dependencies: []string{},
			Confirmation:      Confirmation{Required: true, Scope: "plan"},
			ElevationRequired: false, RestartRequired: false,
			Download: Download{State: "unknown"},
			Preconditions: []Check{
				{Kind: "lock_id_matches", Subject: "profile.base", Expected: value.ID},
				{Kind: "lock_source_matches", Subject: source.ID, Expected: source.Digest},
				{Kind: "package_state_matches", Subject: item.Capability, Expected: string(lockfile.StateInstallRequired)},
				{Kind: "manager_available", Subject: "homebrew", Expected: "true"},
			},
			Verifications: []Check{
				{Kind: "formula_version_installed", Subject: packageID, Expected: item.Implementation.Version},
				{Kind: "lock_target_matches", Subject: item.Capability, Expected: string(value.Target.OS) + "/" + string(value.Target.Architecture)},
			},
			Recovery: Recovery{Mode: "manual", Summary: "Retain the installed Homebrew formula; uninstalling it requires a separate R3 Plan and explicit confirmation."},
		})
	}
	if !baseFound {
		return nil, errors.New("build Base Plan: Lock does not contain the Base module")
	}
	sort.Slice(actions, func(left, right int) bool { return actions[left].ID < actions[right].ID })
	return actions, nil
}
