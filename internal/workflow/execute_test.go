package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/apply"
	"github.com/gitbagHero/EnvMason/internal/defaultversion"
	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/nodetools"
	"github.com/gitbagHero/EnvMason/internal/plan"
)

type recordingStageExecutor struct {
	t             *testing.T
	terminal      execution.State
	noRecordStage string
	calls         []string
	receipts      []execution.ConfirmationReceipt
}

func (value *recordingStageExecutor) ExecuteInstallNode(
	ctx context.Context,
	prepared apply.Prepared,
	receipt execution.ConfirmationReceipt,
) (execution.Record, error) {
	return value.execute(ctx, StageInstallNode, prepared.Plan, receipt)
}

func (value *recordingStageExecutor) ExecuteSetDefault(
	ctx context.Context,
	prepared defaultversion.Prepared,
	receipt execution.ConfirmationReceipt,
) (execution.Record, error) {
	return value.execute(ctx, StageSetDefault, prepared.Plan, receipt)
}

func (value *recordingStageExecutor) ExecuteNodeTools(
	ctx context.Context,
	prepared nodetools.Prepared,
	receipt execution.ConfirmationReceipt,
) (execution.Record, error) {
	return value.execute(ctx, StageUpdateNodeTools, prepared.Plan, receipt)
}

func (value *recordingStageExecutor) execute(
	ctx context.Context,
	stageID string,
	childPlan plan.Plan,
	receipt execution.ConfirmationReceipt,
) (execution.Record, error) {
	value.calls = append(value.calls, stageID)
	value.receipts = append(value.receipts, receipt)
	if value.noRecordStage == stageID {
		return execution.Record{}, errors.New("simulated environment drift")
	}
	terminal := value.terminal
	if terminal == "" {
		terminal = execution.StateCompleted
	}
	return workflowOperation(
		value.t,
		ctx,
		childPlan,
		receipt,
		terminal,
		operationID(byte('7'+stageIndex(stageID))),
	)
}

func TestExecutePreparedCompletesThreeIndependentlyConfirmedStages(t *testing.T) {
	manifest := mustManifest(t)
	record := mustNewRecord(t, manifest)
	executor := &recordingStageExecutor{t: t}
	planIDs := []string{}

	for index := range manifest.Stages {
		prepared := prepareExecutableStage(t, manifest, record, index)
		originalPrepared := clonePreparedStage(prepared)
		receipt := execution.ConfirmationReceipt{
			Scope:           "plan",
			ConfirmedPlanID: prepared.Plan.ID,
			ConfirmedAt:     prepared.Plan.CreatedAt.Add(30 * time.Second),
		}
		executed, err := ExecutePrepared(
			t.Context(),
			manifest,
			prepared,
			receipt,
			executor,
		)
		if err != nil {
			t.Fatalf("ExecutePrepared(%d) error = %v", index, err)
		}
		if !reflect.DeepEqual(prepared, originalPrepared) {
			t.Fatalf("ExecutePrepared(%d) mutated PreparedStage", index)
		}
		if executed.StageID != manifest.Stages[index].ID ||
			executed.Operation.State != execution.StateCompleted ||
			executed.Record.Stages[index].State != StageCompleted ||
			executed.Record.Stages[index].OperationID != executed.Operation.ID {
			t.Fatalf("executed stage %d = %#v", index, executed)
		}
		assertOperationDigest(t, executed)
		planIDs = append(planIDs, prepared.Plan.ID)
		record = executed.Record
	}
	if record.State != WorkflowCompleted || record.FinishedAt == nil {
		t.Fatalf("final Workflow Record = %#v", record)
	}
	if len(executor.calls) != 3 ||
		!reflect.DeepEqual(
			executor.calls,
			[]string{StageInstallNode, StageSetDefault, StageUpdateNodeTools},
		) {
		t.Fatalf("execution calls = %v", executor.calls)
	}
	if planIDs[0] == planIDs[1] ||
		planIDs[0] == planIDs[2] ||
		planIDs[1] == planIDs[2] {
		t.Fatalf("stage Plan IDs are not independent: %v", planIDs)
	}
	for index, receipt := range executor.receipts {
		if receipt.ConfirmedPlanID != planIDs[index] {
			t.Fatalf("receipt %d bound %q, want %q", index, receipt.ConfirmedPlanID, planIDs[index])
		}
	}
}

func TestExecutePreparedStopsEveryStageForEveryAbnormalTerminal(t *testing.T) {
	terminals := []execution.State{
		execution.StateFailed,
		execution.StateTimedOut,
		execution.StateCancelled,
		execution.StateInterrupted,
	}
	for stageIndex := 0; stageIndex < 3; stageIndex++ {
		for _, terminal := range terminals {
			t.Run(manifestStageName(stageIndex)+"/"+string(terminal), func(t *testing.T) {
				manifest := mustManifest(t)
				record := executeUntilReady(t, manifest, stageIndex)
				prepared := prepareExecutableStage(t, manifest, record, stageIndex)
				executor := &recordingStageExecutor{
					t: t, terminal: terminal,
				}
				executed, err := ExecutePrepared(
					t.Context(),
					manifest,
					prepared,
					execution.ConfirmationReceipt{
						Scope:           "plan",
						ConfirmedPlanID: prepared.Plan.ID,
						ConfirmedAt:     prepared.Plan.CreatedAt.Add(30 * time.Second),
					},
					executor,
				)
				if err == nil {
					t.Fatal("abnormal terminal returned no execution error")
				}
				expected, _ := workflowTerminalState(terminal)
				if executed.Record.Stages[stageIndex].State != expected ||
					executed.Record.State != workflowStateForTerminal(expected) ||
					executed.Record.FinishedAt == nil {
					t.Fatalf("abnormal executed stage = %#v", executed)
				}
				for following := stageIndex + 1; following < 3; following++ {
					if executed.Record.Stages[following].State != StagePending {
						t.Fatalf("following stage %d = %q", following, executed.Record.Stages[following].State)
					}
				}
				assertOperationDigest(t, executed)
				calls := len(executor.calls)
				if _, err := PrepareNext(
					t.Context(),
					manifest,
					executed.Record,
					&recordingStagePreparer{},
				); err == nil {
					t.Fatal("abnormal Workflow allowed another preparation")
				}
				if len(executor.calls) != calls {
					t.Fatal("abnormal Workflow reached another executor")
				}
			})
		}
	}
}

func TestExecutePreparedRejectsWrongOrReusedConfirmationBeforeExecutor(t *testing.T) {
	manifest := mustManifest(t)
	record := mustNewRecord(t, manifest)
	prepared := prepareExecutableStage(t, manifest, record, 0)
	executor := &recordingStageExecutor{t: t}
	tests := []execution.ConfirmationReceipt{
		{
			Scope: "item", ConfirmedPlanID: prepared.Plan.ID,
			ConfirmedAt: prepared.Plan.CreatedAt,
		},
		{
			Scope: "plan", ConfirmedPlanID: digestID('f'),
			ConfirmedAt: prepared.Plan.CreatedAt,
		},
		{
			Scope: "plan", ConfirmedPlanID: prepared.Plan.ID,
			ConfirmedAt: prepared.Plan.CreatedAt.Add(-time.Nanosecond),
		},
	}
	for _, receipt := range tests {
		result, err := ExecutePrepared(
			t.Context(),
			manifest,
			prepared,
			receipt,
			executor,
		)
		if err == nil {
			t.Fatal("invalid confirmation was accepted")
		}
		if !reflect.DeepEqual(result.Record, prepared.Record) {
			t.Fatal("invalid confirmation changed the Workflow Record")
		}
	}
	if len(executor.calls) != 0 {
		t.Fatalf("invalid confirmation reached executor: %v", executor.calls)
	}

	validReceipt := execution.ConfirmationReceipt{
		Scope: "plan", ConfirmedPlanID: prepared.Plan.ID,
		ConfirmedAt: prepared.Plan.CreatedAt.Add(30 * time.Second),
	}
	first, err := ExecutePrepared(
		t.Context(),
		manifest,
		prepared,
		validReceipt,
		executor,
	)
	if err != nil {
		t.Fatal(err)
	}
	next := prepareExecutableStage(t, manifest, first.Record, 1)
	callCount := len(executor.calls)
	if _, err := ExecutePrepared(
		t.Context(),
		manifest,
		next,
		validReceipt,
		executor,
	); err == nil {
		t.Fatal("previous stage confirmation was reused")
	}
	if len(executor.calls) != callCount {
		t.Fatal("reused confirmation reached the next stage executor")
	}
}

func TestExecutePreparedKeepsPlannedStateWhenNoOperationWasCreated(t *testing.T) {
	for stageIndex := 0; stageIndex < 3; stageIndex++ {
		t.Run(manifestStageName(stageIndex), func(t *testing.T) {
			manifest := mustManifest(t)
			record := executeUntilReady(t, manifest, stageIndex)
			prepared := prepareExecutableStage(
				t,
				manifest,
				record,
				stageIndex,
			)
			original := clonePreparedStage(prepared)
			executor := &recordingStageExecutor{
				t: t, noRecordStage: manifest.Stages[stageIndex].ID,
			}
			result, err := ExecutePrepared(
				t.Context(),
				manifest,
				prepared,
				execution.ConfirmationReceipt{
					Scope: "plan", ConfirmedPlanID: prepared.Plan.ID,
					ConfirmedAt: prepared.Plan.CreatedAt.Add(30 * time.Second),
				},
				executor,
			)
			if err == nil || !strings.Contains(err.Error(), "environment drift") {
				t.Fatalf("drift error = %v", err)
			}
			if !reflect.DeepEqual(result.Record, prepared.Record) ||
				result.Record.Stages[stageIndex].State != StagePlanned ||
				result.Operation.ID != "" ||
				!reflect.DeepEqual(prepared, original) {
				t.Fatalf("pre-Operation rejection changed state: %#v", result)
			}
		})
	}
}

func TestExecutePreparedRejectsChangedPublicPlanOrNativeContext(t *testing.T) {
	manifest := mustManifest(t)
	record := mustNewRecord(t, manifest)
	prepared := prepareExecutableStage(t, manifest, record, 0)
	executor := &recordingStageExecutor{t: t}
	receipt := execution.ConfirmationReceipt{
		Scope: "plan", ConfirmedPlanID: prepared.Plan.ID,
		ConfirmedAt: prepared.Plan.CreatedAt.Add(30 * time.Second),
	}

	changedPublic := clonePreparedStage(prepared)
	changedPublic.Plan.Actions[0].TargetVersion = "24.15.0"
	if _, err := ExecutePrepared(
		t.Context(),
		manifest,
		changedPublic,
		receipt,
		executor,
	); err == nil {
		t.Fatal("changed public child Plan was accepted")
	}

	changedNative := clonePreparedStage(prepared)
	changedNative.native.install.Plan = workflowInstallPlan(
		t,
		prepared.Plan.CreatedAt,
		"24.15.0",
	)
	if _, err := ExecutePrepared(
		t.Context(),
		manifest,
		changedNative,
		receipt,
		executor,
	); err == nil {
		t.Fatal("changed native Prepared context was accepted")
	}
	if len(executor.calls) != 0 {
		t.Fatalf("changed Prepared reached executor: %v", executor.calls)
	}
}

func TestAttachOperationRejectsInvalidActiveOrWrongPlanRecords(t *testing.T) {
	manifest := mustManifest(t)
	record := mustNewRecord(t, manifest)
	prepared := prepareExecutableStage(t, manifest, record, 0)
	receipt := execution.ConfirmationReceipt{
		Scope: "plan", ConfirmedPlanID: prepared.Plan.ID,
		ConfirmedAt: prepared.Plan.CreatedAt.Add(30 * time.Second),
	}
	completed, err := workflowOperation(
		t,
		t.Context(),
		prepared.Plan,
		receipt,
		execution.StateCompleted,
		operationID('5'),
	)
	if err != nil {
		t.Fatal(err)
	}
	before, err := execution.MarshalRecord(completed)
	if err != nil {
		t.Fatal(err)
	}
	attached, err := AttachOperation(manifest, prepared, completed)
	if err != nil {
		t.Fatal(err)
	}
	after, err := execution.MarshalRecord(completed)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("AttachOperation mutated its Operation Record input")
	}
	attached.Operation.Steps[0].ActionID = "changed"
	if completed.Steps[0].ActionID == "changed" {
		t.Fatal("returned Operation shares state with its input")
	}

	active := activeWorkflowOperation(
		t,
		t.Context(),
		prepared.Plan,
		receipt,
		operationID('6'),
	)
	if _, err := AttachOperation(manifest, prepared, active); err == nil {
		t.Fatal("active Operation Record was attached")
	}

	otherPlan := workflowInstallPlan(
		t,
		prepared.Plan.CreatedAt,
		"24.15.0",
	)
	otherReceipt := execution.ConfirmationReceipt{
		Scope: "plan", ConfirmedPlanID: otherPlan.ID,
		ConfirmedAt: otherPlan.CreatedAt.Add(30 * time.Second),
	}
	other, err := workflowOperation(
		t,
		t.Context(),
		otherPlan,
		otherReceipt,
		execution.StateCompleted,
		operationID('7'),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AttachOperation(manifest, prepared, other); err == nil {
		t.Fatal("Operation Record for another Plan was attached")
	}

	tampered := completed
	tampered.PlanSchemaVersion = plan.HighRiskExecutableSchemaVersion
	if _, err := AttachOperation(manifest, prepared, tampered); err == nil {
		t.Fatal("tampered Operation Record was attached")
	}
}

func prepareExecutableStage(
	t *testing.T,
	manifest Manifest,
	record Record,
	index int,
) PreparedStage {
	t.Helper()
	createdAt := record.UpdatedAt.Add(time.Minute)
	var child ChildPlan
	switch index {
	case 0:
		child.Plan = workflowInstallPlan(
			t,
			createdAt,
			manifest.TargetNodeVersion,
		)
		sealed := apply.Prepared{Plan: clonePlan(child.Plan)}
		child.native = &nativePrepared{install: &sealed}
	case 1:
		child.Plan = workflowDefaultPlan(
			t,
			createdAt,
			manifest.TargetNodeVersion,
		)
		sealed := defaultversion.Prepared{Plan: clonePlan(child.Plan)}
		child.native = &nativePrepared{setDefault: &sealed}
	case 2:
		child.Plan = workflowNodeToolsPlan(
			t,
			createdAt,
			manifest.TargetNodeVersion,
			manifest.NodeTools,
		)
		sealed := nodetools.Prepared{Plan: clonePlan(child.Plan)}
		child.native = &nativePrepared{nodeTools: &sealed}
	default:
		t.Fatalf("unsupported stage index %d", index)
	}
	preparer := &recordingStagePreparer{}
	switch index {
	case 0:
		preparer.installPlan = child
	case 1:
		preparer.defaultPlan = child
	case 2:
		preparer.toolsPlan = child
	}
	prepared, err := PrepareNext(
		t.Context(),
		manifest,
		record,
		preparer,
	)
	if err != nil {
		t.Fatalf("PrepareNext(%d) error = %v", index, err)
	}
	return prepared
}

func executeUntilReady(
	t *testing.T,
	manifest Manifest,
	target int,
) Record {
	t.Helper()
	record := mustNewRecord(t, manifest)
	for index := 0; index < target; index++ {
		prepared := prepareExecutableStage(t, manifest, record, index)
		executed, err := ExecutePrepared(
			t.Context(),
			manifest,
			prepared,
			execution.ConfirmationReceipt{
				Scope: "plan", ConfirmedPlanID: prepared.Plan.ID,
				ConfirmedAt: prepared.Plan.CreatedAt.Add(30 * time.Second),
			},
			&recordingStageExecutor{t: t},
		)
		if err != nil {
			t.Fatalf("ExecutePrepared(%d) error = %v", index, err)
		}
		record = executed.Record
	}
	return record
}

func workflowOperation(
	t *testing.T,
	ctx context.Context,
	childPlan plan.Plan,
	receipt execution.ConfirmationReceipt,
	terminal execution.State,
	operationID string,
) (execution.Record, error) {
	t.Helper()
	harness := newWorkflowExecutionHarness(t, childPlan, terminal, operationID)
	record, err := harness.executor.Execute(ctx, execution.Request{
		Plan: childPlan, Confirmation: receipt,
	})
	if terminal != execution.StateInterrupted {
		return record, err
	}
	active := harness.store.active(t)
	interrupted, recoverErr := execution.RecoverInterrupted(
		active,
		active.UpdatedAt.Add(time.Minute),
	)
	if recoverErr != nil {
		t.Fatalf("RecoverInterrupted() error = %v", recoverErr)
	}
	return interrupted, errors.New("simulated interruption")
}

func activeWorkflowOperation(
	t *testing.T,
	ctx context.Context,
	childPlan plan.Plan,
	receipt execution.ConfirmationReceipt,
	operationID string,
) execution.Record {
	t.Helper()
	harness := newWorkflowExecutionHarness(
		t,
		childPlan,
		execution.StateCompleted,
		operationID,
	)
	if _, err := harness.executor.Execute(ctx, execution.Request{
		Plan: childPlan, Confirmation: receipt,
	}); err != nil {
		t.Fatalf("execute operation fixture: %v", err)
	}
	return harness.store.active(t)
}

type workflowExecutionHarness struct {
	executor execution.Executor
	store    *workflowRecordStore
}

func newWorkflowExecutionHarness(
	t *testing.T,
	childPlan plan.Plan,
	terminal execution.State,
	operationID string,
) workflowExecutionHarness {
	t.Helper()
	definitions := make([]execution.Definition, 0, len(childPlan.Actions))
	for _, action := range childPlan.Actions {
		action := action
		definitions = append(definitions, execution.Definition{
			Key: execution.ActionKey{
				ToolID: action.ToolID, Operation: action.Operation,
				Adapter: action.Adapter,
			},
			MinimumRisk: action.Risk,
			Build: func(plan.Action) (execution.CommandSpec, error) {
				return execution.CommandSpec{
					Executable: "/usr/bin/true",
					Timeout:    time.Minute,
				}, nil
			},
			Capture: func(context.Context, plan.Action) (execution.Snapshot, error) {
				return execution.NewSnapshot(map[string]string{
					"action": action.ID,
				})
			},
			Verify: func(
				context.Context,
				plan.Action,
				execution.ProcessResult,
			) error {
				return nil
			},
		})
	}
	registry, err := execution.NewRegistry(definitions...)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	runnerResult := execution.ProcessResult{}
	code := 0
	runnerResult.ExitCode = &code
	switch terminal {
	case execution.StateFailed:
		runnerResult.Failure = &execution.ExecutionError{
			Code: execution.CodeExitNonZero, Message: "simulated failure",
		}
	case execution.StateTimedOut:
		runnerResult.Failure = &execution.ExecutionError{
			Code: execution.CodeTimeout, Message: "simulated timeout",
		}
	case execution.StateCancelled:
		runnerResult.Failure = &execution.ExecutionError{
			Code: execution.CodeCancelled, Message: "simulated cancellation",
		}
	}
	store := &workflowRecordStore{}
	clock := childPlan.CreatedAt.Add(time.Minute)
	return workflowExecutionHarness{
		executor: execution.Executor{
			Registry: registry,
			Runner:   fixedWorkflowRunner{result: runnerResult},
			Store:    store,
			Now: func() time.Time {
				current := clock
				clock = clock.Add(time.Second)
				return current
			},
			NewOperationID: func() (string, error) {
				return operationID, nil
			},
		},
		store: store,
	}
}

type fixedWorkflowRunner struct {
	result execution.ProcessResult
}

func (value fixedWorkflowRunner) Run(
	context.Context,
	execution.CommandSpec,
) execution.ProcessResult {
	return value.result
}

type workflowRecordStore struct {
	records []execution.Record
}

func (value *workflowRecordStore) Save(record execution.Record) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	var copied execution.Record
	if err := json.Unmarshal(data, &copied); err != nil {
		return err
	}
	value.records = append(value.records, copied)
	return nil
}

func (value *workflowRecordStore) active(t *testing.T) execution.Record {
	t.Helper()
	for _, record := range value.records {
		for _, step := range record.Steps {
			if step.State == execution.StateRunning ||
				step.State == execution.StateVerifying {
				return record
			}
		}
	}
	t.Fatal("operation fixture has no active record")
	return execution.Record{}
}

func assertOperationDigest(t *testing.T, executed ExecutedStage) {
	t.Helper()
	data, err := execution.MarshalRecord(executed.Operation)
	if err != nil {
		t.Fatalf("MarshalRecord() error = %v", err)
	}
	digest := sha256.Sum256(data)
	expected := "sha256:" + hex.EncodeToString(digest[:])
	index := stageIndex(executed.StageID)
	if executed.Record.Stages[index].CheckpointDigest != expected {
		t.Fatalf(
			"checkpoint = %q, want %q",
			executed.Record.Stages[index].CheckpointDigest,
			expected,
		)
	}
}

func clonePreparedStage(value PreparedStage) PreparedStage {
	result := value
	result.Plan = clonePlan(value.Plan)
	result.Record = cloneRecord(value.Record)
	if value.native == nil {
		return result
	}
	result.native = &nativePrepared{}
	if value.native.install != nil {
		copied := *value.native.install
		copied.Plan = clonePlan(copied.Plan)
		result.native.install = &copied
	}
	if value.native.setDefault != nil {
		copied := *value.native.setDefault
		copied.Plan = clonePlan(copied.Plan)
		result.native.setDefault = &copied
	}
	if value.native.nodeTools != nil {
		copied := *value.native.nodeTools
		copied.Plan = clonePlan(copied.Plan)
		result.native.nodeTools = &copied
	}
	return result
}
