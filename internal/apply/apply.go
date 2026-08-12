// Package apply prepares and executes the single I15 Node/NVM vertical slice.
package apply

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
	"github.com/gitbagHero/EnvMason/internal/assessment"
	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/plan"
	"github.com/gitbagHero/EnvMason/internal/report"
	"github.com/gitbagHero/EnvMason/internal/versiondata"
)

type Options struct {
	ToolID  string
	Version string
	Online  bool
}

type Prepared struct {
	Plan       plan.Plan
	baseline   nvm.Baseline
	policy     assessment.Policy
	versions   versiondata.Result
	nvmOptions nvm.Options
}

type Result struct {
	Record     execution.Record
	RecordPath string
}

type Service struct {
	GOOS        string
	Now         func() time.Time
	LookupEnv   func(string) (string, bool)
	Assess      func(context.Context, report.Options) (report.AssessmentResult, error)
	Scan        func(context.Context) (inventory.Inventory, error)
	Runner      execution.ProcessRunner
	HistoryRoot string
}

func DefaultService() Service {
	return Service{
		GOOS: runtime.GOOS, Now: time.Now, LookupEnv: os.LookupEnv,
		Assess: report.Assess, Scan: report.Scan, Runner: execution.OSRunner{},
	}
}

func ValidateOptions(options Options) error {
	if options.ToolID != "runtime.node" {
		return fmt.Errorf("unsupported apply tool %q", options.ToolID)
	}
	if strings.TrimSpace(options.Version) == "" {
		return errors.New("apply requires an exact --version")
	}
	if !options.Online {
		return errors.New("apply requires --online for fresh official version evidence")
	}
	return nil
}

func (service Service) Prepare(ctx context.Context, options Options) (Prepared, error) {
	if err := ValidateOptions(options); err != nil {
		return Prepared{}, err
	}
	if service.GOOS != "darwin" {
		return Prepared{}, fmt.Errorf("Node/NVM apply is unsupported on %s in I15", service.GOOS)
	}
	if service.Assess == nil || service.Scan == nil || service.Runner == nil || service.LookupEnv == nil {
		return Prepared{}, errors.New("apply service dependencies are incomplete")
	}
	assessmentResult, err := service.Assess(ctx, report.Options{Online: true})
	if err != nil {
		return Prepared{}, fmt.Errorf("assess Node.js target: %w", err)
	}
	policy := assessmentResult.Policy
	if policy.SchemaVersion == "" {
		policy = assessment.DefaultPolicy()
	}
	if policy.Tools == nil {
		policy.Tools = map[string]assessment.ToolPolicy{}
	}
	policy.Tools["runtime.node"] = assessment.ToolPolicy{Channel: assessment.ChannelLTS, Pin: options.Version}

	home := environment(service.LookupEnv, "HOME")
	directory := nvm.Locate(environment(service.LookupEnv, "NVM_DIR"), environment(service.LookupEnv, "XDG_CONFIG_HOME"), home)
	activeVersion, activeBinary := activeNode(assessmentResult.Inventory, home)
	if !filepath.IsAbs(activeBinary) {
		return Prepared{}, errors.New("active Node.js executable path is unavailable for post-install verification")
	}
	baseline, err := nvm.Inspect(directory, activeVersion)
	if err != nil {
		return Prepared{}, err
	}
	planInventory, err := privatePlanInventory(
		assessmentResult.Inventory, home, directory, environment(service.LookupEnv, "TMPDIR"),
	)
	if err != nil {
		return Prepared{}, err
	}
	value, err := plan.BuildExecutable(plan.BuildInput{
		Inventory: planInventory, Policy: policy, Versions: assessmentResult.Versions,
		CreatedAt: service.now(), TTL: plan.DefaultTTL,
	}, baseline.ScriptDigest, baseline.DefaultAliasDigest)
	if err != nil {
		return Prepared{}, err
	}
	nvmOptions := nvm.Options{
		Baseline: baseline, ActiveBinary: activeBinary, Home: home, Temporary: environment(service.LookupEnv, "TMPDIR"),
		ProxyValues: proxyEnvironment(service.LookupEnv),
	}
	return Prepared{Plan: value, baseline: baseline, policy: policy, versions: assessmentResult.Versions, nvmOptions: nvmOptions}, nil
}

func (service Service) Execute(ctx context.Context, prepared Prepared, receipt execution.ConfirmationReceipt) (Result, error) {
	if err := plan.Validate(prepared.Plan); err != nil {
		return Result{}, err
	}
	if service.GOOS != "darwin" || service.Scan == nil || service.Runner == nil {
		return Result{}, errors.New("I15 execution requires the macOS apply service")
	}
	current, err := service.Scan(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("re-scan environment before execution: %w", err)
	}
	planInventory, err := privatePlanInventory(
		current, prepared.nvmOptions.Home, prepared.baseline.Directory, prepared.nvmOptions.Temporary,
	)
	if err != nil {
		return Result{}, err
	}
	rebuilt, err := plan.BuildExecutable(plan.BuildInput{
		Inventory: planInventory, Policy: prepared.policy, Versions: prepared.versions,
		CreatedAt: prepared.Plan.CreatedAt, TTL: plan.DefaultTTL,
	}, prepared.baseline.ScriptDigest, prepared.baseline.DefaultAliasDigest)
	if err != nil {
		return Result{}, fmt.Errorf("revalidate executable Plan: %w", err)
	}
	if rebuilt.ID != prepared.Plan.ID {
		return Result{}, errors.New("environment or Plan content changed after review; generate a new Plan")
	}
	registry, err := execution.NewRegistry(nvm.Definition(prepared.nvmOptions))
	if err != nil {
		return Result{}, err
	}
	root := service.HistoryRoot
	if root == "" {
		root, err = execution.DefaultHistoryDirectory()
		if err != nil {
			return Result{}, err
		}
	}
	executor := execution.Executor{Registry: registry, Runner: service.Runner, Store: execution.FileStore{Root: root}, Now: service.Now}
	record, err := executor.Execute(ctx, execution.Request{Plan: prepared.Plan, Confirmation: receipt})
	result := Result{Record: record}
	if record.ID != "" {
		result.RecordPath = filepath.Join(root, record.ID+".json")
	}
	return result, err
}

// ReviewRecovery performs a read-only checkpoint review of one I15 NVM Node
// installation record. It never assesses online versions, builds a Plan,
// installs or removes a Node version, or writes operation history.
func (service Service) ReviewRecovery(
	ctx context.Context,
	operationID string,
) (execution.RecoveryRevalidation, error) {
	if service.GOOS != "darwin" || service.Scan == nil || service.LookupEnv == nil {
		return execution.RecoveryRevalidation{}, errors.New("NVM install recovery review requires the macOS apply read-only service")
	}
	root := service.HistoryRoot
	var err error
	if root == "" {
		root, err = execution.DefaultHistoryDirectory()
		if err != nil {
			return execution.RecoveryRevalidation{}, err
		}
	}
	source, err := (execution.FileStore{Root: root}).Load(operationID)
	if err != nil {
		return execution.RecoveryRevalidation{}, fmt.Errorf("load source NVM install operation: %w", err)
	}
	assessmentResult, err := execution.AssessRecovery(source)
	if err != nil {
		return execution.RecoveryRevalidation{}, err
	}
	if !reviewableInstallSource(source) {
		return execution.RecoveryRevalidation{}, errors.New("source operation is not an NVM install_version action")
	}
	hasChanged := false
	for _, candidate := range assessmentResult.Candidates {
		hasChanged = hasChanged || candidate.Evidence == execution.RecoveryEvidenceChanged
	}
	if !hasChanged {
		registry, registryErr := execution.NewRegistry()
		if registryErr != nil {
			return execution.RecoveryRevalidation{}, registryErr
		}
		return execution.RevalidateRecovery(ctx, source, registry)
	}

	value, err := service.Scan(ctx)
	if err != nil {
		return execution.RecoveryRevalidation{}, fmt.Errorf("scan before NVM install recovery review: %w", err)
	}
	home := environment(service.LookupEnv, "HOME")
	directory := nvm.Locate(environment(service.LookupEnv, "NVM_DIR"), environment(service.LookupEnv, "XDG_CONFIG_HOME"), home)
	activeVersion, activeBinary := activeNode(value, home)
	if !filepath.IsAbs(activeBinary) {
		return execution.RecoveryRevalidation{}, errors.New("active Node.js executable path is unavailable for recovery review")
	}
	baseline, err := nvm.Inspect(directory, activeVersion)
	if err != nil {
		return execution.RecoveryRevalidation{}, err
	}
	definition := nvm.Definition(nvm.Options{
		Baseline: baseline, ActiveBinary: activeBinary, Home: home,
		Temporary: environment(service.LookupEnv, "TMPDIR"),
	})
	registry, err := execution.NewRegistry(definition)
	if err != nil {
		return execution.RecoveryRevalidation{}, err
	}
	return execution.RevalidateRecovery(ctx, source, registry)
}

func reviewableInstallSource(source execution.Record) bool {
	if source.ConfirmedPlan == nil ||
		source.ConfirmedPlan.SchemaVersion != plan.ExecutableSchemaVersion ||
		len(source.ConfirmedPlan.Actions) != 1 || len(source.Steps) != 1 {
		return false
	}
	switch source.State {
	case execution.StateCompleted, execution.StateFailed, execution.StateTimedOut,
		execution.StateCancelled, execution.StateInterrupted:
	default:
		return false
	}
	action := source.ConfirmedPlan.Actions[0]
	step := source.Steps[0]
	return action.ID == step.ActionID && action.ToolID == "runtime.node" &&
		action.Operation == "install_version" && action.Adapter == "nvm" &&
		action.Risk == plan.RiskR2 && step.ToolID == action.ToolID &&
		step.Operation == action.Operation && step.Adapter == action.Adapter &&
		step.Risk == action.Risk
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}

func activeNode(value inventory.Inventory, home string) (string, string) {
	for _, tool := range value.Tools {
		if tool.ID != "runtime.node" {
			continue
		}
		for _, installation := range tool.Installations {
			if installation.ActiveState == inventory.ActiveStateActive {
				path := installation.Path
				if path == "$HOME" {
					path = home
				} else if strings.HasPrefix(path, "$HOME"+string(filepath.Separator)) {
					path = filepath.Join(home, strings.TrimPrefix(path, "$HOME"+string(filepath.Separator)))
				}
				return installation.Version, path
			}
		}
	}
	return "unknown", ""
}

func environment(lookup func(string) (string, bool), key string) string {
	value, _ := lookup(key)
	return value
}

func privatePlanInventory(value inventory.Inventory, home, nvmDirectory, temporary string) (inventory.Inventory, error) {
	if !filepath.IsAbs(temporary) {
		temporary = ""
	}
	return inventory.RedactInstallationPaths(value,
		inventory.PathRedaction{Root: nvmDirectory, Placeholder: "$NVM_DIR"},
		inventory.PathRedaction{Root: home, Placeholder: "$HOME"},
		inventory.PathRedaction{Root: temporary, Placeholder: "$TMPDIR"},
	)
}

func proxyEnvironment(lookup func(string) (string, bool)) map[string]string {
	result := map[string]string{}
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "all_proxy", "no_proxy"} {
		if value, ok := lookup(key); ok && value != "" {
			result[key] = value
		}
	}
	return result
}
