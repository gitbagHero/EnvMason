package baseinstall

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/lockfile"
	"github.com/gitbagHero/EnvMason/internal/plan"
	"github.com/gitbagHero/EnvMason/internal/profile"
)

var transactionTestTime = time.Date(2026, 8, 10, 9, 2, 0, 0, time.UTC)

func TestPrepareTransactionReviewSealsCompleteDeterministicClosure(t *testing.T) {
	input := validTransactionInput(t)
	before, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	first, err := PrepareTransactionReview(input)
	if err != nil {
		t.Fatalf("PrepareTransactionReview() error = %v", err)
	}
	after, err := json.Marshal(input)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("PrepareTransactionReview mutated its input")
	}
	if first.SchemaVersion != TransactionReviewSchemaVersion || first.Executable || first.Confirmable ||
		first.PlanID != input.Plan.ID || first.LockID != input.Lock.ID || len(first.Actions) != 2 {
		t.Fatalf("review = %#v", first)
	}
	if first.Actions[0].ActionID != "install-base-cmake" || first.Actions[1].ActionID != "install-base-git" ||
		first.Actions[0].Dependencies[0].Name != "gettext" || first.Actions[0].Dependencies[1].Name != "openssl@3" {
		t.Fatalf("canonical actions = %#v", first.Actions)
	}
	if first.Summary.Formulae != 4 || first.Summary.InstallRequired != 3 || first.Summary.Satisfied != 1 ||
		first.Summary.TotalDownloadBytes != 35 {
		t.Fatalf("summary = %#v", first.Summary)
	}
	if err := ValidateTransactionReview(first); err != nil {
		t.Fatalf("ValidateTransactionReview() error = %v", err)
	}

	secondInput := validTransactionInput(t)
	secondInput.Actions[0], secondInput.Actions[1] = secondInput.Actions[1], secondInput.Actions[0]
	for index := range secondInput.Actions {
		reverseFormulae(secondInput.Actions[index].Dependencies)
	}
	second, err := PrepareTransactionReview(secondInput)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("deterministic review = %#v / %#v, error = %v", first, second, err)
	}

	data, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal review: %v", err)
	}
	for _, forbidden := range []string{"/opt/homebrew", "formulae.brew.sh", "HOMEBREW_API_DOMAIN", "secret-value", `"command"`, `"confirmation"`} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("review leaked forbidden value %q: %s", forbidden, data)
		}
	}
}

func TestPrepareTransactionReviewSupportsEachSingleBaseFormula(t *testing.T) {
	for _, test := range []struct {
		name       string
		gitState   lockfile.ResolutionState
		cmakeState lockfile.ResolutionState
		wantAction string
	}{
		{"Git", lockfile.StateInstallRequired, lockfile.StateSatisfied, "install-base-git"},
		{"CMake", lockfile.StateSatisfied, lockfile.StateInstallRequired, "install-base-cmake"},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := transactionInputWithStates(t, test.gitState, test.cmakeState)
			review, err := PrepareTransactionReview(input)
			if err != nil {
				t.Fatalf("PrepareTransactionReview() error = %v", err)
			}
			if len(review.Actions) != 1 || review.Actions[0].ActionID != test.wantAction {
				t.Fatalf("single-formula review actions = %#v", review.Actions)
			}
		})
	}
}

func TestPrepareTransactionReviewRejectsDriftAndIncompletePreview(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*TransactionReviewInput)
		message string
	}{
		{"missing prepared time", func(value *TransactionReviewInput) { value.PreparedAt = time.Time{} }, "prepared_at"},
		{"expired Plan", func(value *TransactionReviewInput) { value.PreparedAt = value.Plan.ExpiresAt }, "not active"},
		{"tampered Plan", func(value *TransactionReviewInput) { value.Plan.ID = transactionDigest('f') }, "valid executable"},
		{"tampered Lock", func(value *TransactionReviewInput) { value.Lock.ID = transactionDigest('f') }, "invalid Lock"},
		{"Homebrew identity", func(value *TransactionReviewInput) { value.Baseline.HomebrewVersion = "6.1.0" }, "identity changed"},
		{"early observation", func(value *TransactionReviewInput) {
			value.Baseline.ObservedAt = value.Plan.CreatedAt.Add(-time.Second)
		}, "invalid or stale"},
		{"executable digest", func(value *TransactionReviewInput) { value.Baseline.ExecutableDigest = "sha256:nope" }, "invalid or stale"},
		{"configuration digest", func(value *TransactionReviewInput) { value.Baseline.ConfigurationDigest = "sha256:nope" }, "invalid or stale"},
		{"unsafe configuration", func(value *TransactionReviewInput) {
			value.Baseline.ConfigurationSafe = false
			value.Baseline.UnsafeConfigurationKeys = []string{"HOMEBREW_API_DOMAIN"}
		}, "not safe"},
		{"invalid unsafe key", func(value *TransactionReviewInput) {
			value.Baseline.UnsafeConfigurationKeys = []string{"secret=value"}
		}, "invalid key"},
		{"catalog digest", func(value *TransactionReviewInput) { value.Baseline.CatalogDigest = transactionDigest('e') }, "locked source"},
		{"catalog time", func(value *TransactionReviewInput) {
			value.Baseline.CatalogSnapshotAt = value.Baseline.CatalogSnapshotAt.Add(time.Second)
		}, "locked source"},
		{"missing action", func(value *TransactionReviewInput) { value.Actions = value.Actions[:1] }, "exactly cover"},
		{"extra action", func(value *TransactionReviewInput) { value.Actions = append(value.Actions, value.Actions[0]) }, "exactly cover"},
		{"duplicate action", func(value *TransactionReviewInput) {
			value.Actions[1].ActionID = value.Actions[0].ActionID
			value.Actions[1].Root.Name = value.Actions[0].Root.Name
		}, "duplicate action"},
		{"wrong root", func(value *TransactionReviewInput) { value.Actions[0].Root.Name = "wget" }, "unsupported"},
		{"wrong version", func(value *TransactionReviewInput) { value.Actions[0].Root.Version = "9.9.9" }, "locked Homebrew formula"},
		{"satisfied root", func(value *TransactionReviewInput) {
			value.Actions[0].Root.State = FormulaSatisfied
			value.Actions[0].Root.DownloadBytes = 0
		}, "root formula"},
		{"unknown size", func(value *TransactionReviewInput) { value.Actions[0].Root.DownloadBytes = 0 }, "known non-zero"},
		{"duplicate dependency", func(value *TransactionReviewInput) {
			value.Actions[0].Dependencies = append(value.Actions[0].Dependencies, value.Actions[0].Dependencies[0])
		}, "duplicate formula"},
		{"conflicting shared dependency", func(value *TransactionReviewInput) {
			value.Actions[1].Dependencies[1].Version = "0.0.1"
		}, "conflicting transaction facts"},
		{"overflow", func(value *TransactionReviewInput) {
			value.Actions[0].Dependencies = append(value.Actions[0].Dependencies,
				FormulaPreview{Name: "huge-a", Version: "1.0.0", State: FormulaInstallRequired, DownloadBytes: math.MaxInt64},
				FormulaPreview{Name: "huge-b", Version: "1.0.0", State: FormulaInstallRequired, DownloadBytes: 1})
		}, "overflows"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validTransactionInput(t)
			test.mutate(&input)
			_, err := PrepareTransactionReview(input)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("PrepareTransactionReview() error = %v, want containing %q", err, test.message)
			}
		})
	}
}

func TestValidateTransactionReviewRejectsTampering(t *testing.T) {
	review, err := PrepareTransactionReview(validTransactionInput(t))
	if err != nil {
		t.Fatalf("PrepareTransactionReview() error = %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*TransactionReview)
	}{
		{"ID", func(value *TransactionReview) { value.ID = transactionDigest('f') }},
		{"executable", func(value *TransactionReview) { value.Executable = true }},
		{"safe baseline", func(value *TransactionReview) { value.Baseline.ConfigurationSafe = false }},
		{"summary", func(value *TransactionReview) { value.Summary.TotalDownloadBytes++ }},
		{"target", func(value *TransactionReview) { value.Target.OSVersion = "14.0" }},
		{"formula", func(value *TransactionReview) { value.Actions[0].Root.Version = "9.9.9" }},
		{"order", func(value *TransactionReview) {
			value.Actions[0], value.Actions[1] = value.Actions[1], value.Actions[0]
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := cloneTransactionReview(review)
			test.mutate(&value)
			if err := ValidateTransactionReview(value); err == nil {
				t.Fatalf("ValidateTransactionReview(%s) succeeded", test.name)
			}
		})
	}
}

func TestValidateTransactionReviewBindingRejectsStructurallyValidPrePlanReview(t *testing.T) {
	input := validTransactionInput(t)
	review, err := PrepareTransactionReview(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTransactionReviewBinding(review, input.Plan, input.Lock); err != nil {
		t.Fatalf("valid binding error = %v", err)
	}
	beforePlan := cloneTransactionReview(review)
	beforePlan.PreparedAt = input.Plan.CreatedAt.Add(-time.Second)
	beforePlan.Baseline.ObservedAt = beforePlan.PreparedAt
	beforePlan.ID, err = transactionReviewID(beforePlan)
	validationErr := ValidateTransactionReview(beforePlan)
	if err != nil || validationErr != nil {
		t.Fatalf("build structurally valid pre-Plan Review: ID=%v validation=%v", err, validationErr)
	}
	if err := ValidateTransactionReviewBinding(beforePlan, input.Plan, input.Lock); err == nil {
		t.Fatal("structurally valid pre-Plan Review was accepted as bound")
	}
}

func validTransactionInput(t *testing.T) TransactionReviewInput {
	return transactionInputWithStates(t, lockfile.StateInstallRequired, lockfile.StateInstallRequired)
}

func transactionInputWithStates(t *testing.T, gitState, cmakeState lockfile.ResolutionState) TransactionReviewInput {
	t.Helper()
	profileValue := profile.Profile{SchemaVersion: profile.SchemaVersion, Name: "base", Modules: []profile.Module{{ID: profile.ModuleBase}}}
	source := lockfile.Source{
		ID: "homebrew-core", Kind: lockfile.SourcePackageCatalog,
		URI: "https://formulae.brew.sh/api/formula.json", SnapshotAt: transactionTestTime.Add(-2 * time.Hour), Digest: transactionDigest('a'),
	}
	conditions := lockfile.Conditions{OS: inventory.OSMacOS, Architectures: []inventory.Architecture{inventory.ArchitectureARM64}}
	item := func(id, packageID, version string, state lockfile.ResolutionState) lockfile.Item {
		value := lockfile.Item{
			ID: id, Module: profile.ModuleBase, Capability: id, State: state,
			Implementation: &lockfile.Implementation{
				ToolID: "homebrew.formula." + packageID, Manager: "homebrew", PackageKind: "formula",
				PackageID: packageID, Version: version, SourceID: source.ID, Conditions: conditions,
			},
			Observed: []lockfile.Observation{}, Reason: lockfile.ReasonInstallationRequired,
		}
		if state == lockfile.StateSatisfied {
			value.Observed = []lockfile.Observation{{Manager: "homebrew", Version: version}}
			value.Reason = lockfile.ReasonCompatibleInstallation
		}
		return value
	}
	lockValue, err := lockfile.Build(lockfile.BuildInput{
		GeneratedAt: transactionTestTime.Add(-2 * time.Minute), Profile: profileValue,
		Target:  lockfile.Target{OS: inventory.OSMacOS, OSVersion: "15.6", Architecture: inventory.ArchitectureARM64},
		Sources: []lockfile.Source{source},
		Items: []lockfile.Item{
			item("base.git", "git", "2.51.0", gitState),
			item("base.cmake", "cmake", "4.0.3", cmakeState),
		},
	})
	if err != nil {
		t.Fatalf("build fixture Lock: %v", err)
	}
	planValue, err := plan.BuildBaseInstall(plan.BaseInstallInput{
		Lock: lockValue, CreatedAt: transactionTestTime.Add(-time.Minute),
		Inventory: inventory.Inventory{
			SchemaVersion: inventory.SchemaVersion, GeneratedAt: transactionTestTime.Add(-90 * time.Second),
			System: inventory.System{OS: inventory.OSMacOS, OSVersion: "15.6", Architecture: inventory.ArchitectureARM64},
			Tools: []inventory.Tool{{ID: "manager.homebrew", Installations: []inventory.Installation{{
				ID: "homebrew:manager", Version: "6.0.9", Path: "/opt/homebrew/bin/brew", Manager: "homebrew",
				ActiveState: inventory.ActiveStateActive, DefaultState: inventory.DefaultStateDefault,
			}}}},
		},
	})
	if err != nil {
		t.Fatalf("build fixture Plan: %v", err)
	}
	shared := FormulaPreview{Name: "gettext", Version: "0.26", State: FormulaInstallRequired, DownloadBytes: 5}
	input := TransactionReviewInput{
		PreparedAt: transactionTestTime, Plan: planValue, Lock: lockValue,
		Baseline: TransactionBaseline{
			ObservedAt: transactionTestTime, InstallationID: "homebrew:manager", HomebrewVersion: "6.0.9",
			ExecutableDigest: transactionDigest('b'), ConfigurationDigest: transactionDigest('c'), ConfigurationSafe: true,
			CatalogSourceID: source.ID, CatalogDigest: source.Digest, CatalogSnapshotAt: source.SnapshotAt,
		},
		Actions: []ActionPreview{},
	}
	if gitState == lockfile.StateInstallRequired {
		input.Actions = append(input.Actions, ActionPreview{
			ActionID:     "install-base-git",
			Root:         FormulaPreview{Name: "git", Version: "2.51.0", State: FormulaInstallRequired, DownloadBytes: 10},
			Dependencies: []FormulaPreview{shared},
		})
	}
	if cmakeState == lockfile.StateInstallRequired {
		input.Actions = append(input.Actions, ActionPreview{
			ActionID: "install-base-cmake",
			Root:     FormulaPreview{Name: "cmake", Version: "4.0.3", State: FormulaInstallRequired, DownloadBytes: 20},
			Dependencies: []FormulaPreview{
				{Name: "openssl@3", Version: "3.5.2", State: FormulaSatisfied, DownloadBytes: 0},
				shared,
			},
		})
	}
	return input
}

func reverseFormulae(values []FormulaPreview) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func transactionDigest(character byte) string {
	return "sha256:" + strings.Repeat(string(character), 64)
}
