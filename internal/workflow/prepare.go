package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gitbagHero/EnvMason/internal/apply"
	"github.com/gitbagHero/EnvMason/internal/defaultversion"
	"github.com/gitbagHero/EnvMason/internal/nodetools"
	"github.com/gitbagHero/EnvMason/internal/plan"
)

// StagePreparer delegates each stage to its existing deterministic service.
// Implementations must only prepare a Plan; confirmation and execution are
// deliberately outside this contract.
type StagePreparer interface {
	PrepareInstallNode(context.Context, string) (ChildPlan, error)
	PrepareSetDefault(context.Context, string) (ChildPlan, error)
	PrepareNodeTools(context.Context, string, NodeToolTargets) (ChildPlan, error)
}

// ChildPlan is the immutable Plan produced for one workflow stage. Native
// preparers also retain a sealed service-specific context for the later Q3
// execution increment.
type ChildPlan struct {
	Plan   plan.Plan
	native *nativePrepared
}

// PreparedStage contains one newly prepared child Plan and a new Record with
// that exact Plan ID bound to the only Ready stage.
type PreparedStage struct {
	StageID string
	Plan    plan.Plan
	Record  Record
	native  *nativePrepared
}

type nativePrepared struct {
	install    *apply.Prepared
	setDefault *defaultversion.Prepared
	nodeTools  *nodetools.Prepared
}

// NativePreparer adapts the existing I15, I16 and I17 preparation services
// without reimplementing their scans, policy checks or Plan builders.
type NativePreparer struct {
	InstallNode apply.Service
	SetDefault  defaultversion.Service
	NodeTools   nodetools.Service
}

func (value NativePreparer) PrepareInstallNode(
	ctx context.Context,
	targetVersion string,
) (ChildPlan, error) {
	prepared, err := value.InstallNode.Prepare(ctx, apply.Options{
		ToolID:  "runtime.node",
		Version: targetVersion,
		Online:  true,
	})
	if err != nil {
		return ChildPlan{}, err
	}
	return ChildPlan{
		Plan: clonePlan(prepared.Plan),
		native: &nativePrepared{
			install: &prepared,
		},
	}, nil
}

func (value NativePreparer) PrepareSetDefault(
	ctx context.Context,
	targetVersion string,
) (ChildPlan, error) {
	prepared, err := value.SetDefault.PrepareSet(ctx, defaultversion.SetOptions{
		ToolID:  "runtime.node",
		Version: targetVersion,
	})
	if err != nil {
		return ChildPlan{}, err
	}
	return ChildPlan{
		Plan: clonePlan(prepared.Plan),
		native: &nativePrepared{
			setDefault: &prepared,
		},
	}, nil
}

func (value NativePreparer) PrepareNodeTools(
	ctx context.Context,
	nodeVersion string,
	targets NodeToolTargets,
) (ChildPlan, error) {
	prepared, err := value.NodeTools.Prepare(ctx, nodetools.Options{
		NodeVersion:     nodeVersion,
		NPMVersion:      targets.NPM,
		CorepackVersion: targets.Corepack,
		PNPMVersion:     targets.PNPM,
	})
	if err != nil {
		return ChildPlan{}, err
	}
	return ChildPlan{
		Plan: clonePlan(prepared.Plan),
		native: &nativePrepared{
			nodeTools: &prepared,
		},
	}, nil
}

// PrepareNext prepares exactly one Plan for the only Ready stage. It returns a
// new Record and never mutates, confirms, executes or persists its inputs.
func PrepareNext(
	ctx context.Context,
	manifest Manifest,
	record Record,
	preparer StagePreparer,
) (PreparedStage, error) {
	if err := ValidateManifest(manifest); err != nil {
		return PreparedStage{}, errors.New("prepare workflow stage: manifest is invalid")
	}
	if err := ValidateRecord(record, manifest); err != nil {
		return PreparedStage{}, errors.New("prepare workflow stage: record is invalid")
	}
	if preparer == nil {
		return PreparedStage{}, errors.New("prepare workflow stage: stage preparer is required")
	}
	if err := ctx.Err(); err != nil {
		return PreparedStage{}, err
	}
	ready := -1
	for index, stage := range record.Stages {
		if stage.State != StageReady {
			continue
		}
		if ready >= 0 {
			return PreparedStage{}, errors.New("prepare workflow stage: multiple stages are ready")
		}
		ready = index
	}
	if ready < 0 || terminalWorkflowState(record.State) {
		return PreparedStage{}, errors.New("prepare workflow stage: no stage is ready")
	}

	var (
		child ChildPlan
		err   error
	)
	switch record.Stages[ready].ID {
	case StageInstallNode:
		child, err = preparer.PrepareInstallNode(
			ctx,
			manifest.TargetNodeVersion,
		)
	case StageSetDefault:
		child, err = preparer.PrepareSetDefault(
			ctx,
			manifest.TargetNodeVersion,
		)
	case StageUpdateNodeTools:
		child, err = preparer.PrepareNodeTools(
			ctx,
			manifest.TargetNodeVersion,
			manifest.NodeTools,
		)
	default:
		return PreparedStage{}, errors.New("prepare workflow stage: ready stage is unsupported")
	}
	if err != nil {
		return PreparedStage{}, fmt.Errorf(
			"prepare workflow stage %s: %w",
			record.Stages[ready].ID,
			err,
		)
	}
	if err := validateChildPlan(
		child.Plan,
		manifest,
		manifest.Stages[ready],
		record.UpdatedAt,
	); err != nil {
		return PreparedStage{}, err
	}
	if !validNativeChild(child, record.Stages[ready].ID) {
		return PreparedStage{}, errors.New(
			"prepare workflow stage: native context does not match the child Plan",
		)
	}
	next, err := BindStagePlan(
		record,
		manifest,
		record.Stages[ready].ID,
		child.Plan.ID,
		child.Plan.CreatedAt,
	)
	if err != nil {
		return PreparedStage{}, err
	}
	return PreparedStage{
		StageID: record.Stages[ready].ID,
		Plan:    clonePlan(child.Plan),
		Record:  next,
		native:  child.native,
	}, nil
}

func validateChildPlan(
	value plan.Plan,
	manifest Manifest,
	stage Stage,
	notBefore time.Time,
) error {
	if err := plan.Validate(value); err != nil {
		return errors.New("prepare workflow stage: child Plan is invalid")
	}
	if value.SchemaVersion != stage.PlanSchemaVersion ||
		!value.Executable ||
		value.Continuation != nil ||
		value.CreatedAt.Before(notBefore) {
		return errors.New("prepare workflow stage: child Plan Schema or freshness is invalid")
	}
	switch stage.ID {
	case StageInstallNode:
		if len(value.Actions) != 1 ||
			!matchesAction(
				value.Actions[0],
				"install-node-version",
				"runtime.node",
				"install_version",
				"nvm",
				manifest.TargetNodeVersion,
				plan.RiskR2,
			) {
			return errors.New("prepare workflow stage: install Plan does not match the Manifest")
		}
	case StageSetDefault:
		if len(value.Actions) != 1 ||
			!matchesAction(
				value.Actions[0],
				"set-node-default",
				"runtime.node",
				"set_default",
				"nvm",
				manifest.TargetNodeVersion,
				plan.RiskR3,
			) {
			return errors.New("prepare workflow stage: default Plan does not match the Manifest")
		}
	case StageUpdateNodeTools:
		if err := validateNodeToolsPlan(value, manifest); err != nil {
			return err
		}
	default:
		return errors.New("prepare workflow stage: child Plan stage is unsupported")
	}
	return nil
}

func validateNodeToolsPlan(value plan.Plan, manifest Manifest) error {
	type expectedAction struct {
		toolID   string
		target   string
		adapters map[string]bool
	}
	expected := make([]expectedAction, 0, 3)
	if manifest.NodeTools.NPM != "" {
		expected = append(expected, expectedAction{
			toolID: plan.NodeToolNPM,
			target: manifest.NodeTools.NPM,
			adapters: map[string]bool{
				plan.NodeToolProviderNPM: true,
			},
		})
	}
	if manifest.NodeTools.Corepack != "" {
		expected = append(expected, expectedAction{
			toolID: plan.NodeToolCorepack,
			target: manifest.NodeTools.Corepack,
			adapters: map[string]bool{
				plan.NodeToolProviderNPM: true,
			},
		})
	}
	if manifest.NodeTools.PNPM != "" {
		expected = append(expected, expectedAction{
			toolID: plan.NodeToolPNPM,
			target: manifest.NodeTools.PNPM,
			adapters: map[string]bool{
				plan.NodeToolProviderNPM:      true,
				plan.NodeToolProviderCorepack: true,
			},
		})
	}
	if len(value.Actions) != len(expected) ||
		!hasNVMInstallation(value, manifest.TargetNodeVersion) {
		return errors.New("prepare workflow stage: Node tools Plan does not match the Manifest")
	}
	for index, expected := range expected {
		action := value.Actions[index]
		dependencies := []string{}
		switch {
		case expected.toolID == plan.NodeToolCorepack &&
			manifest.NodeTools.NPM != "":
			dependencies = []string{"update-npm"}
		case expected.toolID == plan.NodeToolPNPM &&
			action.Adapter == plan.NodeToolProviderCorepack &&
			manifest.NodeTools.Corepack != "":
			dependencies = []string{"update-corepack"}
		case expected.toolID == plan.NodeToolPNPM &&
			action.Adapter == plan.NodeToolProviderNPM &&
			manifest.NodeTools.NPM != "":
			dependencies = []string{"update-npm"}
		}
		if !matchesAction(
			action,
			"update-"+strings.TrimPrefix(expected.toolID, "ecosystem."),
			expected.toolID,
			"update_version",
			action.Adapter,
			expected.target,
			plan.RiskR2,
		) ||
			!expected.adapters[action.Adapter] ||
			!stringsEqual(action.Dependencies, dependencies) ||
			!hasCheck(
				action.Preconditions,
				"target_node_version_installed",
				"v"+manifest.TargetNodeVersion,
			) {
			return errors.New("prepare workflow stage: Node tools Plan does not match the Manifest")
		}
	}
	return nil
}

func matchesAction(
	value plan.Action,
	id, toolID, operation, adapter, targetVersion string,
	risk plan.Risk,
) bool {
	return value.ID == id &&
		value.ToolID == toolID &&
		value.Operation == operation &&
		value.Adapter == adapter &&
		value.TargetVersion == targetVersion &&
		value.Risk == risk &&
		value.Confirmation.Required &&
		value.Confirmation.Scope == "plan"
}

func hasNVMInstallation(value plan.Plan, targetVersion string) bool {
	for _, installation := range value.Environment.Installations {
		if installation.Manager == "nvm" &&
			strings.TrimPrefix(installation.Version, "v") == targetVersion {
			return true
		}
	}
	return false
}

func validNativeChild(value ChildPlan, stageID string) bool {
	if value.native == nil {
		return true
	}
	var nativePlan *plan.Plan
	switch stageID {
	case StageInstallNode:
		if value.native.install == nil ||
			value.native.setDefault != nil ||
			value.native.nodeTools != nil {
			return false
		}
		nativePlan = &value.native.install.Plan
	case StageSetDefault:
		if value.native.install != nil ||
			value.native.setDefault == nil ||
			value.native.nodeTools != nil {
			return false
		}
		nativePlan = &value.native.setDefault.Plan
	case StageUpdateNodeTools:
		if value.native.install != nil ||
			value.native.setDefault != nil ||
			value.native.nodeTools == nil {
			return false
		}
		nativePlan = &value.native.nodeTools.Plan
	default:
		return false
	}
	return plan.Validate(*nativePlan) == nil && nativePlan.ID == value.Plan.ID
}

func hasCheck(values []plan.Check, kind, expected string) bool {
	for _, value := range values {
		if value.Kind == kind && value.Expected == expected {
			return true
		}
	}
	return false
}

func clonePlan(value plan.Plan) plan.Plan {
	result := value
	result.Environment.Installations = append(
		[]plan.InstallationSummary{},
		value.Environment.Installations...,
	)
	result.Actions = append([]plan.Action{}, value.Actions...)
	for index := range result.Actions {
		if value.Actions[index].Download.Bytes != nil {
			bytes := *value.Actions[index].Download.Bytes
			result.Actions[index].Download.Bytes = &bytes
		}
		result.Actions[index].Dependencies = append(
			[]string{},
			value.Actions[index].Dependencies...,
		)
		result.Actions[index].Preconditions = append(
			[]plan.Check{},
			value.Actions[index].Preconditions...,
		)
		result.Actions[index].Verifications = append(
			[]plan.Check{},
			value.Actions[index].Verifications...,
		)
	}
	if value.Continuation != nil {
		continuation := *value.Continuation
		continuation.SourceActionIDs = append(
			[]string{},
			value.Continuation.SourceActionIDs...,
		)
		continuation.ReusableCheckpoints = append(
			[]plan.CheckpointBinding{},
			value.Continuation.ReusableCheckpoints...,
		)
		continuation.SatisfiedDependencies = append(
			[]plan.SatisfiedDependency{},
			value.Continuation.SatisfiedDependencies...,
		)
		result.Continuation = &continuation
	}
	return result
}
