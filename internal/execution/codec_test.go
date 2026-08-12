package execution

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gitbagHero/EnvMason/internal/plan"
)

func TestOperationCodecRejectsUnknownFieldsTrailingJSONAndFalseCompletion(t *testing.T) {
	t.Parallel()
	executor, request, _, _ := testHarness(t, nil)
	record, err := executor.Execute(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	data, err := MarshalRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	unknown := bytes.Replace(data, []byte(`"schema_version": "0.4.0"`), []byte(`"schema_version": "0.4.0", "unknown": true`), 1)
	if _, err := DecodeRecord(unknown); err == nil {
		t.Fatal("unknown field was accepted")
	}
	if _, err := DecodeRecord(append(data, []byte("{}")...)); err == nil {
		t.Fatal("trailing JSON was accepted")
	}
	falseCompletion := record
	falseCompletion.Steps = append([]StepRecord{}, record.Steps...)
	falseCompletion.Steps[0].Verification = CheckResult{State: CheckFailed}
	if _, err := MarshalRecord(falseCompletion); err == nil {
		t.Fatal("false completed record was accepted")
	}
	illegalTransition := record
	illegalTransition.Transitions = append([]Transition{}, record.Transitions...)
	illegalTransition.Transitions = append(illegalTransition.Transitions, Transition{State: StateRunning, At: record.UpdatedAt, Reason: "forged restart"})
	illegalTransition.State = StateRunning
	illegalTransition.FinishedAt = nil
	if _, err := MarshalRecord(illegalTransition); err == nil {
		t.Fatal("transition out of terminal Completed was accepted")
	}
}

func TestOperationCodecBindsConfirmedPlanToRecordAndSteps(t *testing.T) {
	t.Parallel()
	executor, request, _, _ := dagTestHarness(t, "", "")
	record, err := executor.Execute(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if record.ConfirmedPlan == nil || record.ConfirmedPlan.ID != record.PlanID {
		t.Fatalf("confirmed Plan = %#v", record.ConfirmedPlan)
	}
	tests := []struct {
		name   string
		mutate func(*Record)
	}{
		{name: "missing Plan", mutate: func(value *Record) { value.ConfirmedPlan = nil }},
		{name: "Plan schema mismatch", mutate: func(value *Record) { value.PlanSchemaVersion = plan.HighRiskExecutableSchemaVersion }},
		{name: "confirmation before Plan", mutate: func(value *Record) {
			value.Confirmation.ConfirmedAt = value.ConfirmedPlan.CreatedAt.Add(-1)
		}},
		{name: "step identity", mutate: func(value *Record) { value.Steps[0].ToolID = "ecosystem.changed" }},
		{name: "step order", mutate: func(value *Record) { value.Steps[0], value.Steps[1] = value.Steps[1], value.Steps[0] }},
		{name: "target version", mutate: func(value *Record) { value.ConfirmedPlan.Actions[0].TargetVersion = "12.0.2" }},
		{name: "dependency", mutate: func(value *Record) { value.ConfirmedPlan.Actions[1].Dependencies = nil }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			mutated := cloneRecord(t, record)
			test.mutate(&mutated)
			if _, err := MarshalRecord(mutated); err == nil {
				t.Fatal("mutated confirmed Plan record was accepted")
			}
		})
	}
}

func TestOperationCodecRetainsPriorReadCompatibility(t *testing.T) {
	t.Parallel()
	executor, request, _, _ := testHarness(t, nil)
	record, err := executor.Execute(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{
		PreviousRecordSchemaVersion,
		OlderRecordSchemaVersion,
		LegacyRecordSchemaVersion,
	} {
		version := version
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			compatible := cloneRecord(t, record)
			compatible.SchemaVersion = version
			if version == OlderRecordSchemaVersion || version == LegacyRecordSchemaVersion {
				compatible.ConfirmedPlan = nil
			}
			if version == LegacyRecordSchemaVersion {
				for index := range compatible.Steps {
					compatible.Steps[index].Before = nil
					compatible.Steps[index].After = nil
					compatible.Steps[index].Diff = nil
					compatible.Steps[index].Skipped = false
				}
			}
			data, err := MarshalRecord(compatible)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := DecodeRecord(data)
			wantConfirmed := version == PreviousRecordSchemaVersion
			if err != nil || decoded.SchemaVersion != version ||
				(decoded.ConfirmedPlan != nil) != wantConfirmed {
				t.Fatalf("decode %s = %#v, %v", version, decoded, err)
			}
		})
	}
}

func TestOperationCodecAcceptsAndBindsExecutableContinuationPlanOnlyInRecord04(t *testing.T) {
	t.Parallel()
	record, _ := executableContinuationSource(t, "update-corepack")
	data, err := MarshalRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRecord(data)
	if err != nil || decoded.SchemaVersion != RecordSchemaVersion ||
		decoded.ConfirmedPlan == nil ||
		decoded.ConfirmedPlan.SchemaVersion != plan.ExecutableContinuationSchemaVersion ||
		decoded.ConfirmedPlan.ID != decoded.PlanID {
		t.Fatalf("Record 0.4 executable continuation = %#v, %v", decoded, err)
	}

	tests := []struct {
		name   string
		mutate func(*Record)
	}{
		{
			name:   "Record 0.3 cannot contain Plan 0.5",
			mutate: func(value *Record) { value.SchemaVersion = PreviousRecordSchemaVersion },
		},
		{
			name: "confirmed Plan ID",
			mutate: func(value *Record) {
				value.ConfirmedPlan.ID = "sha256:" + strings.Repeat("f", 64)
			},
		},
		{
			name: "confirmed Plan target",
			mutate: func(value *Record) {
				value.ConfirmedPlan.Actions[0].TargetVersion = "99.0.0"
			},
		},
		{
			name: "confirmed Plan dependency",
			mutate: func(value *Record) {
				value.ConfirmedPlan.Actions[1].Dependencies = []string{}
			},
		},
		{
			name: "confirmation identity",
			mutate: func(value *Record) {
				value.Confirmation.ConfirmedPlanID = "sha256:" + strings.Repeat("e", 64)
			},
		},
		{
			name:   "step identity",
			mutate: func(value *Record) { value.Steps[0].Adapter = "changed" },
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate, err := DecodeRecord(data)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(&candidate)
			if _, err := MarshalRecord(candidate); err == nil {
				t.Fatal("mutated executable continuation record was accepted")
			}
		})
	}
}

func cloneRecord(t *testing.T, value Record) Record {
	t.Helper()
	data, err := MarshalRecord(value)
	if err != nil {
		t.Fatal(err)
	}
	result, err := DecodeRecord(data)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestRedactorCoversExactValuesAndCommonAssignments(t *testing.T) {
	t.Parallel()
	redactor := NewRedactor("long-secret-value", "secret")
	actual := redactor.String("token=long-secret-value password: secret Authorization=abc")
	if actual != "token=[REDACTED] password: [REDACTED] Authorization=[REDACTED]" {
		t.Fatalf("redacted = %q", actual)
	}
}
