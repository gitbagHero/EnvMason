package execution

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/lockfile"
	"github.com/gitbagHero/EnvMason/internal/plan"
	"github.com/gitbagHero/EnvMason/internal/profile"
)

func TestBaseInstallPlanRemainsUnregisteredBeforeHistoryOrProcess(t *testing.T) {
	createdAt := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	profileValue := profile.Profile{SchemaVersion: profile.SchemaVersion, Name: "base", Modules: []profile.Module{{ID: profile.ModuleBase, Variant: profile.VariantMinimal}}}
	source := lockfile.Source{ID: "homebrew-core", Kind: lockfile.SourcePackageCatalog, URI: "https://formulae.brew.sh/api/formula.json", SnapshotAt: createdAt.Add(-2 * time.Hour), Digest: isolationDigest('a')}
	implementation := &lockfile.Implementation{
		ToolID: "homebrew.formula.git", Manager: "homebrew", PackageKind: "formula",
		PackageID: "git", Version: "2.51.0", SourceID: source.ID,
		Conditions: lockfile.Conditions{OS: inventory.OSMacOS, Architectures: []inventory.Architecture{inventory.ArchitectureARM64}},
	}
	lock, err := lockfile.Build(lockfile.BuildInput{
		GeneratedAt: createdAt.Add(-time.Minute), Profile: profileValue,
		Target:  lockfile.Target{OS: inventory.OSMacOS, OSVersion: "15.6", Architecture: inventory.ArchitectureARM64},
		Sources: []lockfile.Source{source},
		Items: []lockfile.Item{
			{ID: "base.git", Module: profile.ModuleBase, Capability: "base.git", State: lockfile.StateInstallRequired, Implementation: implementation, Observed: []lockfile.Observation{}, Reason: lockfile.ReasonInstallationRequired},
			{ID: "base.ssh", Module: profile.ModuleBase, Capability: "base.ssh", State: lockfile.StateUnresolved, Observed: []lockfile.Observation{}, Reason: lockfile.ReasonUnsupportedCapability},
		},
	})
	if err != nil {
		t.Fatalf("Build(Lock) error = %v", err)
	}
	inventoryValue := inventory.Inventory{
		SchemaVersion: inventory.SchemaVersion, GeneratedAt: createdAt.Add(-30 * time.Second),
		System: inventory.System{OS: inventory.OSMacOS, OSVersion: "15.6", Architecture: inventory.ArchitectureARM64},
		Tools: []inventory.Tool{{ID: "manager.homebrew", Installations: []inventory.Installation{{
			ID: "homebrew:manager", Version: "6.0.9", Path: "/opt/homebrew/bin/brew", Manager: "homebrew",
			ActiveState: inventory.ActiveStateActive, DefaultState: inventory.DefaultStateDefault,
		}}}},
	}
	basePlan, err := plan.BuildBaseInstall(plan.BaseInstallInput{Lock: lock, Inventory: inventoryValue, CreatedAt: createdAt})
	if err != nil {
		t.Fatalf("BuildBaseInstall() error = %v", err)
	}
	registry, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	runner := &isolationRunner{}
	store := &isolationStore{}
	executor := Executor{Registry: registry, Runner: runner, Store: store, Now: func() time.Time { return createdAt.Add(time.Minute) }}
	_, err = executor.Execute(t.Context(), Request{
		Plan:         basePlan,
		Confirmation: ConfirmationReceipt{Scope: "plan", ConfirmedPlanID: basePlan.ID, ConfirmedAt: createdAt.Add(30 * time.Second)},
	})
	var executionErr *ExecutionError
	if !errors.As(err, &executionErr) || executionErr.Code != CodeActionUnregistered {
		t.Fatalf("Execute() error = %v", err)
	}
	if runner.calls != 0 || store.saves != 0 {
		t.Fatalf("unregistered Base Plan used runner/store: calls=%d saves=%d", runner.calls, store.saves)
	}
}

type isolationRunner struct{ calls int }

func (runner *isolationRunner) Run(context.Context, CommandSpec) ProcessResult {
	runner.calls++
	return ProcessResult{}
}

type isolationStore struct{ saves int }

func (store *isolationStore) Save(Record) error {
	store.saves++
	return nil
}

func isolationDigest(character byte) string {
	return "sha256:" + strings.Repeat(string(character), 64)
}
