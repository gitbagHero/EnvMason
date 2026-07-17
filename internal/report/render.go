package report

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/gitbagHero/EnvMason/internal/inventory"
)

func renderSummary(value inventory.Inventory) []byte {
	var output bytes.Buffer
	fmt.Fprintln(&output, "EnvMason macOS environment report")
	fmt.Fprintf(&output, "Generated: %s\n", value.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"))
	fmt.Fprintf(&output, "Status: %s\n", reportStatus(value.Findings))
	fmt.Fprintln(&output, "Scope: system, PATH, Homebrew, Node.js, Java, jenv, Maven, Gradle")
	fmt.Fprintf(&output, "System: macOS %s (%s), %s process on %s\n", plainCell(value.System.OSVersion), plainCell(value.System.OSBuild), value.System.ProcessArchitecture, value.System.Architecture)
	fmt.Fprintf(&output, "Shell: %s (%s)\n", plainCell(value.System.Shell.InvokingName), plainCell(value.System.Shell.InvokingPath))
	fmt.Fprintf(&output, "PATH entries: %d\n", len(value.System.PathEntries))
	fmt.Fprintf(&output, "Tools: %d (%d installations)\n", len(value.Tools), installationCount(value.Tools))
	fmt.Fprintf(&output, "Findings: %s\n", findingCounts(value.Findings))
	fmt.Fprintf(&output, "Data sources: %s\n", sourceCounts(collectSources(value)))

	fmt.Fprintln(&output, "\nTools:")
	if len(value.Tools) == 0 {
		fmt.Fprintln(&output, "  none")
	}
	for _, tool := range value.Tools {
		fmt.Fprintf(&output, "  [%s] %s (%s)\n", tool.Category, plainCell(tool.DisplayName), plainCell(tool.ID))
		for _, item := range tool.Installations {
			fmt.Fprintf(&output, "    %s | %s | active=%s | default=%s | reason=%s | %s | %s\n", plainCell(item.Version), plainCell(item.Manager), item.ActiveState, item.DefaultState, item.InstallReason, item.Architecture, plainCell(item.Path))
		}
	}

	fmt.Fprintln(&output, "\nFindings:")
	if len(value.Findings) == 0 {
		fmt.Fprintln(&output, "  none")
	}
	for _, finding := range value.Findings {
		fmt.Fprintf(&output, "  %s %s: %s\n", strings.ToUpper(string(finding.Severity)), plainCell(finding.Code), plainCell(finding.Message))
		if finding.Status != "" {
			fmt.Fprintf(&output, "    status: %s\n", finding.Status)
		}
		if len(finding.Evidence) > 0 {
			evidence := make([]string, len(finding.Evidence))
			for index, item := range finding.Evidence {
				evidence[index] = plainCell(item)
			}
			fmt.Fprintf(&output, "    evidence: %s\n", strings.Join(evidence, "; "))
		}
		if finding.Recommendation != "" {
			fmt.Fprintf(&output, "    recommendation: %s\n", plainCell(finding.Recommendation))
		}
		if len(finding.Impact) > 0 {
			impact := make([]string, len(finding.Impact))
			for index, item := range finding.Impact {
				impact[index] = plainCell(item)
			}
			fmt.Fprintf(&output, "    impact: %s\n", strings.Join(impact, "; "))
		}
	}
	remoteSources := collectRemoteSources(value)
	if len(remoteSources) > 0 {
		fmt.Fprintln(&output, "\nRemote data sources:")
		for _, source := range remoteSources {
			fmt.Fprintf(&output, "  %s | collected=%s | confidence=%s\n", plainCell(source.Name), source.CollectedAt.Format("2006-01-02T15:04:05Z07:00"), source.Confidence)
		}
	}
	return output.Bytes()
}

func renderMarkdown(value inventory.Inventory) []byte {
	var output bytes.Buffer
	fmt.Fprintln(&output, "# EnvMason macOS Environment Report")
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "- Generated: `%s`\n", markdownCell(value.GeneratedAt.Format("2006-01-02T15:04:05Z07:00")))
	fmt.Fprintf(&output, "- Status: **%s**\n", reportStatus(value.Findings))
	fmt.Fprintf(&output, "- Schema: `%s`\n", markdownCell(value.SchemaVersion))
	fmt.Fprintf(&output, "- Tools: %d (%d installations)\n", len(value.Tools), installationCount(value.Tools))
	fmt.Fprintf(&output, "- Findings: %s\n", findingCounts(value.Findings))

	fmt.Fprintln(&output, "\n## System")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "| Field | Value |")
	fmt.Fprintln(&output, "| --- | --- |")
	rows := [][2]string{
		{"Operating system", string(value.System.OS)}, {"Version", value.System.OSVersion}, {"Build", value.System.OSBuild},
		{"Architecture", string(value.System.Architecture)}, {"Process architecture", string(value.System.ProcessArchitecture)},
		{"Translation", string(value.System.TranslationState)}, {"Login shell", value.System.Shell.LoginPath},
		{"Invoking shell", value.System.Shell.InvokingPath},
	}
	for _, row := range rows {
		fmt.Fprintf(&output, "| %s | `%s` |\n", row[0], markdownCell(row[1]))
	}

	fmt.Fprintln(&output, "\n## PATH")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "| Position | Path | State | Duplicate |")
	fmt.Fprintln(&output, "| ---: | --- | --- | --- |")
	for _, entry := range value.System.PathEntries {
		fmt.Fprintf(&output, "| %d | `%s` | %s | %t |\n", entry.Position, markdownCell(entry.Value), entry.State, entry.Duplicate)
	}
	if len(value.System.PathEntries) == 0 {
		fmt.Fprintln(&output, "| — | — | — | — |")
	}

	fmt.Fprintln(&output, "\n## Tools")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "| Category | Tool | Version | Manager | Active | Default | Reason | Architecture | Path |")
	fmt.Fprintln(&output, "| --- | --- | --- | --- | --- | --- | --- | --- | --- |")
	for _, tool := range value.Tools {
		for _, item := range tool.Installations {
			fmt.Fprintf(&output, "| %s | %s (`%s`) | `%s` | %s | %s | %s | %s | %s | `%s` |\n",
				tool.Category, markdownCell(tool.DisplayName), markdownCell(tool.ID), markdownCell(item.Version), markdownCell(item.Manager),
				item.ActiveState, item.DefaultState, item.InstallReason, item.Architecture, markdownCell(item.Path))
		}
	}
	if len(value.Tools) == 0 {
		fmt.Fprintln(&output, "| — | — | — | — | — | — | — | — | — |")
	}

	fmt.Fprintln(&output, "\n## Findings")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "| Severity | Status | Code | Message | Evidence | Recommendation | Impact |")
	fmt.Fprintln(&output, "| --- | --- | --- | --- | --- | --- | --- |")
	for _, finding := range value.Findings {
		fmt.Fprintf(&output, "| %s | %s | `%s` | %s | %s | %s | %s |\n", finding.Severity, finding.Status, markdownCell(finding.Code), markdownCell(finding.Message), markdownCell(strings.Join(finding.Evidence, "; ")), markdownCell(finding.Recommendation), markdownCell(strings.Join(finding.Impact, "; ")))
	}
	if len(value.Findings) == 0 {
		fmt.Fprintln(&output, "| — | — | — | No findings. | — | — | — |")
	}

	fmt.Fprintln(&output, "\n## Data Sources")
	fmt.Fprintln(&output)
	for _, source := range collectSources(value) {
		fmt.Fprintf(&output, "- `%s` — %s (collected `%s`; %s)\n", markdownCell(string(source.Kind)), markdownCell(source.Name), markdownCell(source.CollectedAt.Format("2006-01-02T15:04:05Z07:00")), source.Confidence)
	}
	return output.Bytes()
}

func collectRemoteSources(value inventory.Inventory) []inventory.SourceMetadata {
	result := []inventory.SourceMetadata{}
	for _, source := range collectSources(value) {
		if strings.Contains(source.Name, "https://") || strings.Contains(source.Name, "http://") {
			result = append(result, source)
		}
	}
	return result
}

func markdownCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.ReplaceAll(value, "`", "\\`")
}

func plainCell(value string) string {
	return strings.Map(func(character rune) rune {
		if character == '\n' || character == '\r' || character == '\t' {
			return ' '
		}
		if character < 0x20 || character == 0x7f {
			return -1
		}
		return character
	}, value)
}

func installationCount(tools []inventory.Tool) int {
	count := 0
	for _, tool := range tools {
		count += len(tool.Installations)
	}
	return count
}

func findingCounts(findings []inventory.Finding) string {
	counts := map[inventory.FindingSeverity]int{}
	for _, finding := range findings {
		counts[finding.Severity]++
	}
	return fmt.Sprintf("%d error, %d warning, %d info", counts[inventory.SeverityError], counts[inventory.SeverityWarning], counts[inventory.SeverityInfo])
}

func sourceCounts(sources []inventory.SourceMetadata) string {
	counts := make(map[inventory.SourceKind]int)
	for _, source := range sources {
		counts[source.Kind]++
	}
	kinds := make([]string, 0, len(counts))
	for kind := range counts {
		kinds = append(kinds, string(kind))
	}
	sort.Strings(kinds)
	parts := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		parts = append(parts, fmt.Sprintf("%s=%d", kind, counts[inventory.SourceKind(kind)]))
	}
	return strings.Join(parts, ", ")
}

func reportStatus(findings []inventory.Finding) string {
	for _, finding := range findings {
		if finding.Code == "REPORT_INCOMPLETE" || finding.Code == "REPORT_SECTION_FAILED" {
			return "incomplete"
		}
	}
	if hasIncompleteEvidence(findings) {
		return "incomplete"
	}
	return "complete"
}

func collectSources(value inventory.Inventory) []inventory.SourceMetadata {
	values := append([]inventory.SourceMetadata{}, value.System.Sources...)
	for _, tool := range value.Tools {
		for _, item := range tool.Installations {
			values = append(values, item.Sources...)
		}
	}
	for _, finding := range value.Findings {
		values = append(values, finding.Sources...)
	}
	seen := make(map[string]bool)
	result := make([]inventory.SourceMetadata, 0, len(values))
	for _, source := range values {
		key := string(source.Kind) + "\x00" + source.Name + "\x00" + source.CollectedAt.String() + "\x00" + string(source.Confidence)
		if !seen[key] {
			seen[key] = true
			result = append(result, source)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		return result[i].Name < result[j].Name
	})
	return result
}
