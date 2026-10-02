package refactor

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubWorkspace is a fully controllable Workspace for exercising the
// move planner's defensive branches (stale edges, unreadable files)
// that a real index over a memWorkspace can never produce.
type stubWorkspace struct {
	pathEdges     []index.Edge
	wikilinkEdges []index.Edge
	files         []string
	sources       map[string][]byte
	unresolvable  map[string]bool
}

func (s stubWorkspace) IncomingAnchorEdges(string, string) []index.Edge { return nil }
func (s stubWorkspace) IncomingPathEdges(string) []index.Edge           { return s.pathEdges }
func (s stubWorkspace) IncomingWikilinkEdges(string) []index.Edge       { return s.wikilinkEdges }
func (s stubWorkspace) Files() []string                                 { return s.files }
func (s stubWorkspace) Resolve(file string) (string, []byte, bool) {
	rel := index.NormalizePath(file)
	if s.unresolvable[rel] {
		return "", nil, false
	}
	src, ok := s.sources[rel]
	return rel, src, ok
}

func TestMove_TypedErrorMessages(t *testing.T) {
	assert.Equal(t, "destination already exists: b.md", DestinationExistsError{Dst: "b.md"}.Error())
	assert.Equal(t, "source file not found: a.md", SourceNotFoundError{Src: "a.md"}.Error())
}

func TestWorkspaceRelative(t *testing.T) {
	assert.False(t, workspaceRelative(""))
	assert.False(t, workspaceRelative("/abs/x.md"))
	assert.False(t, workspaceRelative(".."))
	assert.False(t, workspaceRelative("../x.md"))
	assert.True(t, workspaceRelative("a/b.md"))
}

func TestRelFrom_ErrorFallsBackToTarget(t *testing.T) {
	// filepath.Rel cannot make "b" relative to a "../a" base, so relFrom
	// returns the target unchanged.
	assert.Equal(t, "b", relFrom("../a", "b"))
}

func TestFileStem_NonMarkdownFallback(t *testing.T) {
	// A typed non-Markdown basename has no wikilink stem; fileStem falls
	// back to the lowercased basename.
	assert.Equal(t, "image.png", fileStem("dir/Image.PNG"))
	assert.Equal(t, "api", fileStem("docs/API.md"))
}

func TestWikilinkKeyHolders_OldStem(t *testing.T) {
	files := []string{"a.md", "docs/API.md", "api/api.md", "img/api.png", "notes/b.mdx", "notes/c.markdown"}
	licenseFiles := []string{"notes/LICENSE", "docs/license.md"}
	for name, tc := range map[string]struct {
		files []string
		stem  string
		want  int
	}{
		"no files":                       {nil, "api", 0},
		"no match":                       {files, "missing", 0},
		"single match":                   {files, "a", 1},
		"same stem in two directories":   {files, "api", 2},
		"case-folded basename":           {[]string{"docs/API.md"}, "api", 1},
		"markdown extension is stripped": {files, "c", 1},
		"upper-case markdown extension":  {[]string{"docs/Guide.MD"}, "guide", 1},
		"extensionless file is no stem":  {licenseFiles, "license", 1},
		"stem is not a prefix match":     {files, "ap", 0},
	} {
		t.Run(name, func(t *testing.T) {
			// src is listed as the first file so it adds no extra holder.
			files := append([]string{"src.txt"}, tc.files...)
			oldN, _ := wikilinkKeyHolders(files, "src.txt", tc.stem, "zzz", true)
			assert.Equal(t, tc.want, oldN)
			_, newN := wikilinkKeyHolders(files, "src.txt", "zzz", tc.stem, true)
			assert.Equal(t, tc.want, newN, "a Markdown destination counts stems the same way")
		})
	}
}

func TestWikilinkKeyHolders_NewName(t *testing.T) {
	files := []string{"a.md", "img/api.png", "notes/b.mdx", "x/B.MDX"}
	for name, tc := range map[string]struct {
		files []string
		base  string
		want  int
	}{
		"no files":                     {nil, "api.png", 0},
		"non-markdown keeps extension": {files, "api.png", 1},
		"mdx keeps its extension":      {files, "b.mdx", 2},
		"markdown name matches":        {files, "a.md", 1},
		"no prefix match":              {files, "api", 0},
	} {
		t.Run(name, func(t *testing.T) {
			_, newN := wikilinkKeyHolders(tc.files, "", "zzz", tc.base, false)
			assert.Equal(t, tc.want, newN)
		})
	}
}

func TestWikilinkKeyHolders_UnlistedSourceCounts(t *testing.T) {
	files := []string{"docs/guide.md"}
	oldN, _ := wikilinkKeyHolders(files, "a/guide.md", "guide", "manual", true)
	assert.Equal(t, 2, oldN, "an unlisted source holds its own stem")
	oldN, _ = wikilinkKeyHolders(append(files, "a/guide.md"), "a/guide.md", "guide", "manual", true)
	assert.Equal(t, 2, oldN, "a listed source is not counted twice")
}

func TestDstStemSpelling_NonMarkdownKeepsBase(t *testing.T) {
	assert.Equal(t, "Service", dstStemSpelling("docs/Service.md"))
	assert.Equal(t, "diagram.png", dstStemSpelling("img/diagram.png"))
}

func TestDstStemSpelling_DottedStemKeepsMarkdownExt(t *testing.T) {
	assert.Equal(t, "v1.3.md", dstStemSpelling("docs/v1.3.md"))
	assert.Equal(t, "guide.md.md", dstStemSpelling("docs/guide.md.md"))
	assert.Equal(t, "", dstStemSpelling("docs/.md"))
}

// locatedTokens parses body and returns each located destination as
// the bytes at its recorded row position, in walk order. Reading the
// row (not the parsed bytes) pins where each destination was found.
func locatedTokens(t *testing.T, body string) []string {
	t.Helper()
	lf, err := lint.NewFile("a.md", []byte(body))
	require.NoError(t, err)
	loc := destLocator{lf: lf, fileLines: splitLines(lf.Source)}
	_ = ast.Walk(lf.AST, loc.visit)
	toks := make([]string, 0, len(loc.dests))
	for _, d := range loc.dests {
		toks = append(toks, string(d.row[d.ps:d.ps+len(d.dest)]))
	}
	return toks
}

func TestDestLocator(t *testing.T) {
	t.Run("text-less nodes and nested images keep their own destinations", func(t *testing.T) {
		assert.Equal(t, []string{"a.png", "b.md", "c.png", "c.png"},
			locatedTokens(t, "[![](a.png)](b.md) [![x](c.png)](c.png)\n"))
	})
	t.Run("label content never supplies the destination", func(t *testing.T) {
		assert.Equal(t, []string{"code.md", "html.md", "auto.md"},
			locatedTokens(t, "[`](x.md)` c](code.md)\n\n"+
				"[<i title=\"](x.md)\">i</i>](html.md)\n\n"+
				"![<http://h](x.md)>](auto.md)\n"))
	})
	t.Run("raw HTML that ends the label moves the cursor past it", func(t *testing.T) {
		// No text follows the tag, so only the raw-HTML segment can move
		// the cursor past the `](` inside its attribute.
		assert.Equal(t, []string{"html.md"},
			locatedTokens(t, "[<i title=\"](x.md)\">](html.md)\n"))
	})
	t.Run("reference-style uses are not located, their definition is", func(t *testing.T) {
		assert.Equal(t, []string{"b.png", "a.png"},
			locatedTokens(t, "![][r] ![](b.png) [t][r]\n\n[r]: a.png\n"))
	})
	t.Run("destination on the row after the label is located", func(t *testing.T) {
		assert.Equal(t, []string{"b.md", "c.md"},
			locatedTokens(t, "[a\n](b.md) [c](c.md)\n"))
	})
	t.Run("rows are file rows after front matter", func(t *testing.T) {
		source := []byte("---\nk: v\n---\n[a](b.md)\n")
		body, fmOffset := bodyAndFMOffset(source)
		lf, err := lint.NewFile("a.md", body)
		require.NoError(t, err)
		loc := destLocator{lf: lf, fileLines: splitLines(source), fmOffset: fmOffset}
		_ = ast.Walk(lf.AST, loc.visit)
		require.Len(t, loc.dests, 1)
		d := loc.dests[0]
		assert.Equal(t, 3, d.line)
		assert.Equal(t, "b.md", string(d.row[d.ps:d.ps+len(d.dest)]))
		assert.Equal(t, "b.md", string(d.dest))
	})
}

func TestDestLocator_Advance(t *testing.T) {
	d := destLocator{cursor: 3}
	d.advance(false, 9)
	assert.Equal(t, 3, d.cursor, "leaving a node keeps the cursor")
	d.advance(true, 9)
	assert.Equal(t, 9, d.cursor)
}

func TestDestLocator_LinkNode(t *testing.T) {
	d := destLocator{}
	d.linkNode(true, 2, nil, true)
	assert.Equal(t, 2, d.cursor, "entering moves the cursor to the opening byte")
	d.linkNode(false, 2, []byte("r.md"), false)
	assert.Empty(t, d.dests, "a reference-style node is not located")
	assert.Equal(t, 2, d.cursor)
}

func TestOutboundEdit(t *testing.T) {
	// located builds the inlineDest for the single destination on row.
	located := func(row, dest string) inlineDest {
		ps := strings.Index(row, dest)
		require.GreaterOrEqual(t, ps, 0)
		return inlineDest{dest: []byte(dest), row: []byte(row), line: 4, ps: ps}
	}
	r := &destResolver{ws: stubWorkspace{}, src: "docs/a.md"}
	t.Run("relative path is re-spelled from dst", func(t *testing.T) {
		e, ok := outboundEdit(r, located("![](./b.png)", "./b.png"), "docs/a.md", "guide/x/a.md")
		require.True(t, ok)
		assert.Equal(t, "../../docs/b.png", e.NewText)
		assert.Equal(t, 4, e.Range.Start.Line)
		assert.Equal(t, 4, e.Range.Start.Character)
		assert.Equal(t, 11, e.Range.End.Character)
	})
	for name, tc := range map[string]struct{ row, dest string }{
		"external":         {"[t](https://x.io/a.md)", "https://x.io/a.md"},
		"same-file anchor": {"[t](#intro)", "#intro"},
		"root-anchored":    {"[t](/a.md)", "/a.md"},
	} {
		t.Run(name+" is left alone", func(t *testing.T) {
			_, ok := outboundEdit(r, located(tc.row, tc.dest), "docs/a.md", "guide/a.md")
			assert.False(t, ok)
		})
	}
	t.Run("an escape is decoded and written back escaped", func(t *testing.T) {
		e, ok := outboundEdit(r, located("[t](b%20c.md?q#f)", "b%20c.md?q#f"), "docs/a.md", "guide/a.md")
		require.True(t, ok)
		assert.Equal(t, "../docs/b%20c.md", e.NewText)
		assert.Equal(t, 12, e.Range.End.Character, "the query and fragment stay outside the token")
	})
	t.Run("self link follows the file", func(t *testing.T) {
		e, ok := outboundEdit(r, located("[t](a.md)", "a.md"), "docs/a.md", "guide/b.md")
		require.True(t, ok)
		assert.Equal(t, "b.md", e.NewText)
	})
}

func TestWikilinkStemBytes(t *testing.T) {
	t.Run("not a wikilink returns false", func(t *testing.T) {
		_, _, ok := wikilinkStemBytes([]byte("[x](y)"), 0)
		assert.False(t, ok)
	})
	t.Run("out-of-range bracket start returns false", func(t *testing.T) {
		_, _, ok := wikilinkStemBytes([]byte("[["), 0)
		assert.False(t, ok)
	})
	t.Run("empty stem returns false", func(t *testing.T) {
		_, _, ok := wikilinkStemBytes([]byte("[[#frag]]"), 0)
		assert.False(t, ok)
	})
	t.Run("folder prefix narrows to the basename stem", func(t *testing.T) {
		row := []byte("[[folder/Page#f|alias]]")
		s, e, ok := wikilinkStemBytes(row, 0)
		require.True(t, ok)
		assert.Equal(t, "Page", string(row[s:e]))
	})
}

// TestMove_SameDirOutboundIsNoOp covers destEdit's no-op branch: moving
// a file within its own directory leaves an outbound `./c.md` link
// unchanged, so no edit is emitted for it.
func TestMove_SameDirOutboundIsNoOp(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"docs/a.md": "# A\n\nSee [c](./c.md) and [self](#a).\n",
		"docs/c.md": "# C\n",
	})
	plan, err := Move(ws, "docs/a.md", "docs/b.md")
	require.NoError(t, err)
	// The ./c.md link resolves identically from docs/b.md, and the
	// same-file anchor carries no path, so neither is rewritten.
	assert.Empty(t, plan.Edits["docs/a.md"])
	require.NotNil(t, plan.FileOp)
}

func TestAppendReferrerEdits_DefensiveBranches(t *testing.T) {
	changes := map[string][]Edit{}
	ws := stubWorkspace{
		files: []string{"a.md", "gone.md", "c.md", "prose.md"},
		sources: map[string][]byte{
			// src itself is left to the outbound pass.
			"a.md": []byte("[self](a.md)\n"),
			// A link to another file.
			"c.md": []byte("[o](other.md) [p](100%25.md)\n"),
			// The name with no `](` or `]:` to open a destination.
			"prose.md": []byte("See a.md.\n"),
		},
		unresolvable: map[string]bool{"gone.md": true},
	}
	r := &destResolver{ws: ws, src: "a.md"}
	appendReferrerEdits(changes, ws, lint.NewParser(), r, "a.md", "docs/a.md")
	assert.Empty(t, changes, "every file hits a skip branch")
}

func TestMayName(t *testing.T) {
	base := []byte("a.md")
	for name, tc := range map[string]struct {
		source string
		want   bool
	}{
		"inline link":              {"[x](a.md)", true},
		"ref-def":                  {"[r]: ./a.md", true},
		"escaped name":             {"[x](%61.md)", true},
		"link to another file":     {"[x](b.md)", false},
		"name with no link mark":   {"See a.md.", false},
		"escape with no link mark": {"100% a.md", false},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, mayName([]byte(tc.source), base))
		})
	}
}

func TestAppendWikilinkStemEdits_DefensiveBranches(t *testing.T) {
	changes := map[string][]Edit{}
	ws := stubWorkspace{
		wikilinkEdges: []index.Edge{
			{Kind: index.EdgeWikilink, SourceFile: "gone.md", TargetLabel: "api", SourceLine: 1, SourceCol: 1},
			{Kind: index.EdgeWikilink, SourceFile: "b.md", TargetLabel: "api", SourceLine: 999, SourceCol: 1},
			{Kind: index.EdgeWikilink, SourceFile: "c.md", TargetLabel: "api", SourceLine: 1, SourceCol: 3},
		},
		sources: map[string][]byte{
			"b.md": []byte("short\n"),
			"c.md": []byte("no wikilink\n"),
		},
		unresolvable: map[string]bool{"gone.md": true},
	}
	// Basename changes (api -> service) so the pass runs, but every edge
	// hits a skip branch.
	appendWikilinkStemEdits(changes, ws, "api.md", "service.md")
	assert.Empty(t, changes)
}

func TestAppendReferrerEdits_RefDefShapesAndSkips(t *testing.T) {
	changes := map[string][]Edit{}
	ws := stubWorkspace{
		files: []string{"gone.md", "local.md", "other.md", "hit.md"},
		sources: map[string][]byte{
			// A local-anchor ref-def has no path portion to rewrite.
			"local.md": []byte("[l]: #frag\n"),
			// A ref-def pointing elsewhere is skipped.
			"other.md": []byte("[o]: other-target.md\n"),
			// A ref-def pointing at src is rewritten, with no index edge.
			"hit.md": []byte("[h]: a.md\n"),
		},
		unresolvable: map[string]bool{"gone.md": true},
	}
	r := &destResolver{ws: ws, src: "a.md"}
	appendReferrerEdits(changes, ws, lint.NewParser(), r, "a.md", "docs/a.md")
	require.Len(t, changes["hit.md"], 1)
	assert.Equal(t, "docs/a.md", changes["hit.md"][0].NewText)
	assert.NotContains(t, changes, "local.md")
	assert.NotContains(t, changes, "other.md")
}

// outbound runs appendOutboundEdits for a move of src (keyed as src)
// to dst over a workspace holding only src.
func outbound(changes map[string][]Edit, src, dst string, source []byte) {
	ws := stubWorkspace{files: []string{src}}
	r := &destResolver{ws: ws, src: src}
	appendOutboundEdits(changes, lint.NewParser(), r, src, src, dst, source)
}

func TestAppendOutboundEdits_SkipsAnchorAndNonWorkspace(t *testing.T) {
	changes := map[string][]Edit{}
	// [self] is a same-file anchor (no path); [abs] is root-anchored and
	// resolves outside the workspace; only [b] is rewritten.
	src := []byte("# A\n\n[self](#a) [abs](/x.md) [b](./b.md)\n")
	outbound(changes, "docs/a.md", "guide/a.md", src)
	require.Len(t, changes["docs/a.md"], 1)
	assert.Equal(t, "../docs/b.md", changes["docs/a.md"][0].NewText)
}

func TestAppendOutboundEdits_EmptyTextLinkLocated(t *testing.T) {
	changes := map[string][]Edit{}
	// An empty-text link `[](./b.md)` has no text node to anchor on; the
	// locator starts from its opening `[` and still rewrites it.
	src := []byte("# A\n\n[](./b.md)\n")
	outbound(changes, "a.md", "docs/a.md", src)
	require.Len(t, changes["a.md"], 1)
	assert.Equal(t, "../b.md", changes["a.md"][0].NewText)
	assert.Equal(t, 2, changes["a.md"][0].Range.Start.Line)
}

func TestAppendOutboundEdits_SameDirIsNoOp(t *testing.T) {
	changes := map[string][]Edit{}
	// ./b.md resolves identically from docs/, so a within-directory move
	// recomputes it to the same token and emits no edit.
	outbound(changes, "docs/a.md", "docs/moved.md",
		[]byte("[b](./b.md)\n"))
	assert.Empty(t, changes["docs/a.md"])
}

// TestAppendOutboundEdits_SelfPathLinkStaysValid covers a path link inside
// the moved file that addresses the file itself. After relocating into a
// deeper directory the link must stay a same-directory self-reference, not
// be rewritten to `../a.md` (which would point at the vacated old path).
func TestAppendOutboundEdits_SelfPathLinkStaysValid(t *testing.T) {
	changes := map[string][]Edit{}
	// docs/a.md links to itself by path; moved to docs/sub/a.md the link
	// resolves identically from the new directory, so no edit is emitted.
	outbound(changes, "docs/a.md", "docs/sub/a.md",
		[]byte("# A\n\nJump [self](a.md#intro).\n"))
	assert.Empty(t, changes["docs/a.md"])
}

// TestMove_SelfPathLinkStaysValid exercises the same self-link case end to
// end through Move: relocating a file that links to itself by path leaves
// that link untouched rather than breaking it.
func TestMove_SelfPathLinkStaysValid(t *testing.T) {
	ws := newMemWorkspace(map[string]string{
		"docs/a.md": "# A\n\nJump [self](a.md#intro).\n",
	})
	plan, err := Move(ws, "docs/a.md", "docs/sub/a.md")
	require.NoError(t, err)
	assert.Empty(t, plan.Edits["docs/a.md"])
	require.NotNil(t, plan.FileOp)
}

// TestMove_UnlistedSourceCountsTowardStemAmbiguity locks that a moved
// Markdown file absent from ws.Files() (excluded by a `files:` glob,
// yet still readable through Resolve) counts as a holder of its own
// stem. One listed same-stem sibling then makes `[[guide]]` ambiguous,
// so no wikilink is rewritten to the moved file's new name.
func TestMove_UnlistedSourceCountsTowardStemAmbiguity(t *testing.T) {
	ws := stubWorkspace{
		wikilinkEdges: []index.Edge{{SourceFile: "index.md", SourceLine: 1, SourceCol: 5}},
		files:         []string{"docs/guide.md", "index.md"},
		sources: map[string][]byte{
			"a/b/guide.md":  []byte("# Guide\n"),
			"docs/guide.md": []byte("# Docs guide\n"),
			"index.md":      []byte("See [[guide]].\n"),
		},
	}
	plan, err := Move(ws, "a/b/guide.md", "a/b/manual.md")
	require.NoError(t, err)
	assert.Empty(t, plan.Edits["index.md"],
		"unlisted source plus a listed sibling: [[guide]] is ambiguous")
}
