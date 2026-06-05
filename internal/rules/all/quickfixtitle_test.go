package all

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/rule"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// genericTitle mirrors the LSP fallback label in
// internal/lsp/server.go's quickFixTitle. Every fixable rule should
// supply a more helpful, action-specific label than this.
func genericTitle(name string) string {
	return "Fix all " + name + " with mdsmith"
}

// TestEveryFixableRuleHasQuickFixTitle enforces the policy that every
// rule offering an auto-fix presents a specific lightbulb label via
// rule.QuickFixTitler, so the editor quick-fix reads like the action it
// performs (e.g. MDS012 → "Wrap in angle brackets") rather than the
// generic "Fix all <name> with mdsmith" fallback. A new fixable rule
// that forgets FixTitle fails here.
func TestEveryFixableRuleHasQuickFixTitle(t *testing.T) {
	rules := rule.All()
	require.NotEmpty(t, rules, "production rule set must be registered")

	var fixable int
	for _, r := range rules {
		if _, ok := r.(rule.FixableRule); !ok {
			continue
		}
		fixable++
		titler, ok := r.(rule.QuickFixTitler)
		if !assert.Truef(t, ok, "%s (%s) is fixable but does not implement rule.QuickFixTitler", r.ID(), r.Name()) {
			continue
		}
		title := titler.FixTitle()
		assert.NotEmptyf(t, title, "%s (%s) FixTitle is empty", r.ID(), r.Name())
		assert.NotEqualf(t, genericTitle(r.Name()), title,
			"%s (%s) FixTitle should be more specific than the generic fallback", r.ID(), r.Name())
	}
	require.Positive(t, fixable, "expected at least one fixable rule")
}
