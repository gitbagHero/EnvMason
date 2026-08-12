package lockfile

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/profile"
)

var lockTestTime = time.Date(2026, 8, 10, 7, 0, 0, 0, time.UTC)

func TestBuildCanonicalizesAllStatesAndKeepsInputsImmutable(t *testing.T) {
	input := validBuildInput()
	original := cloneBuildInputForTest(input)
	value, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !reflect.DeepEqual(input, original) {
		t.Fatal("Build mutated its input")
	}
	if value.SchemaVersion != SchemaVersion || value.Executable || value.Confirmable ||
		value.Profile.Name != "frontend-workstation" || !digestPattern.MatchString(value.Profile.Digest) ||
		!digestPattern.MatchString(value.ID) {
		t.Fatalf("Lock identity = %#v", value)
	}
	if value.Sources[0].ID != "homebrew-core" || value.Sources[1].ID != "node-releases" {
		t.Fatalf("source order = %#v", value.Sources)
	}
	if got := []string{value.Items[0].ID, value.Items[1].ID, value.Items[2].ID, value.Items[3].ID}; !reflect.DeepEqual(got, []string{"base.cmake", "base.git", "frontend.node", "frontend.pnpm"}) {
		t.Fatalf("item order = %#v", got)
	}
	if value.Summary != (Summary{Satisfied: 1, InstallRequired: 1, Conflict: 1, Unresolved: 1}) {
		t.Fatalf("summary = %#v", value.Summary)
	}
	if value.Items[1].Implementation.Conditions.Architectures[0] != inventory.ArchitectureAMD64 ||
		value.Items[2].Observed[0].Manager != "homebrew" {
		t.Fatalf("nested order = %#v / %#v", value.Items[1], value.Items[2])
	}
	if err := Validate(value); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestBuildIsDeterministicForSameSnapshot(t *testing.T) {
	input := validBuildInput()
	first, err := Build(input)
	if err != nil {
		t.Fatalf("first Build() error = %v", err)
	}
	input.Sources[0], input.Sources[1] = input.Sources[1], input.Sources[0]
	input.Items[0], input.Items[3] = input.Items[3], input.Items[0]
	second, err := Build(input)
	if err != nil {
		t.Fatalf("second Build() error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same snapshot produced different Locks\nfirst=%#v\nsecond=%#v", first, second)
	}
}

func TestValidateRejectsTamperingAndUnsafeShapes(t *testing.T) {
	valid, err := Build(validBuildInput())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	tests := []struct {
		name    string
		mutate  func(*Lock)
		message string
	}{
		{"schema", func(value *Lock) { value.SchemaVersion = "0.2.0" }, "fixed identity"},
		{"executable", func(value *Lock) { value.Executable = true }, "fixed identity"},
		{"profile", func(value *Lock) { value.Profile.Name = "Invalid_Name" }, "Profile reference"},
		{"target", func(value *Lock) { value.Target.OS = inventory.OSUnknown }, "target OS"},
		{"credential URI", func(value *Lock) { value.Sources[0].URI = "https://user:secret@example.test/catalog" }, "credential-free"},
		{"query URI", func(value *Lock) { value.Sources[0].URI += "?token=secret" }, "credential-free"},
		{"future source", func(value *Lock) { value.Sources[0].SnapshotAt = value.GeneratedAt.Add(time.Minute) }, "metadata is invalid"},
		{"duplicate source", func(value *Lock) { value.Sources[1].ID = value.Sources[0].ID }, "duplicate source"},
		{"dangling source", func(value *Lock) { value.Items[0].Implementation.SourceID = "missing" }, "unknown source"},
		{"condition OS", func(value *Lock) { value.Items[0].Implementation.Conditions.OS = inventory.OSWindows }, "conditions do not match"},
		{"condition architecture", func(value *Lock) {
			value.Items[0].Implementation.Conditions.Architectures = []inventory.Architecture{inventory.ArchitectureAMD64}
		}, "exclude"},
		{"duplicate architecture", func(value *Lock) {
			value.Items[0].Implementation.Conditions.Architectures = []inventory.Architecture{inventory.ArchitectureARM64, inventory.ArchitectureARM64}
		}, "duplicate architecture"},
		{"missing implementation", func(value *Lock) { value.Items[0].Implementation = nil }, "requires an implementation"},
		{"unresolved implementation", func(value *Lock) { value.Items[3].Implementation = valid.Items[0].Implementation }, "must omit implementation"},
		{"satisfied mismatch", func(value *Lock) { value.Items[1].Observed[0].Version = "1.0.0" }, "lacks a matching observation"},
		{"install observation", func(value *Lock) { value.Items[0].Observed = []Observation{{Manager: "homebrew", Version: "1.0.0"}} }, "has observations"},
		{"conflict empty", func(value *Lock) { value.Items[2].Observed = []Observation{} }, "requires observations"},
		{"duplicate observation", func(value *Lock) {
			value.Items[2].Observed = append(value.Items[2].Observed, value.Items[2].Observed[0])
		}, "duplicate observation"},
		{"duplicate item", func(value *Lock) { value.Items[1].ID = value.Items[0].ID }, "duplicate item"},
		{"duplicate capability", func(value *Lock) { value.Items[1].Capability = value.Items[0].Capability }, "duplicate capability"},
		{"summary", func(value *Lock) { value.Summary.Unresolved++ }, "summary is invalid"},
		{"order", func(value *Lock) { value.Items[0], value.Items[1] = value.Items[1], value.Items[0] }, "canonically ordered"},
		{"id", func(value *Lock) { value.ID = fakeDigest('f') }, "content-derived ID"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := cloneLock(valid)
			test.mutate(&value)
			err := Validate(value)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.message)
			}
		})
	}
}

func validBuildInput() BuildInput {
	conditions := Conditions{OS: inventory.OSMacOS, Architectures: []inventory.Architecture{inventory.ArchitectureARM64, inventory.ArchitectureAMD64}}
	implementation := func(tool, manager, kind, packageID, version, source string) *Implementation {
		return &Implementation{ToolID: tool, Manager: manager, PackageKind: kind, PackageID: packageID, Version: version, SourceID: source, Conditions: conditions}
	}
	return BuildInput{
		GeneratedAt: lockTestTime,
		Profile:     profile.Profile{SchemaVersion: profile.SchemaVersion, Name: "frontend-workstation", Modules: []profile.Module{{ID: profile.ModuleFrontendNode}, {ID: profile.ModuleBase}}},
		Target:      Target{OS: inventory.OSMacOS, OSVersion: "15.6", Architecture: inventory.ArchitectureARM64},
		Sources: []Source{
			{ID: "node-releases", Kind: SourceRuntimeCatalog, URI: "https://nodejs.org/download/release/index.json", SnapshotAt: lockTestTime.Add(-time.Hour), Digest: fakeDigest('b')},
			{ID: "homebrew-core", Kind: SourcePackageCatalog, URI: "https://formulae.brew.sh/api/formula.json", SnapshotAt: lockTestTime.Add(-2 * time.Hour), Digest: fakeDigest('a')},
		},
		Items: []Item{
			{ID: "frontend.pnpm", Module: profile.ModuleFrontendNode, Capability: "frontend.pnpm", State: StateUnresolved, Observed: []Observation{}, Reason: ReasonVersionUnavailable},
			{ID: "base.git", Module: profile.ModuleBase, Capability: "base.git", State: StateSatisfied, Implementation: implementation("base.git", "homebrew", "formula", "git", "2.51.0", "homebrew-core"), Observed: []Observation{{Manager: "homebrew", Version: "2.51.0"}}, Reason: ReasonCompatibleInstallation},
			{ID: "frontend.node", Module: profile.ModuleFrontendNode, Capability: "frontend.node", State: StateConflict, Implementation: implementation("runtime.node", "nvm", "runtime", "node", "22.18.0", "node-releases"), Observed: []Observation{{Manager: "nvm", Version: "20.19.0"}, {Manager: "homebrew", Version: "24.6.0"}}, Reason: ReasonConflictingInstallation},
			{ID: "base.cmake", Module: profile.ModuleBase, Capability: "base.cmake", State: StateInstallRequired, Implementation: implementation("base.cmake", "homebrew", "formula", "cmake", "4.0.3", "homebrew-core"), Observed: []Observation{}, Reason: ReasonInstallationRequired},
		},
	}
}

func cloneBuildInputForTest(value BuildInput) BuildInput {
	result := value
	result.Profile.Modules = append([]profile.Module{}, value.Profile.Modules...)
	result.Sources = cloneSources(value.Sources)
	result.Items = cloneItems(value.Items)
	return result
}

func fakeDigest(character byte) string { return "sha256:" + strings.Repeat(string(character), 64) }
