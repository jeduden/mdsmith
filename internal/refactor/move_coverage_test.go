package refactor

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/index"
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

func TestDstStemSpelling_NonMarkdownKeepsBase(t *testing.T) {
	assert.Equal(t, "Service", dstStemSpelling("docs/Service.md"))
	assert.Equal(t, "diagram.png", dstStemSpelling("img/diagram.png"))
}

func TestLinkPathBytesResolving(t *testing.T) {
	t.Run("angle-bracketed dest is unwrapped", func(t *testing.T) {
		row := []byte("[t](<a.md#frag>)")
		s, e, ok := linkPathBytesResolving(row, 1, "b.md", "a.md")
		require.True(t, ok)
		assert.Equal(t, "a.md", string(row[s:e]))
	})
	t.Run("negative textStart clamps to zero", func(t *testing.T) {
		row := []byte("[t](a.md)")
		_, _, ok := linkPathBytesResolving(row, -5, "b.md", "a.md")
		assert.True(t, ok)
	})
	t.Run("no destination resolving to want returns false", func(t *testing.T) {
		row := []byte("[t](other.md)")
		_, _, ok := linkPathBytesResolving(row, 1, "b.md", "a.md")
		assert.False(t, ok)
	})
	t.Run("image-in-link skips the inner image and rewrites the outer", func(t *testing.T) {
		row := []byte("[![alt](img.png)](a.md)")
		s, e, ok := linkPathBytesResolving(row, 1, "b.md", "a.md")
		require.True(t, ok)
		assert.Equal(t, "a.md", string(row[s:e]))
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

// TestMove_SameDirOutboundIsNoOp covers pathEdit's no-op branch: moving
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

func TestAppendIncomingPathEdits_DefensiveBranches(t *testing.T) {
	changes := map[string][]Edit{}
	ws := stubWorkspace{
		pathEdges: []index.Edge{
			// Non-file-link kind is skipped.
			{Kind: index.EdgeInclude, SourceFile: "inc.md", TargetFile: "a.md", SourceLine: 1, SourceCol: 1},
			// A self-referencing edge in the moved file is left to the
			// outbound pass.
			{Kind: index.EdgeFileLink, SourceFile: "a.md", TargetFile: "a.md", SourceLine: 1, SourceCol: 1},
			// Source file cannot be resolved.
			{Kind: index.EdgeFileLink, SourceFile: "gone.md", TargetFile: "a.md", SourceLine: 1, SourceCol: 1},
			// SourceLine past EOF (stale index).
			{Kind: index.EdgeFileLink, SourceFile: "b.md", TargetFile: "a.md", SourceLine: 999, SourceCol: 1},
			// A row whose link cannot be located for want.
			{Kind: index.EdgeFileLink, SourceFile: "c.md", TargetFile: "a.md", SourceLine: 1, SourceCol: 3},
		},
		sources: map[string][]byte{
			"b.md": []byte("short\n"),
			"c.md": []byte("no link here\n"),
		},
		unresolvable: map[string]bool{"gone.md": true},
	}
	appendIncomingPathEdits(changes, ws, "a.md", "docs/a.md")
	assert.Empty(t, changes, "every edge hits a skip branch")
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

func TestAppendRefDefPathEdits_ShapesAndSkips(t *testing.T) {
	changes := map[string][]Edit{}
	ws := stubWorkspace{
		files: []string{"gone.md", "local.md", "other.md", "hit.md"},
		sources: map[string][]byte{
			// A local-anchor ref-def has no path portion to rewrite.
			"local.md": []byte("[l]: #frag\n"),
			// A ref-def pointing elsewhere is skipped.
			"other.md": []byte("[o]: other-target.md\n"),
			// A ref-def pointing at src is rewritten.
			"hit.md": []byte("[h]: a.md\n"),
		},
		unresolvable: map[string]bool{"gone.md": true},
	}
	appendRefDefPathEdits(changes, ws, "a.md", "docs/a.md")
	require.Len(t, changes["hit.md"], 1)
	assert.Equal(t, "docs/a.md", changes["hit.md"][0].NewText)
	assert.NotContains(t, changes, "local.md")
	assert.NotContains(t, changes, "other.md")
}

func TestAppendOutboundEdits_SkipsAnchorAndNonWorkspace(t *testing.T) {
	changes := map[string][]Edit{}
	// [self] is a same-file anchor (no path); [abs] is root-anchored and
	// resolves outside the workspace; only [b] is rewritten.
	src := []byte("# A\n\n[self](#a) [abs](/x.md) [b](./b.md)\n")
	appendOutboundEdits(changes, "docs/a.md", "docs/a.md", "guide/a.md", src)
	require.Len(t, changes["docs/a.md"], 1)
	assert.Equal(t, "../docs/b.md", changes["docs/a.md"][0].NewText)
}

func TestAppendOutboundEdits_UnlocatableLinkSkipped(t *testing.T) {
	changes := map[string][]Edit{}
	// An empty-text link `[](./b.md)` has no text node, so its reported
	// position collapses to (1,1); on any line but the first the byte
	// scan can't find it, and it is skipped rather than mis-edited.
	src := []byte("# A\n\n[](./b.md)\n")
	appendOutboundEdits(changes, "a.md", "a.md", "docs/a.md", src)
	assert.Empty(t, changes["a.md"])
}

func TestAppendOutboundEdits_SameDirIsNoOp(t *testing.T) {
	changes := map[string][]Edit{}
	// ./b.md resolves identically from docs/, so a within-directory move
	// recomputes it to the same token and emits no edit.
	appendOutboundEdits(changes, "docs/a.md", "docs/a.md", "docs/moved.md",
		[]byte("[b](./b.md)\n"))
	assert.Empty(t, changes["docs/a.md"])
}

func TestRefDefPathEditForMatch_DirectBranches(t *testing.T) {
	m := []int{0, 3, 1, 2} // label bracket at body offsets 1..2

	t.Run("no colon returns false", func(t *testing.T) {
		body := []byte("[a] not a def\n")
		_, ok := refDefPathEditForMatch(body, splitLines(body), 0, m, "x.md", "a.md", "b.md")
		assert.False(t, ok)
	})
	t.Run("empty destination returns false", func(t *testing.T) {
		body := []byte("[a]:   \n")
		_, ok := refDefPathEditForMatch(body, splitLines(body), 0, m, "x.md", "a.md", "b.md")
		assert.False(t, ok)
	})
	t.Run("fileLine past EOF returns false", func(t *testing.T) {
		body := []byte("[a]: a.md\n")
		_, ok := refDefPathEditForMatch(body, splitLines(body), 999, m, "x.md", "a.md", "b.md")
		assert.False(t, ok)
	})
	t.Run("fragment in the dest path is preserved", func(t *testing.T) {
		body := []byte("[a]: a.md#frag\n")
		edit, ok := refDefPathEditForMatch(body, splitLines(body), 0, m, "x.md", "a.md", "docs/a.md")
		require.True(t, ok)
		assert.Equal(t, "docs/a.md", edit.NewText)
	})
	t.Run("dest pointing elsewhere returns false", func(t *testing.T) {
		body := []byte("[a]: other.md\n")
		_, ok := refDefPathEditForMatch(body, splitLines(body), 0, m, "x.md", "a.md", "docs/a.md")
		assert.False(t, ok)
	})
}
