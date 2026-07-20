package execution

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/plan"
)

var testBaseTime = time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)

func TestExecutorCompletesConfirmedRegisteredPlan(t *testing.T) {
	t.Parallel()
	executor, request, store, runner := testHarness(t, nil)
	record, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if record.State != StateCompleted || record.Steps[0].State != StateCompleted || record.Steps[0].Verification.State != CheckPassed {
		t.Fatalf("record = %#v", record)
	}
	if runner.calls != 1 || len(store.records) < 5 || store.records[len(store.records)-1].State != StateCompleted {
		t.Fatalf("calls=%d records=%d", runner.calls, len(store.records))
	}
	if _, err := MarshalRecord(record); err != nil {
		t.Fatal(err)
	}
}

func TestExecutorMapsProcessFailuresWithoutFalseCompletion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		code Code
		want State
	}{
		{name: "non-zero", code: CodeExitNonZero, want: StateFailed},
		{name: "start", code: CodeStartFailed, want: StateFailed},
		{name: "abnormal", code: CodeAbnormalExit, want: StateFailed},
		{name: "timeout", code: CodeTimeout, want: StateTimedOut},
		{name: "cancelled", code: CodeCancelled, want: StateCancelled},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			exitCode := 7
			runner := &fakeRunner{result: ProcessResult{ExitCode: &exitCode, Failure: executionError(test.code, "fixed failure")}}
			executor, request, store, _ := testHarness(t, runner)
			record, err := executor.Execute(context.Background(), request)
			var executionFailure *ExecutionError
			if !errors.As(err, &executionFailure) || executionFailure.Code != test.code {
				t.Fatalf("error = %v", err)
			}
			if record.State != test.want || store.records[len(store.records)-1].State != test.want {
				t.Fatalf("states = %s/%s, want %s", record.State, store.records[len(store.records)-1].State, test.want)
			}
			if record.State == StateCompleted {
				t.Fatal("failed process was marked completed")
			}
		})
	}
}

func TestExecutorRejectsInvalidConfirmationExpiredAndMutatedPlans(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*Executor, *Request)
		code   Code
	}{
		{name: "wrong plan", code: CodeConfirmationRequired, mutate: func(_ *Executor, request *Request) {
			request.Confirmation.ConfirmedPlanID = "sha256:" + strings.Repeat("0", 64)
		}},
		{name: "before creation", code: CodeConfirmationRequired, mutate: func(_ *Executor, request *Request) { request.Confirmation.ConfirmedAt = testBaseTime.Add(-time.Second) }},
		{name: "expired", code: CodePlanExpired, mutate: func(executor *Executor, _ *Request) {
			executor.Now = func() time.Time { return testBaseTime.Add(31 * time.Minute) }
		}},
		{name: "mutated", code: CodePlanInvalid, mutate: func(_ *Executor, request *Request) {
			request.Plan.Actions = append([]plan.Action{}, request.Plan.Actions...)
			request.Plan.Actions[0].TargetVersion = "changed"
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			executor, request, store, runner := testHarness(t, nil)
			test.mutate(&executor, &request)
			_, err := executor.Execute(context.Background(), request)
			var executionFailure *ExecutionError
			if !errors.As(err, &executionFailure) || executionFailure.Code != test.code {
				t.Fatalf("error = %v, want %s", err, test.code)
			}
			if len(store.records) != 0 || runner.calls != 0 {
				t.Fatal("invalid request reached log store or process runner")
			}
		})
	}
}

func TestExecutorRejectsUnregisteredAndRiskDowngradedActions(t *testing.T) {
	t.Parallel()
	executor, request, store, runner := testHarness(t, nil)
	executor.Registry = Registry{definitions: map[string]Definition{}}
	_, err := executor.Execute(context.Background(), request)
	assertExecutionCode(t, err, CodeActionUnregistered)
	if len(store.records) != 0 || runner.calls != 0 {
		t.Fatal("unregistered action executed")
	}

	executor, request, store, runner = testHarness(t, nil)
	definition, err := executor.Registry.Resolve(request.Plan.Actions[0])
	if err != nil {
		t.Fatal(err)
	}
	definition.MinimumRisk = plan.RiskR2
	executor.Registry, err = NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	_, err = executor.Execute(context.Background(), request)
	assertExecutionCode(t, err, CodePlanInvalid)
	if len(store.records) != 0 || runner.calls != 0 {
		t.Fatal("risk-downgraded action executed")
	}
}

func TestExecutorDoesNotStartWhenAuditPersistenceFails(t *testing.T) {
	t.Parallel()
	executor, request, store, runner := testHarness(t, nil)
	store.failAt = 3
	record, err := executor.Execute(context.Background(), request)
	assertExecutionCode(t, err, CodeLogWriteFailed)
	if runner.calls != 0 {
		t.Fatal("process started after audit persistence failure")
	}
	if record.State == StateCompleted {
		t.Fatal("log failure was marked completed")
	}
}

func TestExecutorVerificationFailureIsNotCompleted(t *testing.T) {
	t.Parallel()
	executor, request, store, _ := testHarness(t, nil)
	definition, err := executor.Registry.Resolve(request.Plan.Actions[0])
	if err != nil {
		t.Fatal(err)
	}
	definition.Verify = func(context.Context, plan.Action, ProcessResult) error {
		return errors.New("secret raw verifier failure")
	}
	executor.Registry, err = NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	record, err := executor.Execute(context.Background(), request)
	assertExecutionCode(t, err, CodeVerificationFailed)
	if record.State != StateFailed || record.Steps[0].Verification.State != CheckFailed || strings.Contains(fmt.Sprint(record), "secret raw") {
		t.Fatalf("record = %#v", record)
	}
	if store.records[len(store.records)-1].State == StateCompleted {
		t.Fatal("verification failure was persisted as completed")
	}
}

func TestExecutorRedactsSecretsAndBoundsAllPersistedOutput(t *testing.T) {
	t.Parallel()
	const token = "mock-token-super-secret"
	long := strings.Repeat("x", DefaultOutputLimit+500)
	exitCode := 0
	runner := &fakeRunner{result: ProcessResult{
		ExitCode: &exitCode,
		Stdout:   CapturedOutput{Text: "token=" + token + long},
		Stderr:   CapturedOutput{Text: "Authorization: " + token},
	}}
	executor, request, store, _ := testHarness(t, runner)
	request.SensitiveValues = []string{token}
	definition, err := executor.Registry.Resolve(request.Plan.Actions[0])
	if err != nil {
		t.Fatal(err)
	}
	definition.Build = func(plan.Action) (CommandSpec, error) {
		return CommandSpec{Executable: filepath.Join(t.TempDir(), "helper-"+token), Args: []string{"--token=" + token}, Timeout: time.Second, SensitiveValues: []string{token}}, nil
	}
	executor.Registry, err = NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	record, err := executor.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	data, err := MarshalRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), token) || !strings.Contains(string(data), redactedValue) {
		t.Fatalf("redaction failed: %s", data)
	}
	if len(record.Steps[0].Stdout.Text) > DefaultOutputLimit || !record.Steps[0].Stdout.Truncated {
		t.Fatalf("stdout len/truncated = %d/%v", len(record.Steps[0].Stdout.Text), record.Steps[0].Stdout.Truncated)
	}
	for _, persisted := range store.data {
		if strings.Contains(string(persisted), token) {
			t.Fatal("mock token reached persisted record bytes")
		}
	}
}

func TestRecoverInterruptedNeverInventsCompletion(t *testing.T) {
	t.Parallel()
	executor, request, store, _ := testHarness(t, nil)
	store.failAt = 4
	record, err := executor.Execute(context.Background(), request)
	assertExecutionCode(t, err, CodeLogWriteFailed)
	active := store.records[len(store.records)-1]
	if active.State != StateRunning {
		t.Fatalf("last persisted state = %s", active.State)
	}
	recovered, err := RecoverInterrupted(active, testBaseTime.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State != StateInterrupted || recovered.Steps[0].State != StateInterrupted || recovered.State == StateCompleted {
		t.Fatalf("recovered = %#v (execute result %#v)", recovered, record)
	}
	if _, err := MarshalRecord(recovered); err != nil {
		t.Fatal(err)
	}
}

func TestExecutorCapturesStateDiffAndSkipsSatisfiedAction(t *testing.T) {
	t.Parallel()
	executor, request, _, runner := testHarness(t, nil)
	definition, err := executor.Registry.Resolve(request.Plan.Actions[0])
	if err != nil {
		t.Fatal(err)
	}
	captures := 0
	definition.Capture = func(context.Context, plan.Action) (Snapshot, error) {
		captures++
		return NewSnapshot(map[string]string{"target_installed": "true"})
	}
	definition.Satisfied = func(context.Context, plan.Action) (bool, error) { return true, nil }
	executor.Registry, err = NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	record, err := executor.Execute(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	step := record.Steps[0]
	if runner.calls != 0 || captures != 2 || !step.Skipped || step.Before == nil || step.After == nil || len(step.Diff) != 0 {
		t.Fatalf("idempotent execution = calls %d, captures %d, step %#v", runner.calls, captures, step)
	}
}

func TestExecutorRecordsDeterministicBeforeAfterDiff(t *testing.T) {
	t.Parallel()
	executor, request, _, _ := testHarness(t, nil)
	definition, err := executor.Registry.Resolve(request.Plan.Actions[0])
	if err != nil {
		t.Fatal(err)
	}
	captures := 0
	definition.Capture = func(context.Context, plan.Action) (Snapshot, error) {
		captures++
		installed := "false"
		if captures > 1 {
			installed = "true"
		}
		return NewSnapshot(map[string]string{"target_installed": installed})
	}
	executor.Registry, err = NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	record, err := executor.Execute(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	diff := record.Steps[0].Diff
	if len(diff) != 1 || diff[0] != (Change{Key: "target_installed", Kind: "changed", Before: "false", After: "true"}) {
		t.Fatalf("diff = %#v", diff)
	}
}

func testHarness(t *testing.T, suppliedRunner *fakeRunner) (Executor, Request, *memoryStore, *fakeRunner) {
	t.Helper()
	value, err := plan.BuildSelfTest(plan.SelfTestInput{CreatedAt: testBaseTime, OS: "darwin", OSVersion: "26.0", Architecture: "arm64"})
	if err != nil {
		t.Fatal(err)
	}
	if suppliedRunner == nil {
		exitCode := 0
		suppliedRunner = &fakeRunner{result: ProcessResult{ExitCode: &exitCode, Stdout: CapturedOutput{Text: "ok"}}}
	}
	executable := filepath.Join(t.TempDir(), "envmason-test")
	definition := Definition{
		Key: ActionKey{ToolID: "internal.executor", Operation: "self_test", Adapter: "builtin"}, MinimumRisk: plan.RiskR1,
		Build: func(plan.Action) (CommandSpec, error) {
			return CommandSpec{Executable: executable, Args: []string{"version"}, Timeout: time.Second}, nil
		},
		Verify: func(_ context.Context, _ plan.Action, result ProcessResult) error {
			if result.ExitCode == nil || *result.ExitCode != 0 {
				return errors.New("not zero")
			}
			return nil
		},
	}
	registry, err := NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryStore{}
	executor := Executor{
		Registry: registry, Runner: suppliedRunner, Store: store,
		Now:            func() time.Time { return testBaseTime.Add(time.Minute) },
		NewOperationID: func() (string, error) { return "op-00000000000000000000000000000001", nil },
	}
	request := Request{Plan: value, Confirmation: ConfirmationReceipt{Scope: "plan", ConfirmedPlanID: value.ID, ConfirmedAt: testBaseTime.Add(30 * time.Second)}}
	return executor, request, store, suppliedRunner
}

type fakeRunner struct {
	result ProcessResult
	calls  int
}

func (runner *fakeRunner) Run(context.Context, CommandSpec) ProcessResult {
	runner.calls++
	return runner.result
}

type memoryStore struct {
	records []Record
	data    [][]byte
	calls   int
	failAt  int
}

func (store *memoryStore) Save(record Record) error {
	store.calls++
	if store.failAt > 0 && store.calls == store.failAt {
		return errors.New("injected store failure")
	}
	data, err := MarshalRecord(record)
	if err != nil {
		return err
	}
	clone, err := DecodeRecord(data)
	if err != nil {
		return err
	}
	store.records = append(store.records, clone)
	store.data = append(store.data, data)
	return nil
}

func assertExecutionCode(t *testing.T, err error, code Code) {
	t.Helper()
	var failure *ExecutionError
	if !errors.As(err, &failure) || failure.Code != code {
		t.Fatalf("error = %v, want %s", err, code)
	}
}
