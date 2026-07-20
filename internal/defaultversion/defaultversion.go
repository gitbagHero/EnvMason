// Package defaultversion prepares and executes the single-action I16 NVM
// default change and its explicit recovery Plan.
package defaultversion

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gitbagHero/EnvMason/internal/adapter/nvm"
	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/plan"
	"github.com/gitbagHero/EnvMason/internal/report"
	versioncore "github.com/gitbagHero/EnvMason/internal/version"
)

type SetOptions struct {
	ToolID  string
	Version string
}

type RestoreOptions struct {
	OperationID string
}

type Prepared struct {
	Plan              plan.Plan
	baseline          nvm.Baseline
	desiredAlias      string
	desiredVersion    string
	sourceOperationID string
	sourcePlanID      string
	originalAlias     string
	originalAliasHash string
	originalVersion   string
}

type Result struct {
	Record     execution.Record
	RecordPath string
}

type Service struct {
	GOOS        string
	Now         func() time.Time
	LookupEnv   func(string) (string, bool)
	Scan        func(context.Context) (inventory.Inventory, error)
	Runner      execution.ProcessRunner
	HistoryRoot string
}

func DefaultService() Service {
	return Service{GOOS: runtime.GOOS, Now: time.Now, LookupEnv: os.LookupEnv, Scan: report.Scan, Runner: execution.OSRunner{}}
}

func ValidateSetOptions(options SetOptions) error {
	if options.ToolID != "runtime.node" {
		return fmt.Errorf("unsupported default tool %q", options.ToolID)
	}
	if _, err := normalizeVersion(options.Version); err != nil {
		return errors.New("default set requires an exact stable --version")
	}
	return nil
}

func ValidateRestoreOptions(options RestoreOptions) error {
	if !strings.HasPrefix(options.OperationID, "op-") || len(options.OperationID) != 35 {
		return errors.New("default restore requires a valid --operation ID")
	}
	for _, character := range strings.TrimPrefix(options.OperationID, "op-") {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return errors.New("default restore requires a valid --operation ID")
		}
	}
	return nil
}

func (service Service) PrepareSet(ctx context.Context, options SetOptions) (Prepared, error) {
	if err := ValidateSetOptions(options); err != nil {
		return Prepared{}, err
	}
	if err := service.validate(); err != nil {
		return Prepared{}, err
	}
	value, err := service.Scan(ctx)
	if err != nil {
		return Prepared{}, fmt.Errorf("scan before default Plan: %w", err)
	}
	baseline, _, err := service.inspect(value)
	if err != nil {
		return Prepared{}, err
	}
	target, _ := normalizeVersion(options.Version)
	defaultPlan, err := plan.BuildDefaultSet(plan.DefaultSetInput{
		Inventory: value, CreatedAt: service.now(), TargetVersion: target,
		ScriptDigest: baseline.ScriptDigest, CurrentAliasDigest: baseline.DefaultAliasDigest,
		CurrentAlias: baseline.DefaultAlias, CurrentDefaultVersion: baseline.DefaultVersion,
	})
	if err != nil {
		return Prepared{}, err
	}
	return Prepared{Plan: defaultPlan, baseline: baseline, desiredAlias: "v" + target, desiredVersion: target}, nil
}

func (service Service) PrepareRestore(ctx context.Context, options RestoreOptions) (Prepared, error) {
	if err := ValidateRestoreOptions(options); err != nil {
		return Prepared{}, err
	}
	if err := service.validate(); err != nil {
		return Prepared{}, err
	}
	root, err := service.historyRoot()
	if err != nil {
		return Prepared{}, err
	}
	source, err := (execution.FileStore{Root: root}).Load(options.OperationID)
	if err != nil {
		return Prepared{}, fmt.Errorf("load source default operation: %w", err)
	}
	before, after, err := recoverableSnapshots(source)
	if err != nil {
		return Prepared{}, err
	}
	value, err := service.Scan(ctx)
	if err != nil {
		return Prepared{}, fmt.Errorf("scan before recovery Plan: %w", err)
	}
	baseline, _, err := service.inspect(value)
	if err != nil {
		return Prepared{}, err
	}
	if baseline.DefaultAlias != after.Facts["default_alias"] || baseline.DefaultAliasDigest != after.Facts["default_alias_hash"] || baseline.DefaultVersion != after.Facts["default_version"] {
		return Prepared{}, errors.New("NVM default changed after the source operation; refusing to overwrite it with recovery")
	}
	restorePlan, err := plan.BuildDefaultRestore(plan.DefaultRestoreInput{
		Inventory: value, CreatedAt: service.now(), ScriptDigest: baseline.ScriptDigest,
		CurrentAliasDigest: baseline.DefaultAliasDigest, CurrentAlias: baseline.DefaultAlias, CurrentDefaultVersion: baseline.DefaultVersion,
		OriginalAliasDigest: before.Facts["default_alias_hash"], OriginalAlias: before.Facts["default_alias"], OriginalDefaultVersion: before.Facts["default_version"],
		SourceOperationID: source.ID, SourcePlanID: source.PlanID,
	})
	if err != nil {
		return Prepared{}, err
	}
	original, _ := normalizeVersion(before.Facts["default_version"])
	return Prepared{
		Plan: restorePlan, baseline: baseline,
		desiredAlias: before.Facts["default_alias"], desiredVersion: original,
		sourceOperationID: source.ID, sourcePlanID: source.PlanID,
		originalAlias: before.Facts["default_alias"], originalAliasHash: before.Facts["default_alias_hash"], originalVersion: before.Facts["default_version"],
	}, nil
}

func (service Service) Execute(ctx context.Context, prepared Prepared, receipt execution.ConfirmationReceipt) (Result, error) {
	if err := plan.Validate(prepared.Plan); err != nil {
		return Result{}, err
	}
	if prepared.Plan.SchemaVersion != plan.HighRiskExecutableSchemaVersion || len(prepared.Plan.Actions) != 1 {
		return Result{}, errors.New("I16 execution requires one Plan 0.3.0 action")
	}
	if err := service.validate(); err != nil {
		return Result{}, err
	}
	currentInventory, err := service.Scan(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("re-scan environment before default execution: %w", err)
	}
	currentBaseline, currentOptions, err := service.inspect(currentInventory)
	if err != nil {
		return Result{}, err
	}
	if currentBaseline.ScriptDigest != prepared.baseline.ScriptDigest || currentBaseline.DefaultAliasDigest != prepared.baseline.DefaultAliasDigest {
		return Result{}, errors.New("environment or NVM default changed after review; generate a new Plan")
	}
	var rebuilt plan.Plan
	switch prepared.Plan.Actions[0].Operation {
	case "set_default":
		rebuilt, err = plan.BuildDefaultSet(plan.DefaultSetInput{
			Inventory: currentInventory, CreatedAt: prepared.Plan.CreatedAt, TargetVersion: prepared.desiredVersion,
			ScriptDigest: prepared.baseline.ScriptDigest, CurrentAliasDigest: prepared.baseline.DefaultAliasDigest,
			CurrentAlias: prepared.baseline.DefaultAlias, CurrentDefaultVersion: prepared.baseline.DefaultVersion,
		})
	case "restore_default":
		rebuilt, err = plan.BuildDefaultRestore(plan.DefaultRestoreInput{
			Inventory: currentInventory, CreatedAt: prepared.Plan.CreatedAt, ScriptDigest: prepared.baseline.ScriptDigest,
			CurrentAliasDigest: prepared.baseline.DefaultAliasDigest, CurrentAlias: prepared.baseline.DefaultAlias, CurrentDefaultVersion: prepared.baseline.DefaultVersion,
			OriginalAliasDigest: prepared.originalAliasHash, OriginalAlias: prepared.originalAlias, OriginalDefaultVersion: prepared.originalVersion,
			SourceOperationID: prepared.sourceOperationID, SourcePlanID: prepared.sourcePlanID,
		})
	default:
		return Result{}, errors.New("unsupported I16 default operation")
	}
	if err != nil {
		return Result{}, fmt.Errorf("revalidate default Plan: %w", err)
	}
	if rebuilt.ID != prepared.Plan.ID {
		return Result{}, errors.New("environment or Plan content changed after review; generate a new Plan")
	}
	defaultOptions := nvm.DefaultOptions{Options: currentOptions, DesiredAlias: prepared.desiredAlias, DesiredVersion: prepared.desiredVersion}
	var definition execution.Definition
	if prepared.Plan.Actions[0].Operation == "set_default" {
		definition = nvm.SetDefaultDefinition(defaultOptions)
	} else {
		definition = nvm.RestoreDefaultDefinition(defaultOptions)
	}
	registry, err := execution.NewRegistry(definition)
	if err != nil {
		return Result{}, err
	}
	root, err := service.historyRoot()
	if err != nil {
		return Result{}, err
	}
	executor := execution.Executor{Registry: registry, Runner: service.Runner, Store: execution.FileStore{Root: root}, Now: service.Now}
	record, executeErr := executor.Execute(ctx, execution.Request{Plan: prepared.Plan, Confirmation: receipt})
	result := Result{Record: record}
	if record.ID != "" {
		result.RecordPath = filepath.Join(root, record.ID+".json")
	}
	return result, executeErr
}

func (service Service) inspect(value inventory.Inventory) (nvm.Baseline, nvm.Options, error) {
	home := environment(service.LookupEnv, "HOME")
	directory := nvm.Locate(environment(service.LookupEnv, "NVM_DIR"), environment(service.LookupEnv, "XDG_CONFIG_HOME"), home)
	activeVersion, activeBinary := activeNode(value, home)
	if !filepath.IsAbs(activeBinary) {
		return nvm.Baseline{}, nvm.Options{}, errors.New("active Node.js executable path is unavailable for default verification")
	}
	baseline, err := nvm.InspectDefault(directory, activeVersion)
	if err != nil {
		return nvm.Baseline{}, nvm.Options{}, err
	}
	return baseline, nvm.Options{Baseline: baseline, ActiveBinary: activeBinary, Home: home, Temporary: environment(service.LookupEnv, "TMPDIR")}, nil
}

func (service Service) validate() error {
	if service.GOOS != "darwin" {
		return fmt.Errorf("Node/NVM default changes are unsupported on %s in I16", service.GOOS)
	}
	if service.Scan == nil || service.Runner == nil || service.LookupEnv == nil {
		return errors.New("default service dependencies are incomplete")
	}
	return nil
}

func (service Service) historyRoot() (string, error) {
	if service.HistoryRoot != "" {
		return service.HistoryRoot, nil
	}
	return execution.DefaultHistoryDirectory()
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}

func recoverableSnapshots(source execution.Record) (execution.Snapshot, execution.Snapshot, error) {
	if source.PlanSchemaVersion != plan.HighRiskExecutableSchemaVersion || source.Confirmation.ConfirmedPlanID != source.PlanID ||
		len(source.Steps) != 1 || source.Steps[0].ToolID != "runtime.node" || source.Steps[0].Operation != "set_default" ||
		source.Steps[0].Adapter != "nvm" || source.Steps[0].Risk != plan.RiskR3 || source.Steps[0].Before == nil || source.Steps[0].After == nil {
		return execution.Snapshot{}, execution.Snapshot{}, errors.New("source operation is not a recoverable I16 set-default action")
	}
	before, after := *source.Steps[0].Before, *source.Steps[0].After
	for _, snapshot := range []execution.Snapshot{before, after} {
		for _, key := range []string{"default_alias", "default_alias_hash", "default_version"} {
			if snapshot.Facts[key] == "" {
				return execution.Snapshot{}, execution.Snapshot{}, errors.New("source operation does not contain complete default-alias snapshots")
			}
		}
	}
	if before.Facts["default_alias_hash"] == after.Facts["default_alias_hash"] {
		return execution.Snapshot{}, execution.Snapshot{}, errors.New("source operation did not change the NVM default alias")
	}
	return before, after, nil
}

func activeNode(value inventory.Inventory, home string) (string, string) {
	for _, tool := range value.Tools {
		if tool.ID != "runtime.node" {
			continue
		}
		for _, installation := range tool.Installations {
			if installation.ActiveState != inventory.ActiveStateActive {
				continue
			}
			path := installation.Path
			if path == "$HOME" {
				path = home
			} else if strings.HasPrefix(path, "$HOME"+string(filepath.Separator)) {
				path = filepath.Join(home, strings.TrimPrefix(path, "$HOME"+string(filepath.Separator)))
			}
			return installation.Version, path
		}
	}
	return "unknown", ""
}

func environment(lookup func(string) (string, bool), key string) string {
	value, _ := lookup(key)
	return value
}

func normalizeVersion(raw string) (string, error) {
	if strings.TrimSpace(raw) != raw {
		return "", errors.New("exact stable version required")
	}
	parsed := versioncore.ParseSemVer(raw)
	if !parsed.Comparable || strings.Contains(parsed.Normalized, "-") || strings.Contains(parsed.Normalized, "+") {
		return "", errors.New("exact stable version required")
	}
	return parsed.Normalized, nil
}
