package plan

import (
	"errors"
	"regexp"
)

const BaseReviewCheckKind = "homebrew_transaction_review_matches"

var baseReviewIDPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// BindBaseTransactionReview derives the final executable Base Plan from the
// reviewed candidate Plan. The derived Plan ID is what execution confirmation
// and operation history bind, while every action retains the Review ID.
func BindBaseTransactionReview(candidate Plan, reviewID string) (Plan, error) {
	if err := Validate(candidate); err != nil || candidate.SchemaVersion != ExecutableSchemaVersion ||
		!candidate.Executable || candidate.Continuation != nil || !baseReviewIDPattern.MatchString(reviewID) {
		return Plan{}, errors.New("bind Base transaction review: valid candidate Plan and Review ID are required")
	}
	if candidate.Environment.ToolID != "profile.base" || candidate.Environment.ActiveManager != "homebrew" ||
		len(candidate.Actions) == 0 {
		return Plan{}, errors.New("bind Base transaction review: candidate is not a Base Homebrew Plan")
	}
	data, err := Marshal(candidate)
	if err != nil {
		return Plan{}, errors.New("bind Base transaction review: copy candidate Plan")
	}
	value, err := Decode(data)
	if err != nil {
		return Plan{}, errors.New("bind Base transaction review: decode candidate Plan")
	}
	for index := range value.Actions {
		action := &value.Actions[index]
		if action.ID != "install-base-git" && action.ID != "install-base-cmake" {
			return Plan{}, errors.New("bind Base transaction review: candidate contains an unsupported action")
		}
		for _, check := range action.Preconditions {
			if check.Kind == BaseReviewCheckKind {
				return Plan{}, errors.New("bind Base transaction review: candidate is already review-bound")
			}
		}
		action.Preconditions = append(action.Preconditions, Check{
			Kind: BaseReviewCheckKind, Subject: action.ID, Expected: reviewID,
		})
	}
	value.ID = ""
	value.ID, err = planID(value)
	if err != nil {
		return Plan{}, errors.New("bind Base transaction review: calculate Plan ID")
	}
	if err := Validate(value); err != nil {
		return Plan{}, err
	}
	return value, nil
}
