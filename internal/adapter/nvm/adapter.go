// Package nvm contains the single fixed NVM write adapter admitted by I15.
// It never evaluates user shell profiles and never accepts command text.
package nvm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/plan"
	versioncore "github.com/gitbagHero/EnvMason/internal/version"
)

const (
	DefaultBashPath = "/bin/bash"
	ActionTimeout   = 10 * time.Minute
	maximumScript   = 4 << 20
	maximumAlias    = 4 << 10
	fixedScript     = "set -eu\nNVM_DIR=$1\nversion=$2\nexport NVM_DIR\n. \"$NVM_DIR/nvm.sh\" --no-use\nnvm install -b --skip-default-packages --no-progress \"$version\"\n"
)

type Baseline struct {
	Directory          string
	ScriptDigest       string
	DefaultAliasDigest string
	InstalledVersions  []string
	ActiveVersion      string
}

type Options struct {
	Baseline     Baseline
	ActiveBinary string
	Home         string
	Temporary    string
	ProxyValues  map[string]string
}

func Locate(explicit, xdgConfigHome, home string) string {
	switch {
	case explicit != "":
		return filepath.Clean(explicit)
	case xdgConfigHome != "":
		return filepath.Join(xdgConfigHome, "nvm")
	case home != "":
		return filepath.Join(home, ".nvm")
	default:
		return ""
	}
}

func Inspect(directory, activeVersion string) (Baseline, error) {
	if !filepath.IsAbs(directory) {
		return Baseline{}, errors.New("inspect nvm: absolute NVM directory is required")
	}
	scriptDigest, err := digestRegularFile(filepath.Join(directory, "nvm.sh"))
	if err != nil {
		return Baseline{}, fmt.Errorf("inspect nvm: nvm.sh: %w", err)
	}
	defaultDigest, err := digestRegularFile(filepath.Join(directory, "alias", "default"))
	if err != nil {
		return Baseline{}, fmt.Errorf("inspect nvm: default alias: %w", err)
	}
	versions, err := installedVersions(directory)
	if err != nil {
		return Baseline{}, fmt.Errorf("inspect nvm: installed versions: %w", err)
	}
	return Baseline{Directory: filepath.Clean(directory), ScriptDigest: scriptDigest, DefaultAliasDigest: defaultDigest, InstalledVersions: versions, ActiveVersion: activeVersion}, nil
}

func Definition(options Options) execution.Definition {
	return execution.Definition{
		Key:         execution.ActionKey{ToolID: "runtime.node", Operation: "install_version", Adapter: "nvm"},
		MinimumRisk: plan.RiskR2,
		Build: func(action plan.Action) (execution.CommandSpec, error) {
			version, err := normalizedTarget(action.TargetVersion)
			if err != nil {
				return execution.CommandSpec{}, err
			}
			environment, sensitive := controlledEnvironment(options)
			return execution.CommandSpec{
				Executable:  DefaultBashPath,
				Args:        []string{"--noprofile", "--norc", "-c", fixedScript, "envmason-nvm", options.Baseline.Directory, version},
				Environment: environment, Timeout: ActionTimeout, SensitiveValues: sensitive, TerminateTree: true,
			}, nil
		},
		Preflight: func(_ context.Context, _ plan.Action) error {
			current, err := Inspect(options.Baseline.Directory, options.Baseline.ActiveVersion)
			if err != nil {
				return err
			}
			if current.ScriptDigest != options.Baseline.ScriptDigest {
				return errors.New("nvm.sh changed after Plan creation")
			}
			if current.DefaultAliasDigest != options.Baseline.DefaultAliasDigest {
				return errors.New("NVM default alias changed after Plan creation")
			}
			return nil
		},
		Capture: func(_ context.Context, action plan.Action) (execution.Snapshot, error) {
			return capture(options.Baseline, action.TargetVersion)
		},
		Satisfied: func(_ context.Context, action plan.Action) (bool, error) {
			path, err := targetBinary(options.Baseline.Directory, action.TargetVersion)
			if err != nil {
				return false, err
			}
			info, err := os.Lstat(path)
			if errors.Is(err, fs.ErrNotExist) {
				return false, nil
			}
			return err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0, err
		},
		Verify: func(ctx context.Context, action plan.Action, _ execution.ProcessResult) error {
			return verify(ctx, options, action)
		},
	}
}

func capture(baseline Baseline, target string) (execution.Snapshot, error) {
	versions, err := installedVersions(baseline.Directory)
	if err != nil {
		return execution.Snapshot{}, err
	}
	defaultDigest, err := digestRegularFile(filepath.Join(baseline.Directory, "alias", "default"))
	if err != nil {
		return execution.Snapshot{}, err
	}
	targetVersion, err := normalizedTarget(target)
	if err != nil {
		return execution.Snapshot{}, err
	}
	installed := "false"
	binary := filepath.Join(baseline.Directory, "versions", "node", "v"+targetVersion, "bin", "node")
	if info, statErr := os.Lstat(binary); statErr == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
		installed = "true"
	}
	return execution.NewSnapshot(map[string]string{
		"active_version":     baseline.ActiveVersion,
		"default_alias_hash": defaultDigest,
		"installed_versions": strings.Join(versions, ","),
		"target_installed":   installed,
	})
}

func verify(ctx context.Context, options Options, action plan.Action) error {
	current, err := Inspect(options.Baseline.Directory, options.Baseline.ActiveVersion)
	if err != nil {
		return err
	}
	if current.DefaultAliasDigest != options.Baseline.DefaultAliasDigest {
		return errors.New("NVM default alias changed during installation")
	}
	installed := make(map[string]bool, len(current.InstalledVersions))
	for _, value := range current.InstalledVersions {
		installed[value] = true
	}
	for _, original := range options.Baseline.InstalledVersions {
		if !installed[original] {
			return fmt.Errorf("original Node.js version %s was not retained", original)
		}
		binary := filepath.Join(options.Baseline.Directory, "versions", "node", original, "bin", "node")
		info, statErr := os.Lstat(binary)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
			return fmt.Errorf("original Node.js version %s is no longer executable", original)
		}
	}
	binary, err := targetBinary(options.Baseline.Directory, action.TargetVersion)
	if err != nil {
		return err
	}
	info, err := os.Lstat(binary)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return errors.New("installed Node.js target is not a regular executable")
	}
	result := (execution.OSRunner{}).Run(ctx, execution.CommandSpec{
		Executable: binary, Args: []string{"--version"},
		Environment: []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin"}, Timeout: 10 * time.Second,
	})
	if result.Failure != nil || result.ExitCode == nil || *result.ExitCode != 0 {
		return errors.New("installed Node.js target could not be verified")
	}
	want, _ := normalizedTarget(action.TargetVersion)
	got, err := normalizedTarget(strings.TrimSpace(result.Stdout.Text))
	if err != nil || got != want {
		return errors.New("installed Node.js target version did not match")
	}
	if options.ActiveBinary != "" {
		if !filepath.IsAbs(options.ActiveBinary) {
			return errors.New("active Node.js verification path is invalid")
		}
		active := (execution.OSRunner{}).Run(ctx, execution.CommandSpec{
			Executable: options.ActiveBinary, Args: []string{"--version"},
			Environment: []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin"}, Timeout: 10 * time.Second,
		})
		if active.Failure != nil || active.ExitCode == nil || *active.ExitCode != 0 {
			return errors.New("active Node.js version could not be re-verified")
		}
		wantActive, parseErr := normalizedTarget(options.Baseline.ActiveVersion)
		gotActive, outputErr := normalizedTarget(strings.TrimSpace(active.Stdout.Text))
		if parseErr != nil || outputErr != nil || gotActive != wantActive {
			return errors.New("active Node.js version changed during installation")
		}
	}
	return nil
}

func controlledEnvironment(options Options) ([]string, []string) {
	temporary := options.Temporary
	if !filepath.IsAbs(temporary) {
		temporary = "/tmp"
	}
	values := map[string]string{
		"HOME": options.Home, "NVM_DIR": options.Baseline.Directory,
		"PATH": "/usr/bin:/bin:/usr/sbin:/sbin", "TMPDIR": temporary,
	}
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "all_proxy", "no_proxy"} {
		if value := options.ProxyValues[key]; value != "" {
			values[key] = value
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	sensitive := []string{}
	for _, key := range keys {
		environment = append(environment, key+"="+values[key])
		if strings.Contains(strings.ToLower(key), "proxy") && values[key] != "" {
			sensitive = append(sensitive, values[key])
		}
	}
	for _, value := range []string{options.Home, options.Baseline.Directory, temporary} {
		if value != "" && value != "/tmp" {
			sensitive = append(sensitive, value)
		}
	}
	return environment, sensitive
}

func installedVersions(directory string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(directory, "versions", "node"))
	if errors.Is(err, fs.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	versions := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			if _, err := normalizedTarget(entry.Name()); err == nil {
				versions = append(versions, entry.Name())
			}
		}
	}
	sort.Strings(versions)
	return versions, nil
}

func targetBinary(directory, target string) (string, error) {
	normalized, err := normalizedTarget(target)
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "versions", "node", "v"+normalized, "bin", "node"), nil
}

func normalizedTarget(raw string) (string, error) {
	parsed := versioncore.ParseSemVer(strings.TrimSpace(raw))
	if !parsed.Comparable || strings.Contains(parsed.Normalized, "+") || strings.Contains(parsed.Normalized, "-") {
		return "", errors.New("NVM target must be an exact stable semantic version")
	}
	return parsed.Normalized, nil
}

func digestRegularFile(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("path must be a regular non-symlink file")
	}
	maximum := int64(maximumScript)
	if filepath.Base(path) == "default" {
		maximum = maximumAlias
	}
	if info.Size() < 0 || info.Size() > maximum {
		return "", errors.New("file exceeds the fixed size limit")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
