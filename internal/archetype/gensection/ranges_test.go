package gensection

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustNewFile(t *testing.T, path, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile(path, []byte(src))
	require.NoError(t, err)
	return f
}

func TestFindAllGeneratedRanges_NoDirectives(t *testing.T) {
	f := mustNewFile(t, "plain.md", "# Hello\n\nSome text.\n")
	ranges := FindAllGeneratedRanges(f)
	assert.Empty(t, ranges, "no directives means no generated ranges")
}

func TestFindAllGeneratedRanges_IncludeSection(t *testing.T) {
	// Lines:
	// 1: # Host
	// 2: (empty)
	// 3: <?include
	// 4: file: frag.md
	// 5: ?>
	// 6: embedded line one
	// 7: embedded line two
	// 8: <?/include?>
	src := "# Host\n\n<?include\nfile: frag.md\n?>\nembedded line one\nembedded line two\n<?/include?>\n"
	f := mustNewFile(t, "host.md", src)

	ranges := FindAllGeneratedRanges(f)
	require.Len(t, ranges, 1)
	assert.Equal(t, 6, ranges[0].From)
	assert.Equal(t, 7, ranges[0].To)
}

func TestFindAllGeneratedRanges_CatalogSection(t *testing.T) {
	// Lines:
	// 1: # Catalog
	// 2: (empty)
	// 3: <?catalog
	// 4: glob: "*.md"
	// 5: ?>
	// 6: - foo.md
	// 7: <?/catalog?>
	src := "# Catalog\n\n<?catalog\nglob: \"*.md\"\n?>\n- foo.md\n<?/catalog?>\n"
	f := mustNewFile(t, "catalog.md", src)

	ranges := FindAllGeneratedRanges(f)
	require.Len(t, ranges, 1)
	assert.Equal(t, 6, ranges[0].From)
	assert.Equal(t, 6, ranges[0].To)
}

func TestFindAllGeneratedRanges_EmptyBody(t *testing.T) {
	// No content between markers: ContentFrom > ContentTo → no range recorded.
	src := "# Host\n\n<?include\nfile: frag.md\n?>\n<?/include?>\n"
	f := mustNewFile(t, "host.md", src)

	ranges := FindAllGeneratedRanges(f)
	assert.Empty(t, ranges, "empty generated body must not produce a range")
}

func TestFindAllGeneratedRanges_MultipleDirectives(t *testing.T) {
	// Two <?include?> sections.
	src := "# Host\n\n" +
		"<?include\nfile: a.md\n?>\nfrom a\n<?/include?>\n\n" +
		"<?include\nfile: b.md\n?>\nfrom b\n<?/include?>\n"
	f := mustNewFile(t, "host.md", src)

	ranges := FindAllGeneratedRanges(f)
	require.Len(t, ranges, 2, "two include sections must yield two ranges")
}

// --- AuthoredSource tests ---

func TestAuthoredSource_NoDirectives(t *testing.T) {
	src := []byte("# Plain\n\nSome text.\n")
	got := AuthoredSource(src)
	assert.Equal(t, src, got, "source without directives must be returned unchanged")
}

func TestAuthoredSource_StripIncludeBody(t *testing.T) {
	// Host with three lines in the generated body.
	src := "# Host\n\n" +
		"<?include\nfile: frag.md\n?>\n" +
		"embedded 1\nembedded 2\nembedded 3\n" +
		"<?/include?>\n\n" +
		"# After\n"
	expected := "# Host\n\n" +
		"<?include\nfile: frag.md\n?>\n" +
		"<?/include?>\n\n" +
		"# After\n"
	got := string(AuthoredSource([]byte(src)))
	assert.Equal(t, expected, got)
}

func TestAuthoredSource_StripCatalogBody(t *testing.T) {
	src := "# Catalog\n\n<?catalog\nglob: \"*.md\"\n?>\n- a.md\n- b.md\n<?/catalog?>\n"
	expected := "# Catalog\n\n<?catalog\nglob: \"*.md\"\n?>\n<?/catalog?>\n"
	got := string(AuthoredSource([]byte(src)))
	assert.Equal(t, expected, got)
}

func TestAuthoredSource_EmptyBody(t *testing.T) {
	// No content between markers: AuthoredSource must round-trip unchanged.
	src := "# Host\n\n<?include\nfile: frag.md\n?>\n<?/include?>\n"
	got := AuthoredSource([]byte(src))
	assert.Equal(t, src, string(got), "empty-body include must be unchanged by AuthoredSource")
}

func TestLineRange_Contains(t *testing.T) {
	r := lint.LineRange{From: 5, To: 8}
	assert.True(t, r.Contains(5))
	assert.True(t, r.Contains(6))
	assert.True(t, r.Contains(8))
	assert.False(t, r.Contains(4))
	assert.False(t, r.Contains(9))
}
