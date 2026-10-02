package linkgraph

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFileNameKey(t *testing.T) {
	assert.Equal(t, "guide.mdx", FileNameKey("Guide.MDX"))
	assert.Equal(t, "logo.png", FileNameKey("logo.png"))
}

func TestFileStemKey(t *testing.T) {
	for _, tc := range []struct {
		base, stem string
		ok         bool
	}{
		{"Guide.md", "guide", true},
		{"notes.markdown", "notes", true},
		{" guide.md", " guide", true},
		{"v1.3.md", "v1.3", true},
		{".md", "", true},
		{"image.png", "", false},
		{"COPYING", "", false},
	} {
		stem, ok := FileStemKey(tc.base)
		assert.Equal(t, tc.ok, ok, tc.base)
		assert.Equal(t, tc.stem, stem, tc.base)
	}
}

func TestWikilinkReaches(t *testing.T) {
	for _, tc := range []struct {
		spelling, base string
		want           bool
	}{
		{"Guide", "Guide.md", true},
		{"guide", "Guide.md", true},
		{"v1.3.md", "v1.3.md", true},
		{"guide.mdx", "guide.mdx", true},
		{"logo.png", "logo.png", true},
		{"v1.3", "v1.3.md", false},
		{"COPYING", "COPYING", false},
		{"", ".md", false},
		{"C#", "C#.md", false},
		{"a|b", "a|b.md", false},
		{"[x]", "[x].md", false},
		{"x]", "x].txt", false},
		{"guide.md ", "guide.md ", false},
		{" notes", " notes.md", false},
		{"a\nb", "a\nb.md", false},
		{"a\rb", "a\rb.md", false},
		{"guide .md", "guide .md", true},
		{"C:x", "C:x.md", false},
		{`a\b`, `a\b.md`, false},
		{"..", "...md", false},
		{"guide", "other.md", false},
	} {
		assert.Equal(t, tc.want, WikilinkReaches(tc.spelling, tc.base), "%q -> %q", tc.spelling, tc.base)
	}
}
