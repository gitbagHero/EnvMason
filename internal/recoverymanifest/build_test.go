package recoverymanifest

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/execution"
)

func TestBuildCreatesDeterministicFiveDispositionManifest(t *testing.T) {
	reviews := recoveryReviews()
	original := cloneReviews(reviews)
	createdAt := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	first, err := Build(createdAt, reviews)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !reflect.DeepEqual(reviews, original) {
		t.Fatal("Build mutated RecoveryRevalidation input")
	}
	reversed := cloneReviews(reviews)
	slices.Reverse(reversed)
	for index := range reversed {
		slices.Reverse(reversed[index].Candidates)
	}
	second, err := Build(createdAt, reversed)
	if err != nil {
		t.Fatalf("Build(reversed) error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("input order changed the normalized Recovery Manifest")
	}
	if first.ID == "" || first.Executable || first.Confirmable ||
		len(first.Sources) != 2 || len(first.Items) != 5 {
		t.Fatalf("Recovery Manifest = %#v", first)
	}
	dispositions := map[string]Disposition{}
	for _, item := range first.Items {
		dispositions[item.ActionID] = item.Disposition
	}
	expected := map[string]Disposition{
		"current-manual": DispositionManualAction,
		"current-plan":   DispositionPrepareNewPlan,
		"drifted-plan":   DispositionReassessDrifted,
		"no-verifier":    DispositionReviewNoVerifier,
		"uncertain":      DispositionInvestigateUncertain,
	}
	if !reflect.DeepEqual(dispositions, expected) {
		t.Fatalf("dispositions = %#v", dispositions)
	}
	if first.Sources[0].OperationID >= first.Sources[1].OperationID {
		t.Fatalf("sources are not sorted: %#v", first.Sources)
	}
}

func TestBuildRejectsInvalidSourcesAndCandidates(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*[]execution.RecoveryRevalidation)
	}{
		{"no sources", func(value *[]execution.RecoveryRevalidation) { *value = nil }},
		{"bad source", func(value *[]execution.RecoveryRevalidation) { (*value)[0].SourceOperationID = "invalid" }},
		{"bad plan", func(value *[]execution.RecoveryRevalidation) { (*value)[0].SourcePlanID = "invalid" }},
		{"duplicate source", func(value *[]execution.RecoveryRevalidation) { *value = append(*value, (*value)[0]) }},
		{"duplicate action", func(value *[]execution.RecoveryRevalidation) {
			(*value)[0].Candidates = append((*value)[0].Candidates, (*value)[0].Candidates[0])
		}},
		{"bad action", func(value *[]execution.RecoveryRevalidation) { (*value)[0].Candidates[0].ActionID = "Bad_Action" }},
		{"bad tool", func(value *[]execution.RecoveryRevalidation) { (*value)[0].Candidates[0].ToolID = "Bad Tool" }},
		{"bad operation", func(value *[]execution.RecoveryRevalidation) { (*value)[0].Candidates[0].Operation = "Bad Operation" }},
		{"active step", func(value *[]execution.RecoveryRevalidation) {
			(*value)[0].Candidates[0].StepState = execution.StateRunning
		}},
		{"bad mode", func(value *[]execution.RecoveryRevalidation) { (*value)[0].Candidates[0].RecoveryMode = "auto" }},
		{"blank summary", func(value *[]execution.RecoveryRevalidation) { (*value)[0].Candidates[0].RecoverySummary = " " }},
		{"uncertain marked current", func(value *[]execution.RecoveryRevalidation) {
			(*value)[0].Candidates[4].CurrentState = execution.RecoveryCheckpointCurrent
		}},
		{"changed marked uncertain", func(value *[]execution.RecoveryRevalidation) {
			(*value)[0].Candidates[0].CurrentState = execution.RecoveryCheckpointUncertain
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reviews := recoveryReviews()
			test.mutate(&reviews)
			if _, err := Build(
				time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC),
				reviews,
			); err == nil {
				t.Fatal("Build unexpectedly succeeded")
			}
		})
	}
	if _, err := Build(time.Time{}, recoveryReviews()); err == nil {
		t.Fatal("zero creation time was accepted")
	}
}

func TestValidateRejectsManifestTampering(t *testing.T) {
	value := mustRecoveryManifest(t)
	tests := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{"schema", func(value *Manifest) { value.SchemaVersion = "0.2.0" }},
		{"id", func(value *Manifest) { value.ID = recoveryDigest('f') }},
		{"time", func(value *Manifest) { value.CreatedAt = value.CreatedAt.Add(time.Second) }},
		{"executable", func(value *Manifest) { value.Executable = true }},
		{"confirmable", func(value *Manifest) { value.Confirmable = true }},
		{"summary", func(value *Manifest) { value.Summary += " changed" }},
		{"source plan", func(value *Manifest) { value.Sources[0].PlanID = recoveryDigest('e') }},
		{"source order", func(value *Manifest) { value.Sources[0], value.Sources[1] = value.Sources[1], value.Sources[0] }},
		{"item source", func(value *Manifest) { value.Items[0].SourceOperationID = recoveryOperationID('f') }},
		{"item action", func(value *Manifest) { value.Items[0].ActionID = "changed-action" }},
		{"item state", func(value *Manifest) { value.Items[0].CurrentState = execution.RecoveryCheckpointDrifted }},
		{"item mode", func(value *Manifest) { value.Items[0].RecoveryMode = "plan" }},
		{"item disposition", func(value *Manifest) { value.Items[0].Disposition = DispositionPrepareNewPlan }},
		{"item summary", func(value *Manifest) { value.Items[0].RecoverySummary += " changed" }},
		{"item order", func(value *Manifest) { value.Items[0], value.Items[1] = value.Items[1], value.Items[0] }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := cloneManifest(value)
			test.mutate(&changed)
			if err := Validate(changed); err == nil {
				t.Fatal("Validate unexpectedly succeeded")
			}
		})
	}
}

func recoveryReviews() []execution.RecoveryRevalidation {
	source := execution.RecoveryRevalidation{
		SourceOperationID: recoveryOperationID('2'),
		SourcePlanID:      recoveryDigest('b'),
		Candidates: []execution.RecoveryCheckpoint{
			recoveryCandidate(
				"current-manual",
				execution.RecoveryEvidenceChanged,
				execution.RecoveryCheckpointCurrent,
				"manual",
				execution.StateCompleted,
			),
			recoveryCandidate(
				"current-plan",
				execution.RecoveryEvidenceChanged,
				execution.RecoveryCheckpointCurrent,
				"plan",
				execution.StateCompleted,
			),
			recoveryCandidate(
				"drifted-plan",
				execution.RecoveryEvidenceChanged,
				execution.RecoveryCheckpointDrifted,
				"plan",
				execution.StateFailed,
			),
			recoveryCandidate(
				"no-verifier",
				execution.RecoveryEvidenceChanged,
				execution.RecoveryCheckpointVerifierUnavailable,
				"manual",
				execution.StateTimedOut,
			),
			recoveryCandidate(
				"uncertain",
				execution.RecoveryEvidenceUncertain,
				execution.RecoveryCheckpointUncertain,
				"manual",
				execution.StateInterrupted,
			),
		},
	}
	empty := execution.RecoveryRevalidation{
		SourceOperationID: recoveryOperationID('1'),
		SourcePlanID:      recoveryDigest('a'),
		Candidates:        []execution.RecoveryCheckpoint{},
	}
	return []execution.RecoveryRevalidation{source, empty}
}

func recoveryCandidate(
	actionID string,
	evidence execution.RecoveryEvidence,
	current execution.RecoveryCheckpointState,
	mode string,
	state execution.State,
) execution.RecoveryCheckpoint {
	return execution.RecoveryCheckpoint{
		RecoveryCandidate: execution.RecoveryCandidate{
			ActionID: actionID, ToolID: "runtime.node",
			Operation: "set_default", StepState: state,
			Evidence: evidence, RecoveryMode: mode,
			RecoverySummary: "Review the fixed recovery guidance for " +
				actionID + ".",
		},
		CurrentState: current,
	}
}

func mustRecoveryManifest(t *testing.T) Manifest {
	t.Helper()
	value, err := Build(
		time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC),
		recoveryReviews(),
	)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return value
}

func cloneReviews(
	value []execution.RecoveryRevalidation,
) []execution.RecoveryRevalidation {
	result := append([]execution.RecoveryRevalidation{}, value...)
	for index := range result {
		result[index].Candidates = append(
			[]execution.RecoveryCheckpoint{},
			value[index].Candidates...,
		)
	}
	return result
}

func cloneManifest(value Manifest) Manifest {
	result := value
	result.Sources = append([]Source{}, value.Sources...)
	result.Items = append([]Item{}, value.Items...)
	return result
}

func recoveryOperationID(character byte) string {
	return "op-" + strings.Repeat(string(character), 32)
}

func recoveryDigest(character byte) string {
	return "sha256:" + strings.Repeat(string(character), 64)
}
