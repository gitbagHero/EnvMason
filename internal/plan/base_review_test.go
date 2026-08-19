package plan

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gitbagHero/EnvMason/internal/lockfile"
)

func TestBindBaseTransactionReviewDerivesAuditableImmutablePlan(t *testing.T) {
	candidate, err := BuildBaseInstall(baseInstallInput(lockfile.StateInstallRequired, lockfile.StateInstallRequired))
	if err != nil {
		t.Fatal(err)
	}
	before := clonePlanForTest(t, candidate)
	reviewID := baseFakeDigest('f')
	value, err := BindBaseTransactionReview(candidate, reviewID)
	if err != nil {
		t.Fatalf("BindBaseTransactionReview() error = %v", err)
	}
	if value.ID == candidate.ID || value.ID == "" || len(value.Actions) != len(candidate.Actions) ||
		!reflect.DeepEqual(candidate, before) {
		t.Fatalf("derived/candidate = %#v / %#v", value, candidate)
	}
	for _, action := range value.Actions {
		if !hasReviewCheck(action.Preconditions, action.ID, reviewID) ||
			len(action.Preconditions) != len(candidate.Actions[0].Preconditions)+1 {
			t.Fatalf("bound action = %#v", action)
		}
	}
	if err := Validate(value); err != nil {
		t.Fatalf("Validate(bound Plan) error = %v", err)
	}
}

func TestBindBaseTransactionReviewRejectsInvalidOrRepeatedBinding(t *testing.T) {
	candidate, err := BuildBaseInstall(baseInstallInput(lockfile.StateInstallRequired, lockfile.StateInstallRequired))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BindBaseTransactionReview(candidate, "sha256:nope"); err == nil {
		t.Fatal("invalid Review ID was accepted")
	}
	bound, err := BindBaseTransactionReview(candidate, baseFakeDigest('f'))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BindBaseTransactionReview(bound, baseFakeDigest('e')); err == nil ||
		!strings.Contains(err.Error(), "already review-bound") {
		t.Fatalf("repeated binding error = %v", err)
	}
}

func hasReviewCheck(values []Check, subject, expected string) bool {
	for _, value := range values {
		if value.Kind == BaseReviewCheckKind && value.Subject == subject && value.Expected == expected {
			return true
		}
	}
	return false
}

func clonePlanForTest(t *testing.T, value Plan) Plan {
	t.Helper()
	data, err := Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
