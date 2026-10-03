package refactor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRenameKind(t *testing.T) {
	for _, s := range []string{"", "heading", "label"} {
		k, err := ParseRenameKind(s)
		require.NoError(t, err, s)
		assert.Equal(t, RenameKind(s), k)
	}
	_, err := ParseRenameKind("bogus")
	var invalid InvalidRenameKindError
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, "bogus", invalid.Kind)
	assert.Contains(t, err.Error(), `"bogus"`)
}

func TestDetectRenameKind(t *testing.T) {
	tests := []struct {
		name string
		src  string
		old  string
		kind RenameKind
		line int
		err  error
	}{
		{"heading only", "# Setup\n\nSee [docs].\n\n[docs]: u\n", "Setup", KindHeading, 1, nil},
		{"heading line is reported", "Intro.\n\n## Setup\n", "Setup", KindHeading, 3, nil},
		{"label only", "# Setup\n\nSee [docs].\n\n[docs]: u\n", "docs", KindLabel, 0, nil},
		{"both match", "# docs\n\nSee [docs].\n\n[docs]: u\n", "docs", "", 0, ErrAmbiguousRename},
		{"neither match", "# Setup\n\nSee [docs].\n\n[docs]: u\n", "ghost", "", 0, ErrNoRenameTarget},
		{"label match is case-insensitive", "# Setup\n\nSee [docs].\n\n[docs]: u\n", "DOCS", KindLabel, 0, nil},
		{"fenced def is not a label", "# Setup\n\n```\n[docs]: u\n```\n", "docs", "", 0, ErrNoRenameTarget},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, line, err := detectRenameKind([]byte(tt.src), tt.old)
			assert.Equal(t, tt.kind, kind)
			assert.Equal(t, tt.line, line)
			assert.ErrorIs(t, err, tt.err)
		})
	}
}

const dispatchSrc = "# Setup\n\nSee [docs].\n\n[docs]: u\n"

func newDispatchWorkspace() *memWorkspace {
	return newMemWorkspace(map[string]string{
		"a.md": dispatchSrc,
		"b.md": "See [go](a.md#setup).\n",
	})
}

func TestRename_DispatchSuccess(t *testing.T) {
	ws := newDispatchWorkspace()
	src := []byte(dispatchSrc)

	t.Run("auto heading rewrites anchors", func(t *testing.T) {
		p, err := Rename(ws, "a.md", src, "", "Setup", "Install")
		require.NoError(t, err)
		assert.Contains(t, p.Edits, "a.md")
		assert.Contains(t, p.Edits, "b.md")
	})
	t.Run("auto label", func(t *testing.T) {
		p, err := Rename(ws, "a.md", src, "", "docs", "rfc")
		require.NoError(t, err)
		assert.Len(t, p.Edits["a.md"], 2)
	})
	t.Run("explicit heading", func(t *testing.T) {
		p, err := Rename(ws, "a.md", src, KindHeading, "Setup", "Install")
		require.NoError(t, err)
		assert.Contains(t, p.Edits, "b.md")
	})
	t.Run("explicit label", func(t *testing.T) {
		p, err := Rename(ws, "a.md", src, KindLabel, "docs", "rfc")
		require.NoError(t, err)
		assert.Len(t, p.Edits["a.md"], 2)
	})
}

func TestRename_DispatchErrors(t *testing.T) {
	ws := newDispatchWorkspace()
	src := []byte(dispatchSrc)
	t.Run("ambiguous", func(t *testing.T) {
		both := []byte("# docs\n\nSee [docs].\n\n[docs]: u\n")
		_, err := Rename(ws, "a.md", both, "", "docs", "x")
		assert.ErrorIs(t, err, ErrAmbiguousRename)
	})
	t.Run("neither", func(t *testing.T) {
		_, err := Rename(ws, "a.md", src, "", "ghost", "x")
		assert.ErrorIs(t, err, ErrNoRenameTarget)
	})
	t.Run("explicit heading missing", func(t *testing.T) {
		_, err := Rename(ws, "a.md", src, KindHeading, "Ghost", "X")
		var missing MissingSymbolError
		require.ErrorAs(t, err, &missing)
		assert.Equal(t, KindHeading, missing.Kind)
		assert.Equal(t, `no heading "Ghost"`, err.Error())
	})
	t.Run("explicit label missing", func(t *testing.T) {
		_, err := Rename(ws, "a.md", src, KindLabel, "ghost", "x")
		var missing MissingSymbolError
		require.ErrorAs(t, err, &missing)
		assert.Equal(t, KindLabel, missing.Kind)
		assert.Equal(t, `no link reference "ghost"`, err.Error())
	})
	t.Run("same-name heading is nothing to rename", func(t *testing.T) {
		_, err := Rename(ws, "a.md", src, "", "Setup", "Setup")
		assert.ErrorIs(t, err, ErrNothingToRename)
	})
	t.Run("engine error passes through", func(t *testing.T) {
		two := []byte("# Setup\n\n# Other\n")
		_, err := Rename(ws, "a.md", two, KindHeading, "Setup", "Other")
		var collision HeadingCollisionError
		assert.ErrorAs(t, err, &collision)
		_, err = Rename(ws, "a.md", src, KindLabel, "docs", "bad]name")
		var invalidRune InvalidLabelRuneError
		assert.ErrorAs(t, err, &invalidRune)
	})
	t.Run("invalid kind", func(t *testing.T) {
		_, err := Rename(ws, "a.md", src, "bogus", "Setup", "X")
		var invalid InvalidRenameKindError
		assert.ErrorAs(t, err, &invalid)
	})
}
