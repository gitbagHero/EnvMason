package workflow

import (
	"reflect"
	"testing"
	"time"
)

func TestBuildManifestIsDeterministicAndImmutable(t *testing.T) {
	input := testBuildInput()
	original := input

	first, err := BuildManifest(input)
	if err != nil {
		t.Fatalf("BuildManifest() error = %v", err)
	}
	second, err := BuildManifest(input)
	if err != nil {
		t.Fatalf("BuildManifest() second error = %v", err)
	}
	if !reflect.DeepEqual(input, original) {
		t.Fatal("BuildManifest mutated its input")
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("identical inputs produced different manifests")
	}
	if first.ID == "" || first.Executable || first.Confirmable {
		t.Fatal("manifest identity or read-only flags are invalid")
	}
	if len(first.Stages) != 3 ||
		first.Stages[0].ID != StageInstallNode ||
		first.Stages[1].ID != StageSetDefault ||
		first.Stages[2].ID != StageUpdateNodeTools {
		t.Fatalf("unexpected fixed stages: %#v", first.Stages)
	}

	changed := testBuildInput()
	changed.TargetNodeVersion = "24.15.0"
	different, err := BuildManifest(changed)
	if err != nil {
		t.Fatalf("BuildManifest(changed) error = %v", err)
	}
	if different.ID == first.ID {
		t.Fatal("changed target retained the same content-derived ID")
	}
}

func TestBuildManifestRejectsNonExactOrEmptyTargets(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*BuildInput)
	}{
		{"zero time", func(input *BuildInput) { input.CreatedAt = time.Time{} }},
		{"empty node", func(input *BuildInput) { input.TargetNodeVersion = "" }},
		{"node range", func(input *BuildInput) { input.TargetNodeVersion = ">=24.14.0" }},
		{"node prerelease", func(input *BuildInput) { input.TargetNodeVersion = "24.14.0-rc.1" }},
		{"noncanonical node", func(input *BuildInput) { input.TargetNodeVersion = "v24.14.0" }},
		{"empty tools", func(input *BuildInput) { input.NodeTools = NodeToolTargets{} }},
		{"tool range", func(input *BuildInput) { input.NodeTools.NPM = "^12.0.1" }},
		{"tool prerelease", func(input *BuildInput) { input.NodeTools.PNPM = "11.1.0-beta.1" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := testBuildInput()
			test.mutate(&input)
			if _, err := BuildManifest(input); err == nil {
				t.Fatal("BuildManifest unexpectedly succeeded")
			}
		})
	}
}

func TestValidateManifestRejectsTampering(t *testing.T) {
	value := mustManifest(t)
	tests := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{"schema", func(value *Manifest) { value.SchemaVersion = "0.2.0" }},
		{"id", func(value *Manifest) { value.ID = digestID('0') }},
		{"time", func(value *Manifest) { value.CreatedAt = value.CreatedAt.Add(time.Second) }},
		{"executable", func(value *Manifest) { value.Executable = true }},
		{"confirmable", func(value *Manifest) { value.Confirmable = true }},
		{"summary", func(value *Manifest) { value.Summary += " changed" }},
		{"target", func(value *Manifest) { value.TargetNodeVersion = "24.15.0" }},
		{"tools", func(value *Manifest) { value.NodeTools.NPM = "12.0.2" }},
		{"stage id", func(value *Manifest) { value.Stages[0].ID = "other" }},
		{"stage order", func(value *Manifest) { value.Stages[1].Order = 3 }},
		{"stage risk", func(value *Manifest) { value.Stages[1].Risk = "R2" }},
		{"stage schema", func(value *Manifest) { value.Stages[1].PlanSchemaVersion = "0.2.0" }},
		{"dependency", func(value *Manifest) { value.Stages[2].DependsOn[0] = StageInstallNode }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := cloneManifest(value)
			test.mutate(&changed)
			if err := ValidateManifest(changed); err == nil {
				t.Fatal("ValidateManifest unexpectedly succeeded")
			}
		})
	}
}

func testBuildInput() BuildInput {
	return BuildInput{
		CreatedAt:         time.Date(2026, 7, 31, 8, 0, 0, 0, time.UTC),
		TargetNodeVersion: "24.14.0",
		NodeTools: NodeToolTargets{
			NPM:      "12.0.1",
			Corepack: "0.35.0",
			PNPM:     "11.1.0",
		},
	}
}

func mustManifest(t *testing.T) Manifest {
	t.Helper()
	value, err := BuildManifest(testBuildInput())
	if err != nil {
		t.Fatalf("BuildManifest() error = %v", err)
	}
	return value
}
