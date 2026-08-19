package baseinstall

import (
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
)

func TestValidateInstalledTransactionClosureRequiresEveryExactFormula(t *testing.T) {
	input := validCollectionInput(t)
	facts, err := CollectTransactionFacts(input)
	if err != nil {
		t.Fatal(err)
	}
	review, err := PrepareTransactionReview(TransactionReviewInput{
		PreparedAt: input.ObservedAt, Plan: input.Plan, Lock: input.Lock,
		Baseline: facts.Baseline, Actions: facts.Actions,
	})
	if err != nil {
		t.Fatal(err)
	}
	postAt := input.ObservedAt.Add(time.Minute)
	current := collectionInventory(postAt, true)
	current.Tools = append(current.Tools,
		formulaTool("git", "2.51.0", postAt),
		formulaTool("cmake", "4.0.3", postAt),
		formulaTool("openssl@3", "3.5.2", postAt),
	)
	if err := ValidateInstalledTransactionClosure(review, current); err != nil {
		t.Fatalf("valid closure error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*inventory.Inventory)
	}{
		{"early Inventory", func(value *inventory.Inventory) { value.GeneratedAt = review.PreparedAt.Add(-time.Second) }},
		{"target drift", func(value *inventory.Inventory) { value.System.OSVersion = "14.0" }},
		{"missing root", func(value *inventory.Inventory) { value.Tools = value.Tools[:len(value.Tools)-3] }},
		{"wrong dependency", func(value *inventory.Inventory) {
			for index := range value.Tools {
				if value.Tools[index].ID == homebrewFormulaToolID("openssl@3") {
					value.Tools[index].Installations[0].Version = "3.5.1"
					value.Tools[index].Installations[0].NormalizedVersion = "3.5.1"
				}
			}
		}},
		{"multiple target installations", func(value *inventory.Inventory) {
			for index := range value.Tools {
				if value.Tools[index].ID == homebrewFormulaToolID("git") {
					duplicate := value.Tools[index].Installations[0]
					duplicate.ID += ":duplicate"
					value.Tools[index].Installations = append(value.Tools[index].Installations, duplicate)
				}
			}
		}},
		{"other architecture only", func(value *inventory.Inventory) {
			for index := range value.Tools {
				if value.Tools[index].ID == homebrewFormulaToolID("git") {
					value.Tools[index].Installations[0].Architecture = inventory.ArchitectureAMD64
				}
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := cloneInventoryForPostTest(current)
			test.mutate(&changed)
			err := ValidateInstalledTransactionClosure(review, changed)
			if err == nil || !strings.Contains(err.Error(), "installed Homebrew transaction closure") {
				t.Fatalf("unsafe closure error = %v", err)
			}
		})
	}

	missing := cloneInventoryForPostTest(current)
	missing.Tools = missing.Tools[:len(missing.Tools)-3]
	first := ValidateInstalledTransactionClosure(review, missing)
	second := ValidateInstalledTransactionClosure(review, missing)
	if first == nil || second == nil || first.Error() != second.Error() || !strings.Contains(first.Error(), `formula "cmake"`) {
		t.Fatalf("deterministic closure error = %v / %v", first, second)
	}
}

func cloneInventoryForPostTest(value inventory.Inventory) inventory.Inventory {
	result := value
	result.Tools = make([]inventory.Tool, len(value.Tools))
	for index, tool := range value.Tools {
		result.Tools[index] = tool
		result.Tools[index].Installations = append([]inventory.Installation(nil), tool.Installations...)
	}
	return result
}
