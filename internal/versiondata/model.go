// Package versiondata retrieves and interprets trusted remote version data.
package versiondata

import (
	"context"
	"errors"
	"time"
)

type Freshness string

const (
	FreshnessFresh       Freshness = "fresh"
	FreshnessStale       Freshness = "stale"
	FreshnessUnavailable Freshness = "unavailable"
)

type Lifecycle string

const (
	LifecycleStable  Lifecycle = "stable"
	LifecycleLTS     Lifecycle = "lts"
	LifecycleEOL     Lifecycle = "eol"
	LifecycleUnknown Lifecycle = "unknown"
)

type Source struct {
	ID        string
	Name      string
	URL       string
	FetchedAt time.Time
	ExpiresAt time.Time
	Freshness Freshness
}

type Issue struct {
	Code     string
	SourceID string
}

type NodeLifecycle struct {
	Major     int
	Codename  string
	State     Lifecycle
	End       time.Time
	Freshness Freshness
}

type NodeData struct {
	LatestStable          string
	LatestStableFreshness Freshness
	LatestLTS             string
	LatestLTSFreshness    Freshness
	Lifecycle             []NodeLifecycle
}

type JavaLifecycle struct {
	Major        int
	LTS          bool
	SupportUntil string
	State        Lifecycle
	Freshness    Freshness
}

type JavaData struct {
	LatestFeature          int
	LatestFeatureFreshness Freshness
	LatestLTS              int
	LatestLTSFreshness     Freshness
	TemurinLifecycle       []JavaLifecycle
}

// LifecycleForVendor is intentionally conservative: Adoptium's support
// schedule applies only to Eclipse Temurin, never to an arbitrary JDK vendor.
func (data JavaData) LifecycleForVendor(major int, vendor string) Lifecycle {
	if vendor != "temurin" {
		return LifecycleUnknown
	}
	for _, release := range data.TemurinLifecycle {
		if release.Major == major && release.Freshness == FreshnessFresh {
			return release.State
		}
	}
	return LifecycleUnknown
}

type Result struct {
	Node    NodeData
	Java    JavaData
	Sources []Source
	Issues  []Issue
}

type CacheEntry struct {
	Body      []byte
	FetchedAt time.Time
}

// Cache is read-only in I10. Persisting remote data is deliberately deferred
// until a later increment can perform the write through an approved Plan.
type Cache interface {
	Read(context.Context, string) (CacheEntry, error)
}

var ErrCacheMiss = errors.New("version data cache miss")
