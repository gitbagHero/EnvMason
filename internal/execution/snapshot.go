package execution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
)

func NewSnapshot(facts map[string]string) (Snapshot, error) {
	if facts == nil {
		return Snapshot{}, errors.New("snapshot facts are required")
	}
	clone := make(map[string]string, len(facts))
	for key, value := range facts {
		if key == "" {
			return Snapshot{}, errors.New("snapshot fact key is required")
		}
		clone[key] = value
	}
	data, err := json.Marshal(clone)
	if err != nil {
		return Snapshot{}, err
	}
	digest := sha256.Sum256(data)
	return Snapshot{Digest: "sha256:" + hex.EncodeToString(digest[:]), Facts: clone}, nil
}

func DiffSnapshots(before, after Snapshot) []Change {
	keys := make(map[string]bool, len(before.Facts)+len(after.Facts))
	for key := range before.Facts {
		keys[key] = true
	}
	for key := range after.Facts {
		keys[key] = true
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	changes := make([]Change, 0)
	for _, key := range ordered {
		beforeValue, beforeOK := before.Facts[key]
		afterValue, afterOK := after.Facts[key]
		switch {
		case !beforeOK:
			changes = append(changes, Change{Key: key, Kind: "added", After: afterValue})
		case !afterOK:
			changes = append(changes, Change{Key: key, Kind: "removed", Before: beforeValue})
		case beforeValue != afterValue:
			changes = append(changes, Change{Key: key, Kind: "changed", Before: beforeValue, After: afterValue})
		}
	}
	return changes
}
