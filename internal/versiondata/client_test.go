package versiondata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var fixtureNow = time.Date(2026, 7, 17, 5, 0, 0, 0, time.UTC)

var fixtureBodies = map[string][]byte{
	"node-releases":   []byte(`[{"version":"v26.1.0","lts":false,"date":"2026-06-01"},{"version":"v24.2.0","lts":"Krypton"}]`),
	"node-schedule":   []byte(`{"v22":{"start":"2024-04-01","end":"2025-04-01"},"v24":{"start":"2025-05-01","lts":"2025-10-01","end":"2028-04-01","codename":"Krypton"},"v26":{"start":"2026-05-01","end":"2027-04-01"}}`),
	"java-releases":   []byte(`{"available_lts_releases":[8,11,17,21,25],"available_releases":[8,11,17,21,24,25,26],"most_recent_feature_release":26,"most_recent_lts":25,"tip_version":28}`),
	"temurin-support": []byte(`<table><tbody><tr><td><p>Java 26</p></td><td>Mar 2026</td><td>Apr 2026</td><td>Jul 2026</td><td><p>Sep 2026</p></td></tr><tr><td><p>Java 25 (LTS)</p></td><td>Sep 2025</td><td>Apr 2026</td><td>Jul 2026</td><td><p>At least Sep 2031</p></td></tr><tr><td><p>Java 24</p></td><td>Mar 2025</td><td>Jul 2025</td><td>EOSL</td><td><p>Sep 2025</p></td></tr></tbody></table>`),
}

func TestCollectsOfficialFormatsAndLifecycle(t *testing.T) {
	server := fixtureServer(t, 0)
	defer server.Close()
	result := testClient(server.Client(), endpoints(server.URL), nil, time.Second).Collect(context.Background())

	if result.Node.LatestStable != "v26.1.0" || result.Node.LatestLTS != "v24.2.0" {
		t.Fatalf("Node release data = %#v", result.Node)
	}
	if got := nodeState(result.Node.Lifecycle, 22); got != LifecycleEOL {
		t.Fatalf("Node 22 lifecycle = %q", got)
	}
	if got := nodeState(result.Node.Lifecycle, 24); got != LifecycleLTS {
		t.Fatalf("Node 24 lifecycle = %q", got)
	}
	if result.Java.LatestFeature != 26 || result.Java.LatestLTS != 25 {
		t.Fatalf("Java release data = %#v", result.Java)
	}
	if got := result.Java.LifecycleForVendor(24, "temurin"); got != LifecycleEOL {
		t.Fatalf("Temurin 24 lifecycle = %q", got)
	}
	if got := result.Java.LifecycleForVendor(24, "oracle"); got != LifecycleUnknown {
		t.Fatalf("Oracle 24 lifecycle = %q, want Unknown", got)
	}
	if len(result.Issues) != 0 || len(result.Sources) != 4 {
		t.Fatalf("issues/sources = %#v / %#v", result.Issues, result.Sources)
	}
	for _, source := range result.Sources {
		if source.Freshness != FreshnessFresh || source.FetchedAt != fixtureNow || source.ExpiresAt.IsZero() {
			t.Errorf("source = %#v", source)
		}
	}
}

func TestFreshCacheAvoidsNetwork(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	cache := memoryCache{entries: cachedBodies(fixtureNow.Add(-time.Hour))}
	result := testClient(server.Client(), endpoints(server.URL), cache, time.Second).Collect(context.Background())
	if requests.Load() != 0 || len(result.Issues) != 0 {
		t.Fatalf("requests/issues = %d / %#v", requests.Load(), result.Issues)
	}
}

func TestOfflineUsesExpiredCacheAsStale(t *testing.T) {
	cache := memoryCache{entries: cachedBodies(fixtureNow.Add(-48 * time.Hour))}
	client := testClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("offline secret must not escape")
	})}, endpoints("http://offline.invalid"), cache, time.Second)
	result := client.Collect(context.Background())
	if result.Node.LatestStableFreshness != FreshnessStale || result.Java.LatestFeatureFreshness != FreshnessStale {
		t.Fatalf("stale data = %#v / %#v", result.Node, result.Java)
	}
	if countIssue(result.Issues, "VERSION_SOURCE_STALE") != 4 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	for _, source := range result.Sources {
		if source.Freshness != FreshnessStale {
			t.Errorf("source = %#v", source)
		}
	}
}

func TestCorruptCacheAndOfflineAreExplicit(t *testing.T) {
	entries := map[string]CacheEntry{}
	for id := range fixtureBodies {
		entries[id] = CacheEntry{Body: []byte("not valid data"), FetchedAt: fixtureNow}
	}
	client := testClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("offline")
	})}, endpoints("http://offline.invalid"), memoryCache{entries: entries}, time.Second)
	result := client.Collect(context.Background())
	if countIssue(result.Issues, "VERSION_CACHE_CORRUPT") != 4 || countIssue(result.Issues, "VERSION_SOURCE_UNAVAILABLE") != 4 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	for _, source := range result.Sources {
		if source.Freshness != FreshnessUnavailable {
			t.Errorf("source = %#v", source)
		}
	}
}

func TestSourceMetadataRemovesCredentialsAndQuery(t *testing.T) {
	custom := endpoints("http://user:secret@offline.invalid?token=secret")
	result := testClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("offline")
	})}, custom, nil, time.Second).Collect(context.Background())
	for _, source := range result.Sources {
		if strings.Contains(source.URL, "secret") || strings.Contains(source.URL, "user:") || strings.Contains(source.URL, "token=") {
			t.Errorf("sensitive source URL leaked: %q", source.URL)
		}
	}
}

func TestSourceTimeoutDoesNotBlockCollection(t *testing.T) {
	server := fixtureServer(t, 100*time.Millisecond)
	defer server.Close()
	started := time.Now()
	result := testClient(server.Client(), endpoints(server.URL), nil, 10*time.Millisecond).Collect(context.Background())
	if elapsed := time.Since(started); elapsed > 150*time.Millisecond {
		t.Fatalf("collection took %s; sources may not be concurrent", elapsed)
	}
	if countIssue(result.Issues, "VERSION_SOURCE_UNAVAILABLE") != 4 {
		t.Fatalf("issues = %#v", result.Issues)
	}
}

func TestOversizedResponsesAreRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(strings.Repeat("x", maxResponse+1)))
	}))
	defer server.Close()
	result := testClient(server.Client(), endpoints(server.URL), nil, time.Second).Collect(context.Background())
	if countIssue(result.Issues, "VERSION_SOURCE_UNAVAILABLE") != 4 {
		t.Fatalf("oversized response issues = %#v", result.Issues)
	}
}

func fixtureServer(t *testing.T, delay time.Duration) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if delay > 0 {
			time.Sleep(delay)
		}
		id := strings.TrimPrefix(request.URL.Path, "/")
		body, exists := fixtureBodies[id]
		if !exists {
			http.NotFound(response, request)
			return
		}
		_, _ = response.Write(body)
	}))
}

func endpoints(base string) Endpoints {
	return Endpoints{NodeReleases: base + "/node-releases", NodeSchedule: base + "/node-schedule", JavaReleases: base + "/java-releases", TemurinSupport: base + "/temurin-support"}
}

func testClient(httpClient *http.Client, endpoints Endpoints, cache Cache, timeout time.Duration) *Client {
	return NewClient(Config{HTTPClient: httpClient, Endpoints: endpoints, Cache: cache, Timeout: timeout, Now: func() time.Time { return fixtureNow }})
}

type memoryCache struct{ entries map[string]CacheEntry }

func (cache memoryCache) Read(_ context.Context, key string) (CacheEntry, error) {
	entry, exists := cache.entries[key]
	if !exists {
		return CacheEntry{}, ErrCacheMiss
	}
	return entry, nil
}

func cachedBodies(fetchedAt time.Time) map[string]CacheEntry {
	result := map[string]CacheEntry{}
	for id, body := range fixtureBodies {
		result[id] = CacheEntry{Body: body, FetchedAt: fetchedAt}
	}
	return result
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func countIssue(issues []Issue, code string) int {
	count := 0
	for _, issue := range issues {
		if issue.Code == code {
			count++
		}
	}
	return count
}

func nodeState(entries []NodeLifecycle, major int) Lifecycle {
	for _, entry := range entries {
		if entry.Major == major {
			return entry.State
		}
	}
	return LifecycleUnknown
}
