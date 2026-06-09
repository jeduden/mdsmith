package gitignore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMatchDoublestar_LeadingDoublestar_ZeroAllocs confirms the
// leading-** path allocates nothing after the strings.Split/Join rewrite.
func TestMatchDoublestar_LeadingDoublestar_ZeroAllocs(t *testing.T) {
	cases := []struct{ pattern, path string }{
		{"**/*.md", "readme.md"},
		{"**/*.md", "sub/readme.md"},
		{"**/*.md", "a/b/c.md"},
		{"**/foo", "a/b/c/foo"},
	}
	for _, c := range cases {
		allocs := testing.AllocsPerRun(100, func() {
			matchDoublestar(c.pattern, c.path)
		})
		assert.Equal(t, 0.0, allocs,
			"matchDoublestar(%q, %q) allocs: want 0, got %v", c.pattern, c.path, allocs)
	}
}

// TestMatchDoublestar_MiddleDoublestar_ZeroAllocs confirms the
// middle-** path allocates nothing after the rewrite.
func TestMatchDoublestar_MiddleDoublestar_ZeroAllocs(t *testing.T) {
	cases := []struct{ pattern, path string }{
		{"a/**/b.md", "a/b.md"},
		{"docs/**/readme.md", "docs/readme.md"},
		{"docs/**/readme.md", "docs/sub/readme.md"},
	}
	for _, c := range cases {
		allocs := testing.AllocsPerRun(100, func() {
			matchDoublestar(c.pattern, c.path)
		})
		assert.Equal(t, 0.0, allocs,
			"matchDoublestar(%q, %q) allocs: want 0, got %v", c.pattern, c.path, allocs)
	}
}
