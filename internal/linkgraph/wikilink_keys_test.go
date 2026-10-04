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
		{"a`b", "a`b.md", false},
		{"guide .md", "guide .md", true},
		{"C:x", "C:x.md", false},
		{`a\b`, `a\b.md`, false},
		{"..", "...md", false},
		{"guide", "other.md", false},
	} {
		assert.Equal(t, tc.want, WikilinkReaches(tc.spelling, tc.base), "%q -> %q", tc.spelling, tc.base)
	}
}

// TestWikilinkAtRE_Anchored locks that the edge-column matcher only
// matches a link starting at offset 0, so a stale column whose row
// holds a link further on is refused without scanning to it.
func TestWikilinkAtRE_Anchored(t *testing.T) {
	assert.Nil(t, wikilinkAtRE.FindStringIndex("x [[a]]"))
	assert.Equal(t, []int{0, 5}, wikilinkAtRE.FindStringIndex("[[a]] x"))
	assert.Equal(t, []int{0, 6}, wikilinkAtRE.FindStringIndex("![[a]]"))
	assert.Equal(t, wikilinkRE.NumSubexp(), wikilinkAtRE.NumSubexp())
}

// TestStemResolvesTo locks which same-stem file a `[[stem]]` reaches:
// the shallowest, then alphabetically first, holder, counting p as a
// holder even when the index lacks it.
func TestStemResolvesTo(t *testing.T) {
	idx := NewWikilinkIndexFromPaths([]string{"docs/Guide.md", "ref/Guide.md", "a/b/guide.md"})
	for name, tc := range map[string]struct {
		idx  *WikilinkIndex
		key  string
		p    string
		want bool
	}{
		"first holder":             {idx, "guide", "docs/Guide.md", true},
		"later holder":             {idx, "guide", "ref/Guide.md", false},
		"deeper holder":            {idx, "guide", "a/b/guide.md", false},
		"unindexed shallower path": {idx, "guide", "guide.md", true},
		"unindexed sorts first":    {idx, "guide", "ab/guide.md", true},
		"unindexed sorts later":    {idx, "guide", "zz/guide.md", false},
		"capitals sort first":      {idx, "guide", "Zz/guide.md", true},
		"no holder":                {idx, "manual", "docs/manual.md", true},
		"nil index holds only p":   {nil, "guide", "ref/Guide.md", true},
		// On a case-insensitive file system Docs/guide.md may be the
		// indexed docs/guide.md, which sorts after b/guide.md.
		"holder spelled in other case": {
			NewWikilinkIndexFromPaths([]string{"b/guide.md", "docs/guide.md"}), "guide", "Docs/guide.md", false,
		},
		"first holder in other case": {
			NewWikilinkIndexFromPaths([]string{"docs/guide.md", "z/guide.md"}), "guide", "Docs/guide.md", false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.idx.StemResolvesTo(tc.key, tc.p))
		})
	}
}

// TestNameResolvesTo locks that a typed `[[name.ext]]` picks its file
// by the same order as a `[[stem]]`, counting p as a holder of the
// exact-name key.
func TestNameResolvesTo(t *testing.T) {
	idx := NewWikilinkIndexFromPaths([]string{"img/logo.png", "z/x/Logo.PNG"})
	assert.True(t, idx.NameResolvesTo("logo.png", "a/logo.png"), "a shallower path wins")
	assert.False(t, idx.NameResolvesTo("logo.png", "z/logo.png"), "img/ sorts first")
	assert.False(t, idx.NameResolvesTo("logo.png", "Img/logo.png"), "a holder in other case may be p")
	assert.True(t, idx.NameResolvesTo("logo.svg", "z/logo.svg"), "no holder")
	assert.True(t, (*WikilinkIndex)(nil).NameResolvesTo("logo.png", "z/logo.png"), "a nil index holds only p")
}
