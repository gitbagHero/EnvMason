package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/adapter/homebrew"
	"github.com/gitbagHero/EnvMason/internal/adapter/java"
	"github.com/gitbagHero/EnvMason/internal/adapter/nodejs"
	"github.com/gitbagHero/EnvMason/internal/inventory"
)

func TestThreeFormatsShareOneFilteredFactSet(t *testing.T) {
	t.Parallel()
	value := reportFixture()
	options := reportOptions(FormatSummary)
	summary, err := Render(value, options)
	if err != nil {
		t.Fatalf("render summary: %v", err)
	}
	options.Format = FormatMarkdown
	markdown, err := Render(value, options)
	if err != nil {
		t.Fatalf("render Markdown: %v", err)
	}
	options.Format = FormatJSON
	jsonData, err := Render(value, options)
	if err != nil {
		t.Fatalf("render JSON: %v", err)
	}
	decoded, err := inventory.Decode(jsonData)
	if err != nil {
		t.Fatalf("JSON does not validate against Inventory Schema: %v", err)
	}
	if len(decoded.Tools) != 1 || decoded.Tools[0].ID != "runtime.node" || len(decoded.Findings) != 1 || decoded.Findings[0].Code != "NODE_PATH_SHADOWED" {
		t.Fatalf("filtered JSON facts = %#v / %#v", decoded.Tools, decoded.Findings)
	}
	for _, fact := range []string{"Node.js", "runtime.node", "v26.5.0", "NODE_PATH_SHADOWED", "$HOME/.nvm/versions/node/v26.5.0/bin/node"} {
		if !bytes.Contains(summary, []byte(fact)) {
			t.Errorf("summary missing %q:\n%s", fact, summary)
		}
		if !bytes.Contains(markdown, []byte(fact)) {
			t.Errorf("Markdown missing %q:\n%s", fact, markdown)
		}
		if !bytes.Contains(jsonData, []byte(fact)) {
			t.Errorf("JSON missing %q:\n%s", fact, jsonData)
		}
	}
}

func TestFilterUsesORWithinDimensionsAndANDBetweenThem(t *testing.T) {
	t.Parallel()
	value := reportFixture()
	value.Tools = append(value.Tools, inventory.Tool{
		ID: "ecosystem.npm", DisplayName: "npm", Category: inventory.CategoryEcosystem,
		Installations: []inventory.Installation{fixtureInstallation("npm-1", "11.4.2")},
	}, inventory.Tool{
		ID: "base.git", DisplayName: "Git", Category: inventory.CategoryBase,
		Installations: []inventory.Installation{fixtureInstallation("git-1", "2.50.0")},
	})
	value.Findings = append(value.Findings,
		inventory.Finding{ID: "f-info", Code: "NPM_INFO", Severity: inventory.SeverityInfo, Message: "info", Evidence: []string{}, Confidence: inventory.ConfidenceHigh, ToolID: "ecosystem.npm", Sources: []inventory.SourceMetadata{fixtureSource()}},
		inventory.Finding{ID: "f-error", Code: "GIT_ERROR", Severity: inventory.SeverityError, Message: "error", Evidence: []string{}, Confidence: inventory.ConfidenceHigh, ToolID: "base.git", Sources: []inventory.SourceMetadata{fixtureSource()}},
		inventory.Finding{ID: "f-global", Code: "GLOBAL_WARNING", Severity: inventory.SeverityWarning, Message: "global", Evidence: []string{}, Confidence: inventory.ConfidenceHigh, Sources: []inventory.SourceMetadata{fixtureSource()}},
	)
	filtered := Filter(value,
		[]inventory.ToolCategory{inventory.CategoryRuntime, inventory.CategoryEcosystem},
		[]inventory.FindingSeverity{inventory.SeverityWarning, inventory.SeverityError},
	)
	if len(filtered.Tools) != 2 || filtered.Tools[0].ID != "runtime.node" || filtered.Tools[1].ID != "ecosystem.npm" {
		t.Fatalf("filtered tools = %#v", filtered.Tools)
	}
	if len(filtered.Findings) != 2 || filtered.Findings[0].Code != "NODE_PATH_SHADOWED" || filtered.Findings[1].Code != "GLOBAL_WARNING" {
		t.Fatalf("filtered findings = %#v", filtered.Findings)
	}
}

func TestMarkdownHasStableRendererFriendlyStructureAndEscapesCells(t *testing.T) {
	t.Parallel()
	value := reportFixture()
	value.Findings[0].Message = "contains | pipe\nand newline"
	data, err := Render(value, Options{Format: FormatMarkdown})
	if err != nil {
		t.Fatalf("render Markdown: %v", err)
	}
	text := string(data)
	for _, heading := range []string{"# EnvMason macOS Environment Report", "## System", "## PATH", "## Tools", "## Findings", "## Data Sources"} {
		if strings.Count(text, heading) != 1 {
			t.Errorf("heading %q count != 1:\n%s", heading, text)
		}
	}
	if !strings.Contains(text, `contains \| pipe and newline`) {
		t.Fatalf("Markdown table cell was not escaped: %s", text)
	}
}

func TestParseAndValidateOptions(t *testing.T) {
	t.Parallel()
	categories, err := ParseCategories([]string{"runtime", "RUNTIME", "ecosystem"})
	if err != nil || len(categories) != 2 {
		t.Fatalf("ParseCategories = %#v, %v", categories, err)
	}
	severities, err := ParseSeverities([]string{"warning", "error", "warning"})
	if err != nil || len(severities) != 2 {
		t.Fatalf("ParseSeverities = %#v, %v", severities, err)
	}
	for _, options := range []Options{
		{Format: "yaml"},
		{Format: FormatJSON, Categories: []inventory.ToolCategory{"language"}},
		{Format: FormatJSON, Severities: []inventory.FindingSeverity{"fatal"}},
	} {
		if err := ValidateOptions(options); err == nil {
			t.Fatalf("ValidateOptions(%#v) unexpectedly succeeded", options)
		}
	}
}

func TestMappingPreservesManagerSelectionsAndRuntimeRelationships(t *testing.T) {
	t.Parallel()
	collectedAt := fixtureSource().CollectedAt
	value := reportFixture()
	value.Findings = []inventory.Finding{}
	appendHomebrewFacts(&value, homebrew.Result{
		State: homebrew.StateInstalled, Prefix: "/opt/homebrew", Repository: "/opt/homebrew",
		Cellar: "/opt/homebrew/Cellar", Caskroom: "/opt/homebrew/Caskroom", Origin: "https://github.com/Homebrew/brew", DataFormat: homebrew.JSONFormatV2,
	}, collectedAt)
	appendNodeFacts(&value, nodejs.Result{
		NVM:             nodejs.NVM{State: nodejs.StateInstalled, Directory: "$HOME/.nvm", Loaded: true, DefaultAlias: "default", DefaultVersion: "v26.5.0"},
		PackageManagers: []nodejs.PackageManager{{Name: "npm", NodeInstallationID: "node-nvm-v26"}},
	}, collectedAt)
	appendJavaFacts(&value, java.Result{
		JavaHome: "$HOME/.jenv/versions/25", Jenv: java.Jenv{GlobalVersion: "25", LocalVersion: unknown, ShellVersion: unknown},
		Maven:  java.BuildTool{State: java.StateInstalled, Name: "maven", JavaVersion: "25.0.3", JavaHome: "$HOME/.jenv/versions/25"},
		Gradle: java.BuildTool{State: java.StateNotInstalled},
	}, collectedAt)
	for _, code := range []string{"HOMEBREW_CONFIGURATION", "NVM_DEFAULT_SELECTION", "NODE_PACKAGE_MANAGER_RUNTIME_ASSOCIATIONS", "JAVA_HOME_SELECTION", "JENV_GLOBAL_SELECTION", "MAVEN_RUNTIME"} {
		if !hasFinding(value.Findings, code) {
			t.Errorf("mapped report missing %s: %#v", code, value.Findings)
		}
	}
	for _, finding := range value.Findings {
		if len(finding.Sources) == 0 || len(finding.Evidence) == 0 {
			t.Errorf("mapped fact lacks source or evidence: %#v", finding)
		}
	}
}

func reportOptions(format Format) Options {
	return Options{Format: format, Categories: []inventory.ToolCategory{inventory.CategoryRuntime}, Severities: []inventory.FindingSeverity{inventory.SeverityWarning}}
}

func reportFixture() inventory.Inventory {
	source := fixtureSource()
	return inventory.Inventory{
		SchemaVersion: inventory.SchemaVersion,
		GeneratedAt:   source.CollectedAt,
		System: inventory.System{
			OS: inventory.OSMacOS, OSVersion: "26.0", OSBuild: "25A1", Architecture: inventory.ArchitectureARM64,
			ProcessArchitecture: inventory.ArchitectureARM64, TranslationState: inventory.TranslationStateNative,
			Shell:       inventory.Shell{LoginPath: "/bin/zsh", LoginName: "zsh", InvokingPath: "/bin/zsh", InvokingName: "zsh"},
			PathEntries: []inventory.PathEntry{{Position: 0, Value: "/opt/homebrew/bin", State: inventory.PathStateExists}},
			Sources:     []inventory.SourceMetadata{source, scanScopeSource(source.CollectedAt)},
		},
		Tools: []inventory.Tool{{
			ID: "runtime.node", DisplayName: "Node.js", Category: inventory.CategoryRuntime,
			Installations: []inventory.Installation{{
				ID: "node-nvm-v26", Version: "v26.5.0", NormalizedVersion: "26.5.0",
				Path: "$HOME/.nvm/versions/node/v26.5.0/bin/node", Architecture: inventory.ArchitectureARM64,
				Manager: "nvm", ActiveState: inventory.ActiveStateActive, DefaultState: inventory.DefaultStateDefault,
				InstallReason: inventory.InstallReasonUnknown, Sources: []inventory.SourceMetadata{source},
			}},
		}},
		Findings: []inventory.Finding{{
			ID: "node-shadowed", Code: "NODE_PATH_SHADOWED", Severity: inventory.SeverityWarning,
			Message: "Another Node.js installation is shadowed.", Evidence: []string{"runtime.node"},
			Confidence: inventory.ConfidenceHigh, ToolID: "runtime.node", Sources: []inventory.SourceMetadata{source},
		}},
	}
}

func fixtureSource() inventory.SourceMetadata {
	return inventory.SourceMetadata{Kind: inventory.SourceFixture, Name: "I08 report fixture", CollectedAt: time.Date(2026, 7, 17, 1, 2, 3, 0, time.UTC), Confidence: inventory.ConfidenceHigh}
}

func fixtureInstallation(id, version string) inventory.Installation {
	return inventory.Installation{ID: id, Version: version, Path: "/fixture/" + id, Architecture: inventory.ArchitectureARM64, Manager: "fixture", ActiveState: inventory.ActiveStateActive, DefaultState: inventory.DefaultStateUnknown, InstallReason: inventory.InstallReasonUnknown, Sources: []inventory.SourceMetadata{fixtureSource()}}
}
