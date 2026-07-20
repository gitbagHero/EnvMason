package plan

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gitbagHero/EnvMason/internal/inventory"
)

func TestBuildDefaultSetCreatesSingleExplicitR3Plan(t *testing.T) {
	input := defaultSetFixture()
	value, err := BuildDefaultSet(input)
	if err != nil {
		t.Fatal(err)
	}
	if value.SchemaVersion != HighRiskExecutableSchemaVersion || !value.Executable || len(value.Actions) != 1 || value.Actions[0].Risk != RiskR3 || value.Actions[0].Recovery.Mode != "plan" {
		t.Fatalf("default set Plan = %#v", value)
	}
	data, err := Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"operation": "set_default"`, `"risk": "R3"`, `"expected": "22"`, `"expected": "v24.14.0"`} {
		if !bytes.Contains(data, []byte(expected)) {
			t.Errorf("Plan JSON missing %q:\n%s", expected, data)
		}
	}
	summary, err := Render(value, FormatSummary)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`from "22"`, `to "v24.14.0"`, "risk=R3", "explicit confirmation"} {
		if !strings.Contains(string(summary), expected) {
			t.Errorf("summary missing %q:\n%s", expected, summary)
		}
	}
}

func TestBuildDefaultSetRejectsMissingTargetAndNonCanonicalAlias(t *testing.T) {
	input := defaultSetFixture()
	input.Inventory.Tools[0].Installations = input.Inventory.Tools[0].Installations[:1]
	if _, err := BuildDefaultSet(input); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("missing target error = %v", err)
	}
	input = defaultSetFixture()
	input.CurrentAlias = "22\n"
	if _, err := BuildDefaultSet(input); err == nil || !strings.Contains(err.Error(), "canonical") {
		t.Fatalf("non-canonical alias error = %v", err)
	}
}

func TestBuildDefaultRestoreBindsSourceAndOriginalAlias(t *testing.T) {
	set := defaultSetFixture()
	restore, err := BuildDefaultRestore(DefaultRestoreInput{
		Inventory: set.Inventory, CreatedAt: set.CreatedAt, ScriptDigest: set.ScriptDigest,
		CurrentAliasDigest: digestAliasValue("v24.14.0"), CurrentAlias: "v24.14.0", CurrentDefaultVersion: "v24.14.0",
		OriginalAliasDigest: digestAliasValue("22"), OriginalAlias: "22", OriginalDefaultVersion: "v22.0.0",
		SourceOperationID: "op-00000000000000000000000000000001", SourcePlanID: "sha256:" + strings.Repeat("d", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	if restore.Actions[0].Operation != "restore_default" || restore.Actions[0].TargetVersion != "22.0.0" || restore.Actions[0].Recovery.Mode != "manual" {
		t.Fatalf("restore Plan = %#v", restore)
	}
	data, _ := Marshal(restore)
	for _, expected := range []string{"op-00000000000000000000000000000001", `"expected": "22"`, digestAliasValue("22")} {
		if !bytes.Contains(data, []byte(expected)) {
			t.Errorf("restore Plan missing %q:\n%s", expected, data)
		}
	}

	mutated := clonePlan(restore)
	mutated.Actions = append(mutated.Actions, mutated.Actions[0])
	mutated.Actions[1].ID = "another-default-action"
	mutated.ID, _ = planID(mutated)
	if err := Validate(mutated); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("multi-action Plan 0.3 validation = %v", err)
	}
}

func TestPlanZeroThreeRejectsRiskDownloadDependencyAndRecoveryDowngrades(t *testing.T) {
	base, err := BuildDefaultSet(defaultSetFixture())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		edit func(*Plan)
		want string
	}{
		{name: "risk", edit: func(value *Plan) { value.Actions[0].Risk = RiskR2 }, want: "requires R3"},
		{name: "download", edit: func(value *Plan) { value.Actions[0].Download = Download{State: "unknown"} }, want: "cannot download"},
		{name: "dependency", edit: func(value *Plan) { value.Actions[0].Dependencies = []string{"other"} }, want: "cannot download"},
		{name: "recovery", edit: func(value *Plan) { value.Actions[0].Recovery.Mode = "manual" }, want: "recovery Plan"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := clonePlan(base)
			test.edit(&value)
			value.ID, _ = planID(value)
			if err := Validate(value); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate = %v, want %q", err, test.want)
			}
		})
	}
}

func defaultSetFixture() DefaultSetInput {
	base := buildFixture()
	base.Inventory.Tools[0].Installations = append(base.Inventory.Tools[0].Installations, inventory.Installation{
		ID: "node-nvm-24", Version: "v24.14.0", Path: "$HOME/.nvm/versions/node/v24.14.0/bin/node", Manager: "nvm",
		ActiveState: inventory.ActiveStateInactive, DefaultState: inventory.DefaultStateNonDefault,
	})
	return DefaultSetInput{
		Inventory: base.Inventory, CreatedAt: base.CreatedAt, TargetVersion: "24.14.0",
		ScriptDigest: "sha256:" + strings.Repeat("a", 64), CurrentAliasDigest: digestAliasValue("22"),
		CurrentAlias: "22", CurrentDefaultVersion: "v22.0.0",
	}
}
