package refactor

import (
	"path"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/linkgraph"
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
func (s stubWorkspace) WikilinkIndex() *linkgraph.WikilinkIndex {
	files := make([]string, len(s.files))
	for i, f := range s.files {
		files[i] = index.NormalizePath(f)
	}
	return holderIndex(files...)
}
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

// holderIndex builds the wikilink index over files, as the resolver
// would index a workspace holding exactly them. The test workspaces'
// WikilinkIndex methods build theirs through it too.
func holderIndex(files ...string) *linkgraph.WikilinkIndex {
	return linkgraph.NewWikilinkIndexFromPaths(files)
}

// wikilinkRewriteSafe composes the two checks appendWikilinkStemEdits
// runs for a lone move: src wins oldStem in idx, and newKey reaches
// dst in the resolver's post-move index.
func wikilinkRewriteSafe(idx *linkgraph.WikilinkIndex, src, dst, oldStem, newKey string, newIsStem bool) bool {
	t := stemTarget{dst: dst, key: newKey, isStem: newIsStem}
	return idx.StemResolvesTo(oldStem, src) && t.reaches(soloResolver(nil, src, dst).postIndex(idx))
}

func TestWikilinkRewriteSafe_OldStem(t *testing.T) {
	files := []string{"a.md", "docs/API.md", "api/api.md", "img/api.png", "notes/b.mdx", "notes/c.markdown"}
	for name, tc := range map[string]struct {
		files   []string
		stem    string
		holders bool
	}{
		"no files":                       {nil, "api", false},
		"no match":                       {files, "missing", false},
		"single match":                   {files, "a", true},
		"same stem in two directories":   {files, "api", true},
		"case-folded basename":           {[]string{"docs/API.md"}, "api", true},
		"markdown extension is stripped": {files, "c", true},
		"upper-case markdown extension":  {[]string{"docs/Guide.MD"}, "guide", true},
		"extensionless file is no stem":  {[]string{"notes/LICENSE"}, "license", false},
		"stem is not a prefix match":     {files, "ap", false},
		"typed name is no stem":          {files, "b", false},
	} {
		t.Run(name, func(t *testing.T) {
			idx := holderIndex(tc.files...)
			// The source sorts after every listed file, so any indexed
			// holder of the stem is the file the link resolves to.
			const src = "z/z/z/src.md"
			assert.Equal(t, !tc.holders, wikilinkRewriteSafe(idx, src, "z/z/z/zzz.md", tc.stem, "zzz", true))
			assert.Equal(t, !tc.holders, wikilinkRewriteSafe(idx, src, "z/z/z/dst.md", "zzz", tc.stem, true),
				"a Markdown destination reads stems the same way")
		})
	}
}

func TestWikilinkRewriteSafe_NewName(t *testing.T) {
	files := []string{"a.md", "img/api.png", "notes/b.mdx", "x/B.MDX"}
	for name, tc := range map[string]struct {
		files []string
		base  string
		safe  bool
	}{
		"no files":                     {nil, "api.png", true},
		"non-markdown keeps extension": {files, "api.png", false},
		"mdx keeps its extension":      {files, "b.mdx", false},
		"markdown name matches":        {files, "a.md", false},
		"no prefix match":              {files, "api", true},
	} {
		t.Run(name, func(t *testing.T) {
			// The destination sorts after every listed file, so any
			// holder of the name is the file the link would reach.
			idx := holderIndex(tc.files...)
			assert.Equal(t, tc.safe, wikilinkRewriteSafe(idx, "src.md", "z/z/z/"+tc.base, "zzz", tc.base, false))
		})
	}
}

// TestWikilinkRewriteSafe_DestinationResolution locks that a file
// already holding the new stem or name blocks the rewrite only when it,
// not dst, is the file the rewritten link would reach.
func TestWikilinkRewriteSafe_DestinationResolution(t *testing.T) {
	idx := holderIndex("z/x/manual.md", "z/x/logo.png")
	assert.True(t, wikilinkRewriteSafe(idx, "src.md", "a/manual.md", "src", "manual", true),
		"dst is shallower than the stem holder")
	assert.False(t, wikilinkRewriteSafe(idx, "src.md", "z/y/w/manual.md", "src", "manual", true),
		"the stem holder is shallower than dst")
	assert.True(t, wikilinkRewriteSafe(idx, "src.md", "a/logo.png", "src", "logo.png", false),
		"dst is shallower than the name holder")
	assert.False(t, wikilinkRewriteSafe(idx, "src.md", "z/y/w/logo.png", "src", "logo.png", false),
		"the name holder is shallower than dst")
}

func TestWikilinkRewriteSafe_SourceResolution(t *testing.T) {
	safe := func(idx *linkgraph.WikilinkIndex, src string) bool {
		return wikilinkRewriteSafe(idx, src, "z/z/z/manual.md", "guide", "manual", true)
	}
	files := []string{"docs/guide.md"}
	assert.False(t, safe(holderIndex(files...), "z/guide.md"),
		"an unindexed source a sibling outsorts is not the link's file")
	assert.True(t, safe(holderIndex(files...), "a/guide.md"),
		"an unindexed source that sorts first is the link's file")
	assert.True(t, safe(holderIndex("docs/guide.md", "a/guide.md"), "a/guide.md"),
		"an indexed source that sorts first is the link's file")
	assert.False(t, safe(holderIndex("docs/guide.md", "z/guide.md"), "z/guide.md"),
		"an indexed sibling that sorts first keeps the link")
	assert.True(t, safe(nil, "a/guide.md"), "a nil index holds only the source")
	// The nil-index fallback indexes r.paths(), which normalizes the
	// listing, so a source listed as `./z/guide.md` is found as itself,
	// not as a second holder that outsorts it.
	r := &destResolver{ws: stubWorkspace{files: []string{"./z/guide.md"}}}
	assert.True(t, safe(linkgraph.NewWikilinkIndexFromPaths(r.paths()), "z/guide.md"),
		"a source listed with a ./ prefix is still indexed")
}

// spellDst calls dstWikilinkSpelling the way the planner does, passing
// FileStemKey's answer for dst's basename.
func spellDst(dst string) (spelling string, needsPrefix, ok bool) {
	_, isMarkdown := linkgraph.FileStemKey(path.Base(dst))
	return dstWikilinkSpelling(dst, isMarkdown)
}

func TestDstWikilinkSpelling_NonMarkdownKeepsBase(t *testing.T) {
	for dst, want := range map[string]string{
		"docs/Service.md": "Service",
		"img/diagram.png": "diagram.png",
	} {
		got, needsPrefix, ok := spellDst(dst)
		assert.True(t, ok, dst)
		assert.False(t, needsPrefix, dst)
		assert.Equal(t, want, got, dst)
	}
}

// TestDstWikilinkSpelling_FallsBackToBase locks that the whole basename
// is written whenever the bare stem would not reach dst: a dotted stem
// reads as a typed extension, and a stem ending in a space loses it to
// the target trim. A name the resolver refuses as a drive path or trims
// bare is returned without its `./` and flagged needsPrefix.
func TestDstWikilinkSpelling_FallsBackToBase(t *testing.T) {
	for dst, want := range map[string]struct {
		spelling    string
		needsPrefix bool
	}{
		"docs/v1.3.md":     {"v1.3.md", false},
		"docs/guide.md.md": {"guide.md.md", false},
		"docs/guide .md":   {"guide .md", false},
		"docs/C:x.md":      {"C:x", true},
		"img/C:x.png":      {"C:x.png", true},
		"docs/ notes.md":   {" notes", true},
	} {
		got, needsPrefix, ok := spellDst(dst)
		assert.True(t, ok, dst)
		assert.Equal(t, want.spelling, got, dst)
		assert.Equal(t, want.needsPrefix, needsPrefix, dst)
	}
	for _, dst := range []string{"docs/.md", "docs/COPYING", "docs/guide.md ", "docs/C#.md"} {
		_, _, ok := spellDst(dst)
		assert.False(t, ok, dst)
	}
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
	r := soloResolver(stubWorkspace{}, "docs/a.md", "guide/x/a.md")
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
	r := soloResolver(ws, "a.md", "docs/a.md")
	appendReferrerEdits(changes, ws, lint.NewParser(), r)
	assert.Empty(t, changes, "every file hits a skip branch")

	// A batch with no planned member reads no file.
	counter := &resolveCounter{calls: map[string]int{}, stubWorkspace: ws}
	r = &destResolver{ws: counter, batch: &moveBatch{members: map[string]batchMember{"a.md": {dst: "b.md"}}}}
	appendReferrerEdits(changes, counter, lint.NewParser(), r)
	assert.Empty(t, changes)
	assert.Empty(t, counter.calls)
}

func TestMayNameAny(t *testing.T) {
	bases := [][]byte{[]byte("a.md"), []byte("b.md")}
	for name, tc := range map[string]struct {
		source string
		bases  [][]byte
		want   bool
	}{
		"inline link":              {"[x](a.md)", bases, true},
		"second base":              {"[x](b.md)", bases, true},
		"ref-def":                  {"[r]: ./a.md", bases, true},
		"escaped name":             {"[x](%62.md)", bases, true},
		"link to another file":     {"[x](c.md)", bases, false},
		"name with no link mark":   {"See b.md.", bases, false},
		"escape with no link mark": {"100% b.md", bases, false},
		"no bases":                 {"[x](%62.md)", nil, false},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, mayNameAny([]byte(tc.source), tc.bases))
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
	appendWikilinkStemEdits(changes, ws, soloResolver(ws, "api.md", "service.md"), "api.md", "service.md")
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
	r := soloResolver(ws, "a.md", "docs/a.md")
	appendReferrerEdits(changes, ws, lint.NewParser(), r)
	require.Len(t, changes["hit.md"], 1)
	assert.Equal(t, "docs/a.md", changes["hit.md"][0].NewText)
	assert.NotContains(t, changes, "local.md")
	assert.NotContains(t, changes, "other.md")
}

// outbound runs appendOutboundEdits for a move of src (keyed as src)
// to dst over a workspace holding only src.
func outbound(changes map[string][]Edit, src, dst string, source []byte) {
	ws := stubWorkspace{files: []string{src}}
	r := soloResolver(ws, src, dst)
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
// yet still readable through Resolve) is weighed as a holder of its
// own stem against the listed ones. The listed docs/guide.md is
// shallower, so `[[guide]]` resolves to it and no wikilink is
// rewritten to the moved file's new name.
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
		"a shallower listed sibling wins [[guide]] over the unlisted source")
}

// countingWorkspace counts Files calls on a wrapped workspace.
type countingWorkspace struct {
	*memWorkspace
	files int
}

func (w *countingWorkspace) Files() []string {
	w.files++
	return w.memWorkspace.Files()
}

// TestMove_ListsFilesOnce locks that a move reads the workspace file
// list once and shares the normalized copy between the referrer scan,
// the listed-source check, and, for a workspace with no wikilink
// index, the wikilink same-stem guard, instead of copying the list per
// pass.
func TestMove_ListsFilesOnce(t *testing.T) {
	ws := &countingWorkspace{memWorkspace: newMemWorkspace(map[string]string{
		"docs/api.md": "# API\n",
		"index.md":    "See [[api]] and [a](docs/api.md?x).\n",
	})}
	plan, err := Move(ws, "docs/api.md", "docs/service.md")
	require.NoError(t, err)
	require.NotEmpty(t, plan.Edits["index.md"])
	assert.Equal(t, 1, ws.files)

	nilIdx := &nilIndexCountingWorkspace{countingWorkspace{memWorkspace: ws.memWorkspace}}
	plan, err = Move(nilIdx, "docs/api.md", "docs/service.md")
	require.NoError(t, err)
	require.NotEmpty(t, plan.Edits["index.md"])
	assert.Equal(t, 1, nilIdx.files, "the nil-index same-stem guard reuses the list")
}

// nilIndexCountingWorkspace is a countingWorkspace with no wikilink
// index, so the move reads wikilink holders from the listed files.
type nilIndexCountingWorkspace struct{ countingWorkspace }

func (*nilIndexCountingWorkspace) WikilinkIndex() *linkgraph.WikilinkIndex { return nil }

// TestAppendWikilinkStemEdits_StaleEdgeKeyMismatch locks that an edge
// whose column now holds a link to another stem, or a typed name such
// as `[[api.png]]`, is skipped, while a link still keyed by the old
// stem in other casing or behind a folder is rewritten. `[[docs/Api /]]`
// keys as `api ` (the space before the slash stays), so it never
// reached api.md and is skipped too.
func TestAppendWikilinkStemEdits_StaleEdgeKeyMismatch(t *testing.T) {
	for row, want := range map[string]int{
		"[[other]]":      0,
		"[[api.png]]":    0,
		"[[docs/API]]":   1,
		"[[docs/Api /]]": 0,
	} {
		t.Run(row, func(t *testing.T) {
			changes := map[string][]Edit{}
			ws := stubWorkspace{
				wikilinkEdges: []index.Edge{
					{Kind: index.EdgeWikilink, SourceFile: "d.md", TargetLabel: "api", SourceLine: 1, SourceCol: 1},
				},
				files:   []string{"api.md", "d.md"},
				sources: map[string][]byte{"d.md": []byte(row + "\n")},
			}
			appendWikilinkStemEdits(changes, ws, soloResolver(ws, "api.md", "service.md"), "api.md", "service.md")
			assert.Len(t, changes["d.md"], want)
		})
	}
}

// TestWikilinkRewriteSafe_SourceInOtherCase locks that a source spelled
// in another letter case than an indexed holder, as a case-insensitive
// file system accepts, is not taken to win the stem: docs/guide.md may
// be that very file, and it sorts after b/guide.md.
func TestWikilinkRewriteSafe_SourceInOtherCase(t *testing.T) {
	idx := holderIndex("b/guide.md", "docs/guide.md")
	assert.False(t, wikilinkRewriteSafe(idx, "Docs/guide.md", "docs/manual.md", "guide", "manual", true))
}

// TestWikilinkRewriteSafe_DestinationInOtherCase locks that a
// destination an indexed file spells in another letter case is not
// taken to win its key, though the post-move index holds the
// destination too: on a case-insensitive file system docs/manual.md
// may be that very file.
func TestWikilinkRewriteSafe_DestinationInOtherCase(t *testing.T) {
	idx := holderIndex("src.md", "docs/manual.md", "img/logo.png")
	assert.False(t, wikilinkRewriteSafe(idx, "src.md", "Docs/manual.md", "src", "manual", true))
	assert.False(t, wikilinkRewriteSafe(idx, "src.md", "IMG/logo.png", "src", "logo.png", false))
	assert.True(t, wikilinkRewriteSafe(idx, "src.md", "a/manual.md", "src", "manual", true),
		"a holder that differs in more than case still sorts after a/")
}

func TestStemSiblings(t *testing.T) {
	holders := []string{"a/guide.md", "docs/guide.md", "ref/guide.md"}
	assert.Equal(t, []string{"a/guide.md", "ref/guide.md"}, stemSiblings(holders, "docs/guide.md"))
	assert.Equal(t, []string{"a/guide.md", "docs/guide.md", "ref/guide.md"}, holders, "the index's slice is left alone")
	assert.Nil(t, stemSiblings([]string{"docs/guide.md"}, "docs/guide.md"))
}

func TestWikilinkNamedSibling(t *testing.T) {
	siblings := []string{"ref/guide.md", "x/api/v1/guide.md"}
	for lead, want := range map[string]bool{
		"[[":            false,
		"[[./":          false,
		"[[ ref/":       true,
		`[[REF\`:        true,
		"[[api/v1/":     true,
		"[[pi/v1/":      false,
		"[[docs/":       false,
		"[[a/../ref/":   true,
		"[[other/":      false,
		"[[x/api/v1/":   true,
		"[[y/x/api/v1/": false,
	} {
		t.Run(lead, func(t *testing.T) {
			_, named := wikilinkNamedSibling([]byte(lead), "docs/guide.md", siblings)
			assert.Equal(t, want, named)
		})
	}
	_, named := wikilinkNamedSibling([]byte("[[ref/"), "a/ref/guide.md", siblings)
	assert.False(t, named, "a prefix that names src's folder too reaches src")
	sib, named := wikilinkNamedSibling([]byte("[[v1/"), "docs/guide.md", siblings)
	assert.True(t, named)
	assert.Equal(t, "x/api/v1/guide.md", sib)
}

func TestFolderNames(t *testing.T) {
	assert.True(t, folderNames("ref", "ref/guide.md"))
	assert.True(t, folderNames("Ref", "a/REF/guide.md"))
	assert.False(t, folderNames("ef", "ref/guide.md"))
	assert.False(t, folderNames("a/ref", "ref/guide.md"))
}
