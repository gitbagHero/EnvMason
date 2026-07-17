package versiondata

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	versioncore "github.com/gitbagHero/EnvMason/internal/version"
)

type nodeReleaseDocument struct {
	Version string          `json:"version"`
	LTS     json.RawMessage `json:"lts"`
}

func validateNodeReleases(body []byte) error {
	var releases []nodeReleaseDocument
	if err := decodeJSON(body, &releases); err != nil || len(releases) == 0 {
		return errors.New("invalid Node.js release index")
	}
	for _, release := range releases {
		if versioncore.ParseSemVer(release.Version).Comparable {
			return nil
		}
	}
	return errors.New("Node.js release index has no comparable version")
}

func parseNodeReleases(body []byte, freshness Freshness, data *NodeData) {
	if freshness == FreshnessUnavailable {
		return
	}
	var releases []nodeReleaseDocument
	if decodeJSON(body, &releases) != nil {
		return
	}
	var stable, lts versioncore.Value
	for _, release := range releases {
		candidate := versioncore.ParseSemVer(release.Version)
		if !candidate.Comparable {
			continue
		}
		if !stable.Comparable || versioncore.Compare(candidate, stable) == versioncore.RelationGreater {
			stable = candidate
			data.LatestStable = release.Version
		}
		if nodeLTS(release.LTS) && (!lts.Comparable || versioncore.Compare(candidate, lts) == versioncore.RelationGreater) {
			lts = candidate
			data.LatestLTS = release.Version
		}
	}
	if data.LatestStable != "" {
		data.LatestStableFreshness = freshness
	}
	if data.LatestLTS != "" {
		data.LatestLTSFreshness = freshness
	}
}

func nodeLTS(value json.RawMessage) bool {
	trimmed := bytes.TrimSpace(value)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("false")) && !bytes.Equal(trimmed, []byte("null")) && !bytes.Equal(trimmed, []byte(`""`))
}

type nodeScheduleEntry struct {
	Start    string `json:"start"`
	LTS      string `json:"lts"`
	End      string `json:"end"`
	Codename string `json:"codename"`
}

func validateNodeSchedule(body []byte) error {
	var schedule map[string]nodeScheduleEntry
	if err := decodeJSON(body, &schedule); err != nil || len(schedule) == 0 {
		return errors.New("invalid Node.js lifecycle schedule")
	}
	for major, entry := range schedule {
		if strings.HasPrefix(major, "v") && entry.Start != "" && entry.End != "" {
			return nil
		}
	}
	return errors.New("Node.js lifecycle schedule has no usable entry")
}

func parseNodeSchedule(body []byte, freshness Freshness, now time.Time, data *NodeData) {
	if freshness == FreshnessUnavailable {
		return
	}
	var schedule map[string]nodeScheduleEntry
	if decodeJSON(body, &schedule) != nil {
		return
	}
	for key, entry := range schedule {
		major, err := strconv.Atoi(strings.TrimPrefix(key, "v"))
		start, startErr := time.Parse(time.DateOnly, entry.Start)
		end, endErr := time.Parse(time.DateOnly, entry.End)
		if err != nil || startErr != nil || endErr != nil {
			continue
		}
		state := LifecycleUnknown
		switch {
		case !now.Before(end):
			state = LifecycleEOL
		case now.Before(start):
			state = LifecycleUnknown
		case entry.LTS != "":
			ltsAt, ltsErr := time.Parse(time.DateOnly, entry.LTS)
			if ltsErr == nil && !now.Before(ltsAt) {
				state = LifecycleLTS
			} else {
				state = LifecycleStable
			}
		default:
			state = LifecycleStable
		}
		data.Lifecycle = append(data.Lifecycle, NodeLifecycle{Major: major, Codename: entry.Codename, State: state, End: end, Freshness: freshness})
	}
	sort.Slice(data.Lifecycle, func(i, j int) bool { return data.Lifecycle[i].Major < data.Lifecycle[j].Major })
}

type javaReleaseDocument struct {
	AvailableLTS      []int `json:"available_lts_releases"`
	Available         []int `json:"available_releases"`
	MostRecentFeature int   `json:"most_recent_feature_release"`
	MostRecentLTS     int   `json:"most_recent_lts"`
}

func validateJavaReleases(body []byte) error {
	var document javaReleaseDocument
	if err := decodeJSON(body, &document); err != nil || document.MostRecentFeature <= 0 || document.MostRecentLTS <= 0 || len(document.Available) == 0 {
		return errors.New("invalid Adoptium release data")
	}
	return nil
}

func parseJavaReleases(body []byte, freshness Freshness, data *JavaData) {
	if freshness == FreshnessUnavailable {
		return
	}
	var document javaReleaseDocument
	if decodeJSON(body, &document) != nil {
		return
	}
	data.LatestFeature = document.MostRecentFeature
	data.LatestFeatureFreshness = freshness
	data.LatestLTS = document.MostRecentLTS
	data.LatestLTSFreshness = freshness
}

var temurinRowPattern = regexp.MustCompile(`(?s)<tr><td[^>]*>.*?Java ([0-9]+)( \(LTS\))?.*?</td>(?:<td[^>]*>.*?</td>){3}<td[^>]*>.*?<p[^>]*>(At least )?([A-Z][a-z]{2}) ([0-9]{4})</p>.*?</td></tr>`)

func validateTemurinSupport(body []byte) error {
	if len(temurinRowPattern.FindAllSubmatch(body, -1)) == 0 {
		return errors.New("invalid Temurin support schedule")
	}
	return nil
}

func parseTemurinSupport(body []byte, freshness Freshness, now time.Time, data *JavaData) {
	if freshness == FreshnessUnavailable {
		return
	}
	for _, match := range temurinRowPattern.FindAllSubmatch(body, -1) {
		major, majorErr := strconv.Atoi(string(match[1]))
		year, yearErr := strconv.Atoi(string(match[5]))
		month, monthErr := time.Parse("Jan", string(match[4]))
		if majorErr != nil || yearErr != nil || monthErr != nil {
			continue
		}
		atLeast := len(match[3]) > 0
		endExclusive := time.Date(year, month.Month()+1, 1, 0, 0, 0, 0, time.UTC)
		state := LifecycleStable
		lts := len(match[2]) > 0
		if lts {
			state = LifecycleLTS
		}
		if !now.Before(endExclusive) {
			if atLeast {
				state = LifecycleUnknown
			} else {
				state = LifecycleEOL
			}
		}
		data.TemurinLifecycle = append(data.TemurinLifecycle, JavaLifecycle{
			Major: major, LTS: lts, SupportUntil: strings.TrimSpace(string(match[3]) + string(match[4]) + " " + string(match[5])), State: state, Freshness: freshness,
		})
	}
	sort.Slice(data.TemurinLifecycle, func(i, j int) bool { return data.TemurinLifecycle[i].Major < data.TemurinLifecycle[j].Major })
}

func decodeJSON(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("trailing JSON value")
	}
	return nil
}
