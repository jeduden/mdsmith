package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsIgnored_MatchesGlobPattern(t *testing.T) {
	patterns := []string{"vendor/**"}
	assert.True(t, IsIgnored(patterns, "vendor/lib.md"), "expected vendor/lib.md to be ignored")
}

func TestIsIgnored_MatchesBasename(t *testing.T) {
	patterns := []string{"CHANGELOG.md"}
	assert.True(t, IsIgnored(patterns, "/some/path/CHANGELOG.md"), "expected CHANGELOG.md to be ignored by basename")
}

func TestIsIgnored_NoMatch(t *testing.T) {
	patterns := []string{"vendor/**"}
	assert.False(t, IsIgnored(patterns, "src/main.md"), "expected src/main.md not to be ignored")
}

func TestIsIgnored_EmptyPatterns(t *testing.T) {
	assert.False(t, IsIgnored(nil, "test.md"), "expected no match with empty patterns")
}

func TestIsIgnored_InvalidPatternSkipped(t *testing.T) {
	patterns := []string{"[invalid"}
	assert.False(t, IsIgnored(patterns, "test.md"), "expected invalid pattern to be skipped")
}

func TestIsIgnored_CleanedPath(t *testing.T) {
	patterns := []string{"vendor/**"}
	assert.True(t, IsIgnored(patterns, "vendor/./lib.md"), "expected cleaned path to match")
}

func TestIsIgnored_NegationReIncludes(t *testing.T) {
	patterns := []string{"plan/*.md", "!plan/proto.md"}
	assert.True(t, IsIgnored(patterns, "plan/96_kinds.md"),
		"plan/96_kinds.md should match the inclusion pattern")
	assert.False(t, IsIgnored(patterns, "plan/proto.md"),
		"plan/proto.md should be re-included by the negation")
}

func TestIsIgnored_NegationOrderIndependent(t *testing.T) {
	beforeInclude := []string{"!plan/proto.md", "plan/*.md"}
	assert.False(t, IsIgnored(beforeInclude, "plan/proto.md"),
		"negation must win even when listed before its inclusion")
}

func TestIsIgnored_OnlyExclusionsMatchNothing(t *testing.T) {
	patterns := []string{"!plan/proto.md"}
	assert.False(t, IsIgnored(patterns, "plan/proto.md"),
		"a list of only exclusions should not match anything")
	assert.False(t, IsIgnored(patterns, "plan/96_kinds.md"),
		"a list of only exclusions should not match anything")
}
