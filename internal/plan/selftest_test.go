package plan

import (
	"bytes"
	"testing"
	"time"
)

func TestBuildSelfTestCreatesImmutableExecutablePlan(t *testing.T) {
	t.Parallel()
	createdAt := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	value, err := BuildSelfTest(SelfTestInput{CreatedAt: createdAt, OS: "darwin", OSVersion: "26.0", Architecture: "arm64"})
	if err != nil {
		t.Fatal(err)
	}
	if value.SchemaVersion != ExecutableSchemaVersion || !value.Executable || value.Actions[0].Risk != RiskR1 {
		t.Fatalf("unexpected self-test plan: %#v", value)
	}
	if value.Actions[0].ToolID != "internal.executor" || value.Actions[0].Operation != "self_test" || value.Actions[0].Adapter != "builtin" {
		t.Fatalf("unexpected action identity: %#v", value.Actions[0])
	}
	data, err := Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, again) {
		t.Fatal("self-test plan did not round-trip deterministically")
	}
	withCommand := bytes.Replace(data, []byte(`"adapter": "builtin"`), []byte(`"adapter": "builtin", "command": "sh -c anything"`), 1)
	if err := ValidateJSON(withCommand); err == nil {
		t.Fatal("Plan 0.2.0 accepted an arbitrary command field")
	}

	tampered := decoded
	tampered.Actions[0].Risk = RiskR0
	if err := Validate(tampered); err == nil {
		t.Fatal("risk downgrade was accepted")
	}
	tampered = decoded
	tampered.Actions[0].TargetVersion = "changed"
	if err := Validate(tampered); err == nil {
		t.Fatal("Plan ID was reusable after action mutation")
	}
}

func TestBuildSelfTestRejectsIncompleteInput(t *testing.T) {
	t.Parallel()
	if _, err := BuildSelfTest(SelfTestInput{}); err == nil {
		t.Fatal("incomplete self-test input was accepted")
	}
}
