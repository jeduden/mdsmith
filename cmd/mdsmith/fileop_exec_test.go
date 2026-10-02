//go:build !wasm

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/refactor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteFileOp_MovesFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A\n"), 0o644))

	require.NoError(t, executeFileOp(refactor.FileOp{From: "a.md", To: "sub/b.md"}, dir))

	assert.NoFileExists(t, filepath.Join(dir, "a.md"))
	assert.FileExists(t, filepath.Join(dir, "sub", "b.md"))
}
