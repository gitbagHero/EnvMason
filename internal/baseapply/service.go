// Package baseapply binds one confirmed I21-C review to the fixed I21-D
// Homebrew write adapter. It is internal and exposes no CLI or public schema.
package baseapply

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"time"

	"github.com/gitbagHero/EnvMason/internal/adapter/homebrewinstall"
	"github.com/gitbagHero/EnvMason/internal/baseinstall"
	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/lockfile"
	"github.com/gitbagHero/EnvMason/internal/plan"
)

const ConfirmationScope = "plan+homebrew-review"

type Prepared struct {
	CandidatePlan plan.Plan
	Plan          plan.Plan
	Lock          lockfile.Lock
	Review        baseinstall.TransactionReview
}

// FreshSnapshots are explicit current facts. Execute never discovers files,
// configuration or catalog content outside this structure.
type FreshSnapshots struct {
	Inventory      inventory.Inventory
	ExecutablePath string
	ExecutableData []byte
	Configuration  baseinstall.ConfigurationSnapshot
	CatalogJSON    []byte
	BottleTag      string
	Artifacts      []baseinstall.BottleArtifact
}

type Confirmation struct {
	Scope             string
	ConfirmedPlanID   string
	ConfirmedReviewID string
	ConfirmedAt       time.Time
}

type Service struct {
	GOOS           string
	Now            func() time.Time
	Runner         execution.ProcessRunner
	Verifier       execution.ProcessRunner
	Store          execution.RecordStore
	NewOperationID func() (string, error)
}

// Prepare derives the final execution Plan whose ID includes the reviewed
// transaction ID. The caller must show and confirm this returned Plan.
func Prepare(
	candidate plan.Plan,
	lock lockfile.Lock,
	review baseinstall.TransactionReview,
) (Prepared, error) {
	if baseinstall.ValidateTransactionReviewBinding(review, candidate, lock) != nil {
		return Prepared{}, errors.New("prepare Base Homebrew execution: Plan, Lock and Review are not bound")
	}
	bound, err := plan.BindBaseTransactionReview(candidate, review.ID)
	if err != nil {
		return Prepared{}, err
	}
	return Prepared{CandidatePlan: candidate, Plan: bound, Lock: lock, Review: review}, nil
}

// Execute re-collects the reviewed facts before delegating to the existing
// deterministic executor. A changed Plan, Review, environment, catalog,
// bottle closure or installed formula state stops before history or writes.
func (service Service) Execute(
	ctx context.Context,
	prepared Prepared,
	snapshots FreshSnapshots,
	confirmation Confirmation,
) (execution.Record, error) {
	now := service.now()
	if err := validateConfirmation(prepared, confirmation, now); err != nil {
		return execution.Record{}, err
	}
	if service.goos() != "darwin" {
		return execution.Record{}, errors.New("Base Homebrew execution is supported only on macOS")
	}
	if service.Runner == nil || service.Verifier == nil || service.Store == nil {
		return execution.Record{}, errors.New("Base Homebrew execution service dependencies are incomplete")
	}
	facts, err := baseinstall.CollectTransactionFacts(baseinstall.TransactionCollectionInput{
		ObservedAt: now, Plan: prepared.CandidatePlan, Lock: prepared.Lock,
		Inventory: snapshots.Inventory, ExecutablePath: snapshots.ExecutablePath,
		ExecutableData: snapshots.ExecutableData, Configuration: snapshots.Configuration,
		CatalogJSON: snapshots.CatalogJSON, BottleTag: snapshots.BottleTag,
		Artifacts: snapshots.Artifacts,
	})
	if err != nil {
		return execution.Record{}, errors.New("recollect Homebrew transaction before execution: current facts are unsafe or invalid")
	}
	fresh, err := baseinstall.PrepareTransactionReview(baseinstall.TransactionReviewInput{
		PreparedAt: now, Plan: prepared.CandidatePlan, Lock: prepared.Lock,
		Baseline: facts.Baseline, Actions: facts.Actions,
	})
	if err != nil || !sameExecutionFacts(prepared.Review, fresh) {
		return execution.Record{}, errors.New("Homebrew transaction changed after confirmation; prepare and confirm a new review")
	}
	definitions, err := homebrewinstall.InstallDefinitions(homebrewinstall.InstallOptions{
		CandidatePlan: prepared.CandidatePlan, Plan: prepared.Plan, Review: prepared.Review,
		BrewPath: snapshots.ExecutablePath, ExecutableData: snapshots.ExecutableData,
		Configuration: snapshots.Configuration, Verifier: service.Verifier,
	})
	if err != nil {
		return execution.Record{}, err
	}
	registry, err := execution.NewRegistry(definitions...)
	if err != nil {
		return execution.Record{}, err
	}
	executor := execution.Executor{
		Registry: registry, Runner: service.Runner, Store: service.Store,
		Now: service.Now, NewOperationID: service.NewOperationID,
	}
	return executor.Execute(ctx, execution.Request{
		Plan: prepared.Plan,
		Confirmation: execution.ConfirmationReceipt{
			Scope: "plan", ConfirmedPlanID: confirmation.ConfirmedPlanID,
			ConfirmedAt: confirmation.ConfirmedAt,
		},
	})
}

func validateConfirmation(prepared Prepared, confirmation Confirmation, now time.Time) error {
	if plan.Validate(prepared.Plan) != nil ||
		baseinstall.ValidateTransactionReviewBinding(prepared.Review, prepared.CandidatePlan, prepared.Lock) != nil {
		return errors.New("Base Homebrew execution requires one valid bound Plan, Lock and Review")
	}
	want, err := plan.BindBaseTransactionReview(prepared.CandidatePlan, prepared.Review.ID)
	if err != nil || !reflect.DeepEqual(want, prepared.Plan) {
		return errors.New("Base Homebrew execution Plan does not match the reviewed candidate")
	}
	if confirmation.Scope != ConfirmationScope ||
		confirmation.ConfirmedPlanID != prepared.Plan.ID ||
		confirmation.ConfirmedReviewID != prepared.Review.ID ||
		confirmation.ConfirmedAt.Before(prepared.Review.PreparedAt) ||
		confirmation.ConfirmedAt.Before(prepared.Plan.CreatedAt) ||
		confirmation.ConfirmedAt.After(now) {
		return errors.New("current confirmation is not bound to both the Plan and Homebrew Review")
	}
	return nil
}

func sameExecutionFacts(confirmed, fresh baseinstall.TransactionReview) bool {
	if confirmed.PlanID != fresh.PlanID || confirmed.LockID != fresh.LockID ||
		!reflect.DeepEqual(confirmed.Target, fresh.Target) ||
		!reflect.DeepEqual(confirmed.Actions, fresh.Actions) ||
		confirmed.Summary != fresh.Summary {
		return false
	}
	left, right := confirmed.Baseline, fresh.Baseline
	left.ObservedAt = time.Time{}
	right.ObservedAt = time.Time{}
	return reflect.DeepEqual(left, right)
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}

func (service Service) goos() string {
	if service.GOOS != "" {
		return service.GOOS
	}
	return runtime.GOOS
}
