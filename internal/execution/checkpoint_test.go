package execution

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/gitbagHero/EnvMason/internal/plan"
)

func TestAssessContinuationRevalidatesOnlyCompletedCheckpointsAcrossDAGFailures(t *testing.T) {
	t.Parallel()
	actionIDs := []string{"update-npm", "update-corepack", "update-pnpm"}
	for _, phase := range []string{"process", "verification"} {
		for failureIndex, failureAction := range actionIDs {
			phase := phase
			failureIndex := failureIndex
			failureAction := failureAction
			t.Run(phase+"-"+failureAction, func(t *testing.T) {
				t.Parallel()
				processFailure := ""
				verificationFailure := ""
				if phase == "process" {
					processFailure = failureAction
				} else {
					verificationFailure = failureAction
				}
				source, registry, runner, calls := checkpointSource(
					t, processFailure, verificationFailure, "", "", false,
				)
				before, err := MarshalRecord(source)
				if err != nil {
					t.Fatal(err)
				}
				runnerCalls := len(runner.calls)

				assessment, err := AssessContinuation(t.Context(), source, registry)
				if err != nil {
					t.Fatal(err)
				}
				if !assessment.Eligible || assessment.Blocker != nil {
					t.Fatalf("assessment = %#v", assessment)
				}
				if !slices.Equal(assessment.ReusableActionIDs, actionIDs[:failureIndex]) ||
					!slices.Equal(assessment.RemainingActionIDs, actionIDs[failureIndex:]) ||
					len(assessment.Checkpoints) != failureIndex {
					t.Fatalf("assessment actions = %#v", assessment)
				}
				for index, actionID := range actionIDs {
					wantCalls := 0
					if index < failureIndex {
						wantCalls = 1
					}
					if calls[actionID] != wantCalls {
						t.Fatalf("checkpoint calls = %#v", calls)
					}
				}
				after, err := MarshalRecord(source)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(before, after) || len(runner.calls) != runnerCalls {
					t.Fatal("continuation assessment mutated the source or executed an action")
				}
			})
		}
	}
}

func TestAssessContinuationBlocksIncompleteUnavailableDriftedAndInvalidCheckpoints(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		drift      string
		missing    string
		invalid    bool
		mutate     func(*Record)
		wantCode   ContinuationBlockCode
		wantAction string
	}{
		{
			name: "missing after snapshot",
			mutate: func(source *Record) {
				source.Steps[0].After = nil
				source.Steps[0].Diff = nil
			},
			wantCode: ContinuationBlockCheckpointIncomplete, wantAction: "update-npm",
		},
		{
			name: "missing verifier", missing: "update-npm",
			wantCode: ContinuationBlockVerifierUnavailable, wantAction: "update-npm",
		},
		{
			name: "checkpoint drift", drift: "update-npm",
			wantCode: ContinuationBlockCheckpointDrifted, wantAction: "update-npm",
		},
		{
			name: "invalid verifier evidence", invalid: true,
			wantCode: ContinuationBlockEvidenceInvalid, wantAction: "update-npm",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			source, registry, _, _ := checkpointSource(
				t, "update-corepack", "", test.drift, test.missing, test.invalid,
			)
			if test.mutate != nil {
				test.mutate(&source)
			}
			assessment, err := AssessContinuation(t.Context(), source, registry)
			if err != nil {
				t.Fatal(err)
			}
			if assessment.Eligible || assessment.Blocker == nil ||
				assessment.Blocker.Code != test.wantCode || assessment.Blocker.ActionID != test.wantAction {
				t.Fatalf("assessment = %#v", assessment)
			}
			if strings.Contains(assessment.Blocker.Message, "secret raw drift") {
				t.Fatal("checkpoint verifier error leaked through the assessment")
			}
		})
	}
}

func TestAssessContinuationRejectsUntrustedSourceStatesAndDependencies(t *testing.T) {
	t.Parallel()
	t.Run("active", func(t *testing.T) {
		t.Parallel()
		executor, request, store, _, _ := checkpointExecutor(t, "update-corepack", "", "", "", false)
		if _, err := executor.Execute(t.Context(), request); err == nil {
			t.Fatal("injected execution unexpectedly succeeded")
		}
		var active Record
		for _, candidate := range store.records {
			if candidate.State == StateRunning || candidate.State == StateVerifying {
				active = candidate
				break
			}
		}
		assessment, err := AssessContinuation(t.Context(), active, executor.Registry)
		if err != nil {
			t.Fatal(err)
		}
		assertContinuationBlock(t, assessment, ContinuationBlockSourceNotTerminal, "")
	})

	t.Run("terminal record still contains active step", func(t *testing.T) {
		t.Parallel()
		source, registry, _, _ := checkpointSource(t, "update-npm", "", "", "", false)
		source.Steps[0].State = StateRunning
		source.Steps[0].FinishedAt = nil
		source.Steps[0].Error = nil
		assessment, err := AssessContinuation(t.Context(), source, registry)
		if err != nil {
			t.Fatal(err)
		}
		assertContinuationBlock(t, assessment, ContinuationBlockSourceNotTerminal, "update-npm")
	})

	t.Run("completed", func(t *testing.T) {
		t.Parallel()
		source, registry, _, _ := checkpointSource(t, "", "", "", "", false)
		assessment, err := AssessContinuation(t.Context(), source, registry)
		if err != nil {
			t.Fatal(err)
		}
		assertContinuationBlock(t, assessment, ContinuationBlockSourceCompleted, "")
	})

	t.Run("legacy", func(t *testing.T) {
		t.Parallel()
		for _, version := range []string{PreviousRecordSchemaVersion, LegacyRecordSchemaVersion} {
			source, registry, _, _ := checkpointSource(t, "update-npm", "", "", "", false)
			source.SchemaVersion = version
			source.ConfirmedPlan = nil
			for index := range source.Steps {
				source.Steps[index].Before = nil
				source.Steps[index].After = nil
				source.Steps[index].Diff = nil
				source.Steps[index].Skipped = false
			}
			data, err := MarshalRecord(source)
			if err != nil {
				t.Fatal(err)
			}
			source, err = DecodeRecord(data)
			if err != nil {
				t.Fatal(err)
			}
			assessment, err := AssessContinuation(t.Context(), source, registry)
			if err != nil {
				t.Fatal(err)
			}
			assertContinuationBlock(t, assessment, ContinuationBlockUnsupportedRecord, "")
		}
	})

	t.Run("tampered", func(t *testing.T) {
		t.Parallel()
		source, registry, _, _ := checkpointSource(t, "update-corepack", "", "", "", false)
		source.ConfirmedPlan.Actions[0].TargetVersion = "99.0.0"
		if _, err := AssessContinuation(t.Context(), source, registry); err == nil {
			t.Fatal("tampered source record was accepted")
		}
	})

	t.Run("completed dependency was not reusable", func(t *testing.T) {
		t.Parallel()
		source, registry, _, _ := checkpointSource(t, "update-npm", "", "", "", false)
		checkpoint, err := NewSnapshot(map[string]string{"action_id": "update-corepack", "target_version": "0.35.0"})
		if err != nil {
			t.Fatal(err)
		}
		source.Steps[1].State = StateCompleted
		source.Steps[1].Verification = CheckResult{State: CheckPassed}
		source.Steps[1].FinishedAt = source.FinishedAt
		source.Steps[1].After = &checkpoint
		assessment, err := AssessContinuation(t.Context(), source, registry)
		if err != nil {
			t.Fatal(err)
		}
		assertContinuationBlock(t, assessment, ContinuationBlockDependencyUnverified, "update-corepack")
	})

	t.Run("terminal source has nothing remaining", func(t *testing.T) {
		t.Parallel()
		source, registry, _, _ := checkpointSource(t, "", "", "", "", false)
		source.State = StateFailed
		source.Transitions[len(source.Transitions)-1].State = StateFailed
		assessment, err := AssessContinuation(t.Context(), source, registry)
		if err != nil {
			t.Fatal(err)
		}
		assertContinuationBlock(t, assessment, ContinuationBlockNothingToContinue, "")
	})
}

func TestAssessContinuationAcceptsEveryPartialTerminalState(t *testing.T) {
	t.Parallel()
	for _, state := range []State{StateFailed, StateTimedOut, StateCancelled, StateInterrupted} {
		state := state
		t.Run(string(state), func(t *testing.T) {
			t.Parallel()
			source, registry, _, _ := checkpointSource(t, "update-npm", "", "", "", false)
			source.State = state
			source.Steps[0].State = state
			source.Transitions[len(source.Transitions)-1].State = state
			assessment, err := AssessContinuation(t.Context(), source, registry)
			if err != nil {
				t.Fatal(err)
			}
			if !assessment.Eligible || len(assessment.ReusableActionIDs) != 0 ||
				len(assessment.RemainingActionIDs) != 3 {
				t.Fatalf("assessment = %#v", assessment)
			}
		})
	}
}

func TestAssessContinuationHonorsContextCancellation(t *testing.T) {
	t.Parallel()
	source, registry, _, _ := checkpointSource(t, "update-corepack", "", "", "", false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := AssessContinuation(ctx, source, registry); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func checkpointSource(
	t *testing.T,
	processFailure, verificationFailure, driftAction, missingVerifier string,
	invalidEvidence bool,
) (Record, Registry, *actionRunner, map[string]int) {
	t.Helper()
	executor, request, _, runner, calls := checkpointExecutor(
		t, processFailure, verificationFailure, driftAction, missingVerifier, invalidEvidence,
	)
	record, err := executor.Execute(t.Context(), request)
	if processFailure == "" && verificationFailure == "" {
		if err != nil {
			t.Fatal(err)
		}
	} else if err == nil {
		t.Fatal("injected execution unexpectedly succeeded")
	}
	definitions := make([]Definition, 0, len(request.Plan.Actions))
	for _, action := range request.Plan.Actions {
		definition, err := executor.Registry.Resolve(action)
		if err != nil {
			t.Fatal(err)
		}
		definition.Build = func(plan.Action) (CommandSpec, error) {
			panic("continuation assessment invoked action Build")
		}
		definitions = append(definitions, definition)
	}
	assessmentRegistry, err := NewRegistry(definitions...)
	if err != nil {
		t.Fatal(err)
	}
	return record, assessmentRegistry, runner, calls
}

func checkpointExecutor(
	t *testing.T,
	processFailure, verificationFailure, driftAction, missingVerifier string,
	invalidEvidence bool,
) (Executor, Request, *memoryStore, *actionRunner, map[string]int) {
	t.Helper()
	executor, request, store, runner := dagTestHarness(t, processFailure, verificationFailure)
	definitions := make([]Definition, 0, len(request.Plan.Actions))
	calls := make(map[string]int, len(request.Plan.Actions))
	for _, action := range request.Plan.Actions {
		action := action
		definition, err := executor.Registry.Resolve(action)
		if err != nil {
			t.Fatal(err)
		}
		definition.Capture = func(_ context.Context, candidate plan.Action) (Snapshot, error) {
			return NewSnapshot(map[string]string{
				"action_id": candidate.ID, "target_version": candidate.TargetVersion,
			})
		}
		if action.ID == missingVerifier {
			definition.RevalidateCheckpoint = nil
		} else {
			definition.RevalidateCheckpoint = func(_ context.Context, candidate plan.Action, recorded Snapshot) (Snapshot, error) {
				calls[candidate.ID]++
				if len(candidate.Preconditions) > 0 {
					candidate.Preconditions[0].Expected = "callback mutation"
				}
				if candidate.ID == driftAction {
					return Snapshot{}, errors.New("secret raw drift")
				}
				if invalidEvidence && candidate.ID == "update-npm" {
					return Snapshot{Digest: recorded.Digest, Facts: map[string]string{"invalid": "evidence"}}, nil
				}
				recorded.Facts["fresh_probe"] = "passed"
				return NewSnapshot(recorded.Facts)
			}
		}
		definitions = append(definitions, definition)
	}
	registry, err := NewRegistry(definitions...)
	if err != nil {
		t.Fatal(err)
	}
	executor.Registry = registry
	return executor, request, store, runner, calls
}

func assertContinuationBlock(t *testing.T, assessment ContinuationAssessment, code ContinuationBlockCode, actionID string) {
	t.Helper()
	if assessment.Eligible || assessment.Blocker == nil ||
		assessment.Blocker.Code != code || assessment.Blocker.ActionID != actionID {
		t.Fatalf("assessment = %#v", assessment)
	}
}
