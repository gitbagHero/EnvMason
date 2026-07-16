// Package macos provides read-only discovery of macOS system information.
package macos

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
)

const unknown = "unknown"

// Result contains the discovered system and non-fatal probe findings.
type Result struct {
	System   inventory.System
	Findings []inventory.Finding
}

type dependencies struct {
	runner    commandRunner
	lookupEnv func(string) (string, bool)
	stat      func(string) (fs.FileInfo, error)
	now       func() time.Time
	goos      string
	goarch    string
	parentPID int
}

// Discover performs read-only macOS discovery. It invokes only bounded query
// commands and never passes input through a shell.
func Discover(ctx context.Context) (Result, error) {
	return discover(ctx, dependencies{
		runner:    execRunner{timeout: defaultCommandTimeout},
		lookupEnv: os.LookupEnv,
		stat:      os.Stat,
		now:       time.Now,
		goos:      runtime.GOOS,
		goarch:    runtime.GOARCH,
		parentPID: os.Getppid(),
	})
}

func discover(ctx context.Context, deps dependencies) (Result, error) {
	if deps.goos != "darwin" {
		return Result{}, errors.New("macOS discovery is unsupported on this platform")
	}
	collectedAt := deps.now().UTC()
	collector := newCollector(ctx, deps.runner, collectedAt)

	osVersion, ok := collector.run("sw_vers", "--productVersion")
	if !ok || osVersion == "" {
		osVersion = unknown
	}
	osBuild, ok := collector.run("sw_vers", "--buildVersion")
	if !ok || osBuild == "" {
		osBuild = unknown
	}

	armCapability, armOK := collector.run("sysctl", "-n", "hw.optional.arm64")
	machine, machineOK := collector.run("sysctl", "-n", "hw.machine")
	systemArchitecture := systemArchitecture(armCapability, armOK, machine, machineOK)
	processArchitecture := normalizeArchitecture(deps.goarch)

	translatedValue, translatedOK := collector.run("sysctl", "-in", "sysctl.proc_translated")
	translationState := translationState(translatedValue, translatedOK)

	invokingPath, invokingOK := collector.run("ps", "-p", strconv.Itoa(deps.parentPID), "-o", "comm=")
	if !invokingOK || invokingPath == "" {
		invokingPath = unknown
	}
	invokingPath, invokingName := knownShell(invokingPath)

	environment := readEnvironment(deps.lookupEnv)
	collector.addEnvironmentSource(environment)
	loginPath := environment["SHELL"]
	if loginPath == "" {
		loginPath = unknown
	}

	result := Result{
		System: inventory.System{
			OS:                  inventory.OSMacOS,
			OSVersion:           osVersion,
			OSBuild:             osBuild,
			Architecture:        systemArchitecture,
			ProcessArchitecture: processArchitecture,
			TranslationState:    translationState,
			Shell: inventory.Shell{
				LoginPath:    redactHome(loginPath, environment["HOME"]),
				LoginName:    shellName(loginPath),
				InvokingPath: redactHome(invokingPath, environment["HOME"]),
				InvokingName: invokingName,
			},
			PathEntries: pathEntries(environment["PATH"], environment["HOME"], deps.stat),
			Sources:     collector.sources,
		},
		Findings: append([]inventory.Finding{}, collector.findings...),
	}
	return result, nil
}

type collector struct {
	ctx         context.Context
	runner      commandRunner
	collectedAt time.Time
	sources     []inventory.SourceMetadata
	findings    []inventory.Finding
}

func newCollector(ctx context.Context, runner commandRunner, collectedAt time.Time) *collector {
	return &collector{ctx: ctx, runner: runner, collectedAt: collectedAt}
}

func (c *collector) run(name string, args ...string) (string, bool) {
	sourceName := strings.Join(append([]string{name}, args...), " ")
	value, err := c.runner.Run(c.ctx, name, args...)
	confidence := inventory.ConfidenceHigh
	if err != nil {
		confidence = inventory.ConfidenceLow
	}
	source := inventory.SourceMetadata{
		Kind:        inventory.SourceCommand,
		Name:        sourceName,
		CollectedAt: c.collectedAt,
		Confidence:  confidence,
	}
	c.sources = append(c.sources, source)
	if err == nil {
		return strings.TrimSpace(value), true
	}

	index := len(c.findings) + 1
	c.findings = append(c.findings, inventory.Finding{
		ID:         "macos-system-probe-" + strconv.Itoa(index),
		Code:       "SYSTEM_PROBE_FAILED",
		Severity:   inventory.SeverityWarning,
		Message:    "A read-only macOS system probe failed.",
		Evidence:   []string{sourceName},
		Confidence: inventory.ConfidenceHigh,
		Sources:    []inventory.SourceMetadata{source},
	})
	return "", false
}

func (c *collector) addEnvironmentSource(environment map[string]string) {
	confidence := inventory.ConfidenceHigh
	if environment["SHELL"] == "" || environment["PATH"] == "" {
		confidence = inventory.ConfidenceLow
	}
	c.sources = append(c.sources, inventory.SourceMetadata{
		Kind:        inventory.SourceEnvironment,
		Name:        "environment whitelist: SHELL, PATH, HOME",
		CollectedAt: c.collectedAt,
		Confidence:  confidence,
	})
}

func readEnvironment(lookup func(string) (string, bool)) map[string]string {
	result := make(map[string]string, 3)
	for _, name := range []string{"SHELL", "PATH", "HOME"} {
		if value, ok := lookup(name); ok {
			result[name] = value
		}
	}
	return result
}

func systemArchitecture(armValue string, armOK bool, machine string, machineOK bool) inventory.Architecture {
	if armOK && strings.TrimSpace(armValue) == "1" {
		return inventory.ArchitectureARM64
	}
	if machineOK {
		return normalizeArchitecture(machine)
	}
	return inventory.ArchitectureUnknown
}

func normalizeArchitecture(value string) inventory.Architecture {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "arm64", "aarch64":
		return inventory.ArchitectureARM64
	case "amd64", "x86_64":
		return inventory.ArchitectureAMD64
	case "386", "i386", "i686":
		return inventory.Architecture386
	case "arm":
		return inventory.ArchitectureARM
	case "ppc64":
		return inventory.ArchitecturePPC64
	case "ppc64le":
		return inventory.ArchitecturePPC64LE
	case "s390x":
		return inventory.ArchitectureS390X
	case "riscv64":
		return inventory.ArchitectureRISCV64
	default:
		return inventory.ArchitectureUnknown
	}
}

func translationState(value string, ok bool) inventory.TranslationState {
	if !ok {
		return inventory.TranslationStateUnknown
	}
	switch strings.TrimSpace(value) {
	case "0", "":
		return inventory.TranslationStateNative
	case "1":
		return inventory.TranslationStateTranslated
	default:
		return inventory.TranslationStateUnknown
	}
}

func shellName(shellPath string) string {
	if shellPath == "" || shellPath == unknown {
		return unknown
	}
	name := path.Base(shellPath)
	if name == "." || name == "/" || name == "" {
		return unknown
	}
	return name
}

func knownShell(shellPath string) (string, string) {
	name := shellName(shellPath)
	switch name {
	case "sh", "bash", "zsh", "fish", "ksh", "csh", "tcsh", "dash", "nu", "pwsh":
		return shellPath, name
	default:
		return unknown, unknown
	}
}

func pathEntries(pathValue, home string, stat func(string) (fs.FileInfo, error)) []inventory.PathEntry {
	if pathValue == "" {
		return []inventory.PathEntry{}
	}
	values := strings.Split(pathValue, ":")
	counts := make(map[string]int, len(values))
	for _, value := range values {
		counts[value]++
	}
	entries := make([]inventory.PathEntry, 0, len(values))
	for position, value := range values {
		entries = append(entries, inventory.PathEntry{
			Position:  position,
			Value:     redactHome(value, home),
			State:     pathState(value, stat),
			Duplicate: counts[value] > 1,
		})
	}
	return entries
}

func pathState(path string, stat func(string) (fs.FileInfo, error)) inventory.PathState {
	if path == "" || !strings.HasPrefix(path, "/") {
		return inventory.PathStateUnknown
	}
	_, err := stat(path)
	if err == nil {
		return inventory.PathStateExists
	}
	if errors.Is(err, fs.ErrNotExist) {
		return inventory.PathStateMissing
	}
	return inventory.PathStateUnknown
}

func redactHome(value, home string) string {
	if home == "" || value == home {
		if value == home && home != "" {
			return "$HOME"
		}
		return value
	}
	prefix := home + "/"
	if strings.HasPrefix(value, prefix) {
		return "$HOME" + strings.TrimPrefix(value, home)
	}
	return value
}
