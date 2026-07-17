package report

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/adapter/homebrew"
	"github.com/gitbagHero/EnvMason/internal/adapter/java"
	"github.com/gitbagHero/EnvMason/internal/adapter/nodejs"
	"github.com/gitbagHero/EnvMason/internal/discovery/macos"
	"github.com/gitbagHero/EnvMason/internal/inventory"
)

func TestScanKeepsReportWhenAdaptersFail(t *testing.T) {
	t.Parallel()
	deps := fixtureScanDependencies()
	deps.homebrewDiscover = func(context.Context, homebrew.Request) (homebrew.Result, error) {
		return homebrew.Result{}, errors.New("secret-homebrew-error")
	}
	deps.javaDiscover = func(context.Context, java.Request) (java.Result, error) {
		return java.Result{}, errors.New("secret-java-error")
	}
	value, err := scan(context.Background(), deps)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(value.Tools) != 1 || value.Tools[0].ID != "runtime.node" {
		t.Fatalf("successful Node section was not retained: %#v", value.Tools)
	}
	if !hasFinding(value.Findings, "REPORT_INCOMPLETE") || countFinding(value.Findings, "REPORT_SECTION_FAILED") != 2 {
		t.Fatalf("partial report findings = %#v", value.Findings)
	}
	data, err := inventory.Marshal(value)
	if err != nil {
		t.Fatalf("partial report does not validate: %v", err)
	}
	if strings.Contains(string(data), "secret-") {
		t.Fatalf("adapter error leaked into report: %s", data)
	}
	for _, format := range []Format{FormatSummary, FormatMarkdown, FormatJSON} {
		output, renderErr := Render(value, Options{Format: format})
		if renderErr != nil {
			t.Fatalf("render %s: %v", format, renderErr)
		}
		if format != FormatJSON && !strings.Contains(string(output), "incomplete") {
			t.Errorf("%s did not visibly mark incomplete:\n%s", format, output)
		}
	}
}

func TestScanUsesOnlyEnvironmentWhitelistAndOneTimestamp(t *testing.T) {
	t.Parallel()
	deps := fixtureScanDependencies()
	secret := "token-i08-must-not-leak"
	values := map[string]string{
		"PATH": "/fixture/bin", "HOME": "/Users/fixture", "JAVA_HOME": "/fixture/jdk",
		"SECRET_TOKEN": secret,
	}
	var mutex sync.Mutex
	requested := []string{}
	deps.lookupEnv = func(name string) (string, bool) {
		mutex.Lock()
		requested = append(requested, name)
		mutex.Unlock()
		value, ok := values[name]
		return value, ok
	}
	value, err := scan(context.Background(), deps)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	data, err := inventory.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytesContain(data, secret) {
		t.Fatalf("secret environment value leaked: %s", data)
	}
	for _, name := range requested {
		if name == "SECRET_TOKEN" {
			t.Fatal("scanner requested a non-whitelisted environment variable")
		}
	}
	if value.GeneratedAt != fixtureSource().CollectedAt {
		t.Fatalf("generated_at = %s", value.GeneratedAt)
	}
	for _, source := range collectSources(value) {
		if !source.CollectedAt.Equal(value.GeneratedAt) {
			t.Fatalf("source %q time = %s, want %s", source.Name, source.CollectedAt, value.GeneratedAt)
		}
	}
}

func TestScanMarksSubprobeFailureIncomplete(t *testing.T) {
	t.Parallel()
	deps := fixtureScanDependencies()
	deps.nodeDiscover = func(context.Context, nodejs.Request) (nodejs.Result, error) {
		result := nodejs.Result{State: nodejs.StateUnknown, Nodes: []nodejs.NodeInstallation{}, PackageManagers: []nodejs.PackageManager{}}
		result.Findings = []inventory.Finding{{
			ID: "node-query", Code: "NODE_VERSION_QUERY_FAILED", Severity: inventory.SeverityWarning,
			Message: "A Node.js version query failed.", Evidence: []string{"node"}, Confidence: inventory.ConfidenceHigh,
			Sources: []inventory.SourceMetadata{fixtureSource()},
		}}
		return result, nil
	}
	value, err := scan(context.Background(), deps)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !hasFinding(value.Findings, "NODE_VERSION_QUERY_FAILED") || !hasFinding(value.Findings, "REPORT_INCOMPLETE") {
		t.Fatalf("subprobe failure was not marked incomplete: %#v", value.Findings)
	}
}

func TestScanRejectsUnsupportedPlatformBeforeCallingAdapters(t *testing.T) {
	t.Parallel()
	deps := fixtureScanDependencies()
	deps.goos = "linux"
	deps.macosDiscover = func(context.Context) (macos.Result, error) {
		t.Fatal("macOS adapter was called")
		return macos.Result{}, nil
	}
	if _, err := scan(context.Background(), deps); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("scan error = %v", err)
	}
}

func fixtureScanDependencies() scanDependencies {
	collectedAt := fixtureSource().CollectedAt
	return scanDependencies{
		goos: "darwin", now: func() time.Time { return collectedAt }, getwd: func() (string, error) { return "/fixture/project", nil },
		lookupEnv: func(name string) (string, bool) {
			values := map[string]string{"PATH": "/fixture/bin", "HOME": "/Users/fixture"}
			value, ok := values[name]
			return value, ok
		},
		macosDiscover: func(context.Context) (macos.Result, error) {
			value := reportFixture()
			return macos.Result{System: value.System, Findings: []inventory.Finding{}}, nil
		},
		homebrewDiscover: func(context.Context, homebrew.Request) (homebrew.Result, error) {
			return homebrew.Result{State: homebrew.StateNotInstalled, Prefix: unknown, Tools: []inventory.Tool{}, Findings: []inventory.Finding{}}, nil
		},
		nodeDiscover: func(context.Context, nodejs.Request) (nodejs.Result, error) {
			return nodejs.Result{State: nodejs.StateInstalled, Nodes: []nodejs.NodeInstallation{{
				ID: "node-fixture", Version: "v26.5.0", NormalizedVersion: "26.5.0", Path: "/fixture/bin/node",
				Architecture: inventory.ArchitectureARM64, Manager: nodejs.ManagerSystem, Effective: true,
			}}, PackageManagers: []nodejs.PackageManager{}, Findings: []inventory.Finding{}}, nil
		},
		javaDiscover: func(context.Context, java.Request) (java.Result, error) {
			return java.Result{State: java.StateInstalled, JavaHome: "/fixture/jdk", JDKs: []java.JDKInstallation{{
				ID: "jdk-fixture", Version: "25.0.2", Home: "/fixture/jdk", Architecture: inventory.ArchitectureARM64, Manager: java.ManagerSystem,
			}}, Current: java.Runtime{State: java.StateInstalled, JDKID: "jdk-fixture", Version: "25.0.2", Home: "/fixture/jdk"},
				Jenv: java.Jenv{State: java.StateNotInstalled}, Maven: java.BuildTool{State: java.StateNotInstalled}, Gradle: java.BuildTool{State: java.StateNotInstalled}, Findings: []inventory.Finding{}}, nil
		},
	}
}

func hasFinding(findings []inventory.Finding, code string) bool {
	return countFinding(findings, code) > 0
}

func countFinding(findings []inventory.Finding, code string) int {
	count := 0
	for _, finding := range findings {
		if finding.Code == code {
			count++
		}
	}
	return count
}

func bytesContain(data []byte, value string) bool { return strings.Contains(string(data), value) }
