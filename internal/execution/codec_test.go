package execution

import (
	"bytes"
	"testing"
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
	unknown := bytes.Replace(data, []byte(`"schema_version": "0.2.0"`), []byte(`"schema_version": "0.2.0", "unknown": true`), 1)
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

func TestOperationCodecRetainsVersionZeroOneReadCompatibility(t *testing.T) {
	t.Parallel()
	executor, request, _, _ := testHarness(t, nil)
	record, err := executor.Execute(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	record.SchemaVersion = PreviousRecordSchemaVersion
	data, err := MarshalRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRecord(data)
	if err != nil || decoded.SchemaVersion != PreviousRecordSchemaVersion {
		t.Fatalf("decode 0.1.0 = %#v, %v", decoded, err)
	}
}

func TestRedactorCoversExactValuesAndCommonAssignments(t *testing.T) {
	t.Parallel()
	redactor := NewRedactor("long-secret-value", "secret")
	actual := redactor.String("token=long-secret-value password: secret Authorization=abc")
	if actual != "token=[REDACTED] password: [REDACTED] Authorization=[REDACTED]" {
		t.Fatalf("redacted = %q", actual)
	}
}
