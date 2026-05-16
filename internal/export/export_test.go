package export_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jeduden/mdsmith/internal/export"
	"github.com/jeduden/mdsmith/internal/lint"

	// Register the production directive rules (toc, catalog, include,
	// build, …) so export.Export sees them via rule.All().
	_ "github.com/jeduden/mdsmith/internal/rules/all"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFile builds a *lint.File from a source string for tests that
// don't need front-matter stripping or a real filesystem.
func newFile(t *testing.T, path, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile(path, []byte(src))
	require.NoError(t, err)
	return f
}

func TestExport_NoDirectives_Noop(t *testing.T) {
	src := "# Title\n\nSome content.\n\n## Section\n\nMore content.\n"
	f := newFile(t, "doc.md", src)

	out, diags := export.Export(f, export.NoCheck)
	require.Empty(t, diags)
	assert.Equal(t, src, string(out))
}

func TestExport_TOCMarkers_BodyKept(t *testing.T) {
	src := "# Title\n\n<?toc?>\n\n- [Title](#title)\n- [Two](#two)\n\n<?/toc?>\n\n## Two\n\nbody\n"
	f := newFile(t, "doc.md", src)

	out, diags := export.Export(f, export.NoCheck)
	require.Empty(t, diags)
	got := string(out)
	assert.NotContains(t, got, "<?toc")
	assert.NotContains(t, got, "<?/toc")
	assert.Contains(t, got, "- [Title](#title)")
	assert.Contains(t, got, "- [Two](#two)")
}

func TestExport_MarkerlessRequire_Removed(t *testing.T) {
	src := "---\nid: 1\n---\n<?require\nfilename: \"[0-9]*.md\"\n?>\n\n# Hello\n\nBody.\n"
	f, err := lint.NewFileFromSource("doc.md", []byte(src), true)
	require.NoError(t, err)

	out, diags := export.Export(f, export.NoCheck)
	require.Empty(t, diags)
	got := string(out)
	assert.NotContains(t, got, "<?require")
	assert.Contains(t, got, "---\nid: 1\n---")
	assert.Contains(t, got, "# Hello")
	assert.Contains(t, got, "Body.")
}

func TestExport_MarkerlessAllowEmptySection_Removed(t *testing.T) {
	src := "# Title\n\n## Stub\n\n<?allow-empty-section?>\n\n## Real\n\nbody\n"
	f := newFile(t, "doc.md", src)

	out, diags := export.Export(f, export.NoCheck)
	require.Empty(t, diags)
	got := string(out)
	assert.NotContains(t, got, "<?allow-empty-section?>")
	assert.Contains(t, got, "## Stub")
	assert.Contains(t, got, "## Real")
}

func TestExport_Include_BodyKeptInline(t *testing.T) {
	// Fresh include body — when stripped, the inlined content remains.
	src := "# Title\n\n<?include\nfile: snippet.md\n?>\n\nsnippet body\n\n<?/include?>\n\nAfter.\n"
	f := newFile(t, "doc.md", src)
	f.FS = fstest.MapFS{
		"snippet.md": &fstest.MapFile{Data: []byte("snippet body\n")},
	}

	out, diags := export.Export(f, export.NoCheck)
	require.Empty(t, diags)
	got := string(out)
	assert.NotContains(t, got, "<?include")
	assert.NotContains(t, got, "<?/include")
	assert.Contains(t, got, "snippet body")
	assert.Contains(t, got, "After.")
}

func TestExport_NestedSameTypeMarkers_LiteralContentSurvives(t *testing.T) {
	// A balanced inner <?toc?>...<?/toc?> inside an outer <?toc?>
	// pair is treated by the engine as literal content; only the
	// outermost markers are removed.
	src := strings.Join([]string{
		"# Title",
		"",
		"<?toc",
		"min-level: \"2\"",
		"?>",
		"- a",
		"<?toc?>",
		"- nested literal",
		"<?/toc?>",
		"- b",
		"<?/toc?>",
		"",
		"## Section",
		"",
	}, "\n")
	f := newFile(t, "doc.md", src)

	out, diags := export.Export(f, export.NoCheck)
	require.Empty(t, diags)
	got := string(out)
	// Outer markers gone.
	lines := strings.Split(got, "\n")
	leading := strings.Join(lines[:6], "\n")
	assert.NotContains(t, leading, "<?toc\nmin-level")
	// Inner same-type markers preserved as literal content.
	assert.Contains(t, got, "<?toc?>")
	assert.Contains(t, got, "<?/toc?>")
	assert.Contains(t, got, "- nested literal")
}

func TestExport_Idempotent(t *testing.T) {
	src := "# Title\n\n<?toc?>\n\n- [Section](#section)\n\n<?/toc?>\n\n## Section\n\nbody\n"
	f := newFile(t, "doc.md", src)

	first, diags := export.Export(f, export.NoCheck)
	require.Empty(t, diags)

	f2 := newFile(t, "doc.md", string(first))
	second, diags := export.Export(f2, export.NoCheck)
	require.Empty(t, diags)

	assert.Equal(t, string(first), string(second))
}

func TestExport_CheckMode_StaleBody_Refuses(t *testing.T) {
	// TOC body should be `- [Section](#section)` but file has wrong text.
	src := "# Title\n\n<?toc?>\n\n- [Wrong](#wrong)\n\n<?/toc?>\n\n## Section\n\nbody\n"
	f := newFile(t, "doc.md", src)

	out, diags := export.Export(f, export.Check)
	assert.Nil(t, out, "stale body must produce nil bytes")
	require.NotEmpty(t, diags)
	// Diagnostic mentions the offending directive and its location.
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "out of date") && d.RuleName == "toc" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected an 'out of date' diagnostic for toc, got %+v", diags)
}

func TestExport_FixMode_StaleBody_Regenerates(t *testing.T) {
	src := "# Title\n\n<?toc?>\n\n- [Wrong](#wrong)\n\n<?/toc?>\n\n## Section\n\nbody\n"
	f := newFile(t, "doc.md", src)

	out, diags := export.Export(f, export.Fix)
	require.Empty(t, diags)
	got := string(out)
	assert.NotContains(t, got, "<?toc")
	// Regenerated body links to the actual heading.
	assert.Contains(t, got, "- [Section](#section)")
	assert.NotContains(t, got, "Wrong")
}

func TestExport_NoCheckMode_StaleBody_ExportsAsIs(t *testing.T) {
	src := "# Title\n\n<?toc?>\n\n- [Wrong](#wrong)\n\n<?/toc?>\n\n## Section\n\nbody\n"
	f := newFile(t, "doc.md", src)

	out, diags := export.Export(f, export.NoCheck)
	require.Empty(t, diags)
	got := string(out)
	assert.NotContains(t, got, "<?toc")
	// On-disk body kept verbatim — wrong link survives.
	assert.Contains(t, got, "- [Wrong](#wrong)")
}

func TestExport_Catalog_MarkersRemoved_BodyKept(t *testing.T) {
	// A fresh catalog body is kept as-is once the markers are
	// stripped. The catalog directive needs an FS to discover files
	// for its glob, so wire a fake one.
	src := strings.Join([]string{
		"# Index",
		"",
		"<?catalog",
		"glob:",
		"  - \"*.md\"",
		"  - \"!index.md\"",
		"sort: filename",
		"row: \"- [{title}]({filename})\"",
		"?>",
		"- [Alpha](alpha.md)",
		"- [Beta](beta.md)",
		"<?/catalog?>",
		"",
	}, "\n")
	f := newFile(t, "index.md", src)
	f.FS = fstest.MapFS{
		"alpha.md": &fstest.MapFile{Data: []byte("---\ntitle: Alpha\n---\n# Alpha\n")},
		"beta.md":  &fstest.MapFile{Data: []byte("---\ntitle: Beta\n---\n# Beta\n")},
	}

	out, diags := export.Export(f, export.Check)
	require.Empty(t, diags, "fresh catalog should pass Check")
	got := string(out)
	assert.NotContains(t, got, "<?catalog")
	assert.NotContains(t, got, "<?/catalog")
	assert.Contains(t, got, "- [Alpha](alpha.md)")
	assert.Contains(t, got, "- [Beta](beta.md)")
	assert.Contains(t, got, "# Index")
}

func TestExport_FullSourceIncludesFrontMatter(t *testing.T) {
	src := "---\ntitle: Doc\n---\n# Title\n\n<?toc?>\n\n- [Section](#section)\n\n<?/toc?>\n\n## Section\n\nbody\n"
	f, err := lint.NewFileFromSource("doc.md", []byte(src), true)
	require.NoError(t, err)

	out, diags := export.Export(f, export.Check)
	require.Empty(t, diags)
	got := string(out)
	// Front matter is preserved exactly.
	assert.True(t, strings.HasPrefix(got, "---\ntitle: Doc\n---\n"),
		"expected front matter prefix, got: %q", got[:30])
	assert.NotContains(t, got, "<?toc")
	assert.Contains(t, got, "- [Section](#section)")
}

func TestExport_FreshOutputPassesCheck(t *testing.T) {
	// After Fix-mode export, the bytes should not contain any
	// directive markers, and the result should be a clean Markdown
	// document with no MDS003/MDS010 stitching artifacts.
	src := strings.Join([]string{
		"# Title",
		"",
		"<?toc?>",
		"",
		"- [Wrong](#wrong)",
		"",
		"<?/toc?>",
		"",
		"## Section",
		"",
		"body",
		"",
	}, "\n")
	f := newFile(t, "doc.md", src)

	out, diags := export.Export(f, export.Fix)
	require.Empty(t, diags)
	got := string(out)
	// No directive markers.
	assert.NotContains(t, got, "<?")
	// No 2+ consecutive blank lines.
	assert.NotContains(t, got, "\n\n\n",
		"output should not contain runs of multiple blank lines")
	// Ends with exactly one newline.
	assert.True(t, strings.HasSuffix(got, "\n"))
	assert.False(t, strings.HasSuffix(got, "\n\n"))
}

func TestExport_NoDirectives_FullSource(t *testing.T) {
	src := "---\nid: 1\n---\n# Hello\n\nNo directives here.\n"
	f, err := lint.NewFileFromSource("doc.md", []byte(src), true)
	require.NoError(t, err)

	out, diags := export.Export(f, export.Check)
	require.Empty(t, diags)
	assert.Equal(t, src, string(out),
		"export of a directive-free file should equal the input")
}

func TestExport_CheckMode_StaleBody_DiagnosticLine_IncludesFrontmatterOffset(t *testing.T) {
	// Front matter occupies lines 1-3; the stale <?toc?> sits at
	// file-relative line 6. The returned diagnostic must point at
	// the file-relative line so the CLI prints a navigable location.
	src := "---\nid: 1\n---\n# Hello\n\n<?toc?>\n\n- [Wrong](#wrong)\n\n<?/toc?>\n\n## Section\n\nbody\n"
	f, err := lint.NewFileFromSource("doc.md", []byte(src), true)
	require.NoError(t, err)

	out, diags := export.Export(f, export.Check)
	assert.Nil(t, out)
	require.NotEmpty(t, diags)
	assert.Equal(t, 6, diags[0].Line,
		"diagnostic line should be file-relative (include the 3-line front matter)")
}

func TestExport_CheckMode_SuppressesDiagnosticsInsideGeneratedRange(t *testing.T) {
	// An inner toc body inside an outer include's body is not the
	// host file's responsibility: the host file's GeneratedRanges
	// cover the include body, and any directive diagnostic anchored
	// there must be suppressed (matching `mdsmith check`).
	src := strings.Join([]string{
		"# Title",
		"",
		"<?include",
		"file: snippet.md",
		"?>",
		"<?toc?>",
		"",
		"- [Wrong](#wrong)",
		"",
		"<?/toc?>",
		"<?/include?>",
		"",
	}, "\n")
	f := newFile(t, "doc.md", src)
	f.FS = fstest.MapFS{
		"snippet.md": &fstest.MapFile{Data: []byte("snippet body\n")},
	}
	// Pretend the include body covers lines 6-10 (the lines that hold
	// the stale inner <?toc?> ... <?/toc?> markers); the host file is
	// not responsible for staleness within that range.
	f.GeneratedRanges = []lint.LineRange{{From: 6, To: 10}}

	out, diags := export.Export(f, export.Check)
	// Outer include itself is stale (its body should be `snippet
	// body\n`), so the export still refuses — but the diagnostic
	// points at the include marker, not at the suppressed inner toc.
	require.NotEmpty(t, diags)
	assert.Nil(t, out)
	for _, d := range diags {
		assert.NotEqual(t, "toc", d.RuleName,
			"diagnostics inside a GeneratedRange should be suppressed: %+v", d)
	}
}
