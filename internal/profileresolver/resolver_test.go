package profileresolver

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

var resolverTestTime = time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)

func TestResolveStandardBaseAndFrontendNodeAcrossAllStates(t *testing.T) {
	input := resolverInput()
	before, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	result, err := Resolve(input)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	after, err := json.Marshal(input)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("Resolve mutated its input")
	}
	if result.Summary != (lockfile.Summary{Satisfied: 2, InstallRequired: 4, Conflict: 1, Unresolved: 2}) {
		t.Fatalf("summary = %#v", result.Summary)
	}
	assertItem(t, result, CapabilityBaseGit, lockfile.StateSatisfied, lockfile.ReasonCompatibleInstallation, "2.51.0")
	assertItem(t, result, CapabilityBaseSSH, lockfile.StateInstallRequired, lockfile.ReasonInstallationRequired, "10.0")
	assertItem(t, result, CapabilityBaseCMake, lockfile.StateInstallRequired, lockfile.ReasonInstallationRequired, "4.0.3")
	assertItem(t, result, CapabilityBaseTerminal, lockfile.StateUnresolved, lockfile.ReasonUnsupportedCapability, "")
	assertItem(t, result, CapabilityFrontendNode, lockfile.StateSatisfied, lockfile.ReasonCompatibleInstallation, "22.18.0")
	assertItem(t, result, CapabilityFrontendNPM, lockfile.StateConflict, lockfile.ReasonConflictingInstallation, "11.5.0")
	assertItem(t, result, CapabilityFrontendCorepack, lockfile.StateInstallRequired, lockfile.ReasonInstallationRequired, "0.34.0")
	assertItem(t, result, CapabilityFrontendPNPM, lockfile.StateInstallRequired, lockfile.ReasonInstallationRequired, "10.15.0")
	assertItem(t, result, CapabilityFrontendBrowserTesting, lockfile.StateUnresolved, lockfile.ReasonUnsupportedCapability, "")
	data, err := lockfile.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal(Lock) error = %v", err)
	}
	if strings.Contains(string(data), "/Users/private") || strings.Contains(string(data), "fixture source private") {
		t.Fatalf("Lock leaked Inventory private data: %s", data)
	}
}

func TestResolveNodeVersionStrategies(t *testing.T) {
	tests := []struct {
		name     string
		policy   *profile.VersionPolicy
		expected string
		state    lockfile.ResolutionState
	}{
		{"default lts", nil, "22.18.0", lockfile.StateSatisfied},
		{"stable", &profile.VersionPolicy{Strategy: profile.StrategyStable}, "24.6.0", lockfile.StateConflict},
		{"exact", &profile.VersionPolicy{Strategy: profile.StrategyExact, Pin: "20.19.4"}, "20.19.4", lockfile.StateConflict},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := resolverInput()
			input.Profile.Modules = []profile.Module{{
				ID: profile.ModuleFrontendNode, Variant: profile.VariantMinimal,
				VersionPolicy: test.policy,
			}}
			if test.name == "exact" {
				input.Catalog.Entries = append(input.Catalog.Entries, catalogEntry(
					CapabilityFrontendNode, ChannelLTS, "runtime.node", "nvm", "runtime", "node", "20.19.4", "node-packages",
				))
			}
			result, err := Resolve(input)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			reason := lockfile.ReasonConflictingInstallation
			if test.state == lockfile.StateSatisfied {
				reason = lockfile.ReasonCompatibleInstallation
			}
			assertItem(t, result, CapabilityFrontendNode, test.state, reason, test.expected)
			if len(result.Items) != 2 {
				t.Fatalf("minimal Frontend Node items = %#v", result.Items)
			}
		})
	}
}

func TestResolveMinimalOptionsAndMissingOrAmbiguousCatalog(t *testing.T) {
	input := resolverInput()
	input.Profile.Modules = []profile.Module{
		{ID: profile.ModuleBase, Variant: profile.VariantMinimal, Options: &profile.Options{BuildTools: testBool(true)}},
		{ID: profile.ModuleFrontendNode, Variant: profile.VariantMinimal, Options: &profile.Options{Corepack: testBool(true), PNPM: testBool(true)}},
	}
	input.Catalog.Entries = append(input.Catalog.Entries, catalogEntry(
		CapabilityFrontendPNPM, ChannelStable, "ecosystem.pnpm", "nvm", "runtime", "pnpm", "10.16.0", "node-packages",
	))
	result, err := Resolve(input)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if findItem(t, result, CapabilityFrontendPNPM).Reason != lockfile.ReasonVersionUnavailable {
		t.Fatalf("ambiguous pnpm = %#v", findItem(t, result, CapabilityFrontendPNPM))
	}
	if findOptionalItem(result, CapabilityBaseTerminal) != nil ||
		findOptionalItem(result, CapabilityFrontendBrowserTesting) != nil {
		t.Fatalf("minimal variants unexpectedly included default optional capabilities: %#v", result.Items)
	}
	input.Catalog.Entries = removeCapability(input.Catalog.Entries, CapabilityBaseCMake)
	result, err = Resolve(input)
	if err != nil {
		t.Fatalf("Resolve() missing catalog error = %v", err)
	}
	assertItem(t, result, CapabilityBaseCMake, lockfile.StateUnresolved, lockfile.ReasonVersionUnavailable, "")
}

func TestResolveUnsupportedPlatformAndArchitecture(t *testing.T) {
	tests := []struct {
		name   string
		target lockfile.Target
		reason string
	}{
		{"windows", lockfile.Target{OS: inventory.OSWindows, OSVersion: "11.0", Architecture: inventory.ArchitectureAMD64}, lockfile.ReasonUnsupportedPlatform},
		{"macos 386", lockfile.Target{OS: inventory.OSMacOS, OSVersion: "15.6", Architecture: inventory.Architecture386}, lockfile.ReasonUnsupportedArchitecture},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := resolverInput()
			input.Target = test.target
			input.Inventory.System.OS = test.target.OS
			input.Inventory.System.OSVersion = test.target.OSVersion
			input.Inventory.System.Architecture = test.target.Architecture
			result, err := Resolve(input)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			for _, item := range result.Items {
				if item.State != lockfile.StateUnresolved || item.Reason != test.reason || item.Implementation != nil {
					t.Fatalf("unsupported target item = %#v", item)
				}
			}
		})
	}
}

func TestResolveIsStableAcrossCatalogAndInventoryOrder(t *testing.T) {
	input := resolverInput()
	first, err := Resolve(input)
	if err != nil {
		t.Fatalf("first Resolve() error = %v", err)
	}
	input.Catalog.Sources[0], input.Catalog.Sources[1] = input.Catalog.Sources[1], input.Catalog.Sources[0]
	for left, right := 0, len(input.Catalog.Entries)-1; left < right; left, right = left+1, right-1 {
		input.Catalog.Entries[left], input.Catalog.Entries[right] = input.Catalog.Entries[right], input.Catalog.Entries[left]
	}
	for left, right := 0, len(input.Inventory.Tools)-1; left < right; left, right = left+1, right-1 {
		input.Inventory.Tools[left], input.Inventory.Tools[right] = input.Inventory.Tools[right], input.Inventory.Tools[left]
	}
	second, err := Resolve(input)
	if err != nil {
		t.Fatalf("second Resolve() error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same snapshots produced different Locks\nfirst=%#v\nsecond=%#v", first, second)
	}
}

func TestResolveRejectsInvalidInputsBeforeResolution(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Input)
		message string
	}{
		{"time", func(value *Input) { value.GeneratedAt = time.Time{} }, "generated_at"},
		{"profile", func(value *Input) { value.Profile.SchemaVersion = "0.2.0" }, "unsupported schema_version"},
		{"inventory version", func(value *Input) { value.Inventory.SchemaVersion = "0.2.0" }, "current Inventory"},
		{"future inventory", func(value *Input) { value.Inventory.GeneratedAt = value.GeneratedAt.Add(time.Minute) }, "current Inventory"},
		{"target mismatch", func(value *Input) { value.Inventory.System.OSVersion = "14.0" }, "does not match"},
		{"no sources", func(value *Input) { value.Catalog.Sources = nil }, "at least one source"},
		{"duplicate source", func(value *Input) { value.Catalog.Sources[1].ID = value.Catalog.Sources[0].ID }, "duplicate catalog source"},
		{"unknown capability", func(value *Input) { value.Catalog.Entries[0].Capability = "backend.java" }, "unsupported capability"},
		{"unknown channel", func(value *Input) { value.Catalog.Entries[0].Channel = "current" }, "unsupported channel"},
		{"dangling source", func(value *Input) { value.Catalog.Entries[0].Implementation.SourceID = "missing" }, "unknown source"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := resolverInput()
			test.mutate(&input)
			_, err := Resolve(input)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("Resolve() error = %v, want containing %q", err, test.message)
			}
		})
	}
}

func resolverInput() Input {
	profileValue := profile.Profile{
		SchemaVersion: profile.SchemaVersion,
		Name:          "frontend-workstation",
		Modules: []profile.Module{
			{ID: profile.ModuleFrontendNode},
			{ID: profile.ModuleBase},
		},
	}
	sources := []lockfile.Source{
		{ID: "node-packages", Kind: lockfile.SourceRuntimeCatalog, URI: "https://nodejs.org/download/release/index.json", SnapshotAt: resolverTestTime.Add(-time.Hour), Digest: fakeDigest('b')},
		{ID: "homebrew-core", Kind: lockfile.SourcePackageCatalog, URI: "https://formulae.brew.sh/api/formula.json", SnapshotAt: resolverTestTime.Add(-2 * time.Hour), Digest: fakeDigest('a')},
	}
	entries := []CatalogEntry{
		catalogEntry(CapabilityBaseGit, ChannelStable, "homebrew.formula.git", "homebrew", "formula", "git", "2.51.0", "homebrew-core"),
		catalogEntry(CapabilityBaseSSH, ChannelStable, "base.ssh", "system", "system", "com.apple.openssh", "10.0", "homebrew-core"),
		catalogEntry(CapabilityBaseCMake, ChannelStable, "homebrew.formula.cmake", "homebrew", "formula", "cmake", "4.0.3", "homebrew-core"),
		catalogEntry(CapabilityFrontendNode, ChannelLTS, "runtime.node", "nvm", "runtime", "node", "22.18.0", "node-packages"),
		catalogEntry(CapabilityFrontendNode, ChannelStable, "runtime.node", "nvm", "runtime", "node", "24.6.0", "node-packages"),
		catalogEntry(CapabilityFrontendNPM, ChannelStable, "ecosystem.npm", "nvm", "runtime", "npm", "11.5.0", "node-packages"),
		catalogEntry(CapabilityFrontendCorepack, ChannelStable, "ecosystem.corepack", "nvm", "runtime", "corepack", "0.34.0", "node-packages"),
		catalogEntry(CapabilityFrontendPNPM, ChannelStable, "ecosystem.pnpm", "nvm", "runtime", "pnpm", "10.15.0", "node-packages"),
	}
	return Input{
		GeneratedAt: resolverTestTime,
		Profile:     profileValue,
		Target:      lockfile.Target{OS: inventory.OSMacOS, OSVersion: "15.6", Architecture: inventory.ArchitectureARM64},
		Inventory: inventory.Inventory{
			SchemaVersion: inventory.SchemaVersion,
			GeneratedAt:   resolverTestTime.Add(-time.Minute),
			System: inventory.System{
				OS: inventory.OSMacOS, OSVersion: "15.6", Architecture: inventory.ArchitectureARM64,
			},
			Tools: []inventory.Tool{
				{ID: "homebrew.formula.git", Installations: []inventory.Installation{{Version: "2.51.0", NormalizedVersion: "2.51.0", Manager: "homebrew", Architecture: inventory.ArchitectureARM64, Path: "/Users/private/homebrew/git", Sources: []inventory.SourceMetadata{{Name: "fixture source private"}}}}},
				{ID: "runtime.node", Installations: []inventory.Installation{{Version: "v22.18.0", NormalizedVersion: "22.18.0", Manager: "nvm", Architecture: inventory.ArchitectureARM64, Path: "/Users/private/.nvm/node"}}},
				{ID: "ecosystem.npm", Installations: []inventory.Installation{{Version: "10.9.0", Manager: "nvm", Architecture: inventory.ArchitectureUnknown, Path: "/Users/private/.nvm/npm"}}},
			},
		},
		Catalog: Catalog{Sources: sources, Entries: entries},
	}
}

func catalogEntry(capability, channel, tool, manager, kind, packageID, version, source string) CatalogEntry {
	return CatalogEntry{
		Capability: capability,
		Channel:    channel,
		Implementation: lockfile.Implementation{
			ToolID: tool, Manager: manager, PackageKind: kind, PackageID: packageID,
			Version: version, SourceID: source,
			Conditions: lockfile.Conditions{OS: inventory.OSMacOS, Architectures: []inventory.Architecture{inventory.ArchitectureARM64, inventory.ArchitectureAMD64}},
		},
	}
}

func assertItem(t *testing.T, value lockfile.Lock, id string, state lockfile.ResolutionState, reason, version string) {
	t.Helper()
	item := findItem(t, value, id)
	if item.State != state || item.Reason != reason {
		t.Fatalf("item %s = %#v", id, item)
	}
	if version == "" {
		if item.Implementation != nil {
			t.Fatalf("item %s unexpectedly has implementation %#v", id, item.Implementation)
		}
	} else if item.Implementation == nil || item.Implementation.Version != version {
		t.Fatalf("item %s implementation = %#v, want version %q", id, item.Implementation, version)
	}
}

func findItem(t *testing.T, value lockfile.Lock, id string) lockfile.Item {
	t.Helper()
	if item := findOptionalItem(value, id); item != nil {
		return *item
	}
	t.Fatalf("item %q not found", id)
	return lockfile.Item{}
}

func findOptionalItem(value lockfile.Lock, id string) *lockfile.Item {
	for index := range value.Items {
		if value.Items[index].ID == id {
			return &value.Items[index]
		}
	}
	return nil
}

func removeCapability(values []CatalogEntry, capability string) []CatalogEntry {
	result := []CatalogEntry{}
	for _, value := range values {
		if value.Capability != capability {
			result = append(result, value)
		}
	}
	return result
}

func testBool(value bool) *bool { return &value }

func fakeDigest(character byte) string { return "sha256:" + strings.Repeat(string(character), 64) }
