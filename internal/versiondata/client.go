package versiondata

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"
)

const (
	defaultTimeout = 5 * time.Second
	maxResponse    = 2 << 20
)

type Endpoints struct {
	NodeReleases   string
	NodeSchedule   string
	JavaReleases   string
	TemurinSupport string
}

func OfficialEndpoints() Endpoints {
	return Endpoints{
		NodeReleases:   "https://nodejs.org/download/release/index.json",
		NodeSchedule:   "https://raw.githubusercontent.com/nodejs/Release/main/schedule.json",
		JavaReleases:   "https://api.adoptium.net/v3/info/available_releases",
		TemurinSupport: "https://adoptium.net/support/",
	}
}

type Config struct {
	HTTPClient *http.Client
	Cache      Cache
	Now        func() time.Time
	Endpoints  Endpoints
	Timeout    time.Duration
}

type Client struct {
	httpClient *http.Client
	cache      Cache
	now        func() time.Time
	endpoints  Endpoints
	timeout    time.Duration
}

func NewClient(config Config) *Client {
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	endpoints := config.Endpoints
	if endpoints == (Endpoints{}) {
		endpoints = OfficialEndpoints()
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Client{httpClient: httpClient, cache: config.Cache, now: now, endpoints: endpoints, timeout: timeout}
}

func Collect(ctx context.Context) Result { return NewClient(Config{}).Collect(ctx) }

type sourceSpec struct {
	id, name, url string
	ttl           time.Duration
	validate      func([]byte) error
}

type sourceValue struct {
	spec   sourceSpec
	body   []byte
	source Source
	issues []Issue
}

func (client *Client) Collect(ctx context.Context) Result {
	specs := []sourceSpec{
		{id: "node-releases", name: "Node.js official release index", url: client.endpoints.NodeReleases, ttl: 6 * time.Hour, validate: validateNodeReleases},
		{id: "node-schedule", name: "Node.js Release Working Group schedule", url: client.endpoints.NodeSchedule, ttl: 24 * time.Hour, validate: validateNodeSchedule},
		{id: "java-releases", name: "Adoptium available releases API", url: client.endpoints.JavaReleases, ttl: 6 * time.Hour, validate: validateJavaReleases},
		{id: "temurin-support", name: "Eclipse Temurin support schedule", url: client.endpoints.TemurinSupport, ttl: 24 * time.Hour, validate: validateTemurinSupport},
	}
	values := make([]sourceValue, len(specs))
	var group sync.WaitGroup
	for index, spec := range specs {
		group.Add(1)
		go func(index int, spec sourceSpec) {
			defer group.Done()
			values[index] = client.read(ctx, spec)
		}(index, spec)
	}
	group.Wait()

	result := Result{Sources: make([]Source, 0, len(values)), Issues: []Issue{}}
	for _, value := range values {
		result.Sources = append(result.Sources, value.source)
		result.Issues = append(result.Issues, value.issues...)
		switch value.spec.id {
		case "node-releases":
			parseNodeReleases(value.body, value.source.Freshness, &result.Node)
		case "node-schedule":
			parseNodeSchedule(value.body, value.source.Freshness, client.now().UTC(), &result.Node)
		case "java-releases":
			parseJavaReleases(value.body, value.source.Freshness, &result.Java)
		case "temurin-support":
			parseTemurinSupport(value.body, value.source.Freshness, client.now().UTC(), &result.Java)
		}
	}
	sort.Slice(result.Issues, func(i, j int) bool {
		if result.Issues[i].SourceID != result.Issues[j].SourceID {
			return result.Issues[i].SourceID < result.Issues[j].SourceID
		}
		return result.Issues[i].Code < result.Issues[j].Code
	})
	return result
}

func (client *Client) read(ctx context.Context, spec sourceSpec) sourceValue {
	now := client.now().UTC()
	value := sourceValue{spec: spec, source: Source{ID: spec.id, Name: spec.name, URL: safeURL(spec.url), Freshness: FreshnessUnavailable}}
	var stale *CacheEntry
	if client.cache != nil {
		entry, err := client.cache.Read(ctx, spec.id)
		switch {
		case err == nil && spec.validate(entry.Body) == nil:
			entry.FetchedAt = entry.FetchedAt.UTC()
			if !entry.FetchedAt.After(now) && now.Before(entry.FetchedAt.Add(spec.ttl)) {
				value.body = entry.Body
				value.source.FetchedAt = entry.FetchedAt
				value.source.ExpiresAt = entry.FetchedAt.Add(spec.ttl)
				value.source.Freshness = FreshnessFresh
				return value
			}
			stale = &entry
		case err == nil:
			value.issues = append(value.issues, Issue{Code: "VERSION_CACHE_CORRUPT", SourceID: spec.id})
		case errors.Is(err, ErrCacheMiss):
		default:
			value.issues = append(value.issues, Issue{Code: "VERSION_CACHE_UNAVAILABLE", SourceID: spec.id})
		}
	}

	body, fetchedAt, err := client.fetch(ctx, spec)
	if err == nil && spec.validate(body) == nil {
		value.body = body
		value.source.FetchedAt = fetchedAt
		value.source.ExpiresAt = fetchedAt.Add(spec.ttl)
		value.source.Freshness = FreshnessFresh
		return value
	}
	if stale != nil {
		value.body = stale.Body
		value.source.FetchedAt = stale.FetchedAt
		value.source.ExpiresAt = stale.FetchedAt.Add(spec.ttl)
		value.source.Freshness = FreshnessStale
		value.issues = append(value.issues, Issue{Code: "VERSION_SOURCE_STALE", SourceID: spec.id})
		return value
	}
	value.issues = append(value.issues, Issue{Code: "VERSION_SOURCE_UNAVAILABLE", SourceID: spec.id})
	return value
}

func safeURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "invalid-source-url"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func (client *Client) fetch(ctx context.Context, spec sourceSpec) ([]byte, time.Time, error) {
	requestContext, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, spec.url, nil)
	if err != nil {
		return nil, time.Time{}, errors.New("invalid source URL")
	}
	request.Header.Set("Accept", "application/json, text/html;q=0.9")
	request.Header.Set("User-Agent", "EnvMason/version-data")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, time.Time{}, errors.New("source request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, time.Time{}, fmt.Errorf("unexpected source status")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil || len(body) > maxResponse {
		return nil, time.Time{}, errors.New("invalid source response")
	}
	return body, client.now().UTC(), nil
}
