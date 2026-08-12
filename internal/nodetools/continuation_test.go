package nodetools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/plan"
)

func TestPrepareContinuationBuildsReviewOnlyPlanForEveryFailurePosition(t *testing.T) {
	requirePOSIXFixture(t)
	tests := []struct {
		failure               string
		remaining             []string
		checkpoints           []string
		satisfiedDependencies []plan.SatisfiedDependency
	}{
		{
			failure: "npm",
			remaining: []string{
				"update-npm", "update-corepack", "update-pnpm",
			},
		},
		{
			failure:     "corepack",
			remaining:   []string{"update-corepack", "update-pnpm"},
			checkpoints: []string{"update-npm"},
			satisfiedDependencies: []plan.SatisfiedDependency{{
				ActionID: "update-corepack", DependencyActionID: "update-npm",
			}},
		},
		{
			failure:     "pnpm",
			remaining:   []string{"update-pnpm"},
			checkpoints: []string{"update-npm", "update-corepack"},
			satisfiedDependencies: []plan.SatisfiedDependency{{
				ActionID: "update-pnpm", DependencyActionID: "update-corepack",
			}},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.failure, func(t *testing.T) {
			service, source, prepared := failedNodeToolsSource(t, test.failure)
			historyBefore := historySnapshot(t, service.HistoryRoot)
			scan := service.Scan
			scans := 0
			service.Scan = func(ctx context.Context) (inventory.Inventory, error) {
				scans++
				return scan(ctx)
			}
			writeCalls := 0
			service.Runner = continuationProbeRunner(prepared, &writeCalls)
			service.Now = func() time.Time { return source.UpdatedAt.Add(time.Minute) }

			result, err := service.PrepareContinuation(t.Context(), source.ID)
			if err != nil {
				t.Fatal(err)
			}
			value := result.Plan
			if value.SchemaVersion != plan.ContinuationSchemaVersion || value.Executable ||
				value.CreatedAt != source.UpdatedAt.Add(time.Minute) ||
				value.ExpiresAt.Sub(value.CreatedAt) != plan.DefaultTTL {
				t.Fatalf("continuation Plan identity/time = %#v", value)
			}
			if value.Continuation == nil ||
				value.Continuation.SourceOperationID != source.ID ||
				value.Continuation.SourcePlanID != source.PlanID ||
				!slices.Equal(actionIDs(value.Actions), test.remaining) ||
				!slices.Equal(checkpointIDs(value.Continuation.ReusableCheckpoints), test.checkpoints) ||
				!slices.Equal(value.Continuation.SatisfiedDependencies, test.satisfiedDependencies) {
				t.Fatalf("continuation Plan = %#v", value)
			}
			if scans != 1 || writeCalls != 0 {
				t.Fatalf("scan/write calls = %d/%d", scans, writeCalls)
			}
			if after := historySnapshot(t, service.HistoryRoot); !historyEqual(historyBefore, after) {
				t.Fatal("read-only continuation preparation changed operation history")
			}
			data, err := plan.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(data, []byte(prepared.baseline.NVM.Directory)) ||
				bytes.Contains(data, []byte(`"command"`)) ||
				bytes.Contains(data, []byte(`"args"`)) {
				t.Fatalf("continuation Plan leaked private or executable data: %s", data)
			}
		})
	}
}

func TestReviewRecoveryRevalidatesChangedUpstreamAndKeepsFailureUncertain(t *testing.T) {
	requirePOSIXFixture(t)
	service, source, prepared, runner := changedUpstreamNodeToolsSource(t)
	historyBefore := historySnapshot(t, service.HistoryRoot)
	scan := service.Scan
	scans := 0
	service.Scan = func(ctx context.Context) (inventory.Inventory, error) {
		scans++
		return scan(ctx)
	}
	runner.writeCalls = 0
	service.Runner = runner

	result, err := service.ReviewRecovery(t.Context(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if scans != 1 || runner.writeCalls != 0 || len(result.Candidates) != 2 ||
		result.Candidates[0].ActionID != "update-npm" ||
		result.Candidates[0].CurrentState != execution.RecoveryCheckpointCurrent ||
		result.Candidates[1].ActionID != "update-corepack" ||
		result.Candidates[1].CurrentState != execution.RecoveryCheckpointUncertain {
		t.Fatalf("recovery review = %#v, scans/writes = %d/%d", result, scans, runner.writeCalls)
	}
	if after := historySnapshot(t, service.HistoryRoot); !historyEqual(historyBefore, after) {
		t.Fatal("Node tools recovery review changed operation history")
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{
		prepared.baseline.NVM.Directory,
		prepared.baseline.NPM.PackageRoot,
		prepared.baseline.NPM.Executable,
		`"command"`, `"args"`, `"facts"`,
	} {
		if private != "" && bytes.Contains(data, []byte(private)) {
			t.Fatalf("recovery review leaked %q: %s", private, data)
		}
	}
}

func TestReviewRecoveryReportsUpstreamDriftWithoutWriteCall(t *testing.T) {
	requirePOSIXFixture(t)
	service, source, prepared, runner := changedUpstreamNodeToolsSource(t)
	if err := os.WriteFile(
		filepath.Join(prepared.baseline.NPM.PackageRoot, "package.json"),
		[]byte(`{"name":"npm","version":"99.0.0"}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	scans := 0
	scan := service.Scan
	service.Scan = func(ctx context.Context) (inventory.Inventory, error) {
		scans++
		return scan(ctx)
	}
	runner.writeCalls = 0
	service.Runner = runner

	result, err := service.ReviewRecovery(t.Context(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if scans != 1 || runner.writeCalls != 0 || len(result.Candidates) != 2 ||
		result.Candidates[0].CurrentState != execution.RecoveryCheckpointDrifted ||
		result.Candidates[1].CurrentState != execution.RecoveryCheckpointUncertain {
		t.Fatalf("drifted recovery review = %#v, scans/writes = %d/%d", result, scans, runner.writeCalls)
	}
}

func TestReviewRecoveryFirstFailureNeedsNoEnvironmentScan(t *testing.T) {
	requirePOSIXFixture(t)
	service, source, _ := failedNodeToolsSource(t, "npm")
	scans := 0
	service.Scan = func(context.Context) (inventory.Inventory, error) {
		scans++
		return inventory.Inventory{}, errors.New("scan must not be called")
	}

	result, err := service.ReviewRecovery(t.Context(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if scans != 0 || len(result.Candidates) != 1 ||
		result.Candidates[0].ActionID != "update-npm" ||
		result.Candidates[0].CurrentState != execution.RecoveryCheckpointUncertain {
		t.Fatalf("first-failure review = %#v, scans = %d", result, scans)
	}
}

func TestReviewRecoveryRejectsNonNodeToolsSourceBeforeScan(t *testing.T) {
	requirePOSIXFixture(t)
	service, _, _ := nodeToolsServiceFixture(t)
	source := failedSelfTestSource(t, service.HistoryRoot)
	scans := 0
	service.Scan = func(context.Context) (inventory.Inventory, error) {
		scans++
		return inventory.Inventory{}, errors.New("scan must not be called")
	}

	if _, err := service.ReviewRecovery(t.Context(), source.ID); err == nil ||
		!strings.Contains(err.Error(), "source is unsupported") ||
		scans != 0 {
		t.Fatalf("unsupported recovery source error = %v, scans = %d", err, scans)
	}
}

func TestReviewRecoverySupportsCompletedChangedSource(t *testing.T) {
	requirePOSIXFixture(t)
	service, source, prepared, runner := completedChangedNodeToolsSource(t)
	historyBefore := historySnapshot(t, service.HistoryRoot)
	scans := 0
	scan := service.Scan
	service.Scan = func(ctx context.Context) (inventory.Inventory, error) {
		scans++
		return scan(ctx)
	}
	runner.writeCalls = 0
	service.Runner = runner

	result, err := service.ReviewRecovery(t.Context(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if scans != 1 || runner.writeCalls != 0 || len(result.Candidates) != 1 ||
		result.Candidates[0].ActionID != "update-npm" ||
		result.Candidates[0].StepState != execution.StateCompleted ||
		result.Candidates[0].Evidence != execution.RecoveryEvidenceChanged ||
		result.Candidates[0].CurrentState != execution.RecoveryCheckpointCurrent {
		t.Fatalf("completed recovery review = %#v, scans/writes = %d/%d", result, scans, runner.writeCalls)
	}
	if after := historySnapshot(t, service.HistoryRoot); !historyEqual(historyBefore, after) {
		t.Fatal("completed recovery review changed operation history")
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(prepared.baseline.NPM.PackageRoot)) {
		t.Fatal("completed recovery review leaked package path")
	}
}

func TestReviewRecoveryCompletedUnchangedNeedsNoScan(t *testing.T) {
	requirePOSIXFixture(t)
	service, source := completedNodeToolsSource(t)
	scans := 0
	service.Scan = func(context.Context) (inventory.Inventory, error) {
		scans++
		return inventory.Inventory{}, errors.New("scan must not be called")
	}

	result, err := service.ReviewRecovery(t.Context(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if scans != 0 || result.SourceOperationID != source.ID ||
		result.Candidates == nil || len(result.Candidates) != 0 {
		t.Fatalf("completed unchanged review = %#v, scans = %d", result, scans)
	}
}

func TestPrepareContinuationRetainsRecord03FirstContinuationCompatibility(t *testing.T) {
	requirePOSIXFixture(t)
	service, source, prepared := failedNodeToolsSource(t, "corepack")
	source.SchemaVersion = execution.PreviousRecordSchemaVersion
	if err := (execution.FileStore{Root: service.HistoryRoot}).Save(source); err != nil {
		t.Fatal(err)
	}
	writeCalls := 0
	service.Runner = continuationProbeRunner(prepared, &writeCalls)
	service.Now = func() time.Time { return source.UpdatedAt.Add(time.Minute) }

	result, err := service.PrepareContinuation(t.Context(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(actionIDs(result.Plan.Actions), []string{"update-corepack", "update-pnpm"}) ||
		result.Plan.Continuation == nil ||
		result.Plan.Continuation.SourceOperationID != source.ID {
		t.Fatalf("Record 0.3 Node tools continuation = %#v", result.Plan)
	}
	if writeCalls != 0 {
		t.Fatal("Record 0.3 continuation preparation started a write-shaped process")
	}
}

func TestPrepareContinuationSupportsRecord04Plan05AcrossRepeatedFailurePositions(t *testing.T) {
	requirePOSIXFixture(t)
	actionOrder := []string{"update-npm", "update-corepack", "update-pnpm"}
	satisfiedByFailure := [][]plan.SatisfiedDependency{
		nil,
		{{ActionID: "update-corepack", DependencyActionID: "update-npm"}},
		{{ActionID: "update-pnpm", DependencyActionID: "update-corepack"}},
	}
	for failureIndex, failure := range []string{"npm", "corepack", "pnpm"} {
		failureIndex := failureIndex
		failure := failure
		t.Run(failure, func(t *testing.T) {
			service, source, prepared := repeatedNodeToolsSource(t, failureIndex)
			historyBefore := historySnapshot(t, service.HistoryRoot)
			scan := service.Scan
			scans := 0
			service.Scan = func(ctx context.Context) (inventory.Inventory, error) {
				scans++
				return scan(ctx)
			}
			writeCalls := 0
			service.Runner = continuationProbeRunner(prepared, &writeCalls)
			service.Now = func() time.Time { return source.UpdatedAt.Add(time.Minute) }

			result, err := service.PrepareContinuation(t.Context(), source.ID)
			if err != nil {
				t.Fatal(err)
			}
			value := result.Plan
			if source.SchemaVersion != execution.RecordSchemaVersion ||
				source.ConfirmedPlan == nil ||
				source.ConfirmedPlan.SchemaVersion != plan.ExecutableContinuationSchemaVersion ||
				value.Continuation == nil ||
				value.Continuation.SourceOperationID != source.ID ||
				value.Continuation.SourcePlanID != source.PlanID ||
				!slices.Equal(actionIDs(value.Actions), actionOrder[failureIndex:]) ||
				!slices.Equal(
					checkpointIDs(value.Continuation.ReusableCheckpoints),
					actionOrder[:failureIndex],
				) ||
				!slices.Equal(
					value.Continuation.SatisfiedDependencies,
					satisfiedByFailure[failureIndex],
				) {
				t.Fatalf("repeated Node tools continuation = %#v from %#v", value, source)
			}
			if scans != 1 || writeCalls != 0 {
				t.Fatalf("scan/write calls = %d/%d", scans, writeCalls)
			}
			if after := historySnapshot(t, service.HistoryRoot); !historyEqual(historyBefore, after) {
				t.Fatal("repeated continuation preparation changed lineage history")
			}
		})
	}
}

func TestPrepareContinuationBlocksCheckpointDriftWithoutLeakingDetails(t *testing.T) {
	requirePOSIXFixture(t)
	service, source, prepared := failedNodeToolsSource(t, "corepack")
	if err := os.WriteFile(
		filepath.Join(prepared.baseline.NVM.Directory, "alias", "default"),
		[]byte("24.12.0\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	service.Now = func() time.Time { return source.UpdatedAt.Add(time.Minute) }
	writeCalls := 0
	service.Runner = continuationProbeRunner(prepared, &writeCalls)

	_, err := service.PrepareContinuation(t.Context(), source.ID)
	if err == nil || !strings.Contains(err.Error(), string(execution.ContinuationBlockCheckpointDrifted)) ||
		strings.Contains(err.Error(), prepared.baseline.NVM.Directory) {
		t.Fatalf("drift error = %v", err)
	}
	if writeCalls != 0 {
		t.Fatal("checkpoint drift started a write-shaped process")
	}
}

func TestPrepareContinuationRejectsUnsupportedSourcesBeforeEnvironmentScan(t *testing.T) {
	requirePOSIXFixture(t)
	t.Run("legacy record", func(t *testing.T) {
		service, source, _ := failedNodeToolsSource(t, "npm")
		source.SchemaVersion = execution.OlderRecordSchemaVersion
		source.ConfirmedPlan = nil
		if err := (execution.FileStore{Root: service.HistoryRoot}).Save(source); err != nil {
			t.Fatal(err)
		}
		assertContinuationRejectedBeforeScan(t, service, source.ID, "supported executable R1/R2 confirmed Plan")
	})

	t.Run("completed operation", func(t *testing.T) {
		service, source := completedNodeToolsSource(t)
		assertContinuationRejectedBeforeScan(t, service, source.ID, "terminal failure")
	})

	t.Run("non Node tools Plan", func(t *testing.T) {
		service, _, _ := nodeToolsServiceFixture(t)
		source := failedSelfTestSource(t, service.HistoryRoot)
		assertContinuationRejectedBeforeScan(t, service, source.ID, "supported Node tools Plan")
	})

	t.Run("invalid operation identity", func(t *testing.T) {
		service, _, _ := nodeToolsServiceFixture(t)
		assertContinuationRejectedBeforeScan(t, service, "../escape", "invalid")
	})
}

func TestPrepareContinuationKeepsFailedActionWhenCurrentStateAlreadyMatchesTarget(t *testing.T) {
	requirePOSIXFixture(t)
	service, source, prepared := failedNodeToolsSource(t, "corepack")
	packageJSON := filepath.Join(prepared.baseline.Corepack.PackageRoot, "package.json")
	if err := os.WriteFile(packageJSON, []byte(`{"name":"corepack","version":"0.35.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	service.Now = func() time.Time { return source.UpdatedAt.Add(time.Minute) }
	writeCalls := 0
	service.Runner = continuationProbeRunner(prepared, &writeCalls)

	result, err := service.PrepareContinuation(t.Context(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Plan.Actions) != 2 || result.Plan.Actions[0].ID != "update-corepack" {
		t.Fatalf("remaining actions = %#v", result.Plan.Actions)
	}
	current, err := requiredActionCheck(result.Plan.Actions[0].Preconditions, "current_tool_version_matches")
	if err != nil || current.Expected != "0.35.0" {
		t.Fatalf("fresh current-state check = %#v, %v", current, err)
	}
	if writeCalls != 0 {
		t.Fatal("already-satisfied failed action started a write-shaped process")
	}
}

func TestContinuationSourceRejectsUnsafeActionMetadata(t *testing.T) {
	requirePOSIXFixture(t)
	_, source, _ := failedNodeToolsSource(t, "npm")
	data, err := execution.MarshalRecord(source)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*execution.Record)
		want   string
	}{
		{
			name: "floating target",
			mutate: func(value *execution.Record) {
				value.ConfirmedPlan.Actions[0].TargetVersion = "latest"
			},
			want: "target version",
		},
		{
			name: "provider substitution",
			mutate: func(value *execution.Record) {
				value.ConfirmedPlan.Actions[1].Adapter = plan.NodeToolProviderCorepack
			},
			want: "identity or risk",
		},
		{
			name: "missing safety check",
			mutate: func(value *execution.Record) {
				value.ConfirmedPlan.Actions[0].Preconditions =
					value.ConfirmedPlan.Actions[0].Preconditions[:7]
			},
			want: "missing required safety metadata",
		},
		{
			name: "changed safety scope",
			mutate: func(value *execution.Record) {
				value.ConfirmedPlan.Actions[0].Preconditions[0].Subject = "other.runtime"
			},
			want: "safety scope",
		},
		{
			name: "changed verification scope",
			mutate: func(value *execution.Record) {
				value.ConfirmedPlan.Actions[0].Verifications[1].Expected = "other-node"
			},
			want: "verification metadata",
		},
		{
			name: "extra safety check",
			mutate: func(value *execution.Record) {
				value.ConfirmedPlan.Actions[0].Preconditions = append(
					value.ConfirmedPlan.Actions[0].Preconditions,
					plan.Check{Kind: "unexpected", Subject: "runtime.node", Expected: "accepted"},
				)
			},
			want: "unsupported safety metadata",
		},
		{
			name: "weakened execution constraint",
			mutate: func(value *execution.Record) {
				value.ConfirmedPlan.Actions[0].Confirmation.Required = false
			},
			want: "execution constraints",
		},
		{
			name: "changed dependency topology",
			mutate: func(value *execution.Record) {
				value.ConfirmedPlan.Actions[1].Dependencies = nil
			},
			want: "dependency topology",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			candidate, err := execution.DecodeRecord(data)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(&candidate)
			if _, _, err := continuationSource(candidate); err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRevalidateContinuationAcceptsUnchangedReviewedPlanWithoutSideEffects(t *testing.T) {
	requirePOSIXFixture(t)
	service, source, prepared := failedNodeToolsSource(t, "corepack")
	writeCalls := 0
	service.Runner = continuationProbeRunner(prepared, &writeCalls)
	service.Now = func() time.Time { return source.UpdatedAt.Add(time.Minute) }
	reviewed, err := service.PrepareContinuation(t.Context(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	reviewedBefore, err := plan.Marshal(reviewed.Plan)
	if err != nil {
		t.Fatal(err)
	}
	historyBefore := historySnapshot(t, service.HistoryRoot)
	scan := service.Scan
	scans := 0
	service.Scan = func(ctx context.Context) (inventory.Inventory, error) {
		scans++
		return scan(ctx)
	}
	writeCalls = 0
	service.Now = func() time.Time { return reviewed.Plan.CreatedAt.Add(time.Minute) }

	if err := service.RevalidateContinuation(t.Context(), reviewed.Plan); err != nil {
		t.Fatal(err)
	}
	reviewedAfter, err := plan.Marshal(reviewed.Plan)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(reviewedBefore, reviewedAfter) {
		t.Fatal("revalidation mutated the reviewed Plan")
	}
	if scans != 1 || writeCalls != 0 {
		t.Fatalf("scan/write calls = %d/%d", scans, writeCalls)
	}
	if after := historySnapshot(t, service.HistoryRoot); !historyEqual(historyBefore, after) {
		t.Fatal("revalidation changed operation history")
	}
}

func TestRevalidateContinuationRejectsInvalidTimeOrSchemaBeforeScan(t *testing.T) {
	requirePOSIXFixture(t)
	service, source, prepared := failedNodeToolsSource(t, "npm")
	writeCalls := 0
	service.Runner = continuationProbeRunner(prepared, &writeCalls)
	service.Now = func() time.Time { return source.UpdatedAt.Add(time.Minute) }
	reviewed, err := service.PrepareContinuation(t.Context(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := plan.Marshal(reviewed.Plan)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		value func() plan.Plan
		now   time.Time
		want  string
	}{
		{
			name: "invalid content",
			value: func() plan.Plan {
				value, decodeErr := plan.Decode(data)
				if decodeErr != nil {
					t.Fatal(decodeErr)
				}
				value.ID = "sha256:" + strings.Repeat("0", 64)
				return value
			},
			now:  reviewed.Plan.CreatedAt.Add(time.Minute),
			want: "reviewed Plan is invalid",
		},
		{
			name:  "wrong schema",
			value: func() plan.Plan { return prepared.Plan },
			now:   reviewed.Plan.CreatedAt.Add(time.Minute),
			want:  "review-only Plan 0.4.0",
		},
		{
			name:  "not yet valid",
			value: func() plan.Plan { return reviewed.Plan },
			now:   reviewed.Plan.CreatedAt.Add(-time.Nanosecond),
			want:  "not yet valid",
		},
		{
			name:  "expired",
			value: func() plan.Plan { return reviewed.Plan },
			now:   reviewed.Plan.ExpiresAt,
			want:  "expired",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			candidate := service
			scans := 0
			candidate.Scan = func(context.Context) (inventory.Inventory, error) {
				scans++
				return inventory.Inventory{}, errors.New("unexpected scan")
			}
			candidate.HistoryRoot = filepath.Join(t.TempDir(), "missing-history")
			candidate.Now = func() time.Time { return test.now }
			writeCalls = 0

			err := candidate.RevalidateContinuation(t.Context(), test.value())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if scans != 0 || writeCalls != 0 {
				t.Fatalf("scan/write calls = %d/%d", scans, writeCalls)
			}
		})
	}
}

func TestRevalidateContinuationRejectsCheckpointDriftWithoutWrites(t *testing.T) {
	requirePOSIXFixture(t)
	service, source, prepared := failedNodeToolsSource(t, "corepack")
	writeCalls := 0
	service.Runner = continuationProbeRunner(prepared, &writeCalls)
	service.Now = func() time.Time { return source.UpdatedAt.Add(time.Minute) }
	reviewed, err := service.PrepareContinuation(t.Context(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(prepared.baseline.NVM.Directory, "alias", "default"),
		[]byte("24.12.0\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	historyBefore := historySnapshot(t, service.HistoryRoot)
	scan := service.Scan
	scans := 0
	service.Scan = func(ctx context.Context) (inventory.Inventory, error) {
		scans++
		return scan(ctx)
	}
	writeCalls = 0
	service.Now = func() time.Time { return reviewed.Plan.CreatedAt.Add(time.Minute) }

	err = service.RevalidateContinuation(t.Context(), reviewed.Plan)
	if err == nil ||
		!strings.Contains(err.Error(), string(execution.ContinuationBlockCheckpointDrifted)) ||
		strings.Contains(err.Error(), prepared.baseline.NVM.Directory) {
		t.Fatalf("drift error = %v", err)
	}
	if scans != 1 || writeCalls != 0 {
		t.Fatalf("scan/write calls = %d/%d", scans, writeCalls)
	}
	if after := historySnapshot(t, service.HistoryRoot); !historyEqual(historyBefore, after) {
		t.Fatal("drift revalidation changed operation history")
	}
}

func TestRevalidateContinuationRejectsChangedRemainingActionFacts(t *testing.T) {
	requirePOSIXFixture(t)
	service, source, prepared := failedNodeToolsSource(t, "npm")
	writeCalls := 0
	service.Runner = continuationProbeRunner(prepared, &writeCalls)
	service.Now = func() time.Time { return source.UpdatedAt.Add(time.Minute) }
	reviewed, err := service.PrepareContinuation(t.Context(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	packageJSON := filepath.Join(prepared.baseline.NPM.PackageRoot, "package.json")
	if err := os.WriteFile(packageJSON, []byte(`{"name":"npm","version":"11.6.3"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	historyBefore := historySnapshot(t, service.HistoryRoot)
	scan := service.Scan
	scans := 0
	service.Scan = func(ctx context.Context) (inventory.Inventory, error) {
		scans++
		return scan(ctx)
	}
	writeCalls = 0
	service.Now = func() time.Time { return reviewed.Plan.CreatedAt.Add(time.Minute) }

	err = service.RevalidateContinuation(t.Context(), reviewed.Plan)
	if err == nil || !strings.Contains(err.Error(), "state changed after continuation review") ||
		strings.Contains(err.Error(), prepared.baseline.NVM.Directory) {
		t.Fatalf("changed-state error = %v", err)
	}
	if scans != 1 || writeCalls != 0 {
		t.Fatalf("scan/write calls = %d/%d", scans, writeCalls)
	}
	if after := historySnapshot(t, service.HistoryRoot); !historyEqual(historyBefore, after) {
		t.Fatal("changed-state revalidation changed operation history")
	}
}

func TestExecuteContinuationRejectsInvalidPlanTimeAndConfirmationBeforeSourceRead(t *testing.T) {
	requirePOSIXFixture(t)
	service, source, prepared := failedNodeToolsSource(t, "npm")
	writeCalls := 0
	service.Runner = continuationProbeRunner(prepared, &writeCalls)
	service.Now = func() time.Time { return source.UpdatedAt.Add(time.Minute) }
	reviewed, err := service.PrepareContinuation(t.Context(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	finalPlan, err := plan.BuildExecutableContinuation(reviewed.Plan)
	if err != nil {
		t.Fatal(err)
	}
	validReceipt := execution.ConfirmationReceipt{
		Scope: "plan", ConfirmedPlanID: finalPlan.ID, ConfirmedAt: finalPlan.CreatedAt,
	}
	tests := []struct {
		name    string
		value   func() plan.Plan
		receipt execution.ConfirmationReceipt
		now     time.Time
		want    string
	}{
		{
			name: "invalid immutable content",
			value: func() plan.Plan {
				value := finalPlan
				value.ID = "sha256:" + strings.Repeat("0", 64)
				return value
			},
			receipt: validReceipt, now: finalPlan.CreatedAt.Add(time.Minute),
			want: "final Plan is invalid",
		},
		{
			name:    "review draft",
			value:   func() plan.Plan { return reviewed.Plan },
			receipt: validReceipt, now: finalPlan.CreatedAt.Add(time.Minute),
			want: "executable Plan 0.5.0",
		},
		{
			name:    "not yet valid",
			value:   func() plan.Plan { return finalPlan },
			receipt: validReceipt, now: finalPlan.CreatedAt.Add(-time.Nanosecond),
			want: "not yet valid",
		},
		{
			name:    "expired",
			value:   func() plan.Plan { return finalPlan },
			receipt: validReceipt, now: finalPlan.ExpiresAt,
			want: "has expired",
		},
		{
			name:  "wrong confirmation",
			value: func() plan.Plan { return finalPlan },
			receipt: execution.ConfirmationReceipt{
				Scope:           "plan",
				ConfirmedPlanID: "sha256:" + strings.Repeat("f", 64),
				ConfirmedAt:     finalPlan.CreatedAt,
			},
			now: finalPlan.CreatedAt.Add(time.Minute), want: "confirmation",
		},
		{
			name:  "future confirmation",
			value: func() plan.Plan { return finalPlan },
			receipt: execution.ConfirmationReceipt{
				Scope:           "plan",
				ConfirmedPlanID: finalPlan.ID,
				ConfirmedAt:     finalPlan.CreatedAt.Add(2 * time.Minute),
			},
			now: finalPlan.CreatedAt.Add(time.Minute), want: "confirmation",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			candidate := service
			scans := 0
			candidate.Scan = func(context.Context) (inventory.Inventory, error) {
				scans++
				return inventory.Inventory{}, errors.New("unexpected scan")
			}
			candidate.HistoryRoot = filepath.Join(t.TempDir(), "missing-history")
			candidate.Now = func() time.Time { return test.now }
			writeCalls = 0
			_, err := candidate.ExecuteContinuation(t.Context(), test.value(), test.receipt)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if scans != 0 || writeCalls != 0 {
				t.Fatalf("scan/write calls = %d/%d", scans, writeCalls)
			}
		})
	}
}

func TestExecuteContinuationRejectsFinalIDDriftBeforeHistoryOrWriteAction(t *testing.T) {
	requirePOSIXFixture(t)
	service, source, prepared := failedNodeToolsSource(t, "npm")
	writeCalls := 0
	service.Runner = continuationProbeRunner(prepared, &writeCalls)
	service.Now = func() time.Time { return source.UpdatedAt.Add(time.Minute) }
	reviewed, err := service.PrepareContinuation(t.Context(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	finalPlan, err := plan.BuildExecutableContinuation(reviewed.Plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(prepared.baseline.NPM.PackageRoot, "package.json"),
		[]byte(`{"name":"npm","version":"11.6.3"}`),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	historyBefore := historySnapshot(t, service.HistoryRoot)
	scan := service.Scan
	scans := 0
	service.Scan = func(ctx context.Context) (inventory.Inventory, error) {
		scans++
		return scan(ctx)
	}
	writeCalls = 0
	service.Now = func() time.Time { return finalPlan.CreatedAt.Add(time.Minute) }

	_, err = service.ExecuteContinuation(t.Context(), finalPlan, execution.ConfirmationReceipt{
		Scope: "plan", ConfirmedPlanID: finalPlan.ID, ConfirmedAt: finalPlan.CreatedAt,
	})
	if err == nil || !strings.Contains(err.Error(), "generate and confirm a new Plan") {
		t.Fatalf("drift error = %v", err)
	}
	if scans != 1 || writeCalls != 0 {
		t.Fatalf("scan/write calls = %d/%d", scans, writeCalls)
	}
	if after := historySnapshot(t, service.HistoryRoot); !historyEqual(historyBefore, after) {
		t.Fatal("final ID drift changed operation history")
	}
}

func TestExecuteContinuationRejectsCheckpointDriftBeforeHistoryOrWriteAction(t *testing.T) {
	requirePOSIXFixture(t)
	service, source, prepared := failedNodeToolsSource(t, "corepack")
	writeCalls := 0
	service.Runner = continuationProbeRunner(prepared, &writeCalls)
	service.Now = func() time.Time { return source.UpdatedAt.Add(time.Minute) }
	reviewed, err := service.PrepareContinuation(t.Context(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	finalPlan, err := plan.BuildExecutableContinuation(reviewed.Plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(prepared.baseline.NVM.Directory, "alias", "default"),
		[]byte("24.12.0\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	historyBefore := historySnapshot(t, service.HistoryRoot)
	scan := service.Scan
	scans := 0
	service.Scan = func(ctx context.Context) (inventory.Inventory, error) {
		scans++
		return scan(ctx)
	}
	writeCalls = 0
	service.Now = func() time.Time { return finalPlan.CreatedAt.Add(time.Minute) }

	_, err = service.ExecuteContinuation(t.Context(), finalPlan, execution.ConfirmationReceipt{
		Scope: "plan", ConfirmedPlanID: finalPlan.ID, ConfirmedAt: finalPlan.CreatedAt,
	})
	if err == nil ||
		!strings.Contains(err.Error(), string(execution.ContinuationBlockCheckpointDrifted)) ||
		strings.Contains(err.Error(), prepared.baseline.NVM.Directory) {
		t.Fatalf("checkpoint drift error = %v", err)
	}
	if scans != 1 || writeCalls != 0 {
		t.Fatalf("scan/write calls = %d/%d", scans, writeCalls)
	}
	if after := historySnapshot(t, service.HistoryRoot); !historyEqual(historyBefore, after) {
		t.Fatal("checkpoint drift changed operation history")
	}
}

func TestExecuteContinuationWritesOnlyRemainingPlan05Steps(t *testing.T) {
	requirePOSIXFixture(t)
	service, source, prepared := failedNodeToolsSource(t, "corepack")
	clock := source.UpdatedAt.Add(time.Minute)
	service.Now = func() time.Time { return clock }
	readRunner := newContinuationExecutionRunner(t, prepared)
	service.Runner = readRunner
	reviewed, err := service.PrepareContinuation(t.Context(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	finalPlan, err := plan.BuildExecutableContinuation(reviewed.Plan)
	if err != nil {
		t.Fatal(err)
	}
	finalBefore, err := plan.Marshal(finalPlan)
	if err != nil {
		t.Fatal(err)
	}
	historyBefore := historySnapshot(t, service.HistoryRoot)
	scan := service.Scan
	scans := 0
	service.Scan = func(ctx context.Context) (inventory.Inventory, error) {
		scans++
		return scan(ctx)
	}
	clock = finalPlan.CreatedAt.Add(time.Minute)

	result, err := service.ExecuteContinuation(t.Context(), finalPlan, execution.ConfirmationReceipt{
		Scope: "plan", ConfirmedPlanID: finalPlan.ID, ConfirmedAt: finalPlan.CreatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if scans != 2 || readRunner.writeCalls != 2 {
		t.Fatalf("scan/write calls = %d/%d", scans, readRunner.writeCalls)
	}
	record := result.Record
	if result.RecordPath == "" ||
		record.SchemaVersion != execution.RecordSchemaVersion ||
		record.PlanSchemaVersion != plan.ExecutableContinuationSchemaVersion ||
		record.PlanID != finalPlan.ID ||
		record.ConfirmedPlan == nil || record.ConfirmedPlan.ID != finalPlan.ID ||
		record.Confirmation.ConfirmedPlanID != finalPlan.ID ||
		record.State != execution.StateCompleted ||
		!slices.Equal(stepActionIDs(record.Steps), []string{"update-corepack", "update-pnpm"}) {
		t.Fatalf("continuation execution record = %#v", record)
	}
	for _, step := range record.Steps {
		if step.State != execution.StateCompleted ||
			step.Verification.State != execution.CheckPassed {
			t.Fatalf("continuation execution step = %#v", step)
		}
	}
	if result.Outcome == nil ||
		!slices.Equal(outcomeActionIDs(result.Outcome.Actions),
			[]string{"update-corepack", "update-pnpm"}) ||
		!slices.Equal(outcomeVerified(result.Outcome.Actions), []bool{true, true}) ||
		result.Outcome.Actions[0].BeforeVersion != "0.34.5" ||
		result.Outcome.Actions[0].AfterVersion != "0.35.0" ||
		result.Outcome.Actions[0].Provider != plan.NodeToolProviderNPM ||
		result.Outcome.Actions[1].BeforeVersion != "10.0.0" ||
		result.Outcome.Actions[1].AfterVersion != "11.1.0" ||
		result.Outcome.Actions[1].Provider != plan.NodeToolProviderCorepack {
		t.Fatalf("continuation outcome = %#v", result.Outcome)
	}
	outcomeJSON, err := json.Marshal(result.Outcome)
	if err != nil {
		t.Fatal(err)
	}
	for _, sensitive := range []string{
		prepared.baseline.NVM.Directory,
		prepared.baseline.Corepack.PackageRoot,
		prepared.baseline.PNPM.Executable,
		`"command"`,
		`"args"`,
	} {
		if sensitive != "" && bytes.Contains(outcomeJSON, []byte(sensitive)) {
			t.Fatalf("continuation outcome leaked %q: %s", sensitive, outcomeJSON)
		}
	}
	finalAfter, err := plan.Marshal(finalPlan)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(finalBefore, finalAfter) {
		t.Fatal("continuation execution mutated the confirmed final Plan")
	}
	historyAfter := historySnapshot(t, service.HistoryRoot)
	assertHistoryPrefixUnchanged(t, historyBefore, historyAfter)
	if len(historyAfter) != len(historyBefore)+1 {
		t.Fatalf("history entries = %d, want %d", len(historyAfter), len(historyBefore)+1)
	}
}

func TestContinuationOutcomeDoesNotMisreportAnyFailurePosition(t *testing.T) {
	requirePOSIXFixture(t)
	for failureIndex, failureTool := range []string{"npm", "corepack", "pnpm"} {
		failureIndex := failureIndex
		failureTool := failureTool
		t.Run(failureTool, func(t *testing.T) {
			service, source, prepared := failedNodeToolsSource(t, "npm")
			clock := source.UpdatedAt.Add(time.Minute)
			service.Now = func() time.Time { return clock }
			runner := newContinuationExecutionRunner(t, prepared)
			runner.failTool = failureTool
			service.Runner = runner
			reviewed, err := service.PrepareContinuation(t.Context(), source.ID)
			if err != nil {
				t.Fatal(err)
			}
			finalPlan, err := plan.BuildExecutableContinuation(reviewed.Plan)
			if err != nil {
				t.Fatal(err)
			}
			scan := service.Scan
			scans := 0
			service.Scan = func(ctx context.Context) (inventory.Inventory, error) {
				scans++
				return scan(ctx)
			}
			clock = finalPlan.CreatedAt.Add(time.Minute)
			result, err := service.ExecuteContinuation(
				t.Context(),
				finalPlan,
				execution.ConfirmationReceipt{
					Scope: "plan", ConfirmedPlanID: finalPlan.ID,
					ConfirmedAt: finalPlan.CreatedAt,
				},
			)
			if err == nil || scans != 2 || result.Outcome == nil {
				t.Fatalf("failure result = %#v, error=%v, scans=%d", result, err, scans)
			}
			wantStates := make([]execution.State, len(finalPlan.Actions))
			wantVerified := make([]bool, len(finalPlan.Actions))
			for index := range wantStates {
				switch {
				case index < failureIndex:
					wantStates[index] = execution.StateCompleted
					wantVerified[index] = true
				case index == failureIndex:
					wantStates[index] = execution.StateFailed
				default:
					wantStates[index] = execution.StatePending
				}
			}
			if !slices.Equal(stepStates(result.Record.Steps), wantStates) ||
				!slices.Equal(outcomeStates(result.Outcome.Actions), wantStates) ||
				!slices.Equal(outcomeVerified(result.Outcome.Actions), wantVerified) {
				t.Fatalf("failure outcome = %#v, record=%#v", result.Outcome, result.Record)
			}
		})
	}
}

func TestExecuteContinuationFailureCanBePreparedAndExecutedAgain(t *testing.T) {
	requirePOSIXFixture(t)
	service, original, prepared := failedNodeToolsSource(t, "npm")
	clock := original.UpdatedAt.Add(time.Minute)
	service.Now = func() time.Time { return clock }
	runner := newContinuationExecutionRunner(t, prepared)
	runner.failTool = "corepack"
	service.Runner = runner
	firstReview, err := service.PrepareContinuation(t.Context(), original.ID)
	if err != nil {
		t.Fatal(err)
	}
	firstFinal, err := plan.BuildExecutableContinuation(firstReview.Plan)
	if err != nil {
		t.Fatal(err)
	}
	lineageBefore := historySnapshot(t, service.HistoryRoot)
	clock = firstFinal.CreatedAt.Add(time.Minute)
	firstResult, err := service.ExecuteContinuation(t.Context(), firstFinal, execution.ConfirmationReceipt{
		Scope: "plan", ConfirmedPlanID: firstFinal.ID, ConfirmedAt: firstFinal.CreatedAt,
	})
	if err == nil || firstResult.Record.State != execution.StateFailed ||
		!slices.Equal(stepStates(firstResult.Record.Steps),
			[]execution.State{
				execution.StateCompleted,
				execution.StateFailed,
				execution.StatePending,
			}) {
		t.Fatalf("first continuation result = %#v, %v", firstResult, err)
	}
	if firstResult.Record.ConfirmedPlan == nil ||
		firstResult.Record.ConfirmedPlan.SchemaVersion != plan.ExecutableContinuationSchemaVersion {
		t.Fatalf("first continuation confirmed Plan = %#v", firstResult.Record.ConfirmedPlan)
	}
	if firstResult.Outcome == nil ||
		!slices.Equal(outcomeVerified(firstResult.Outcome.Actions),
			[]bool{true, false, false}) ||
		!slices.Equal(outcomeStates(firstResult.Outcome.Actions),
			[]execution.State{
				execution.StateCompleted,
				execution.StateFailed,
				execution.StatePending,
			}) {
		t.Fatalf("first continuation outcome = %#v", firstResult.Outcome)
	}
	assertHistoryPrefixUnchanged(t, lineageBefore, historySnapshot(t, service.HistoryRoot))

	clock = firstResult.Record.UpdatedAt.Add(time.Minute)
	secondReview, err := service.PrepareContinuation(t.Context(), firstResult.Record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(actionIDs(secondReview.Plan.Actions),
		[]string{"update-corepack", "update-pnpm"}) ||
		secondReview.Plan.Continuation == nil ||
		secondReview.Plan.Continuation.SourceOperationID != firstResult.Record.ID ||
		secondReview.Plan.Continuation.SourcePlanID != firstFinal.ID {
		t.Fatalf("second continuation review = %#v", secondReview.Plan)
	}
	secondFinal, err := plan.BuildExecutableContinuation(secondReview.Plan)
	if err != nil {
		t.Fatal(err)
	}
	runner.failTool = ""
	clock = secondFinal.CreatedAt.Add(time.Minute)
	secondResult, err := service.ExecuteContinuation(t.Context(), secondFinal, execution.ConfirmationReceipt{
		Scope: "plan", ConfirmedPlanID: secondFinal.ID, ConfirmedAt: secondFinal.CreatedAt,
	})
	if err != nil || secondResult.Record.State != execution.StateCompleted ||
		!slices.Equal(stepActionIDs(secondResult.Record.Steps),
			[]string{"update-corepack", "update-pnpm"}) ||
		secondResult.Record.ConfirmedPlan == nil ||
		secondResult.Record.ConfirmedPlan.ID != secondFinal.ID {
		t.Fatalf("second continuation result = %#v, %v", secondResult, err)
	}
	if secondResult.Outcome == nil ||
		!slices.Equal(outcomeVerified(secondResult.Outcome.Actions), []bool{true, true}) {
		t.Fatalf("second continuation outcome = %#v", secondResult.Outcome)
	}
	finalHistory := historySnapshot(t, service.HistoryRoot)
	assertHistoryPrefixUnchanged(t, lineageBefore, finalHistory)
	if len(finalHistory) != len(lineageBefore)+2 {
		t.Fatalf("lineage entries = %d, want %d", len(finalHistory), len(lineageBefore)+2)
	}
}

func TestExecuteContinuationPreservesTerminalRecordWhenPostScanFails(t *testing.T) {
	requirePOSIXFixture(t)
	service, source, prepared := failedNodeToolsSource(t, "corepack")
	clock := source.UpdatedAt.Add(time.Minute)
	service.Now = func() time.Time { return clock }
	runner := newContinuationExecutionRunner(t, prepared)
	service.Runner = runner
	reviewed, err := service.PrepareContinuation(t.Context(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	finalPlan, err := plan.BuildExecutableContinuation(reviewed.Plan)
	if err != nil {
		t.Fatal(err)
	}
	scan := service.Scan
	scans := 0
	service.Scan = func(ctx context.Context) (inventory.Inventory, error) {
		scans++
		if scans == 2 {
			return inventory.Inventory{}, errors.New("post-execution fixture scan failed")
		}
		return scan(ctx)
	}
	clock = finalPlan.CreatedAt.Add(time.Minute)

	result, err := service.ExecuteContinuation(t.Context(), finalPlan, execution.ConfirmationReceipt{
		Scope: "plan", ConfirmedPlanID: finalPlan.ID, ConfirmedAt: finalPlan.CreatedAt,
	})
	if err == nil || !strings.Contains(err.Error(), "scan after Node tools continuation execution") {
		t.Fatalf("post-scan error = %v", err)
	}
	if scans != 2 || result.Record.ID == "" ||
		result.Record.State != execution.StateCompleted ||
		result.RecordPath == "" || result.Outcome != nil {
		t.Fatalf("post-scan result = %#v, scans=%d", result, scans)
	}
	loaded, loadErr := (execution.FileStore{Root: service.HistoryRoot}).Load(result.Record.ID)
	if loadErr != nil || loaded.State != execution.StateCompleted ||
		loaded.PlanID != finalPlan.ID {
		t.Fatalf("persisted terminal record = %#v, %v", loaded, loadErr)
	}
}

func TestExecuteContinuationPreservesExecutionAndPostScanErrors(t *testing.T) {
	requirePOSIXFixture(t)
	service, source, prepared := failedNodeToolsSource(t, "npm")
	clock := source.UpdatedAt.Add(time.Minute)
	service.Now = func() time.Time { return clock }
	runner := newContinuationExecutionRunner(t, prepared)
	runner.failTool = "corepack"
	service.Runner = runner
	reviewed, err := service.PrepareContinuation(t.Context(), source.ID)
	if err != nil {
		t.Fatal(err)
	}
	finalPlan, err := plan.BuildExecutableContinuation(reviewed.Plan)
	if err != nil {
		t.Fatal(err)
	}
	scan := service.Scan
	scans := 0
	service.Scan = func(ctx context.Context) (inventory.Inventory, error) {
		scans++
		if scans == 2 {
			return inventory.Inventory{}, errors.New("post-execution fixture scan failed")
		}
		return scan(ctx)
	}
	clock = finalPlan.CreatedAt.Add(time.Minute)

	result, err := service.ExecuteContinuation(t.Context(), finalPlan, execution.ConfirmationReceipt{
		Scope: "plan", ConfirmedPlanID: finalPlan.ID, ConfirmedAt: finalPlan.CreatedAt,
	})
	if err == nil ||
		!strings.Contains(err.Error(), "injected continuation fixture failure") ||
		!strings.Contains(err.Error(), "scan after Node tools continuation execution") {
		t.Fatalf("combined error = %v", err)
	}
	if result.Record.State != execution.StateFailed || result.Outcome != nil ||
		!slices.Equal(stepStates(result.Record.Steps), []execution.State{
			execution.StateCompleted,
			execution.StateFailed,
			execution.StatePending,
		}) {
		t.Fatalf("combined failure result = %#v", result)
	}
}

func failedNodeToolsSource(t *testing.T, failure string) (Service, execution.Record, Prepared) {
	t.Helper()
	service, _, _ := nodeToolsServiceFixture(t)
	service.Runner = corepackVersionRunner("10.0.0")
	options := Options{
		NodeVersion: "24.12.0", NPMVersion: "11.6.2",
		CorepackVersion: "0.34.5", PNPMVersion: "11.1.0",
	}
	switch failure {
	case "npm":
		options.NPMVersion = "12.0.1"
		options.CorepackVersion = "0.35.0"
	case "corepack":
		options.CorepackVersion = "0.35.0"
	case "pnpm":
	default:
		t.Fatalf("unsupported failure fixture %q", failure)
	}
	prepared, err := service.Prepare(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	calls := []execution.CommandSpec{}
	failureRunner := nodeToolsFailureRunner(prepared)
	service.Runner = functionRunner(func(spec execution.CommandSpec) execution.ProcessResult {
		calls = append(calls, spec)
		return failureRunner.Run(t.Context(), spec)
	})
	result, err := service.Execute(t.Context(), prepared, execution.ConfirmationReceipt{
		Scope: "plan", ConfirmedPlanID: prepared.Plan.ID, ConfirmedAt: prepared.Plan.CreatedAt,
	})
	if err == nil || result.Record.State != execution.StateFailed {
		t.Fatalf("failed source = %#v, %v", result.Record, err)
	}
	expectedCompleted := map[string]int{"npm": 0, "corepack": 1, "pnpm": 2}[failure]
	for index := 0; index < expectedCompleted; index++ {
		if result.Record.Steps[index].State != execution.StateCompleted {
			t.Fatalf("source failed before %s: steps=%#v calls=%#v", failure, result.Record.Steps, calls)
		}
	}
	return service, result.Record, prepared
}

func changedUpstreamNodeToolsSource(
	t *testing.T,
) (Service, execution.Record, Prepared, *continuationExecutionRunner) {
	t.Helper()
	service, _, _ := nodeToolsServiceFixture(t)
	service.Runner = corepackVersionRunner("10.0.0")
	prepared, err := service.Prepare(t.Context(), Options{
		NodeVersion: "24.12.0", NPMVersion: "12.0.1",
		CorepackVersion: "0.35.0", PNPMVersion: "11.1.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := newContinuationExecutionRunner(t, prepared)
	runner.failTool = "corepack"
	service.Runner = runner
	result, err := service.Execute(t.Context(), prepared, execution.ConfirmationReceipt{
		Scope: "plan", ConfirmedPlanID: prepared.Plan.ID, ConfirmedAt: prepared.Plan.CreatedAt,
	})
	if err == nil || result.Record.State != execution.StateFailed ||
		!slices.Equal(stepStates(result.Record.Steps), []execution.State{
			execution.StateCompleted, execution.StateFailed, execution.StatePending,
		}) ||
		len(result.Record.Steps[0].Diff) == 0 {
		t.Fatalf("changed-upstream source = %#v, %v", result.Record, err)
	}
	return service, result.Record, prepared, runner
}

func completedChangedNodeToolsSource(
	t *testing.T,
) (Service, execution.Record, Prepared, *continuationExecutionRunner) {
	t.Helper()
	service, _, _ := nodeToolsServiceFixture(t)
	service.Runner = corepackVersionRunner("10.0.0")
	prepared, err := service.Prepare(t.Context(), Options{
		NodeVersion: "24.12.0", NPMVersion: "12.0.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := newContinuationExecutionRunner(t, prepared)
	service.Runner = runner
	result, err := service.Execute(t.Context(), prepared, execution.ConfirmationReceipt{
		Scope: "plan", ConfirmedPlanID: prepared.Plan.ID, ConfirmedAt: prepared.Plan.CreatedAt,
	})
	if err != nil || result.Record.State != execution.StateCompleted ||
		len(result.Record.Steps) != 1 || len(result.Record.Steps[0].Diff) == 0 {
		t.Fatalf("completed changed source = %#v, %v", result.Record, err)
	}
	return service, result.Record, prepared, runner
}

func completedNodeToolsSource(t *testing.T) (Service, execution.Record) {
	t.Helper()
	service, _, _ := nodeToolsServiceFixture(t)
	prepared, err := service.Prepare(t.Context(), Options{
		NodeVersion: "24.12.0", NPMVersion: "11.6.2",
	})
	if err != nil {
		t.Fatal(err)
	}
	writeCalls := 0
	service.Runner = continuationProbeRunner(prepared, &writeCalls)
	result, err := service.Execute(t.Context(), prepared, execution.ConfirmationReceipt{
		Scope: "plan", ConfirmedPlanID: prepared.Plan.ID, ConfirmedAt: prepared.Plan.CreatedAt,
	})
	if err != nil || result.Record.State != execution.StateCompleted || writeCalls != 0 {
		t.Fatalf("completed source = %#v, %v, writes=%d", result.Record, err, writeCalls)
	}
	return service, result.Record
}

func repeatedNodeToolsSource(
	t *testing.T,
	failureIndex int,
) (Service, execution.Record, Prepared) {
	t.Helper()
	service, _, _ := nodeToolsServiceFixture(t)
	service.Runner = corepackVersionRunner("10.0.0")
	targets := Options{
		NodeVersion: "24.12.0", NPMVersion: "11.6.2",
		CorepackVersion: "0.34.5", PNPMVersion: "10.0.0",
	}
	prepared, err := service.Prepare(t.Context(), targets)
	if err != nil {
		t.Fatal(err)
	}
	writeCalls := 0
	service.Runner = continuationProbeRunner(prepared, &writeCalls)
	completed, err := service.Execute(t.Context(), prepared, execution.ConfirmationReceipt{
		Scope: "plan", ConfirmedPlanID: prepared.Plan.ID, ConfirmedAt: prepared.Plan.CreatedAt,
	})
	if err != nil || completed.Record.State != execution.StateCompleted || writeCalls != 0 {
		t.Fatalf("completed lineage fixture = %#v, %v, writes=%d", completed.Record, err, writeCalls)
	}

	first := failedRecordFromCompleted(
		t,
		completed.Record,
		prepared.Plan,
		0,
		"op-dddddddddddddddddddddddddddddddd",
	)
	if err := (execution.FileStore{Root: service.HistoryRoot}).Save(first); err != nil {
		t.Fatal(err)
	}

	service.Now = func() time.Time { return first.UpdatedAt.Add(time.Minute) }
	service.Runner = corepackVersionRunner("10.0.0")
	fresh, err := service.Prepare(t.Context(), targets)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := plan.BuildContinuationDraft(plan.ContinuationDraftInput{
		SourceOperationID: first.ID,
		SourcePlanID:      first.PlanID,
		SourceActionIDs:   actionIDs(first.ConfirmedPlan.Actions),
		PreparedPlan:      fresh.Plan,
	})
	if err != nil {
		t.Fatal(err)
	}
	executable, err := plan.BuildExecutableContinuation(draft)
	if err != nil {
		t.Fatal(err)
	}
	record := failedRecordFromCompleted(
		t,
		completed.Record,
		executable,
		failureIndex,
		[]string{
			"op-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"op-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			"op-cccccccccccccccccccccccccccccccc",
		}[failureIndex],
	)
	if err := (execution.FileStore{Root: service.HistoryRoot}).Save(record); err != nil {
		t.Fatal(err)
	}
	return service, record, fresh
}

func failedRecordFromCompleted(
	t *testing.T,
	completed execution.Record,
	confirmed plan.Plan,
	failureIndex int,
	operationID string,
) execution.Record {
	t.Helper()
	data, err := execution.MarshalRecord(completed)
	if err != nil {
		t.Fatal(err)
	}
	value, err := execution.DecodeRecord(data)
	if err != nil {
		t.Fatal(err)
	}
	value.ID = operationID
	value.PlanID = confirmed.ID
	value.PlanSchemaVersion = confirmed.SchemaVersion
	value.ConfirmedPlan = &confirmed
	value.Confirmation = execution.ConfirmationReceipt{
		Scope: "plan", ConfirmedPlanID: confirmed.ID, ConfirmedAt: confirmed.CreatedAt,
	}
	value.CreatedAt = confirmed.CreatedAt.Add(time.Minute)
	value.StartedAt = timePointer(value.CreatedAt)
	value.UpdatedAt = value.CreatedAt.Add(time.Second)
	value.FinishedAt = timePointer(value.UpdatedAt)
	value.State = execution.StateFailed
	value.Transitions = []execution.Transition{
		{State: execution.StatePending, At: value.CreatedAt, Reason: "test continuation accepted"},
		{
			State: execution.StateFailed, At: value.UpdatedAt,
			ActionID: value.Steps[failureIndex].ActionID, Reason: "test continuation failed",
		},
	}
	for index := range value.Steps {
		step := &value.Steps[index]
		switch {
		case index < failureIndex:
			step.StartedAt = timePointer(value.CreatedAt)
			step.FinishedAt = timePointer(value.CreatedAt)
		case index == failureIndex:
			step.State = execution.StateFailed
			step.StartedAt = timePointer(value.CreatedAt)
			step.FinishedAt = timePointer(value.UpdatedAt)
			step.Invocation = nil
			step.ExitCode = nil
			step.Error = &execution.ErrorDetail{
				Code: execution.CodeExitNonZero, Message: "test continuation failure",
			}
			step.Verification = execution.CheckResult{State: execution.CheckNotRun}
			step.After = nil
			step.Diff = nil
			step.Skipped = false
		default:
			step.State = execution.StatePending
			step.StartedAt = nil
			step.FinishedAt = nil
			step.Invocation = nil
			step.ExitCode = nil
			step.Error = nil
			step.Precondition = execution.CheckResult{State: execution.CheckPending}
			step.Verification = execution.CheckResult{State: execution.CheckPending}
			step.Before = nil
			step.After = nil
			step.Diff = nil
			step.Skipped = false
		}
	}
	data, err = execution.MarshalRecord(value)
	if err != nil {
		t.Fatal(err)
	}
	value, err = execution.DecodeRecord(data)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func timePointer(value time.Time) *time.Time {
	return &value
}

type continuationExecutionRunner struct {
	t               *testing.T
	prepared        Prepared
	npmVersion      string
	corepackVersion string
	pnpmVersion     string
	failTool        string
	writeCalls      int
}

func newContinuationExecutionRunner(
	t *testing.T,
	prepared Prepared,
) *continuationExecutionRunner {
	t.Helper()
	return &continuationExecutionRunner{
		t: t, prepared: prepared,
		npmVersion:      prepared.baseline.NPM.Version,
		corepackVersion: prepared.baseline.Corepack.Version,
		pnpmVersion:     "10.0.0",
	}
}

func (runner *continuationExecutionRunner) Run(
	_ context.Context,
	spec execution.CommandSpec,
) execution.ProcessResult {
	code := 0
	if len(spec.Args) == 1 && spec.Args[0] == "--version" {
		version := ""
		switch spec.Executable {
		case runner.prepared.baseline.NPM.Executable:
			version = runner.npmVersion
		case runner.prepared.baseline.Corepack.Executable:
			version = runner.corepackVersion
		case runner.prepared.baseline.PNPM.Executable:
			version = runner.pnpmVersion
		case runner.prepared.baseline.NodeBinary:
			version = "v" + runner.prepared.baseline.NodeVersion
		default:
			if filepath.Base(spec.Executable) == "node" {
				version = "v" + runner.prepared.baseline.NodeVersion
			}
		}
		if version == "" {
			return execution.ProcessResult{
				Failure: &execution.ExecutionError{
					Code: execution.CodeStartFailed, Message: "unknown read-only continuation fixture call",
				},
			}
		}
		return execution.ProcessResult{
			ExitCode: &code, Stdout: execution.CapturedOutput{Text: version + "\n"},
		}
	}
	if len(spec.Args) == 0 {
		return execution.ProcessResult{
			Failure: &execution.ExecutionError{
				Code: execution.CodeStartFailed, Message: "missing continuation fixture target",
			},
		}
	}
	name, version, ok := strings.Cut(spec.Args[len(spec.Args)-1], "@")
	if !ok || (name != "npm" && name != "corepack" && name != "pnpm") {
		return execution.ProcessResult{
			Failure: &execution.ExecutionError{
				Code: execution.CodeStartFailed, Message: "unsupported continuation fixture target",
			},
		}
	}
	runner.writeCalls++
	if name == runner.failTool {
		exitCode := 7
		return execution.ProcessResult{
			ExitCode: &exitCode,
			Failure: &execution.ExecutionError{
				Code: execution.CodeExitNonZero, Message: "injected continuation fixture failure",
			},
		}
	}
	switch name {
	case "npm":
		runner.npmVersion = version
		runner.writePackageVersion(runner.prepared.baseline.NPM.PackageRoot, name, version)
	case "corepack":
		runner.corepackVersion = version
		runner.writePackageVersion(runner.prepared.baseline.Corepack.PackageRoot, name, version)
	case "pnpm":
		runner.pnpmVersion = version
	}
	return execution.ProcessResult{ExitCode: &code}
}

func (runner *continuationExecutionRunner) writePackageVersion(root, name, version string) {
	runner.t.Helper()
	if err := os.WriteFile(
		filepath.Join(root, "package.json"),
		[]byte(`{"name":"`+name+`","version":"`+version+`"}`),
		0o644,
	); err != nil {
		runner.t.Fatal(err)
	}
}

func stepActionIDs(steps []execution.StepRecord) []string {
	result := make([]string, 0, len(steps))
	for _, step := range steps {
		result = append(result, step.ActionID)
	}
	return result
}

func stepStates(steps []execution.StepRecord) []execution.State {
	result := make([]execution.State, 0, len(steps))
	for _, step := range steps {
		result = append(result, step.State)
	}
	return result
}

func outcomeActionIDs(actions []ContinuationActionOutcome) []string {
	result := make([]string, 0, len(actions))
	for _, action := range actions {
		result = append(result, action.ActionID)
	}
	return result
}

func outcomeVerified(actions []ContinuationActionOutcome) []bool {
	result := make([]bool, 0, len(actions))
	for _, action := range actions {
		result = append(result, action.Verified)
	}
	return result
}

func outcomeStates(actions []ContinuationActionOutcome) []execution.State {
	result := make([]execution.State, 0, len(actions))
	for _, action := range actions {
		result = append(result, action.State)
	}
	return result
}

func assertHistoryPrefixUnchanged(
	t *testing.T,
	before, after map[string][]byte,
) {
	t.Helper()
	for name, data := range before {
		if !bytes.Equal(data, after[name]) {
			t.Fatalf("history entry %q changed", name)
		}
	}
}

func failedSelfTestSource(t *testing.T, root string) execution.Record {
	t.Helper()
	createdAt := time.Date(2026, 7, 28, 8, 0, 0, 0, time.UTC)
	value, err := plan.BuildSelfTest(plan.SelfTestInput{
		CreatedAt: createdAt, OS: "darwin", OSVersion: "15.0", Architecture: "arm64",
	})
	if err != nil {
		t.Fatal(err)
	}
	definition := execution.SelfTestDefinition(filepath.Join(t.TempDir(), "envmason-self-test"))
	registry, err := execution.NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	runner := functionRunner(func(execution.CommandSpec) execution.ProcessResult {
		exitCode := 1
		return execution.ProcessResult{
			ExitCode: &exitCode,
			Failure: &execution.ExecutionError{
				Code: execution.CodeExitNonZero, Message: "fixture failure",
			},
		}
	})
	executor := execution.Executor{
		Registry: registry, Runner: runner, Store: execution.FileStore{Root: root},
		Now: func() time.Time { return createdAt.Add(time.Minute) },
		NewOperationID: func() (string, error) {
			return "op-99999999999999999999999999999999", nil
		},
	}
	record, err := executor.Execute(t.Context(), execution.Request{
		Plan: value,
		Confirmation: execution.ConfirmationReceipt{
			Scope: "plan", ConfirmedPlanID: value.ID, ConfirmedAt: value.CreatedAt,
		},
	})
	if err == nil || record.State != execution.StateFailed {
		t.Fatalf("self-test source = %#v, %v", record, err)
	}
	return record
}

func nodeToolsFailureRunner(prepared Prepared) execution.ProcessRunner {
	return functionRunner(func(spec execution.CommandSpec) execution.ProcessResult {
		if len(spec.Args) == 1 && spec.Args[0] == "--version" {
			return versionResult(prepared, spec.Executable)
		}
		exitCode := 7
		return execution.ProcessResult{
			ExitCode: &exitCode,
			Failure: &execution.ExecutionError{
				Code: execution.CodeExitNonZero, Message: "fixture failure",
			},
		}
	})
}

func continuationProbeRunner(prepared Prepared, writeCalls *int) execution.ProcessRunner {
	return functionRunner(func(spec execution.CommandSpec) execution.ProcessResult {
		if len(spec.Args) == 1 && spec.Args[0] == "--version" {
			return versionResult(prepared, spec.Executable)
		}
		*writeCalls++
		return execution.ProcessResult{
			Failure: &execution.ExecutionError{
				Code: execution.CodeStartFailed, Message: "write-shaped fixture call",
			},
		}
	})
}

func versionResult(prepared Prepared, executable string) execution.ProcessResult {
	version := ""
	switch executable {
	case prepared.baseline.NPM.Executable:
		version = prepared.baseline.NPM.Version
	case prepared.baseline.Corepack.Executable:
		version = prepared.baseline.Corepack.Version
	case prepared.baseline.PNPM.Executable:
		version = "10.0.0"
	case prepared.baseline.NodeBinary:
		version = "v" + prepared.baseline.NodeVersion
	}
	if version == "" && filepath.Base(executable) == "node" {
		version = "v" + prepared.baseline.NodeVersion
	}
	if version == "" {
		return execution.ProcessResult{
			Failure: &execution.ExecutionError{
				Code: execution.CodeStartFailed, Message: "unknown read-only fixture call",
			},
		}
	}
	exitCode := 0
	return execution.ProcessResult{
		ExitCode: &exitCode, Stdout: execution.CapturedOutput{Text: version + "\n"},
	}
}

func assertContinuationRejectedBeforeScan(t *testing.T, service Service, operationID, want string) {
	t.Helper()
	scans := 0
	service.Scan = func(context.Context) (inventory.Inventory, error) {
		scans++
		return inventory.Inventory{}, errors.New("unexpected scan")
	}
	_, err := service.PrepareContinuation(t.Context(), operationID)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want %q", err, want)
	}
	if scans != 0 {
		t.Fatal("unsupported continuation source reached environment scan")
	}
}

func actionIDs(actions []plan.Action) []string {
	result := make([]string, 0, len(actions))
	for _, action := range actions {
		result = append(result, action.ID)
	}
	return result
}

func checkpointIDs(checkpoints []plan.CheckpointBinding) []string {
	result := make([]string, 0, len(checkpoints))
	for _, checkpoint := range checkpoints {
		result = append(result, checkpoint.ActionID)
	}
	return result
}

func historySnapshot(t *testing.T, root string) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	result := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("unexpected history directory %q", entry.Name())
		}
		data, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		result[entry.Name()] = data
	}
	return result
}

func historyEqual(left, right map[string][]byte) bool {
	if len(left) != len(right) {
		return false
	}
	for name, data := range left {
		if !bytes.Equal(data, right[name]) {
			return false
		}
	}
	return true
}
