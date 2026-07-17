package report

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gitbagHero/EnvMason/internal/adapter/homebrew"
	"github.com/gitbagHero/EnvMason/internal/adapter/java"
	"github.com/gitbagHero/EnvMason/internal/adapter/nodejs"
	"github.com/gitbagHero/EnvMason/internal/discovery/macos"
	"github.com/gitbagHero/EnvMason/internal/inventory"
)

type scanDependencies struct {
	goos             string
	now              func() time.Time
	getwd            func() (string, error)
	lookupEnv        func(string) (string, bool)
	macosDiscover    func(context.Context) (macos.Result, error)
	homebrewDiscover func(context.Context, homebrew.Request) (homebrew.Result, error)
	nodeDiscover     func(context.Context, nodejs.Request) (nodejs.Result, error)
	javaDiscover     func(context.Context, java.Request) (java.Result, error)
}

// Scan assembles one macOS inventory from the read-only discovery adapters.
func Scan(ctx context.Context) (inventory.Inventory, error) {
	return scan(ctx, scanDependencies{
		goos: runtime.GOOS, now: time.Now, getwd: os.Getwd, lookupEnv: os.LookupEnv,
		macosDiscover: macos.Discover, homebrewDiscover: homebrew.Discover,
		nodeDiscover: nodejs.Discover, javaDiscover: java.Discover,
	})
}

func scan(ctx context.Context, deps scanDependencies) (inventory.Inventory, error) {
	if deps.goos != "darwin" {
		return inventory.Inventory{}, fmt.Errorf("macOS report is unsupported on %s", deps.goos)
	}
	workingDirectory, err := deps.getwd()
	if err != nil {
		return inventory.Inventory{}, fmt.Errorf("determine working directory: %w", err)
	}
	collectedAt := deps.now().UTC()
	pathDirectories := filepath.SplitList(environment(deps.lookupEnv, "PATH"))
	home := environment(deps.lookupEnv, "HOME")

	value := inventory.Inventory{SchemaVersion: inventory.SchemaVersion, GeneratedAt: collectedAt, Tools: []inventory.Tool{}, Findings: []inventory.Finding{}}
	systemResult, systemErr := deps.macosDiscover(ctx)
	if systemErr != nil {
		value.System = unknownSystem(collectedAt)
		addSectionFailure(&value, "system", collectedAt)
	} else {
		value.System = systemResult.System
		value.Findings = append(value.Findings, systemResult.Findings...)
	}
	value.System.Sources = append(value.System.Sources, scanScopeSource(collectedAt))

	brewRequest := homebrew.Request{
		PathDirectories: pathDirectories, WorkingDirectory: workingDirectory, Home: home,
		CollectedAt: collectedAt, ProcessArchitecture: value.System.ProcessArchitecture,
	}
	brewResult, brewErr := deps.homebrewDiscover(ctx, brewRequest)
	if brewErr != nil {
		addSectionFailure(&value, "homebrew", collectedAt)
	} else {
		value.Tools = append(value.Tools, mapHomebrew(brewResult, collectedAt)...)
		value.Findings = append(value.Findings, brewResult.Findings...)
		appendHomebrewFacts(&value, brewResult, collectedAt)
	}

	homebrewPrefixes := []string{"/opt/homebrew", "/usr/local"}
	if prefix := restoreHome(brewResult.Prefix, home); prefix != "" && prefix != "unknown" {
		homebrewPrefixes = appendUniqueString(homebrewPrefixes, prefix)
	}

	var nodeResult nodejs.Result
	var javaResult java.Result
	var nodeErr error
	var javaErr error
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		nodeResult, nodeErr = deps.nodeDiscover(ctx, nodejs.Request{
			PathDirectories: pathDirectories, WorkingDirectory: workingDirectory, Home: home,
			XDGConfigHome: environment(deps.lookupEnv, "XDG_CONFIG_HOME"),
			NVMDirectory:  environment(deps.lookupEnv, "NVM_DIR"), NVMBin: environment(deps.lookupEnv, "NVM_BIN"),
			HomebrewPrefixes: homebrewPrefixes, CollectedAt: collectedAt,
			ProcessArchitecture: value.System.ProcessArchitecture,
		})
	}()
	go func() {
		defer group.Done()
		javaResult, javaErr = deps.javaDiscover(ctx, java.Request{
			PathDirectories: pathDirectories, WorkingDirectory: workingDirectory, Home: home,
			JavaHome: environment(deps.lookupEnv, "JAVA_HOME"), JenvRoot: environment(deps.lookupEnv, "JENV_ROOT"),
			JenvShellVersion: environment(deps.lookupEnv, "JENV_VERSION"), HomebrewPrefixes: homebrewPrefixes,
			GradleUserHome: environment(deps.lookupEnv, "GRADLE_USER_HOME"), CollectedAt: collectedAt,
			ProcessArchitecture: value.System.ProcessArchitecture,
		})
	}()
	group.Wait()

	if nodeErr != nil {
		addSectionFailure(&value, "nodejs", collectedAt)
	} else {
		value.Tools = append(value.Tools, mapNode(nodeResult, collectedAt)...)
		value.Findings = append(value.Findings, nodeResult.Findings...)
		appendNodeFacts(&value, nodeResult, collectedAt)
	}
	if javaErr != nil {
		addSectionFailure(&value, "java", collectedAt)
	} else {
		value.Tools = append(value.Tools, mapJava(javaResult, collectedAt)...)
		value.Findings = append(value.Findings, javaResult.Findings...)
		appendJavaFacts(&value, javaResult, collectedAt)
	}

	if hasIncompleteEvidence(value.Findings) {
		value.Findings = append(value.Findings, inventory.Finding{
			ID: "report-incomplete", Code: "REPORT_INCOMPLETE", Severity: inventory.SeverityWarning,
			Message:  "The report is incomplete because one or more discovery probes failed.",
			Evidence: []string{"See failure findings for affected fields or sections."}, Confidence: inventory.ConfidenceHigh,
			Sources: []inventory.SourceMetadata{reportSource("report assembly", collectedAt)},
		})
	}
	normalizeInventory(&value)
	return value, nil
}

func environment(lookup func(string) (string, bool), name string) string {
	value, _ := lookup(name)
	return value
}

func restoreHome(value, home string) string {
	if value == "$HOME" {
		return home
	}
	if strings.HasPrefix(value, "$HOME"+string(filepath.Separator)) {
		return filepath.Join(home, strings.TrimPrefix(value, "$HOME"+string(filepath.Separator)))
	}
	return value
}

func appendUniqueString(values []string, candidate string) []string {
	for _, value := range values {
		if filepath.Clean(value) == filepath.Clean(candidate) {
			return values
		}
	}
	return append(values, candidate)
}

func addSectionFailure(value *inventory.Inventory, section string, collectedAt time.Time) {
	value.Findings = append(value.Findings, inventory.Finding{
		ID: "report-section-" + section, Code: "REPORT_SECTION_FAILED", Severity: inventory.SeverityWarning,
		Message: "A report section could not be collected.", Evidence: []string{section}, Confidence: inventory.ConfidenceHigh,
		Sources: []inventory.SourceMetadata{reportSource("report section: "+section, collectedAt)},
	})
}

func hasIncompleteEvidence(findings []inventory.Finding) bool {
	for _, finding := range findings {
		code := finding.Code
		if code == "REPORT_SECTION_FAILED" || code == "HOMEBREW_LOCKED" ||
			strings.HasSuffix(code, "_FAILED") || strings.HasSuffix(code, "_INVALID") ||
			strings.HasSuffix(code, "_UNAVAILABLE") || strings.HasSuffix(code, "_BROKEN") ||
			strings.HasSuffix(code, "_NOT_FOUND") || strings.HasSuffix(code, "_STALE") {
			return true
		}
	}
	return false
}

func reportSource(name string, collectedAt time.Time) inventory.SourceMetadata {
	return inventory.SourceMetadata{Kind: inventory.SourceUnknown, Name: name, CollectedAt: collectedAt, Confidence: inventory.ConfidenceHigh}
}
