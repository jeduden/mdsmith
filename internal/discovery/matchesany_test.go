package discovery

import (
	"testing"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMatchesAny_BraceAlternateWithSyntaxErrorFallsBackToMatch pins the
// regression a code review found: a pattern can pass
// doublestar.ValidatePattern yet still make doublestar.Match and
// doublestar.MatchUnvalidated disagree, when a brace alternative
// contains a construct (here, a malformed-looking negated class) that
// Match's internal validation aborts on before trying a later,
// perfectly matching alternative — while MatchUnvalidated tries every
// alternative regardless. matchesAny must reproduce doublestar.Match's
// answer for such a pattern, not MatchUnvalidated's, so it takes the
// safe Match path whenever a pattern contains "{".
func TestMatchesAny_BraceAlternateWithSyntaxErrorFallsBackToMatch(t *testing.T) {
	pattern := "{[!mdb[],docs/**/*.md}"
	name := "docs/a.md"

	require.True(t, doublestar.ValidatePattern(pattern),
		"precondition: pattern must pass ValidatePattern for this test to be meaningful")
	// Match itself errors here — it aborts on the first brace
	// alternative's internal syntax error instead of trying the
	// second, matching alternative. matchesAny's fallback path treats
	// a Match error the same as "no match", exactly like the
	// unguarded doublestar.Match-based implementation this PR started
	// from.
	matched, err := doublestar.Match(pattern, name)
	require.Error(t, err,
		"precondition: doublestar.Match must itself error on this pattern, "+
			"or this test is not exercising the divergence it claims to")
	require.False(t, matched)
	require.True(t, doublestar.MatchUnvalidated(pattern, name),
		"precondition: Match and MatchUnvalidated must actually diverge on this pattern")

	w := &walker{patterns: []string{pattern}}
	assert.False(t, w.matchesAny(name),
		"matchesAny must match doublestar.Match's verdict, not MatchUnvalidated's, "+
			"for a pattern using brace alternation")
}

// TestMatchesAny_BraceFreePatternUsesFastPath is a sanity check that
// the fast MatchUnvalidated path still runs (and still matches) for an
// ordinary pattern with no brace syntax — the common case this
// optimization targets.
func TestMatchesAny_BraceFreePatternUsesFastPath(t *testing.T) {
	w := &walker{patterns: []string{"docs/**/*.md"}}
	assert.True(t, w.matchesAny("docs/a.md"))
	assert.False(t, w.matchesAny("src/main.go"))
}
