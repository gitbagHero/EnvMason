package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/gitbagHero/EnvMason/internal/apply"
	"github.com/gitbagHero/EnvMason/internal/defaultversion"
	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/nodetools"
	"github.com/gitbagHero/EnvMason/internal/plan"
)

// StageExecutor delegates an already prepared stage to the corresponding
// existing execution service. It cannot create or alter a confirmation.
type StageExecutor interface {
	ExecuteInstallNode(
		context.Context,
		apply.Prepared,
		execution.ConfirmationReceipt,
	) (execution.Record, error)
	ExecuteSetDefault(
		context.Context,
		defaultversion.Prepared,
		execution.ConfirmationReceipt,
	) (execution.Record, error)
	ExecuteNodeTools(
		context.Context,
		nodetools.Prepared,
		execution.ConfirmationReceipt,
	) (execution.Record, error)
}

// NativeStageExecutor adapts the existing I15, I16 and I17 execution services.
type NativeStageExecutor struct {
	InstallNode apply.Service
	SetDefault  defaultversion.Service
	NodeTools   nodetools.Service
}

func (value NativeStageExecutor) ExecuteInstallNode(
	ctx context.Context,
	prepared apply.Prepared,
	receipt execution.ConfirmationReceipt,
) (execution.Record, error) {
	result, err := value.InstallNode.Execute(ctx, prepared, receipt)
	return result.Record, err
}

func (value NativeStageExecutor) ExecuteSetDefault(
	ctx context.Context,
	prepared defaultversion.Prepared,
	receipt execution.ConfirmationReceipt,
) (execution.Record, error) {
	result, err := value.SetDefault.Execute(ctx, prepared, receipt)
	return result.Record, err
}

func (value NativeStageExecutor) ExecuteNodeTools(
	ctx context.Context,
	prepared nodetools.Prepared,
	receipt execution.ConfirmationReceipt,
) (execution.Record, error) {
	result, err := value.NodeTools.Execute(ctx, prepared, receipt)
	return result.Record, err
}

// ExecutedStage contains the terminal Operation Record and a new Workflow
// Record that binds its identity and canonical digest.
type ExecutedStage struct {
	StageID   string
	Plan      plan.Plan
	Operation execution.Record
	Record    Record
}

// ExecutePrepared validates one independently confirmed child Plan, delegates
// it to its native service and attaches any valid terminal Operation Record.
// A rejection before Operation creation leaves the Workflow Record Planned.
func ExecutePrepared(
	ctx context.Context,
	manifest Manifest,
	prepared PreparedStage,
	receipt execution.ConfirmationReceipt,
	executor StageExecutor,
) (ExecutedStage, error) {
	result := executionResultBase(prepared)
	if err := validatePreparedStage(manifest, prepared, true); err != nil {
		return result, err
	}
	if receipt.Scope != "plan" ||
		receipt.ConfirmedPlanID != prepared.Plan.ID ||
		receipt.ConfirmedAt.Before(prepared.Plan.CreatedAt) {
		return result, errors.New(
			"execute workflow stage: current confirmation does not match the child Plan",
		)
	}
	if executor == nil {
		return result, errors.New("execute workflow stage: stage executor is required")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}

	var (
		operation  execution.Record
		executeErr error
	)
	switch prepared.StageID {
	case StageInstallNode:
		operation, executeErr = executor.ExecuteInstallNode(
			ctx,
			*prepared.native.install,
			receipt,
		)
	case StageSetDefault:
		operation, executeErr = executor.ExecuteSetDefault(
			ctx,
			*prepared.native.setDefault,
			receipt,
		)
	case StageUpdateNodeTools:
		operation, executeErr = executor.ExecuteNodeTools(
			ctx,
			*prepared.native.nodeTools,
			receipt,
		)
	default:
		return result, errors.New("execute workflow stage: unsupported stage")
	}
	if operation.ID == "" {
		if executeErr == nil {
			executeErr = errors.New(
				"execute workflow stage: executor returned no Operation Record",
			)
		}
		return result, executeErr
	}
	attached, attachErr := AttachOperation(manifest, prepared, operation)
	if attachErr != nil {
		return result, errors.Join(executeErr, attachErr)
	}
	return attached, executeErr
}

// AttachOperation deterministically replays one terminal Operation Record into
// the Workflow state machine. It is also the recovery boundary for a valid
// Interrupted Operation Record observed after a process restart.
func AttachOperation(
	manifest Manifest,
	prepared PreparedStage,
	operation execution.Record,
) (ExecutedStage, error) {
	result := executionResultBase(prepared)
	if err := validatePreparedStage(manifest, prepared, false); err != nil {
		return result, err
	}
	data, err := execution.MarshalRecord(operation)
	if err != nil {
		return result, fmt.Errorf(
			"attach workflow Operation: Operation Record is invalid: %w",
			err,
		)
	}
	copied, err := execution.DecodeRecord(data)
	if err != nil {
		return result, errors.New(
			"attach workflow Operation: Operation Record could not be copied",
		)
	}
	if copied.PlanID != prepared.Plan.ID ||
		copied.PlanSchemaVersion != prepared.Plan.SchemaVersion ||
		copied.ConfirmedPlan == nil ||
		copied.ConfirmedPlan.ID != prepared.Plan.ID ||
		copied.FinishedAt == nil {
		return result, errors.New(
			"attach workflow Operation: Operation does not match the child Plan",
		)
	}
	terminal, ok := workflowTerminalState(copied.State)
	if !ok {
		return result, errors.New(
			"attach workflow Operation: terminal Operation Record is required",
		)
	}
	running, err := StartStage(
		prepared.Record,
		manifest,
		prepared.StageID,
		copied.ID,
		copied.CreatedAt,
	)
	if err != nil {
		return result, fmt.Errorf("attach workflow Operation: %w", err)
	}
	digest := sha256.Sum256(data)
	finished, err := FinishStage(
		running,
		manifest,
		prepared.StageID,
		terminal,
		"sha256:"+hex.EncodeToString(digest[:]),
		*copied.FinishedAt,
	)
	if err != nil {
		return result, fmt.Errorf("attach workflow Operation: %w", err)
	}
	return ExecutedStage{
		StageID:   prepared.StageID,
		Plan:      clonePlan(prepared.Plan),
		Operation: copied,
		Record:    finished,
	}, nil
}

func validatePreparedStage(
	manifest Manifest,
	prepared PreparedStage,
	requireNative bool,
) error {
	if err := ValidateManifest(manifest); err != nil {
		return errors.New("validate prepared workflow stage: manifest is invalid")
	}
	if err := ValidateRecord(prepared.Record, manifest); err != nil {
		return errors.New("validate prepared workflow stage: Record is invalid")
	}
	index := stageIndex(prepared.StageID)
	if index < 0 ||
		prepared.Record.Stages[index].State != StagePlanned ||
		prepared.Record.Stages[index].PlanID != prepared.Plan.ID ||
		prepared.Record.Stages[index].PlanSchemaVersion !=
			prepared.Plan.SchemaVersion ||
		!prepared.Record.UpdatedAt.Equal(prepared.Plan.CreatedAt) {
		return errors.New(
			"validate prepared workflow stage: Plan binding is invalid",
		)
	}
	if err := validateChildPlan(
		prepared.Plan,
		manifest,
		manifest.Stages[index],
		prepared.Record.UpdatedAt,
	); err != nil {
		return err
	}
	if requireNative && prepared.native == nil {
		return errors.New(
			"validate prepared workflow stage: native context is required",
		)
	}
	if prepared.native != nil && !validNativeChild(
		ChildPlan{Plan: prepared.Plan, native: prepared.native},
		prepared.StageID,
	) {
		return errors.New(
			"validate prepared workflow stage: native context is invalid",
		)
	}
	return nil
}

func workflowTerminalState(value execution.State) (StageState, bool) {
	switch value {
	case execution.StateCompleted:
		return StageCompleted, true
	case execution.StateFailed:
		return StageFailed, true
	case execution.StateTimedOut:
		return StageTimedOut, true
	case execution.StateCancelled:
		return StageCancelled, true
	case execution.StateInterrupted:
		return StageInterrupted, true
	default:
		return "", false
	}
}

func executionResultBase(prepared PreparedStage) ExecutedStage {
	return ExecutedStage{
		StageID: prepared.StageID,
		Plan:    clonePlan(prepared.Plan),
		Record:  cloneRecord(prepared.Record),
	}
}
