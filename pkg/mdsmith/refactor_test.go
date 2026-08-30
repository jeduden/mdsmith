package mdsmith

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestSession_CapabilitiesIncludeRenameAndMove(t *testing.T) {
	s := newRefactorSession(t, nil)
	caps := s.Capabilities()
	assert.Contains(t, caps, "rename")
	assert.Contains(t, caps, "move")
}
