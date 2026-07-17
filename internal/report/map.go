package report

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gitbagHero/EnvMason/internal/adapter/homebrew"
	"github.com/gitbagHero/EnvMason/internal/adapter/java"
	"github.com/gitbagHero/EnvMason/internal/adapter/nodejs"
	"github.com/gitbagHero/EnvMason/internal/inventory"
)

const unknown = "unknown"

func mapHomebrew(result homebrew.Result, collectedAt time.Time) []inventory.Tool {
	tools := append([]inventory.Tool{}, result.Tools...)
	if result.State == homebrew.StateInstalled || result.State == homebrew.StateUnknown {
		tools = append(tools, inventory.Tool{
			ID: "manager.homebrew", DisplayName: "Homebrew", Category: inventory.CategoryEcosystem,
			Installations: []inventory.Installation{installation(
				"homebrew:manager", valueOrUnknown(result.Version), valueOrUnknown(result.BrewPath),
				result.Architecture, "homebrew", inventory.ActiveStateActive, inventory.DefaultStateDefault,
				inventory.SourcePackageManager, "brew read-only adapter", collectedAt,
			)},
		})
	}
	return tools
}

func mapNode(result nodejs.Result, collectedAt time.Time) []inventory.Tool {
	tools := []inventory.Tool{}
	if len(result.Nodes) > 0 {
		installations := make([]inventory.Installation, 0, len(result.Nodes))
		for _, node := range result.Nodes {
			active := inventory.ActiveStateInactive
			if node.Effective {
				active = inventory.ActiveStateActive
			}
			defaultState := inventory.DefaultStateNonDefault
			if node.Default {
				defaultState = inventory.DefaultStateDefault
			}
			item := installation(node.ID, valueOrUnknown(node.Version), valueOrUnknown(node.Path), node.Architecture,
				string(node.Manager), active, defaultState, inventory.SourceCommand, "Node.js read-only adapter", collectedAt)
			item.NormalizedVersion = node.NormalizedVersion
			installations = append(installations, item)
		}
		tools = append(tools, inventory.Tool{ID: "runtime.node", DisplayName: "Node.js", Category: inventory.CategoryRuntime, Installations: installations})
	}

	byName := make(map[string][]nodejs.PackageManager)
	for _, manager := range result.PackageManagers {
		byName[manager.Name] = append(byName[manager.Name], manager)
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		installations := []inventory.Installation{}
		for index, manager := range byName[name] {
			active := inventory.ActiveStateInactive
			if manager.Effective {
				active = inventory.ActiveStateActive
			}
			item := installation(fmt.Sprintf("node:%s:%02d", name, index+1), valueOrUnknown(manager.Version), valueOrUnknown(manager.Path),
				inventory.ArchitectureUnknown, string(manager.Manager), active, inventory.DefaultStateUnknown,
				inventory.SourceCommand, name+" read-only discovery", collectedAt)
			installations = append(installations, item)
		}
		tools = append(tools, inventory.Tool{ID: "ecosystem." + safeIDPart(name), DisplayName: name, Category: inventory.CategoryEcosystem, Installations: installations})
	}
	return tools
}

func mapJava(result java.Result, collectedAt time.Time) []inventory.Tool {
	tools := []inventory.Tool{}
	if len(result.JDKs) > 0 {
		installations := make([]inventory.Installation, 0, len(result.JDKs))
		for _, jdk := range result.JDKs {
			active := inventory.ActiveStateInactive
			if jdk.ID == result.Current.JDKID {
				active = inventory.ActiveStateActive
			}
			defaultState := inventory.DefaultStateUnknown
			if contains(jdk.JenvAliases, result.Jenv.EffectiveVersion) {
				defaultState = inventory.DefaultStateDefault
			}
			item := installation(jdk.ID, valueOrUnknown(jdk.Version), valueOrUnknown(jdk.Home), jdk.Architecture,
				string(jdk.Manager), active, defaultState, inventory.SourceFile, "JDK metadata and registrations", collectedAt)
			if len(jdk.JenvAliases) > 0 {
				item.Sources = append(item.Sources, inventory.SourceMetadata{
					Kind: inventory.SourceFile, Name: "jenv aliases: " + strings.Join(jdk.JenvAliases, ", "),
					CollectedAt: collectedAt, Confidence: inventory.ConfidenceHigh,
				})
			}
			installations = append(installations, item)
		}
		tools = append(tools, inventory.Tool{ID: "runtime.java", DisplayName: "Java Development Kit", Category: inventory.CategoryRuntime, Installations: installations})
	}
	if result.Maven.State != java.StateNotInstalled {
		tools = append(tools, buildTool("ecosystem.maven", "Maven", result.Maven, collectedAt))
	}
	if result.Gradle.State != java.StateNotInstalled {
		tools = append(tools, buildTool("ecosystem.gradle", "Gradle", result.Gradle, collectedAt))
	}
	return tools
}

func buildTool(id, displayName string, tool java.BuildTool, collectedAt time.Time) inventory.Tool {
	return inventory.Tool{
		ID: id, DisplayName: displayName, Category: inventory.CategoryEcosystem,
		Installations: []inventory.Installation{installation(
			id+":active", valueOrUnknown(tool.Version), valueOrUnknown(tool.Path), inventory.ArchitectureUnknown,
			"unknown", inventory.ActiveStateActive, inventory.DefaultStateUnknown,
			inventory.SourceCommand, displayName+" read-only discovery", collectedAt,
		)},
	}
}

func installation(id, version, path string, architecture inventory.Architecture, manager string, active inventory.ActiveState, defaultState inventory.DefaultState, kind inventory.SourceKind, sourceName string, collectedAt time.Time) inventory.Installation {
	if architecture == "" {
		architecture = inventory.ArchitectureUnknown
	}
	if manager == "" {
		manager = unknown
	}
	return inventory.Installation{
		ID: id, Version: version, Path: path, Architecture: architecture, Manager: manager,
		ActiveState: active, DefaultState: defaultState, InstallReason: inventory.InstallReasonUnknown,
		Sources: []inventory.SourceMetadata{{Kind: kind, Name: sourceName, CollectedAt: collectedAt, Confidence: inventory.ConfidenceHigh}},
	}
}

func appendNodeFacts(value *inventory.Inventory, result nodejs.Result, collectedAt time.Time) {
	if result.NVM.State == nodejs.StateInstalled {
		appendFact(value, "NVM_DEFAULT_SELECTION", "NVM default selection was discovered.",
			[]string{"directory=" + valueOrUnknown(result.NVM.Directory), "loaded=" + fmt.Sprint(result.NVM.Loaded), "alias=" + valueOrUnknown(result.NVM.DefaultAlias), "version=" + valueOrUnknown(result.NVM.DefaultVersion)},
			"runtime.node", inventory.SourceFile, "NVM alias metadata", collectedAt)
	}
	associations := []string{}
	for _, manager := range result.PackageManagers {
		if manager.NodeInstallationID != "" {
			associations = append(associations, manager.Name+"="+manager.NodeInstallationID)
		}
	}
	if len(associations) > 0 {
		sort.Strings(associations)
		appendFact(value, "NODE_PACKAGE_MANAGER_RUNTIME_ASSOCIATIONS", "Package-manager runtime associations were discovered.",
			associations, "runtime.node", inventory.SourceFile, "Node.js package-manager ownership", collectedAt)
	}
}

func appendHomebrewFacts(value *inventory.Inventory, result homebrew.Result, collectedAt time.Time) {
	if result.State != homebrew.StateInstalled {
		return
	}
	appendFact(value, "HOMEBREW_CONFIGURATION", "Homebrew installation metadata was discovered.", []string{
		"prefix=" + valueOrUnknown(result.Prefix), "repository=" + valueOrUnknown(result.Repository),
		"cellar=" + valueOrUnknown(result.Cellar), "caskroom=" + valueOrUnknown(result.Caskroom),
		"origin=" + valueOrUnknown(result.Origin), "data_format=" + valueOrUnknown(result.DataFormat),
	}, "manager.homebrew", inventory.SourcePackageManager, "Homebrew path and repository queries", collectedAt)
}

func appendJavaFacts(value *inventory.Inventory, result java.Result, collectedAt time.Time) {
	if result.JavaHome != "" && result.JavaHome != unknown {
		appendFact(value, "JAVA_HOME_SELECTION", "JAVA_HOME selection was discovered.", []string{"home=" + result.JavaHome},
			"runtime.java", inventory.SourceEnvironment, "JAVA_HOME", collectedAt)
	}
	for code, selection := range map[string]string{
		"JENV_GLOBAL_SELECTION": result.Jenv.GlobalVersion,
		"JENV_LOCAL_SELECTION":  result.Jenv.LocalVersion,
		"JENV_SHELL_SELECTION":  result.Jenv.ShellVersion,
	} {
		if selection != "" && selection != unknown {
			appendFact(value, code, "A jenv version selection was discovered.", []string{"version=" + selection},
				"runtime.java", inventory.SourceFile, strings.ToLower(strings.ReplaceAll(code, "_", " ")), collectedAt)
		}
	}
	appendBuildRuntimeFact(value, "MAVEN_RUNTIME", "ecosystem.maven", result.Maven, collectedAt)
	appendBuildRuntimeFact(value, "GRADLE_RUNTIME", "ecosystem.gradle", result.Gradle, collectedAt)
}

func appendBuildRuntimeFact(value *inventory.Inventory, code, toolID string, tool java.BuildTool, collectedAt time.Time) {
	if tool.State != java.StateInstalled || (tool.JavaVersion == "" && tool.JavaHome == "") {
		return
	}
	appendFact(value, code, "A build tool's Java runtime was discovered.",
		[]string{"java_version=" + valueOrUnknown(tool.JavaVersion), "java_home=" + valueOrUnknown(tool.JavaHome)},
		toolID, inventory.SourceCommand, strings.ToLower(tool.Name)+" runtime metadata", collectedAt)
}

func appendFact(value *inventory.Inventory, code, message string, evidence []string, toolID string, kind inventory.SourceKind, sourceName string, collectedAt time.Time) {
	value.Findings = append(value.Findings, inventory.Finding{
		ID: fmt.Sprintf("report-fact-%03d", len(value.Findings)+1), Code: code, Severity: inventory.SeverityInfo,
		Message: message, Evidence: evidence, Confidence: inventory.ConfidenceHigh, ToolID: toolID,
		Sources: []inventory.SourceMetadata{{Kind: kind, Name: sourceName, CollectedAt: collectedAt, Confidence: inventory.ConfidenceHigh}},
	})
}

func valueOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return unknown
	}
	return value
}

func safeIDPart(value string) string {
	var builder strings.Builder
	for _, character := range strings.ToLower(value) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' || character == '_' {
			builder.WriteRune(character)
		}
	}
	if builder.Len() == 0 || builder.String()[0] < 'a' || builder.String()[0] > 'z' {
		return "unknown"
	}
	return builder.String()
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
