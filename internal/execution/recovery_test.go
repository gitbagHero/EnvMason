package execution

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/plan"
)

func TestAssessRecoveryClassifiesChangedAndUncertainActionsWithoutMutation(t *testing.T) {
	t.Parallel()
	source, _, _, _ := checkpointSource(t, "update-corepack", "", "", "", false)
	setStepChange(t, &source.Steps[0], "secret-npm-version", "11.6.2", "12.0.1")
	setStepChange(t, &source.Steps[1], "secret-corepack-version", "0.34.5", "0.35.0")
	before, err := MarshalRecord(source)
	if err != nil {
		t.Fatal(err)
	}

	assessment, err := AssessRecovery(source)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.SourceOperationID != source.ID ||
		assessment.SourcePlanID != source.PlanID ||
		len(assessment.Candidates) != 2 ||
		assessment.Candidates[0].ActionID != "update-npm" ||
		assessment.Candidates[0].Evidence != RecoveryEvidenceChanged ||
		assessment.Candidates[0].RecoveryMode != "manual" ||
		assessment.Candidates[1].ActionID != "update-corepack" ||
		assessment.Candidates[1].Evidence != RecoveryEvidenceChanged ||
		assessment.Candidates[1].StepState != StateFailed {
		t.Fatalf("recovery assessment = %#v", assessment)
	}
	after, err := MarshalRecord(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("recovery assessment mutated the source record")
	}
	data, err := json.Marshal(assessment)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{
		"secret-npm-version",
		"secret-corepack-version",
		"/secret/invocation",
	} {
		if bytes.Contains(data, []byte(secret)) {
			t.Fatalf("recovery assessment leaked %q: %s", secret, data)
		}
	}
}

func TestAssessRecoveryDistinguishesUncertainFromUnchangedAndUnstarted(t *testing.T) {
	t.Parallel()
	source, _, _, _ := checkpointSource(t, "update-corepack", "", "", "", false)
	source.Steps[0].Skipped = true
	source.Steps[0].Diff = nil
	source.Steps[1].After = nil
	source.Steps[1].Diff = nil
	source.Steps[1].Invocation = &Invocation{
		Executable: "/secret/invocation", Args: []string{"secret-argument"},
	}
	source.Steps[1].Error = &ErrorDetail{
		Code: CodeExitNonZero, Message: "secret process failure",
	}
	setStepChange(t, &source.Steps[2], "pending-secret", "before", "after")

	assessment, err := AssessRecovery(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(assessment.Candidates) != 1 ||
		assessment.Candidates[0].ActionID != "update-corepack" ||
		assessment.Candidates[0].Evidence != RecoveryEvidenceUncertain ||
		assessment.Candidates[0].RecoveryMode != "manual" {
		t.Fatalf("uncertain recovery assessment = %#v", assessment)
	}

	for _, code := range []Code{CodePreconditionFailed, CodeStartFailed} {
		candidate := cloneRecord(t, source)
		candidate.Steps[1].Error.Code = code
		value, err := AssessRecovery(candidate)
		if err != nil {
			t.Fatal(err)
		}
		if len(value.Candidates) != 0 {
			t.Fatalf("%s recovery candidates = %#v", code, value.Candidates)
		}
	}
}

func TestAssessRecoverySupportsEveryPartialTerminalAndCompletedChange(t *testing.T) {
	t.Parallel()
	tests := []struct {
		state State
		code  Code
	}{
		{state: StateFailed, code: CodeExitNonZero},
		{state: StateTimedOut, code: CodeTimeout},
		{state: StateCancelled, code: CodeCancelled},
		{state: StateInterrupted, code: CodeInterrupted},
	}
	for _, test := range tests {
		test := test
		t.Run(string(test.state), func(t *testing.T) {
			t.Parallel()
			source, _, _, _ := checkpointSource(t, "update-npm", "", "", "", false)
			source.State = test.state
			source.Steps[0].State = test.state
			source.Steps[0].After = nil
			source.Steps[0].Diff = nil
			source.Steps[0].Error.Code = test.code
			source.Transitions[len(source.Transitions)-1].State = test.state
			assessment, err := AssessRecovery(source)
			if err != nil {
				t.Fatal(err)
			}
			if len(assessment.Candidates) != 1 ||
				assessment.Candidates[0].Evidence != RecoveryEvidenceUncertain {
				t.Fatalf("%s assessment = %#v", test.state, assessment)
			}
		})
	}

	t.Run("completed changed", func(t *testing.T) {
		t.Parallel()
		source, _, _, _ := checkpointSource(t, "", "", "", "", false)
		setStepChange(t, &source.Steps[0], "version", "11.6.2", "12.0.1")
		assessment, err := AssessRecovery(source)
		if err != nil {
			t.Fatal(err)
		}
		if len(assessment.Candidates) != 1 ||
			assessment.Candidates[0].Evidence != RecoveryEvidenceChanged ||
			assessment.Candidates[0].StepState != StateCompleted {
			t.Fatalf("completed assessment = %#v", assessment)
		}
	})
}

func TestAssessRecoveryPreservesPlanRequiredClassification(t *testing.T) {
	t.Parallel()
	source := defaultSetRecoveryRecord(t)
	assessment, err := AssessRecovery(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(assessment.Candidates) != 1 ||
		assessment.Candidates[0].ActionID != "set-node-default" ||
		assessment.Candidates[0].RecoveryMode != "plan" ||
		assessment.Candidates[0].Evidence != RecoveryEvidenceChanged ||
		!strings.Contains(assessment.Candidates[0].RecoverySummary, "new R3 recovery Plan") {
		t.Fatalf("Plan recovery assessment = %#v", assessment)
	}
}

func TestAssessRecoveryRejectsOldActiveAndTamperedRecords(t *testing.T) {
	t.Parallel()
	source, _, _, _ := checkpointSource(t, "update-npm", "", "", "", false)

	t.Run("old records", func(t *testing.T) {
		for _, version := range []string{OlderRecordSchemaVersion, LegacyRecordSchemaVersion} {
			candidate := cloneRecord(t, source)
			candidate.SchemaVersion = version
			candidate.ConfirmedPlan = nil
			if version == LegacyRecordSchemaVersion {
				for index := range candidate.Steps {
					candidate.Steps[index].Before = nil
					candidate.Steps[index].After = nil
					candidate.Steps[index].Diff = nil
					candidate.Steps[index].Skipped = false
				}
			}
			data, err := MarshalRecord(candidate)
			if err != nil {
				t.Fatal(err)
			}
			candidate, err = DecodeRecord(data)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := AssessRecovery(candidate); err == nil ||
				!strings.Contains(err.Error(), "no confirmed Plan provenance") {
				t.Fatalf("%s error = %v", version, err)
			}
		}
	})

	t.Run("active", func(t *testing.T) {
		executor, request, store, _, _ := checkpointExecutor(
			t, "update-corepack", "", "", "", false,
		)
		if _, err := executor.Execute(t.Context(), request); err == nil {
			t.Fatal("injected execution unexpectedly succeeded")
		}
		var active Record
		for _, value := range store.records {
			if value.State == StateRunning || value.State == StateVerifying {
				active = value
				break
			}
		}
		if _, err := AssessRecovery(active); err == nil ||
			!strings.Contains(err.Error(), "must be terminal") {
			t.Fatalf("active error = %v", err)
		}
	})

	t.Run("terminal record with active step", func(t *testing.T) {
		candidate := cloneRecord(t, source)
		candidate.Steps[0].State = StateRunning
		candidate.Steps[0].FinishedAt = nil
		candidate.Steps[0].Error = nil
		if _, err := AssessRecovery(candidate); err == nil ||
			!strings.Contains(err.Error(), "contains an active step") {
			t.Fatalf("active step error = %v", err)
		}
	})

	t.Run("tampered", func(t *testing.T) {
		candidate := cloneRecord(t, source)
		candidate.ConfirmedPlan.Actions[0].TargetVersion = "99.0.0"
		if _, err := AssessRecovery(candidate); err == nil ||
			!strings.Contains(err.Error(), "record is invalid") {
			t.Fatalf("tampered error = %v", err)
		}
	})
}

func setStepChange(
	t *testing.T,
	step *StepRecord,
	key, beforeValue, afterValue string,
) {
	t.Helper()
	before, err := NewSnapshot(map[string]string{key: beforeValue})
	if err != nil {
		t.Fatal(err)
	}
	after, err := NewSnapshot(map[string]string{key: afterValue})
	if err != nil {
		t.Fatal(err)
	}
	step.Before = &before
	step.After = &after
	step.Diff = DiffSnapshots(before, after)
}

func defaultSetRecoveryRecord(t *testing.T) Record {
	t.Helper()
	createdAt := testBaseTime
	value, err := plan.BuildDefaultSet(plan.DefaultSetInput{
		Inventory: inventory.Inventory{
			SchemaVersion: inventory.SchemaVersion,
			GeneratedAt:   createdAt,
			System: inventory.System{
				OS: inventory.OSMacOS, OSVersion: "26.0",
				Architecture: inventory.ArchitectureARM64,
			},
			Tools: []inventory.Tool{{
				ID: "runtime.node", DisplayName: "Node.js",
				Category: inventory.CategoryRuntime,
				Installations: []inventory.Installation{
					{
						ID: "node-nvm-22", Version: "v22.0.0",
						Path:    "$NVM_DIR/versions/node/v22.0.0/bin/node",
						Manager: "nvm", Architecture: inventory.ArchitectureARM64,
						ActiveState:  inventory.ActiveStateActive,
						DefaultState: inventory.DefaultStateDefault,
					},
					{
						ID: "node-nvm-24", Version: "v24.14.0",
						Path:    "$NVM_DIR/versions/node/v24.14.0/bin/node",
						Manager: "nvm", Architecture: inventory.ArchitectureARM64,
						ActiveState:  inventory.ActiveStateInactive,
						DefaultState: inventory.DefaultStateNonDefault,
					},
				},
			}},
		},
		CreatedAt: createdAt, TargetVersion: "24.14.0",
		ScriptDigest:       "sha256:" + strings.Repeat("a", 64),
		CurrentAliasDigest: aliasDigestForTest("22"),
		CurrentAlias:       "22", CurrentDefaultVersion: "v22.0.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	startedAt := createdAt.Add(time.Minute)
	finishedAt := startedAt.Add(time.Second)
	before, err := NewSnapshot(map[string]string{"default_alias": "22"})
	if err != nil {
		t.Fatal(err)
	}
	after, err := NewSnapshot(map[string]string{"default_alias": "v24.14.0"})
	if err != nil {
		t.Fatal(err)
	}
	action := value.Actions[0]
	record := Record{
		SchemaVersion: RecordSchemaVersion,
		ID:            "op-ffffffffffffffffffffffffffffffff",
		PlanID:        value.ID, PlanSchemaVersion: value.SchemaVersion,
		ConfirmedPlan: &value, State: StateCompleted,
		CreatedAt: startedAt, UpdatedAt: finishedAt,
		StartedAt: &startedAt, FinishedAt: &finishedAt,
		Confirmation: ConfirmationReceipt{
			Scope: "plan", ConfirmedPlanID: value.ID, ConfirmedAt: value.CreatedAt,
		},
		Steps: []StepRecord{{
			ActionID: action.ID, ToolID: action.ToolID,
			Operation: action.Operation, Adapter: action.Adapter, Risk: action.Risk,
			State: StateCompleted, StartedAt: &startedAt, FinishedAt: &finishedAt,
			Verification: CheckResult{State: CheckPassed},
			Precondition: CheckResult{State: CheckPassed},
			Stdout:       CapturedOutput{}, Stderr: CapturedOutput{},
			Before: &before, After: &after, Diff: DiffSnapshots(before, after),
		}},
		Transitions: []Transition{
			{State: StatePending, At: startedAt, Reason: "confirmed"},
			{State: StateRunning, At: startedAt, ActionID: action.ID, Reason: "started"},
			{State: StateVerifying, At: finishedAt, ActionID: action.ID, Reason: "verifying"},
			{State: StateCompleted, At: finishedAt, ActionID: action.ID, Reason: "completed"},
		},
	}
	data, err := MarshalRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	record, err = DecodeRecord(data)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func aliasDigestForTest(value string) string {
	digest := sha256.Sum256([]byte(value + "\n"))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func TestRecoveryEvidenceValuesAreStable(t *testing.T) {
	t.Parallel()
	if !slices.Equal(
		[]RecoveryEvidence{RecoveryEvidenceChanged, RecoveryEvidenceUncertain},
		[]RecoveryEvidence{"changed", "uncertain"},
	) {
		t.Fatal("recovery evidence values changed")
	}
}
