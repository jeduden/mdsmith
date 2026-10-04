package mdsmith

import (
	"errors"
	"fmt"
	"io/fs"
	"testing"

	"github.com/jeduden/mdsmith/internal/refactor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failFS is an fs.FS whose root cannot be opened, so fs.WalkDir invokes
// the callback with a non-nil error.
type failFS struct{}

func (failFS) Open(string) (fs.File, error) { return nil, fs.ErrPermission }

// failFSWorkspace reads files normally but hands the refactor walk a
// failing FS, exercising indexRefactorWorkspace's walk-error branch.
type failFSWorkspace struct{ *MemWorkspace }

func (failFSWorkspace) FS() fs.FS { return failFS{} }

func newRefactorSession(t *testing.T, files map[string][]byte) *Session {
	t.Helper()
	s, err := NewSession(SessionOptions{
		Workspace: NewMemWorkspace(files),
		Config:    ConfigYAML(""),
	})
	require.NoError(t, err)
	t.Cleanup(s.Dispose)
	return s
}

func TestSession_Rename_HeadingAutoDetect(t *testing.T) {
	aSrc := []byte("# Setup\n\nBody.\n")
	s := newRefactorSession(t, map[string][]byte{
		"a.md": aSrc,
		"b.md": []byte("See [go](a.md#setup).\n"),
	})
	// as="" auto-detects the heading "Setup".
	plan, err := s.Rename("a.md", aSrc, "", "Setup", "Install")
	require.NoError(t, err)
	assert.Nil(t, plan.Move)
	// a.md heading edit + b.md anchor edit.
	require.Len(t, plan.Edits["a.md"], 1)
	assert.Equal(t, "Install", plan.Edits["a.md"][0].NewText)
	require.Len(t, plan.Edits["b.md"], 1)
	assert.Equal(t, "install", plan.Edits["b.md"][0].NewText)
}

func TestSession_Rename_LabelExplicit(t *testing.T) {
	bSrc := []byte("# B\n\nSee [the docs][docs].\n\n[docs]: https://x.example\n")
	s := newRefactorSession(t, map[string][]byte{"b.md": bSrc})
	plan, err := s.Rename("b.md", bSrc, "label", "docs", "rfc")
	require.NoError(t, err)
	assert.Len(t, plan.Edits["b.md"], 2)
}

func TestSession_Rename_AmbiguousErrors(t *testing.T) {
	dSrc := []byte("# Spec\n\nSee [Spec].\n\n[Spec]: https://x.example\n")
	s := newRefactorSession(t, map[string][]byte{"d.md": dSrc})
	_, err := s.Rename("d.md", dSrc, "", "Spec", "Rfc")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "both")
}

func TestSession_Move_RewritesAndDescribesMove(t *testing.T) {
	s := newRefactorSession(t, map[string][]byte{
		"a.md": []byte("# A\n"),
		"b.md": []byte("See [a](a.md#intro).\n"),
	})
	plan, err := s.Move("a.md", "docs/a.md")
	require.NoError(t, err)
	require.NotNil(t, plan.Move)
	assert.Equal(t, "a.md", plan.Move.From)
	assert.Equal(t, "docs/a.md", plan.Move.To)
	require.Len(t, plan.Edits["b.md"], 1)
	assert.Equal(t, "docs/a.md", plan.Edits["b.md"][0].NewText)
}

func TestSession_Move_DestinationExistsErrors(t *testing.T) {
	s := newRefactorSession(t, map[string][]byte{
		"a.md": []byte("# A\n"),
		"b.md": []byte("# B\n"),
	})
	_, err := s.Move("a.md", "b.md")
	require.Error(t, err)
}

func TestSession_Rename_InvalidAs(t *testing.T) {
	src := []byte("# Setup\n")
	s := newRefactorSession(t, map[string][]byte{"a.md": src})
	_, err := s.Rename("a.md", src, "bogus", "Setup", "Install")
	require.Error(t, err)
	assert.Equal(t, `rename: as must be "heading" or "label", got "bogus"`, err.Error())
}

func TestSession_Rename_HeadingNotFound(t *testing.T) {
	src := []byte("# Setup\n")
	s := newRefactorSession(t, map[string][]byte{"a.md": src})
	_, err := s.Rename("a.md", src, "heading", "Ghost", "X")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no heading")
}

func TestSession_Rename_AutoDetectNeither(t *testing.T) {
	src := []byte("# Setup\n\nPlain prose with no matching symbol.\n")
	s := newRefactorSession(t, map[string][]byte{"a.md": src})
	_, err := s.Rename("a.md", src, "", "nothing-here", "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no heading or link-ref label")
}

// A heading renamed to its own text yields no edits. The CLI exits 1
// with "nothing to rename"; Session.Rename must error too rather than
// return an empty plan, so the two surfaces mirror each other.
func TestSession_Rename_SameNameHeadingErrors(t *testing.T) {
	src := []byte("# Setup\n")
	s := newRefactorSession(t, map[string][]byte{"a.md": src})
	_, err := s.Rename("a.md", src, "", "Setup", "Setup")
	require.Error(t, err)
	assert.Equal(t, `nothing to rename for heading "Setup"`, err.Error())
}

// A label renamed to its own spelling everywhere yields no edits and
// errors like the heading case.
func TestSession_Rename_SameNameLabelErrors(t *testing.T) {
	src := []byte("# T\n\nSee [docs].\n\n[docs]: u\n")
	s := newRefactorSession(t, map[string][]byte{"a.md": src})
	_, err := s.Rename("a.md", src, "label", "docs", "docs")
	require.Error(t, err)
	assert.Equal(t, `nothing to rename for label "docs"`, err.Error())
}

// A host must tell a harmless no-op rename from a real failure without
// matching message text: the error matches ErrNothingToRename and
// ErrorCode names it; any other rename error does neither.
func TestSession_Rename_NothingToRenameIsTyped(t *testing.T) {
	src := []byte("# Setup\n\nSee [docs].\n\n[docs]: u\n")
	s := newRefactorSession(t, map[string][]byte{"a.md": src})
	for _, c := range [][3]string{{"", "Setup", "Setup"}, {"label", "docs", "docs"}} {
		_, err := s.Rename("a.md", src, c[0], c[1], c[2])
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrNothingToRename, c[1])
		assert.Equal(t, ErrorCodeNothingToRename, ErrorCode(err), c[1])
	}
	_, err := s.Rename("a.md", src, "label", "ghost", "x")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNothingToRename)
	assert.Empty(t, ErrorCode(err))
}

func TestErrorCode(t *testing.T) {
	assert.Empty(t, ErrorCode(nil))
	assert.Empty(t, ErrorCode(errors.New("boom")))
	assert.Equal(t, ErrorCodeNothingToRename, ErrorCode(ErrNothingToRename))
	assert.Equal(t, ErrorCodeNothingToRename, ErrorCode(fmt.Errorf("wrapped: %w", ErrNothingToRename)))
}

// An explicit label that is not defined errors, matching the CLI's
// exit-1 "no link reference" outcome.
func TestSession_Rename_LabelNotFound(t *testing.T) {
	src := []byte("# T\n\nSee [docs].\n\n[docs]: u\n")
	s := newRefactorSession(t, map[string][]byte{"a.md": src})
	_, err := s.Rename("a.md", src, "label", "ghost", "x")
	require.Error(t, err)
	assert.Equal(t, `no link reference "ghost" in a.md`, err.Error())

	// The missing label is reported, not a collision with the existing
	// [docs] it was asked to be renamed to.
	_, err = s.Rename("a.md", src, "label", "ghost", "docs")
	require.Error(t, err)
	assert.Equal(t, `no link reference "ghost" in a.md`, err.Error())
}

func TestSession_Rename_HeadingCollisionErrors(t *testing.T) {
	src := []byte("# Alpha\n\n## Beta\n")
	s := newRefactorSession(t, map[string][]byte{"a.md": src})
	// Renaming "Alpha" to "Beta" collides with the existing Beta heading.
	_, err := s.Rename("a.md", src, "heading", "Alpha", "Beta")
	require.Error(t, err)
}

func TestSession_Rename_LabelAutoDetect(t *testing.T) {
	src := []byte("# T\n\nSee [docs].\n\n[docs]: https://x.example\n")
	s := newRefactorSession(t, map[string][]byte{"a.md": src})
	// as="" and "docs" matches only a link-ref label.
	plan, err := s.Rename("a.md", src, "", "docs", "rfc")
	require.NoError(t, err)
	assert.Len(t, plan.Edits["a.md"], 2)
}

func TestSession_Rename_LabelInvalidNewName(t *testing.T) {
	src := []byte("# T\n\nSee [docs].\n\n[docs]: u\n")
	s := newRefactorSession(t, map[string][]byte{"a.md": src})
	_, err := s.Rename("a.md", src, "label", "docs", "bad]label")
	require.Error(t, err)
}

func TestSession_Move_BasenameChangeRewritesWikilink(t *testing.T) {
	s := newRefactorSession(t, map[string][]byte{
		"api.md":   []byte("# API\n"),
		"guide.md": []byte("See [[api]].\n"),
	})
	plan, err := s.Move("api.md", "service.md")
	require.NoError(t, err)
	require.NotNil(t, plan.Move)
	require.Len(t, plan.Edits["guide.md"], 1)
	assert.Equal(t, "service", plan.Edits["guide.md"][0].NewText)
}

// TestSession_Move_UnlistedNameBlocksWikilinkRewrite locks that the
// session's move guard reads the whole workspace FS, not the Markdown
// files the edge index lists: a root logo.png, which `[[logo.png]]`
// reaches first, blocks retargeting `[[logo]]` at docs/logo.png.
func TestSession_Move_UnlistedNameBlocksWikilinkRewrite(t *testing.T) {
	s := newRefactorSession(t, map[string][]byte{
		"docs/logo.md": []byte("# Logo\n"),
		"logo.png":     []byte("png"),
		"index.md":     []byte("See [[logo]].\n"),
	})
	plan, err := s.Move("docs/logo.md", "docs/logo.png")
	require.NoError(t, err)
	assert.Empty(t, plan.Edits["index.md"])
}

// fsCountingWorkspace counts FS calls, each of which a MemWorkspace
// pays for with a copy of every file's bytes.
type fsCountingWorkspace struct {
	*MemWorkspace
	fsCalls int
}

func (w *fsCountingWorkspace) FS() fs.FS {
	w.fsCalls++
	return w.MemWorkspace.FS()
}

// TestSession_Move_TakesOneFSSnapshot locks that a move whose wikilink
// pass runs builds the edge index and the wikilink index from one FS
// snapshot.
func TestSession_Move_TakesOneFSSnapshot(t *testing.T) {
	ws := &fsCountingWorkspace{MemWorkspace: NewMemWorkspace(map[string][]byte{
		"api.md":   []byte("# API\n"),
		"guide.md": []byte("See [[api]].\n"),
	})}
	s, err := NewSession(SessionOptions{Workspace: ws, Config: ConfigYAML("")})
	require.NoError(t, err)
	t.Cleanup(s.Dispose)

	ws.fsCalls = 0
	plan, err := s.Move("api.md", "service.md")
	require.NoError(t, err)
	require.Len(t, plan.Edits["guide.md"], 1)
	assert.Equal(t, 1, ws.fsCalls)
}

func TestSession_Move_WalkErrorStillPlans(t *testing.T) {
	s, err := NewSession(SessionOptions{
		Workspace: failFSWorkspace{NewMemWorkspace(map[string][]byte{"a.md": []byte("# A\n")})},
		Config:    ConfigYAML(""),
	})
	require.NoError(t, err)
	t.Cleanup(s.Dispose)
	// The refactor walk can't stat the root, so the index is empty, but
	// the move still resolves its source through ReadFile and plans the
	// relocation.
	plan, err := s.Move("a.md", "b.md")
	require.NoError(t, err)
	require.NotNil(t, plan.Move)
}

func TestSession_CapabilitiesIncludeRenameAndMove(t *testing.T) {
	s := newRefactorSession(t, nil)
	caps := s.Capabilities()
	assert.Contains(t, caps, "rename")
	assert.Contains(t, caps, "move")
}

func TestRenameError(t *testing.T) {
	cases := []struct {
		in   error
		want string
	}{
		{
			refactor.ErrAmbiguousRename,
			`"docs" matches both a heading and a link-ref label; pass as="heading" or as="label"`,
		},
		{refactor.ErrNoRenameTarget, `no heading or link-ref label "docs"`},
		{
			refactor.NothingToRenameError{Kind: refactor.KindHeading, Name: "docs"},
			`nothing to rename for heading "docs"`,
		},
		{
			refactor.NothingToRenameError{Kind: refactor.KindLabel, Name: "docs"},
			`nothing to rename for label "docs"`,
		},
		{refactor.MissingSymbolError{Kind: refactor.KindHeading, Name: "docs"}, `no heading "docs" in a.md`},
		{refactor.MissingSymbolError{Kind: refactor.KindLabel, Name: "docs"}, `no link reference "docs" in a.md`},
		{refactor.ErrEmptyLabel, refactor.ErrEmptyLabel.Error()},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, renameError(c.in, "a.md", "docs").Error())
	}
}

func TestToRefactorPlan(t *testing.T) {
	empty := toRefactorPlan(refactor.Plan{})
	assert.NotNil(t, empty.Edits)
	assert.Empty(t, empty.Edits)
	assert.Nil(t, empty.Move)

	p := refactor.Plan{
		Edits: map[string][]refactor.Edit{
			"a.md": {{
				Range: refactor.Range{
					Start: refactor.Position{Line: 1, Character: 2},
					End:   refactor.Position{Line: 3, Character: 4},
				},
				NewText: "x",
			}},
		},
		FileOp: &refactor.FileOp{From: "a.md", To: "b.md"},
	}
	got := toRefactorPlan(p)
	require.Len(t, got.Edits["a.md"], 1)
	assert.Equal(t, TextEdit{StartLine: 1, StartChar: 2, EndLine: 3, EndChar: 4, NewText: "x"},
		got.Edits["a.md"][0])
	require.NotNil(t, got.Move)
	assert.Equal(t, FileMove{From: "a.md", To: "b.md"}, *got.Move)
}

func TestSessionRefactorWorkspace_Resolve(t *testing.T) {
	s := newRefactorSession(t, map[string][]byte{
		"a.md":     []byte("# A\n"),
		"sub/b.md": []byte("# B\n"),
	})
	ws := s.buildRefactorWorkspace("a.md", []byte("# Buffer\n"), isMovePath)

	// The overlay URI resolves to the supplied buffer, not the file.
	rel, src, ok := ws.Resolve("./a.md")
	require.True(t, ok)
	assert.Equal(t, "a.md", rel)
	assert.Equal(t, "# Buffer\n", string(src))

	// Other files read through the session workspace.
	rel, src, ok = ws.Resolve("sub/b.md")
	require.True(t, ok)
	assert.Equal(t, "sub/b.md", rel)
	assert.Equal(t, "# B\n", string(src))

	_, _, ok = ws.Resolve("missing.md")
	assert.False(t, ok)

	// Without an overlay the file's own bytes are returned.
	_, src, ok = s.buildRefactorWorkspace("", nil, isMovePath).Resolve("a.md")
	require.True(t, ok)
	assert.Equal(t, "# A\n", string(src))
}

func TestSession_BuildRefactorWorkspace(t *testing.T) {
	s := newRefactorSession(t, map[string][]byte{
		"a.md":      []byte("# A\n"),
		"c.md":      []byte("See [x](a.md#other).\n"),
		"sub/b.md":  []byte("# B\n"),
		"notes.txt": []byte("not markdown"),
	})
	plain := s.buildRefactorWorkspace("", nil, isMovePath)
	assert.ElementsMatch(t, []string{"a.md", "c.md", "sub/b.md"}, plain.Files())
	// a.md has no "Other" heading on disk, but c.md already links to it.
	assert.Len(t, plain.IncomingAnchorEdges("a.md", "other"), 1)

	// With an overlay the index reads the unsaved buffer for a.md. The
	// buffer's link to b.md#b shows up only when the overlay is used.
	overlay := s.buildRefactorWorkspace("a.md", []byte("# A\n\n[b](sub/b.md#b)\n"), isMovePath)
	assert.Len(t, overlay.IncomingAnchorEdges("sub/b.md", "b"), 1)
	assert.Empty(t, plain.IncomingAnchorEdges("sub/b.md", "b"))
}

// indexRefactorWorkspace indexes every Markdown file in the session's
// workspace, skipping other extensions, and reads the overlay buffer in
// place of the overlay file's saved bytes.
func TestSession_IndexRefactorWorkspace(t *testing.T) {
	s := newRefactorSession(t, map[string][]byte{
		"a.md":      []byte("# A\n"),
		"sub/b.md":  []byte("# B\n"),
		"notes.txt": []byte("[b](sub/b.md#b)\n"),
	})
	t.Run("indexes only Markdown files", func(t *testing.T) {
		idx := s.indexRefactorWorkspace(walkWorkspacePaths(s.ws.FS(), false, isMovePath), "", nil)
		assert.ElementsMatch(t, []string{"a.md", "sub/b.md"}, idx.Files())
		assert.Empty(t, idx.IncomingEdges("sub/b.md", "b"))
	})
	t.Run("overlay replaces the saved bytes", func(t *testing.T) {
		paths := walkWorkspacePaths(s.ws.FS(), false, isMovePath)
		idx := s.indexRefactorWorkspace(paths, "./a.md", []byte("# A\n\n[b](sub/b.md#b)\n"))
		edges := idx.IncomingEdges("sub/b.md", "b")
		require.Len(t, edges, 1)
		assert.Equal(t, "a.md", edges[0].SourceFile)
	})
}

// countingWorkspace counts ReadFile calls so a test can tell whether
// Session.Rename indexed the workspace.
type countingWorkspace struct {
	*MemWorkspace
	reads int
}

func (w *countingWorkspace) ReadFile(p string) ([]byte, error) {
	w.reads++
	return w.MemWorkspace.ReadFile(p)
}

// buildRefactorWorkspace defers its walk and index to the first edge
// or Files query, so Resolve alone reads only the file it names, and
// every later query reuses the one index.
func TestBuildRefactorWorkspace_IndexesOnFirstQuery(t *testing.T) {
	ws := &countingWorkspace{MemWorkspace: NewMemWorkspace(map[string][]byte{
		"a.md": []byte("# Setup\n"),
		"b.md": []byte("See [go](a.md#setup).\n"),
	})}
	s, err := NewSession(SessionOptions{Workspace: ws, Config: ConfigYAML("")})
	require.NoError(t, err)
	t.Cleanup(s.Dispose)

	ws.reads = 0
	w := s.buildRefactorWorkspace("", nil, isMovePath)
	assert.Zero(t, ws.reads, "building reads no workspace file")

	_, _, ok := w.Resolve("b.md")
	require.True(t, ok)
	assert.Equal(t, 1, ws.reads, "Resolve reads only the named file")

	assert.Len(t, w.IncomingAnchorEdges("a.md", "setup"), 1)
	indexed := ws.reads
	assert.Greater(t, indexed, 1, "the first edge query indexes")
	assert.ElementsMatch(t, []string{"a.md", "b.md"}, w.Files())
	assert.Equal(t, indexed, ws.reads, "later queries reuse the index")
}

// A label rename and a failed detection touch only the target's own
// bytes, so Session.Rename must not walk and index the workspace for
// them; a heading rename still does, to find incoming anchors.
func TestSession_Rename_IndexesWorkspaceOnlyForHeadings(t *testing.T) {
	src := []byte("# Setup\n\nSee [docs].\n\n[docs]: u\n")
	ws := &countingWorkspace{MemWorkspace: NewMemWorkspace(map[string][]byte{
		"a.md": src,
		"b.md": []byte("See [go](a.md#setup).\n"),
	})}
	s, err := NewSession(SessionOptions{Workspace: ws, Config: ConfigYAML("")})
	require.NoError(t, err)
	t.Cleanup(s.Dispose)

	ws.reads = 0
	_, err = s.Rename("a.md", src, "", "docs", "rfc")
	require.NoError(t, err)
	assert.Zero(t, ws.reads, "a label rename reads no workspace file")

	_, err = s.Rename("a.md", src, "", "ghost", "x")
	require.Error(t, err)
	assert.Zero(t, ws.reads, "a failed detection reads no workspace file")

	p, err := s.Rename("a.md", src, "", "Setup", "Install")
	require.NoError(t, err)
	assert.Contains(t, p.Edits, "b.md")
	assert.NotZero(t, ws.reads, "a heading rename indexes the workspace")
}

// walkCountingWorkspace hands out an FS that counts root ReadDir
// calls: fs.WalkDir reads the root once per walk.
type walkCountingWorkspace struct {
	*MemWorkspace
	walks int
}

func (w *walkCountingWorkspace) FS() fs.FS { return walkCountingFS{w.MemWorkspace.FS(), &w.walks} }

type walkCountingFS struct {
	fs.FS
	walks *int
}

func (c walkCountingFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == "." {
		*c.walks++
	}
	return fs.ReadDir(c.FS, name)
}

// TestSession_Move_WalksOnce locks that a move with a `[[stem]]` edge
// walks the workspace FS once: the edge-index walk collects every path,
// and the wikilink index is built from that list.
func TestSession_Move_WalksOnce(t *testing.T) {
	ws := &walkCountingWorkspace{MemWorkspace: NewMemWorkspace(map[string][]byte{
		"api.md":   []byte("# API\n"),
		"guide.md": []byte("See [[api]].\n"),
	})}
	s, err := NewSession(SessionOptions{Workspace: ws, Config: ConfigYAML("")})
	require.NoError(t, err)
	t.Cleanup(s.Dispose)

	ws.walks = 0
	plan, err := s.Move("api.md", "service.md")
	require.NoError(t, err)
	require.Len(t, plan.Edits["guide.md"], 1)
	assert.Equal(t, 1, ws.walks)
}

// TestWalkWorkspacePaths locks the one walk a refactor workspace makes:
// it lists every file its two readers use — every Markdown file at any
// depth for the edge index, and every other file outside `.git` and
// `node_modules` for WikilinkIndex — and an unreadable root lists none.
// A non-Markdown file under a pruned directory is read by neither, so
// the walk does not hold its path.
func TestWalkWorkspacePaths(t *testing.T) {
	ws := NewMemWorkspace(map[string][]byte{
		"a.md":                       []byte("# A\n"),
		"sub/logo.png":               []byte("png"),
		".git/HEAD":                  []byte("ref\n"),
		"node_modules/pkg/index.js":  []byte("x\n"),
		"node_modules/pkg/README.md": []byte("# R\n"),
	})
	assert.ElementsMatch(t,
		[]string{"a.md", "sub/logo.png", "node_modules/pkg/README.md"},
		walkWorkspacePaths(ws.FS(), false, isMovePath))
	assert.Empty(t, walkWorkspacePaths(failFS{}, false, isMovePath))
}

// TestWalkWorkspacePaths_ClosesOnlyOwnedFS locks that the walk closes a
// closable FS the caller owns (an OSWorkspace's fresh os.Root view) and
// leaves one it does not own open for the workspace that handed it out.
func TestWalkWorkspacePaths_ClosesOnlyOwnedFS(t *testing.T) {
	mem := NewMemWorkspace(map[string][]byte{"a.md": []byte("# A\n")})
	for _, owned := range []bool{true, false} {
		closed := 0
		got := walkWorkspacePaths(closeRecordingFS{mem.FS(), &closed}, owned, isMovePath)
		assert.Equal(t, []string{"a.md"}, got)
		want := 0
		if owned {
			want = 1
		}
		assert.Equal(t, want, closed, "owned=%v", owned)
	}
}

// TestOwnsFS locks which workspaces hand a caller of FS a view it owns:
// only an OSWorkspace, which opens a fresh os.Root per call. Any other
// workspace, the LSP overlay and a host's own included, may hand out an
// FS it keeps, so a caller must not close it.
func TestOwnsFS(t *testing.T) {
	assert.True(t, ownsFS(OSWorkspace{}))
	assert.True(t, ownsFS(&OSWorkspace{}))
	assert.False(t, ownsFS(NewMemWorkspace(nil)))
	assert.False(t, ownsFS(NewOverlayWorkspace(t.TempDir())))
	assert.False(t, ownsFS(&closeRecordingWorkspace{MemWorkspace: NewMemWorkspace(nil)}))
}

// closeRecordingWorkspace is a host workspace that hands out a closable
// FS (say one it keeps open for its lifetime) and records each Close.
type closeRecordingWorkspace struct {
	*MemWorkspace
	closed int
}

func (w *closeRecordingWorkspace) FS() fs.FS { return closeRecordingFS{w.MemWorkspace.FS(), &w.closed} }

type closeRecordingFS struct {
	fs.FS
	closed *int
}

func (c closeRecordingFS) Close() error {
	*c.closed++
	return nil
}

// TestSession_Move_LeavesHostFSOpen locks that the refactor walk does
// not close a closable FS a host workspace hands out: the workspace owns
// it and may hand it out again, so closing it would break the session's
// next Check. Only an OSWorkspace's fresh view is closed (see ownsFS).
func TestSession_Move_LeavesHostFSOpen(t *testing.T) {
	ws := &closeRecordingWorkspace{MemWorkspace: NewMemWorkspace(map[string][]byte{
		"api.md":   []byte("# API\n"),
		"guide.md": []byte("See [[api]].\n"),
	})}
	s, err := NewSession(SessionOptions{Workspace: ws, Config: ConfigYAML("")})
	require.NoError(t, err)
	t.Cleanup(s.Dispose)

	ws.closed = 0
	_, err = s.Move("api.md", "service.md")
	require.NoError(t, err)
	assert.Zero(t, ws.closed)
}

// TestBuildRefactorWorkspace_KeepsAssetsOnlyForMove locks that a symbol
// rename's walk holds only Markdown paths: a heading or label rename
// never builds a wikilink index, so an asset-heavy vault's image paths
// are dead weight there. A move's walk keeps them for WikilinkIndex.
func TestBuildRefactorWorkspace_KeepsAssetsOnlyForMove(t *testing.T) {
	s := newRefactorSession(t, map[string][]byte{
		"a.md":         []byte("# A\n"),
		"sub/logo.png": []byte("png"),
	})
	assert.Equal(t, []string{"a.md"},
		s.buildRefactorWorkspace("", nil, isMarkdownPath).paths())
	assert.ElementsMatch(t, []string{"a.md", "sub/logo.png"},
		s.buildRefactorWorkspace("", nil, isMovePath).paths())
}
