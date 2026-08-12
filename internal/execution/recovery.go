package execution

import (
	"context"
	"errors"

	"github.com/gitbagHero/EnvMason/internal/plan"
)

type RecoveryEvidence string

const (
	RecoveryEvidenceChanged   RecoveryEvidence = "changed"
	RecoveryEvidenceUncertain RecoveryEvidence = "uncertain"
)

type RecoveryCandidate struct {
	ActionID        string           `json:"action_id"`
	ToolID          string           `json:"tool_id"`
	Operation       string           `json:"operation"`
	StepState       State            `json:"step_state"`
	Evidence        RecoveryEvidence `json:"evidence"`
	RecoveryMode    string           `json:"recovery_mode"`
	RecoverySummary string           `json:"recovery_summary"`
}

type RecoveryAssessment struct {
	SourceOperationID string              `json:"source_operation_id"`
	SourcePlanID      string              `json:"source_plan_id"`
	Candidates        []RecoveryCandidate `json:"candidates"`
}

type RecoveryCheckpointState string

const (
	RecoveryCheckpointCurrent             RecoveryCheckpointState = "current"
	RecoveryCheckpointDrifted             RecoveryCheckpointState = "drifted"
	RecoveryCheckpointUncertain           RecoveryCheckpointState = "uncertain"
	RecoveryCheckpointVerifierUnavailable RecoveryCheckpointState = "verifier_unavailable"
)

type RecoveryCheckpoint struct {
	RecoveryCandidate
	CurrentState RecoveryCheckpointState `json:"current_state"`
}

type RecoveryRevalidation struct {
	SourceOperationID string               `json:"source_operation_id"`
	SourcePlanID      string               `json:"source_plan_id"`
	Candidates        []RecoveryCheckpoint `json:"candidates"`
}

// AssessRecovery identifies actions from one immutable terminal operation that
// need a future recovery decision. It only interprets confirmed Plan metadata
// and redacted step evidence; it never reads the environment, resolves an
// adapter, writes history, or claims that recovery has occurred.
func AssessRecovery(source Record) (RecoveryAssessment, error) {
	if err := ValidateRecord(source); err != nil {
		return RecoveryAssessment{}, errors.New("assess recovery: source operation record is invalid")
	}
	if source.ConfirmedPlan == nil ||
		(source.SchemaVersion != RecordSchemaVersion &&
			source.SchemaVersion != PreviousRecordSchemaVersion) {
		return RecoveryAssessment{}, errors.New("assess recovery: source record has no confirmed Plan provenance")
	}
	if !terminalState(source.State) {
		return RecoveryAssessment{}, errors.New("assess recovery: source operation must be terminal")
	}
	confirmed, err := cloneConfirmedPlan(*source.ConfirmedPlan)
	if err != nil {
		return RecoveryAssessment{}, errors.New("assess recovery: confirmed Plan could not be copied")
	}
	actions, err := orderedActions(confirmed.Actions)
	if err != nil || len(actions) != len(source.Steps) {
		return RecoveryAssessment{}, errors.New("assess recovery: confirmed Plan actions do not match steps")
	}
	result := RecoveryAssessment{
		SourceOperationID: source.ID,
		SourcePlanID:      source.PlanID,
		Candidates:        []RecoveryCandidate{},
	}
	for index, action := range actions {
		step := source.Steps[index]
		if step.State == StateRunning || step.State == StateVerifying {
			return RecoveryAssessment{}, errors.New("assess recovery: terminal source contains an active step")
		}
		evidence := recoveryEvidence(step)
		if evidence == "" {
			continue
		}
		result.Candidates = append(result.Candidates, RecoveryCandidate{
			ActionID: action.ID, ToolID: action.ToolID,
			Operation: action.Operation, StepState: step.State,
			Evidence: evidence, RecoveryMode: action.Recovery.Mode,
			RecoverySummary: action.Recovery.Summary,
		})
	}
	return result, nil
}

// RevalidateRecovery adds fresh, action-scoped checkpoint evidence to one
// immutable recovery assessment. A successful callback only proves that the
// current action state matches the recorded after-state; this function never
// builds a recovery Plan, executes an action, or writes operation history.
func RevalidateRecovery(
	ctx context.Context,
	source Record,
	registry Registry,
) (RecoveryRevalidation, error) {
	if err := ctx.Err(); err != nil {
		return RecoveryRevalidation{}, err
	}
	assessment, err := AssessRecovery(source)
	if err != nil {
		return RecoveryRevalidation{}, err
	}
	confirmed, err := cloneConfirmedPlan(*source.ConfirmedPlan)
	if err != nil {
		return RecoveryRevalidation{}, errors.New("revalidate recovery: confirmed Plan could not be copied")
	}
	actions, err := orderedActions(confirmed.Actions)
	if err != nil || len(actions) != len(source.Steps) {
		return RecoveryRevalidation{}, errors.New("revalidate recovery: confirmed Plan actions do not match steps")
	}
	actionByID := make(map[string]plan.Action, len(actions))
	stepByID := make(map[string]StepRecord, len(actions))
	for index, action := range actions {
		actionByID[action.ID] = action
		stepByID[action.ID] = source.Steps[index]
	}

	result := RecoveryRevalidation{
		SourceOperationID: assessment.SourceOperationID,
		SourcePlanID:      assessment.SourcePlanID,
		Candidates:        make([]RecoveryCheckpoint, 0, len(assessment.Candidates)),
	}
	for _, candidate := range assessment.Candidates {
		if err := ctx.Err(); err != nil {
			return RecoveryRevalidation{}, err
		}
		checkpoint := RecoveryCheckpoint{RecoveryCandidate: candidate}
		if candidate.Evidence == RecoveryEvidenceUncertain {
			checkpoint.CurrentState = RecoveryCheckpointUncertain
			result.Candidates = append(result.Candidates, checkpoint)
			continue
		}
		action, actionOK := actionByID[candidate.ActionID]
		step, stepOK := stepByID[candidate.ActionID]
		if !actionOK || !stepOK || step.After == nil {
			return RecoveryRevalidation{}, errors.New("revalidate recovery: candidate has no recorded after-state")
		}
		recorded, err := NewSnapshot(step.After.Facts)
		if err != nil || recorded.Digest != step.After.Digest {
			return RecoveryRevalidation{}, errors.New("revalidate recovery: recorded checkpoint evidence is invalid")
		}
		definition, err := registry.Resolve(action)
		if err != nil || definition.RevalidateCheckpoint == nil {
			checkpoint.CurrentState = RecoveryCheckpointVerifierUnavailable
			result.Candidates = append(result.Candidates, checkpoint)
			continue
		}
		observed, err := definition.RevalidateCheckpoint(ctx, action, recorded)
		if contextError := ctx.Err(); contextError != nil {
			return RecoveryRevalidation{}, contextError
		}
		if err != nil {
			checkpoint.CurrentState = RecoveryCheckpointDrifted
			result.Candidates = append(result.Candidates, checkpoint)
			continue
		}
		validated, validationError := NewSnapshot(observed.Facts)
		if validationError != nil || validated.Digest != observed.Digest {
			checkpoint.CurrentState = RecoveryCheckpointDrifted
			result.Candidates = append(result.Candidates, checkpoint)
			continue
		}
		checkpoint.CurrentState = RecoveryCheckpointCurrent
		result.Candidates = append(result.Candidates, checkpoint)
	}
	return result, nil
}

func recoveryEvidence(step StepRecord) RecoveryEvidence {
	if step.State == StatePending {
		return ""
	}
	if len(step.Diff) > 0 {
		return RecoveryEvidenceChanged
	}
	if !step.Skipped && step.Invocation != nil && step.Error != nil &&
		writeProcessMayHaveStarted(step.Error.Code) {
		return RecoveryEvidenceUncertain
	}
	return ""
}

func writeProcessMayHaveStarted(code Code) bool {
	switch code {
	case CodeExitNonZero, CodeAbnormalExit, CodeTimeout, CodeCancelled,
		CodeInterrupted, CodeVerificationFailed, CodeLogWriteFailed:
		return true
	default:
		return false
	}
}
