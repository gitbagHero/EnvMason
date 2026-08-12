package nodetools

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	adapter "github.com/gitbagHero/EnvMason/internal/adapter/nodetools"
	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/plan"
)

var nodeToolsDigestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

type PreparedContinuation struct {
	Plan plan.Plan
}

type continuationContext struct {
	Prepared    PreparedContinuation
	Registry    execution.Registry
	HistoryRoot string
	Baseline    adapter.Baseline
	Targets     adapter.Targets
}

// PrepareContinuation loads a terminal Node tools operation and uses one fresh
// environment scan to revalidate completed checkpoints and prepare only the
// remaining actions. It returns a review-only Plan 0.4.0 and never saves
// history or invokes an action Build callback.
func (service Service) PrepareContinuation(ctx context.Context, operationID string) (PreparedContinuation, error) {
	return service.prepareContinuation(ctx, operationID, service.now())
}

// ReviewRecovery loads one supported terminal-failure source and returns only
// fresh, redacted recovery checkpoint states. It never prepares a recovery
// Plan, invokes an action Build callback, or writes operation history.
func (service Service) ReviewRecovery(
	ctx context.Context,
	operationID string,
) (execution.RecoveryRevalidation, error) {
	if err := service.validate(); err != nil {
		return execution.RecoveryRevalidation{}, err
	}
	root, err := service.historyDirectory()
	if err != nil {
		return execution.RecoveryRevalidation{}, err
	}
	source, err := (execution.FileStore{Root: root}).Load(operationID)
	if err != nil {
		return execution.RecoveryRevalidation{}, fmt.Errorf("load source Node tools operation: %w", err)
	}
	assessment, err := execution.AssessRecovery(source)
	if err != nil {
		return execution.RecoveryRevalidation{}, err
	}
	nodeVersion, targets, err := recoverySource(source)
	if err != nil {
		return execution.RecoveryRevalidation{}, errors.New("review Node tools recovery: source is unsupported")
	}
	hasChanged := false
	for _, candidate := range assessment.Candidates {
		hasChanged = hasChanged || candidate.Evidence == execution.RecoveryEvidenceChanged
	}
	if !hasChanged {
		registry, registryErr := execution.NewRegistry()
		if registryErr != nil {
			return execution.RecoveryRevalidation{}, registryErr
		}
		return execution.RevalidateRecovery(ctx, source, registry)
	}

	value, err := service.Scan(ctx)
	if err != nil {
		return execution.RecoveryRevalidation{}, fmt.Errorf("scan before Node tools recovery review: %w", err)
	}
	_, adapterOptions, err := service.inspectTargets(ctx, value, nodeVersion, targets)
	if err != nil {
		return execution.RecoveryRevalidation{}, err
	}
	adapterOptions.Targets = targets
	registry, err := execution.NewRegistry(adapter.Definitions(adapterOptions)...)
	if err != nil {
		return execution.RecoveryRevalidation{}, err
	}
	return execution.RevalidateRecovery(ctx, source, registry)
}

// RevalidateContinuation rebuilds an already reviewed continuation draft with
// its original time window. Matching the complete Plan ID proves that its
// source, checkpoints, current environment and remaining actions are still the
// facts the user reviewed. It performs no confirmation, history write or
// action execution.
func (service Service) RevalidateContinuation(ctx context.Context, reviewed plan.Plan) error {
	now := service.now()
	if err := validateReviewedContinuation(reviewed, now); err != nil {
		return err
	}
	return service.rebuildReviewedContinuation(ctx, reviewed)
}

// ExecuteContinuation revalidates and executes the final Plan 0.5.0 confirmed
// by the caller. It reuses the one fresh scan and fixed adapter registry that
// rebuilt the same Plan ID, writes a new Operation Record 0.4.0, and never
// mutates its source operation.
func (service Service) ExecuteContinuation(
	ctx context.Context,
	finalPlan plan.Plan,
	receipt execution.ConfirmationReceipt,
) (Result, error) {
	now := service.now()
	if err := validateExecutableContinuation(finalPlan, receipt, now); err != nil {
		return Result{}, err
	}
	prepared, err := service.prepareContinuationContext(
		ctx,
		finalPlan.Continuation.SourceOperationID,
		finalPlan.CreatedAt,
	)
	if err != nil {
		return Result{}, err
	}
	rebuilt, err := plan.BuildExecutableContinuation(prepared.Prepared.Plan)
	if err != nil {
		return Result{}, errors.New("execute Node tools continuation Plan: final Plan could not be rebuilt")
	}
	if rebuilt.ID != finalPlan.ID {
		return Result{}, errors.New("Node tool state changed after final continuation review; generate and confirm a new Plan")
	}
	executor := execution.Executor{
		Registry: prepared.Registry,
		Runner:   service.Runner,
		Store:    execution.FileStore{Root: prepared.HistoryRoot},
		Now:      service.Now,
	}
	record, executeErr := executor.Execute(ctx, execution.Request{
		Plan: finalPlan, Confirmation: receipt,
	})
	result := Result{Record: record}
	if record.ID != "" {
		result.RecordPath = filepath.Join(prepared.HistoryRoot, record.ID+".json")
	}
	if record.ID == "" {
		return result, executeErr
	}
	outcome, outcomeErr := service.continuationOutcome(
		ctx, prepared, finalPlan, record,
	)
	if len(outcome.Actions) > 0 {
		result.Outcome = &outcome
	}
	return result, errors.Join(executeErr, outcomeErr)
}

func validateExecutableContinuation(
	finalPlan plan.Plan,
	receipt execution.ConfirmationReceipt,
	now time.Time,
) error {
	if err := plan.Validate(finalPlan); err != nil {
		return errors.New("execute Node tools continuation Plan: final Plan is invalid")
	}
	if finalPlan.SchemaVersion != plan.ExecutableContinuationSchemaVersion ||
		!finalPlan.Executable || finalPlan.Continuation == nil {
		return errors.New("execute Node tools continuation Plan: executable Plan 0.5.0 is required")
	}
	if now.Before(finalPlan.CreatedAt) {
		return errors.New("execute Node tools continuation Plan: final Plan is not yet valid")
	}
	if !now.Before(finalPlan.ExpiresAt) {
		return errors.New("execute Node tools continuation Plan: final Plan has expired")
	}
	if receipt.Scope != "plan" || receipt.ConfirmedPlanID != finalPlan.ID ||
		receipt.ConfirmedAt.Before(finalPlan.CreatedAt) ||
		receipt.ConfirmedAt.After(now) {
		return errors.New("execute Node tools continuation Plan: current user confirmation is not bound to this final Plan ID")
	}
	return nil
}

func (service Service) continuationOutcome(
	ctx context.Context,
	prepared continuationContext,
	finalPlan plan.Plan,
	record execution.Record,
) (ContinuationOutcome, error) {
	value, err := service.Scan(ctx)
	if err != nil {
		return ContinuationOutcome{}, fmt.Errorf("scan after Node tools continuation execution: %w", err)
	}
	after, _, err := service.inspectTargets(
		ctx,
		value,
		prepared.Baseline.NodeVersion,
		prepared.Targets,
	)
	if err != nil {
		return ContinuationOutcome{}, fmt.Errorf("inspect after Node tools continuation execution: %w", err)
	}
	if after.NodeVersion != prepared.Baseline.NodeVersion ||
		after.NVM.ActiveVersion != prepared.Baseline.NVM.ActiveVersion ||
		after.NVM.ScriptDigest != prepared.Baseline.NVM.ScriptDigest ||
		after.NVM.DefaultAliasDigest != prepared.Baseline.NVM.DefaultAliasDigest {
		return ContinuationOutcome{}, errors.New("verify after Node tools continuation execution: Node or NVM control state changed")
	}
	if len(record.Steps) != len(finalPlan.Actions) {
		return ContinuationOutcome{}, errors.New("verify after Node tools continuation execution: record steps do not match final Plan")
	}
	outcome := ContinuationOutcome{
		Actions: make([]ContinuationActionOutcome, 0, len(finalPlan.Actions)),
	}
	var verificationErr error
	for index, action := range finalPlan.Actions {
		step := record.Steps[index]
		if step.ActionID != action.ID {
			return ContinuationOutcome{}, errors.New("verify after Node tools continuation execution: record action order changed")
		}
		beforeTool := continuationTool(prepared.Baseline, action.ToolID)
		afterTool := continuationTool(after, action.ToolID)
		verified := step.State == execution.StateCompleted &&
			step.Verification.State == execution.CheckPassed &&
			afterTool.Version == action.TargetVersion &&
			afterTool.Provider == action.Adapter
		outcome.Actions = append(outcome.Actions, ContinuationActionOutcome{
			ActionID: action.ID, ToolID: action.ToolID,
			BeforeVersion: beforeTool.Version, AfterVersion: afterTool.Version,
			TargetVersion: action.TargetVersion, Provider: afterTool.Provider,
			State: step.State, Verified: verified,
		})
		if step.State == execution.StateCompleted && !verified {
			verificationErr = errors.New("verify after Node tools continuation execution: completed action no longer matches its exact target")
		}
	}
	return outcome, verificationErr
}

func continuationTool(value adapter.Baseline, toolID string) adapter.Tool {
	switch toolID {
	case plan.NodeToolNPM:
		return value.NPM
	case plan.NodeToolCorepack:
		return value.Corepack
	case plan.NodeToolPNPM:
		return value.PNPM
	default:
		return adapter.Tool{}
	}
}

func validateReviewedContinuation(reviewed plan.Plan, now time.Time) error {
	if err := plan.Validate(reviewed); err != nil {
		return errors.New("revalidate Node tools continuation Plan: reviewed Plan is invalid")
	}
	if reviewed.SchemaVersion != plan.ContinuationSchemaVersion ||
		reviewed.Executable || reviewed.Continuation == nil {
		return errors.New("revalidate Node tools continuation Plan: review-only Plan 0.4.0 is required")
	}
	if now.Before(reviewed.CreatedAt) {
		return errors.New("revalidate Node tools continuation Plan: reviewed Plan is not yet valid")
	}
	if !now.Before(reviewed.ExpiresAt) {
		return errors.New("revalidate Node tools continuation Plan: reviewed Plan has expired")
	}
	return nil
}

func (service Service) rebuildReviewedContinuation(ctx context.Context, reviewed plan.Plan) error {
	fresh, err := service.prepareContinuation(
		ctx, reviewed.Continuation.SourceOperationID, reviewed.CreatedAt,
	)
	if err != nil {
		return err
	}
	if fresh.Plan.ID != reviewed.ID {
		return errors.New("Node tool state changed after continuation review; generate a new Plan")
	}
	return nil
}

func (service Service) prepareContinuation(
	ctx context.Context,
	operationID string,
	createdAt time.Time,
) (PreparedContinuation, error) {
	prepared, err := service.prepareContinuationContext(ctx, operationID, createdAt)
	if err != nil {
		return PreparedContinuation{}, err
	}
	return prepared.Prepared, nil
}

func (service Service) prepareContinuationContext(
	ctx context.Context,
	operationID string,
	createdAt time.Time,
) (continuationContext, error) {
	if err := service.validate(); err != nil {
		return continuationContext{}, err
	}
	root, err := service.historyDirectory()
	if err != nil {
		return continuationContext{}, err
	}
	source, err := (execution.FileStore{Root: root}).Load(operationID)
	if err != nil {
		return continuationContext{}, fmt.Errorf("load source Node tools operation: %w", err)
	}
	nodeVersion, sourceTargets, err := continuationSource(source)
	if err != nil {
		return continuationContext{}, err
	}
	value, err := service.Scan(ctx)
	if err != nil {
		return continuationContext{}, fmt.Errorf("scan before Node tools continuation Plan: %w", err)
	}
	baseline, adapterOptions, err := service.inspectTargets(ctx, value, nodeVersion, sourceTargets)
	if err != nil {
		return continuationContext{}, err
	}
	adapterOptions.Targets = sourceTargets
	registry, err := execution.NewRegistry(adapter.Definitions(adapterOptions)...)
	if err != nil {
		return continuationContext{}, err
	}
	assessment, err := execution.AssessContinuation(ctx, source, registry)
	if err != nil {
		return continuationContext{}, err
	}
	if !assessment.Eligible {
		code, actionID := "unknown", ""
		if assessment.Blocker != nil {
			code = string(assessment.Blocker.Code)
			actionID = assessment.Blocker.ActionID
		}
		if actionID != "" {
			return continuationContext{}, fmt.Errorf("prepare Node tools continuation Plan: source is ineligible: %s (%s)", code, actionID)
		}
		return continuationContext{}, fmt.Errorf("prepare Node tools continuation Plan: source is ineligible: %s", code)
	}
	remainingTargets, err := continuationTargets(assessment.RemainingActionIDs, sourceTargets)
	if err != nil {
		return continuationContext{}, err
	}
	prepared, err := buildPreparedPlan(value, baseline, adapterOptions, remainingTargets, createdAt)
	if err != nil {
		return continuationContext{}, err
	}
	continuationPlan, err := execution.BuildContinuationPlan(source, assessment, prepared.Plan)
	if err != nil {
		return continuationContext{}, err
	}
	return continuationContext{
		Prepared: PreparedContinuation{Plan: continuationPlan},
		Registry: registry, HistoryRoot: root,
		Baseline: baseline, Targets: sourceTargets,
	}, nil
}

func continuationSource(source execution.Record) (string, adapter.Targets, error) {
	nodeVersion, targets, err := nodeToolsSource(source)
	if err != nil {
		return "", adapter.Targets{}, err
	}
	switch source.State {
	case execution.StateFailed, execution.StateTimedOut, execution.StateCancelled, execution.StateInterrupted:
	default:
		return "", adapter.Targets{}, errors.New("prepare Node tools continuation Plan: source operation must be a terminal failure")
	}
	for _, step := range source.Steps {
		if step.State != execution.StateCompleted {
			return nodeVersion, targets, nil
		}
	}
	return "", adapter.Targets{}, errors.New("prepare Node tools continuation Plan: source operation has no remaining actions")
}

func recoverySource(source execution.Record) (string, adapter.Targets, error) {
	nodeVersion, targets, err := nodeToolsSource(source)
	if err != nil {
		return "", adapter.Targets{}, err
	}
	switch source.State {
	case execution.StateCompleted, execution.StateFailed, execution.StateTimedOut,
		execution.StateCancelled, execution.StateInterrupted:
		return nodeVersion, targets, nil
	default:
		return "", adapter.Targets{}, errors.New("review Node tools recovery: source operation must be terminal")
	}
}

func nodeToolsSource(source execution.Record) (string, adapter.Targets, error) {
	if source.ConfirmedPlan == nil ||
		(source.SchemaVersion == execution.PreviousRecordSchemaVersion &&
			source.ConfirmedPlan.SchemaVersion != plan.ExecutableSchemaVersion) ||
		(source.SchemaVersion == execution.RecordSchemaVersion &&
			source.ConfirmedPlan.SchemaVersion != plan.ExecutableSchemaVersion &&
			source.ConfirmedPlan.SchemaVersion != plan.ExecutableContinuationSchemaVersion) ||
		(source.SchemaVersion != execution.RecordSchemaVersion &&
			source.SchemaVersion != execution.PreviousRecordSchemaVersion) {
		return "", adapter.Targets{}, errors.New("prepare Node tools continuation Plan: source must retain a supported executable R1/R2 confirmed Plan")
	}
	if len(source.ConfirmedPlan.Actions) == 0 || len(source.ConfirmedPlan.Actions) > 3 ||
		source.ConfirmedPlan.Environment.ToolID != "runtime.node" {
		return "", adapter.Targets{}, errors.New("prepare Node tools continuation Plan: source is not a supported Node tools Plan")
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
