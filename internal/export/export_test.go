package export_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jeduden/mdsmith/internal/export"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"

	// Register the production directive rules (toc, catalog, include,
	// build, …) so allRules() picks them up via rule.All().
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

// allRules returns the registered ruleset unmodified — equivalent to
// what the CLI would pass when no settings narrow it. Tests that want
// to simulate a disabled rule build their own slice.
func allRules() []rule.Rule { return rule.All() }

// rulesExcept returns rule.All() with the directives named in skip
// removed, simulating an effective config where those rules are
// disabled.
func rulesExcept(skip ...string) []rule.Rule {
	out := rule.All()
	skipSet := map[string]bool{}
	for _, n := range skip {
		skipSet[n] = true
	}
	filtered := out[:0]
	for _, r := range out {
		if skipSet[r.Name()] {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

func TestExport_NoDirectives_Noop(t *testing.T) {
	src := "# Title\n\nSome content.\n\n## Section\n\nMore content.\n"
	f := newFile(t, "doc.md", src)

	out, diags := export.Export(f, export.NoCheck, allRules())
	require.Empty(t, diags)
	assert.Equal(t, src, string(out))
}

func TestExport_TOCMarkers_BodyKept(t *testing.T) {
	src := "# Title\n\n<?toc?>\n\n- [Title](#title)\n- [Two](#two)\n\n<?/toc?>\n\n## Two\n\nbody\n"
	f := newFile(t, "doc.md", src)

	out, diags := export.Export(f, export.NoCheck, allRules())
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

	out, diags := export.Export(f, export.NoCheck, allRules())
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

	out, diags := export.Export(f, export.NoCheck, allRules())
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

	out, diags := export.Export(f, export.NoCheck, allRules())
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

	out, diags := export.Export(f, export.NoCheck, allRules())
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

	first, diags := export.Export(f, export.NoCheck, allRules())
	require.Empty(t, diags)

	f2 := newFile(t, "doc.md", string(first))
	second, diags := export.Export(f2, export.NoCheck, allRules())
	require.Empty(t, diags)

	assert.Equal(t, string(first), string(second))
}

func TestExport_CheckMode_StaleBody_Refuses(t *testing.T) {
	// TOC body should be `- [Section](#section)` but file has wrong text.
	src := "# Title\n\n<?toc?>\n\n- [Wrong](#wrong)\n\n<?/toc?>\n\n## Section\n\nbody\n"
	f := newFile(t, "doc.md", src)

	out, diags := export.Export(f, export.Check, allRules())
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

func TestExport_CheckMode_DisabledDirective_NotFlaggedButStripped(t *testing.T) {
	// A stale <?toc?> body that would normally refuse the export is
	// silently allowed when the toc rule is excluded from the
	// effective config — but the markers still vanish, because
	// stripping is independent of which rules the caller passes.
	src := "# Title\n\n<?toc?>\n\n- [Wrong](#wrong)\n\n<?/toc?>\n\n## Section\n\nbody\n"
	f := newFile(t, "doc.md", src)

	out, diags := export.Export(f, export.Check, rulesExcept("toc"))
	require.Empty(t, diags, "disabled toc must not produce a stale-body refusal")
	got := string(out)
	assert.NotContains(t, got, "<?toc", "markers strip regardless of enabled state")
	// On-disk body kept verbatim — the wrong link survives, but no diag.
	assert.Contains(t, got, "- [Wrong](#wrong)")
}

func TestExport_FixMode_DisabledDirective_NotRegenerated(t *testing.T) {
	// In Fix mode with toc disabled, the stale body should NOT be
	// regenerated (the rule isn't in the active set), but its markers
	// still get stripped on the way out.
	src := "# Title\n\n<?toc?>\n\n- [Wrong](#wrong)\n\n<?/toc?>\n\n## Section\n\nbody\n"
	f := newFile(t, "doc.md", src)

	out, diags := export.Export(f, export.Fix, rulesExcept("toc"))
	require.Empty(t, diags)
	got := string(out)
	assert.NotContains(t, got, "<?toc")
	// Stale body survives because Fix didn't touch it.
	assert.Contains(t, got, "- [Wrong](#wrong)")
}

func TestExport_FixMode_StaleBody_Regenerates(t *testing.T) {
	src := "# Title\n\n<?toc?>\n\n- [Wrong](#wrong)\n\n<?/toc?>\n\n## Section\n\nbody\n"
	f := newFile(t, "doc.md", src)

	out, diags := export.Export(f, export.Fix, allRules())
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

	out, diags := export.Export(f, export.NoCheck, allRules())
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

	out, diags := export.Export(f, export.Check, allRules())
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

	out, diags := export.Export(f, export.Check, allRules())
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

	out, diags := export.Export(f, export.Fix, allRules())
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

	out, diags := export.Export(f, export.Check, allRules())
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

	out, diags := export.Export(f, export.Check, allRules())
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

	out, diags := export.Export(f, export.Check, allRules())
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

func TestExport_FixMode_FreshFile_DoesNotInvokeFix(t *testing.T) {
	// In Fix mode, when every directive is already fresh, the
	// underlying rule.Fix should NOT be called — the gate is "only
	// call Fix when Check fires". countingFixable wraps a real
	// directive rule to count Fix invocations.
	src := "# Title\n\n<?toc?>\n\n- [Section](#section)\n\n<?/toc?>\n\n## Section\n\nbody\n"
	f := newFile(t, "doc.md", src)

	wrappers := wrapDirectives(allRules())
	out, diags := export.Export(f, export.Fix, wrappers.rules)
	require.Empty(t, diags)
	require.NotNil(t, out)

	tocFixCount := wrappers.fixCalls("toc")
	assert.Equal(t, 0, tocFixCount,
		"a fresh toc body must not invoke Fix (got %d calls)", tocFixCount)
}

func TestExport_OnlyDirectives_OutputCollapsesToEmpty(t *testing.T) {
	// A file containing only markerless directives — and nothing else
	// once they're stripped — exercises the empty-output branch of
	// normalizeBlankLines so it does not insert a stray "\n" on output.
	src := "<?allow-empty-section?>\n\n<?require\nfilename: \"*.md\"\n?>\n"
	f := newFile(t, "doc.md", src)

	out, diags := export.Export(f, export.NoCheck, allRules())
	require.Empty(t, diags)
	assert.Empty(t, out, "all-directive file should normalise to empty output, got %q", string(out))
}

func TestExport_NormalizeBlankLines_EmptyInput(t *testing.T) {
	// A file that is empty bytes returns empty bytes unchanged —
	// guards normalizeBlankLines' fast-path.
	src := ""
	f := newFile(t, "doc.md", src)

	out, diags := export.Export(f, export.NoCheck, allRules())
	require.Empty(t, diags)
	assert.Empty(t, string(out))
}

func TestExport_SingleMarkerlessPI_FullyStripped(t *testing.T) {
	// A file whose only content is one markerless directive — after
	// stripping, emitLines produces nil bytes, exercising
	// normalizeBlankLines' empty-input fast path.
	src := "<?allow-empty-section?>\n"
	f := newFile(t, "doc.md", src)

	out, diags := export.Export(f, export.NoCheck, allRules())
	require.Empty(t, diags)
	assert.Empty(t, string(out))
}

func TestExport_UnclosedMarkerlessPI_PreservedAsLiteral(t *testing.T) {
	// An unclosed PI (`<?foo` without `?>`) is parsed by goldmark
	// as a PI with HasClosure()==false. piLineRange's no-closure
	// branch returns start==end. stripDirectives won't pair it
	// with any directive, so its line passes through to the output
	// — proving the path is reachable without changing observable
	// behavior.
	src := "# Title\n\n<?weird\n\nbody after blank\n"
	f := newFile(t, "doc.md", src)

	out, diags := export.Export(f, export.NoCheck, allRules())
	require.Empty(t, diags)
	got := string(out)
	// The "weird" PI line survives (no matching directive in the
	// registry, no closing marker, so stripDirectives leaves it).
	assert.Contains(t, got, "body after blank")
}

func TestExport_TrailingBlankLines_Collapsed(t *testing.T) {
	// After stripping markers, trailing blank lines beyond the
	// directive block should be trimmed by normalizeBlankLines'
	// trailing-blank loop.
	src := "<?toc?>\n- foo\n<?/toc?>\n\n\n\n"
	f := newFile(t, "doc.md", src)

	out, diags := export.Export(f, export.NoCheck, allRules())
	require.Empty(t, diags)
	got := string(out)
	assert.True(t, strings.HasSuffix(got, "\n"),
		"output should end with exactly one newline, got %q", got)
	assert.False(t, strings.HasSuffix(got, "\n\n"),
		"trailing blank lines should be trimmed, got %q", got)
}

func TestExport_DirectiveFreeFile_PassesThroughVerbatim(t *testing.T) {
	// The plan promises that exporting a directive-free file is a
	// no-op. Quirky whitespace — multiple consecutive blank lines,
	// no trailing newline — must survive unchanged.
	src := "# Title\n\n\n\nbody\n\n\n## Section\n\nmore"
	f := newFile(t, "doc.md", src)

	out, diags := export.Export(f, export.NoCheck, allRules())
	require.Empty(t, diags)
	assert.Equal(t, src, string(out),
		"directive-free files must round-trip byte-for-byte")
}

func TestExport_PreservesBlankLinesInFencedCodeBlock(t *testing.T) {
	// A fenced code block with intentional consecutive blank lines
	// must survive stripping. The Markdown rule MDS008 already
	// leaves code-block blanks alone; normalizeBlankLines is
	// expected to behave the same.
	src := strings.Join([]string{
		"# Title",
		"",
		"<?toc?>",
		"",
		"- [Section](#section)",
		"",
		"<?/toc?>",
		"",
		"## Section",
		"",
		"```",
		"line 1",
		"",
		"",
		"line 4 after two blank lines",
		"```",
		"",
	}, "\n")
	f := newFile(t, "doc.md", src)

	out, diags := export.Export(f, export.NoCheck, allRules())
	require.Empty(t, diags)
	got := string(out)
	// Markers gone.
	assert.NotContains(t, got, "<?toc")
	// Two consecutive blank lines inside the fenced block survive.
	assert.Contains(t, got, "line 1\n\n\nline 4 after two blank lines",
		"blank lines inside a fenced code block must not be collapsed; got:\n%s", got)
}

func TestExport_PreservesBlankLinesInIndentedCodeBlock(t *testing.T) {
	// Same guarantee for indented code blocks: blank lines that are
	// part of the block — leading-tab/4-space — should pass through.
	src := strings.Join([]string{
		"# Title",
		"",
		"<?toc?>",
		"",
		"- [Section](#section)",
		"",
		"<?/toc?>",
		"",
		"## Section",
		"",
		"prose",
		"",
		"    indented line",
		"",
		"    still indented after blank",
		"",
	}, "\n")
	f := newFile(t, "doc.md", src)

	out, diags := export.Export(f, export.NoCheck, allRules())
	require.Empty(t, diags)
	got := string(out)
	assert.Contains(t, got, "indented line")
	assert.Contains(t, got, "still indented after blank")
}

func TestExport_CheckMode_WarningSeverity_DoesNotRefuse(t *testing.T) {
	// checkStaleness only treats Error-severity diagnostics as
	// blocking. A Warning (e.g. catalog case-mismatch / injection
	// hints) must not turn into a refusal. The countingDirective
	// here returns a fresh body PLUS a warning, simulating that
	// non-blocking case.
	src := "# Title\n\n<?toc?>\n\n- [Section](#section)\n\n<?/toc?>\n\n## Section\n\nbody\n"
	f := newFile(t, "doc.md", src)

	rules := wrapDirectives(allRules())
	tocWrapper := rules.wrappers["toc"]
	require.NotNil(t, tocWrapper)
	tocWrapper.injectWarning = true

	out, diags := export.Export(f, export.Check, rules.rules)
	require.Empty(t, diags, "a Warning-severity diagnostic must not refuse")
	require.NotNil(t, out)
	assert.NotContains(t, string(out), "<?toc")
}

func TestExport_FixMode_StaleBody_InvokesFixForThatDirectiveOnly(t *testing.T) {
	// Same gate, opposite direction: when toc is stale, its Fix must
	// run; an unrelated, fresh directive's Fix must not.
	src := strings.Join([]string{
		"# Title",
		"",
		"<?toc?>",
		"",
		"- [Wrong](#wrong)",
		"",
		"<?/toc?>",
		"",
		"<?allow-empty-section?>",
		"",
		"## Section",
		"",
		"body",
		"",
	}, "\n")
	f := newFile(t, "doc.md", src)

	wrappers := wrapDirectives(allRules())
	out, diags := export.Export(f, export.Fix, wrappers.rules)
	require.Empty(t, diags)
	require.NotNil(t, out)

	assert.GreaterOrEqual(t, wrappers.fixCalls("toc"), 1,
		"stale toc must invoke Fix at least once")
}
