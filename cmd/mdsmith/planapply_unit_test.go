package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/refactor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// planWriteIn writes orig to dir/rel and returns a planWrite that would
// replace it with data.
func planWriteIn(t *testing.T, dir, rel, orig, data string) planWrite {
	t.Helper()
	abs := filepath.Join(dir, rel)
	require.NoError(t, os.WriteFile(abs, []byte(orig), 0o644))
	return planWrite{rel: rel, abs: abs, orig: []byte(orig), data: []byte(data), edits: 1}
}

func TestComputePlanWrites(t *testing.T) {
	dir := renameWorkspace(t)
	ws, code := buildWorkspace(renameOptions{})
	require.Equal(t, -1, code)

	// Keys come back sorted; an empty entry is skipped; nothing is
	// written.
	writes, code := computePlanWrites(ws, map[string][]refactor.Edit{
		"b.md":     {lineEdit(0, 0, 3, "Read")},
		"a.md":     {lineEdit(0, 2, 7, "Install"), lineEdit(2, 0, 4, "Text")},
		"empty.md": {},
	})
	require.Equal(t, 0, code)
	require.Len(t, writes, 2)
	assert.Equal(t, "a.md", writes[0].rel)
	assert.Equal(t, 2, writes[0].edits)
	assert.Equal(t, "# Setup\n\nBody.\n", string(writes[0].orig))
	assert.Equal(t, "# Install\n\nText.\n", string(writes[0].data))
	assert.Equal(t, "b.md", writes[1].rel)
	a, err := os.ReadFile(filepath.Join(dir, "a.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Setup\n\nBody.\n", string(a), "phase one writes nothing")

	// Any failing file fails the whole phase.
	var got []planWrite
	stderr := captureStderr(func() {
		got, code = computePlanWrites(ws, map[string][]refactor.Edit{
			"a.md":       {lineEdit(0, 2, 7, "Install")},
			"missing.md": {lineEdit(0, 0, 0, "x")},
		})
	})
	assert.Equal(t, 2, code)
	assert.Nil(t, got)
	assert.Contains(t, stderr, `cannot read "missing.md" to apply edits`)
}

func TestComputePlanWrite(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "a.md")
	require.NoError(t, os.WriteFile(abs, []byte("# Setup\n"), 0o644))
	mapped := filepath.Join(dir, "mapped.md")
	require.NoError(t, os.WriteFile(mapped, []byte("# M\n"), 0o644))
	ws := cliRenameWorkspace{relToAbs: map[string]string{"m.md": mapped}, rootDir: dir}

	// A discovered file targets its discovered path.
	pw, err := computePlanWrite(ws, "m.md", []refactor.Edit{lineEdit(0, 2, 3, "N")})
	require.NoError(t, err)
	assert.Equal(t, mapped, pw.abs)
	assert.Equal(t, "# N\n", string(pw.data))

	// An undiscovered file falls back to root + rel.
	pw, err = computePlanWrite(ws, "a.md", []refactor.Edit{lineEdit(0, 2, 7, "Install")})
	require.NoError(t, err)
	assert.Equal(t, planWrite{
		rel: "a.md", abs: abs, orig: []byte("# Setup\n"), data: []byte("# Install\n"), edits: 1,
	}, pw)

	_, err = computePlanWrite(ws, "missing.md", []refactor.Edit{lineEdit(0, 0, 0, "x")})
	require.Error(t, err)

	_, err = computePlanWrite(ws, "a.md", []refactor.Edit{lineEdit(99, 0, 0, "x")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a.md: ")
}

func TestCommitPlan_WritesEveryFileThenMoves(t *testing.T) {
	dir := t.TempDir()
	writes := []planWrite{
		planWriteIn(t, dir, "a.md", "old a\n", "new a\n"),
		planWriteIn(t, dir, "b.md", "old b\n", "new b\n"),
	}
	op := &refactor.FileOp{From: "a.md", To: "sub/c.md"}
	require.Nil(t, commitPlan(dir, writes, op))
	assert.NoFileExists(t, filepath.Join(dir, "a.md"))
	got := snapshotFiles(t, dir, "sub/c.md", "b.md")
	assert.Equal(t, map[string]string{"sub/c.md": "new a\n", "b.md": "new b\n"}, got,
		"the moved file carries its rewritten body")
	assertNoTempFiles(t, dir)

	// With no file move, only the writes run.
	writes = []planWrite{planWriteIn(t, dir, "d.md", "old\n", "new\n")}
	require.Nil(t, commitPlan(dir, writes, nil))
	assert.Equal(t, map[string]string{"d.md": "new\n"}, snapshotFiles(t, dir, "d.md"))
}

func TestCommitPlan_StageFailureTouchesNothing(t *testing.T) {
	dir := t.TempDir()
	writes := []planWrite{
		planWriteIn(t, dir, "a.md", "old a\n", "new a\n"),
		// The parent directory does not exist, so staging fails.
		{rel: "gone/b.md", abs: filepath.Join(dir, "gone", "b.md"), data: []byte("x")},
	}
	f := commitPlan(dir, writes, nil)
	require.NotNil(t, f)
	assert.Contains(t, f.cause.Error(), "writing gone/b.md")
	assert.Zero(t, f.restored)
	assert.Empty(t, f.unrestored)
	assert.Equal(t, map[string]string{"a.md": "old a\n"}, snapshotFiles(t, dir, "a.md"))
	assertNoTempFiles(t, dir)
}

func TestCommitPlan_SwapFailureRestoresAndDropsTemps(t *testing.T) {
	dir := t.TempDir()
	writes := []planWrite{
		planWriteIn(t, dir, "a.md", "old a\n", "new a\n"),
		planWriteIn(t, dir, "b.md", "old b\n", "new b\n"),
		planWriteIn(t, dir, "c.md", "old c\n", "new c\n"),
	}
	injectWriteFileFn(t, &writeFileRenameFnMu, &writeFileRenameFn,
		func(from, to string) error {
			if filepath.Base(to) == "b.md" {
				return errors.New("busy")
			}
			return os.Rename(from, to)
		})
	f := commitPlan(dir, writes, nil)
	require.NotNil(t, f)
	assert.Contains(t, f.cause.Error(), "writing b.md")
	assert.Equal(t, 1, f.restored)
	assert.Empty(t, f.unrestored)
	assert.Equal(t, map[string]string{"a.md": "old a\n", "b.md": "old b\n", "c.md": "old c\n"},
		snapshotFiles(t, dir, "a.md", "b.md", "c.md"))
	assertNoTempFiles(t, dir)
}

func TestCommitPlan_MoveFailureRestoresEveryFile(t *testing.T) {
	dir := t.TempDir()
	writes := []planWrite{planWriteIn(t, dir, "a.md", "old a\n", "new a\n")}
	f := commitPlan(dir, writes, &refactor.FileOp{From: "ghost.md", To: "x.md"})
	require.NotNil(t, f)
	assert.Contains(t, f.cause.Error(), "ghost.md")
	assert.Equal(t, 1, f.restored)
	assert.Equal(t, map[string]string{"a.md": "old a\n"}, snapshotFiles(t, dir, "a.md"))
}

func TestStageWrites(t *testing.T) {
	dir := t.TempDir()
	writes := []planWrite{
		planWriteIn(t, dir, "a.md", "old a\n", "new a\n"),
		planWriteIn(t, dir, "b.md", "old b\n", "new b\n"),
	}
	staged, err := stageWrites(writes)
	require.NoError(t, err)
	require.Len(t, staged, 2)
	for i, tmp := range staged {
		assert.Equal(t, dir, filepath.Dir(tmp), "a temp is staged beside its file")
		b, err := os.ReadFile(tmp)
		require.NoError(t, err)
		assert.Equal(t, string(writes[i].data), string(b))
	}
	assert.Equal(t, map[string]string{"a.md": "old a\n", "b.md": "old b\n"},
		snapshotFiles(t, dir, "a.md", "b.md"), "staging leaves the files alone")
	removeStaged(staged)

	// A failure removes the temps already staged.
	staged, err = stageWrites([]planWrite{
		writes[0], writes[1],
		{rel: "gone/c.md", abs: filepath.Join(dir, "gone", "c.md")},
	})
	require.Error(t, err)
	assert.Nil(t, staged)
	assert.Contains(t, err.Error(), "writing gone/c.md")
	assertNoTempFiles(t, dir)
}

func TestRemoveStaged(t *testing.T) {
	dir := t.TempDir()
	tmp := filepath.Join(dir, "a.md.1.tmp")
	require.NoError(t, os.WriteFile(tmp, []byte("x"), 0o644))
	// A temp already gone is not an error.
	removeStaged([]string{tmp, filepath.Join(dir, "gone.tmp")})
	assert.NoFileExists(t, tmp)
}

func TestRollbackWrites(t *testing.T) {
	dir := t.TempDir()
	a := planWriteIn(t, dir, "a.md", "new a\n", "")
	a.orig = []byte("old a\n")
	b := planWriteIn(t, dir, "b.md", "new b\n", "")
	b.orig = []byte("old b\n")
	cause := errors.New("boom")

	f := rollbackWrites([]planWrite{a, b}, cause)
	assert.Same(t, cause, f.cause)
	assert.Equal(t, 2, f.restored)
	assert.Empty(t, f.unrestored)
	assert.Equal(t, map[string]string{"a.md": "old a\n", "b.md": "old b\n"},
		snapshotFiles(t, dir, "a.md", "b.md"))

	// A file that cannot be restored is listed with its error.
	gone := planWrite{rel: "gone/c.md", abs: filepath.Join(dir, "gone", "c.md"), orig: []byte("x")}
	f = rollbackWrites([]planWrite{a, gone}, cause)
	assert.Equal(t, 1, f.restored)
	require.Len(t, f.unrestored, 1)
	assert.Equal(t, "gone/c.md", f.unrestored[0].rel)
	require.Error(t, f.unrestored[0].err)
}

func TestCommitFailureMessage(t *testing.T) {
	cause := errors.New("writing b.md: disk full")
	assert.Equal(t, "mdsmith: writing b.md: disk full\nmdsmith: no file was changed\n",
		(&commitFailure{cause: cause}).message())

	assert.Equal(t, "mdsmith: writing b.md: disk full\n"+
		"mdsmith: restored 2 file(s) to their original content\n",
		(&commitFailure{cause: cause, restored: 2}).message())

	assert.Equal(t, "mdsmith: writing b.md: disk full\n"+
		"mdsmith: restoring a.md: denied\n"+
		"mdsmith: restoring c.md: gone\n"+
		"mdsmith: 2 file(s) keep the rewritten content: a.md, c.md\n",
		(&commitFailure{cause: cause, restored: 1, unrestored: []restoreFailure{
			{rel: "a.md", err: errors.New("denied")},
			{rel: "c.md", err: errors.New("gone")},
		}}).message())
}
