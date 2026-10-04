package main

import (
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/lint/rootfstest"
)

// assertRootsClosed fails for each recorded root a Stat still reads.
func assertRootsClosed(t *testing.T, opened func() []lint.RootFS) {
	t.Helper()
	got := opened()
	require.NotEmpty(t, got)
	for i, r := range got {
		_, err := fs.Stat(r, ".")
		assert.Error(t, err, "root %d is closed once the command ends", i)
	}
}

// TestRunExportClosesFileRoots locks that export closes the roots it
// opens for a file below the project root (its directory and the
// project root). Not parallel: it chdirs and records lint.OpenRootFS.
func TestRunExportClosesFileRoots(t *testing.T) {
	dir := extractUnitDir(t, "", map[string]string{"docs/a.md": "# A\n"})
	opened := rootfstest.Record(t)
	var code int
	captureStdout(func() {
		code = runExport([]string{"--no-check", "-o", filepath.Join(dir, "out.md"), "docs/a.md"})
	})
	require.Equal(t, 0, code)
	assertRootsClosed(t, opened)
}

// TestRunExtractClosesFileRoots locks that extract closes the project
// root it opens for the extracted file, as well as the roots its check
// gate opens. Not parallel: it chdirs and records lint.OpenRootFS.
func TestRunExtractClosesFileRoots(t *testing.T) {
	extractUnitDir(t, extractUnitCfg, map[string]string{"recipes/cake.md": extractUnitDoc})
	opened := rootfstest.Record(t)
	var code int
	captureStdout(func() {
		code = runExtract([]string{"recipe", "recipes/cake.md", "--format", "json"})
	})
	require.Equal(t, 0, code)
	assertRootsClosed(t, opened)
}
