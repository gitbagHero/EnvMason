package plan

import (
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
)

func TestBuildNodeToolsCreatesOrderedImmutableR2Actions(t *testing.T) {
	input := nodeToolsInput()
	value, err := BuildNodeTools(input)
	if err != nil {
		t.Fatal(err)
	}
	if value.SchemaVersion != ExecutableSchemaVersion || !value.Executable || len(value.Actions) != 3 {
		t.Fatalf("Plan = %#v", value)
	}
	if value.Actions[0].ToolID != NodeToolNPM || value.Actions[1].ToolID != NodeToolCorepack || value.Actions[2].ToolID != NodeToolPNPM {
		t.Fatalf("action order = %#v", value.Actions)
	}
	if len(value.Actions[1].Dependencies) != 1 || value.Actions[1].Dependencies[0] != "update-npm" ||
		len(value.Actions[2].Dependencies) != 1 || value.Actions[2].Dependencies[0] != "update-corepack" {
		t.Fatalf("dependencies = %#v", value.Actions)
	}
	if value.Actions[2].Adapter != NodeToolProviderCorepack || value.Actions[2].Risk != RiskR2 {
		t.Fatalf("pnpm action = %#v", value.Actions[2])
	}
	again, err := BuildNodeTools(input)
	if err != nil || again.ID != value.ID {
		t.Fatalf("deterministic ID = %q / %q / %v", value.ID, again.ID, err)
	}
	changed := input
	changed.Targets = append([]NodeToolTarget{}, input.Targets...)
	changed.Targets[0].TargetVersion = "12.0.2"
	other, err := BuildNodeTools(changed)
	if err != nil || other.ID == value.ID {
		t.Fatalf("changed Plan ID = %q / %q / %v", value.ID, other.ID, err)
	}
}

func TestBuildNodeToolsRejectsUnsafeOrUnscopedInputs(t *testing.T) {
	tests := []struct {
		name string
		edit func(*NodeToolsInput)
		want string
	}{
		{name: "no selection", edit: func(value *NodeToolsInput) { value.Targets = nil }, want: "at least one"},
		{name: "floating target", edit: func(value *NodeToolsInput) { value.Targets[0].TargetVersion = "latest" }, want: "invalid version"},
		{name: "downgrade", edit: func(value *NodeToolsInput) { value.Targets[1].TargetVersion = "10.0.0" }, want: "older than"},
		{name: "wrong provider", edit: func(value *NodeToolsInput) { value.Targets[1].Provider = NodeToolProviderCorepack }, want: "must be updated through npm"},
		{name: "duplicate", edit: func(value *NodeToolsInput) { value.Targets = append(value.Targets, value.Targets[0]) }, want: "duplicate"},
		{name: "missing target Node", edit: func(value *NodeToolsInput) { value.NodeVersion = "22.22.3" }, want: "not an installed NVM"},
		{name: "bad digest", edit: func(value *NodeToolsInput) { value.NVMScriptDigest = "changed" }, want: "control-file digests"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := nodeToolsInput()
			test.edit(&input)
			if _, err := BuildNodeTools(input); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func nodeToolsInput() NodeToolsInput {
	digest := "sha256:" + strings.Repeat("a", 64)
	return NodeToolsInput{
		CreatedAt: time.Date(2026, 7, 28, 8, 0, 0, 0, time.UTC), NodeVersion: "24.12.0",
		NVMScriptDigest: digest, DefaultAliasDigest: "sha256:" + strings.Repeat("b", 64),
		Inventory: inventory.Inventory{
			SchemaVersion: inventory.SchemaVersion, GeneratedAt: time.Date(2026, 7, 28, 7, 59, 0, 0, time.UTC),
			System: inventory.System{OS: inventory.OSMacOS, OSVersion: "15.0", Architecture: inventory.ArchitectureARM64},
			Tools: []inventory.Tool{{ID: "runtime.node", DisplayName: "Node.js", Category: inventory.CategoryRuntime, Installations: []inventory.Installation{
				{ID: "node-nvm-active", Version: "v26.5.0", Path: "$HOME/.nvm/versions/node/v26.5.0/bin/node", Manager: "nvm", Architecture: inventory.ArchitectureARM64, ActiveState: inventory.ActiveStateActive, DefaultState: inventory.DefaultStateDefault},
				{ID: "node-nvm-target", Version: "v24.12.0", Path: "$HOME/.nvm/versions/node/v24.12.0/bin/node", Manager: "nvm", Architecture: inventory.ArchitectureARM64, ActiveState: inventory.ActiveStateInactive, DefaultState: inventory.DefaultStateNonDefault},
			}}},
		},
		Targets: []NodeToolTarget{
			{ToolID: NodeToolPNPM, CurrentVersion: "unknown", TargetVersion: "11.1.0", Provider: NodeToolProviderCorepack, ControlDigest: digest},
			{ToolID: NodeToolNPM, CurrentVersion: "11.6.2", TargetVersion: "12.0.1", Provider: NodeToolProviderNPM, ControlDigest: digest},
			{ToolID: NodeToolCorepack, CurrentVersion: "0.34.5", TargetVersion: "0.35.0", Provider: NodeToolProviderNPM, ControlDigest: digest},
		},
	}
}
