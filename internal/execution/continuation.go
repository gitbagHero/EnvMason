package execution

import (
	"errors"
	"slices"

	"github.com/gitbagHero/EnvMason/internal/plan"
)

// BuildContinuationPlan binds a successful checkpoint assessment to a fresh
// Plan 0.2.0 containing exactly the remaining R1/R2 actions. The result is a
// review-only Plan 0.4.0; this function performs no registry resolution,
// history write or process execution.
func BuildContinuationPlan(source Record, assessment ContinuationAssessment, prepared plan.Plan) (plan.Plan, error) {
	if err := ValidateRecord(source); err != nil {
		return plan.Plan{}, errors.New("build continuation Plan: source operation record is invalid")
	}
	if source.SchemaVersion != RecordSchemaVersion || source.ConfirmedPlan == nil {
		return plan.Plan{}, errors.New("build continuation Plan: source operation has no confirmed Plan provenance")
	}
	if source.State != StateFailed && source.State != StateTimedOut &&
		source.State != StateCancelled && source.State != StateInterrupted {
		return plan.Plan{}, errors.New("build continuation Plan: source operation is not an eligible terminal failure")
	}
	if source.ConfirmedPlan.SchemaVersion != plan.ExecutableSchemaVersion {
		return plan.Plan{}, errors.New("build continuation Plan: only an R1/R2 Plan 0.2.0 source is supported")
	}
	if !assessment.Eligible || assessment.Blocker != nil ||
		assessment.SourceOperationID != source.ID || assessment.SourcePlanID != source.PlanID {
		return plan.Plan{}, errors.New("build continuation Plan: checkpoint assessment does not match the source operation")
	}
	if err := plan.Validate(prepared); err != nil ||
		prepared.SchemaVersion != plan.ExecutableSchemaVersion || !prepared.Executable ||
		prepared.Continuation != nil {
		return plan.Plan{}, errors.New("build continuation Plan: freshly prepared Plan must be executable Plan 0.2.0")
	}
	if !prepared.CreatedAt.After(source.UpdatedAt) {
		return plan.Plan{}, errors.New("build continuation Plan: freshly prepared Plan does not postdate the terminal source state")
	}

	sourceActions, err := orderedActions(source.ConfirmedPlan.Actions)
	if err != nil || !sameActionOrder(sourceActions, source.Steps) {
		return plan.Plan{}, errors.New("build continuation Plan: source action order is invalid")
	}
	preparedActions, err := orderedActions(prepared.Actions)
	if err != nil || !samePlanActionOrder(preparedActions, prepared.Actions) {
		return plan.Plan{}, errors.New("build continuation Plan: freshly prepared action order is invalid")
	}

	sourceActionIDs := make([]string, 0, len(sourceActions))
	reusableActionIDs := make([]string, 0, len(sourceActions))
	remainingActionIDs := make([]string, 0, len(sourceActions))
	sourceByID := make(map[string]plan.Action, len(sourceActions))
	sourceStepByID := make(map[string]StepRecord, len(source.Steps))
	reusable := make(map[string]bool, len(sourceActions))
	remaining := make(map[string]bool, len(sourceActions))
	for index, action := range sourceActions {
		sourceActionIDs = append(sourceActionIDs, action.ID)
		sourceByID[action.ID] = action
		sourceStepByID[action.ID] = source.Steps[index]
		if source.Steps[index].State == StateCompleted {
			reusableActionIDs = append(reusableActionIDs, action.ID)
			reusable[action.ID] = true
		} else {
			remainingActionIDs = append(remainingActionIDs, action.ID)
			remaining[action.ID] = true
		}
	}
	if len(remainingActionIDs) == 0 ||
		!slices.Equal(assessment.ReusableActionIDs, reusableActionIDs) ||
		!slices.Equal(assessment.RemainingActionIDs, remainingActionIDs) ||
		len(assessment.Checkpoints) != len(reusableActionIDs) {
		return plan.Plan{}, errors.New("build continuation Plan: checkpoint assessment action partition is invalid")
	}
	if len(preparedActions) != len(remainingActionIDs) {
		return plan.Plan{}, errors.New("build continuation Plan: freshly prepared Plan does not contain exactly the remaining actions")
	}

	checkpoints := make([]plan.CheckpointBinding, 0, len(assessment.Checkpoints))
	for index, evidence := range assessment.Checkpoints {
		actionID := reusableActionIDs[index]
		sourceStep := sourceStepByID[actionID]
		if evidence.ActionID != actionID || sourceStep.ActionID != actionID || sourceStep.After == nil ||
			evidence.RecordedAfterDigest != sourceStep.After.Digest ||
			!planIDPattern.MatchString(evidence.RecordedAfterDigest) ||
			!planIDPattern.MatchString(evidence.ObservedDigest) {
			return plan.Plan{}, errors.New("build continuation Plan: checkpoint evidence does not match the source operation")
		}
		checkpoints = append(checkpoints, plan.CheckpointBinding{
			ActionID: evidence.ActionID, RecordedAfterDigest: evidence.RecordedAfterDigest,
			ObservedDigest: evidence.ObservedDigest,
		})
	}

	satisfied := make([]plan.SatisfiedDependency, 0)
	for index, freshAction := range preparedActions {
		if freshAction.ID != remainingActionIDs[index] {
			return plan.Plan{}, errors.New("build continuation Plan: freshly prepared actions do not preserve remaining action order")
		}
		sourceAction := sourceByID[freshAction.ID]
		if !sameContinuationActionIdentity(sourceAction, freshAction) {
			return plan.Plan{}, errors.New("build continuation Plan: freshly prepared action identity or target changed")
		}
		expectedDependencies := make([]string, 0, len(sourceAction.Dependencies))
		for _, dependency := range sourceAction.Dependencies {
			switch {
			case remaining[dependency]:
				expectedDependencies = append(expectedDependencies, dependency)
			case reusable[dependency]:
				satisfied = append(satisfied, plan.SatisfiedDependency{
					ActionID: freshAction.ID, DependencyActionID: dependency,
				})
			default:
				return plan.Plan{}, errors.New("build continuation Plan: source dependency is outside the continuation partition")
			}
		}
		if !slices.Equal(freshAction.Dependencies, expectedDependencies) {
			return plan.Plan{}, errors.New("build continuation Plan: freshly prepared dependencies do not match the source DAG")
		}
	}

	return plan.BuildContinuationDraft(plan.ContinuationDraftInput{
		SourceOperationID: source.ID, SourcePlanID: source.PlanID,
		SourceActionIDs: sourceActionIDs, PreparedPlan: prepared,
		ReusableCheckpoints: checkpoints, SatisfiedDependencies: satisfied,
	})
}

func sameActionOrder(actions []plan.Action, steps []StepRecord) bool {
	if len(actions) != len(steps) {
		return false
	}
	for index := range actions {
		if actions[index].ID != steps[index].ActionID {
			return false
		}
	}
	return true
}

func samePlanActionOrder(ordered, original []plan.Action) bool {
	if len(ordered) != len(original) {
		return false
	}
	for index := range ordered {
		if ordered[index].ID != original[index].ID {
			return false
		}
	}
	return true
}

func sameContinuationActionIdentity(source, prepared plan.Action) bool {
	return source.ID == prepared.ID &&
		source.ToolID == prepared.ToolID &&
		source.Operation == prepared.Operation &&
		source.Adapter == prepared.Adapter &&
		source.TargetVersion == prepared.TargetVersion &&
		source.Risk == prepared.Risk &&
		source.Confirmation == prepared.Confirmation &&
		source.ElevationRequired == prepared.ElevationRequired &&
		source.RestartRequired == prepared.RestartRequired
}
