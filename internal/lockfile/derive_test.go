package lockfile

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDeriveStatePreservesResolutionIdentityAndInputs(t *testing.T) {
	previous, err := Build(validBuildInput())
	if err != nil {
		t.Fatal(err)
	}
	previousBefore := cloneLock(previous)
	updates := []StateUpdate{{
		ItemID: "base.cmake", State: StateSatisfied,
		Observed: []Observation{{Manager: "homebrew", Version: "4.0.3"}},
		Reason:   ReasonCompatibleInstallation,
	}}
	updatesBefore := append([]StateUpdate(nil), updates...)
	updatesBefore[0].Observed = append([]Observation(nil), updates[0].Observed...)
	generatedAt := previous.GeneratedAt.Add(time.Minute)

	first, err := DeriveState(previous, generatedAt, updates)
	if err != nil {
		t.Fatalf("DeriveState() error = %v", err)
	}
	second, err := DeriveState(previous, generatedAt, updates)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("deterministic DeriveState() = %#v, %v", second, err)
	}
	if !reflect.DeepEqual(previous, previousBefore) || !reflect.DeepEqual(updates, updatesBefore) {
		t.Fatal("DeriveState mutated its inputs")
	}
	if first.ID == previous.ID || !first.GeneratedAt.Equal(generatedAt) ||
		first.Profile != previous.Profile || first.Target != previous.Target ||
		!reflect.DeepEqual(first.Sources, previous.Sources) {
		t.Fatalf("derived identity = %#v", first)
	}
	for index := range previous.Items {
		before, after := previous.Items[index], first.Items[index]
		if before.ID != after.ID || before.Module != after.Module || before.Capability != after.Capability ||
			!reflect.DeepEqual(before.Implementation, after.Implementation) {
			t.Fatalf("item identity changed: %#v / %#v", before, after)
		}
	}
	if first.Items[0].ID != "base.cmake" || first.Items[0].State != StateSatisfied ||
		first.Items[0].Observed[0] != (Observation{Manager: "homebrew", Version: "4.0.3"}) ||
		first.Summary != (Summary{Satisfied: 2, Conflict: 1, Unresolved: 1}) {
		t.Fatalf("derived state = %#v", first)
	}
}

func TestDeriveStateRejectsInvalidOrNonChangingUpdates(t *testing.T) {
	previous, err := Build(validBuildInput())
	if err != nil {
		t.Fatal(err)
	}
	valid := StateUpdate{
		ItemID: "base.cmake", State: StateSatisfied,
		Observed: []Observation{{Manager: "homebrew", Version: "4.0.3"}},
		Reason:   ReasonCompatibleInstallation,
	}
	tests := []struct {
		name    string
		at      time.Time
		updates []StateUpdate
		mutate  func(*Lock)
		message string
	}{
		{"invalid previous", previous.GeneratedAt.Add(time.Minute), []StateUpdate{valid}, func(value *Lock) { value.ID = fakeDigest('f') }, "previous Lock"},
		{"zero time", time.Time{}, []StateUpdate{valid}, func(*Lock) {}, "generated_at"},
		{"early time", previous.GeneratedAt.Add(-time.Second), []StateUpdate{valid}, func(*Lock) {}, "generated_at"},
		{"no updates", previous.GeneratedAt.Add(time.Minute), nil, func(*Lock) {}, "one update"},
		{"unknown item", previous.GeneratedAt.Add(time.Minute), []StateUpdate{{ItemID: "base.unknown", State: StateSatisfied}}, func(*Lock) {}, "does not exist"},
		{"duplicate item", previous.GeneratedAt.Add(time.Minute), []StateUpdate{valid, valid}, func(*Lock) {}, "more than once"},
		{"invalid satisfied evidence", previous.GeneratedAt.Add(time.Minute), []StateUpdate{{ItemID: "base.cmake", State: StateSatisfied, Reason: ReasonCompatibleInstallation}}, func(*Lock) {}, "lacks a matching observation"},
		{"unchanged", previous.GeneratedAt.Add(time.Minute), []StateUpdate{{
			ItemID: "base.git", State: StateSatisfied,
			Observed: []Observation{{Manager: "homebrew", Version: "2.51.0"}}, Reason: ReasonCompatibleInstallation,
		}}, func(*Lock) {}, "do not change"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := cloneLock(previous)
			test.mutate(&value)
			_, err := DeriveState(value, test.at, test.updates)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("DeriveState() error = %v, want containing %q", err, test.message)
			}
		})
	}
}
