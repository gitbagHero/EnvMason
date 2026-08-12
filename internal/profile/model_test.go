package profile

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeAppliesDefaultsCanonicalOrderAndKeepsInputImmutable(t *testing.T) {
	input := Profile{
		SchemaVersion: SchemaVersion,
		Name:          "frontend-workstation",
		Description:   "A deterministic profile.",
		Modules: []Module{
			{ID: ModuleFrontendNode},
			{ID: ModuleBase},
		},
	}
	original := cloneProfileForTest(input)
	result, err := Normalize(input)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if !reflect.DeepEqual(input, original) {
		t.Fatal("Normalize mutated its input")
	}
	if len(result.Modules) != 2 || result.Modules[0].ID != ModuleBase ||
		result.Modules[1].ID != ModuleFrontendNode {
		t.Fatalf("canonical modules = %#v", result.Modules)
	}
	base := result.Modules[0]
	if base.Variant != VariantStandard || base.VersionPolicy != nil ||
		base.Options == nil || !boolValue(base.Options.BuildTools) ||
		!boolValue(base.Options.TerminalConfiguration) {
		t.Fatalf("normalized Base = %#v", base)
	}
	frontend := result.Modules[1]
	if frontend.Variant != VariantStandard || frontend.VersionPolicy == nil ||
		frontend.VersionPolicy.Strategy != StrategyLTS || frontend.VersionPolicy.Pin != "" ||
		frontend.Options == nil || !boolValue(frontend.Options.Corepack) ||
		!boolValue(frontend.Options.PNPM) || !boolValue(frontend.Options.BrowserTesting) {
		t.Fatalf("normalized Frontend Node = %#v", frontend)
	}
	if !IsNormalized(result) || IsNormalized(input) {
		t.Fatalf("normalization state input=%t result=%t", IsNormalized(input), IsNormalized(result))
	}
}

func TestNormalizeMinimalOverridesAndExactPin(t *testing.T) {
	input := Profile{
		SchemaVersion: SchemaVersion,
		Name:          "minimal-node",
		Modules: []Module{
			{
				ID: ModuleBase, Variant: VariantMinimal,
				Options: &Options{BuildTools: testBool(true)},
			},
			{
				ID: ModuleFrontendNode, Variant: VariantMinimal,
				VersionPolicy: &VersionPolicy{Strategy: StrategyExact, Pin: "22.14.0"},
				Options:       &Options{PNPM: testBool(true)},
			},
		},
	}
	result, err := Normalize(input)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if !boolValue(result.Modules[0].Options.BuildTools) ||
		boolValue(result.Modules[0].Options.TerminalConfiguration) {
		t.Fatalf("Base overrides = %#v", result.Modules[0].Options)
	}
	frontend := result.Modules[1]
	if frontend.VersionPolicy.Pin != "22.14.0" ||
		boolValue(frontend.Options.Corepack) || !boolValue(frontend.Options.PNPM) ||
		boolValue(frontend.Options.BrowserTesting) {
		t.Fatalf("Frontend overrides = %#v", frontend)
	}
}

func TestNormalizeRejectsInvalidSemantics(t *testing.T) {
	base := func() Profile {
		return Profile{SchemaVersion: SchemaVersion, Name: "test", Modules: []Module{{ID: ModuleBase}}}
	}
	tests := []struct {
		name    string
		mutate  func(*Profile)
		message string
	}{
		{"missing version", func(value *Profile) { value.SchemaVersion = "" }, "schema_version is required"},
		{"unsupported version", func(value *Profile) { value.SchemaVersion = "0.2.0" }, "migration is unavailable"},
		{"invalid name", func(value *Profile) { value.Name = "Invalid Name" }, "name must match"},
		{"description whitespace", func(value *Profile) { value.Description = " padded " }, "description"},
		{"no modules", func(value *Profile) { value.Modules = nil }, "modules must contain"},
		{"unknown module", func(value *Profile) { value.Modules[0].ID = "backend_java" }, "unsupported module"},
		{"unknown variant", func(value *Profile) { value.Modules[0].Variant = "full" }, "unsupported variant"},
		{"duplicate module", func(value *Profile) { value.Modules = append(value.Modules, Module{ID: ModuleBase}) }, "duplicate module"},
		{"conflicting variants", func(value *Profile) {
			value.Modules[0].Variant = VariantMinimal
			value.Modules = append(value.Modules, Module{ID: ModuleBase, Variant: VariantStandard})
		}, "conflicting variants"},
		{"base policy", func(value *Profile) { value.Modules[0].VersionPolicy = &VersionPolicy{Strategy: StrategyLTS} }, "does not accept version_policy"},
		{"base foreign option", func(value *Profile) { value.Modules[0].Options = &Options{PNPM: testBool(true)} }, "Frontend Node options"},
		{"frontend foreign option", func(value *Profile) {
			value.Modules[0] = Module{ID: ModuleFrontendNode, Options: &Options{BuildTools: testBool(true)}}
		}, "Base options"},
		{"unknown policy", func(value *Profile) {
			value.Modules[0] = Module{ID: ModuleFrontendNode, VersionPolicy: &VersionPolicy{Strategy: "current"}}
		}, "unsupported version strategy"},
		{"pin with lts", func(value *Profile) {
			value.Modules[0] = Module{ID: ModuleFrontendNode, VersionPolicy: &VersionPolicy{Strategy: StrategyLTS, Pin: "22.14.0"}}
		}, "does not accept a pin"},
		{"missing exact pin", func(value *Profile) {
			value.Modules[0] = Module{ID: ModuleFrontendNode, VersionPolicy: &VersionPolicy{Strategy: StrategyExact}}
		}, "requires a stable SemVer pin"},
		{"prefixed exact pin", func(value *Profile) {
			value.Modules[0] = Module{ID: ModuleFrontendNode, VersionPolicy: &VersionPolicy{Strategy: StrategyExact, Pin: "v22.14.0"}}
		}, "requires a stable SemVer pin"},
		{"prerelease pin", func(value *Profile) {
			value.Modules[0] = Module{ID: ModuleFrontendNode, VersionPolicy: &VersionPolicy{Strategy: StrategyExact, Pin: "22.14.0-rc.1"}}
		}, "requires a stable SemVer pin"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := base()
			test.mutate(&value)
			_, err := Normalize(value)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("Normalize() error = %v, want containing %q", err, test.message)
			}
		})
	}
}

func cloneProfileForTest(value Profile) Profile {
	result := value
	result.Modules = append([]Module(nil), value.Modules...)
	return result
}

func testBool(value bool) *bool { return &value }

func boolValue(value *bool) bool { return value != nil && *value }
