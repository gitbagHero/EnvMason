package plan

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/lockfile"
	"github.com/gitbagHero/EnvMason/internal/profile"
)

var basePlanTestTime = time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)

func TestBuildBaseInstallCreatesOnlyRequiredFormulaActions(t *testing.T) {
	input := baseInstallInput(lockfile.StateInstallRequired, lockfile.StateInstallRequired)
	before, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	value, err := BuildBaseInstall(input)
	if err != nil {
		t.Fatalf("BuildBaseInstall() error = %v", err)
	}
	after, err := json.Marshal(input)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("BuildBaseInstall mutated its input")
	}
	if value.SchemaVersion != ExecutableSchemaVersion || !value.Executable ||
		value.Summary != basePlanSummary || value.PolicyDigest != input.Lock.Profile.Digest ||
		len(value.Actions) != 2 {
		t.Fatalf("Base Plan = %#v", value)
	}
	if value.Environment.ToolID != "profile.base" || value.Environment.ActiveManager != "homebrew" ||
		value.Environment.ActiveInstallationID != "homebrew:manager" {
		t.Fatalf("Base environment = %#v", value.Environment)
	}
	for index, expected := range []struct{ id, tool, version string }{
		{"install-base-cmake", "homebrew.formula.cmake", "4.0.3"},
		{"install-base-git", "homebrew.formula.git", "2.51.0"},
	} {
		action := value.Actions[index]
		if action.ID != expected.id || action.ToolID != expected.tool || action.Operation != "install" ||
			action.Adapter != "homebrew" || action.TargetVersion != expected.version || action.Risk != RiskR2 ||
			!action.Confirmation.Required || action.Confirmation.Scope != "plan" ||
			action.ElevationRequired || action.RestartRequired || action.Download.State != "unknown" ||
			len(action.Dependencies) != 0 {
			t.Fatalf("action %d = %#v", index, action)
		}
		if !hasCheck(action.Preconditions, "lock_id_matches", input.Lock.ID) ||
			!hasCheck(action.Preconditions, "manager_available", "true") ||
			!hasCheck(action.Verifications, "formula_version_installed", expected.version) {
			t.Fatalf("action %s checks = %#v / %#v", action.ID, action.Preconditions, action.Verifications)
		}
	}
	if err := Validate(value); err != nil {
		t.Fatalf("Validate(Base Plan) error = %v", err)
	}
	data, err := Marshal(value)
	if err != nil || !strings.Contains(string(data), `"schema_version": "0.2.0"`) {
		t.Fatalf("Marshal(Base Plan) = %s, %v", data, err)
	}
}

func TestBuildBaseInstallSkipsSatisfiedAndIsDeterministic(t *testing.T) {
	input := baseInstallInput(lockfile.StateSatisfied, lockfile.StateInstallRequired)
	first, err := BuildBaseInstall(input)
	if err != nil {
		t.Fatalf("first BuildBaseInstall() error = %v", err)
	}
	second, err := BuildBaseInstall(input)
	if err != nil {
		t.Fatalf("second BuildBaseInstall() error = %v", err)
	}
	if !reflect.DeepEqual(first, second) || len(first.Actions) != 1 || first.Actions[0].ID != "install-base-cmake" {
		t.Fatalf("deterministic partial Plan = %#v / %#v", first, second)
	}
	input = baseInstallInput(lockfile.StateSatisfied, lockfile.StateSatisfied)
	if _, err := BuildBaseInstall(input); err == nil || !strings.Contains(err.Error(), "already satisfied") {
		t.Fatalf("all-satisfied error = %v", err)
	}
}

func TestBuildBaseInstallRejectsUnsafeOrInconsistentInputs(t *testing.T) {
	tests := []struct {
		name    string
		input   func() BaseInstallInput
		mutate  func(*BaseInstallInput)
		message string
	}{
		{"time", func() BaseInstallInput {
			return baseInstallInput(lockfile.StateInstallRequired, lockfile.StateInstallRequired)
		}, func(value *BaseInstallInput) { value.CreatedAt = time.Time{} }, "created_at"},
		{"tampered Lock", func() BaseInstallInput {
			return baseInstallInput(lockfile.StateInstallRequired, lockfile.StateInstallRequired)
		}, func(value *BaseInstallInput) { value.Lock.ID = baseFakeDigest('f') }, "invalid Lock"},
		{"future Lock", func() BaseInstallInput {
			return baseInstallInputAt(lockfile.StateInstallRequired, lockfile.StateInstallRequired, basePlanTestTime.Add(time.Minute))
		}, func(*BaseInstallInput) {}, "generated after"},
		{"inventory version", func() BaseInstallInput {
			return baseInstallInput(lockfile.StateInstallRequired, lockfile.StateInstallRequired)
		}, func(value *BaseInstallInput) { value.Inventory.SchemaVersion = "0.2.0" }, "current Inventory"},
		{"future inventory", func() BaseInstallInput {
			return baseInstallInput(lockfile.StateInstallRequired, lockfile.StateInstallRequired)
		}, func(value *BaseInstallInput) { value.Inventory.GeneratedAt = value.CreatedAt.Add(time.Minute) }, "current Inventory"},
		{"target mismatch", func() BaseInstallInput {
			return baseInstallInput(lockfile.StateInstallRequired, lockfile.StateInstallRequired)
		}, func(value *BaseInstallInput) { value.Inventory.System.OSVersion = "14.0" }, "does not match"},
		{"Homebrew missing", func() BaseInstallInput {
			return baseInstallInput(lockfile.StateInstallRequired, lockfile.StateInstallRequired)
		}, func(value *BaseInstallInput) { value.Inventory.Tools = nil }, "bootstrap is not supported"},
		{"multiple active Homebrew", func() BaseInstallInput {
			return baseInstallInput(lockfile.StateInstallRequired, lockfile.StateInstallRequired)
		}, func(value *BaseInstallInput) {
			value.Inventory.Tools[0].Installations = append(value.Inventory.Tools[0].Installations, inventory.Installation{ID: "homebrew:other", Version: "6.1.0", Path: "/usr/local/bin/brew", Manager: "homebrew", ActiveState: inventory.ActiveStateActive, DefaultState: inventory.DefaultStateDefault})
		}, "one active"},
		{"Git conflict", func() BaseInstallInput {
			return baseInstallInput(lockfile.StateConflict, lockfile.StateInstallRequired)
		}, func(*BaseInstallInput) {}, "installation conflict"},
		{"Git unresolved", func() BaseInstallInput {
			return baseInstallInput(lockfile.StateUnresolved, lockfile.StateInstallRequired)
		}, func(*BaseInstallInput) {}, "is unresolved"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := test.input()
			test.mutate(&input)
			_, err := BuildBaseInstall(input)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("BuildBaseInstall() error = %v, want containing %q", err, test.message)
			}
		})
	}
}

func baseInstallInput(gitState, cmakeState lockfile.ResolutionState) BaseInstallInput {
	return baseInstallInputAt(gitState, cmakeState, basePlanTestTime.Add(-time.Minute))
}

func baseInstallInputAt(gitState, cmakeState lockfile.ResolutionState, lockTime time.Time) BaseInstallInput {
	profileValue := profile.Profile{SchemaVersion: profile.SchemaVersion, Name: "base", Modules: []profile.Module{{ID: profile.ModuleBase}}}
	source := lockfile.Source{ID: "homebrew-core", Kind: lockfile.SourcePackageCatalog, URI: "https://formulae.brew.sh/api/formula.json", SnapshotAt: lockTime.Add(-time.Hour), Digest: baseFakeDigest('a')}
	conditions := lockfile.Conditions{OS: inventory.OSMacOS, Architectures: []inventory.Architecture{inventory.ArchitectureAMD64, inventory.ArchitectureARM64}}
	item := func(id, packageID, version string, state lockfile.ResolutionState) lockfile.Item {
		implementation := &lockfile.Implementation{ToolID: "homebrew.formula." + packageID, Manager: "homebrew", PackageKind: "formula", PackageID: packageID, Version: version, SourceID: source.ID, Conditions: conditions}
		result := lockfile.Item{ID: id, Module: profile.ModuleBase, Capability: id, State: state, Implementation: implementation, Observed: []lockfile.Observation{}}
		switch state {
		case lockfile.StateInstallRequired:
			result.Reason = lockfile.ReasonInstallationRequired
		case lockfile.StateSatisfied:
			result.Reason = lockfile.ReasonCompatibleInstallation
			result.Observed = []lockfile.Observation{{Manager: "homebrew", Version: version}}
		case lockfile.StateConflict:
			result.Reason = lockfile.ReasonConflictingInstallation
			result.Observed = []lockfile.Observation{{Manager: "system", Version: "1.0.0"}}
		case lockfile.StateUnresolved:
			result.Implementation = nil
			result.Reason = lockfile.ReasonVersionUnavailable
		}
		return result
	}
	lock, err := lockfile.Build(lockfile.BuildInput{
		GeneratedAt: lockTime, Profile: profileValue,
		Target:  lockfile.Target{OS: inventory.OSMacOS, OSVersion: "15.6", Architecture: inventory.ArchitectureARM64},
		Sources: []lockfile.Source{source},
		Items: []lockfile.Item{
			item("base.git", "git", "2.51.0", gitState),
			item("base.cmake", "cmake", "4.0.3", cmakeState),
			{ID: "base.ssh", Module: profile.ModuleBase, Capability: "base.ssh", State: lockfile.StateUnresolved, Observed: []lockfile.Observation{}, Reason: lockfile.ReasonUnsupportedCapability},
			{ID: "base.terminal", Module: profile.ModuleBase, Capability: "base.terminal", State: lockfile.StateUnresolved, Observed: []lockfile.Observation{}, Reason: lockfile.ReasonUnsupportedCapability},
		},
	})
	if err != nil {
		panic(err)
	}
	return BaseInstallInput{
		Lock: lock, CreatedAt: basePlanTestTime,
		Inventory: inventory.Inventory{
			SchemaVersion: inventory.SchemaVersion, GeneratedAt: basePlanTestTime.Add(-30 * time.Second),
			System: inventory.System{OS: inventory.OSMacOS, OSVersion: "15.6", Architecture: inventory.ArchitectureARM64},
			Tools: []inventory.Tool{{ID: "manager.homebrew", Installations: []inventory.Installation{{
				ID: "homebrew:manager", Version: "6.0.9", Path: "/opt/homebrew/bin/brew", Manager: "homebrew",
				ActiveState: inventory.ActiveStateActive, DefaultState: inventory.DefaultStateDefault,
			}}}},
		},
	}
}

func hasCheck(values []Check, kind, expected string) bool {
	for _, value := range values {
		if value.Kind == kind && value.Expected == expected {
			return true
		}
	}
	return false
}

func baseFakeDigest(character byte) string { return "sha256:" + strings.Repeat(string(character), 64) }
