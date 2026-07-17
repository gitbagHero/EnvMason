package report

import (
	"fmt"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/versiondata"
)

func appendVersionData(value *inventory.Inventory, result versiondata.Result) {
	sources := make(map[string]versiondata.Source, len(result.Sources))
	for _, source := range result.Sources {
		sources[source.ID] = source
	}
	if result.Node.LatestStable != "" || result.Node.LatestLTS != "" || len(result.Node.Lifecycle) > 0 {
		value.Findings = append(value.Findings, inventory.Finding{
			ID: "remote-node-version-data", Code: "REMOTE_NODE_VERSION_DATA", Severity: inventory.SeverityInfo,
			Message: "Official Node.js release and lifecycle data was collected.",
			Evidence: []string{
				versionEvidence("Latest stable", result.Node.LatestStable, result.Node.LatestStableFreshness),
				versionEvidence("Latest LTS", result.Node.LatestLTS, result.Node.LatestLTSFreshness),
				lifecycleEvidence(result.Node.Lifecycle),
			},
			Confidence: confidenceFor(result.Node.LatestStableFreshness, result.Node.LatestLTSFreshness), ToolID: "runtime.node",
			Sources: sourcesFor(value.GeneratedAt, sources, "node-releases", "node-schedule"),
		})
	}
	if result.Java.LatestFeature > 0 || result.Java.LatestLTS > 0 || len(result.Java.TemurinLifecycle) > 0 {
		value.Findings = append(value.Findings, inventory.Finding{
			ID: "remote-java-version-data", Code: "REMOTE_JAVA_VERSION_DATA", Severity: inventory.SeverityInfo,
			Message: "Official Adoptium release data and Temurin-specific lifecycle data was collected.",
			Evidence: []string{
				versionEvidence("Latest feature", intString(result.Java.LatestFeature), result.Java.LatestFeatureFreshness),
				versionEvidence("Latest LTS", intString(result.Java.LatestLTS), result.Java.LatestLTSFreshness),
				temurinLifecycleEvidence(result.Java.TemurinLifecycle),
				"Lifecycle status applies only to Eclipse Temurin; other JDK vendors remain Unknown.",
			},
			Confidence: confidenceFor(result.Java.LatestFeatureFreshness, result.Java.LatestLTSFreshness), ToolID: "runtime.java",
			Sources: sourcesFor(value.GeneratedAt, sources, "java-releases", "temurin-support"),
		})
	}
	for index, issue := range result.Issues {
		message := "Remote version data is unavailable; the local report remains usable."
		if issue.Code == "VERSION_SOURCE_STALE" {
			message = "Only stale remote version data is available and it is not confirmed latest."
		} else if issue.Code == "VERSION_CACHE_CORRUPT" {
			message = "Cached remote version data was corrupt and was ignored."
		} else if issue.Code == "VERSION_CACHE_UNAVAILABLE" {
			message = "The configured version-data cache could not be read."
		}
		value.Findings = append(value.Findings, inventory.Finding{
			ID: fmt.Sprintf("remote-version-issue-%03d", index+1), Code: issue.Code, Severity: inventory.SeverityWarning,
			Message: message, Evidence: []string{issue.SourceID}, Confidence: inventory.ConfidenceHigh,
			Sources: sourcesFor(value.GeneratedAt, sources, issue.SourceID),
		})
	}
	normalizeInventory(value)
}

func versionEvidence(label, value string, freshness versiondata.Freshness) string {
	if value == "" {
		return label + ": Unknown"
	}
	if freshness == versiondata.FreshnessStale {
		return fmt.Sprintf("%s candidate: %s (stale; not confirmed latest)", label, value)
	}
	return fmt.Sprintf("%s: %s (fresh)", label, value)
}

func lifecycleEvidence(entries []versiondata.NodeLifecycle) string {
	counts := map[versiondata.Lifecycle]int{}
	for _, entry := range entries {
		counts[entry.State]++
	}
	return fmt.Sprintf("Node lifecycle entries: LTS=%d, stable=%d, EOL=%d, Unknown=%d", counts[versiondata.LifecycleLTS], counts[versiondata.LifecycleStable], counts[versiondata.LifecycleEOL], counts[versiondata.LifecycleUnknown])
}

func temurinLifecycleEvidence(entries []versiondata.JavaLifecycle) string {
	counts := map[versiondata.Lifecycle]int{}
	for _, entry := range entries {
		counts[entry.State]++
	}
	return fmt.Sprintf("Temurin lifecycle entries: LTS=%d, stable=%d, EOL=%d, Unknown=%d", counts[versiondata.LifecycleLTS], counts[versiondata.LifecycleStable], counts[versiondata.LifecycleEOL], counts[versiondata.LifecycleUnknown])
}

func sourcesFor(generatedAt time.Time, values map[string]versiondata.Source, ids ...string) []inventory.SourceMetadata {
	result := make([]inventory.SourceMetadata, 0, len(ids))
	for _, id := range ids {
		source, exists := values[id]
		if !exists {
			continue
		}
		collectedAt := source.FetchedAt
		if collectedAt.IsZero() {
			collectedAt = generatedAt
		}
		confidence := inventory.ConfidenceHigh
		if source.Freshness == versiondata.FreshnessStale {
			confidence = inventory.ConfidenceLow
		} else if source.Freshness == versiondata.FreshnessUnavailable {
			confidence = inventory.ConfidenceUnknown
		}
		result = append(result, inventory.SourceMetadata{
			Kind: inventory.SourceUnknown, Name: fmt.Sprintf("%s [%s; %s]", source.Name, source.URL, source.Freshness),
			CollectedAt: collectedAt, Confidence: confidence,
		})
	}
	return result
}

func confidenceFor(values ...versiondata.Freshness) inventory.Confidence {
	for _, value := range values {
		if value != versiondata.FreshnessFresh {
			return inventory.ConfidenceLow
		}
	}
	return inventory.ConfidenceHigh
}

func intString(value int) string {
	if value <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", value)
}
