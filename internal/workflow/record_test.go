package workflow

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRecordCompletesFixedThreeStageWorkflow(t *testing.T) {
	manifest := mustManifest(t)
	record := mustNewRecord(t, manifest)
	originalManifest := cloneManifest(manifest)

	for index, stage := range manifest.Stages {
		before := cloneRecord(record)
		at := record.UpdatedAt.Add(time.Minute)
		planned, err := BindStagePlan(
			record,
			manifest,
			stage.ID,
			digestID(byte('1'+index)),
			at,
		)
		if err != nil {
			t.Fatalf("BindStagePlan(%s) error = %v", stage.ID, err)
		}
		if !reflect.DeepEqual(record, before) {
			t.Fatalf("BindStagePlan(%s) mutated its input", stage.ID)
		}
		if planned.Stages[index].PlanSchemaVersion != stage.PlanSchemaVersion {
			t.Fatalf("stage %s bound schema %q", stage.ID, planned.Stages[index].PlanSchemaVersion)
		}

		running, err := StartStage(
			planned,
			manifest,
			stage.ID,
			operationID(byte('1'+index)),
			at.Add(time.Minute),
		)
		if err != nil {
			t.Fatalf("StartStage(%s) error = %v", stage.ID, err)
		}
		record, err = FinishStage(
			running,
			manifest,
			stage.ID,
			StageCompleted,
			digestID(byte('a'+index)),
			at.Add(2*time.Minute),
		)
		if err != nil {
			t.Fatalf("FinishStage(%s) error = %v", stage.ID, err)
		}
		if index+1 < len(manifest.Stages) {
			if record.Stages[index+1].State != StageReady ||
				record.State != WorkflowPending ||
				record.FinishedAt != nil {
				t.Fatalf("stage %s did not ready only its successor", stage.ID)
			}
		}
	}
	if record.State != WorkflowCompleted || record.FinishedAt == nil {
		t.Fatalf("final workflow state = %q, finished_at = %v", record.State, record.FinishedAt)
	}
	if len(record.Transitions) != 12 {
		t.Fatalf("transition count = %d, want 12", len(record.Transitions))
	}
	if err := ValidateRecord(record, manifest); err != nil {
		t.Fatalf("ValidateRecord(completed) error = %v", err)
	}
	if !reflect.DeepEqual(manifest, originalManifest) {
		t.Fatal("record transitions mutated the manifest")
	}
}

func TestRecordStopsForEveryStageAndAbnormalTerminal(t *testing.T) {
	terminals := []StageState{
		StageFailed,
		StageTimedOut,
		StageCancelled,
		StageInterrupted,
	}
	for stageIndex := 0; stageIndex < 3; stageIndex++ {
		for _, terminal := range terminals {
			t.Run(manifestStageName(stageIndex)+"/"+string(terminal), func(t *testing.T) {
				manifest := mustManifest(t)
				record := runningAtStage(t, manifest, stageIndex)
				finished, err := FinishStage(
					record,
					manifest,
					manifest.Stages[stageIndex].ID,
					terminal,
					"",
					record.UpdatedAt.Add(time.Minute),
				)
				if err != nil {
					t.Fatalf("FinishStage() error = %v", err)
				}
				if finished.State != workflowStateForTerminal(terminal) ||
					finished.FinishedAt == nil {
					t.Fatalf("workflow state = %q, finish = %v", finished.State, finished.FinishedAt)
				}
				for following := stageIndex + 1; following < len(finished.Stages); following++ {
					if finished.Stages[following].State != StagePending {
						t.Fatalf("following stage %d state = %q", following, finished.Stages[following].State)
					}
				}
				if _, err := BindStagePlan(
					finished,
					manifest,
					StageUpdateNodeTools,
					digestID('f'),
					finished.UpdatedAt.Add(time.Minute),
				); err == nil {
					t.Fatal("terminal workflow accepted another Plan")
				}
			})
		}
	}
}

func TestRecordRejectsInvalidCalls(t *testing.T) {
	manifest := mustManifest(t)
	record := mustNewRecord(t, manifest)
	if _, err := NewRecord(
		manifest,
		"wf-invalid",
		manifest.CreatedAt.Add(time.Minute),
	); err == nil {
		t.Fatal("invalid Workflow ID was accepted")
	}
	if _, err := NewRecord(
		manifest,
		"wf-"+strings.Repeat("2", 32),
		manifest.CreatedAt.Add(-time.Minute),
	); err == nil {
		t.Fatal("record creation before its manifest was accepted")
	}
	if _, err := BindStagePlan(
		record,
		manifest,
		StageSetDefault,
		digestID('1'),
		record.UpdatedAt.Add(time.Minute),
	); err == nil {
		t.Fatal("later stage was planned before its dependency")
	}
	if _, err := BindStagePlan(
		record,
		manifest,
		StageInstallNode,
		"plan-invalid",
		record.UpdatedAt.Add(time.Minute),
	); err == nil {
		t.Fatal("invalid Plan ID was accepted")
	}
	if _, err := BindStagePlan(
		record,
		manifest,
		StageInstallNode,
		digestID('1'),
		record.UpdatedAt.Add(-time.Minute),
	); err == nil {
		t.Fatal("time before the current record was accepted")
	}

	planned, err := BindStagePlan(
		record,
		manifest,
		StageInstallNode,
		digestID('1'),
		record.UpdatedAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("BindStagePlan() error = %v", err)
	}
	if _, err := StartStage(
		planned,
		manifest,
		StageInstallNode,
		"operation-invalid",
		planned.UpdatedAt.Add(time.Minute),
	); err == nil {
		t.Fatal("invalid Operation ID was accepted")
	}
	running, err := StartStage(
		planned,
		manifest,
		StageInstallNode,
		operationID('1'),
		planned.UpdatedAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("StartStage() error = %v", err)
	}
	if _, err := FinishStage(
		running,
		manifest,
		StageInstallNode,
		StageCompleted,
		"",
		running.UpdatedAt.Add(time.Minute),
	); err == nil {
		t.Fatal("completed stage without checkpoint was accepted")
	}
	if _, err := FinishStage(
		running,
		manifest,
		StageInstallNode,
		StageReady,
		digestID('a'),
		running.UpdatedAt.Add(time.Minute),
	); err == nil {
		t.Fatal("non-terminal finish state was accepted")
	}
}

func TestValidateRecordRejectsTampering(t *testing.T) {
	manifest := mustManifest(t)
	record := runningAtStage(t, manifest, 0)
	tests := []struct {
		name   string
		mutate func(*Record)
	}{
		{"schema", func(value *Record) { value.SchemaVersion = "0.2.0" }},
		{"workflow id", func(value *Record) { value.ID = "wf-invalid" }},
		{"manifest id", func(value *Record) { value.ManifestID = digestID('f') }},
		{"created time", func(value *Record) { value.CreatedAt = value.CreatedAt.Add(time.Second) }},
		{"updated time", func(value *Record) { value.UpdatedAt = value.UpdatedAt.Add(time.Second) }},
		{"workflow state", func(value *Record) { value.State = WorkflowCompleted }},
		{"stage order", func(value *Record) { value.Stages[0].ID = StageSetDefault }},
		{"stage state", func(value *Record) { value.Stages[0].State = StageCompleted }},
		{"plan id", func(value *Record) { value.Stages[0].PlanID = "invalid" }},
		{"plan schema", func(value *Record) { value.Stages[0].PlanSchemaVersion = "0.3.0" }},
		{"operation id", func(value *Record) { value.Stages[0].OperationID = "invalid" }},
		{"transition stage", func(value *Record) { value.Transitions[1].StageID = StageSetDefault }},
		{"transition from", func(value *Record) { value.Transitions[1].From = StagePending }},
		{"transition workflow", func(value *Record) { value.Transitions[2].WorkflowState = WorkflowPending }},
		{"transition reason", func(value *Record) { value.Transitions[1].Reason = "changed" }},
		{"transition time", func(value *Record) { value.Transitions[2].At = value.CreatedAt.Add(-time.Second) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := cloneRecord(record)
			test.mutate(&changed)
			if err := ValidateRecord(changed, manifest); err == nil {
				t.Fatal("ValidateRecord unexpectedly succeeded")
			}
		})
	}
}

func TestValidateRecordRejectsReusedChildIdentities(t *testing.T) {
	manifest := mustManifest(t)
	record := runningAtStage(t, manifest, 1)
	reusedPlan := cloneRecord(record)
	reusedPlan.Stages[1].PlanID = reusedPlan.Stages[0].PlanID
	if err := ValidateRecord(reusedPlan, manifest); err == nil {
		t.Fatal("reused child Plan ID was accepted")
	}
	reusedOperation := cloneRecord(record)
	reusedOperation.Stages[1].OperationID =
		reusedOperation.Stages[0].OperationID
	if err := ValidateRecord(reusedOperation, manifest); err == nil {
		t.Fatal("reused child Operation ID was accepted")
	}
}

func runningAtStage(t *testing.T, manifest Manifest, target int) Record {
	t.Helper()
	record := mustNewRecord(t, manifest)
	for index := 0; index <= target; index++ {
		at := record.UpdatedAt.Add(time.Minute)
		planned, err := BindStagePlan(
			record,
			manifest,
			manifest.Stages[index].ID,
			digestID(byte('1'+index)),
			at,
		)
		if err != nil {
			t.Fatalf("BindStagePlan(%d) error = %v", index, err)
		}
		record, err = StartStage(
			planned,
			manifest,
			manifest.Stages[index].ID,
			operationID(byte('1'+index)),
			at.Add(time.Minute),
		)
		if err != nil {
			t.Fatalf("StartStage(%d) error = %v", index, err)
		}
		if index < target {
			record, err = FinishStage(
				record,
				manifest,
				manifest.Stages[index].ID,
				StageCompleted,
				digestID(byte('a'+index)),
				at.Add(2*time.Minute),
			)
			if err != nil {
				t.Fatalf("FinishStage(%d) error = %v", index, err)
			}
		}
	}
	return record
}

func mustNewRecord(t *testing.T, manifest Manifest) Record {
	t.Helper()
	value, err := NewRecord(
		manifest,
		"wf-"+strings.Repeat("1", 32),
		manifest.CreatedAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("NewRecord() error = %v", err)
	}
	return value
}

func digestID(character byte) string {
	return "sha256:" + strings.Repeat(string(character), 64)
}

func operationID(character byte) string {
	return "op-" + strings.Repeat(string(character), 32)
}

func manifestStageName(index int) string {
	return []string{StageInstallNode, StageSetDefault, StageUpdateNodeTools}[index]
}

func workflowStateForTerminal(terminal StageState) WorkflowState {
	return map[StageState]WorkflowState{
		StageFailed:      WorkflowFailed,
		StageTimedOut:    WorkflowTimedOut,
		StageCancelled:   WorkflowCancelled,
		StageInterrupted: WorkflowInterrupted,
	}[terminal]
}
