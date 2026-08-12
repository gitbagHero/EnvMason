package recoverymanifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gitbagHero/EnvMason/internal/execution"
)

var (
	operationIDPattern = regexp.MustCompile(`^op-[a-f0-9]{32}$`)
	digestPattern      = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	actionIDPattern    = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	toolIDPattern      = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)
	operationPattern   = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
)

func Build(
	createdAt time.Time,
	reviews []execution.RecoveryRevalidation,
) (Manifest, error) {
	if createdAt.IsZero() {
		return Manifest{}, errors.New(
			"build Recovery Manifest: created_at is required",
		)
	}
	if len(reviews) == 0 {
		return Manifest{}, errors.New(
			"build Recovery Manifest: at least one reviewed source is required",
		)
	}
	value := Manifest{
		SchemaVersion: SchemaVersion,
		CreatedAt:     createdAt.UTC(),
		Executable:    false,
		Confirmable:   false,
		Summary:       manifestSummary,
		Sources:       make([]Source, 0, len(reviews)),
		Items:         []Item{},
	}
	seenSources := make(map[string]bool, len(reviews))
	seenItems := map[string]bool{}
	for _, review := range reviews {
		if !operationIDPattern.MatchString(review.SourceOperationID) ||
			!digestPattern.MatchString(review.SourcePlanID) ||
			seenSources[review.SourceOperationID] {
			return Manifest{}, errors.New(
				"build Recovery Manifest: source identity is invalid or duplicated",
			)
		}
		seenSources[review.SourceOperationID] = true
		value.Sources = append(value.Sources, Source{
			OperationID: review.SourceOperationID,
			PlanID:      review.SourcePlanID,
		})
		for _, candidate := range review.Candidates {
			key := review.SourceOperationID + "\x00" + candidate.ActionID
			if seenItems[key] {
				return Manifest{}, errors.New(
					"build Recovery Manifest: source action is duplicated",
				)
			}
			seenItems[key] = true
			item, err := itemFromCandidate(review, candidate)
			if err != nil {
				return Manifest{}, err
			}
			value.Items = append(value.Items, item)
		}
	}
	sort.Slice(value.Sources, func(left, right int) bool {
		return value.Sources[left].OperationID <
			value.Sources[right].OperationID
	})
	sort.Slice(value.Items, func(left, right int) bool {
		if value.Items[left].SourceOperationID !=
			value.Items[right].SourceOperationID {
			return value.Items[left].SourceOperationID <
				value.Items[right].SourceOperationID
		}
		return value.Items[left].ActionID < value.Items[right].ActionID
	})
	id, err := calculateID(value)
	if err != nil {
		return Manifest{}, err
	}
	value.ID = id
	if err := Validate(value); err != nil {
		return Manifest{}, err
	}
	return value, nil
}

func Validate(value Manifest) error {
	if value.SchemaVersion != SchemaVersion ||
		value.CreatedAt.IsZero() ||
		value.Executable ||
		value.Confirmable ||
		value.Summary != manifestSummary ||
		len(value.Sources) == 0 {
		return errors.New(
			"validate Recovery Manifest: fixed identity or read-only fields are invalid",
		)
	}
	sourcePlans := make(map[string]string, len(value.Sources))
	previousSource := ""
	for _, source := range value.Sources {
		if !operationIDPattern.MatchString(source.OperationID) ||
			!digestPattern.MatchString(source.PlanID) ||
			sourcePlans[source.OperationID] != "" ||
			(previousSource != "" && source.OperationID <= previousSource) {
			return errors.New(
				"validate Recovery Manifest: source identity or order is invalid",
			)
		}
		sourcePlans[source.OperationID] = source.PlanID
		previousSource = source.OperationID
	}
	previousItem := ""
	seenItems := map[string]bool{}
	for _, item := range value.Items {
		key := item.SourceOperationID + "\x00" + item.ActionID
		if sourcePlans[item.SourceOperationID] != item.SourcePlanID ||
			seenItems[key] ||
			(previousItem != "" && key <= previousItem) ||
			!validItem(item) {
			return errors.New(
				"validate Recovery Manifest: recovery item is invalid or out of order",
			)
		}
		seenItems[key] = true
		previousItem = key
	}
	expected, err := calculateID(value)
	if err != nil || expected != value.ID {
		return errors.New(
			"validate Recovery Manifest: content-derived ID does not match",
		)
	}
	return nil
}

func itemFromCandidate(
	review execution.RecoveryRevalidation,
	candidate execution.RecoveryCheckpoint,
) (Item, error) {
	item := Item{
		SourceOperationID: review.SourceOperationID,
		SourcePlanID:      review.SourcePlanID,
		ActionID:          candidate.ActionID,
		ToolID:            candidate.ToolID,
		Operation:         candidate.Operation,
		StepState:         candidate.StepState,
		Evidence:          candidate.Evidence,
		CurrentState:      candidate.CurrentState,
		RecoveryMode:      candidate.RecoveryMode,
		Disposition: dispositionFor(
			candidate.CurrentState,
			candidate.RecoveryMode,
		),
		RecoverySummary: candidate.RecoverySummary,
	}
	if !validItem(item) {
		return Item{}, errors.New(
			"build Recovery Manifest: recovery candidate is invalid",
		)
	}
	return item, nil
}

func validItem(value Item) bool {
	if !operationIDPattern.MatchString(value.SourceOperationID) ||
		!digestPattern.MatchString(value.SourcePlanID) ||
		!actionIDPattern.MatchString(value.ActionID) ||
		!toolIDPattern.MatchString(value.ToolID) ||
		!operationPattern.MatchString(value.Operation) ||
		!terminalStepState(value.StepState) ||
		(value.RecoveryMode != "plan" && value.RecoveryMode != "manual") ||
		strings.TrimSpace(value.RecoverySummary) == "" ||
		utf8.RuneCountInString(value.RecoverySummary) > 1024 {
		return false
	}
	switch value.Evidence {
	case execution.RecoveryEvidenceUncertain:
		return value.CurrentState == execution.RecoveryCheckpointUncertain &&
			value.Disposition == DispositionInvestigateUncertain
	case execution.RecoveryEvidenceChanged:
		switch value.CurrentState {
		case execution.RecoveryCheckpointCurrent,
			execution.RecoveryCheckpointDrifted,
			execution.RecoveryCheckpointVerifierUnavailable:
			return value.Disposition == dispositionFor(
				value.CurrentState,
				value.RecoveryMode,
			)
		default:
			return false
		}
	default:
		return false
	}
}

func dispositionFor(
	state execution.RecoveryCheckpointState,
	mode string,
) Disposition {
	switch state {
	case execution.RecoveryCheckpointCurrent:
		if mode == "plan" {
			return DispositionPrepareNewPlan
		}
		if mode == "manual" {
			return DispositionManualAction
		}
	case execution.RecoveryCheckpointUncertain:
		return DispositionInvestigateUncertain
	case execution.RecoveryCheckpointDrifted:
		return DispositionReassessDrifted
	case execution.RecoveryCheckpointVerifierUnavailable:
		return DispositionReviewNoVerifier
	}
	return ""
}

func terminalStepState(value execution.State) bool {
	switch value {
	case execution.StateCompleted,
		execution.StateFailed,
		execution.StateTimedOut,
		execution.StateCancelled,
		execution.StateInterrupted:
		return true
	default:
		return false
	}
}

func calculateID(value Manifest) (string, error) {
	value.ID = ""
	data, err := json.Marshal(value)
	if err != nil {
		return "", errors.New(
			"calculate Recovery Manifest ID: encode content",
		)
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
