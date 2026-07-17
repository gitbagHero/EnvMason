package report

import (
	"fmt"
	"strings"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/projectscan"
)

func appendProjectData(value *inventory.Inventory, result projectscan.Result) {
	index := 0
	for _, project := range result.Projects {
		for _, reference := range project.References {
			index++
			evidence := []string{"project=" + project.Root, "constraint=" + reference.Constraint, "kind=" + string(reference.Kind), "file=" + reference.File}
			if reference.Normalized != "" {
				evidence = append(evidence, "normalized="+reference.Normalized)
			}
			value.Findings = append(value.Findings, inventory.Finding{
				ID: fmt.Sprintf("project-reference-%03d", index), Code: "PROJECT_VERSION_REFERENCE", Severity: inventory.SeverityInfo,
				Message: "A static project runtime version reference was discovered.", Evidence: evidence,
				Confidence: referenceConfidence(reference.Kind), ToolID: runtimeToolID(reference.Runtime),
				Sources: []inventory.SourceMetadata{projectSource(project.Root, reference.File, result.CollectedAt, referenceConfidence(reference.Kind))},
			})
		}
	}
	for issueIndex, issue := range result.Issues {
		message := "A project path or version declaration could not be scanned completely."
		if issue.Code == "PROJECT_VERSION_CONFLICT" {
			message = "The same project contains provably conflicting runtime version declarations."
		}
		evidence := []string{"project=" + issue.Root}
		if issue.File != "" {
			evidence = append(evidence, "file="+issue.File)
		}
		if issue.Runtime != "" {
			evidence = append(evidence, "runtime="+string(issue.Runtime))
		}
		evidence = append(evidence, issue.Details...)
		value.Findings = append(value.Findings, inventory.Finding{
			ID: fmt.Sprintf("project-issue-%03d", issueIndex+1), Code: issue.Code, Severity: inventory.SeverityWarning,
			Message: message, Evidence: evidence, Confidence: inventory.ConfidenceHigh, ToolID: runtimeToolID(issue.Runtime),
			Sources: []inventory.SourceMetadata{projectSource(issue.Root, issue.File, result.CollectedAt, inventory.ConfidenceHigh)},
		})
	}
	normalizeInventory(value)
}

func projectSource(root, file string, collectedAt time.Time, confidence inventory.Confidence) inventory.SourceMetadata {
	name := "project root: " + root
	if file != "" {
		name = "project file: " + strings.TrimSuffix(root, "/") + "/" + file
	}
	return inventory.SourceMetadata{Kind: inventory.SourceFile, Name: name, CollectedAt: collectedAt, Confidence: confidence}
}

func runtimeToolID(runtime projectscan.Runtime) string {
	switch runtime {
	case projectscan.RuntimeNode:
		return "runtime.node"
	case projectscan.RuntimeJava:
		return "runtime.java"
	default:
		return ""
	}
}

func referenceConfidence(kind projectscan.ConstraintKind) inventory.Confidence {
	if kind == projectscan.ConstraintUnknown {
		return inventory.ConfidenceLow
	}
	return inventory.ConfidenceHigh
}
