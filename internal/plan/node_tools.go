package plan

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
	versioncore "github.com/gitbagHero/EnvMason/internal/version"
)

const (
	NodeToolNPM      = "ecosystem.npm"
	NodeToolCorepack = "ecosystem.corepack"
	NodeToolPNPM     = "ecosystem.pnpm"

	NodeToolProviderNPM      = "npm"
	NodeToolProviderCorepack = "corepack"
)

// NodeToolTarget binds one package-manager update to its current bytes,
// provider and exact reviewed target.
type NodeToolTarget struct {
	ToolID         string `json:"tool_id"`
	CurrentVersion string `json:"current_version"`
	TargetVersion  string `json:"target_version"`
	Provider       string `json:"provider"`
	ControlDigest  string `json:"control_digest"`
}

// NodeToolsInput contains all facts required to build an immutable I17 Plan.
type NodeToolsInput struct {
	Inventory          inventory.Inventory
	CreatedAt          time.Time
	NodeVersion        string
	NVMScriptDigest    string
	DefaultAliasDigest string
	Targets            []NodeToolTarget
}

// BuildNodeTools creates an executable R2 Plan for selected Node-scoped
// package managers. It intentionally reuses Plan 0.2.0 and does not add I18
// checkpoint or continuation semantics.
func BuildNodeTools(input NodeToolsInput) (Plan, error) {
	if input.CreatedAt.IsZero() {
		return Plan{}, errors.New("build Node tools Plan: created_at is required")
	}
	if !exactStableVersion(input.NodeVersion) {
		return Plan{}, errors.New("build Node tools Plan: exact stable target Node.js version is required")
	}
	if !digestPattern.MatchString(input.NVMScriptDigest) || !digestPattern.MatchString(input.DefaultAliasDigest) {
		return Plan{}, errors.New("build Node tools Plan: valid NVM control-file digests are required")
	}
	if len(input.Targets) == 0 {
		return Plan{}, errors.New("build Node tools Plan: at least one package manager must be selected")
	}

	environment, targetInstallationID, err := summarizeNodeToolsEnvironment(input.Inventory, input.NodeVersion)
	if err != nil {
		return Plan{}, err
	}
	targets, err := normalizeNodeToolTargets(input.Targets)
	if err != nil {
		return Plan{}, err
	}
	environmentDigest, err := digestValue(environment)
	if err != nil {
		return Plan{}, fmt.Errorf("build Node tools Plan: digest environment: %w", err)
	}
	policyDigest, err := digestValue(struct {
		NodeVersion        string           `json:"node_version"`
		NodeInstallationID string           `json:"node_installation_id"`
		NVMScriptDigest    string           `json:"nvm_script_digest"`
		DefaultAliasDigest string           `json:"default_alias_digest"`
		Registry           string           `json:"registry"`
		Targets            []NodeToolTarget `json:"targets"`
	}{
		NodeVersion: input.NodeVersion, NodeInstallationID: targetInstallationID,
		NVMScriptDigest: input.NVMScriptDigest, DefaultAliasDigest: input.DefaultAliasDigest,
		Registry: "https://registry.npmjs.org", Targets: targets,
	})
	if err != nil {
		return Plan{}, fmt.Errorf("build Node tools Plan: digest policy: %w", err)
	}

	selected := make(map[string]bool, len(targets))
	for _, target := range targets {
		selected[target.ToolID] = true
	}
	actions := make([]Action, 0, len(targets))
	for _, target := range targets {
		dependencies := []string{}
		switch {
		case target.ToolID == NodeToolCorepack && selected[NodeToolNPM]:
			dependencies = append(dependencies, "update-npm")
		case target.ToolID == NodeToolPNPM && target.Provider == NodeToolProviderCorepack && selected[NodeToolCorepack]:
			dependencies = append(dependencies, "update-corepack")
		case target.ToolID == NodeToolPNPM && target.Provider == NodeToolProviderNPM && selected[NodeToolNPM]:
			dependencies = append(dependencies, "update-npm")
		}
		subject := target.ToolID + "@node-v" + input.NodeVersion
		recovery := "Reinstall the previously recorded exact " + target.ToolID + " version through the same provider."
		if target.CurrentVersion == "unknown" {
			recovery = "The previous provider-managed version is unknown; review the operation record and restore the provider state manually."
		}
		actions = append(actions, Action{
			ID:     "update-" + strings.TrimPrefix(target.ToolID, "ecosystem."),
			ToolID: target.ToolID, Operation: "update_version", Adapter: target.Provider,
			TargetVersion: target.TargetVersion, Risk: RiskR2, Dependencies: dependencies,
			Confirmation:      Confirmation{Required: true, Scope: "plan"},
			ElevationRequired: false, RestartRequired: false, Download: Download{State: "unknown"},
			Preconditions: []Check{
				{Kind: "inventory_digest_matches", Subject: "runtime.node", Expected: environmentDigest},
				{Kind: "adapter_script_digest_matches", Subject: "nvm.sh", Expected: input.NVMScriptDigest},
				{Kind: "default_alias_digest_matches", Subject: "nvm/default", Expected: input.DefaultAliasDigest},
				{Kind: "target_node_version_installed", Subject: targetInstallationID, Expected: "v" + input.NodeVersion},
				{Kind: "current_tool_version_matches", Subject: subject, Expected: target.CurrentVersion},
				{Kind: "tool_provider_matches", Subject: subject, Expected: target.Provider},
				{Kind: "tool_control_digest_matches", Subject: subject, Expected: target.ControlDigest},
				{Kind: "registry_matches", Subject: target.ToolID, Expected: "https://registry.npmjs.org"},
			},
			Verifications: []Check{
				{Kind: "tool_version_matches", Subject: subject, Expected: target.TargetVersion},
				{Kind: "tool_node_owner_matches", Subject: target.ToolID, Expected: targetInstallationID},
				{Kind: "active_version_unchanged", Subject: environment.ActiveInstallationID, Expected: environment.ActiveVersion},
				{Kind: "default_alias_digest_matches", Subject: "nvm/default", Expected: input.DefaultAliasDigest},
			},
			Recovery: Recovery{Mode: "manual", Summary: recovery},
		})
	}

	createdAt := input.CreatedAt.UTC()
	value := Plan{
		SchemaVersion: ExecutableSchemaVersion, CreatedAt: createdAt, ExpiresAt: createdAt.Add(DefaultTTL), Executable: true,
		Summary:           fmt.Sprintf("Update %d selected Node-scoped package manager(s) under NVM Node v%s without changing the active Shell or NVM default.", len(actions), input.NodeVersion),
		EnvironmentDigest: environmentDigest, PolicyDigest: policyDigest, Environment: environment, Actions: actions,
	}
	value.ID, err = planID(value)
	if err != nil {
		return Plan{}, fmt.Errorf("build Node tools Plan: calculate ID: %w", err)
	}
	if err := Validate(value); err != nil {
		return Plan{}, err
	}
	return value, nil
}

func normalizeNodeToolTargets(values []NodeToolTarget) ([]NodeToolTarget, error) {
	result := append([]NodeToolTarget{}, values...)
	sort.Slice(result, func(i, j int) bool { return nodeToolRank(result[i].ToolID) < nodeToolRank(result[j].ToolID) })
	seen := map[string]bool{}
	for index := range result {
		target := &result[index]
		if seen[target.ToolID] || nodeToolRank(target.ToolID) == 99 {
			return nil, fmt.Errorf("build Node tools Plan: unsupported or duplicate tool %q", target.ToolID)
		}
		seen[target.ToolID] = true
		if strings.TrimSpace(target.CurrentVersion) == "" || !exactStableVersion(target.TargetVersion) || !digestPattern.MatchString(target.ControlDigest) {
			return nil, fmt.Errorf("build Node tools Plan: invalid version or control digest for %s", target.ToolID)
		}
		current := versioncore.ParseSemVer(target.CurrentVersion)
		desired := versioncore.ParseSemVer(target.TargetVersion)
		if current.Comparable && versioncore.Compare(current, desired) == versioncore.RelationGreater {
			return nil, fmt.Errorf("build Node tools Plan: target version for %s is older than the installed version", target.ToolID)
		}
		if target.Provider != NodeToolProviderNPM && target.Provider != NodeToolProviderCorepack {
			return nil, fmt.Errorf("build Node tools Plan: unsupported provider %q for %s", target.Provider, target.ToolID)
		}
		if target.ToolID != NodeToolPNPM && target.Provider != NodeToolProviderNPM {
			return nil, fmt.Errorf("build Node tools Plan: %s must be updated through npm", target.ToolID)
		}
	}
	return result, nil
}

func nodeToolRank(toolID string) int {
	switch toolID {
	case NodeToolNPM:
		return 0
	case NodeToolCorepack:
		return 1
	case NodeToolPNPM:
		return 2
	default:
		return 99
	}
}

func summarizeNodeToolsEnvironment(value inventory.Inventory, targetVersion string) (EnvironmentSummary, string, error) {
	for _, tool := range value.Tools {
		if tool.ID != "runtime.node" {
			continue
		}
		result := EnvironmentSummary{
			OS: string(value.System.OS), OSVersion: value.System.OSVersion, Architecture: string(value.System.Architecture),
			ToolID: "runtime.node", Installations: make([]InstallationSummary, 0, len(tool.Installations)),
		}
		targetInstallationID := ""
		for _, item := range tool.Installations {
			if item.ActiveState == inventory.ActiveStateActive {
				if result.ActiveInstallationID != "" {
					return EnvironmentSummary{}, "", errors.New("build Node tools Plan: multiple active Node.js installations were found")
				}
				result.ActiveInstallationID = item.ID
				result.ActiveVersion = item.Version
				result.ActiveManager = item.Manager
			}
			if item.Manager == "nvm" && strings.TrimPrefix(item.Version, "v") == targetVersion {
				targetInstallationID = item.ID
			}
			result.Installations = append(result.Installations, InstallationSummary{
				ID: item.ID, Version: item.Version, Path: item.Path, Manager: item.Manager,
				ActiveState: string(item.ActiveState), DefaultState: string(item.DefaultState),
			})
		}
		if result.ActiveInstallationID == "" {
			return EnvironmentSummary{}, "", errors.New("build Node tools Plan: active Node.js installation is unavailable")
		}
		if targetInstallationID == "" {
			return EnvironmentSummary{}, "", errors.New("build Node tools Plan: target is not an installed NVM Node.js version")
		}
		sort.Slice(result.Installations, func(i, j int) bool { return result.Installations[i].ID < result.Installations[j].ID })
		return result, targetInstallationID, nil
	}
	return EnvironmentSummary{}, "", errors.New("build Node tools Plan: runtime.node inventory is missing")
}
