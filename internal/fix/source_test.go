package fix_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/config"
	fixpkg "github.com/jeduden/mdsmith/internal/fix"
	"github.com/jeduden/mdsmith/internal/rule"

	// Register the rules we exercise in these tests.
	_ "github.com/jeduden/mdsmith/internal/rules/notrailingspaces"
	_ "github.com/jeduden/mdsmith/internal/rules/singletrailingnewline"
)

// TestFixSourceMatchesFixerOnDisk pins the LSP-side guarantee that
// FixSource returns the same bytes the on-disk Fixer would write for
// the same content. The matching pair is the acceptance criterion
// behind `source.fixAll.mdsmith` in plan 121.
func TestFixSourceMatchesFixerOnDisk(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.md")
	original := []byte("# Hi\n\ndirty line   \nanother dirty   \n")
	require.NoError(t, os.WriteFile(path, original, 0o644))

	cfg := config.Merge(config.Defaults(), nil)
	fixer := &fixpkg.Fixer{
		Config:           cfg,
		Rules:            rule.All(),
		StripFrontMatter: true,
	}
	_ = fixer.Fix([]string{path})
	onDisk, err := os.ReadFile(path)
	require.NoError(t, err)

	inMem, err := fixpkg.Source(fixpkg.SourceOptions{
		Config:           cfg,
		Rules:            rule.All(),
		Path:             path,
		Source:           original,
		StripFrontMatter: true,
	})
	require.NoError(t, err)
	assert.Equal(t, string(onDisk), string(inMem))
}

func TestFixSourceWithRulesEmptyNamesNoOp(t *testing.T) {
	t.Parallel()
	original := []byte("# Hi\n\ndirty   \n")
	out, err := fixpkg.SourceWithRules(fixpkg.SourceOptions{
		Config:           config.Merge(config.Defaults(), nil),
		Rules:            rule.All(),
		Path:             "buf.md",
		Source:           original,
		StripFrontMatter: true,
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, string(original), string(out))
}

func TestFixSourceWithRulesAppliesOnlyNamed(t *testing.T) {
	t.Parallel()
	// Two distinct issues: trailing spaces (no-trailing-spaces) and a
	// missing terminal newline (single-trailing-newline). When we ask
	// only for no-trailing-spaces, single-trailing-newline must not run.
	original := []byte("# Hi\n\ndirty   ")
	out, err := fixpkg.SourceWithRules(fixpkg.SourceOptions{
		Config:           config.Merge(config.Defaults(), nil),
		Rules:            rule.All(),
		Path:             "buf.md",
		Source:           original,
		StripFrontMatter: true,
	}, []string{"no-trailing-spaces"})
	require.NoError(t, err)
	assert.Equal(t, "# Hi\n\ndirty", string(out))
}
