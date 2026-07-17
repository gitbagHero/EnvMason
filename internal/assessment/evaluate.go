package assessment

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/projectscan"
	"github.com/gitbagHero/EnvMason/internal/version"
	"github.com/gitbagHero/EnvMason/internal/versiondata"
)

func Evaluate(input Input) []inventory.Finding {
	policy := input.Policy
	if policy.SchemaVersion == "" {
		policy = DefaultPolicy()
	}
	findings := []inventory.Finding{}
	findings = append(findings, projectRetention(input)...)
	findings = append(findings, nodeAssessment(input, policy.Tools["runtime.node"])...)
	findings = append(findings, javaAssessment(input, policy.Tools["runtime.java"])...)
	findings = append(findings, conflictAssessment(input.Inventory)...)
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].ToolID != findings[j].ToolID {
			return findings[i].ToolID < findings[j].ToolID
		}
		if findings[i].Code != findings[j].Code {
			return findings[i].Code < findings[j].Code
		}
		return findings[i].ID < findings[j].ID
	})
	return findings
}

func nodeAssessment(input Input, policy ToolPolicy) []inventory.Finding {
	tool, ok := findTool(input.Inventory, "runtime.node")
	if !ok || len(tool.Installations) == 0 {
		return nil
	}
	current, ok := effectiveInstallation(tool.Installations)
	if !ok {
		return []inventory.Finding{finding(input, "assessment-node-unknown", "NODE_VERSION_UNKNOWN", inventory.SeverityWarning, inventory.StatusUnknown,
			"A single effective Node.js installation could not be determined.", []string{"active_installation=Unknown"}, inventory.ConfidenceLow,
			"Resolve the active/default Node.js selection before planning an update.", []string{"An update target could affect a different installation."}, "runtime.node", "")}
	}

	result := []inventory.Finding{}
	if lifecycle := nodeLifecycle(input.Versions.Node, semverMajor(current.Version)); lifecycle == versiondata.LifecycleEOL {
		result = append(result, finding(input, "assessment-node-eol", "NODE_VERSION_EOL", inventory.SeverityWarning, inventory.StatusEOL,
			"The effective Node.js major line is end-of-life.", []string{"installed=" + current.Version, "lifecycle=eol"}, inventory.ConfidenceHigh,
			"Plan a move to a supported line while retaining project-required versions.", []string{"Unsupported runtimes may no longer receive fixes."}, "runtime.node", current.ID))
	}

	target, targetEvidence, fresh := nodeTarget(input.Versions.Node, policy)
	if !fresh {
		result = append(result, finding(input, "assessment-node-recommendation-unknown", "NODE_RECOMMENDATION_UNKNOWN", inventory.SeverityInfo, inventory.StatusUnknown,
			"A deterministic Node.js update recommendation is unavailable.", []string{"installed=" + current.Version, targetEvidence}, inventory.ConfidenceLow,
			"Refresh official version data or provide a valid pin before planning an update.", []string{"No update conclusion is produced from stale or missing data."}, "runtime.node", current.ID))
		return result
	}
	return append(result, compareNode(input, current, target, targetEvidence, policy))
}

func compareNode(input Input, current inventory.Installation, target, targetEvidence string, policy ToolPolicy) inventory.Finding {
	relation := version.Compare(version.ParseSemVer(current.Version), version.ParseSemVer(target))
	evidence := []string{"installed=" + current.Version, "target=" + target, targetEvidence}
	switch relation {
	case version.RelationLess:
		if policy.IgnoreUpdates {
			return finding(input, "assessment-node-update-ignored", "NODE_UPDATE_IGNORED", inventory.SeverityInfo, inventory.StatusIgnored,
				"A Node.js update is available but ordinary update advice is ignored by policy.", evidence, inventory.ConfidenceHigh,
				"Keep the policy under review; EOL and conflicts remain visible.", []string{"No update plan should be proposed while ignore_updates is true."}, "runtime.node", current.ID)
		}
		return finding(input, "assessment-node-update", "NODE_UPDATE_AVAILABLE", inventory.SeverityInfo, inventory.StatusUpdateAvailable,
			"The effective Node.js version is older than the selected target.", evidence, inventory.ConfidenceHigh,
			"Review a plan to install the selected target without deleting project-required versions.", []string{"Installing a runtime is an R2 operation and requires a confirmed Plan."}, "runtime.node", current.ID)
	case version.RelationEqual:
		return finding(input, "assessment-node-recommended", "NODE_VERSION_RECOMMENDED", inventory.SeverityInfo, inventory.StatusRecommended,
			"The effective Node.js version matches the selected target.", evidence, inventory.ConfidenceHigh,
			"No runtime update is recommended.", []string{"Existing project-required versions should still be retained."}, "runtime.node", current.ID)
	case version.RelationGreater:
		return finding(input, "assessment-node-channel-mismatch", "NODE_CHANNEL_MISMATCH", inventory.SeverityWarning, inventory.StatusConflict,
			"The effective Node.js version is newer than the selected channel target.", evidence, inventory.ConfidenceHigh,
			"Review the selected stable/LTS channel and project requirements before changing defaults.", []string{"A newer Current release is not automatically the recommended LTS release."}, "runtime.node", current.ID)
	default:
		return finding(input, "assessment-node-version-unknown", "NODE_VERSION_UNKNOWN", inventory.SeverityWarning, inventory.StatusUnknown,
			"The effective Node.js version cannot be compared safely.", evidence, inventory.ConfidenceLow,
			"Inspect the reported version and installation source.", []string{"No update conclusion is produced for an incomparable version."}, "runtime.node", current.ID)
	}
}

func javaAssessment(input Input, policy ToolPolicy) []inventory.Finding {
	tool, ok := findTool(input.Inventory, "runtime.java")
	if !ok || len(tool.Installations) == 0 {
		return nil
	}
	current, ok := effectiveInstallation(tool.Installations)
	if !ok {
		return []inventory.Finding{finding(input, "assessment-java-unknown", "JAVA_VERSION_UNKNOWN", inventory.SeverityWarning, inventory.StatusUnknown,
			"A single effective Java installation could not be determined.", []string{"active_installation=Unknown"}, inventory.ConfidenceLow,
			"Resolve the active Java selection before planning an update.", []string{"Build tools may use a different Java installation."}, "runtime.java", "")}
	}
	result := []inventory.Finding{}
	vendor := normalizeJavaVendor(input.JavaVendors[current.ID])
	major := javaMajor(current.Version)
	if lifecycle := input.Versions.Java.LifecycleForVendor(major, vendor); lifecycle == versiondata.LifecycleEOL {
		result = append(result, finding(input, "assessment-java-eol", "JAVA_VERSION_EOL", inventory.SeverityWarning, inventory.StatusEOL,
			"The effective Eclipse Temurin Java line is end-of-life.", []string{"installed=" + current.Version, "vendor=temurin", "lifecycle=eol"}, inventory.ConfidenceHigh,
			"Review a move to a supported Temurin LTS while retaining project-required JDKs.", []string{"Unsupported runtimes may no longer receive fixes."}, "runtime.java", current.ID))
	}

	target, targetEvidence, fresh := javaTarget(input.Versions.Java, policy)
	if !fresh {
		result = append(result, finding(input, "assessment-java-recommendation-unknown", "JAVA_RECOMMENDATION_UNKNOWN", inventory.SeverityInfo, inventory.StatusUnknown,
			"A deterministic Java update recommendation is unavailable.", []string{"installed=" + current.Version, targetEvidence}, inventory.ConfidenceLow,
			"Refresh official version data or provide a valid pin before planning an update.", []string{"Unknown vendor lifecycle data never becomes a definite EOL conclusion."}, "runtime.java", current.ID))
		return result
	}
	relation := version.Compare(version.ParseJava(current.Version), version.ParseJava(target))
	evidence := []string{"installed=" + current.Version, "target=" + target, targetEvidence}
	switch relation {
	case version.RelationLess:
		if policy.IgnoreUpdates {
			result = append(result, finding(input, "assessment-java-update-ignored", "JAVA_UPDATE_IGNORED", inventory.SeverityInfo, inventory.StatusIgnored,
				"A Java update is available but ordinary update advice is ignored by policy.", evidence, inventory.ConfidenceHigh,
				"Keep the policy under review; EOL and conflicts remain visible.", []string{"No update plan should be proposed while ignore_updates is true."}, "runtime.java", current.ID))
		} else {
			result = append(result, finding(input, "assessment-java-update", "JAVA_UPDATE_AVAILABLE", inventory.SeverityInfo, inventory.StatusUpdateAvailable,
				"The effective Java version is older than the selected target.", evidence, inventory.ConfidenceHigh,
				"Review project requirements before planning a Java installation.", []string{"Installing a runtime is an R2 operation and requires a confirmed Plan."}, "runtime.java", current.ID))
		}
	case version.RelationEqual:
		result = append(result, finding(input, "assessment-java-recommended", "JAVA_VERSION_RECOMMENDED", inventory.SeverityInfo, inventory.StatusRecommended,
			"The effective Java feature line matches the selected target.", evidence, inventory.ConfidenceHigh,
			"No runtime feature-line update is recommended.", []string{"Patch maintenance remains vendor-specific."}, "runtime.java", current.ID))
	case version.RelationGreater:
		result = append(result, finding(input, "assessment-java-channel-mismatch", "JAVA_CHANNEL_MISMATCH", inventory.SeverityWarning, inventory.StatusConflict,
			"The effective Java feature line is newer than the selected channel target.", evidence, inventory.ConfidenceHigh,
			"Review the selected stable/LTS channel and project requirements before changing defaults.", []string{"A newer feature release is not automatically the recommended LTS."}, "runtime.java", current.ID))
	default:
		result = append(result, finding(input, "assessment-java-version-unknown", "JAVA_VERSION_UNKNOWN", inventory.SeverityWarning, inventory.StatusUnknown,
			"The effective Java version cannot be compared safely.", evidence, inventory.ConfidenceLow,
			"Inspect the reported version and vendor.", []string{"No update conclusion is produced for an incomparable version."}, "runtime.java", current.ID))
	}
	return result
}

func projectRetention(input Input) []inventory.Finding {
	result := []inventory.Finding{}
	for _, project := range input.Projects.Projects {
		for _, reference := range project.References {
			if reference.Kind != projectscan.ConstraintExact || reference.Normalized == "" {
				continue
			}
			toolID := runtimeToolID(reference.Runtime)
			tool, ok := findTool(input.Inventory, toolID)
			if !ok {
				continue
			}
			for _, installation := range tool.Installations {
				if !sameRuntimeVersion(reference.Runtime, installation.Version, reference.Constraint, reference.Normalized) {
					continue
				}
				id := fmt.Sprintf("assessment-retain-%s-%s-%s-%s", safeID(project.ID), safeID(installation.ID), safeID(reference.File), safeID(reference.Normalized))
				result = append(result, finding(input, id, strings.ToUpper(string(reference.Runtime))+"_PROJECT_VERSION_RETAIN", inventory.SeverityInfo, inventory.StatusRetainRequired,
					"An installed runtime version is referenced by an explicitly scanned project.", []string{"project=" + project.Root, "constraint=" + reference.Constraint, "file=" + reference.File, "installed=" + installation.Version}, inventory.ConfidenceHigh,
					"Retain this installation unless the project requirement is changed and rescanned.", []string{"Removal would make the referenced project environment unavailable."}, toolID, installation.ID))
			}
		}
	}
	return result
}

func conflictAssessment(value inventory.Inventory) []inventory.Finding {
	result := []inventory.Finding{}
	if tool, ok := findTool(value, "runtime.node"); ok {
		managers := map[string]bool{}
		active, defaults := []string{}, []string{}
		for _, item := range tool.Installations {
			managers[item.Manager] = true
			if item.ActiveState == inventory.ActiveStateActive {
				active = append(active, item.ID+"="+item.Version)
			}
			if item.DefaultState == inventory.DefaultStateDefault {
				defaults = append(defaults, item.ID+"="+item.Version)
			}
		}
		if len(managers) > 1 {
			result = append(result, assessmentConflict(value, "assessment-node-multiple-sources", "NODE_MULTIPLE_SOURCES_CONFLICT", "runtime.node", "Node.js installations from multiple managers require review.", sortedKeys(managers), "Choose the intended manager before changing an active or default version."))
		}
		if len(active) != 1 || len(defaults) > 1 || (len(defaults) == 1 && len(active) == 1 && strings.SplitN(defaults[0], "=", 2)[0] != strings.SplitN(active[0], "=", 2)[0]) {
			evidence := append([]string{"active=" + strings.Join(active, ",")}, "default="+strings.Join(defaults, ","))
			result = append(result, assessmentConflict(value, "assessment-node-default-conflict", "NODE_DEFAULT_VERSION_CONFLICT", "runtime.node", "The effective and default Node.js selections are inconsistent or ambiguous.", evidence, "Resolve the default selection explicitly before planning an update."))
		}
	}
	for _, existing := range value.Findings {
		switch existing.Code {
		case "MAVEN_JAVA_MISMATCH", "GRADLE_JAVA_MISMATCH", "JAVA_HOME_RUNTIME_MISMATCH", "JENV_RUNTIME_MISMATCH":
			result = append(result, finding(Input{Inventory: value}, "assessment-"+strings.ToLower(existing.Code), existing.Code+"_ASSESSMENT", inventory.SeverityWarning, inventory.StatusConflict,
				"Java runtime selections disagree and require investigation.", append([]string{"source_finding=" + existing.Code}, existing.Evidence...), existing.Confidence,
				"Align the shell, build-tool and version-manager selections only after reviewing project requirements.", []string{"Changing a default Java runtime may affect Maven, Gradle or project builds."}, "runtime.java", existing.InstallationID))
		}
	}
	return result
}

func assessmentConflict(value inventory.Inventory, id, code, toolID, message string, evidence []string, recommendation string) inventory.Finding {
	return finding(Input{Inventory: value}, id, code, inventory.SeverityWarning, inventory.StatusConflict, message, evidence, inventory.ConfidenceHigh, recommendation,
		[]string{"Changing defaults may affect shells and projects that resolve another installation."}, toolID, "")
}

func nodeTarget(data versiondata.NodeData, policy ToolPolicy) (string, string, bool) {
	if policy.Pin != "" {
		return policy.Pin, "policy=pin", true
	}
	if policy.Channel == ChannelStable {
		return data.LatestStable, "policy_channel=stable", data.LatestStable != "" && data.LatestStableFreshness == versiondata.FreshnessFresh
	}
	return data.LatestLTS, "policy_channel=lts", data.LatestLTS != "" && data.LatestLTSFreshness == versiondata.FreshnessFresh
}

func javaTarget(data versiondata.JavaData, policy ToolPolicy) (string, string, bool) {
	if policy.Pin != "" {
		return policy.Pin, "policy=pin", true
	}
	if policy.Channel == ChannelStable {
		return strconv.Itoa(data.LatestFeature), "policy_channel=stable", data.LatestFeature > 0 && data.LatestFeatureFreshness == versiondata.FreshnessFresh
	}
	return strconv.Itoa(data.LatestLTS), "policy_channel=lts", data.LatestLTS > 0 && data.LatestLTSFreshness == versiondata.FreshnessFresh
}

func nodeLifecycle(data versiondata.NodeData, major int) versiondata.Lifecycle {
	for _, entry := range data.Lifecycle {
		if entry.Major == major && entry.Freshness == versiondata.FreshnessFresh {
			return entry.State
		}
	}
	return versiondata.LifecycleUnknown
}

func finding(input Input, id, code string, severity inventory.FindingSeverity, status inventory.FindingStatus, message string, evidence []string, confidence inventory.Confidence, recommendation string, impact []string, toolID, installationID string) inventory.Finding {
	return inventory.Finding{ID: id, Code: code, Severity: severity, Status: status, Message: message, Evidence: evidence, Confidence: confidence,
		Recommendation: recommendation, Impact: impact, ToolID: toolID, InstallationID: installationID,
		Sources: []inventory.SourceMetadata{{Kind: inventory.SourceManual, Name: "EnvMason deterministic assessment rules 0.1.0", CollectedAt: input.Inventory.GeneratedAt, Confidence: inventory.ConfidenceHigh}}}
}

func findTool(value inventory.Inventory, id string) (inventory.Tool, bool) {
	for _, tool := range value.Tools {
		if tool.ID == id {
			return tool, true
		}
	}
	return inventory.Tool{}, false
}

func effectiveInstallation(values []inventory.Installation) (inventory.Installation, bool) {
	active := []inventory.Installation{}
	for _, item := range values {
		if item.ActiveState == inventory.ActiveStateActive {
			active = append(active, item)
		}
	}
	if len(active) == 1 {
		return active[0], true
	}
	return inventory.Installation{}, false
}

func semverMajor(raw string) int {
	parsed := version.ParseSemVer(raw)
	if !parsed.Comparable {
		return 0
	}
	value, _ := strconv.Atoi(strings.Split(parsed.Normalized, ".")[0])
	return value
}

func javaMajor(raw string) int {
	parsed := version.ParseJava(raw)
	if !parsed.Comparable {
		return 0
	}
	value, _ := strconv.Atoi(strings.Split(parsed.Normalized, ".")[0])
	return value
}

func sameRuntimeVersion(runtime projectscan.Runtime, installed, constraint, required string) bool {
	if runtime == projectscan.RuntimeNode {
		if precision := numericVersionPrecision(constraint); precision > 0 && precision < 3 {
			return sameNormalizedPrefix(version.ParseSemVer(installed).Normalized, required, precision)
		}
		return version.Compare(version.ParseSemVer(installed), version.ParseSemVer(required)) == version.RelationEqual
	}
	if precision := numericVersionPrecision(constraint); precision > 0 && precision < 3 {
		return sameNormalizedPrefix(version.ParseJava(installed).Normalized, required, precision)
	}
	return version.Compare(version.ParseJava(installed), version.ParseJava(required)) == version.RelationEqual
}

func numericVersionPrecision(value string) int {
	value = strings.TrimPrefix(value, "v")
	parts := strings.Split(value, ".")
	for _, part := range parts {
		if part == "" {
			return 0
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return 0
			}
		}
	}
	return len(parts)
}

func sameNormalizedPrefix(installed, required string, precision int) bool {
	if installed == "" {
		return false
	}
	installedParts := strings.Split(installed, ".")
	requiredParts := strings.Split(strings.TrimPrefix(required, "v"), ".")
	if len(installedParts) < precision || len(requiredParts) < precision {
		return false
	}
	for index := 0; index < precision; index++ {
		left, leftErr := strconv.Atoi(installedParts[index])
		right, rightErr := strconv.Atoi(requiredParts[index])
		if leftErr != nil || rightErr != nil || left != right {
			return false
		}
	}
	return true
}

func normalizeJavaVendor(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "temurin" || value == "adoptium" || value == "eclipse adoptium" || strings.Contains(value, "eclipse temurin") {
		return "temurin"
	}
	return value
}

func runtimeToolID(runtime projectscan.Runtime) string {
	if runtime == projectscan.RuntimeNode {
		return "runtime.node"
	}
	if runtime == projectscan.RuntimeJava {
		return "runtime.java"
	}
	return ""
}

func safeID(value string) string {
	var result strings.Builder
	for _, character := range strings.ToLower(value) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-' {
			result.WriteRune(character)
		} else {
			result.WriteByte('-')
		}
	}
	return strings.Trim(result.String(), "-")
}

func sortedKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, "manager="+value)
	}
	sort.Strings(result)
	return result
}
