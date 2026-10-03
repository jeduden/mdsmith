package mdsmith

import (
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
// failing FS, exercising buildRefactorWorkspace's walk-error branch.
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

// An explicit label that is not defined errors, matching the CLI's
// exit-1 "no link reference" outcome.
func TestSession_Rename_LabelNotFound(t *testing.T) {
	src := []byte("# T\n\nSee [docs].\n\n[docs]: u\n")
	s := newRefactorSession(t, map[string][]byte{"a.md": src})
	_, err := s.Rename("a.md", src, "label", "ghost", "x")
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
		{refactor.NothingToRenameError{Kind: refactor.KindHeading, Name: "docs"}, `nothing to rename for heading "docs"`},
		{refactor.NothingToRenameError{Kind: refactor.KindLabel, Name: "docs"}, `nothing to rename for label "docs"`},
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
	ws := s.buildRefactorWorkspace("a.md", []byte("# Buffer\n"))

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
	_, src, ok = s.buildRefactorWorkspace("", nil).Resolve("a.md")
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
	plain := s.buildRefactorWorkspace("", nil)
	assert.ElementsMatch(t, []string{"a.md", "c.md", "sub/b.md"}, plain.Files())
	// a.md has no "Other" heading on disk, but c.md already links to it.
	assert.Len(t, plain.IncomingAnchorEdges("a.md", "other"), 1)

	// With an overlay the index reads the unsaved buffer for a.md. The
	// buffer's link to b.md#b shows up only when the overlay is used.
	overlay := s.buildRefactorWorkspace("a.md", []byte("# A\n\n[b](sub/b.md#b)\n"))
	assert.Len(t, overlay.IncomingAnchorEdges("sub/b.md", "b"), 1)
	assert.Empty(t, plain.IncomingAnchorEdges("sub/b.md", "b"))
}
