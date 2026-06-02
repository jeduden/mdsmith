package fix_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/config"
	fixpkg "github.com/jeduden/mdsmith/internal/fix"
	"github.com/jeduden/mdsmith/internal/lint"
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
	res := fixer.Fix([]string{path})
	require.Empty(t, res.Errors, "Fixer.Fix reported errors: %v", res.Errors)
	require.Contains(t, res.Modified, path, "expected fixer to modify the test file")
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

// TestFixSourceWithRulesUnlimitedMaxBytes pins the new unlimited
// semantics: SourceOptions.MaxInputBytes <= 0 must NOT trigger the
// 2 MB DefaultMaxInputBytes cap — it must skip the size guard
// entirely (matching bytelimit.ReadFileLimited / cmd resolveMaxInputBytes
// when the user sets `max-input-size: 0`). We use a buffer larger
// than DefaultMaxInputBytes so a regression to the old
// "0 means default" behavior would surface as a "file too large"
// error rather than a successful fix.
func TestFixSourceWithRulesUnlimitedMaxBytes(t *testing.T) {
	t.Parallel()
	const sentinel = "# Hi\n\ndirty   \n"
	// 3 MB of filler so the input exceeds DefaultMaxInputBytes (2 MB).
	// The trailing-spaces rule still finds the violation on the
	// first body line.
	body := strings.Repeat("x", int(bytelimit.DefaultMaxInputBytes)+1024*1024)
	source := []byte(sentinel + body + "\n")
	require.Greater(t, int64(len(source)), bytelimit.DefaultMaxInputBytes,
		"source must exceed DefaultMaxInputBytes for the test to be meaningful")

	out, err := fixpkg.SourceWithRules(fixpkg.SourceOptions{
		Config:           config.Merge(config.Defaults(), nil),
		Rules:            rule.All(),
		Path:             "buf.md",
		Source:           source,
		StripFrontMatter: true,
		MaxInputBytes:    0, // explicit: 0 means unlimited
	}, []string{"no-trailing-spaces"})
	require.NoError(t, err, "MaxInputBytes=0 must propagate as unlimited; got file-too-large")
	// Trailing whitespace fixed on the first body line, and the
	// 3 MB filler survives unchanged.
	assert.True(t, strings.HasPrefix(string(out), "# Hi\n\ndirty\n"),
		"fix output must start with the cleaned sentinel; got: %q", string(out)[:64])
}

// Regression: in-memory fixes must enforce MaxInputBytes the same
// way on-disk Fixer does via bytelimit.ReadFileLimited.
func TestFixSourceRejectsOversizedSource(t *testing.T) {
	t.Parallel()
	_, err := fixpkg.Source(fixpkg.SourceOptions{
		Config:        config.Merge(config.Defaults(), nil),
		Rules:         rule.All(),
		Path:          "buf.md",
		Source:        []byte("this body is well past sixteen bytes"),
		MaxInputBytes: 16,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file too large")
}

// TestFixSourcePropagatesPrepareError pins the prepareFile error
// branch: a kind reference in front matter that does not exist in
// the config trips ValidateFrontMatterKinds and surfaces as an
// error rather than crashing.
func TestFixSourcePropagatesPrepareError(t *testing.T) {
	t.Parallel()
	cfg := config.Merge(config.Defaults(), nil)
	// Front matter references an undeclared kind, which prepareFile
	// rejects via config.ValidateFrontMatterKinds.
	src := []byte("---\nkinds: [does-not-exist]\n---\n# Hi\n")
	_, err := fixpkg.Source(fixpkg.SourceOptions{
		Config:           cfg,
		Rules:            rule.All(),
		Path:             "buf.md",
		Source:           src,
		StripFrontMatter: true,
	})
	require.Error(t, err)
}

// fsSpyRule is a fixable rule that records whether
// `lint.File.FS` was non-nil during Check. The test below uses it
// to prove that SourceOptions.SourceFS is propagated all the way
// to the lint.File rules see, not just stored on the Fixer.
type fsSpyRule struct {
	sawFS bool
}

func (*fsSpyRule) ID() string       { return "MDS999" }
func (*fsSpyRule) Name() string     { return "fs-spy" }
func (*fsSpyRule) Category() string { return "test" }
func (r *fsSpyRule) Check(f *lint.File) []lint.Diagnostic {
	if f.FS != nil {
		r.sawFS = true
	}
	return nil
}
func (*fsSpyRule) Fix(f *lint.File) []byte { return f.Source }

// TestFixSourceWiresSourceFSIntoLintFile pins that
// SourceOptions.SourceFS is honored end-to-end: the lint.File the
// fixable rule sees during Check has its FS pointed at the
// caller-supplied filesystem. The previous version of this test
// only ran no-trailing-spaces, which never reads f.FS, so a
// regression that dropped SourceFS on the floor would still pass.
func TestFixSourceWiresSourceFSIntoLintFile(t *testing.T) {
	t.Parallel()
	spy := &fsSpyRule{}
	cfg := config.Merge(config.Defaults(), nil)
	cfg.Rules[spy.Name()] = config.RuleCfg{Enabled: true}

	_, err := fixpkg.SourceWithRules(fixpkg.SourceOptions{
		Config:           cfg,
		Rules:            []rule.Rule{spy},
		Path:             "buf.md",
		Source:           []byte("# Hi\n\nbody\n"),
		RootDir:          t.TempDir(),
		SourceFS:         os.DirFS(t.TempDir()),
		StripFrontMatter: true,
	}, []string{spy.Name()})
	require.NoError(t, err)
	assert.True(t, spy.sawFS,
		"lint.File.FS must be non-nil when SourceOptions.SourceFS is set; "+
			"otherwise FS-aware rules (include, catalog) silently short-circuit")
}

// Companion test: when SourceOptions.SourceFS is left nil, the
// fix pipeline derives dirFS from filepath.Dir(Path) — which is
// still non-nil for relative paths (it's an os.DirFS rooted at
// "."). The spy rule should still see a non-nil FS, but via the
// fallback path rather than the supplied one.
func TestFixSourceFallsBackToDirFSWhenSourceFSNil(t *testing.T) {
	t.Parallel()
	spy := &fsSpyRule{}
	cfg := config.Merge(config.Defaults(), nil)
	cfg.Rules[spy.Name()] = config.RuleCfg{Enabled: true}

	_, err := fixpkg.SourceWithRules(fixpkg.SourceOptions{
		Config:           cfg,
		Rules:            []rule.Rule{spy},
		Path:             "buf.md",
		Source:           []byte("# Hi\n\nbody\n"),
		StripFrontMatter: true,
		// SourceFS: nil — exercise the dirFS fallback.
	}, []string{spy.Name()})
	require.NoError(t, err)
	assert.True(t, spy.sawFS, "dirFS fallback must still produce a non-nil FS")
}

// TestFixSourceNilConfigUsesDefaults pins the nil-Config fallback so
// callers can pass a zero-value Options without crashing
// prepareFile (which derefs Fixer.Config via
// config.ValidateFrontMatterKinds).
func TestFixSourceNilConfigUsesDefaults(t *testing.T) {
	t.Parallel()
	out, err := fixpkg.Source(fixpkg.SourceOptions{
		Rules:            rule.All(),
		Path:             "buf.md",
		Source:           []byte("# Hi\n\ndirty   \n"),
		StripFrontMatter: true,
		// Config left nil — must not panic.
	})
	require.NoError(t, err)
	assert.Equal(t, "# Hi\n\ndirty\n", string(out))
}
