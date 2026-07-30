package plan

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestBuildContinuationDraftCreatesImmutableReviewOnlyPlan(t *testing.T) {
	t.Parallel()
	input := continuationDraftInput(t)
	value, err := BuildContinuationDraft(input)
	if err != nil {
		t.Fatal(err)
	}
	if value.SchemaVersion != ContinuationSchemaVersion || value.Executable ||
		len(value.Actions) != 2 || value.Actions[0].ID != "update-corepack" ||
		len(value.Actions[0].Dependencies) != 0 ||
		len(value.Actions[1].Dependencies) != 1 || value.Actions[1].Dependencies[0] != "update-corepack" {
		t.Fatalf("continuation Plan = %#v", value)
	}
	if value.Continuation == nil ||
		value.Continuation.PreparedPlanID != input.PreparedPlan.ID ||
		len(value.Continuation.ReusableCheckpoints) != 1 ||
		len(value.Continuation.SatisfiedDependencies) != 1 ||
		value.Continuation.SatisfiedDependencies[0] != (SatisfiedDependency{
			ActionID: "update-corepack", DependencyActionID: "update-npm",
		}) {
		t.Fatalf("continuation provenance = %#v", value.Continuation)
	}
	if value.CreatedAt != input.PreparedPlan.CreatedAt || value.ExpiresAt != input.PreparedPlan.ExpiresAt ||
		value.EnvironmentDigest != input.PreparedPlan.EnvironmentDigest ||
		value.PolicyDigest != input.PreparedPlan.PolicyDigest ||
		value.ID == input.PreparedPlan.ID {
		t.Fatalf("fresh Plan binding = %#v", value)
	}

	before, err := Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(before)
	if err != nil || decoded.ID != value.ID {
		t.Fatalf("decode = %#v, %v", decoded, err)
	}
	summary, err := Render(value, FormatSummary)
	if err != nil || !bytes.Contains(summary, []byte("review-only continuation Plan")) {
		t.Fatalf("summary = %q, %v", summary, err)
	}
	if bytes.Contains(before, []byte(`"command"`)) || bytes.Contains(before, []byte(`"args"`)) {
		t.Fatalf("continuation Plan contains process specification: %s", before)
	}

	input.PreparedPlan.Actions[0].TargetVersion = "changed"
	input.PreparedPlan.Environment.Installations[0].Path = "changed"
	input.SourceActionIDs[0] = "changed"
	input.ReusableCheckpoints[0].ObservedDigest = "changed"
	input.SatisfiedDependencies[0].DependencyActionID = "changed"
	after, err := Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("continuation Plan shares mutable input state")
	}
}

func TestContinuationValidationRejectsUnsafeProvenanceMutations(t *testing.T) {
	t.Parallel()
	value, err := BuildContinuationDraft(continuationDraftInput(t))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*Plan)
		want   string
	}{
		{name: "executable", mutate: func(candidate *Plan) { candidate.Executable = true }, want: "non-executable"},
		{name: "missing provenance", mutate: func(candidate *Plan) { candidate.Continuation = nil }, want: "requires continuation"},
		{name: "duplicate source", mutate: func(candidate *Plan) {
			candidate.Continuation.SourceActionIDs[1] = candidate.Continuation.SourceActionIDs[0]
		}, want: "unique"},
		{name: "checkpoint overlaps remaining", mutate: func(candidate *Plan) {
			candidate.Continuation.ReusableCheckpoints[0].ActionID = "update-corepack"
		}, want: "overlapping"},
		{name: "incomplete partition", mutate: func(candidate *Plan) {
			candidate.Continuation.SourceActionIDs = candidate.Continuation.SourceActionIDs[:2]
		}, want: "subset"},
		{name: "invalid checkpoint digest", mutate: func(candidate *Plan) {
			candidate.Continuation.ReusableCheckpoints[0].ObservedDigest = "invalid"
		}, want: "checkpoints"},
		{name: "unknown satisfied dependency", mutate: func(candidate *Plan) {
			candidate.Continuation.SatisfiedDependencies[0].DependencyActionID = "unknown-action"
		}, want: "dependency"},
		{name: "remaining order changed", mutate: func(candidate *Plan) {
			candidate.Actions[0], candidate.Actions[1] = candidate.Actions[1], candidate.Actions[0]
		}, want: "source action order"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := clonePlan(value)
			test.mutate(&candidate)
			candidate.ID, _ = planID(candidate)
			if err := Validate(candidate); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v, want %q", err, test.want)
			}
		})
	}

	legacy := continuationDraftInput(t).PreparedPlan
	legacy.Continuation = value.Continuation
	legacy.ID, _ = planID(legacy)
	if err := Validate(legacy); err == nil || !strings.Contains(err.Error(), "requires Plan 0.4.0") {
		t.Fatalf("legacy continuation validation = %v", err)
	}
}

func continuationDraftInput(t *testing.T) ContinuationDraftInput {
	t.Helper()
	preparedInput := nodeToolsInput()
	preparedInput.CreatedAt = time.Date(2026, 7, 30, 9, 2, 0, 0, time.UTC)
	preparedInput.Targets = []NodeToolTarget{
		preparedInput.Targets[2],
		preparedInput.Targets[0],
	}
	prepared, err := BuildNodeTools(preparedInput)
	if err != nil {
		t.Fatal(err)
	}
	return ContinuationDraftInput{
		SourceOperationID: "op-00000000000000000000000000000002",
		SourcePlanID:      "sha256:" + strings.Repeat("c", 64),
		SourceActionIDs:   []string{"update-npm", "update-corepack", "update-pnpm"},
		PreparedPlan:      prepared,
		ReusableCheckpoints: []CheckpointBinding{{
			ActionID: "update-npm", RecordedAfterDigest: "sha256:" + strings.Repeat("d", 64),
			ObservedDigest: "sha256:" + strings.Repeat("e", 64),
		}},
		SatisfiedDependencies: []SatisfiedDependency{{
			ActionID: "update-corepack", DependencyActionID: "update-npm",
		}},
	}
}
