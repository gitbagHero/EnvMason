package assessment

import (
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/projectscan"
	"github.com/gitbagHero/EnvMason/internal/versiondata"
)

func TestNodeDecisionTable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		installed  string
		policy     ToolPolicy
		freshness  versiondata.Freshness
		wantCode   string
		wantStatus inventory.FindingStatus
	}{
		{name: "LTS update", installed: "v22.0.0", policy: ToolPolicy{Channel: ChannelLTS}, freshness: versiondata.FreshnessFresh, wantCode: "NODE_UPDATE_AVAILABLE", wantStatus: inventory.StatusUpdateAvailable},
		{name: "stable differs from LTS", installed: "v24.2.0", policy: ToolPolicy{Channel: ChannelStable}, freshness: versiondata.FreshnessFresh, wantCode: "NODE_UPDATE_AVAILABLE", wantStatus: inventory.StatusUpdateAvailable},
		{name: "recommended", installed: "v24.2.0", policy: ToolPolicy{Channel: ChannelLTS}, freshness: versiondata.FreshnessFresh, wantCode: "NODE_VERSION_RECOMMENDED", wantStatus: inventory.StatusRecommended},
		{name: "current is not LTS", installed: "v26.1.0", policy: ToolPolicy{Channel: ChannelLTS}, freshness: versiondata.FreshnessFresh, wantCode: "NODE_CHANNEL_MISMATCH", wantStatus: inventory.StatusConflict},
		{name: "ignore ordinary update", installed: "v22.0.0", policy: ToolPolicy{Channel: ChannelLTS, IgnoreUpdates: true}, freshness: versiondata.FreshnessFresh, wantCode: "NODE_UPDATE_IGNORED", wantStatus: inventory.StatusIgnored},
		{name: "stale is unknown", installed: "v22.0.0", policy: ToolPolicy{Channel: ChannelLTS}, freshness: versiondata.FreshnessStale, wantCode: "NODE_RECOMMENDATION_UNKNOWN", wantStatus: inventory.StatusUnknown},
		{name: "invalid installed is unknown", installed: "dynamic", policy: ToolPolicy{Channel: ChannelLTS}, freshness: versiondata.FreshnessFresh, wantCode: "NODE_VERSION_UNKNOWN", wantStatus: inventory.StatusUnknown},
		{name: "pin", installed: "v22.0.0", policy: ToolPolicy{Channel: ChannelLTS, Pin: "23.1.0"}, freshness: versiondata.FreshnessUnavailable, wantCode: "NODE_UPDATE_AVAILABLE", wantStatus: inventory.StatusUpdateAvailable},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			input := fixtureInput(test.installed)
			input.Policy.Tools["runtime.node"] = test.policy
			input.Versions.Node.LatestLTSFreshness = test.freshness
			input.Versions.Node.LatestStableFreshness = test.freshness
			findings := Evaluate(input)
			finding, ok := findCode(findings, test.wantCode)
			if !ok || finding.Status != test.wantStatus || finding.Recommendation == "" || len(finding.Impact) == 0 || len(finding.Evidence) == 0 {
				t.Fatalf("%s/%s not found with structured advice: %#v", test.wantCode, test.wantStatus, findings)
			}
		})
	}
}

func TestProjectMajorReferenceAlwaysProducesRetainAndNeverDeleteAdvice(t *testing.T) {
	t.Parallel()
	input := fixtureInput("v22.22.0")
	input.Projects = projectscan.Result{CollectedAt: input.Inventory.GeneratedAt, Projects: []projectscan.Project{{ID: "project:app", Root: "/workspace/app", References: []projectscan.Reference{{Runtime: projectscan.RuntimeNode, Constraint: "22", Normalized: "22.0.0", Kind: projectscan.ConstraintExact, File: ".nvmrc"}}}}}
	findings := Evaluate(input)
	retain, ok := findCode(findings, "NODE_PROJECT_VERSION_RETAIN")
	if !ok || retain.Status != inventory.StatusRetainRequired || retain.InstallationID != "node-active" {
		t.Fatalf("retain finding = %#v", findings)
	}
	for _, finding := range findings {
		if containsFold(finding.Message+finding.Recommendation, "delete") || containsFold(finding.Message+finding.Recommendation, "remove") || containsFold(finding.Message+finding.Recommendation, "uninstall") {
			t.Fatalf("assessment proposed destructive advice: %#v", finding)
		}
	}
}

func TestEOLAndConflictsRemainVisibleWhenUpdatesIgnored(t *testing.T) {
	t.Parallel()
	input := fixtureInput("v22.0.0")
	input.Policy.Tools["runtime.node"] = ToolPolicy{Channel: ChannelLTS, IgnoreUpdates: true}
	input.Versions.Node.Lifecycle = []versiondata.NodeLifecycle{{Major: 22, State: versiondata.LifecycleEOL, Freshness: versiondata.FreshnessFresh}}
	input.Inventory.Tools[0].Installations = append(input.Inventory.Tools[0].Installations, installation("node-brew", "v24.2.0", "homebrew", inventory.ActiveStateInactive, inventory.DefaultStateDefault))
	findings := Evaluate(input)
	for _, code := range []string{"NODE_UPDATE_IGNORED", "NODE_VERSION_EOL", "NODE_MULTIPLE_SOURCES_CONFLICT", "NODE_DEFAULT_VERSION_CONFLICT"} {
		if _, ok := findCode(findings, code); !ok {
			t.Errorf("missing %s: %#v", code, findings)
		}
	}
}

func TestJavaVendorLifecycleAndRuntimeConflictAreConservative(t *testing.T) {
	t.Parallel()
	input := fixtureInput("v24.2.0")
	input.Inventory.Tools = append(input.Inventory.Tools, inventory.Tool{ID: "runtime.java", DisplayName: "Java", Category: inventory.CategoryRuntime, Installations: []inventory.Installation{installation("jdk-active", "17.0.14", "system", inventory.ActiveStateActive, inventory.DefaultStateDefault)}})
	input.Versions.Java = versiondata.JavaData{LatestLTS: 21, LatestLTSFreshness: versiondata.FreshnessFresh, LatestFeature: 26, LatestFeatureFreshness: versiondata.FreshnessFresh, TemurinLifecycle: []versiondata.JavaLifecycle{{Major: 17, State: versiondata.LifecycleEOL, Freshness: versiondata.FreshnessFresh}}}
	input.JavaVendors = map[string]string{"jdk-active": "Eclipse Adoptium"}
	input.Inventory.Findings = append(input.Inventory.Findings, inventory.Finding{ID: "maven", Code: "MAVEN_JAVA_MISMATCH", Severity: inventory.SeverityWarning, Message: "mismatch", Evidence: []string{"17", "21"}, Confidence: inventory.ConfidenceHigh, Sources: []inventory.SourceMetadata{source()}})
	findings := Evaluate(input)
	for _, code := range []string{"JAVA_VERSION_EOL", "JAVA_UPDATE_AVAILABLE", "MAVEN_JAVA_MISMATCH_ASSESSMENT"} {
		if _, ok := findCode(findings, code); !ok {
			t.Errorf("missing %s: %#v", code, findings)
		}
	}
	input.JavaVendors["jdk-active"] = "unknown"
	if finding, ok := findCode(Evaluate(input), "JAVA_VERSION_EOL"); ok {
		t.Fatalf("unknown vendor received definite EOL: %#v", finding)
	}
}

func fixtureInput(nodeVersion string) Input {
	generatedAt := time.Date(2026, 7, 17, 7, 0, 0, 0, time.UTC)
	return Input{
		Inventory: inventory.Inventory{SchemaVersion: inventory.SchemaVersion, GeneratedAt: generatedAt, Tools: []inventory.Tool{{ID: "runtime.node", DisplayName: "Node.js", Category: inventory.CategoryRuntime, Installations: []inventory.Installation{installation("node-active", nodeVersion, "nvm", inventory.ActiveStateActive, inventory.DefaultStateDefault)}}}},
		Versions:  versiondata.Result{Node: versiondata.NodeData{LatestLTS: "v24.2.0", LatestLTSFreshness: versiondata.FreshnessFresh, LatestStable: "v26.1.0", LatestStableFreshness: versiondata.FreshnessFresh}},
		Policy:    DefaultPolicy(),
	}
}

func installation(id, value, manager string, active inventory.ActiveState, defaultState inventory.DefaultState) inventory.Installation {
	return inventory.Installation{ID: id, Version: value, Path: "/fixture/" + id, Architecture: inventory.ArchitectureARM64, Manager: manager, ActiveState: active, DefaultState: defaultState, InstallReason: inventory.InstallReasonUnknown, Sources: []inventory.SourceMetadata{source()}}
}

func source() inventory.SourceMetadata {
	return inventory.SourceMetadata{Kind: inventory.SourceFixture, Name: "assessment fixture", CollectedAt: time.Date(2026, 7, 17, 7, 0, 0, 0, time.UTC), Confidence: inventory.ConfidenceHigh}
}

func findCode(findings []inventory.Finding, code string) (inventory.Finding, bool) {
	for _, finding := range findings {
		if finding.Code == code {
			return finding, true
		}
	}
	return inventory.Finding{}, false
}

func containsFold(value, part string) bool {
	for index := 0; index+len(part) <= len(value); index++ {
		if equalFoldASCII(value[index:index+len(part)], part) {
			return true
		}
	}
	return false
}

func equalFoldASCII(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		l, r := left[index], right[index]
		if l >= 'A' && l <= 'Z' {
			l += 'a' - 'A'
		}
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		if l != r {
			return false
		}
	}
	return true
}
