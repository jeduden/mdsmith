package engine

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint/rootfstest"
	"github.com/jeduden/mdsmith/internal/rule"
)

// TestRunClosesEachFileRoots locks that Run closes every root it opens
// for a linted file once that file's check ends: the file's own
// directory (f.FS) and, for a file below the root, the separate project
// root (f.RootFS) MDS027's wikilink walk reads. The File is not
// published anywhere, so lintFile owns both. Not parallel: it records
// lint.OpenRootFS.
func TestRunClosesEachFileRoots(t *testing.T) {
	root := t.TempDir()
	host := filepath.Join(root, "host.md")
	nested := filepath.Join(root, "sub", "b.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(nested), 0o755))
	require.NoError(t, os.WriteFile(host, []byte("# Host\n"), 0o644))
	require.NoError(t, os.WriteFile(nested, []byte("# B\n"), 0o644))
	opened := rootfstest.Record(t)

	runner := &Runner{
		Config:  &config.Config{Rules: map[string]config.RuleCfg{"snap-rule": {Enabled: true}}},
		Rules:   []rule.Rule{&fileSnapRule{id: "MDS999", name: "snap-rule"}},
		RootDir: root,
	}
	require.Empty(t, runner.Run([]string{host, nested}).Errors)

	got := opened()
	require.Len(t, got, 3, "host's root, b's directory, b's project root")
	for i, r := range got {
		_, err := fs.Stat(r, ".")
		assert.Error(t, err, "root %d is closed once its file's check ends", i)
	}
}
