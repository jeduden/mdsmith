package globpath_test

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/globpath"
	"github.com/stretchr/testify/assert"
)

func TestMatch_Basic(t *testing.T) {
	assert.True(t, globpath.Match("vendor/**", "vendor/lib.md"))
	assert.True(t, globpath.Match("vendor/**", "vendor/sub/lib.md"))
	assert.False(t, globpath.Match("vendor/**", "src/main.md"))
}

func TestMatch_Basename(t *testing.T) {
	assert.True(t, globpath.Match("CHANGELOG.md", "/some/path/CHANGELOG.md"),
		"basename match: pattern without separator should match file in any directory")
	assert.True(t, globpath.Match("CHANGELOG.md", "CHANGELOG.md"))
	assert.False(t, globpath.Match("CHANGELOG.md", "other.md"))
}

func TestMatch_CleanedPath(t *testing.T) {
	assert.True(t, globpath.Match("vendor/**", "vendor/./lib.md"),
		"cleaned path: vendor/./lib.md should match vendor/**")
	// "vendor/*.md" cannot match the raw "vendor/./lib.md" (the "."
	// segment defeats the single-star), so only the cleaned-path
	// candidate produces this match.
	assert.True(t, globpath.Match("vendor/*.md", "vendor/./lib.md"),
		"match must fall through to the cleaned-path candidate")
}

func TestMatch_DoubleStarRecursion(t *testing.T) {
	assert.True(t, globpath.Match("docs/**/*.md", "docs/a/b/c.md"))
	assert.True(t, globpath.Match("docs/**/*.md", "docs/foo.md"))
	assert.False(t, globpath.Match("docs/**/*.md", "other/foo.md"))
}

func TestMatch_BraceExpansion(t *testing.T) {
	assert.True(t, globpath.Match("*.{md,markdown}", "README.md"))
	assert.True(t, globpath.Match("*.{md,markdown}", "README.markdown"))
	assert.False(t, globpath.Match("*.{md,markdown}", "README.txt"))
}

func TestMatch_InvalidPattern(t *testing.T) {
	assert.False(t, globpath.Match("[invalid", "test.md"),
		"invalid pattern should return false")
}

func TestMatchAny_IncludeOnly(t *testing.T) {
	patterns := []string{"vendor/**"}
	assert.True(t, globpath.MatchAny(patterns, "vendor/lib.md"))
	assert.False(t, globpath.MatchAny(patterns, "src/main.md"))
}

func TestMatchAny_Negation(t *testing.T) {
	patterns := []string{"plan/*.md", "!plan/proto.md"}
	assert.True(t, globpath.MatchAny(patterns, "plan/96_kinds.md"))
	assert.False(t, globpath.MatchAny(patterns, "plan/proto.md"),
		"negation pattern should exclude proto.md")
}

func TestMatchAny_NegationOrderIndependent(t *testing.T) {
	before := []string{"!plan/proto.md", "plan/*.md"}
	assert.False(t, globpath.MatchAny(before, "plan/proto.md"),
		"negation must win even when listed before its inclusion")
}

func TestMatchAny_OnlyExclusionsMatchNothing(t *testing.T) {
	patterns := []string{"!plan/proto.md"}
	assert.False(t, globpath.MatchAny(patterns, "plan/proto.md"))
	assert.False(t, globpath.MatchAny(patterns, "plan/other.md"))
}

func TestMatchAny_Empty(t *testing.T) {
	assert.False(t, globpath.MatchAny(nil, "test.md"))
	assert.False(t, globpath.MatchAny([]string{}, "test.md"))
}

func TestMatchAny_DoubleStarAndBraces(t *testing.T) {
	patterns := []string{"docs/**/*.md", "!docs/secret/**"}
	assert.True(t, globpath.MatchAny(patterns, "docs/a/b/c.md"))
	assert.False(t, globpath.MatchAny(patterns, "docs/secret/foo.md"))
}

func TestSplitIncludeExclude_Mixed(t *testing.T) {
	patterns := []string{"docs/**", "!docs/secret/**", "plan/*.md"}
	inc, exc := globpath.SplitIncludeExclude(patterns)
	assert.Equal(t, []string{"docs/**", "plan/*.md"}, inc)
	assert.Equal(t, []string{"docs/secret/**"}, exc)
}

func TestSplitIncludeExclude_IncludeOnly(t *testing.T) {
	inc, exc := globpath.SplitIncludeExclude([]string{"a", "b"})
	assert.Equal(t, []string{"a", "b"}, inc)
	assert.Nil(t, exc)
}

func TestSplitIncludeExclude_ExcludeOnly(t *testing.T) {
	inc, exc := globpath.SplitIncludeExclude([]string{"!a", "!b"})
	assert.Nil(t, inc)
	assert.Equal(t, []string{"a", "b"}, exc)
}

func TestSplitIncludeExclude_Empty(t *testing.T) {
	inc, exc := globpath.SplitIncludeExclude(nil)
	assert.Nil(t, inc)
	assert.Nil(t, exc)
}

func TestResolveAgainstRoot_StaysInside(t *testing.T) {
	resolved, escapes := globpath.ResolveAgainstRoot("a/b/c", "../../d/*.md")
	assert.False(t, escapes)
	assert.Equal(t, "a/d/*.md", resolved)
}

func TestResolveAgainstRoot_EmptyBase(t *testing.T) {
	resolved, escapes := globpath.ResolveAgainstRoot("", "docs/*.md")
	assert.False(t, escapes)
	assert.Equal(t, "docs/*.md", resolved)
}

func TestResolveAgainstRoot_DotBase(t *testing.T) {
	resolved, escapes := globpath.ResolveAgainstRoot(".", "docs/*.md")
	assert.False(t, escapes)
	assert.Equal(t, "docs/*.md", resolved)
}

func TestResolveAgainstRoot_ResolvesToRoot(t *testing.T) {
	resolved, escapes := globpath.ResolveAgainstRoot("a", "..")
	assert.False(t, escapes)
	assert.Equal(t, "", resolved)
}

func TestResolveAgainstRoot_Escapes(t *testing.T) {
	resolved, escapes := globpath.ResolveAgainstRoot("a", "../../x.md")
	assert.True(t, escapes)
	assert.Equal(t, "../x.md", resolved)
}

func TestResolveAgainstRoot_EscapesToDotDot(t *testing.T) {
	resolved, escapes := globpath.ResolveAgainstRoot("", "..")
	assert.True(t, escapes)
	assert.Equal(t, "..", resolved)
}

func TestContainsDotDotSegment(t *testing.T) {
	cases := map[string]bool{
		"../foo":     true,
		"foo/../bar": true,
		"foo/bar/..": true,
		"":           false,
		"foo..bar":   false,
		"...":        false,
		"..":         true,
		"foo/bar":    false,
	}
	for input, want := range cases {
		assert.Equal(t, want, globpath.ContainsDotDotSegment(input), input)
	}
}

func TestContainsDotDotSegment_ZeroAllocs(t *testing.T) {
	paths := []string{
		"..",
		"../foo",
		"foo/..",
		"foo/../bar",
		"foo/bar/baz",
	}
	for _, p := range paths {
		allocs := testing.AllocsPerRun(100, func() {
			globpath.ContainsDotDotSegment(p)
		})
		assert.Equal(t, 0.0, allocs,
			"ContainsDotDotSegment(%q) allocs: want 0, got %v", p, allocs)
	}
}
