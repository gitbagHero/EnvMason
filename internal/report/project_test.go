package report

import (
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/projectscan"
)

func TestProjectReferencesAndConflictsUseExistingReportSchema(t *testing.T) {
	value := reportFixture()
	collectedAt := time.Date(2026, 7, 17, 6, 0, 0, 0, time.UTC)
	appendProjectData(&value, projectscan.Result{
		CollectedAt: collectedAt,
		Projects: []projectscan.Project{{ID: "project:123", Root: "$HOME/workspace/app", References: []projectscan.Reference{
			{Runtime: projectscan.RuntimeNode, Constraint: "20", Normalized: "20.0.0", Kind: projectscan.ConstraintExact, File: ".nvmrc"},
			{Runtime: projectscan.RuntimeJava, Constraint: "dynamic", Kind: projectscan.ConstraintUnknown, File: "build.gradle"},
		}}},
		Issues: []projectscan.Issue{{Code: "PROJECT_VERSION_CONFLICT", Root: "$HOME/workspace/app", Runtime: projectscan.RuntimeNode, Details: []string{".nvmrc=20", "package.json=>=22"}}},
	})

	data, err := Render(value, Options{Format: FormatJSON})
	if err != nil {
		t.Fatal(err)
	}
	if err := inventory.ValidateJSON(data); err != nil {
		t.Fatalf("project report does not validate: %v", err)
	}
	text := string(data)
	for _, expected := range []string{"PROJECT_VERSION_REFERENCE", "PROJECT_VERSION_CONFLICT", "$HOME/workspace/app", "20.0.0", "dynamic"} {
		if !strings.Contains(text, expected) {
			t.Errorf("JSON missing %q: %s", expected, text)
		}
	}
}

func TestProjectScanFailuresMarkReportIncomplete(t *testing.T) {
	value := reportFixture()
	appendProjectData(&value, projectscan.Result{
		CollectedAt: value.GeneratedAt,
		Issues:      []projectscan.Issue{{Code: "PROJECT_SCAN_INCOMPLETE", Root: "$HOME/workspace"}},
	})
	data := string(renderSummary(value))
	if !strings.Contains(data, "Status: incomplete") || !strings.Contains(data, "PROJECT_SCAN_INCOMPLETE") {
		t.Fatalf("incomplete project report:\n%s", data)
	}
}
