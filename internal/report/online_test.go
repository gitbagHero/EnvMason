package report

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/versiondata"
)

func TestGenerateOnlyCollectsRemoteDataWhenExplicit(t *testing.T) {
	collected := 0
	scan := func(context.Context) (inventory.Inventory, error) { return reportFixture(), nil }
	collect := func(context.Context) versiondata.Result {
		collected++
		return onlineFixture()
	}
	if _, err := generate(context.Background(), Options{Format: FormatSummary}, scan, collect); err != nil {
		t.Fatal(err)
	}
	if collected != 0 {
		t.Fatalf("default report made %d online collections", collected)
	}
	data, err := generate(context.Background(), Options{Format: FormatMarkdown, Online: true}, scan, collect)
	if err != nil {
		t.Fatal(err)
	}
	if collected != 1 || !strings.Contains(string(data), "REMOTE_NODE_VERSION_DATA") || !strings.Contains(string(data), "node.example/index.json") || !strings.Contains(string(data), "2026-07-17") {
		t.Fatalf("online report = collected %d\n%s", collected, data)
	}
}

func TestStaleDataIsNeverPresentedAsConfirmedLatest(t *testing.T) {
	value := reportFixture()
	result := onlineFixture()
	result.Node.LatestStableFreshness = versiondata.FreshnessStale
	result.Node.LatestLTSFreshness = versiondata.FreshnessStale
	result.Sources[0].Freshness = versiondata.FreshnessStale
	result.Issues = []versiondata.Issue{{Code: "VERSION_SOURCE_STALE", SourceID: "node-releases"}}
	appendVersionData(&value, result)
	data := string(renderSummary(value))
	if !strings.Contains(data, "stale; not confirmed latest") || !strings.Contains(data, "VERSION_SOURCE_STALE") {
		t.Fatalf("stale report did not degrade honestly:\n%s", data)
	}
	if !strings.Contains(data, "Status: incomplete") {
		t.Fatalf("stale report was not marked incomplete:\n%s", data)
	}
}

func TestUnavailableRemoteSourcesDoNotFailLocalReport(t *testing.T) {
	data, err := generate(context.Background(), Options{Format: FormatJSON, Online: true}, func(context.Context) (inventory.Inventory, error) {
		return reportFixture(), nil
	}, func(context.Context) versiondata.Result {
		return versiondata.Result{
			Sources: []versiondata.Source{{ID: "node-releases", Name: "Node source", URL: "https://node.example", Freshness: versiondata.FreshnessUnavailable}},
			Issues:  []versiondata.Issue{{Code: "VERSION_SOURCE_UNAVAILABLE", SourceID: "node-releases"}},
		}
	})
	if err != nil || !strings.Contains(string(data), "VERSION_SOURCE_UNAVAILABLE") {
		t.Fatalf("degraded report = %v\n%s", err, data)
	}
}

func onlineFixture() versiondata.Result {
	fetchedAt := time.Date(2026, 7, 17, 5, 0, 0, 0, time.UTC)
	return versiondata.Result{
		Node: versiondata.NodeData{LatestStable: "v26.1.0", LatestStableFreshness: versiondata.FreshnessFresh, LatestLTS: "v24.2.0", LatestLTSFreshness: versiondata.FreshnessFresh},
		Java: versiondata.JavaData{LatestFeature: 26, LatestFeatureFreshness: versiondata.FreshnessFresh, LatestLTS: 25, LatestLTSFreshness: versiondata.FreshnessFresh},
		Sources: []versiondata.Source{
			{ID: "node-releases", Name: "Node releases", URL: "https://node.example/index.json", FetchedAt: fetchedAt, Freshness: versiondata.FreshnessFresh},
			{ID: "node-schedule", Name: "Node schedule", URL: "https://node.example/schedule.json", FetchedAt: fetchedAt, Freshness: versiondata.FreshnessFresh},
			{ID: "java-releases", Name: "Java releases", URL: "https://java.example/releases", FetchedAt: fetchedAt, Freshness: versiondata.FreshnessFresh},
			{ID: "temurin-support", Name: "Temurin support", URL: "https://java.example/support", FetchedAt: fetchedAt, Freshness: versiondata.FreshnessFresh},
		},
	}
}
