package integration

import (
	"fmt"
	"testing"

	"github.com/jeduden/mdsmith/internal/foreignregion"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
)

// TestRuleIDsAreUnique locks the rule-ID surface documented in
// docs/development/architecture/cross-system.md: an MDS### ID is the
// addressable unit of the `.mdsmith.yml` rules: map, CLI output, and
// inline-suppression comments, so no two diagnostics may share one.
// MDS073 (slide-structure) and the foreign-region scanner's
// engine-emitted MDS073 diagnostic collided until the scanner moved to
// MDS074 — this test catches a repeat of that collision, including
// against any future non-rule.Rule diagnostic ID added here.
func TestRuleIDsAreUnique(t *testing.T) {
	seen := make(map[string]string)
	assertUnique := func(id, source string) {
		if owner, ok := seen[id]; ok {
			assert.Fail(t, fmt.Sprintf(
				"rule ID %q is used by both %q and %q", id, owner, source))
			return
		}
		seen[id] = source
	}

	for _, r := range rule.All() {
		assertUnique(r.ID(), r.Name())
	}
	assertUnique(foreignregion.RuleID, foreignregion.RuleName)
}
