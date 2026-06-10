package linkgraph

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
)

func TestExtractDirectives_Include(t *testing.T) {
	src := "# Top\n\n<?include\nfile: \"sub/x.md\"\n?>\n<?/include?>\n"
	f := newFile(t, src)
	edges := ExtractDirectives(f)
	require.Len(t, edges, 1)
	assert.Equal(t, DirectiveInclude, edges[0].Kind)
	assert.Equal(t, "sub/x.md", edges[0].Path)
	assert.False(t, edges[0].IsUnresolved())
}

func TestExtractDirectives_Build(t *testing.T) {
	// One literal inputs: entry yields one resolved build edge.
	src := "# Top\n\n<?build\nrecipe: r\ninputs:\n  - \"src.md\"\noutputs:\n  - out.html\n?>\n<?/build?>\n"
	f := newFile(t, src)
	edges := ExtractDirectives(f)
	require.Len(t, edges, 1)
	assert.Equal(t, DirectiveBuild, edges[0].Kind)
	assert.Equal(t, "src.md", edges[0].Path)
	assert.False(t, edges[0].IsUnresolved())
}

func TestExtractDirectives_Build_MultipleInputs(t *testing.T) {
	// One build edge per inputs: entry, in declared order.
	src := "# Top\n\n<?build\nrecipe: r\ninputs:\n  - a.md\n  - b.md\noutputs:\n  - out.html\n?>\n<?/build?>\n"
	f := newFile(t, src)
	edges := ExtractDirectives(f)
	require.Len(t, edges, 2)
	assert.Equal(t, DirectiveBuild, edges[0].Kind)
	assert.Equal(t, "a.md", edges[0].Path)
	assert.Equal(t, DirectiveBuild, edges[1].Kind)
	assert.Equal(t, "b.md", edges[1].Path)
}

func TestExtractDirectives_Build_GlobInput(t *testing.T) {
	// A glob inputs: entry is carried as an unresolved glob edge, like
	// a catalog directive.
	src := "# Top\n\n<?build\nrecipe: r\ninputs:\n  - chapters/*.md\noutputs:\n  - out.html\n?>\n<?/build?>\n"
	f := newFile(t, src)
	edges := ExtractDirectives(f)
	require.Len(t, edges, 1)
	assert.Equal(t, DirectiveBuild, edges[0].Kind)
	assert.Empty(t, edges[0].Path)
	assert.Equal(t, []string{"chapters/*.md"}, edges[0].Globs)
	assert.True(t, edges[0].IsUnresolved())
}

func TestExtractDirectives_Build_MixedLiteralAndGlobInputs(t *testing.T) {
	src := "# Top\n\n<?build\nrecipe: r\ninputs:\n  - intro.md\n  - chapters/**/*.md\n" +
		"outputs:\n  - out.html\n?>\n<?/build?>\n"
	f := newFile(t, src)
	edges := ExtractDirectives(f)
	require.Len(t, edges, 2)
	assert.Equal(t, "intro.md", edges[0].Path)
	assert.False(t, edges[0].IsUnresolved())
	assert.Equal(t, []string{"chapters/**/*.md"}, edges[1].Globs)
	assert.True(t, edges[1].IsUnresolved())
}

func TestExtractDirectives_Build_NoInputsSkipped(t *testing.T) {
	// A build directive with no inputs: contributes no edges (the
	// outputs are artifacts the file produces, not dependencies).
	src := "# Top\n\n<?build\nrecipe: r\noutputs:\n  - out.html\n?>\n<?/build?>\n"
	f := newFile(t, src)
	assert.Empty(t, ExtractDirectives(f),
		"build with no inputs: must not produce an edge")
}

func TestExtractDirectives_Catalog(t *testing.T) {
	src := "# Top\n\n<?catalog\nglob:\n  - \"docs/*.md\"\n  - \"!docs/internal/*.md\"\n?>\n<?/catalog?>\n"
	f := newFile(t, src)
	edges := ExtractDirectives(f)
	require.Len(t, edges, 1)
	assert.Equal(t, DirectiveCatalog, edges[0].Kind)
	assert.Equal(t, []string{"docs/*.md", "!docs/internal/*.md"}, edges[0].Globs)
	assert.Empty(t, edges[0].Path)
	assert.True(t, edges[0].IsUnresolved(),
		"catalog edges must be marked unresolved so reverse-edge queries skip them generically")
}

func TestExtractDirectives_Mixed(t *testing.T) {
	src := "# T\n\n" +
		"<?include\nfile: \"a.md\"\n?>\n<?/include?>\n\n" +
		"<?build\nrecipe: r\ninputs:\n  - \"b.md\"\noutputs:\n  - out.html\n?>\n<?/build?>\n\n" +
		"<?catalog\nglob: \"docs/*.md\"\n?>\n<?/catalog?>\n"
	f := newFile(t, src)
	edges := ExtractDirectives(f)
	require.Len(t, edges, 3)
	assert.Equal(t, DirectiveInclude, edges[0].Kind)
	assert.Equal(t, DirectiveBuild, edges[1].Kind)
	assert.Equal(t, "b.md", edges[1].Path)
	assert.Equal(t, DirectiveCatalog, edges[2].Kind)
}

func TestExtractDirectives_EmptyParamsSkipped(t *testing.T) {
	src := "# T\n\n<?include\nfile: \"\"\n?>\n<?/include?>\n"
	f := newFile(t, src)
	assert.Empty(t, ExtractDirectives(f),
		"empty include file: must not produce an edge (dedicated lint rule reports the diagnostic)")
}

func TestExtractDirectives_MalformedYAMLSkipped(t *testing.T) {
	src := "# T\n\n<?include\nfile: [unclosed\n?>\n<?/include?>\n"
	f := newFile(t, src)
	assert.Empty(t, ExtractDirectives(f))
}

func TestExtractDirectives_IgnoresClosingMarkers(t *testing.T) {
	src := "# T\n\n<?/include?>\n"
	f := newFile(t, src)
	assert.Empty(t, ExtractDirectives(f))
}

func TestExtractDirectives_IgnoresUnknownDirectiveNames(t *testing.T) {
	// allow-empty-section is not include/build/catalog → ExtractDirectives skips it.
	src := "# T\n\n<?allow-empty-section?>\n"
	f := newFile(t, src)
	assert.Empty(t, ExtractDirectives(f))
}

func TestExtractDirectives_NilFile(t *testing.T) {
	assert.Nil(t, ExtractDirectives(nil))
}

func TestExtractDirectives_RespectsFrontMatterOffset(t *testing.T) {
	source := []byte("---\ntitle: x\n---\n# T\n\n<?include\nfile: \"x.md\"\n?>\n<?/include?>\n")
	f, err := lint.NewFileFromSource("file.md", source, true)
	require.NoError(t, err)
	edges := ExtractDirectives(f)
	require.Len(t, edges, 1)
	// Body-relative: front matter (3 lines) is stripped, so the include
	// marker sits on body line 3 of the parsed body. Callers add
	// f.LineOffset for file-relative coordinates.
	assert.Equal(t, 3, edges[0].Line)
	assert.Equal(t, 3, f.LineOffset, "front matter occupies 3 lines")
}

func TestExpandCatalog(t *testing.T) {
	files := []string{
		"docs/intro.md",
		"docs/api.md",
		"docs/internal/notes.md",
		"plan/045.md",
	}
	cases := []struct {
		name  string
		globs []string
		want  []string
	}{
		{
			name:  "single include glob",
			globs: []string{"docs/*.md"},
			want:  []string{"docs/intro.md", "docs/api.md"},
		},
		{
			name:  "include with exclusion",
			globs: []string{"docs/**/*.md", "!docs/internal/**"},
			want:  []string{"docs/intro.md", "docs/api.md"},
		},
		{
			name:  "no globs",
			globs: nil,
			want:  nil,
		},
		{
			name:  "no matches",
			globs: []string{"no-such-dir/**"},
			want:  []string{},
		},
		{
			name:  "no files",
			globs: []string{"docs/*.md"},
			want:  nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			if tc.name == "no files" {
				got = ExpandCatalog(tc.globs, nil)
			} else {
				got = ExpandCatalog(tc.globs, files)
			}
			if tc.want == nil {
				assert.Nil(t, got)
				return
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestExpandCatalog_PreservesOrder(t *testing.T) {
	files := []string{"z.md", "a.md", "m.md"}
	got := ExpandCatalog([]string{"*.md"}, files)
	assert.Equal(t, files, got, "order matches input file list, not pattern order")
}

func TestExtractRefLinks_Basic(t *testing.T) {
	src := "# T\n\nSee [Foo][lab].\n\n[lab]: https://example.com\n"
	f := newFile(t, src)
	refs := ExtractRefLinks(f)
	require.Len(t, refs, 1)
	assert.Equal(t, "lab", refs[0].Label)
	assert.Equal(t, "Foo", refs[0].Text)
	assert.Equal(t, 3, refs[0].Line)
}

func TestExtractRefLinks_NormalizesLabel(t *testing.T) {
	// Mixed-case labels + internal whitespace collapse via ToLinkReference.
	src := "# T\n\nSee [x][Foo  Bar].\n\n[foo bar]: ./x.md\n"
	f := newFile(t, src)
	refs := ExtractRefLinks(f)
	require.Len(t, refs, 1)
	assert.Equal(t, "foo bar", refs[0].Label)
}

func TestExtractRefLinks_SkipsInlineLinks(t *testing.T) {
	src := "# T\n\nSee [x](./y.md) and [a][lab].\n\n[lab]: ./z.md\n"
	f := newFile(t, src)
	refs := ExtractRefLinks(f)
	require.Len(t, refs, 1, "inline link must be excluded")
	assert.Equal(t, "lab", refs[0].Label)
}

func TestExtractRefLinks_NilFile(t *testing.T) {
	assert.Nil(t, ExtractRefLinks(nil))
}

func TestResolveRelTarget(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		linkPath string
		want     string
	}{
		{"sibling", "docs/index.md", "api.md", "docs/api.md"},
		{"dot-prefix", "docs/index.md", "./api.md", "docs/api.md"},
		{"parent dir", "docs/sub/index.md", "../api.md", "docs/api.md"},
		{"escapes root", "docs/api.md", "../../etc/passwd", ""},
		{"absolute link", "docs/api.md", "/etc/passwd", ""},
		{"absolute source", "/abs/docs/api.md", "guide.md", ""},
		{"drive letter link", "docs/api.md", "C:/Windows/system.md", ""},
		{"drive letter source", "C:/docs/api.md", "guide.md", ""},
		{"UNC link", "docs/api.md", "//server/share/file.md", ""},
		{"UNC source", "//server/share/api.md", "guide.md", ""},
		{"backslash separator", "docs/a.md", `sub\x.md`, "docs/sub/x.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveRelTarget(tc.src, tc.linkPath)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDecodeAnchor(t *testing.T) {
	assert.Equal(t, "hello world", DecodeAnchor("hello%20world"))
	assert.Equal(t, "abc", DecodeAnchor("abc"))
	// Stray `%` — PathUnescape errors and the raw input is returned.
	assert.Equal(t, "foo%zz", DecodeAnchor("foo%zz"))
}
