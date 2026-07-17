// Package report assembles and renders EnvMason's read-only workstation report.
package report

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gitbagHero/EnvMason/internal/assessment"
	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/projectscan"
	"github.com/gitbagHero/EnvMason/internal/versiondata"
)

type Format string

const (
	FormatSummary  Format = "summary"
	FormatMarkdown Format = "markdown"
	FormatJSON     Format = "json"
)

type Options struct {
	Format     Format
	Categories []inventory.ToolCategory
	Severities []inventory.FindingSeverity
	Online     bool
	Projects   []string
	Excludes   []string
	PolicyPath string
}

func ValidateOptions(options Options) error {
	switch options.Format {
	case "", FormatSummary, FormatMarkdown, FormatJSON:
	default:
		return fmt.Errorf("unsupported report format %q", options.Format)
	}
	for _, category := range options.Categories {
		if !validCategory(category) {
			return fmt.Errorf("unsupported tool category %q", category)
		}
	}
	for _, severity := range options.Severities {
		if !validSeverity(severity) {
			return fmt.Errorf("unsupported finding severity %q", severity)
		}
	}
	if len(options.Excludes) > 0 && len(options.Projects) == 0 {
		return errors.New("--exclude requires at least one --project")
	}
	for _, root := range options.Projects {
		if strings.TrimSpace(root) == "" {
			return errors.New("project root must not be empty")
		}
	}
	for _, exclude := range options.Excludes {
		if strings.TrimSpace(exclude) == "" {
			return errors.New("project exclusion must not be empty")
		}
	}
	if options.PolicyPath != "" && strings.TrimSpace(options.PolicyPath) == "" {
		return errors.New("policy path must not be empty")
	}
	return nil
}

func ParseCategories(values []string) ([]inventory.ToolCategory, error) {
	result := make([]inventory.ToolCategory, 0, len(values))
	for _, value := range values {
		category := inventory.ToolCategory(strings.ToLower(strings.TrimSpace(value)))
		if !validCategory(category) {
			return nil, fmt.Errorf("unsupported tool category %q", value)
		}
		result = appendUniqueCategory(result, category)
	}
	return result, nil
}

func ParseSeverities(values []string) ([]inventory.FindingSeverity, error) {
	result := make([]inventory.FindingSeverity, 0, len(values))
	for _, value := range values {
		severity := inventory.FindingSeverity(strings.ToLower(strings.TrimSpace(value)))
		if !validSeverity(severity) {
			return nil, fmt.Errorf("unsupported finding severity %q", value)
		}
		result = appendUniqueSeverity(result, severity)
	}
	return result, nil
}

// Generate performs one read-only scan and renders the filtered result.
func Generate(ctx context.Context, options Options) ([]byte, error) {
	snapshot, err := scanWithContext(ctx, defaultScanDependencies())
	if err != nil {
		return nil, err
	}
	return assemble(ctx, options, snapshot, versiondata.Collect)
}

func generate(ctx context.Context, options Options, scan func(context.Context) (inventory.Inventory, error), collect func(context.Context) versiondata.Result) ([]byte, error) {
	if options.Format == "" {
		options.Format = FormatSummary
	}
	if err := ValidateOptions(options); err != nil {
		return nil, err
	}
	value, err := scan(ctx)
	if err != nil {
		return nil, err
	}
	return assemble(ctx, options, discoverySnapshot{inventory: value}, collect)
}

func assemble(ctx context.Context, options Options, snapshot discoverySnapshot, collect func(context.Context) versiondata.Result) ([]byte, error) {
	value := snapshot.inventory
	policy, err := assessment.LoadPolicy(options.PolicyPath)
	if err != nil {
		return nil, err
	}
	versions := versiondata.Result{}
	if options.Online {
		versions = collect(ctx)
		appendVersionData(&value, versions)
	}
	projects := projectscan.Result{CollectedAt: value.GeneratedAt}
	if len(options.Projects) > 0 {
		projects = projectscan.Scan(ctx, projectscan.Request{Roots: options.Projects, Excludes: options.Excludes, CollectedAt: value.GeneratedAt})
		appendProjectData(&value, projects)
	}
	value.Findings = append(value.Findings, assessment.Evaluate(assessment.Input{Inventory: value, Versions: versions, Projects: projects, Policy: policy, JavaVendors: snapshot.javaVendors})...)
	normalizeInventory(&value)
	return Render(value, options)
}

// Render filters and renders an already assembled inventory without rescanning.
func Render(value inventory.Inventory, options Options) ([]byte, error) {
	if options.Format == "" {
		options.Format = FormatSummary
	}
	if err := ValidateOptions(options); err != nil {
		return nil, err
	}
	value = Filter(value, options.Categories, options.Severities)
	switch options.Format {
	case FormatSummary:
		return renderSummary(value), nil
	case FormatMarkdown:
		return renderMarkdown(value), nil
	case FormatJSON:
		return inventory.Marshal(value)
	default:
		return nil, errors.New("unreachable report format")
	}
}

// Filter applies exact-match OR filters within each dimension and ANDs the
// category and severity dimensions. System information is always retained.
func Filter(value inventory.Inventory, categories []inventory.ToolCategory, severities []inventory.FindingSeverity) inventory.Inventory {
	categorySet := make(map[inventory.ToolCategory]bool, len(categories))
	for _, category := range categories {
		categorySet[category] = true
	}
	severitySet := make(map[inventory.FindingSeverity]bool, len(severities))
	for _, severity := range severities {
		severitySet[severity] = true
	}

	tools := make([]inventory.Tool, 0, len(value.Tools))
	includedToolIDs := make(map[string]bool, len(value.Tools))
	for _, tool := range value.Tools {
		if len(categorySet) > 0 && !categorySet[tool.Category] {
			continue
		}
		tools = append(tools, tool)
		includedToolIDs[tool.ID] = true
	}
	findings := make([]inventory.Finding, 0, len(value.Findings))
	for _, finding := range value.Findings {
		if len(severitySet) > 0 && !severitySet[finding.Severity] {
			continue
		}
		if len(categorySet) > 0 && finding.ToolID != "" && !includedToolIDs[finding.ToolID] {
			continue
		}
		findings = append(findings, finding)
	}
	value.Tools = tools
	value.Findings = findings
	return value
}

func validCategory(value inventory.ToolCategory) bool {
	switch value {
	case inventory.CategoryBase, inventory.CategoryRuntime, inventory.CategoryEcosystem,
		inventory.CategoryContainer, inventory.CategorySDK, inventory.CategoryDevOps,
		inventory.CategoryAgent, inventory.CategoryUnknown:
		return true
	default:
		return false
	}
}

func validSeverity(value inventory.FindingSeverity) bool {
	switch value {
	case inventory.SeverityInfo, inventory.SeverityWarning, inventory.SeverityError:
		return true
	default:
		return false
	}
}

func appendUniqueCategory(values []inventory.ToolCategory, candidate inventory.ToolCategory) []inventory.ToolCategory {
	for _, value := range values {
		if value == candidate {
			return values
		}
	}
	return append(values, candidate)
}

func appendUniqueSeverity(values []inventory.FindingSeverity, candidate inventory.FindingSeverity) []inventory.FindingSeverity {
	for _, value := range values {
		if value == candidate {
			return values
		}
	}
	return append(values, candidate)
}

func unknownSystem(collectedAt time.Time) inventory.System {
	return inventory.System{
		OS: inventory.OSMacOS, OSVersion: "unknown", OSBuild: "unknown",
		Architecture: inventory.ArchitectureUnknown, ProcessArchitecture: inventory.ArchitectureUnknown,
		TranslationState: inventory.TranslationStateUnknown,
		Shell:            inventory.Shell{LoginPath: "unknown", LoginName: "unknown", InvokingPath: "unknown", InvokingName: "unknown"},
		PathEntries:      []inventory.PathEntry{},
		Sources:          []inventory.SourceMetadata{{Kind: inventory.SourceUnknown, Name: "macOS system discovery unavailable", CollectedAt: collectedAt, Confidence: inventory.ConfidenceLow}},
	}
}

func normalizeInventory(value *inventory.Inventory) {
	if value.Tools == nil {
		value.Tools = []inventory.Tool{}
	}
	if value.Findings == nil {
		value.Findings = []inventory.Finding{}
	}
	sort.SliceStable(value.Tools, func(i, j int) bool { return value.Tools[i].ID < value.Tools[j].ID })
	for index := range value.Tools {
		sort.SliceStable(value.Tools[index].Installations, func(i, j int) bool {
			return value.Tools[index].Installations[i].ID < value.Tools[index].Installations[j].ID
		})
	}
	sort.SliceStable(value.Findings, func(i, j int) bool {
		if value.Findings[i].Severity != value.Findings[j].Severity {
			return severityRank(value.Findings[i].Severity) > severityRank(value.Findings[j].Severity)
		}
		if value.Findings[i].Code != value.Findings[j].Code {
			return value.Findings[i].Code < value.Findings[j].Code
		}
		return value.Findings[i].ID < value.Findings[j].ID
	})
}

func severityRank(value inventory.FindingSeverity) int {
	switch value {
	case inventory.SeverityError:
		return 3
	case inventory.SeverityWarning:
		return 2
	case inventory.SeverityInfo:
		return 1
	default:
		return 0
	}
}

func scanScopeSource(collectedAt time.Time) inventory.SourceMetadata {
	return inventory.SourceMetadata{
		Kind: inventory.SourceManual, Name: "scan scope: system, PATH, Homebrew, Node.js, Java, jenv, Maven, Gradle",
		CollectedAt: collectedAt, Confidence: inventory.ConfidenceHigh,
	}
}
