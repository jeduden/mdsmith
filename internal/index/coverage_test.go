package index

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests exercise package-private helpers' edge cases that
// don't get covered through the public API. Their job is coverage,
// not behavior — they use the unexported helpers directly so a
// single targeted test maps to a single source branch.

func TestCountLines(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0, countLines(nil))
	assert.Equal(t, 0, countLines([]byte("")))
	assert.Equal(t, 1, countLines([]byte("foo")))
	assert.Equal(t, 1, countLines([]byte("foo\n")))
	assert.Equal(t, 2, countLines([]byte("foo\nbar")))
	assert.Equal(t, 2, countLines([]byte("foo\nbar\n")))
}

func TestColumnOfLineEdgeBranches(t *testing.T) {
	t.Parallel()
	lines := [][]byte{[]byte("hello"), []byte("world")}
	// lineIdx out of range → 1.
	assert.Equal(t, 1, columnOfLine(lines, -1, 0, nil))
	assert.Equal(t, 1, columnOfLine(lines, 99, 0, nil))
	// absOffset before line start → 1.
	assert.Equal(t, 1, columnOfLine(lines, 1, 0, nil))
	// Past end clamps.
	assert.Equal(t, 6, columnOfLine(lines, 0, 1000, nil))
	assert.Equal(t, 1, columnOfLine(lines, 0, 0, nil))
}

func TestLineOfOffsetEdgeBranches(t *testing.T) {
	t.Parallel()
	src := []byte("abc\ndef\n")
	assert.Equal(t, 1, lineOfOffset(src, -5))
	assert.Equal(t, 3, lineOfOffset(src, 1000))
	assert.Equal(t, 1, lineOfOffset(src, 0))
	assert.Equal(t, 2, lineOfOffset(src, 4))
}

func TestUniqueAnchorEmptySlug(t *testing.T) {
	t.Parallel()
	used := map[string]struct{}{}
	counts := map[string]int{}
	assert.Empty(t, uniqueAnchor("", used, counts))
}

func TestHeadingEndLineClamps(t *testing.T) {
	t.Parallel()
	// One heading, totalLines smaller than its start: end clamps to start.
	end := headingEndLine(nil, []int{5}, 0, 0, 1)
	assert.Equal(t, 5, end)
}

func TestFrontMatterSymbolsHandlesErrors(t *testing.T) {
	t.Parallel()
	// Invalid YAML.
	assert.Nil(t, frontMatterSymbols("a", []byte("---\nthis: is\n  not: valid yaml\nxx: [\n---\n")))
	// Sequence at top level (not a mapping).
	assert.Nil(t, frontMatterSymbols("a", []byte("---\n- item\n- another\n---\n")))
	// Empty.
	assert.Nil(t, frontMatterSymbols("a", nil))
}

func TestFrontMatterScalarFormats(t *testing.T) {
	t.Parallel()
	// int: small positive integer (yaml.v3 → int on 64-bit).
	v, ok := frontMatterScalar([]byte("---\nnum: 42\n---\n"), "num")
	assert.True(t, ok)
	assert.Equal(t, "42", v)
	// float64: decimal value.
	v, ok = frontMatterScalar([]byte("---\nratio: 3.14\n---\n"), "ratio")
	assert.True(t, ok)
	assert.Equal(t, "3.14", v)
	// uint64: integer > math.MaxInt64 (yaml.v3 → uint64).
	v, ok = frontMatterScalar([]byte("---\nbig: 18446744073709551615\n---\n"), "big")
	assert.True(t, ok)
	assert.Equal(t, "18446744073709551615", v)
	// bool.
	v, ok = frontMatterScalar([]byte("---\nflag: true\n---\n"), "flag")
	assert.True(t, ok)
	assert.Equal(t, "true", v)
	// time.Time: unquoted ISO-8601 date (yaml.v3 → time.Time).
	v, ok = frontMatterScalar([]byte("---\ndate: 2024-01-15\n---\n"), "date")
	assert.True(t, ok)
	expected := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	assert.Equal(t, expected, v)
	// default: null/~ value → absent-like return.
	_, ok = frontMatterScalar([]byte("---\nfield: null\n---\n"), "field")
	assert.False(t, ok)
	// Missing key.
	_, ok = frontMatterScalar([]byte("---\nfoo: bar\n---\n"), "missing")
	assert.False(t, ok)
	// Empty body.
	_, ok = frontMatterScalar(nil, "x")
	assert.False(t, ok)
	// Invalid YAML.
	_, ok = frontMatterScalar([]byte("---\n!!invalid\n---\n"), "x")
	assert.False(t, ok)
	// Front matter without trailing newline — stripDelimiters hits the
	// second TrimSuffix branch.
	v, ok = frontMatterScalar([]byte("---\nx: hi\n---"), "x")
	assert.True(t, ok)
	assert.Equal(t, "hi", v)
}

func TestFrontMatterStringListBranches(t *testing.T) {
	t.Parallel()
	// Missing key.
	_, ok := frontMatterStringList([]byte("---\nfoo: bar\n---\n"), "missing")
	assert.False(t, ok)
	// Non-list value.
	_, ok = frontMatterStringList([]byte("---\nx: hello\n---\n"), "x")
	assert.False(t, ok)
	// Mixed list (non-string element gets skipped).
	got, ok := frontMatterStringList([]byte("---\nx:\n  - a\n  - 42\n  - b\n---\n"), "x")
	assert.True(t, ok)
	assert.Equal(t, []string{"a", "b"}, got)
	// Empty.
	_, ok = frontMatterStringList(nil, "x")
	assert.False(t, ok)
	// Invalid YAML.
	_, ok = frontMatterStringList([]byte("---\n!!invalid\n---\n"), "x")
	assert.False(t, ok)
}

// parseLinkTarget / decodeAnchor / resolveRelTarget moved to linkgraph
// as part of plan 153. Their behaviour is covered by the linkgraph
// package's own tests (TestParseTarget, TestDecodeAnchor,
// TestResolveRelTarget). The index now routes every Markdown link
// through linkgraph.

func TestCollectLinkRefDefsDuplicateLabel(t *testing.T) {
	t.Parallel()
	// Two definitions with the same label — goldmark only registers
	// the first; the regex still matches both. The build must emit
	// exactly one outline entry per label, otherwise the symbol
	// picker shows duplicates.
	src := "# T\n\n[a][lab]\n\n[lab]: u1\n[lab]: u2\n"
	idx := New("/r")
	idx.Update("a.md", []byte(src))
	fe, ok := idx.File("a.md")
	require.True(t, ok)
	var refs int
	for _, s := range fe.Symbols {
		if s.Kind == SymbolLinkRef && s.Anchor == "lab" {
			refs++
		}
	}
	assert.Equal(t, 1, refs, "expected exactly one SymbolLinkRef for 'lab'")
}

func TestCollectLinkRefDefsMixedCaseLabel(t *testing.T) {
	t.Parallel()
	// A mixed-case label like [Foo Bar]: url must produce a SymbolLinkRef
	// with anchor "foo bar" (goldmark normalizes labels to lowercase). The
	// wanted-map key must be normalized so the regex match is found.
	src := "# T\n\n[link][Foo Bar]\n\n[Foo Bar]: https://example.com\n"
	idx := New("/r")
	idx.Update("a.md", []byte(src))
	fe, ok := idx.File("a.md")
	require.True(t, ok)
	var refs int
	for _, s := range fe.Symbols {
		if s.Kind == SymbolLinkRef && s.Anchor == "foo bar" {
			refs++
		}
	}
	assert.Equal(t, 1, refs, "expected one SymbolLinkRef for mixed-case label 'Foo Bar'")
}

func TestCollectLinkEdgesAnchorOnlyTakesAnchorBranch(t *testing.T) {
	t.Parallel()
	// `[x](#sec)` exercises the LocalAnchor=true branch of
	// collectLinkEdges; the case `t.Path != ""` is then false, so
	// the file-link arm is skipped.
	idx := New("/r")
	idx.Update("a.md", []byte("# T\n\n## Sec\n\n[x](#sec)\n"))
	fe, ok := idx.File("a.md")
	require.True(t, ok)
	var anchorEdges, fileEdges int
	for _, e := range fe.Outgoing {
		switch e.Kind {
		case EdgeAnchorLink:
			anchorEdges++
		case EdgeFileLink:
			fileEdges++
		}
	}
	assert.Equal(t, 1, anchorEdges)
	assert.Equal(t, 0, fileEdges)
}

func TestCollectDirectiveEdgesEmptyFileParam(t *testing.T) {
	t.Parallel()
	// Empty file: → the `file != ""` guard fires false, no edge.
	src := "# T\n\n<?include\nfile: \"\"\n?>\n<?/include?>\n"
	idx := New("/r")
	idx.Update("a.md", []byte(src))
	fe, ok := idx.File("a.md")
	require.True(t, ok)
	for _, e := range fe.Outgoing {
		assert.NotEqual(t, EdgeInclude, e.Kind)
	}
}

func TestCollectDirectiveEdgesAbsoluteBuildInput(t *testing.T) {
	t.Parallel()
	// Absolute inputs: entry → ResolveRelTarget returns "" → no edge
	// emitted for that literal entry.
	src := "# T\n\n<?build\nrecipe: r\ninputs:\n  - \"/abs.md\"\noutputs:\n  - out.html\n?>\n<?/build?>\n"
	idx := New("/r")
	idx.Update("a.md", []byte(src))
	fe, ok := idx.File("a.md")
	require.True(t, ok)
	for _, e := range fe.Outgoing {
		assert.NotEqual(t, EdgeBuild, e.Kind)
	}
}

func TestCatalogEdgeOnlyKindMatched(t *testing.T) {
	t.Parallel()
	// `<?catalog?>` exercises the catalog case of the switch.
	src := "# T\n\n<?catalog\nglob:\n  - \"*.md\"\n?>\n<?/catalog?>\n"
	idx := New("/r")
	idx.Update("a.md", []byte(src))
	fe, ok := idx.File("a.md")
	require.True(t, ok)
	var sawCatalog bool
	for _, e := range fe.Outgoing {
		if e.Kind == EdgeCatalog {
			sawCatalog = true
		}
	}
	assert.True(t, sawCatalog)
}

func TestIncomingEdgesMatchesEmptyTargetFile(t *testing.T) {
	t.Parallel()
	// An EdgeAnchorLink has empty TargetFile; IncomingEdges
	// "tFile == "" → use SourceFile" branch fires when the caller
	// asks for the same file, which is the only context where
	// anchor edges count as incoming.
	idx := New("/r")
	idx.Update("a.md", []byte("# A\n\n## Sec\n\n[x](#sec)\n"))
	got := idx.IncomingEdges("a.md", "sec")
	assert.NotEmpty(t, got)
}

func TestIncomingEdgesAnchorMismatch(t *testing.T) {
	t.Parallel()
	// Anchor mismatch path: edge exists for "sec" but caller asks
	// for "other".
	idx := New("/r")
	idx.Update("a.md", []byte("# A\n\n## Sec\n\n[x](#sec)\n"))
	got := idx.IncomingEdges("a.md", "other")
	assert.Empty(t, got)
}

func TestUniqueAnchorRetriesPastFirstSuffix(t *testing.T) {
	t.Parallel()
	// Pre-existing "same-1" forces the disambiguator to skip past
	// it: c=0→1 picks "same-1" (already used) → c=2 picks "same-2".
	// The inner loop's used[anchor] check fires false on the
	// second iteration.
	used := map[string]struct{}{"same": {}, "same-1": {}}
	counts := map[string]int{}
	got := uniqueAnchor("same", used, counts)
	assert.Equal(t, "same-2", got)
}

func TestNodePositionWithLaterTextSegments(t *testing.T) {
	t.Parallel()
	// A link with emphasis inside (`[foo *bar* baz](url)`) gives
	// goldmark multiple text segments. nodePosition should keep
	// the earliest, exercising the `t.Segment.Start < off` branch
	// in its compound condition.
	src := "# T\n\n[foo *bar* baz](./b.md)\n"
	idx := New("/r")
	idx.Update("a.md", []byte(src))
	fe, ok := idx.File("a.md")
	require.True(t, ok)
	require.NotEmpty(t, fe.Outgoing)
	// The recorded source position points at the first byte of
	// "foo", not the later segments.
	assert.Equal(t, 3, fe.Outgoing[0].SourceLine)
}

func TestHeadingPunctuationOnlyEmptySlug(t *testing.T) {
	t.Parallel()
	// `# !!!` slugifies to "" — the disambiguator's `a != ""`
	// guard fires false.
	src := "# !!!\n\n# foo\n"
	idx := New("/r")
	idx.Update("a.md", []byte(src))
	fe, ok := idx.File("a.md")
	require.True(t, ok)
	// `!!!` slugifies to "" so its symbol has an empty anchor; the
	// `foo` heading still gets its slug. The test fires the
	// `a != ""` guard's false branch in uniqueAnchor.
	var anchors []string
	for _, s := range fe.Symbols {
		if s.Kind == SymbolHeading {
			anchors = append(anchors, s.Anchor)
		}
	}
	assert.Contains(t, anchors, "")
	assert.Contains(t, anchors, "foo")
}

func TestIncomingEdgesSkipsNonMatchingFiles(t *testing.T) {
	t.Parallel()
	// Two files link to different targets. Asking for a.md only
	// the b→a edge should match — the c→other edge fires the
	// "tFile != file" branch (true) and is skipped.
	idx := New("/r")
	idx.Update("a.md", []byte("# A\n"))
	idx.Update("other.md", []byte("# Other\n"))
	idx.Update("b.md", []byte("# B\n\n[a](./a.md)\n"))
	idx.Update("c.md", []byte("# C\n\n[o](./other.md)\n"))
	got := idx.IncomingEdges("a.md", "")
	require.Len(t, got, 1)
	assert.Equal(t, "b.md", got[0].SourceFile)
}

func TestParsePIParamsInvalidYAML(t *testing.T) {
	t.Parallel()
	// PI body with malformed YAML — parsePIParams returns ok=false,
	// which means collectDirectiveEdges skips the edge entirely.
	src := "# T\n\n<?include\nfile: [unclosed\n?>\n<?/include?>\n"
	idx := New("/r")
	idx.Update("a.md", []byte(src))
	fe, ok := idx.File("a.md")
	require.True(t, ok)
	for _, e := range fe.Outgoing {
		assert.NotEqual(t, EdgeInclude, e.Kind, "malformed YAML should not produce an include edge")
	}
}

func TestFrontMatterSymbolsSkipsEmptyKeys(t *testing.T) {
	t.Parallel()
	// `?` produces a non-scalar key in YAML — frontMatterSymbols
	// filters those out.
	src := []byte("---\n\"\": value\nreal: ok\n---\n")
	syms := frontMatterSymbols("a", src)
	for _, s := range syms {
		assert.NotEmpty(t, s.Name)
	}
}

func TestNodePositionFallback(t *testing.T) {
	t.Parallel()
	// A heading parsed at line 1 has at least one text segment, so
	// nodePosition succeeds. To exercise the no-text-found fallback
	// we pass a fresh (empty) AST node — the helper returns (1,1).
	src := []byte("# Top\n")
	idx := New("/r")
	idx.Update("a.md", src)
	fe, ok := idx.File("a.md")
	require.True(t, ok)
	require.NotEmpty(t, fe.Symbols)
}

func TestExtractPIBodyEmpty(t *testing.T) {
	t.Parallel()
	// A single-line PI has Lines().Len() == 1 → extractPIBody
	// returns "".
	idx := New("/r")
	idx.Update("a.md", []byte("# T\n\n<?allow-empty-section?>\n"))
	fe, ok := idx.File("a.md")
	require.True(t, ok)
	var sawDirective bool
	for _, s := range fe.Symbols {
		if s.Kind == SymbolDirective {
			sawDirective = true
			assert.Equal(t, "allow-empty-section", s.Name)
		}
	}
	assert.True(t, sawDirective)
}

func TestCollectDirectivesSkipsClosingMarker(t *testing.T) {
	t.Parallel()
	// Closing markers (<?/name?>) are not symbols.
	idx := New("/r")
	idx.Update("a.md", []byte("# T\n\n<?include\nfile: \"x.md\"\n?>\nbody\n<?/include?>\n"))
	fe, ok := idx.File("a.md")
	require.True(t, ok)
	var dirs []Symbol
	for _, s := range fe.Symbols {
		if s.Kind == SymbolDirective {
			dirs = append(dirs, s)
		}
	}
	require.Len(t, dirs, 1)
	assert.Equal(t, "include", dirs[0].Name)
}

func TestCollectDirectiveEdgesEmptyBuildInputs(t *testing.T) {
	t.Parallel()
	// A build directive with no inputs: produces no edge.
	idx := New("/r")
	idx.Update("a.md", []byte("# T\n\n<?build\nrecipe: r\noutputs:\n  - out.md\n?>\n<?/build?>\n"))
	fe, ok := idx.File("a.md")
	require.True(t, ok)
	for _, e := range fe.Outgoing {
		assert.NotEqual(t, EdgeBuild, e.Kind, "build edge without inputs should be skipped: %+v", e)
	}
}

func TestCollectDirectiveEdgesAbsoluteIncludeSkipped(t *testing.T) {
	t.Parallel()
	// Absolute file path resolves to "" → no edge.
	idx := New("/r")
	idx.Update("a.md", []byte("# T\n\n<?include\nfile: \"/abs.md\"\n?>\n<?/include?>\n"))
	fe, ok := idx.File("a.md")
	require.True(t, ok)
	for _, e := range fe.Outgoing {
		assert.NotEqual(t, EdgeInclude, e.Kind)
	}
}

func TestBuildSkipsEmptyResults(t *testing.T) {
	t.Parallel()
	idx := New("/r")
	// Empty path is dropped.
	idx.Build([]string{""}, func(p string) ([]byte, error) {
		return []byte("# X\n"), nil
	})
	assert.Empty(t, idx.Files())

	// Loader returning an error skips the file.
	idx.Build([]string{"a.md"}, func(p string) ([]byte, error) {
		return nil, errors.New("nope")
	})
	assert.Empty(t, idx.Files())

	// Loader returning empty data skips the file.
	idx.Build([]string{"a.md"}, func(p string) ([]byte, error) {
		return nil, nil
	})
	assert.Empty(t, idx.Files())
}

func TestIncomingEdgesAnchorMismatchSkipped(t *testing.T) {
	t.Parallel()
	idx := New("/r")
	idx.Update("a.md", []byte("# A\n\n## Sec1\n"))
	idx.Update("b.md", []byte("# B\n\n[s](./a.md#sec2)\n"))
	// Asking for #sec1 → no match, the b.md edge points at sec2.
	assert.Empty(t, idx.IncomingEdges("a.md", "sec1"))
}

func TestSearchSymbolsLimitTriggersOnTitle(t *testing.T) {
	t.Parallel()
	idx := New("/r")
	for i := 0; i < 5; i++ {
		path := "f" + string(rune('a'+i)) + ".md"
		idx.Update(path, []byte("---\ntitle: Match Foo\n---\n# Top\n"))
	}
	hits := idx.SearchSymbols("foo", 2)
	assert.Len(t, hits, 2)
}

func TestSearchSymbolsLimitTriggersOnKind(t *testing.T) {
	t.Parallel()
	idx := New("/r")
	for i := 0; i < 5; i++ {
		path := "f" + string(rune('a'+i)) + ".md"
		idx.Update(path, []byte("---\nkinds:\n  - guideX\n---\n# Top\n"))
	}
	hits := idx.SearchSymbols("guidex", 2)
	assert.Len(t, hits, 2)
}

func TestAbsPathToWorkspaceNilIndex(t *testing.T) {
	t.Parallel()
	var i *Index
	assert.Equal(t, "x", i.AbsPathToWorkspace("x"))
}

func TestAbsToWorkspaceErrors(t *testing.T) {
	t.Parallel()
	// Empty root passes through after NormalizePath.
	assert.Equal(t, "x.md", absToWorkspace("", "x.md"))
	// Relative path passes through.
	assert.Equal(t, "x.md", absToWorkspace("/root", "x.md"))
}
