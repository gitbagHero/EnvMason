package baseinstall

import (
	"errors"
	"fmt"
	"sort"

	"github.com/gitbagHero/EnvMason/internal/inventory"
)

// ValidateInstalledTransactionClosure verifies that a current explicit
// Inventory still contains exactly one reviewed version of every formula in
// the sealed transaction closure. It performs no discovery or process calls.
func ValidateInstalledTransactionClosure(review TransactionReview, current inventory.Inventory) error {
	if err := ValidateTransactionReview(review); err != nil {
		return errors.New("validate installed Homebrew transaction closure: Review is invalid")
	}
	if _, err := inventory.Marshal(current); err != nil || current.SchemaVersion != inventory.SchemaVersion {
		return errors.New("validate installed Homebrew transaction closure: Inventory is invalid")
	}
	if current.GeneratedAt.Before(review.PreparedAt) || current.System.OS != review.Target.OS ||
		current.System.OSVersion != review.Target.OSVersion ||
		current.System.Architecture != review.Target.Architecture {
		return errors.New("validate installed Homebrew transaction closure: Inventory does not match the reviewed target or time")
	}

	formulae := make(map[string]string)
	for _, action := range review.Actions {
		values := append([]FormulaPreview{action.Root}, action.Dependencies...)
		for _, formula := range values {
			if version, exists := formulae[formula.Name]; exists && version != formula.Version {
				return errors.New("validate installed Homebrew transaction closure: Review contains conflicting formula facts")
			}
			formulae[formula.Name] = formula.Version
		}
	}
	ordered := make([]string, 0, len(formulae))
	for name := range formulae {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	for _, name := range ordered {
		version := formulae[name]
		if !inventoryHasOneExactFormula(current, name, version, review.Target.Architecture) {
			return fmt.Errorf("validate installed Homebrew transaction closure: formula %q is not installed exactly as reviewed", name)
		}
	}
	return nil
}

func inventoryHasOneExactFormula(
	value inventory.Inventory,
	name, version string,
	architecture inventory.Architecture,
) bool {
	wantID := homebrewFormulaToolID(name)
	found := 0
	exact := false
	for _, tool := range value.Tools {
		if tool.ID != wantID {
			continue
		}
		for _, installation := range tool.Installations {
			if installation.Architecture != "" && installation.Architecture != inventory.ArchitectureUnknown &&
				installation.Architecture != architecture {
				continue
			}
			found++
			if installation.Manager == "homebrew" &&
				(installation.Version == version || installation.NormalizedVersion == version) {
				exact = true
			}
		}
	}
	return found == 1 && exact
}
