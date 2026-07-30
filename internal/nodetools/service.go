// Package nodetools prepares and executes the I17 Node ancillary-tool update
// slice through the shared deterministic execution core.
package nodetools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	adapter "github.com/gitbagHero/EnvMason/internal/adapter/nodetools"
	"github.com/gitbagHero/EnvMason/internal/adapter/nvm"
	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/plan"
	"github.com/gitbagHero/EnvMason/internal/report"
	versioncore "github.com/gitbagHero/EnvMason/internal/version"
)

type Options struct {
	NodeVersion     string
	NPMVersion      string
	CorepackVersion string
	PNPMVersion     string
}

type Prepared struct {
	Plan     plan.Plan
	baseline adapter.Baseline
	targets  adapter.Targets
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
	return Service{
		GOOS: runtime.GOOS, Now: time.Now, LookupEnv: os.LookupEnv,
		Scan: report.Scan, Runner: execution.OSRunner{},
	}
}

func ValidateOptions(options Options) error {
	if _, err := exactVersion(options.NodeVersion); err != nil {
		return errors.New("node-tools update requires an exact stable --node-version")
	}
	selected := 0
	for _, candidate := range []struct {
		name  string
		value string
	}{
		{name: "--npm", value: options.NPMVersion},
		{name: "--corepack", value: options.CorepackVersion},
		{name: "--pnpm", value: options.PNPMVersion},
	} {
		if candidate.value == "" {
			continue
		}
		selected++
		if _, err := exactVersion(candidate.value); err != nil {
			return fmt.Errorf("node-tools update requires an exact stable %s version", candidate.name)
		}
	}
	if selected == 0 {
		return errors.New("node-tools update requires at least one of --npm, --corepack or --pnpm")
	}
	return nil
}

func (service Service) Prepare(ctx context.Context, options Options) (Prepared, error) {
	if err := ValidateOptions(options); err != nil {
		return Prepared{}, err
	}
	if err := service.validate(); err != nil {
		return Prepared{}, err
	}
	value, err := service.Scan(ctx)
	if err != nil {
		return Prepared{}, fmt.Errorf("scan before Node tools Plan: %w", err)
	}
	baseline, adapterOptions, err := service.inspect(value, options.NodeVersion)
	if err != nil {
		return Prepared{}, err
	}
	targets := adapter.Targets{
		NPM: options.NPMVersion, Corepack: options.CorepackVersion, PNPM: options.PNPMVersion,
	}
	if err := resolveCorepackPNPM(ctx, &baseline, &adapterOptions, targets); err != nil {
		return Prepared{}, err
	}
	planTargets, err := selectedTargets(baseline, targets)
	if err != nil {
		return Prepared{}, err
	}
	planInventory, err := privatePlanInventory(
		value, adapterOptions.Home, baseline.NVM.Directory, adapterOptions.Temporary,
	)
	if err != nil {
		return Prepared{}, err
	}
	nodePlan, err := plan.BuildNodeTools(plan.NodeToolsInput{
		Inventory: planInventory, CreatedAt: service.now(), NodeVersion: baseline.NodeVersion,
		NVMScriptDigest: baseline.NVM.ScriptDigest, DefaultAliasDigest: baseline.NVM.DefaultAliasDigest,
		Targets: planTargets,
	})
	if err != nil {
		return Prepared{}, err
	}
	return Prepared{Plan: nodePlan, baseline: baseline, targets: targets}, nil
}

func (service Service) Execute(ctx context.Context, prepared Prepared, receipt execution.ConfirmationReceipt) (Result, error) {
	if err := plan.Validate(prepared.Plan); err != nil {
		return Result{}, err
	}
	if prepared.Plan.SchemaVersion != plan.ExecutableSchemaVersion || len(prepared.Plan.Actions) == 0 {
		return Result{}, errors.New("I17 execution requires a non-empty Plan 0.2.0")
	}
	if err := service.validate(); err != nil {
		return Result{}, err
	}
	currentInventory, err := service.Scan(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("re-scan environment before Node tools execution: %w", err)
	}
	current, currentOptions, err := service.inspect(currentInventory, prepared.baseline.NodeVersion)
	if err != nil {
		return Result{}, err
	}
	if err := resolveCorepackPNPM(ctx, &current, &currentOptions, prepared.targets); err != nil {
		return Result{}, err
	}
	currentTargets, err := selectedTargets(current, prepared.targets)
	if err != nil {
		return Result{}, err
	}
	planInventory, err := privatePlanInventory(
		currentInventory, currentOptions.Home, current.NVM.Directory, currentOptions.Temporary,
	)
	if err != nil {
		return Result{}, err
	}
	rebuilt, err := plan.BuildNodeTools(plan.NodeToolsInput{
		Inventory: planInventory, CreatedAt: prepared.Plan.CreatedAt, NodeVersion: current.NodeVersion,
		NVMScriptDigest: current.NVM.ScriptDigest, DefaultAliasDigest: current.NVM.DefaultAliasDigest,
		Targets: currentTargets,
	})
	if err != nil {
		return Result{}, fmt.Errorf("revalidate Node tools Plan: %w", err)
	}
	if rebuilt.ID != prepared.Plan.ID {
		return Result{}, errors.New("environment or Node tool state changed after review; generate a new Plan")
	}
	currentOptions.Targets = prepared.targets
	definitions := adapter.Definitions(currentOptions)
	registry, err := execution.NewRegistry(definitions...)
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
	executor := execution.Executor{
		Registry: registry, Runner: service.Runner, Store: execution.FileStore{Root: root}, Now: service.Now,
	}
	record, executeErr := executor.Execute(ctx, execution.Request{Plan: prepared.Plan, Confirmation: receipt})
	result := Result{Record: record}
	if record.ID != "" {
		result.RecordPath = filepath.Join(root, record.ID+".json")
	}
	return result, executeErr
}

func (service Service) inspect(value inventory.Inventory, nodeVersion string) (adapter.Baseline, adapter.Options, error) {
	home := environment(service.LookupEnv, "HOME")
	if !filepath.IsAbs(home) {
		return adapter.Baseline{}, adapter.Options{}, errors.New("absolute HOME is required for Node tools updates")
	}
	directory := nvm.Locate(environment(service.LookupEnv, "NVM_DIR"), environment(service.LookupEnv, "XDG_CONFIG_HOME"), home)
	activeVersion, activeBinary := activeNode(value, home)
	if !filepath.IsAbs(activeBinary) {
		return adapter.Baseline{}, adapter.Options{}, errors.New("active Node.js executable path is unavailable for post-update verification")
	}
	baseline, err := adapter.Inspect(directory, nodeVersion, activeVersion)
	if err != nil {
		return adapter.Baseline{}, adapter.Options{}, err
	}
	options := adapter.Options{
		Baseline: baseline, ActiveBinary: activeBinary, Home: home,
		Temporary:   environment(service.LookupEnv, "TMPDIR"),
		ProxyValues: proxyEnvironment(service.LookupEnv), Verifier: service.Runner,
	}
	return baseline, options, nil
}

func selectedTargets(baseline adapter.Baseline, targets adapter.Targets) ([]plan.NodeToolTarget, error) {
	result := []plan.NodeToolTarget{}
	for _, selected := range []struct {
		toolID string
		target string
		tool   adapter.Tool
	}{
		{toolID: plan.NodeToolNPM, target: targets.NPM, tool: baseline.NPM},
		{toolID: plan.NodeToolCorepack, target: targets.Corepack, tool: baseline.Corepack},
		{toolID: plan.NodeToolPNPM, target: targets.PNPM, tool: baseline.PNPM},
	} {
		if selected.target == "" {
			continue
		}
		if !selected.tool.Present || selected.tool.ControlDigest == "" {
			return nil, fmt.Errorf("%s is unavailable under the target NVM Node.js version", selected.toolID)
		}
		if _, err := exactVersion(selected.tool.Version); err != nil {
			return nil, fmt.Errorf("%s installed version is not an exact stable version", selected.toolID)
		}
		switch selected.toolID {
		case plan.NodeToolNPM:
			if selected.tool.Name != "npm" || selected.tool.Provider != plan.NodeToolProviderNPM {
				return nil, errors.New("npm is not owned by the target Node.js npm installation")
			}
		case plan.NodeToolCorepack:
			if selected.tool.Name != "corepack" || selected.tool.Provider != plan.NodeToolProviderNPM {
				return nil, errors.New("Corepack is not owned by the target Node.js npm installation")
			}
		case plan.NodeToolPNPM:
			if selected.tool.Provider != plan.NodeToolProviderNPM && selected.tool.Provider != plan.NodeToolProviderCorepack {
				return nil, errors.New("pnpm provider is unsupported; only npm or Corepack ownership is allowed")
			}
		}
		result = append(result, plan.NodeToolTarget{
			ToolID: selected.toolID, CurrentVersion: selected.tool.Version,
			TargetVersion: selected.target, Provider: selected.tool.Provider,
			ControlDigest: selected.tool.ControlDigest,
		})
	}
	return result, nil
}

func resolveCorepackPNPM(ctx context.Context, baseline *adapter.Baseline, options *adapter.Options, targets adapter.Targets) error {
	if targets.PNPM == "" || baseline.PNPM.Provider != plan.NodeToolProviderCorepack {
		return nil
	}
	version, err := adapter.CorepackPNPMVersion(ctx, *options)
	if err != nil {
		return fmt.Errorf("read target Node.js Corepack-managed pnpm version: %w", err)
	}
	baseline.PNPM.Version = version
	options.Baseline = *baseline
	return nil
}

func (service Service) validate() error {
	if service.GOOS != "darwin" {
		return fmt.Errorf("Node ancillary-tool updates are unsupported on %s in I17", service.GOOS)
	}
	if service.Scan == nil || service.Runner == nil || service.LookupEnv == nil {
		return errors.New("Node tools service dependencies are incomplete")
	}
	return nil
}

func (service Service) now() time.Time {
	if service.Now != nil {
		return service.Now().UTC()
	}
	return time.Now().UTC()
}

func exactVersion(raw string) (string, error) {
	parsed := versioncore.ParseSemVer(raw)
	if !parsed.Comparable || parsed.Normalized != raw || strings.ContainsAny(parsed.Normalized, "+-") {
		return "", errors.New("exact stable semantic version is required")
	}
	return parsed.Normalized, nil
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
