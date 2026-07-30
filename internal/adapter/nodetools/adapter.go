// Package nodetools contains the fixed I17 adapters for Node-scoped npm,
// Corepack and pnpm updates. It never evaluates shell text, project metadata
// or user npm configuration.
package nodetools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gitbagHero/EnvMason/internal/adapter/nvm"
	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/plan"
	versioncore "github.com/gitbagHero/EnvMason/internal/version"
)

const (
	RegistryURL       = "https://registry.npmjs.org"
	ActionTimeout     = 10 * time.Minute
	VerificationLimit = 10 * time.Second
	maximumPackage    = 256 << 10
)

type Tool struct {
	Name          string
	Version       string
	Provider      string
	Executable    string
	ResolvedPath  string
	PackageRoot   string
	ControlDigest string
	Present       bool
}

type Baseline struct {
	NVM         nvm.Baseline
	NodeVersion string
	NodeRoot    string
	NodeBinary  string
	NPM         Tool
	Corepack    Tool
	PNPM        Tool
}

type Targets struct {
	NPM      string
	Corepack string
	PNPM     string
}

type Options struct {
	Baseline     Baseline
	Targets      Targets
	ActiveBinary string
	Home         string
	Temporary    string
	ProxyValues  map[string]string
	Verifier     execution.ProcessRunner
}

type packageMetadata struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Inspect binds the update surface to one already-installed NVM Node version.
func Inspect(directory, nodeVersion, activeVersion string) (Baseline, error) {
	target, err := exactVersion(nodeVersion)
	if err != nil {
		return Baseline{}, errors.New("inspect Node tools: exact stable Node.js version is required")
	}
	nvmBaseline, err := nvm.InspectDefault(directory, activeVersion)
	if err != nil {
		return Baseline{}, err
	}
	installed := false
	for _, value := range nvmBaseline.InstalledVersions {
		if strings.TrimPrefix(value, "v") == target {
			installed = true
			break
		}
	}
	if !installed {
		return Baseline{}, errors.New("inspect Node tools: target is not an installed NVM Node.js version")
	}
	root := filepath.Join(nvmBaseline.Directory, "versions", "node", "v"+target)
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return Baseline{}, fmt.Errorf("inspect Node tools: target Node.js root: %w", err)
	}
	root = filepath.Clean(canonicalRoot)
	canonicalNVM, err := filepath.EvalSymlinks(nvmBaseline.Directory)
	if err != nil || !inside(canonicalNVM, root) {
		return Baseline{}, errors.New("inspect Node tools: target Node.js root escapes the NVM directory")
	}
	node := filepath.Join(root, "bin", "node")
	if err := requireExecutable(node); err != nil {
		return Baseline{}, fmt.Errorf("inspect Node tools: target Node.js executable: %w", err)
	}
	npm, err := inspectTool(root, "npm")
	if err != nil {
		return Baseline{}, err
	}
	corepack, err := inspectTool(root, "corepack")
	if err != nil {
		return Baseline{}, err
	}
	pnpm, err := inspectTool(root, "pnpm")
	if err != nil {
		return Baseline{}, err
	}
	if npm.Present && npm.Name == "npm" {
		npm.Provider = plan.NodeToolProviderNPM
	}
	if corepack.Present && corepack.Name == "corepack" {
		corepack.Provider = plan.NodeToolProviderNPM
	}
	if pnpm.Present {
		switch pnpm.Name {
		case "pnpm":
			pnpm.Provider = plan.NodeToolProviderNPM
		case "corepack":
			pnpm.Provider = plan.NodeToolProviderCorepack
			pnpm.Version = "unknown"
		}
	}
	return Baseline{
		NVM: nvmBaseline, NodeVersion: target, NodeRoot: root, NodeBinary: node,
		NPM: npm, Corepack: corepack, PNPM: pnpm,
	}, nil
}

func Definitions(options Options) []execution.Definition {
	return []execution.Definition{
		definition(plan.NodeToolNPM, plan.NodeToolProviderNPM, options),
		definition(plan.NodeToolCorepack, plan.NodeToolProviderNPM, options),
		definition(plan.NodeToolPNPM, plan.NodeToolProviderNPM, options),
		definition(plan.NodeToolPNPM, plan.NodeToolProviderCorepack, options),
	}
}

// CorepackPNPMVersion reads the exact cached Known Good pnpm version through
// the target Node's proxy without allowing project selection or network use.
func CorepackPNPMVersion(ctx context.Context, options Options) (string, error) {
	if !options.Baseline.PNPM.Present || options.Baseline.PNPM.Provider != plan.NodeToolProviderCorepack {
		return "", errors.New("target pnpm is not a Corepack proxy")
	}
	return queryVersion(ctx, options, options.Baseline.PNPM.Executable)
}

func definition(toolID, provider string, options Options) execution.Definition {
	return execution.Definition{
		Key:         execution.ActionKey{ToolID: toolID, Operation: "update_version", Adapter: provider},
		MinimumRisk: plan.RiskR2,
		Build: func(action plan.Action) (execution.CommandSpec, error) {
			target, err := exactVersion(action.TargetVersion)
			if err != nil || action.TargetVersion != desiredTarget(options.Targets, toolID) {
				return execution.CommandSpec{}, errors.New("Node tool action does not match its registered exact target")
			}
			ownedTool := baselineTool(options.Baseline, toolID)
			if !ownedTool.Present || ownedTool.Provider != provider {
				return execution.CommandSpec{}, errors.New("Node tool action does not match its registered provider")
			}
			var executable string
			var arguments []string
			switch {
			case provider == plan.NodeToolProviderNPM:
				if !options.Baseline.NPM.Present || options.Baseline.NPM.Provider != plan.NodeToolProviderNPM {
					return execution.CommandSpec{}, errors.New("target Node.js npm provider is unavailable")
				}
				executable = options.Baseline.NPM.Executable
				arguments = []string{
					"install", "--global", "--ignore-scripts", "--no-audit", "--no-fund",
					"--registry=" + RegistryURL, "--prefix=" + options.Baseline.NodeRoot,
					packageName(toolID) + "@" + target,
				}
			case toolID == plan.NodeToolPNPM && provider == plan.NodeToolProviderCorepack:
				if !options.Baseline.Corepack.Present || options.Baseline.Corepack.Provider != plan.NodeToolProviderNPM {
					return execution.CommandSpec{}, errors.New("target Node.js Corepack provider is unavailable")
				}
				executable = options.Baseline.Corepack.Executable
				arguments = []string{"install", "--global", "pnpm@" + target}
			default:
				return execution.CommandSpec{}, errors.New("unsupported Node tool provider")
			}
			environment, sensitive := controlledEnvironment(options, true)
			return execution.CommandSpec{
				Executable: executable, Args: arguments, Environment: environment,
				Directory: temporaryDirectory(options.Temporary), Timeout: ActionTimeout,
				SensitiveValues: sensitive, TerminateTree: true,
			}, nil
		},
		Preflight: func(ctx context.Context, _ plan.Action) error {
			current, err := Inspect(options.Baseline.NVM.Directory, options.Baseline.NodeVersion, options.Baseline.NVM.ActiveVersion)
			if err != nil {
				return err
			}
			if current.NVM.ScriptDigest != options.Baseline.NVM.ScriptDigest ||
				current.NVM.DefaultAliasDigest != options.Baseline.NVM.DefaultAliasDigest {
				return errors.New("NVM control files changed after Plan creation")
			}
			expected := baselineTool(options.Baseline, toolID)
			actual := baselineTool(current, toolID)
			if toolID == plan.NodeToolPNPM && provider == plan.NodeToolProviderCorepack {
				actual.Version, err = queryVersion(ctx, options, actual.Executable)
				if err != nil || actual.Version != expected.Version {
					return errors.New("Corepack-managed pnpm version changed after Plan creation")
				}
			}
			if toolID == plan.NodeToolPNPM && provider == plan.NodeToolProviderCorepack && options.Targets.Corepack != "" {
				if actual.Provider != provider || current.Corepack.Version != options.Targets.Corepack {
					return errors.New("Corepack provider state does not match the completed dependency")
				}
				return nil
			}
			if !sameTool(actual, expected) || actual.Provider != provider {
				return errors.New("Node tool provider state changed after Plan creation")
			}
			return nil
		},
		Capture: func(ctx context.Context, _ plan.Action) (execution.Snapshot, error) {
			current, err := Inspect(options.Baseline.NVM.Directory, options.Baseline.NodeVersion, options.Baseline.NVM.ActiveVersion)
			if err != nil {
				return execution.Snapshot{}, err
			}
			if toolID == plan.NodeToolPNPM && provider == plan.NodeToolProviderCorepack {
				current.PNPM.Version, err = queryVersion(ctx, options, current.PNPM.Executable)
				if err != nil {
					return execution.Snapshot{}, err
				}
			}
			return capture(current)
		},
		Satisfied: func(ctx context.Context, action plan.Action) (bool, error) {
			current, err := Inspect(options.Baseline.NVM.Directory, options.Baseline.NodeVersion, options.Baseline.NVM.ActiveVersion)
			if err != nil {
				return false, err
			}
			actual := baselineTool(current, toolID)
			if actual.Provider != provider {
				return false, errors.New("Node tool provider changed before idempotency check")
			}
			if provider == plan.NodeToolProviderCorepack {
				version, err := queryVersion(ctx, options, actual.Executable)
				return err == nil && version == action.TargetVersion, err
			}
			return actual.Version == action.TargetVersion, nil
		},
		Verify: func(ctx context.Context, action plan.Action, _ execution.ProcessResult) error {
			current, err := Inspect(options.Baseline.NVM.Directory, options.Baseline.NodeVersion, options.Baseline.NVM.ActiveVersion)
			if err != nil {
				return err
			}
			if current.NVM.ScriptDigest != options.Baseline.NVM.ScriptDigest ||
				current.NVM.DefaultAliasDigest != options.Baseline.NVM.DefaultAliasDigest ||
				current.NVM.ActiveVersion != options.Baseline.NVM.ActiveVersion {
				return errors.New("NVM ownership, default or active version changed during Node tool update")
			}
			actual := baselineTool(current, toolID)
			if !actual.Present || actual.Provider != provider {
				return errors.New("updated Node tool is not owned by the planned provider")
			}
			if provider != plan.NodeToolProviderCorepack && actual.Version != action.TargetVersion {
				return errors.New("updated Node tool package version did not match the Plan")
			}
			got, err := queryVersion(ctx, options, actual.Executable)
			if err != nil || got != action.TargetVersion {
				return errors.New("updated Node tool executable version did not match the Plan")
			}
			nodeVersion, err := queryVersion(ctx, options, current.NodeBinary)
			if err != nil || nodeVersion != options.Baseline.NodeVersion {
				return errors.New("target Node.js version changed during Node tool update")
			}
			if options.ActiveBinary != "" {
				activeVersion, activeErr := queryVersion(ctx, options, options.ActiveBinary)
				expectedActive := strings.TrimPrefix(options.Baseline.NVM.ActiveVersion, "v")
				if activeErr != nil || activeVersion != expectedActive {
					return errors.New("active Node.js version changed during Node tool update")
				}
			}
			return nil
		},
		RevalidateCheckpoint: func(ctx context.Context, action plan.Action, recorded execution.Snapshot) (execution.Snapshot, error) {
			return revalidateCheckpoint(ctx, toolID, provider, options, action, recorded)
		},
	}
}

func inspectTool(root, command string) (Tool, error) {
	executable := filepath.Join(root, "bin", command)
	info, err := os.Lstat(executable)
	if errors.Is(err, fs.ErrNotExist) {
		return Tool{Name: command, Executable: executable}, nil
	}
	if err != nil {
		return Tool{}, fmt.Errorf("inspect Node tools: %s: %w", command, err)
	}
	if info.Mode()&os.ModeSymlink == 0 && (!info.Mode().IsRegular() || info.Mode()&0o111 == 0) {
		return Tool{}, fmt.Errorf("inspect Node tools: %s is not a regular executable or symlink", command)
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return Tool{}, fmt.Errorf("inspect Node tools: resolve %s: %w", command, err)
	}
	resolved = filepath.Clean(resolved)
	if !inside(root, resolved) {
		return Tool{}, fmt.Errorf("inspect Node tools: %s resolves outside the target Node.js root", command)
	}
	if err := requireRegular(resolved); err != nil {
		return Tool{}, fmt.Errorf("inspect Node tools: resolved %s: %w", command, err)
	}
	metadata, packageRoot, digest, err := findPackage(root, resolved)
	if err != nil {
		return Tool{}, fmt.Errorf("inspect Node tools: %s package metadata: %w", command, err)
	}
	return Tool{
		Name: metadata.Name, Version: metadata.Version, Executable: executable,
		ResolvedPath: resolved, PackageRoot: packageRoot, ControlDigest: digest, Present: true,
	}, nil
}

func findPackage(root, resolved string) (packageMetadata, string, string, error) {
	directory := filepath.Dir(resolved)
	for depth := 0; depth <= 8 && inside(root, directory); depth++ {
		path := filepath.Join(directory, "package.json")
		info, err := os.Lstat(path)
		switch {
		case err == nil:
			if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 || info.Size() > maximumPackage {
				return packageMetadata{}, "", "", errors.New("package.json must be a bounded regular non-symlink file")
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return packageMetadata{}, "", "", readErr
			}
			var metadata packageMetadata
			if json.Unmarshal(data, &metadata) != nil ||
				metadata.Name == "" || len(metadata.Name) > 64 || strings.TrimSpace(metadata.Name) != metadata.Name ||
				metadata.Version == "" || len(metadata.Version) > 256 || strings.TrimSpace(metadata.Version) != metadata.Version {
				return packageMetadata{}, "", "", errors.New("package.json name and version are required")
			}
			digest := sha256.Sum256(data)
			return metadata, directory, "sha256:" + hex.EncodeToString(digest[:]), nil
		case errors.Is(err, fs.ErrNotExist):
		default:
			return packageMetadata{}, "", "", err
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			break
		}
		directory = parent
	}
	return packageMetadata{}, "", "", errors.New("bounded package root was not found")
}

func capture(value Baseline) (execution.Snapshot, error) {
	return execution.NewSnapshot(map[string]string{
		"active_version":             value.NVM.ActiveVersion,
		"default_alias_hash":         value.NVM.DefaultAliasDigest,
		"node_version":               value.NodeVersion,
		"npm_version":                toolVersion(value.NPM),
		"npm_provider":               value.NPM.Provider,
		"npm_control_hash":           value.NPM.ControlDigest,
		"npm_path":                   toolPath(value.NodeRoot, value.NPM),
		"corepack_version":           toolVersion(value.Corepack),
		"corepack_provider":          value.Corepack.Provider,
		"corepack_control_hash":      value.Corepack.ControlDigest,
		"corepack_path":              toolPath(value.NodeRoot, value.Corepack),
		"pnpm_package_version":       toolVersion(value.PNPM),
		"pnpm_provider":              value.PNPM.Provider,
		"pnpm_provider_control_hash": value.PNPM.ControlDigest,
		"pnpm_path":                  toolPath(value.NodeRoot, value.PNPM),
	})
}

func revalidateCheckpoint(
	ctx context.Context,
	toolID, provider string,
	options Options,
	action plan.Action,
	recorded execution.Snapshot,
) (execution.Snapshot, error) {
	if action.TargetVersion != desiredTarget(options.Targets, toolID) {
		return execution.Snapshot{}, errors.New("checkpoint action does not match its registered target")
	}
	expectedScriptDigest, err := actionCheckExpected(action.Preconditions, "adapter_script_digest_matches")
	if err != nil {
		return execution.Snapshot{}, err
	}
	expectedDefaultDigest, err := actionCheckExpected(action.Preconditions, "default_alias_digest_matches")
	if err != nil {
		return execution.Snapshot{}, err
	}
	expectedNodeVersion, err := actionCheckExpected(action.Preconditions, "target_node_version_installed")
	if err != nil {
		return execution.Snapshot{}, err
	}
	expectedNodeVersion, err = exactVersion(strings.TrimPrefix(expectedNodeVersion, "v"))
	if err != nil || expectedNodeVersion != options.Baseline.NodeVersion {
		return execution.Snapshot{}, errors.New("checkpoint target Node.js identity is invalid")
	}
	expectedActiveVersion, err := actionCheckExpected(action.Verifications, "active_version_unchanged")
	if err != nil {
		return execution.Snapshot{}, err
	}
	expectedActiveVersion, err = exactVersion(strings.TrimPrefix(expectedActiveVersion, "v"))
	if err != nil {
		return execution.Snapshot{}, errors.New("checkpoint active Node.js identity is invalid")
	}
	current, err := Inspect(options.Baseline.NVM.Directory, options.Baseline.NodeVersion, options.Baseline.NVM.ActiveVersion)
	if err != nil {
		return execution.Snapshot{}, err
	}
	if current.NVM.ScriptDigest != expectedScriptDigest || current.NVM.DefaultAliasDigest != expectedDefaultDigest {
		return execution.Snapshot{}, errors.New("NVM control files changed after the source operation")
	}
	actual := baselineTool(current, toolID)
	if !actual.Present || actual.Provider != provider {
		return execution.Snapshot{}, errors.New("checkpoint tool provider or ownership changed")
	}
	reportedToolVersion, err := queryVersion(ctx, options, actual.Executable)
	if err != nil || reportedToolVersion != action.TargetVersion {
		return execution.Snapshot{}, errors.New("checkpoint tool version changed")
	}
	if provider == plan.NodeToolProviderCorepack {
		current.PNPM.Version = reportedToolVersion
	} else if actual.Version != action.TargetVersion {
		return execution.Snapshot{}, errors.New("checkpoint package version changed")
	}
	reportedNodeVersion, err := queryVersion(ctx, options, current.NodeBinary)
	if err != nil || reportedNodeVersion != expectedNodeVersion {
		return execution.Snapshot{}, errors.New("checkpoint target Node.js version changed")
	}
	reportedActiveVersion := strings.TrimPrefix(current.NVM.ActiveVersion, "v")
	if options.ActiveBinary != "" {
		reportedActiveVersion, err = queryVersion(ctx, options, options.ActiveBinary)
		if err != nil {
			return execution.Snapshot{}, errors.New("checkpoint active Node.js version could not be verified")
		}
	}
	currentSnapshot, err := capture(current)
	if err != nil {
		return execution.Snapshot{}, err
	}
	expected, err := nodeToolCheckpointEvidence(
		recorded, toolID, action.TargetVersion, expectedNodeVersion, expectedActiveVersion,
	)
	if err != nil {
		return execution.Snapshot{}, err
	}
	observed, err := nodeToolCheckpointEvidence(
		currentSnapshot, toolID, reportedToolVersion, reportedNodeVersion, reportedActiveVersion,
	)
	if err != nil {
		return execution.Snapshot{}, err
	}
	if expected.Digest != observed.Digest {
		return execution.Snapshot{}, errors.New("checkpoint action-scoped evidence drifted")
	}
	return observed, nil
}

func actionCheckExpected(checks []plan.Check, kind string) (string, error) {
	expected := ""
	for _, check := range checks {
		if check.Kind != kind {
			continue
		}
		if expected != "" {
			return "", errors.New("checkpoint Plan contains duplicate check metadata")
		}
		expected = check.Expected
	}
	if expected == "" {
		return "", errors.New("checkpoint Plan is missing required check metadata")
	}
	return expected, nil
}

func nodeToolCheckpointEvidence(
	value execution.Snapshot,
	toolID, reportedToolVersion, reportedNodeVersion, reportedActiveVersion string,
) (execution.Snapshot, error) {
	versionKey, providerKey, controlKey, pathKey := "", "", "", ""
	switch toolID {
	case plan.NodeToolNPM:
		versionKey, providerKey, controlKey, pathKey = "npm_version", "npm_provider", "npm_control_hash", "npm_path"
	case plan.NodeToolCorepack:
		versionKey, providerKey, controlKey, pathKey = "corepack_version", "corepack_provider", "corepack_control_hash", "corepack_path"
	case plan.NodeToolPNPM:
		versionKey, providerKey, controlKey, pathKey = "pnpm_package_version", "pnpm_provider", "pnpm_provider_control_hash", "pnpm_path"
	default:
		return execution.Snapshot{}, errors.New("unsupported Node tool checkpoint")
	}
	facts := map[string]string{
		"active_version":          value.Facts["active_version"],
		"default_alias_hash":      value.Facts["default_alias_hash"],
		"node_version":            value.Facts["node_version"],
		"tool_version":            value.Facts[versionKey],
		"tool_provider":           value.Facts[providerKey],
		"tool_control_hash":       value.Facts[controlKey],
		"tool_path":               value.Facts[pathKey],
		"reported_tool_version":   reportedToolVersion,
		"reported_node_version":   reportedNodeVersion,
		"reported_active_version": reportedActiveVersion,
	}
	for _, candidate := range facts {
		if candidate == "" || candidate == "absent" || candidate == "unknown" || candidate == "outside" {
			return execution.Snapshot{}, errors.New("Node tool checkpoint evidence is incomplete")
		}
	}
	return execution.NewSnapshot(facts)
}

func controlledEnvironment(options Options, network bool) ([]string, []string) {
	temporary := temporaryDirectory(options.Temporary)
	values := map[string]string{
		"HOME":                               options.Home,
		"PATH":                               options.Baseline.NodeRoot + "/bin:/usr/bin:/bin:/usr/sbin:/sbin",
		"TMPDIR":                             temporary,
		"NPM_CONFIG_USERCONFIG":              "/dev/null",
		"NPM_CONFIG_GLOBALCONFIG":            "/dev/null",
		"NPM_CONFIG_REGISTRY":                RegistryURL,
		"NPM_CONFIG_PREFIX":                  options.Baseline.NodeRoot,
		"NPM_CONFIG_IGNORE_SCRIPTS":          "true",
		"NPM_CONFIG_AUDIT":                   "false",
		"NPM_CONFIG_FUND":                    "false",
		"NPM_CONFIG_UPDATE_NOTIFIER":         "false",
		"COREPACK_NPM_REGISTRY":              RegistryURL,
		"COREPACK_ENABLE_PROJECT_SPEC":       "0",
		"COREPACK_ENABLE_AUTO_PIN":           "0",
		"COREPACK_ENABLE_DOWNLOAD_PROMPT":    "0",
		"COREPACK_ENV_FILE":                  "0",
		"COREPACK_ENABLE_UNSAFE_CUSTOM_URLS": "0",
		"COREPACK_DEFAULT_TO_LATEST":         "0",
		"NODE_USE_ENV_PROXY":                 "1",
	}
	if network {
		values["COREPACK_ENABLE_NETWORK"] = "1"
	} else {
		values["COREPACK_ENABLE_NETWORK"] = "0"
	}
	sensitive := []string{}
	for _, key := range proxyKeys() {
		if value := options.ProxyValues[key]; value != "" {
			values[key] = value
			sensitive = append(sensitive, value)
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		environment = append(environment, key+"="+values[key])
	}
	for _, value := range []string{options.Home, options.Baseline.NVM.Directory, options.Baseline.NodeRoot, temporary} {
		if value != "" && value != "/tmp" {
			sensitive = append(sensitive, value)
		}
	}
	return environment, sensitive
}

func queryVersion(ctx context.Context, options Options, executable string) (string, error) {
	if !filepath.IsAbs(executable) {
		return "", errors.New("Node tool verification executable is invalid")
	}
	runner := options.Verifier
	if runner == nil {
		runner = execution.OSRunner{}
	}
	environment, _ := controlledEnvironment(options, false)
	result := runner.Run(ctx, execution.CommandSpec{
		Executable: executable, Args: []string{"--version"}, Environment: environment,
		Directory: temporaryDirectory(options.Temporary), Timeout: VerificationLimit, TerminateTree: true,
	})
	if result.Failure != nil || result.ExitCode == nil || *result.ExitCode != 0 {
		return "", errors.New("Node tool version command failed")
	}
	lines := strings.Split(strings.TrimSpace(result.Stdout.Text), "\n")
	if len(lines) == 0 {
		return "", errors.New("Node tool version output is empty")
	}
	return exactVersion(strings.TrimPrefix(strings.TrimSpace(lines[len(lines)-1]), "v"))
}

func baselineTool(value Baseline, toolID string) Tool {
	switch toolID {
	case plan.NodeToolNPM:
		return value.NPM
	case plan.NodeToolCorepack:
		return value.Corepack
	case plan.NodeToolPNPM:
		return value.PNPM
	default:
		return Tool{}
	}
}

func sameTool(left, right Tool) bool {
	return left.Present == right.Present && left.Version == right.Version &&
		left.Provider == right.Provider && left.ControlDigest == right.ControlDigest &&
		left.ResolvedPath == right.ResolvedPath
}

func desiredTarget(targets Targets, toolID string) string {
	switch toolID {
	case plan.NodeToolNPM:
		return targets.NPM
	case plan.NodeToolCorepack:
		return targets.Corepack
	case plan.NodeToolPNPM:
		return targets.PNPM
	default:
		return ""
	}
}

func packageName(toolID string) string {
	return strings.TrimPrefix(toolID, "ecosystem.")
}

func toolVersion(value Tool) string {
	if !value.Present {
		return "absent"
	}
	return value.Version
}

func toolPath(root string, tool Tool) string {
	if !tool.Present || tool.Executable == "" {
		return "absent"
	}
	value, err := filepath.Rel(root, tool.Executable)
	if err != nil || value == ".." || strings.HasPrefix(value, ".."+string(filepath.Separator)) {
		return "outside"
	}
	return filepath.ToSlash(value)
}

func exactVersion(raw string) (string, error) {
	parsed := versioncore.ParseSemVer(raw)
	if !parsed.Comparable || parsed.Normalized != raw || strings.ContainsAny(parsed.Normalized, "+-") {
		return "", errors.New("exact stable semantic version is required")
	}
	return parsed.Normalized, nil
}

func requireExecutable(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode()&0o111 == 0 {
		return errors.New("path must be a regular non-symlink executable")
	}
	return nil
}

func requireRegular(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("path must be a regular non-symlink file")
	}
	return nil
}

func inside(root, path string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func temporaryDirectory(value string) string {
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return "/tmp"
}

func proxyKeys() []string {
	return []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "all_proxy", "no_proxy"}
}
