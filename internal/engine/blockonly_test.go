package engine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/engine"
	"github.com/jeduden/mdsmith/internal/rule"
)

// TestRunnerBlockOnlyParse exercises the default-off Runner.BlockOnlyParse
// flag — the lazy-parse spike's measurement seam (plan 2606141901). With
// it set, lintFile selects the block-only constructor
// (pooledFileConstructor's block-only branch) and must complete a run
// without error. The corpus harness that normally drives this path is
// inert in CI (gated on MDSMITH_SPIKE_CORPUS), so this self-contained test
// keeps the branch covered.
func TestRunnerBlockOnlyParse(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(doc,
		[]byte("# Heading\n\nA paragraph with [a](u) and `code`.\n"), 0o644))

	blockRunner := &engine.Runner{
		Config:           config.Defaults(),
		Rules:            rule.All(),
		StripFrontMatter: true,
		RootDir:          dir,
		BlockOnlyParse:   true,
	}
	res := blockRunner.Run([]string{doc})
	require.Empty(t, res.Errors)
	assert.Equal(t, 1, res.FilesChecked)
}
