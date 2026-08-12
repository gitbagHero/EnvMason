package plan

import (
	"errors"
	"fmt"
)

// ContinuationDraftInput contains already-validated history provenance and a
// freshly prepared Plan for only the remaining R1/R2 actions. History and
// checkpoint eligibility are validated by the execution package before this
// deterministic Plan-layer constructor is called.
type ContinuationDraftInput struct {
	SourceOperationID     string
	SourcePlanID          string
	SourceActionIDs       []string
	PreparedPlan          Plan
	ReusableCheckpoints   []CheckpointBinding
	SatisfiedDependencies []SatisfiedDependency
}

// BuildContinuationDraft creates a review-only Plan 0.4.0. It reuses the
// freshly prepared Plan's time window, environment, policy and declarative
// remaining actions, but gives the continuation draft a new content-derived ID.
func BuildContinuationDraft(input ContinuationDraftInput) (Plan, error) {
	if err := Validate(input.PreparedPlan); err != nil {
		return Plan{}, errors.New("build continuation Plan: freshly prepared Plan is invalid")
	}
	if input.PreparedPlan.SchemaVersion != ExecutableSchemaVersion ||
		!input.PreparedPlan.Executable || input.PreparedPlan.Continuation != nil {
		return Plan{}, errors.New("build continuation Plan: freshly prepared Plan must be executable Plan 0.2.0")
	}

	data, err := Marshal(input.PreparedPlan)
	if err != nil {
		return Plan{}, errors.New("build continuation Plan: freshly prepared Plan could not be copied")
	}
	value, err := Decode(data)
	if err != nil {
		return Plan{}, errors.New("build continuation Plan: freshly prepared Plan could not be copied")
	}
	value.SchemaVersion = ContinuationSchemaVersion
	value.ID = ""
	value.Executable = false
	value.Summary = fmt.Sprintf(
		"Review continuing %d remaining action(s) from a terminal operation after revalidating %d checkpoint(s).",
		len(value.Actions), len(input.ReusableCheckpoints),
	)
	value.Continuation = &Continuation{
		SourceOperationID:     input.SourceOperationID,
		SourcePlanID:          input.SourcePlanID,
		PreparedPlanID:        input.PreparedPlan.ID,
		SourceActionIDs:       append([]string{}, input.SourceActionIDs...),
		ReusableCheckpoints:   append([]CheckpointBinding{}, input.ReusableCheckpoints...),
		SatisfiedDependencies: append([]SatisfiedDependency{}, input.SatisfiedDependencies...),
	}
	value.ID, err = planID(value)
	if err != nil {
		return Plan{}, errors.New("build continuation Plan: Plan ID could not be calculated")
	}
	if err := Validate(value); err != nil {
		return Plan{}, fmt.Errorf("build continuation Plan: %w", err)
	}
	return value, nil
}

// BuildExecutableContinuation deterministically promotes one valid review-only
// continuation draft into the final executable Plan that a future caller can
// present for confirmation. This pure transformation never confirms or
// executes the Plan and does not share mutable state with its input.
func BuildExecutableContinuation(draft Plan) (Plan, error) {
	if err := Validate(draft); err != nil {
		return Plan{}, errors.New("build executable continuation Plan: draft is invalid")
	}
	if draft.SchemaVersion != ContinuationSchemaVersion ||
		draft.Executable || draft.Continuation == nil {
		return Plan{}, errors.New("build executable continuation Plan: review-only Plan 0.4.0 is required")
	}
	data, err := Marshal(draft)
	if err != nil {
		return Plan{}, errors.New("build executable continuation Plan: draft could not be copied")
	}
	value, err := Decode(data)
	if err != nil {
		return Plan{}, errors.New("build executable continuation Plan: draft could not be copied")
	}
	value.SchemaVersion = ExecutableContinuationSchemaVersion
	value.ID = ""
	value.Executable = true
	value.Summary = fmt.Sprintf(
		"Continue %d remaining action(s) from a terminal operation after revalidating %d checkpoint(s).",
		len(value.Actions), len(value.Continuation.ReusableCheckpoints),
	)
	value.ID, err = planID(value)
	if err != nil {
		return Plan{}, errors.New("build executable continuation Plan: Plan ID could not be calculated")
	}
	if err := Validate(value); err != nil {
		return Plan{}, fmt.Errorf("build executable continuation Plan: %w", err)
	}
	return value, nil
}
