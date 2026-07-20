package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/gitbagHero/EnvMason/internal/assessment"
	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/versiondata"
)

type BuildInput struct {
	Inventory inventory.Inventory
	Policy    assessment.Policy
	Versions  versiondata.Result
	CreatedAt time.Time
	TTL       time.Duration
}

func Build(input BuildInput) (Plan, error) {
	if input.CreatedAt.IsZero() {
		return Plan{}, errors.New("build plan: created_at is required")
	}
	if input.TTL == 0 {
		input.TTL = DefaultTTL
	}
	if input.TTL != DefaultTTL {
		return Plan{}, fmt.Errorf("build plan: TTL must be %s", DefaultTTL)
	}
	policy, err := assessment.ResolvePolicy(input.Policy)
	if err != nil {
		return Plan{}, fmt.Errorf("build plan: %w", err)
	}
	input.Policy = policy
	candidate, ok := assessment.NodeUpdateCandidate(assessment.Input{Inventory: input.Inventory, Policy: input.Policy, Versions: input.Versions})
	if !ok {
		return Plan{}, errors.New("build plan: no eligible Node.js update recommendation is available")
	}
	if candidate.ExplicitPin && !input.Versions.Node.HasFreshRelease(candidate.TargetVersion) {
		return Plan{}, errors.New("build plan: pinned Node.js target is not verified by the fresh official release index")
	}

	environment, nvmAvailable, err := summarizeEnvironment(input.Inventory, candidate)
	if err != nil {
		return Plan{}, err
	}
	if !nvmAvailable {
		return Plan{}, errors.New("build plan: NVM is not available for this preview")
	}
	environmentDigest, err := digestValue(environment)
	if err != nil {
		return Plan{}, fmt.Errorf("build plan: digest environment: %w", err)
	}
	policyDigest, err := digestValue(input.Policy)
	if err != nil {
		return Plan{}, fmt.Errorf("build plan: digest policy: %w", err)
	}

	createdAt := input.CreatedAt.UTC()
	value := Plan{
		SchemaVersion: SchemaVersion, CreatedAt: createdAt, ExpiresAt: createdAt.Add(input.TTL), Executable: false,
		Summary:           "Preview installing the selected Node.js version with NVM while retaining existing versions and defaults.",
		EnvironmentDigest: environmentDigest, PolicyDigest: policyDigest, Environment: environment,
		Actions: []Action{{
			ID: "install-node-version", ToolID: "runtime.node", Operation: "install_version", Adapter: "nvm",
			TargetVersion: candidate.TargetVersion, Risk: RiskR2, Dependencies: []string{},
			Confirmation: Confirmation{Required: true, Scope: "plan"}, ElevationRequired: false, RestartRequired: false,
			Download: Download{State: "unknown"},
			Preconditions: []Check{
				{Kind: "inventory_digest_matches", Subject: "runtime.node", Expected: environmentDigest},
				{Kind: "policy_digest_matches", Subject: "runtime.node", Expected: policyDigest},
				{Kind: "manager_available", Subject: "nvm", Expected: "true"},
				{Kind: "current_version_matches", Subject: candidate.InstallationID, Expected: candidate.CurrentVersion},
				{Kind: "target_version_allowed", Subject: "runtime.node", Expected: candidate.TargetVersion},
			},
			Verifications: []Check{
				{Kind: "version_installed", Subject: "runtime.node", Expected: candidate.TargetVersion},
				{Kind: "existing_versions_retained", Subject: "runtime.node", Expected: "true"},
				{Kind: "active_version_unchanged", Subject: candidate.InstallationID, Expected: candidate.CurrentVersion},
			},
			Recovery: Recovery{Mode: "manual", Summary: "Keep the existing active/default version unchanged; any removal of a partially installed target requires a separate R3 Plan and explicit confirmation."},
		}},
	}
	value.ID, err = planID(value)
	if err != nil {
		return Plan{}, fmt.Errorf("build plan: calculate ID: %w", err)
	}
	if err := Validate(value); err != nil {
		return Plan{}, err
	}
	return value, nil
}

// BuildExecutable converts the deterministic Node/NVM preview into the single
// executable I15 action and binds it to the exact reviewed nvm.sh bytes.
func BuildExecutable(input BuildInput, nvmScriptDigest, defaultAliasDigest string) (Plan, error) {
	if !digestPattern.MatchString(nvmScriptDigest) || !digestPattern.MatchString(defaultAliasDigest) {
		return Plan{}, errors.New("build executable plan: valid nvm.sh and default alias digests are required")
	}
	value, err := Build(input)
	if err != nil {
		return Plan{}, err
	}
	value.SchemaVersion = ExecutableSchemaVersion
	value.Executable = true
	value.Summary = "Install the exact selected Node.js version with the existing NVM installation while retaining active, default, and older versions."
	value.Actions[0].Preconditions = append(value.Actions[0].Preconditions,
		Check{Kind: "adapter_script_digest_matches", Subject: "nvm.sh", Expected: nvmScriptDigest},
		Check{Kind: "default_alias_digest_matches", Subject: "nvm/default", Expected: defaultAliasDigest},
	)
	value.Actions[0].Verifications = append(value.Actions[0].Verifications,
		Check{Kind: "default_alias_digest_matches", Subject: "nvm/default", Expected: defaultAliasDigest},
	)
	value.ID, err = planID(value)
	if err != nil {
		return Plan{}, fmt.Errorf("build executable plan: calculate ID: %w", err)
	}
	if err := Validate(value); err != nil {
		return Plan{}, err
	}
	return value, nil
}

func summarizeEnvironment(value inventory.Inventory, candidate assessment.UpdateCandidate) (EnvironmentSummary, bool, error) {
	for _, tool := range value.Tools {
		if tool.ID != "runtime.node" {
			continue
		}
		result := EnvironmentSummary{
			OS: string(value.System.OS), OSVersion: value.System.OSVersion, Architecture: string(value.System.Architecture),
			ToolID: "runtime.node", ActiveInstallationID: candidate.InstallationID, ActiveVersion: candidate.CurrentVersion, ActiveManager: candidate.CurrentManager,
			Installations: make([]InstallationSummary, 0, len(tool.Installations)),
		}
		nvmAvailable, candidateFound := false, false
		for _, item := range tool.Installations {
			if item.Manager == "nvm" {
				nvmAvailable = true
			}
			if item.ID == candidate.InstallationID && item.Version == candidate.CurrentVersion && item.ActiveState == inventory.ActiveStateActive {
				candidateFound = true
			}
			result.Installations = append(result.Installations, InstallationSummary{ID: item.ID, Version: item.Version, Path: item.Path, Manager: item.Manager, ActiveState: string(item.ActiveState), DefaultState: string(item.DefaultState)})
		}
		if !candidateFound {
			return EnvironmentSummary{}, false, errors.New("build plan: update candidate no longer matches the active inventory")
		}
		sort.Slice(result.Installations, func(i, j int) bool { return result.Installations[i].ID < result.Installations[j].ID })
		return result, nvmAvailable, nil
	}
	return EnvironmentSummary{}, false, errors.New("build plan: runtime.node inventory is missing")
}

func planID(value Plan) (string, error) {
	value.ID = ""
	return digestValue(value)
}

func digestValue(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
