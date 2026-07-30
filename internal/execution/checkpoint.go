package execution

import (
	"context"
	"errors"
)

type ContinuationBlockCode string

const (
	ContinuationBlockUnsupportedRecord    ContinuationBlockCode = "source_schema_unsupported"
	ContinuationBlockSourceNotTerminal    ContinuationBlockCode = "source_not_terminal"
	ContinuationBlockSourceCompleted      ContinuationBlockCode = "source_completed"
	ContinuationBlockNothingToContinue    ContinuationBlockCode = "nothing_to_continue"
	ContinuationBlockCheckpointIncomplete ContinuationBlockCode = "checkpoint_incomplete"
	ContinuationBlockDependencyUnverified ContinuationBlockCode = "checkpoint_dependency_unverified"
	ContinuationBlockVerifierUnavailable  ContinuationBlockCode = "checkpoint_verifier_unavailable"
	ContinuationBlockCheckpointDrifted    ContinuationBlockCode = "checkpoint_drifted"
	ContinuationBlockEvidenceInvalid      ContinuationBlockCode = "checkpoint_evidence_invalid"
)

type ContinuationBlocker struct {
	Code     ContinuationBlockCode
	ActionID string
	Message  string
}

type CheckpointEvidence struct {
	ActionID            string
	RecordedAfterDigest string
	ObservedDigest      string
}

type ContinuationAssessment struct {
	SourceOperationID  string
	SourcePlanID       string
	Eligible           bool
	ReusableActionIDs  []string
	RemainingActionIDs []string
	Checkpoints        []CheckpointEvidence
	Blocker            *ContinuationBlocker
}

// AssessContinuation determines whether a terminal Operation Record has enough
// freshly revalidated evidence to become the source of a future continuation
// Plan. It never builds commands, executes actions, mutates the source record,
// or writes operation history.
func AssessContinuation(ctx context.Context, source Record, registry Registry) (ContinuationAssessment, error) {
	assessment := ContinuationAssessment{
		SourceOperationID: source.ID,
		SourcePlanID:      source.PlanID,
	}
	if err := ValidateRecord(source); err != nil {
		return ContinuationAssessment{}, errors.New("assess continuation: source operation record is invalid")
	}
	if source.SchemaVersion != RecordSchemaVersion || source.ConfirmedPlan == nil {
		return blockContinuation(assessment, ContinuationBlockUnsupportedRecord, "", "source record has no confirmed Plan provenance"), nil
	}
	switch source.State {
	case StateCompleted:
		return blockContinuation(assessment, ContinuationBlockSourceCompleted, "", "source operation is already completed"), nil
	case StateFailed, StateTimedOut, StateCancelled, StateInterrupted:
	default:
		return blockContinuation(assessment, ContinuationBlockSourceNotTerminal, "", "source operation must be terminal before continuation assessment"), nil
	}

	confirmed, err := cloneConfirmedPlan(*source.ConfirmedPlan)
	if err != nil {
		return ContinuationAssessment{}, errors.New("assess continuation: confirmed Plan could not be copied")
	}
	actions, err := orderedActions(confirmed.Actions)
	if err != nil || len(actions) != len(source.Steps) {
		return ContinuationAssessment{}, errors.New("assess continuation: confirmed Plan actions are invalid")
	}
	for index, action := range actions {
		if source.Steps[index].State == StateRunning || source.Steps[index].State == StateVerifying {
			return blockContinuation(assessment, ContinuationBlockSourceNotTerminal, action.ID, "source operation still contains an active step"), nil
		}
		if source.Steps[index].State != StateCompleted {
			assessment.RemainingActionIDs = append(assessment.RemainingActionIDs, action.ID)
		}
	}
	if len(assessment.RemainingActionIDs) == 0 {
		return blockContinuation(assessment, ContinuationBlockNothingToContinue, "", "source operation has no remaining actions"), nil
	}

	reusable := make(map[string]bool, len(actions))
	for index, action := range actions {
		step := source.Steps[index]
		if step.State != StateCompleted {
			continue
		}
		if step.Verification.State != CheckPassed || step.After == nil {
			return blockContinuation(assessment, ContinuationBlockCheckpointIncomplete, action.ID, "completed action has no verified after-state checkpoint"), nil
		}
		for _, dependency := range action.Dependencies {
			if !reusable[dependency] {
				return blockContinuation(assessment, ContinuationBlockDependencyUnverified, action.ID, "completed action depends on an unverified checkpoint"), nil
			}
		}
		definition, err := registry.Resolve(action)
		if err != nil || definition.RevalidateCheckpoint == nil {
			return blockContinuation(assessment, ContinuationBlockVerifierUnavailable, action.ID, "registered action has no checkpoint revalidator"), nil
		}
		recorded, err := NewSnapshot(step.After.Facts)
		if err != nil || recorded.Digest != step.After.Digest {
			return blockContinuation(assessment, ContinuationBlockEvidenceInvalid, action.ID, "recorded checkpoint evidence is invalid"), nil
		}
		if err := ctx.Err(); err != nil {
			return ContinuationAssessment{}, err
		}
		observed, err := definition.RevalidateCheckpoint(ctx, action, recorded)
		if err != nil {
			if contextError := ctx.Err(); contextError != nil {
				return ContinuationAssessment{}, contextError
			}
			return blockContinuation(assessment, ContinuationBlockCheckpointDrifted, action.ID, "completed action no longer matches its checkpoint"), nil
		}
		validated, err := NewSnapshot(observed.Facts)
		if err != nil || validated.Digest != observed.Digest {
			return blockContinuation(assessment, ContinuationBlockEvidenceInvalid, action.ID, "checkpoint revalidator returned invalid evidence"), nil
		}
		reusable[action.ID] = true
		assessment.ReusableActionIDs = append(assessment.ReusableActionIDs, action.ID)
		assessment.Checkpoints = append(assessment.Checkpoints, CheckpointEvidence{
			ActionID: action.ID, RecordedAfterDigest: recorded.Digest, ObservedDigest: observed.Digest,
		})
	}
	assessment.Eligible = true
	return assessment, nil
}

func blockContinuation(assessment ContinuationAssessment, code ContinuationBlockCode, actionID, message string) ContinuationAssessment {
	assessment.Eligible = false
	assessment.Blocker = &ContinuationBlocker{Code: code, ActionID: actionID, Message: message}
	return assessment
}
