package linkgraph

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractDirectives_IncludeAndBuild(t *testing.T) {
	src := "# T\n\n<?include\nfile: \"sub/x.md\"\n?>\n<?/include?>\n\n" +
		"<?build\nsource: src.md\n?>\n<?/build?>\n"
	f := newFile(t, src)
	dirs := ExtractDirectives(f)
	require.Len(t, dirs, 2)

	assert.Equal(t, DirectiveInclude, dirs[0].Kind)
	assert.Equal(t, "sub/x.md", dirs[0].TargetPath)
	assert.False(t, dirs[0].Unresolved)
	assert.Empty(t, dirs[0].Globs)

	assert.Equal(t, DirectiveBuild, dirs[1].Kind)
	assert.Equal(t, "src.md", dirs[1].TargetPath)
}

func TestExtractDirectives_CatalogIsUnresolved(t *testing.T) {
	src := "# T\n\n<?catalog\nglob:\n  - \"docs/**/*.md\"\n  - \"!docs/draft/*.md\"\n?>\n<?/catalog?>\n"
	f := newFile(t, src)
	dirs := ExtractDirectives(f)
	require.Len(t, dirs, 1)
	d := dirs[0]
	assert.Equal(t, DirectiveCatalog, d.Kind)
	assert.True(t, d.Unresolved, "catalog edges must carry the Unresolved flag")
	assert.Empty(t, d.TargetPath, "catalog edges must not carry a single TargetPath")
	assert.Equal(t, []string{"docs/**/*.md", "!docs/draft/*.md"}, d.Globs)
}

func TestExtractDirectives_SkipsClosingMarkers(t *testing.T) {
	src := "# T\n\n<?include\nfile: a.md\n?>\n<?/include?>\n"
	f := newFile(t, src)
	dirs := ExtractDirectives(f)
	require.Len(t, dirs, 1, "closing marker <?/include?> must not produce an edge")
	assert.Equal(t, DirectiveInclude, dirs[0].Kind)
}

func TestExtractDirectives_DropsMalformedYAML(t *testing.T) {
	src := "# T\n\n<?include\n  : not valid yaml\n?>\n<?/include?>\n"
	f := newFile(t, src)
	assert.Empty(t, ExtractDirectives(f),
		"directives whose YAML body fails to parse must not emit an edge")
}

func TestExtractDirectives_SkipsIncludeWithoutFile(t *testing.T) {
	src := "# T\n\n<?include\nstrip-frontmatter: \"true\"\n?>\n<?/include?>\n"
	f := newFile(t, src)
	assert.Empty(t, ExtractDirectives(f),
		"include directives without a file: arg produce no edge")
}

func TestExtractDirectives_NilFile(t *testing.T) {
	assert.Nil(t, ExtractDirectives(nil))
}

func TestExpandCatalog(t *testing.T) {
	files := []string{
		"docs/a.md",
		"docs/b.md",
		"docs/draft/c.md",
		"plan/p.md",
	}
	got := ExpandCatalog([]string{"docs/**/*.md", "!docs/draft/**"}, files)
	assert.Equal(t, []string{"docs/a.md", "docs/b.md"}, got)
}

func TestExpandCatalog_EmptyInputs(t *testing.T) {
	assert.Nil(t, ExpandCatalog(nil, []string{"a.md"}))
	assert.Nil(t, ExpandCatalog([]string{"*.md"}, nil))
	// Only exclusion patterns → nothing matches.
	assert.Nil(t, ExpandCatalog([]string{"!docs/*.md"}, []string{"docs/a.md"}))
}

func TestExpandCatalog_InvalidExcludeSkipped(t *testing.T) {
	// `[` is not a valid doublestar pattern; the exclude should be
	// skipped (not silently suppress the whole include).
	got := ExpandCatalog([]string{"*.md", "!["}, []string{"a.md"})
	assert.Equal(t, []string{"a.md"}, got)
}
