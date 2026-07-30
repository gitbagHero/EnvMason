package execution

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/plan"
)

func TestBuildContinuationPlanBindsFreshRemainingPlanAndCheckpointEdges(t *testing.T) {
	t.Parallel()
	source, registry, runner, _ := checkpointSource(t, "update-corepack", "", "", "", false)
	assessment, err := AssessContinuation(t.Context(), source, registry)
	if err != nil {
		t.Fatal(err)
	}
	prepared := continuationPreparedPlan(t, testBaseTime.Add(2*time.Minute),
		[]string{plan.NodeToolCorepack, plan.NodeToolPNPM}, "c")
	sourceBefore, err := MarshalRecord(source)
	if err != nil {
		t.Fatal(err)
	}
	runnerCalls := len(runner.calls)

	value, err := BuildContinuationPlan(source, assessment, prepared)
	if err != nil {
		t.Fatal(err)
	}
	if value.SchemaVersion != plan.ContinuationSchemaVersion || value.Executable ||
		len(value.Actions) != 2 || value.Actions[0].ID != "update-corepack" ||
		value.EnvironmentDigest != prepared.EnvironmentDigest ||
		value.PolicyDigest != prepared.PolicyDigest {
		t.Fatalf("continuation Plan = %#v", value)
	}
	if value.Continuation == nil || value.Continuation.SourceOperationID != source.ID ||
		value.Continuation.SourcePlanID != source.PlanID ||
		value.Continuation.PreparedPlanID != prepared.ID ||
		len(value.Continuation.ReusableCheckpoints) != 1 ||
		len(value.Continuation.SatisfiedDependencies) != 1 ||
		value.Continuation.SatisfiedDependencies[0] != (plan.SatisfiedDependency{
			ActionID: "update-corepack", DependencyActionID: "update-npm",
		}) {
		t.Fatalf("continuation provenance = %#v", value.Continuation)
	}
	if len(value.Actions[0].Dependencies) != 0 ||
		len(value.Actions[1].Dependencies) != 1 || value.Actions[1].Dependencies[0] != "update-corepack" {
		t.Fatalf("rewritten dependencies = %#v", value.Actions)
	}
	sourceAfter, err := MarshalRecord(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sourceBefore, sourceAfter) || len(runner.calls) != runnerCalls {
		t.Fatal("continuation Plan generation mutated history or ran a process")
	}

	assessment.Checkpoints[0].ObservedDigest = "changed"
	prepared.Actions[0].TargetVersion = "changed"
	source.ConfirmedPlan.Actions[0].TargetVersion = "changed"
	if value.Continuation.ReusableCheckpoints[0].ObservedDigest == "changed" ||
		value.Actions[0].TargetVersion == "changed" {
		t.Fatal("continuation Plan shares mutable source state")
	}
}

func TestBuildContinuationPlanBindsEvidenceAndFreshPreparationToID(t *testing.T) {
	t.Parallel()
	source, registry, _, _ := checkpointSource(t, "update-corepack", "", "", "", false)
	assessment, err := AssessContinuation(t.Context(), source, registry)
	if err != nil {
		t.Fatal(err)
	}
	prepared := continuationPreparedPlan(t, testBaseTime.Add(2*time.Minute),
		[]string{plan.NodeToolCorepack, plan.NodeToolPNPM}, "c")
	first, err := BuildContinuationPlan(source, assessment, prepared)
	if err != nil {
		t.Fatal(err)
	}

	changedEvidence := assessment
	changedEvidence.Checkpoints = append([]CheckpointEvidence{}, assessment.Checkpoints...)
	changedEvidence.Checkpoints[0].ObservedDigest = "sha256:" + strings.Repeat("f", 64)
	second, err := BuildContinuationPlan(source, changedEvidence, prepared)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID {
		t.Fatal("changed revalidation evidence reused the continuation Plan ID")
	}

	changedPreparation := continuationPreparedPlan(t, testBaseTime.Add(3*time.Minute),
		[]string{plan.NodeToolCorepack, plan.NodeToolPNPM}, "d")
	third, err := BuildContinuationPlan(source, assessment, changedPreparation)
	if err != nil {
		t.Fatal(err)
	}
	if third.ID == first.ID || third.Continuation.PreparedPlanID == first.Continuation.PreparedPlanID {
		t.Fatal("changed fresh preparation reused the continuation Plan identity")
	}
}

func TestBuildContinuationPlanAcceptsFailureBeforeFirstActionWithoutInventingCheckpoint(t *testing.T) {
	t.Parallel()
	source, registry, _, _ := checkpointSource(t, "update-npm", "", "", "", false)
	assessment, err := AssessContinuation(t.Context(), source, registry)
	if err != nil {
		t.Fatal(err)
	}
	prepared := continuationPreparedPlan(t, testBaseTime.Add(2*time.Minute),
		[]string{plan.NodeToolNPM, plan.NodeToolCorepack, plan.NodeToolPNPM}, "c")
	value, err := BuildContinuationPlan(source, assessment, prepared)
	if err != nil {
		t.Fatal(err)
	}
	if len(value.Actions) != 3 || value.Continuation == nil ||
		len(value.Continuation.ReusableCheckpoints) != 0 ||
		len(value.Continuation.SatisfiedDependencies) != 0 {
		t.Fatalf("first-action continuation = %#v", value)
	}
}

func TestBuildContinuationPlanRejectsMismatchedAssessmentOrFreshPlan(t *testing.T) {
	t.Parallel()
	source, registry, _, _ := checkpointSource(t, "update-corepack", "", "", "", false)
	assessment, err := AssessContinuation(t.Context(), source, registry)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		assessment func() ContinuationAssessment
		prepared   func(*testing.T) plan.Plan
		want       string
	}{
		{
			name: "ineligible assessment",
			assessment: func() ContinuationAssessment {
				value := assessment
				value.Eligible = false
				return value
			},
			prepared: validContinuationPreparedPlan,
			want:     "assessment does not match",
		},
		{
			name: "wrong source identity",
			assessment: func() ContinuationAssessment {
				value := assessment
				value.SourceOperationID = "op-ffffffffffffffffffffffffffffffff"
				return value
			},
			prepared: validContinuationPreparedPlan,
			want:     "assessment does not match",
		},
		{
			name: "tampered evidence",
			assessment: func() ContinuationAssessment {
				value := assessment
				value.Checkpoints = append([]CheckpointEvidence{}, assessment.Checkpoints...)
				value.Checkpoints[0].RecordedAfterDigest = "sha256:" + strings.Repeat("f", 64)
				return value
			},
			prepared: validContinuationPreparedPlan,
			want:     "evidence does not match",
		},
		{
			name:       "stale preparation",
			assessment: func() ContinuationAssessment { return assessment },
			prepared: func(t *testing.T) plan.Plan {
				return continuationPreparedPlan(t, testBaseTime, []string{plan.NodeToolCorepack, plan.NodeToolPNPM}, "c")
			},
			want: "postdate",
		},
		{
			name:       "preparation timestamp equals terminal state",
			assessment: func() ContinuationAssessment { return assessment },
			prepared: func(t *testing.T) plan.Plan {
				return continuationPreparedPlan(t, testBaseTime.Add(time.Minute),
					[]string{plan.NodeToolCorepack, plan.NodeToolPNPM}, "c")
			},
			want: "postdate",
		},
		{
			name:       "missing remaining action",
			assessment: func() ContinuationAssessment { return assessment },
			prepared: func(t *testing.T) plan.Plan {
				return continuationPreparedPlan(t, testBaseTime.Add(2*time.Minute), []string{plan.NodeToolCorepack}, "c")
			},
			want: "exactly the remaining",
		},
		{
			name:       "changed target",
			assessment: func() ContinuationAssessment { return assessment },
			prepared: func(t *testing.T) plan.Plan {
				value := continuationPreparedPlan(t, testBaseTime.Add(2*time.Minute),
					[]string{plan.NodeToolCorepack, plan.NodeToolPNPM}, "c")
				input := continuationNodeToolsInput(testBaseTime.Add(2*time.Minute),
					[]string{plan.NodeToolCorepack, plan.NodeToolPNPM}, "c")
				input.Targets[0].TargetVersion = "0.36.0"
				var err error
				value, err = plan.BuildNodeTools(input)
				if err != nil {
					t.Fatal(err)
				}
				return value
			},
			want: "identity or target changed",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := BuildContinuationPlan(source, test.assessment(), test.prepared(t)); err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestExecutorRejectsReviewOnlyContinuationBeforeAnySideEffect(t *testing.T) {
	t.Parallel()
	source, registry, _, _ := checkpointSource(t, "update-corepack", "", "", "", false)
	assessment, err := AssessContinuation(t.Context(), source, registry)
	if err != nil {
		t.Fatal(err)
	}
	prepared := continuationPreparedPlan(t, testBaseTime.Add(2*time.Minute),
		[]string{plan.NodeToolCorepack, plan.NodeToolPNPM}, "c")
	value, err := BuildContinuationPlan(source, assessment, prepared)
	if err != nil {
		t.Fatal(err)
	}

	executor, request, store, runner := testHarness(t, nil)
	executor.Now = func() time.Time { return value.CreatedAt.Add(time.Minute) }
	executor.Registry = Registry{definitions: map[string]Definition{}}
	request.Plan = value
	request.Confirmation = ConfirmationReceipt{
		Scope: "plan", ConfirmedPlanID: value.ID, ConfirmedAt: value.CreatedAt,
	}
	_, err = executor.Execute(t.Context(), request)
	assertExecutionCode(t, err, CodePlanInvalid)
	if len(store.records) != 0 || runner.calls != 0 {
		t.Fatal("review-only continuation reached history or process execution")
	}
}

func validContinuationPreparedPlan(t *testing.T) plan.Plan {
	t.Helper()
	return continuationPreparedPlan(t, testBaseTime.Add(2*time.Minute),
		[]string{plan.NodeToolCorepack, plan.NodeToolPNPM}, "c")
}

func continuationPreparedPlan(t *testing.T, createdAt time.Time, tools []string, digestSeed string) plan.Plan {
	t.Helper()
	value, err := plan.BuildNodeTools(continuationNodeToolsInput(createdAt, tools, digestSeed))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func continuationNodeToolsInput(createdAt time.Time, tools []string, digestSeed string) plan.NodeToolsInput {
	controlDigest := "sha256:" + strings.Repeat(digestSeed, 64)
	targets := map[string]plan.NodeToolTarget{
		plan.NodeToolNPM: {
			ToolID: plan.NodeToolNPM, CurrentVersion: "12.0.1", TargetVersion: "12.0.1",
			Provider: plan.NodeToolProviderNPM, ControlDigest: controlDigest,
		},
		plan.NodeToolCorepack: {
			ToolID: plan.NodeToolCorepack, CurrentVersion: "0.34.5", TargetVersion: "0.35.0",
			Provider: plan.NodeToolProviderNPM, ControlDigest: controlDigest,
		},
		plan.NodeToolPNPM: {
			ToolID: plan.NodeToolPNPM, CurrentVersion: "10.0.0", TargetVersion: "11.1.0",
			Provider: plan.NodeToolProviderCorepack, ControlDigest: controlDigest,
		},
	}
	selected := make([]plan.NodeToolTarget, 0, len(tools))
	for _, toolID := range tools {
		selected = append(selected, targets[toolID])
	}
	return plan.NodeToolsInput{
		CreatedAt: createdAt, NodeVersion: "24.12.0",
		NVMScriptDigest:    controlDigest,
		DefaultAliasDigest: "sha256:" + strings.Repeat("b", 64),
		Inventory: inventory.Inventory{
			SchemaVersion: inventory.SchemaVersion, GeneratedAt: createdAt.Add(-time.Second),
			System: inventory.System{
				OS: inventory.OSMacOS, OSVersion: "26.0", Architecture: inventory.ArchitectureARM64,
			},
			Tools: []inventory.Tool{{
				ID: "runtime.node", DisplayName: "Node.js", Category: inventory.CategoryRuntime,
				Installations: []inventory.Installation{{
					ID: "node-nvm-target", Version: "v24.12.0",
					Path: "$HOME/.nvm/versions/node/v24.12.0/bin/node", Manager: "nvm",
					Architecture: inventory.ArchitectureARM64,
					ActiveState:  inventory.ActiveStateActive, DefaultState: inventory.DefaultStateDefault,
				}},
			}},
		},
		Targets: selected,
	}
}
