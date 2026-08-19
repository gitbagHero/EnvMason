package lockfile

import (
	"errors"
	"fmt"
	"reflect"
	"time"
)

// StateUpdate changes only the resolution evidence of one existing Lock item.
// Profile, target, sources and implementation identities remain inherited from
// the previous Lock.
type StateUpdate struct {
	ItemID   string
	State    ResolutionState
	Observed []Observation
	Reason   string
}

// DeriveState creates a new content-addressed Lock from a validated previous
// Lock without allowing callers to replace its resolution identities.
func DeriveState(previous Lock, generatedAt time.Time, updates []StateUpdate) (Lock, error) {
	if err := Validate(previous); err != nil {
		return Lock{}, errors.New("derive Lock state: previous Lock is invalid")
	}
	if generatedAt.IsZero() || generatedAt.Before(previous.GeneratedAt) {
		return Lock{}, errors.New("derive Lock state: generated_at must not predate the previous Lock")
	}
	if len(updates) == 0 || len(updates) > len(previous.Items) {
		return Lock{}, errors.New("derive Lock state: one update per changed existing item is required")
	}

	value := cloneLock(previous)
	items := make(map[string]int, len(value.Items))
	for index, item := range value.Items {
		items[item.ID] = index
	}
	seen := make(map[string]struct{}, len(updates))
	changed := false
	for _, update := range updates {
		index, exists := items[update.ItemID]
		if !exists {
			return Lock{}, fmt.Errorf("derive Lock state: item %q does not exist", update.ItemID)
		}
		if _, duplicate := seen[update.ItemID]; duplicate {
			return Lock{}, fmt.Errorf("derive Lock state: item %q is updated more than once", update.ItemID)
		}
		seen[update.ItemID] = struct{}{}
		item := &value.Items[index]
		observed := append([]Observation{}, update.Observed...)
		if item.State != update.State || item.Reason != update.Reason || !reflect.DeepEqual(item.Observed, observed) {
			changed = true
		}
		item.State = update.State
		item.Observed = observed
		item.Reason = update.Reason
	}
	if !changed {
		return Lock{}, errors.New("derive Lock state: updates do not change resolution evidence")
	}

	value.ID = ""
	value.GeneratedAt = generatedAt.UTC()
	canonicalize(&value)
	var err error
	value.ID, err = calculateID(value)
	if err != nil {
		return Lock{}, errors.New("derive Lock state: calculate ID")
	}
	if err := Validate(value); err != nil {
		return Lock{}, fmt.Errorf("derive Lock state: %w", err)
	}
	return value, nil
}
