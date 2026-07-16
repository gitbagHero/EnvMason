package homebrew

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/gitbagHero/EnvMason/internal/discovery/executable"
	"github.com/gitbagHero/EnvMason/internal/inventory"
)

const unknown = "unknown"

type dependencies struct {
	runner commandRunner
}

// Discover finds Homebrew through the executable discovery core and invokes a
// fixed allowlist of read-only Homebrew and Git queries.
func Discover(ctx context.Context, request Request) (Result, error) {
	return discover(ctx, request, dependencies{runner: execRunner{}})
}

func discover(ctx context.Context, request Request, deps dependencies) (Result, error) {
	if request.CollectedAt.IsZero() {
		return Result{}, errors.New("collection time is required")
	}
	executableResult, err := discoverExecutable(ctx, "brew", request)
	if err != nil {
		return Result{}, fmt.Errorf("discover brew executable: %w", err)
	}
	result := Result{
		State: StateNotInstalled, Version: unknown, Prefix: unknown, Repository: unknown,
		Cellar: unknown, Caskroom: unknown, Origin: unknown, Architecture: inventory.ArchitectureUnknown,
		DataFormat: JSONFormatV2, Tools: []inventory.Tool{}, Outdated: []OutdatedPackage{},
		Findings: append([]inventory.Finding{}, executableResult.Findings...),
	}
	var brewCandidate *executable.Candidate
	for index := range executableResult.Candidates {
		if executableResult.Candidates[index].Effective {
			brewCandidate = &executableResult.Candidates[index]
			break
		}
	}
	if brewCandidate == nil {
		return result, nil
	}
	brewPath := brewCandidate.AccessPath()
	if brewPath == "" {
		result.State = StateUnknown
		return result, nil
	}
	result.State = StateInstalled
	result.BrewPath = brewCandidate.Path
	result.ResolvedPath = brewCandidate.ResolvedPath

	collector := adapterCollector{result: &result, runner: deps.runner, collectedAt: request.CollectedAt.UTC()}
	environment := map[string]string{
		"HOMEBREW_NO_AUTO_UPDATE": "1",
		"HOMEBREW_NO_ANALYTICS":   "1",
		"HOMEBREW_NO_ENV_HINTS":   "1",
	}
	versionOutput, versionOK := collector.run(ctx, brewPath, []string{"--version"}, environment, "brew --version")
	if versionOK {
		result.Version = parseVersion(versionOutput)
	}
	prefixRaw, prefixOK := collector.run(ctx, brewPath, []string{"--prefix"}, environment, "brew --prefix")
	if prefixOK && prefixRaw != "" {
		result.Prefix = redactHome(prefixRaw, request.Home)
	}
	repositoryRaw, repositoryOK := collector.run(ctx, brewPath, []string{"--repository"}, environment, "brew --repository")
	if repositoryOK && repositoryRaw != "" {
		result.Repository = redactHome(repositoryRaw, request.Home)
	}
	cellarRaw, cellarOK := collector.run(ctx, brewPath, []string{"--cellar"}, environment, "brew --cellar")
	if cellarOK && cellarRaw != "" {
		result.Cellar = redactHome(cellarRaw, request.Home)
	}
	caskroomRaw, caskroomOK := collector.run(ctx, brewPath, []string{"--caskroom"}, environment, "brew --caskroom")
	if caskroomOK && caskroomRaw != "" {
		result.Caskroom = redactHome(caskroomRaw, request.Home)
	}
	result.Architecture = inferArchitecture(prefixRaw, brewCandidate.Architectures, request.ProcessArchitecture)

	if repositoryOK && repositoryRaw != "" {
		gitResult, gitErr := discoverExecutable(ctx, "git", request)
		if gitErr != nil {
			collector.addFinding("HOMEBREW_GIT_DISCOVERY_FAILED", "Git could not be discovered for the Homebrew repository query.", "PATH executable discovery: git", inventory.ConfidenceHigh)
		} else {
			collector.appendExecutableFindings(gitResult.Findings)
			if gitPath := effectiveAccessPath(gitResult); gitPath != "" {
				originOutput, ok := collector.run(ctx, gitPath, []string{"-C", repositoryRaw, "remote", "get-url", "origin"}, environment, "git remote get-url origin")
				if ok && originOutput != "" {
					result.Origin = sanitizeRemote(originOutput)
				}
			} else {
				collector.addFinding("HOMEBREW_GIT_NOT_FOUND", "Git was not found, so the Homebrew repository remote could not be queried.", "PATH executable discovery: git", inventory.ConfidenceHigh)
			}
		}
	}
	infoOutput, infoOK := collector.run(ctx, brewPath, []string{"info", "--json=v2", "--installed"}, environment, "brew info --json=v2 --installed")
	if infoOK {
		source := sourceMetadata("brew info --json=v2 --installed", request.CollectedAt, inventory.ConfidenceHigh)
		tools, parseErr := parseInfo([]byte(infoOutput), cellarRaw, caskroomRaw, request.Home, result.Architecture, source)
		if parseErr != nil {
			collector.parseFailure("HOMEBREW_INFO_JSON_INVALID", "Homebrew installed-package JSON v2 could not be parsed.", "brew info --json=v2 --installed")
		} else {
			result.Tools = tools
		}
	}
	outdatedOutput, outdatedOK := collector.run(ctx, brewPath, []string{"outdated", "--json=v2"}, environment, "brew outdated --json=v2")
	if outdatedOK {
		outdated, parseErr := parseOutdated([]byte(outdatedOutput))
		if parseErr != nil {
			collector.parseFailure("HOMEBREW_OUTDATED_JSON_INVALID", "Homebrew outdated JSON v2 could not be parsed.", "brew outdated --json=v2")
		} else {
			result.Outdated = outdated
		}
	}
	return result, nil
}

func discoverExecutable(ctx context.Context, command string, request Request) (executable.Result, error) {
	return executable.Discover(ctx, executable.Request{
		Command: command, Directories: request.PathDirectories,
		WorkingDirectory: request.WorkingDirectory, Home: request.Home, CollectedAt: request.CollectedAt,
	})
}

func effectiveAccessPath(result executable.Result) string {
	for _, candidate := range result.Candidates {
		if candidate.Effective {
			return candidate.AccessPath()
		}
	}
	return ""
}

type adapterCollector struct {
	result      *Result
	runner      commandRunner
	collectedAt time.Time
}

func (c *adapterCollector) run(ctx context.Context, path string, args []string, environment map[string]string, sourceName string) (string, bool) {
	output, err := c.runner.Run(ctx, path, args, environment)
	if err == nil {
		return output.stdout, true
	}
	code := "HOMEBREW_COMMAND_FAILED"
	message := "A read-only Homebrew query failed."
	if isLockMessage(output.stderr) {
		code = "HOMEBREW_LOCKED"
		message = "Homebrew reported that a lock is currently held."
	}
	c.addFinding(code, message, sourceName, inventory.ConfidenceHigh)
	return "", false
}

func (c *adapterCollector) parseFailure(code, message, sourceName string) {
	c.addFinding(code, message, sourceName, inventory.ConfidenceHigh)
}

func (c *adapterCollector) appendExecutableFindings(findings []inventory.Finding) {
	for _, finding := range findings {
		finding.ID = fmt.Sprintf("homebrew-adapter-%d", len(c.result.Findings)+1)
		c.result.Findings = append(c.result.Findings, finding)
	}
}

func (c *adapterCollector) addFinding(code, message, sourceName string, confidence inventory.Confidence) {
	source := sourceMetadata(sourceName, c.collectedAt, confidence)
	c.result.Findings = append(c.result.Findings, inventory.Finding{
		ID:   fmt.Sprintf("homebrew-adapter-%d", len(c.result.Findings)+1),
		Code: code, Severity: inventory.SeverityWarning, Message: message,
		Evidence: []string{sourceName}, Confidence: confidence, Sources: []inventory.SourceMetadata{source},
	})
}

func sourceMetadata(name string, collectedAt time.Time, confidence inventory.Confidence) inventory.SourceMetadata {
	return inventory.SourceMetadata{Kind: inventory.SourceCommand, Name: name, CollectedAt: collectedAt.UTC(), Confidence: confidence}
}

func parseVersion(output string) string {
	line, _, _ := strings.Cut(output, "\n")
	version := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "Homebrew"))
	if version == "" {
		return unknown
	}
	return version
}

func inferArchitecture(prefix string, candidates []inventory.Architecture, fallback inventory.Architecture) inventory.Architecture {
	for _, architecture := range candidates {
		if architecture != inventory.ArchitectureUnknown {
			return architecture
		}
	}
	switch filepath.Clean(prefix) {
	case "/opt/homebrew":
		return inventory.ArchitectureARM64
	case "/usr/local":
		return inventory.ArchitectureAMD64
	}
	if fallback != "" {
		return fallback
	}
	return inventory.ArchitectureUnknown
}

func isLockMessage(stderr string) bool {
	message := strings.ToLower(stderr)
	return strings.Contains(message, "lock") && (strings.Contains(message, "held") || strings.Contains(message, "already") || strings.Contains(message, "another"))
}

func sanitizeRemote(value string) string {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" {
		return value
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func redactHome(path, home string) string {
	if path == "" || home == "" {
		return path
	}
	cleanPath := filepath.Clean(path)
	cleanHome := filepath.Clean(home)
	if cleanPath == cleanHome {
		return "$HOME"
	}
	prefix := cleanHome + string(filepath.Separator)
	if strings.HasPrefix(cleanPath, prefix) {
		return "$HOME" + strings.TrimPrefix(cleanPath, cleanHome)
	}
	return cleanPath
}
