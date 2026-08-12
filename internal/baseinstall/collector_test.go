package baseinstall

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/lockfile"
	"github.com/gitbagHero/EnvMason/internal/plan"
	"github.com/gitbagHero/EnvMason/internal/profile"
)

var collectionTestTime = time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)

func TestCollectTransactionFactsBuildsExactReadOnlyReviewInputs(t *testing.T) {
	input := validCollectionInput(t)
	before, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := CollectTransactionFacts(input)
	if err != nil {
		t.Fatalf("CollectTransactionFacts() error = %v", err)
	}
	after, err := json.Marshal(input)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("CollectTransactionFacts mutated its input")
	}

	if facts.Baseline.ObservedAt != collectionTestTime ||
		facts.Baseline.InstallationID != "homebrew:manager" ||
		facts.Baseline.HomebrewVersion != "6.0.9" ||
		facts.Baseline.ExecutableDigest != digestBytes(input.ExecutableData) ||
		facts.Baseline.ConfigurationDigest == "" ||
		facts.Baseline.CatalogDigest != digestBytes(input.CatalogJSON) ||
		!facts.Baseline.ConfigurationSafe ||
		len(facts.Baseline.UnsafeConfigurationKeys) != 0 {
		t.Fatalf("baseline = %#v", facts.Baseline)
	}
	if len(facts.Actions) != 2 || facts.Actions[0].ActionID != "install-base-cmake" ||
		facts.Actions[1].ActionID != "install-base-git" {
		t.Fatalf("actions = %#v", facts.Actions)
	}
	cmake := facts.Actions[0]
	if cmake.Root.DownloadBytes != 20 || len(cmake.Dependencies) != 2 ||
		cmake.Dependencies[0].Name != "gettext" || cmake.Dependencies[0].State != FormulaSatisfied ||
		cmake.Dependencies[0].DownloadBytes != 0 || cmake.Dependencies[1].Name != "openssl@3" ||
		cmake.Dependencies[1].State != FormulaInstallRequired || cmake.Dependencies[1].DownloadBytes != 30 {
		t.Fatalf("CMake preview = %#v", cmake)
	}
	git := facts.Actions[1]
	if git.Root.DownloadBytes != 10 || len(git.Dependencies) != 1 ||
		git.Dependencies[0].Name != "gettext" || git.Dependencies[0].State != FormulaSatisfied {
		t.Fatalf("Git preview = %#v", git)
	}

	review, err := PrepareTransactionReview(TransactionReviewInput{
		PreparedAt: input.ObservedAt,
		Plan:       input.Plan,
		Lock:       input.Lock,
		Baseline:   facts.Baseline,
		Actions:    facts.Actions,
	})
	if err != nil {
		t.Fatalf("PrepareTransactionReview(collected) error = %v", err)
	}
	if review.Summary.Formulae != 4 || review.Summary.Satisfied != 1 ||
		review.Summary.InstallRequired != 3 || review.Summary.TotalDownloadBytes != 60 {
		t.Fatalf("review summary = %#v", review.Summary)
	}
	data, err := json.Marshal(facts)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"/opt/homebrew", "formulae.brew.sh", "HOMEBREW_NO_AUTO_UPDATE",
		"private-value", "https://mirror.invalid", "secret-token",
	} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("collected facts leaked %q: %s", forbidden, data)
		}
	}
}

func TestCollectTransactionFactsUsesTargetMacOSPathSemantics(t *testing.T) {
	input := validCollectionInput(t)
	input.ExecutablePath = `C:\opt\homebrew\bin\brew.exe`
	input.Inventory.Tools[0].Installations[0].Path = input.ExecutablePath
	if _, err := CollectTransactionFacts(input); err == nil ||
		!strings.Contains(err.Error(), "executable path") {
		t.Fatalf("Windows host path was accepted for macOS target: %v", err)
	}
}

func TestCollectTransactionFactsIsDeterministicAcrossCatalogAndArtifactOrder(t *testing.T) {
	firstInput := validCollectionInput(t)
	first, err := CollectTransactionFacts(firstInput)
	if err != nil {
		t.Fatal(err)
	}
	secondInput := validCollectionInput(t)
	var formulas []json.RawMessage
	if err := json.Unmarshal(secondInput.CatalogJSON, &formulas); err != nil {
		t.Fatal(err)
	}
	for left, right := 0, len(formulas)-1; left < right; left, right = left+1, right-1 {
		formulas[left], formulas[right] = formulas[right], formulas[left]
	}
	secondInput.CatalogJSON, err = json.Marshal(formulas)
	if err != nil {
		t.Fatal(err)
	}
	secondInput = rebindCollectionCatalog(t, secondInput)
	for left, right := 0, len(secondInput.Artifacts)-1; left < right; left, right = left+1, right-1 {
		secondInput.Artifacts[left], secondInput.Artifacts[right] = secondInput.Artifacts[right], secondInput.Artifacts[left]
	}
	second, err := CollectTransactionFacts(secondInput)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.Actions, second.Actions) {
		t.Fatalf("action ordering changed = %#v / %#v", first.Actions, second.Actions)
	}
	if first.Baseline.CatalogDigest == second.Baseline.CatalogDigest {
		t.Fatal("different catalog byte snapshots unexpectedly shared a digest")
	}
}

func TestCollectTransactionFactsAppliesFixedConfigurationPrecedence(t *testing.T) {
	input := validCollectionInput(t)
	input.Configuration.Environment["HOMEBREW_NO_AUTO_UPDATE"] = "0"
	input.Configuration.User = []byte("HOMEBREW_NO_AUTO_UPDATE=1\n")
	if _, err := CollectTransactionFacts(input); err != nil {
		t.Fatalf("user configuration did not override environment: %v", err)
	}

	unsafe := validCollectionInput(t)
	unsafe.Configuration.Prefix = []byte("HOMEBREW_API_DOMAIN=https://mirror.invalid/secret-token\n")
	_, err := CollectTransactionFacts(unsafe)
	if err == nil || !strings.Contains(err.Error(), "HOMEBREW_API_DOMAIN") ||
		strings.Contains(err.Error(), "mirror.invalid") || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("unsafe configuration error = %v", err)
	}

	missing := validCollectionInput(t)
	delete(missing.Configuration.Environment, "HOMEBREW_NO_INSTALL_CLEANUP")
	if _, err := CollectTransactionFacts(missing); err == nil ||
		!strings.Contains(err.Error(), "HOMEBREW_NO_INSTALL_CLEANUP") {
		t.Fatalf("missing control error = %v", err)
	}

	malformed := validCollectionInput(t)
	malformed.Configuration.System = []byte("export HOMEBREW_NO_AUTO_UPDATE=1\n")
	if _, err := CollectTransactionFacts(malformed); err == nil ||
		!strings.Contains(err.Error(), "unsupported line") {
		t.Fatalf("malformed configuration error = %v", err)
	}
}

func TestCollectTransactionFactsRejectsDriftIncompleteClosureAndArtifacts(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*TransactionCollectionInput)
		message string
	}{
		{"observation time", func(value *TransactionCollectionInput) {
			value.Inventory.GeneratedAt = value.ObservedAt.Add(-time.Second)
		}, "times do not match"},
		{"expired Plan", func(value *TransactionCollectionInput) {
			value.ObservedAt = value.Plan.ExpiresAt
			value.Inventory.GeneratedAt = value.ObservedAt
		}, "not active"},
		{"Homebrew version", func(value *TransactionCollectionInput) {
			value.Inventory.Tools[0].Installations[0].Version = "6.1.0"
		}, "identity"},
		{"Homebrew architecture", func(value *TransactionCollectionInput) {
			value.Inventory.Tools[0].Installations[0].Architecture = inventory.ArchitectureAMD64
		}, "architecture"},
		{"executable path", func(value *TransactionCollectionInput) {
			value.ExecutablePath = "/tmp/brew"
		}, "executable path"},
		{"empty executable", func(value *TransactionCollectionInput) {
			value.ExecutableData = nil
		}, "empty or oversized"},
		{"catalog digest", func(value *TransactionCollectionInput) {
			value.CatalogJSON = append(append([]byte{}, value.CatalogJSON...), ' ')
		}, "locked source"},
		{"bottle tag", func(value *TransactionCollectionInput) {
			value.BottleTag = "sonoma"
		}, "bottle tag"},
		{"missing artifact", func(value *TransactionCollectionInput) {
			value.Artifacts = value.Artifacts[1:]
		}, "no matching bottle artifact"},
		{"extra artifact", func(value *TransactionCollectionInput) {
			value.Artifacts = append(value.Artifacts, BottleArtifact{
				Formula: "extra", Version: "1.0.0", BottleTag: value.BottleTag,
				SHA256: transactionDigest('e'), DownloadBytes: 1,
			})
		}, "unrequested formula"},
		{"wrong artifact digest", func(value *TransactionCollectionInput) {
			value.Artifacts[0].SHA256 = transactionDigest('f')
		}, "no matching bottle artifact"},
		{"zero artifact size", func(value *TransactionCollectionInput) {
			value.Artifacts[0].DownloadBytes = 0
		}, "artifact is invalid"},
		{"missing dependency", func(value *TransactionCollectionInput) {
			value.CatalogJSON = mutateCatalog(t, value.CatalogJSON, func(formulas []map[string]any) []map[string]any {
				for _, formula := range formulas {
					if formula["name"] == "gettext" {
						formula["dependencies"] = []string{"missing"}
					}
				}
				return formulas
			})
			*value = rebindCollectionCatalog(t, *value)
		}, "missing formula"},
		{"dependency cycle", func(value *TransactionCollectionInput) {
			value.CatalogJSON = mutateCatalog(t, value.CatalogJSON, func(formulas []map[string]any) []map[string]any {
				for _, formula := range formulas {
					if formula["name"] == "gettext" {
						formula["dependencies"] = []string{"git"}
					}
				}
				return formulas
			})
			*value = rebindCollectionCatalog(t, *value)
		}, "dependency cycle"},
		{"root version drift", func(value *TransactionCollectionInput) {
			value.CatalogJSON = mutateCatalog(t, value.CatalogJSON, func(formulas []map[string]any) []map[string]any {
				for _, formula := range formulas {
					if formula["name"] == "git" {
						formula["versions"] = map[string]string{"stable": "9.9.9"}
					}
				}
				return formulas
			})
			*value = rebindCollectionCatalog(t, *value)
		}, "changed after Plan"},
		{"root became installed", func(value *TransactionCollectionInput) {
			value.Inventory.Tools = append(value.Inventory.Tools, formulaTool(
				"git", "2.51.0", value.ObservedAt,
			))
		}, "changed after Plan"},
		{"root conflicting version", func(value *TransactionCollectionInput) {
			value.Inventory.Tools = append(value.Inventory.Tools, formulaTool(
				"git", "2.50.0", value.ObservedAt,
			))
		}, "changed after Plan"},
		{"dependency conflicting version", func(value *TransactionCollectionInput) {
			value.Inventory.Tools[1].Installations[0].Version = "0.25"
			value.Inventory.Tools[1].Installations[0].NormalizedVersion = "0.25"
		}, "conflicting installed version"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validCollectionInput(t)
			test.mutate(&input)
			_, err := CollectTransactionFacts(input)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("CollectTransactionFacts() error = %v, want %q", err, test.message)
			}
		})
	}
}

func TestCollectTransactionFactsBoundsUntrustedSnapshotInputs(t *testing.T) {
	tooManyEnvironment := validCollectionInput(t)
	for index := 0; index <= maxConfigurationValues; index++ {
		tooManyEnvironment.Configuration.Environment["IGNORED_"+strings.Repeat("X", index)] = "value"
	}
	if _, err := CollectTransactionFacts(tooManyEnvironment); err == nil ||
		!strings.Contains(err.Error(), "too many entries") {
		t.Fatalf("large environment error = %v", err)
	}

	longValue := validCollectionInput(t)
	longValue.Configuration.Environment["HOMEBREW_NO_AUTO_UPDATE"] = strings.Repeat("x", maxConfigurationValue)
	if _, err := CollectTransactionFacts(longValue); err == nil ||
		!strings.Contains(err.Error(), "entry is invalid") {
		t.Fatalf("long environment value error = %v", err)
	}

	tooManyDependencies := validCollectionInput(t)
	tooManyDependencies.CatalogJSON = mutateCatalog(t, tooManyDependencies.CatalogJSON, func(formulas []map[string]any) []map[string]any {
		dependencies := make([]string, 0, maxTransactionFormulae)
		for index := 0; index < maxTransactionFormulae; index++ {
			name := "dependency-" + strings.Repeat("a", index/26) + string(rune('a'+index%26))
			dependencies = append(dependencies, name)
			formulas = append(formulas, catalogFormulaFixture(name, "1.0.0", []string{}, 'e'))
		}
		for _, formula := range formulas {
			if formula["name"] == "git" {
				formula["dependencies"] = dependencies
			}
		}
		return formulas
	})
	tooManyDependencies = rebindCollectionCatalog(t, tooManyDependencies)
	if _, err := CollectTransactionFacts(tooManyDependencies); err == nil ||
		!strings.Contains(err.Error(), "closure is too large") {
		t.Fatalf("large dependency closure error = %v", err)
	}
}

func TestCollectTransactionFactsUsesTargetVariationAndFormulaRevision(t *testing.T) {
	input := validCollectionInput(t)
	input.CatalogJSON = mutateCatalog(t, input.CatalogJSON, func(formulas []map[string]any) []map[string]any {
		for _, formula := range formulas {
			switch formula["name"] {
			case "git":
				formula["dependencies"] = []string{"gettext", "wrong-global"}
				formula["variations"] = map[string]any{
					"arm64_sequoia": map[string]any{
						"dependencies":             []string{"gettext"},
						"recommended_dependencies": []string{"openssl@3"},
					},
				}
			case "openssl@3":
				formula["revision"] = 1
			}
		}
		return formulas
	})
	input = rebindCollectionCatalog(t, input)
	for index := range input.Artifacts {
		if input.Artifacts[index].Formula == "openssl@3" {
			input.Artifacts[index].Version = "3.5.2_1"
		}
	}
	facts, err := CollectTransactionFacts(input)
	if err != nil {
		t.Fatalf("CollectTransactionFacts(variation) error = %v", err)
	}
	git := facts.Actions[1]
	if len(git.Dependencies) != 2 || git.Dependencies[0].Name != "gettext" ||
		git.Dependencies[1].Name != "openssl@3" || git.Dependencies[1].Version != "3.5.2_1" {
		t.Fatalf("variation dependencies = %#v", git.Dependencies)
	}
}

func TestCollectTransactionFactsDoesNotSatisfyDependencyFromAnotherArchitecture(t *testing.T) {
	input := validCollectionInput(t)
	input.Inventory.Tools[1].Installations[0].Architecture = inventory.ArchitectureAMD64
	input.Artifacts = append(input.Artifacts, BottleArtifact{
		Formula: "gettext", Version: "0.26", BottleTag: input.BottleTag,
		SHA256: transactionDigest('c'), DownloadBytes: 40,
	})
	facts, err := CollectTransactionFacts(input)
	if err != nil {
		t.Fatalf("CollectTransactionFacts(other architecture) error = %v", err)
	}
	if facts.Actions[0].Dependencies[0].State != FormulaInstallRequired ||
		facts.Actions[0].Dependencies[0].DownloadBytes != 40 {
		t.Fatalf("other-architecture dependency = %#v", facts.Actions[0].Dependencies[0])
	}
}

func TestCollectTransactionFactsRejectsUnsafeConfigurationKeyWithoutEchoingIt(t *testing.T) {
	input := validCollectionInput(t)
	key := "HOMEBREW_\x1b[31msecret-token"
	input.Configuration.Environment[key] = "private-value"
	_, err := CollectTransactionFacts(input)
	if err == nil || !strings.Contains(err.Error(), "entry is invalid") ||
		strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "private-value") ||
		strings.Contains(err.Error(), "\x1b") {
		t.Fatalf("unsafe configuration key error = %q", err)
	}
}

func TestCollectTransactionFactsIgnoresUnsupportedVersionsOutsideReachableClosure(t *testing.T) {
	input := validCollectionInput(t)
	input.CatalogJSON = mutateCatalog(t, input.CatalogJSON, func(formulas []map[string]any) []map[string]any {
		formulas = append(formulas, catalogFormulaFixture("unrelated", "r475", []string{}, 'e'))
		return formulas
	})
	input = rebindCollectionCatalog(t, input)
	if _, err := CollectTransactionFacts(input); err != nil {
		t.Fatalf("unrelated unsupported version blocked collection: %v", err)
	}

	reachable := validCollectionInput(t)
	reachable.CatalogJSON = mutateCatalog(t, reachable.CatalogJSON, func(formulas []map[string]any) []map[string]any {
		for _, formula := range formulas {
			if formula["name"] == "gettext" {
				formula["versions"] = map[string]any{"stable": "r475"}
			}
		}
		return formulas
	})
	reachable = rebindCollectionCatalog(t, reachable)
	if _, err := CollectTransactionFacts(reachable); err == nil ||
		!strings.Contains(err.Error(), "reachable formula") {
		t.Fatalf("unsupported reachable version error = %v", err)
	}
}

func TestMacOSBottleTagUsesOnlySupportedLockTargets(t *testing.T) {
	for _, test := range []struct {
		os      inventory.OperatingSystem
		version string
		arch    inventory.Architecture
		want    string
	}{
		{inventory.OSMacOS, "15.6", inventory.ArchitectureARM64, "arm64_sequoia"},
		{inventory.OSMacOS, "14.7.6", inventory.ArchitectureAMD64, "sonoma"},
		{inventory.OSMacOS, "26.0", inventory.ArchitectureARM64, "arm64_tahoe"},
	} {
		got, err := macOSBottleTag(lockfile.Target{OS: test.os, OSVersion: test.version, Architecture: test.arch})
		if err != nil || got != test.want {
			t.Fatalf("macOSBottleTag(%s/%s) = %q, %v", test.version, test.arch, got, err)
		}
	}
	if _, err := macOSBottleTag(lockfile.Target{
		OS: inventory.OSLinux, OSVersion: "15.6", Architecture: inventory.ArchitectureARM64,
	}); err == nil {
		t.Fatal("Linux target was accepted")
	}
	if _, err := macOSBottleTag(lockfile.Target{
		OS: inventory.OSMacOS, OSVersion: "99.0", Architecture: inventory.ArchitectureARM64,
	}); err == nil {
		t.Fatal("unsupported macOS target was accepted")
	}
}

func validCollectionInput(t *testing.T) TransactionCollectionInput {
	t.Helper()
	catalog := []map[string]any{
		catalogFormulaFixture("git", "2.51.0", []string{"gettext"}, 'a'),
		catalogFormulaFixture("cmake", "4.0.3", []string{"gettext", "openssl@3"}, 'b'),
		catalogFormulaFixture("gettext", "0.26", []string{}, 'c'),
		catalogFormulaFixture("openssl@3", "3.5.2", []string{}, 'd'),
	}
	catalogJSON, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	source := lockfile.Source{
		ID: "homebrew-core", Kind: lockfile.SourcePackageCatalog,
		URI:        "https://formulae.brew.sh/api/formula.json",
		SnapshotAt: collectionTestTime.Add(-2 * time.Hour), Digest: digestBytes(catalogJSON),
	}
	profileValue := profile.Profile{
		SchemaVersion: profile.SchemaVersion,
		Name:          "base",
		Modules:       []profile.Module{{ID: profile.ModuleBase}},
	}
	conditions := lockfile.Conditions{
		OS: inventory.OSMacOS,
		Architectures: []inventory.Architecture{
			inventory.ArchitectureARM64,
		},
	}
	items := []lockfile.Item{}
	for _, formula := range []struct{ capability, name, version string }{
		{"base.git", "git", "2.51.0"},
		{"base.cmake", "cmake", "4.0.3"},
	} {
		items = append(items, lockfile.Item{
			ID: formula.capability, Module: profile.ModuleBase,
			Capability: formula.capability, State: lockfile.StateInstallRequired,
			Implementation: &lockfile.Implementation{
				ToolID: "homebrew.formula." + formula.name, Manager: "homebrew",
				PackageKind: "formula", PackageID: formula.name,
				Version: formula.version, SourceID: source.ID, Conditions: conditions,
			},
			Observed: []lockfile.Observation{}, Reason: lockfile.ReasonInstallationRequired,
		})
	}
	lockValue, err := lockfile.Build(lockfile.BuildInput{
		GeneratedAt: collectionTestTime.Add(-2 * time.Minute), Profile: profileValue,
		Target: lockfile.Target{
			OS: inventory.OSMacOS, OSVersion: "15.6", Architecture: inventory.ArchitectureARM64,
		},
		Sources: []lockfile.Source{source}, Items: items,
	})
	if err != nil {
		t.Fatal(err)
	}
	planInventory := collectionInventory(collectionTestTime.Add(-90*time.Second), false)
	planValue, err := plan.BuildBaseInstall(plan.BaseInstallInput{
		Lock: lockValue, Inventory: planInventory, CreatedAt: collectionTestTime.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	return TransactionCollectionInput{
		ObservedAt:     collectionTestTime,
		Plan:           planValue,
		Lock:           lockValue,
		Inventory:      collectionInventory(collectionTestTime, true),
		ExecutablePath: "/opt/homebrew/bin/brew",
		ExecutableData: []byte("#!/bin/bash\n# fixed Homebrew fixture\n"),
		Configuration: ConfigurationSnapshot{
			Environment: map[string]string{
				"HOMEBREW_NO_ANALYTICS":                  "1",
				"HOMEBREW_NO_ASK":                        "1",
				"HOMEBREW_NO_AUTO_UPDATE":                "1",
				"HOMEBREW_NO_ENV_HINTS":                  "1",
				"HOMEBREW_NO_INSTALL_CLEANUP":            "1",
				"HOMEBREW_NO_INSTALL_UPGRADE":            "1",
				"HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK": "1",
			},
			System: []byte{}, Prefix: []byte{}, User: []byte{},
		},
		CatalogJSON: catalogJSON,
		BottleTag:   "arm64_sequoia",
		Artifacts: []BottleArtifact{
			{Formula: "git", Version: "2.51.0", BottleTag: "arm64_sequoia", SHA256: transactionDigest('a'), DownloadBytes: 10},
			{Formula: "cmake", Version: "4.0.3", BottleTag: "arm64_sequoia", SHA256: transactionDigest('b'), DownloadBytes: 20},
			{Formula: "openssl@3", Version: "3.5.2", BottleTag: "arm64_sequoia", SHA256: transactionDigest('d'), DownloadBytes: 30},
		},
	}
}

func collectionInventory(observedAt time.Time, includeDependency bool) inventory.Inventory {
	source := inventory.SourceMetadata{
		Kind: inventory.SourceFixture, Name: "collection fixture",
		CollectedAt: observedAt, Confidence: inventory.ConfidenceHigh,
	}
	tools := []inventory.Tool{{
		ID: "manager.homebrew", DisplayName: "Homebrew", Category: inventory.CategoryEcosystem,
		Installations: []inventory.Installation{{
			ID: "homebrew:manager", Version: "6.0.9", Path: "/opt/homebrew/bin/brew",
			Architecture: inventory.ArchitectureARM64, Manager: "homebrew",
			ActiveState: inventory.ActiveStateActive, DefaultState: inventory.DefaultStateDefault,
			InstallReason: inventory.InstallReasonDirect, Sources: []inventory.SourceMetadata{source},
		}},
	}}
	if includeDependency {
		tools = append(tools, formulaTool("gettext", "0.26", observedAt))
	}
	return inventory.Inventory{
		SchemaVersion: inventory.SchemaVersion, GeneratedAt: observedAt,
		System: inventory.System{
			OS: inventory.OSMacOS, OSVersion: "15.6", OSBuild: "24G90",
			Architecture:        inventory.ArchitectureARM64,
			ProcessArchitecture: inventory.ArchitectureARM64,
			TranslationState:    inventory.TranslationStateNative,
			Shell: inventory.Shell{
				LoginPath: "/bin/zsh", LoginName: "zsh",
				InvokingPath: "/bin/zsh", InvokingName: "zsh",
			},
			PathEntries: []inventory.PathEntry{}, Sources: []inventory.SourceMetadata{source},
		},
		Tools: tools, Findings: []inventory.Finding{},
	}
}

func formulaTool(name, version string, observedAt time.Time) inventory.Tool {
	source := inventory.SourceMetadata{
		Kind: inventory.SourceFixture, Name: "collection fixture",
		CollectedAt: observedAt, Confidence: inventory.ConfidenceHigh,
	}
	return inventory.Tool{
		ID: homebrewFormulaToolID(name), DisplayName: name, Category: inventory.CategoryUnknown,
		Installations: []inventory.Installation{{
			ID:      "homebrew:formula:" + name + ":" + version,
			Version: version, NormalizedVersion: version,
			Path:         "/opt/homebrew/Cellar/" + name + "/" + version,
			Architecture: inventory.ArchitectureARM64, Manager: "homebrew",
			ActiveState: inventory.ActiveStateActive, DefaultState: inventory.DefaultStateDefault,
			InstallReason: inventory.InstallReasonDependency, Sources: []inventory.SourceMetadata{source},
		}},
	}
}

func catalogFormulaFixture(name, version string, dependencies []string, digest byte) map[string]any {
	return map[string]any{
		"name": name, "full_name": name, "tap": "homebrew/core",
		"versions": map[string]any{"stable": version}, "revision": 0,
		"dependencies": dependencies, "recommended_dependencies": []string{},
		"bottle": map[string]any{
			"stable": map[string]any{
				"files": map[string]any{
					"arm64_sequoia": map[string]any{"sha256": strings.Repeat(string(digest), 64)},
				},
			},
		},
		"variations": map[string]any{}, "disabled": false,
	}
}

func mutateCatalog(
	t *testing.T,
	data []byte,
	mutate func([]map[string]any) []map[string]any,
) []byte {
	t.Helper()
	var formulas []map[string]any
	if err := json.Unmarshal(data, &formulas); err != nil {
		t.Fatal(err)
	}
	result, err := json.Marshal(mutate(formulas))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func rebindCollectionCatalog(t *testing.T, input TransactionCollectionInput) TransactionCollectionInput {
	t.Helper()
	profileValue := profile.Profile{
		SchemaVersion: profile.SchemaVersion, Name: input.Lock.Profile.Name,
		Modules: []profile.Module{{ID: profile.ModuleBase}},
	}
	sources := append([]lockfile.Source{}, input.Lock.Sources...)
	sources[0].Digest = digestBytes(input.CatalogJSON)
	lockValue, err := lockfile.Build(lockfile.BuildInput{
		GeneratedAt: input.Lock.GeneratedAt, Profile: profileValue,
		Target: input.Lock.Target, Sources: sources, Items: input.Lock.Items,
	})
	if err != nil {
		t.Fatal(err)
	}
	planInventory := collectionInventory(input.Plan.CreatedAt.Add(-30*time.Second), false)
	planValue, err := plan.BuildBaseInstall(plan.BaseInstallInput{
		Lock: lockValue, Inventory: planInventory, CreatedAt: input.Plan.CreatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	input.Lock = lockValue
	input.Plan = planValue
	return input
}
