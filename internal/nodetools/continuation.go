package nodetools

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	adapter "github.com/gitbagHero/EnvMason/internal/adapter/nodetools"
	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/plan"
)

var nodeToolsDigestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

type PreparedContinuation struct {
	Plan plan.Plan
}

// PrepareContinuation loads a terminal Node tools operation and uses one fresh
// environment scan to revalidate completed checkpoints and prepare only the
// remaining actions. It returns a review-only Plan 0.4.0 and never saves
// history or invokes an action Build callback.
func (service Service) PrepareContinuation(ctx context.Context, operationID string) (PreparedContinuation, error) {
	if err := service.validate(); err != nil {
		return PreparedContinuation{}, err
	}
	root, err := service.historyDirectory()
	if err != nil {
		return PreparedContinuation{}, err
	}
	source, err := (execution.FileStore{Root: root}).Load(operationID)
	if err != nil {
		return PreparedContinuation{}, fmt.Errorf("load source Node tools operation: %w", err)
	}
	nodeVersion, sourceTargets, err := continuationSource(source)
	if err != nil {
		return PreparedContinuation{}, err
	}
	value, err := service.Scan(ctx)
	if err != nil {
		return PreparedContinuation{}, fmt.Errorf("scan before Node tools continuation Plan: %w", err)
	}
	baseline, adapterOptions, err := service.inspectTargets(ctx, value, nodeVersion, sourceTargets)
	if err != nil {
		return PreparedContinuation{}, err
	}
	adapterOptions.Targets = sourceTargets
	registry, err := execution.NewRegistry(adapter.Definitions(adapterOptions)...)
	if err != nil {
		return PreparedContinuation{}, err
	}
	assessment, err := execution.AssessContinuation(ctx, source, registry)
	if err != nil {
		return PreparedContinuation{}, err
	}
	if !assessment.Eligible {
		code, actionID := "unknown", ""
		if assessment.Blocker != nil {
			code = string(assessment.Blocker.Code)
			actionID = assessment.Blocker.ActionID
		}
		if actionID != "" {
			return PreparedContinuation{}, fmt.Errorf("prepare Node tools continuation Plan: source is ineligible: %s (%s)", code, actionID)
		}
		return PreparedContinuation{}, fmt.Errorf("prepare Node tools continuation Plan: source is ineligible: %s", code)
	}
	remainingTargets, err := continuationTargets(assessment.RemainingActionIDs, sourceTargets)
	if err != nil {
		return PreparedContinuation{}, err
	}
	prepared, err := buildPreparedPlan(value, baseline, adapterOptions, remainingTargets, service.now())
	if err != nil {
		return PreparedContinuation{}, err
	}
	continuationPlan, err := execution.BuildContinuationPlan(source, assessment, prepared.Plan)
	if err != nil {
		return PreparedContinuation{}, err
	}
	return PreparedContinuation{Plan: continuationPlan}, nil
}

func continuationSource(source execution.Record) (string, adapter.Targets, error) {
	if source.SchemaVersion != execution.RecordSchemaVersion || source.ConfirmedPlan == nil ||
		source.ConfirmedPlan.SchemaVersion != plan.ExecutableSchemaVersion {
		return "", adapter.Targets{}, errors.New("prepare Node tools continuation Plan: source must be Operation Record 0.3.0 with confirmed Plan 0.2.0")
	}
	switch source.State {
	case execution.StateFailed, execution.StateTimedOut, execution.StateCancelled, execution.StateInterrupted:
	default:
		return "", adapter.Targets{}, errors.New("prepare Node tools continuation Plan: source operation must be a terminal failure")
	}
	if len(source.ConfirmedPlan.Actions) == 0 || len(source.ConfirmedPlan.Actions) > 3 ||
		source.ConfirmedPlan.Environment.ToolID != "runtime.node" {
		return "", adapter.Targets{}, errors.New("prepare Node tools continuation Plan: source is not a supported Node tools Plan")
	}
	hasRemaining := false
	for _, step := range source.Steps {
		if step.State != execution.StateCompleted {
			hasRemaining = true
			break
		}
	}
	if !hasRemaining {
		return "", adapter.Targets{}, errors.New("prepare Node tools continuation Plan: source operation has no remaining actions")
	}

	nodeVersion, nodeInstallationID := "", ""
	targets := adapter.Targets{}
	seen := map[string]bool{}
	for _, action := range source.ConfirmedPlan.Actions {
		expectedID, allowedProvider := "", ""
		switch action.ToolID {
		case plan.NodeToolNPM:
			expectedID, allowedProvider = "update-npm", plan.NodeToolProviderNPM
		case plan.NodeToolCorepack:
			expectedID, allowedProvider = "update-corepack", plan.NodeToolProviderNPM
		case plan.NodeToolPNPM:
			expectedID = "update-pnpm"
			if action.Adapter != plan.NodeToolProviderNPM && action.Adapter != plan.NodeToolProviderCorepack {
				return "", adapter.Targets{}, errors.New("prepare Node tools continuation Plan: source pnpm provider is unsupported")
			}
		default:
			return "", adapter.Targets{}, errors.New("prepare Node tools continuation Plan: source contains a non-Node-tools action")
		}
		if seen[action.ToolID] || action.ID != expectedID || action.Operation != "update_version" ||
			action.Risk != plan.RiskR2 || (allowedProvider != "" && action.Adapter != allowedProvider) {
			return "", adapter.Targets{}, errors.New("prepare Node tools continuation Plan: source action identity or risk is unsupported")
		}
		if !action.Confirmation.Required || action.Confirmation.Scope != "plan" ||
			action.ElevationRequired || action.RestartRequired ||
			action.Download.State != "unknown" || action.Download.Bytes != nil ||
			action.Recovery.Mode != "manual" {
			return "", adapter.Targets{}, errors.New("prepare Node tools continuation Plan: source action execution constraints are unsupported")
		}
		seen[action.ToolID] = true
		targetVersion, err := exactVersion(action.TargetVersion)
		if err != nil || targetVersion != action.TargetVersion {
			return "", adapter.Targets{}, errors.New("prepare Node tools continuation Plan: source target version is invalid")
		}
		targetNode, err := requiredActionCheck(action.Preconditions, "target_node_version_installed")
		if err != nil || !strings.HasPrefix(targetNode.Expected, "v") {
			return "", adapter.Targets{}, errors.New("prepare Node tools continuation Plan: source target Node metadata is invalid")
		}
		currentNode, err := exactVersion(strings.TrimPrefix(targetNode.Expected, "v"))
		if err != nil || targetNode.Expected != "v"+currentNode {
			return "", adapter.Targets{}, errors.New("prepare Node tools continuation Plan: source target Node version is invalid")
		}
		if nodeVersion == "" {
			nodeVersion, nodeInstallationID = currentNode, targetNode.Subject
		} else if currentNode != nodeVersion || targetNode.Subject != nodeInstallationID {
			return "", adapter.Targets{}, errors.New("prepare Node tools continuation Plan: source actions do not share one target Node")
		}
		subject := action.ToolID + "@node-v" + currentNode
		if err := validateContinuationActionChecks(
			action, source.ConfirmedPlan.EnvironmentDigest,
			source.ConfirmedPlan.Environment.ActiveInstallationID, source.ConfirmedPlan.Environment.ActiveVersion,
			subject, nodeInstallationID,
		); err != nil {
			return "", adapter.Targets{}, err
		}
		switch action.ToolID {
		case plan.NodeToolNPM:
			targets.NPM = targetVersion
		case plan.NodeToolCorepack:
			targets.Corepack = targetVersion
		case plan.NodeToolPNPM:
			targets.PNPM = targetVersion
		}
	}
	if err := validateContinuationTopology(source.ConfirmedPlan.Actions, seen); err != nil {
		return "", adapter.Targets{}, err
	}
	return nodeVersion, targets, nil
}

func validateContinuationActionChecks(
	action plan.Action,
	environmentDigest, activeInstallationID, activeVersion, subject, nodeInstallationID string,
) error {
	if len(action.Preconditions) < 8 || len(action.Verifications) < 4 {
		return errors.New("prepare Node tools continuation Plan: source action is missing required safety metadata")
	}
	if len(action.Preconditions) > 8 || len(action.Verifications) > 4 {
		return errors.New("prepare Node tools continuation Plan: source action contains unsupported safety metadata")
	}
	checks := []struct {
		kind, expected, subject string
		digest                  bool
	}{
		{kind: "inventory_digest_matches", subject: "runtime.node", expected: environmentDigest},
		{kind: "adapter_script_digest_matches", subject: "nvm.sh", digest: true},
		{kind: "default_alias_digest_matches", subject: "nvm/default", digest: true},
		{kind: "current_tool_version_matches", subject: subject},
		{kind: "tool_provider_matches", subject: subject, expected: action.Adapter},
		{kind: "tool_control_digest_matches", subject: subject, digest: true},
		{kind: "registry_matches", subject: action.ToolID, expected: adapter.RegistryURL},
	}
	for _, required := range checks {
		check, err := requiredActionCheck(action.Preconditions, required.kind)
		if err != nil {
			return errors.New("prepare Node tools continuation Plan: source action is missing required safety metadata")
		}
		if required.expected != "" && check.Expected != required.expected {
			return errors.New("prepare Node tools continuation Plan: source action safety metadata is inconsistent")
		}
		if check.Subject != required.subject {
			return errors.New("prepare Node tools continuation Plan: source action safety scope is inconsistent")
		}
		if required.digest && !nodeToolsDigestPattern.MatchString(check.Expected) {
			return errors.New("prepare Node tools continuation Plan: source action digest metadata is invalid")
		}
		if required.kind == "current_tool_version_matches" {
			if _, err := exactVersion(check.Expected); err != nil {
				return errors.New("prepare Node tools continuation Plan: source current tool version is invalid")
			}
		}
	}
	preconditionDefault, err := requiredActionCheck(action.Preconditions, "default_alias_digest_matches")
	if err != nil {
		return errors.New("prepare Node tools continuation Plan: source default metadata is invalid")
	}
	verifications := []struct {
		kind, subject, expected string
		digest                  bool
	}{
		{kind: "tool_version_matches", subject: subject, expected: action.TargetVersion},
		{kind: "tool_node_owner_matches", subject: action.ToolID, expected: nodeInstallationID},
		{kind: "active_version_unchanged", subject: activeInstallationID, expected: activeVersion},
		{kind: "default_alias_digest_matches", subject: "nvm/default", expected: preconditionDefault.Expected, digest: true},
	}
	for _, required := range verifications {
		check, err := requiredActionCheck(action.Verifications, required.kind)
		if err != nil || check.Subject != required.subject || check.Expected != required.expected ||
			(required.digest && !nodeToolsDigestPattern.MatchString(check.Expected)) {
			return errors.New("prepare Node tools continuation Plan: source verification metadata is inconsistent")
		}
	}
	return nil
}

func validateContinuationTopology(actions []plan.Action, selected map[string]bool) error {
	index := 0
	for _, toolID := range []string{plan.NodeToolNPM, plan.NodeToolCorepack, plan.NodeToolPNPM} {
		if !selected[toolID] {
			continue
		}
		if index >= len(actions) || actions[index].ToolID != toolID {
			return errors.New("prepare Node tools continuation Plan: source action order is unsupported")
		}
		action := actions[index]
		expectedDependencies := []string{}
		switch {
		case toolID == plan.NodeToolCorepack && selected[plan.NodeToolNPM]:
			expectedDependencies = append(expectedDependencies, "update-npm")
		case toolID == plan.NodeToolPNPM && action.Adapter == plan.NodeToolProviderCorepack &&
			selected[plan.NodeToolCorepack]:
			expectedDependencies = append(expectedDependencies, "update-corepack")
		case toolID == plan.NodeToolPNPM && action.Adapter == plan.NodeToolProviderNPM &&
			selected[plan.NodeToolNPM]:
			expectedDependencies = append(expectedDependencies, "update-npm")
		}
		if !slices.Equal(action.Dependencies, expectedDependencies) {
			return errors.New("prepare Node tools continuation Plan: source dependency topology is unsupported")
		}
		index++
	}
	if index != len(actions) {
		return errors.New("prepare Node tools continuation Plan: source action order is unsupported")
	}
	return nil
}

func requiredActionCheck(checks []plan.Check, kind string) (plan.Check, error) {
	result := plan.Check{}
	for _, check := range checks {
		if check.Kind != kind {
			continue
		}
		if result.Kind != "" {
			return plan.Check{}, errors.New("duplicate action check")
		}
		result = check
	}
	if result.Kind == "" || result.Subject == "" || result.Expected == "" {
		return plan.Check{}, errors.New("missing action check")
	}
	return result, nil
}

func continuationTargets(actionIDs []string, source adapter.Targets) (adapter.Targets, error) {
	result := adapter.Targets{}
	seen := map[string]bool{}
	for _, actionID := range actionIDs {
		if seen[actionID] {
			return adapter.Targets{}, errors.New("prepare Node tools continuation Plan: remaining actions are duplicated")
		}
		seen[actionID] = true
		switch actionID {
		case "update-npm":
			result.NPM = source.NPM
		case "update-corepack":
			result.Corepack = source.Corepack
		case "update-pnpm":
			result.PNPM = source.PNPM
		default:
			return adapter.Targets{}, errors.New("prepare Node tools continuation Plan: remaining action is unsupported")
		}
	}
	if result == (adapter.Targets{}) {
		return adapter.Targets{}, errors.New("prepare Node tools continuation Plan: no remaining Node tools target")
	}
	return result, nil
}
