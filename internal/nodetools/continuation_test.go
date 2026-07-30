package nodetools

import (
	"bytes"
	"context"
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
		source.SchemaVersion = execution.PreviousRecordSchemaVersion
		source.ConfirmedPlan = nil
		if err := (execution.FileStore{Root: service.HistoryRoot}).Save(source); err != nil {
			t.Fatal(err)
		}
		assertContinuationRejectedBeforeScan(t, service, source.ID, "Operation Record 0.3.0")
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
