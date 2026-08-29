package integration

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/foreignregion"
	"github.com/jeduden/mdsmith/internal/rule"

	_ "github.com/jeduden/mdsmith/internal/rules/all"
)

// TestRuleIDsAreUnique guards the public MDS### contract: every
// registered rule.Rule ID is unique, and no out-of-band diagnostic
// (one emitted outside the rule.Rule registry, like foreignregion's
// malformed-marker-pair check) reuses an ID already claimed by a
// registered rule. See docs/development/architecture/cross-system.md
// on rule IDs as part of the public .mdsmith.yml/CLI/docs contract,
// and plan/2608091910_arch-fix-mds073-collision.md for the collision
// this test was added to prevent from recurring.
func TestRuleIDsAreUnique(t *testing.T) {
	seen := make(map[string]string) // id -> owner description

	claim := func(id, owner string) {
		if prior, ok := seen[id]; ok {
			t.Errorf("rule ID %q claimed by both %q and %q", id, prior, owner)
			return
		}
		seen[id] = owner
	}

	for _, r := range rule.All() {
		claim(r.ID(), "rule."+r.Name())
	}
	claim(foreignregion.RuleID, "foreignregion."+foreignregion.RuleName)
}
