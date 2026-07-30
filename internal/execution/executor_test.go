package execution

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
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

func TestExecutorStopsDependentActionsAfterEveryDAGFailure(t *testing.T) {
	t.Parallel()
	actionIDs := []string{"update-npm", "update-corepack", "update-pnpm"}
	for _, phase := range []string{"process", "verification"} {
		for failureIndex, actionID := range actionIDs {
			phase := phase
			failureIndex := failureIndex
			actionID := actionID
			t.Run(phase+"-"+actionID, func(t *testing.T) {
				t.Parallel()
				processFailure := ""
				verificationFailure := ""
				expectedCode := CodeExitNonZero
				if phase == "process" {
					processFailure = actionID
				} else {
					verificationFailure = actionID
					expectedCode = CodeVerificationFailed
				}
				executor, request, store, runner := dagTestHarness(t, processFailure, verificationFailure)
				record, err := executor.Execute(t.Context(), request)
				assertExecutionCode(t, err, expectedCode)
				if record.State != StateFailed || len(record.Steps) != len(actionIDs) {
					t.Fatalf("record state/steps = %s/%d", record.State, len(record.Steps))
				}
				if len(runner.calls) != failureIndex+1 {
					t.Fatalf("runner calls = %#v", runner.calls)
				}
				for index, step := range record.Steps {
					if step.ActionID != actionIDs[index] {
						t.Fatalf("step order = %#v", record.Steps)
					}
					switch {
					case index < failureIndex:
						if step.State != StateCompleted || step.Verification.State != CheckPassed {
							t.Fatalf("completed prerequisite %d = %#v", index, step)
						}
					case index == failureIndex:
						expectedVerification := CheckNotRun
						if phase == "verification" {
							expectedVerification = CheckFailed
						}
						if step.State != StateFailed || step.Verification.State != expectedVerification {
							t.Fatalf("failed action %d = %#v", index, step)
						}
					default:
						if step.State != StatePending || step.StartedAt != nil || step.Invocation != nil ||
							step.Precondition.State != CheckPending || step.Verification.State != CheckNotRun {
							t.Fatalf("dependent action %d was started: %#v", index, step)
						}
					}
				}
				for index, called := range runner.calls {
					if called != actionIDs[index] {
						t.Fatalf("runner calls = %#v", runner.calls)
					}
				}
				if len(store.records) == 0 || store.records[len(store.records)-1].State != StateFailed {
					t.Fatal("terminal failure was not persisted")
				}
				if _, err := MarshalRecord(record); err != nil {
					t.Fatal(err)
				}
			})
		}
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

func dagTestHarness(t *testing.T, processFailure, verificationFailure string) (Executor, Request, *memoryStore, *actionRunner) {
	t.Helper()
	digest := "sha256:" + strings.Repeat("a", 64)
	value, err := plan.BuildNodeTools(plan.NodeToolsInput{
		CreatedAt:          testBaseTime,
		NodeVersion:        "24.12.0",
		NVMScriptDigest:    digest,
		DefaultAliasDigest: "sha256:" + strings.Repeat("b", 64),
		Inventory: inventory.Inventory{
			SchemaVersion: inventory.SchemaVersion,
			GeneratedAt:   testBaseTime.Add(-time.Minute),
			System: inventory.System{
				OS: inventory.OSMacOS, OSVersion: "26.0", Architecture: inventory.ArchitectureARM64,
			},
			Tools: []inventory.Tool{{
				ID: "runtime.node", DisplayName: "Node.js", Category: inventory.CategoryRuntime,
				Installations: []inventory.Installation{{
					ID: "node-nvm-target", Version: "v24.12.0", Path: "$HOME/.nvm/versions/node/v24.12.0/bin/node",
					Manager: "nvm", Architecture: inventory.ArchitectureARM64,
					ActiveState: inventory.ActiveStateActive, DefaultState: inventory.DefaultStateDefault,
				}},
			}},
		},
		Targets: []plan.NodeToolTarget{
			{ToolID: plan.NodeToolNPM, CurrentVersion: "11.6.2", TargetVersion: "12.0.1", Provider: plan.NodeToolProviderNPM, ControlDigest: digest},
			{ToolID: plan.NodeToolCorepack, CurrentVersion: "0.34.5", TargetVersion: "0.35.0", Provider: plan.NodeToolProviderNPM, ControlDigest: digest},
			{ToolID: plan.NodeToolPNPM, CurrentVersion: "10.0.0", TargetVersion: "11.1.0", Provider: plan.NodeToolProviderCorepack, ControlDigest: digest},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	executableRoot := t.TempDir()
	definitions := make([]Definition, 0, len(value.Actions))
	for _, action := range value.Actions {
		definitions = append(definitions, Definition{
			Key:         ActionKey{ToolID: action.ToolID, Operation: action.Operation, Adapter: action.Adapter},
			MinimumRisk: plan.RiskR2,
			Build: func(candidate plan.Action) (CommandSpec, error) {
				return CommandSpec{
					Executable: filepath.Join(executableRoot, candidate.ID),
					Args:       []string{candidate.ID},
					Timeout:    time.Second,
				}, nil
			},
			Verify: func(_ context.Context, candidate plan.Action, _ ProcessResult) error {
				if candidate.ID == verificationFailure {
					return errors.New("injected verification failure")
				}
				return nil
			},
		})
	}
	registry, err := NewRegistry(definitions...)
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryStore{}
	runner := &actionRunner{failAction: processFailure}
	executor := Executor{
		Registry: registry, Runner: runner, Store: store,
		Now:            func() time.Time { return testBaseTime.Add(time.Minute) },
		NewOperationID: func() (string, error) { return "op-00000000000000000000000000000002", nil },
	}
	request := Request{
		Plan: value,
		Confirmation: ConfirmationReceipt{
			Scope: "plan", ConfirmedPlanID: value.ID, ConfirmedAt: testBaseTime.Add(30 * time.Second),
		},
	}
	return executor, request, store, runner
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

type actionRunner struct {
	failAction string
	calls      []string
}

func (runner *actionRunner) Run(_ context.Context, spec CommandSpec) ProcessResult {
	actionID := spec.Args[0]
	runner.calls = append(runner.calls, actionID)
	exitCode := 0
	if actionID == runner.failAction {
		exitCode = 7
		return ProcessResult{
			ExitCode: &exitCode,
			Failure:  executionError(CodeExitNonZero, "injected process failure"),
		}
	}
	return ProcessResult{ExitCode: &exitCode}
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
