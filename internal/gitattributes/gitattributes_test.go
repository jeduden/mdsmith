package gitattributes

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractGitattributesFiles(t *testing.T) {
	content := "# header comment\n" +
		"\n" +
		"PLAN.md merge=mdsmith\n" +
		"docs/foo.md  merge=mdsmith eol=lf\n" +
		"other.md text\n" +
		"# README.md merge=mdsmith\n" +
		"loneword\n"
	got := ExtractGitattributesFiles(content)
	assert.Equal(t, []string{"PLAN.md", "docs/foo.md"}, got)
}

func TestWriteGitattributes_CreatesNewFileWithManagedBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")
	globs := Globs{Include: []string{"a.md", "b.md"}}

	err := WriteGitattributes(path, globs)
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	expected := "# BEGIN mdsmith merge-driver\n" +
		"a.md merge=mdsmith\n" +
		"b.md merge=mdsmith\n" +
		"# END mdsmith merge-driver\n"
	assert.Equal(t, expected, string(content))
}

func TestWriteGitattributes_PreservesExistingNonMdsmithEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	initial := "*.txt text eol=lf\n" +
		"*.jpg binary\n"
	err := os.WriteFile(path, []byte(initial), 0644)
	require.NoError(t, err)

	globs := Globs{Include: []string{"test.md"}}
	err = WriteGitattributes(path, globs)
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	expected := "*.txt text eol=lf\n" +
		"*.jpg binary\n" +
		"# BEGIN mdsmith merge-driver\n" +
		"test.md merge=mdsmith\n" +
		"# END mdsmith merge-driver\n"
	assert.Equal(t, expected, string(content))
}

func TestWriteGitattributes_ReplacesExistingManagedBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	initial := "*.txt text eol=lf\n" +
		"# BEGIN mdsmith merge-driver\n" +
		"old.md merge=mdsmith\n" +
		"# END mdsmith merge-driver\n" +
		"*.jpg binary\n"
	err := os.WriteFile(path, []byte(initial), 0644)
	require.NoError(t, err)

	globs := Globs{Include: []string{"new.md", "other.md"}}
	err = WriteGitattributes(path, globs)
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	expected := "*.txt text eol=lf\n" +
		"# BEGIN mdsmith merge-driver\n" +
		"new.md merge=mdsmith\n" +
		"other.md merge=mdsmith\n" +
		"# END mdsmith merge-driver\n" +
		"*.jpg binary\n"
	assert.Equal(t, expected, string(content))
}

func TestWriteGitattributes_StripsStaleMdsmithEntriesOutsideBlock(t *testing.T) {
	// Older append-only installs (or hand-edited files) may have left
	// merge=mdsmith lines outside the managed block. Those must be
	// removed so ExtractGitattributesFiles does not see stale or
	// duplicated entries.
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	initial := "*.txt text eol=lf\n" +
		"stale.md merge=mdsmith\n" +
		"# BEGIN mdsmith merge-driver\n" +
		"old.md merge=mdsmith\n" +
		"# END mdsmith merge-driver\n" +
		"trailing-stale.md merge=mdsmith\n" +
		"*.jpg binary\n"
	err := os.WriteFile(path, []byte(initial), 0644)
	require.NoError(t, err)

	err = WriteGitattributes(path, Globs{Include: []string{"new.md"}})
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	expected := "*.txt text eol=lf\n" +
		"# BEGIN mdsmith merge-driver\n" +
		"new.md merge=mdsmith\n" +
		"# END mdsmith merge-driver\n" +
		"*.jpg binary\n"
	assert.Equal(t, expected, string(content))
}

func TestWriteGitattributes_StripsStaleMdsmithEntriesWithTrailingAttributes(t *testing.T) {
	// Stale entries can carry extra attributes after merge=mdsmith
	// (e.g., `path merge=mdsmith eol=lf`). ExtractGitattributesFiles
	// treats those as managed, so the strip logic must too.
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	initial := "*.txt text eol=lf\n" +
		"stale.md merge=mdsmith eol=lf\n" +
		"# BEGIN mdsmith merge-driver\n" +
		"old.md merge=mdsmith\n" +
		"# END mdsmith merge-driver\n"
	err := os.WriteFile(path, []byte(initial), 0644)
	require.NoError(t, err)

	err = WriteGitattributes(path, Globs{Include: []string{"new.md"}})
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	expected := "*.txt text eol=lf\n" +
		"# BEGIN mdsmith merge-driver\n" +
		"new.md merge=mdsmith\n" +
		"# END mdsmith merge-driver\n"
	assert.Equal(t, expected, string(content))
}

func TestWriteGitattributes_PreservesCommentsThatMentionMdsmith(t *testing.T) {
	// Comment lines must be preserved even if they textually contain
	// `merge=mdsmith` (e.g., a documentation comment). The strip logic
	// matches ExtractGitattributesFiles, which ignores comment lines.
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	initial := "# Custom: README.md merge=mdsmith\n" +
		"*.txt text eol=lf\n" +
		"# BEGIN mdsmith merge-driver\n" +
		"old.md merge=mdsmith\n" +
		"# END mdsmith merge-driver\n"
	err := os.WriteFile(path, []byte(initial), 0644)
	require.NoError(t, err)

	err = WriteGitattributes(path, Globs{Include: []string{"new.md"}})
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	expected := "# Custom: README.md merge=mdsmith\n" +
		"*.txt text eol=lf\n" +
		"# BEGIN mdsmith merge-driver\n" +
		"new.md merge=mdsmith\n" +
		"# END mdsmith merge-driver\n"
	assert.Equal(t, expected, string(content))
}

func TestWriteGitattributes_StripsStaleMdsmithEntriesWhenNoBlockExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	initial := "*.txt text eol=lf\n" +
		"stale1.md merge=mdsmith\n" +
		"stale2.md merge=mdsmith\n"
	err := os.WriteFile(path, []byte(initial), 0644)
	require.NoError(t, err)

	err = WriteGitattributes(path, Globs{Include: []string{"new.md"}})
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	expected := "*.txt text eol=lf\n" +
		"# BEGIN mdsmith merge-driver\n" +
		"new.md merge=mdsmith\n" +
		"# END mdsmith merge-driver\n"
	assert.Equal(t, expected, string(content))
}

func TestWriteGitattributes_HandlesEmptyFileList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	err := WriteGitattributes(path, Globs{})
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	expected := "# BEGIN mdsmith merge-driver\n" +
		"# END mdsmith merge-driver\n"
	assert.Equal(t, expected, string(content))
}

func TestWriteGitattributes_AppendsBlockWhenNoNewlineAtEOF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	initial := "*.txt text eol=lf"
	err := os.WriteFile(path, []byte(initial), 0644)
	require.NoError(t, err)

	err = WriteGitattributes(path, Globs{Include: []string{"test.md"}})
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	expected := "*.txt text eol=lf\n" +
		"# BEGIN mdsmith merge-driver\n" +
		"test.md merge=mdsmith\n" +
		"# END mdsmith merge-driver\n"
	assert.Equal(t, expected, string(content))
}

func TestWriteGitattributes_HandlesEndMarkerWithoutTrailingNewline(t *testing.T) {
	// When the END marker is the last line without a final newline,
	// the rewriter must still locate the block end (len(content)
	// fallback) instead of dropping content.
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	initial := "*.txt text eol=lf\n" +
		"# BEGIN mdsmith merge-driver\n" +
		"old.md merge=mdsmith\n" +
		"# END mdsmith merge-driver"
	err := os.WriteFile(path, []byte(initial), 0644)
	require.NoError(t, err)

	err = WriteGitattributes(path, Globs{Include: []string{"new.md"}})
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	expected := "*.txt text eol=lf\n" +
		"# BEGIN mdsmith merge-driver\n" +
		"new.md merge=mdsmith\n" +
		"# END mdsmith merge-driver\n"
	assert.Equal(t, expected, string(content))
}

func TestWriteGitattributes_ReplacesTruncatedBlockMissingEndMarker(t *testing.T) {
	// A partial edit or aborted merge can leave a BEGIN marker without
	// the matching END marker. The writer must treat the orphan BEGIN
	// (and everything after it) as the managed block to replace, not
	// append a second managed block that leaves the stray BEGIN line
	// behind.
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	initial := "*.txt text eol=lf\n" +
		"# BEGIN mdsmith merge-driver\n" +
		"old.md merge=mdsmith\n" +
		"# (END marker truncated by a partial edit)\n"
	require.NoError(t, os.WriteFile(path, []byte(initial), 0644))

	require.NoError(t, WriteGitattributes(path, Globs{Include: []string{"new.md"}}))

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	expected := "*.txt text eol=lf\n" +
		"# BEGIN mdsmith merge-driver\n" +
		"new.md merge=mdsmith\n" +
		"# END mdsmith merge-driver\n"
	assert.Equal(t, expected, string(content),
		"truncated block (BEGIN with no END) must be replaced wholesale, not duplicated")
}

func TestWriteGitattributes_DoesNotMatchMarkerInsideOtherComment(t *testing.T) {
	// The BEGIN/END strings must be matched as standalone trimmed
	// lines, not substrings. If a comment elsewhere mentions the
	// marker text (e.g. install instructions), the writer must still
	// treat the file as having no managed block and append a fresh
	// one rather than replacing content around the bogus match.
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	initial := "# Run `# BEGIN mdsmith merge-driver` to install\n" +
		"*.txt text eol=lf\n"
	require.NoError(t, os.WriteFile(path, []byte(initial), 0644))

	require.NoError(t, WriteGitattributes(path, Globs{Include: []string{"new.md"}}))

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	expected := "# Run `# BEGIN mdsmith merge-driver` to install\n" +
		"*.txt text eol=lf\n" +
		"# BEGIN mdsmith merge-driver\n" +
		"new.md merge=mdsmith\n" +
		"# END mdsmith merge-driver\n"
	assert.Equal(t, expected, string(content),
		"comment that contains the marker text must not be mistaken for the block start")
}

func TestStageGitattributes_AddsFileToIndex(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, exec.Command("git", "init", dir).Run())

	attrPath := filepath.Join(dir, ".gitattributes")
	require.NoError(t, os.WriteFile(attrPath, []byte("*.md merge=mdsmith\n"), 0644))

	require.NoError(t, StageGitattributes(dir))

	staged, err := exec.Command(
		"git", "-C", dir, "ls-files", "--stage", "--", ".gitattributes",
	).Output()
	require.NoError(t, err)
	assert.Contains(t, string(staged), ".gitattributes",
		"StageGitattributes must add .gitattributes to the index")
}

func TestStageGitattributes_ReturnsErrorOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	// dir is not a git repo; `git -C dir add` exits non-zero.
	err := StageGitattributes(dir)
	assert.Error(t, err)
}

// withStubGitAdd swaps the package-level git-add seam and the retry
// backoff schedule for the duration of a test, restoring both
// afterward. The schedule is shortened to a few zero-length waits so
// retry-driven tests run instantly.
func withStubGitAdd(t *testing.T, stub func(repoRoot string) ([]byte, error)) {
	t.Helper()
	origAdd := gitAddGitattributes
	origBackoff := stageRetryBackoff
	t.Cleanup(func() {
		gitAddGitattributes = origAdd
		stageRetryBackoff = origBackoff
	})
	gitAddGitattributes = stub
	// Match production's retry count (zero-duration for speed) so
	// assertions on len(stageRetryBackoff) validate the real budget.
	stageRetryBackoff = make([]time.Duration, len(origBackoff))
}

// lockExistsOutput mirrors git's real message when it cannot create
// .git/index.lock, so the lock detector is tested against the exact
// stderr the field bug produced.
const lockExistsOutput = "fatal: Unable to create '/repo/.git/index.lock': File exists.\n\n" +
	"Another git process seems to be running in this repository."

// TestStageGitattributes_RetriesTransientLock drives the
// transient-lock-clears case with a fake git: the first attempts fail
// with the index.lock message, then a later attempt succeeds. The
// staging call must retry and ultimately report success.
func TestStageGitattributes_RetriesTransientLock(t *testing.T) {
	calls := 0
	withStubGitAdd(t, func(repoRoot string) ([]byte, error) {
		calls++
		if calls < 3 {
			return []byte(lockExistsOutput), &exec.ExitError{}
		}
		return nil, nil
	})

	require.NoError(t, StageGitattributes("/repo"),
		"a lock that clears within the retry window must stage successfully")
	assert.Equal(t, 3, calls, "StageGitattributes must retry until the lock clears")
}

// TestStageGitattributes_PersistentLockFailsClearly drives the
// persistent-lock case: every attempt fails with the index.lock
// message. The call must exhaust its retries and return a clear
// "index locked" error rather than retrying forever or surfacing a
// bare exit status.
func TestStageGitattributes_PersistentLockFailsClearly(t *testing.T) {
	calls := 0
	withStubGitAdd(t, func(repoRoot string) ([]byte, error) {
		calls++
		return []byte(lockExistsOutput), &exec.ExitError{}
	})

	err := StageGitattributes("/repo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "index locked",
		"a persistent lock must surface a clear \"index locked\" message")
	assert.Equal(t, len(stageRetryBackoff)+1, calls,
		"StageGitattributes must exhaust the full retry budget before giving up")
}

// TestStageGitattributes_NeverRemovesLockItDidNotCreate proves the
// retry path waits for the lock to clear without deleting it: a
// real .git/index.lock left in place stays in place after a
// persistent-lock failure.
func TestStageGitattributes_NeverRemovesLockItDidNotCreate(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	require.NoError(t, exec.Command("git", "init", dir).Run())
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".gitattributes"), []byte("*.md merge=mdsmith\n"), 0o644))

	lockPath := filepath.Join(dir, ".git", "index.lock")
	require.NoError(t, os.WriteFile(lockPath, []byte("held by someone else"), 0o644))

	// Shorten the backoff so the real `git add` retries finish fast.
	origBackoff := stageRetryBackoff
	t.Cleanup(func() { stageRetryBackoff = origBackoff })
	stageRetryBackoff = []time.Duration{0, 0}

	err := StageGitattributes(dir)
	require.Error(t, err, "a held lock must make staging fail")
	assert.Contains(t, err.Error(), "index locked")

	data, readErr := os.ReadFile(lockPath)
	require.NoError(t, readErr, "the pre-existing lock must not be removed")
	assert.Equal(t, "held by someone else", string(data),
		"StageGitattributes must never delete a lock it did not create")
}

// TestStageGitattributes_NonLockErrorNotRetried proves a non-lock
// failure (e.g. running outside a repo) is returned immediately
// rather than burning the whole retry budget on an error that will
// never clear.
func TestStageGitattributes_NonLockErrorNotRetried(t *testing.T) {
	calls := 0
	withStubGitAdd(t, func(repoRoot string) ([]byte, error) {
		calls++
		return []byte("fatal: not a git repository"), &exec.ExitError{}
	})

	err := StageGitattributes("/repo")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "index locked")
	assert.Equal(t, 1, calls, "a non-lock error must not be retried")
}

// TestStageGitattributes_NonLockErrorEmptyOutput drives a non-lock
// failure that produces no output (e.g. git failing to exec): the
// staging call must still return a wrapped "stage .gitattributes"
// error, not misreport it as a lock or panic on the empty message.
func TestStageGitattributes_NonLockErrorEmptyOutput(t *testing.T) {
	withStubGitAdd(t, func(repoRoot string) ([]byte, error) {
		return nil, &exec.ExitError{}
	})

	err := StageGitattributes("/repo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stage .gitattributes")
	assert.NotContains(t, err.Error(), "index locked",
		"an empty-output non-lock error must not be misreported as a lock")
}

func TestDefaultIncludes(t *testing.T) {
	got := DefaultIncludes()
	assert.Equal(t, []string{"*.md", "*.markdown"}, got)
	// Each call must return a fresh slice so callers can mutate it
	// without affecting later callers.
	got[0] = "mutated"
	assert.Equal(t, []string{"*.md", "*.markdown"}, DefaultIncludes())
}

func TestSplitLastSegment(t *testing.T) {
	cases := []struct {
		in       string
		dir      string
		lastPart string
	}{
		{"demo/**", "demo/", "**"},
		{"editors/**/dist/**", "editors/**/dist/", "**"},
		{"vendor/*", "vendor/", "*"},
		{"generated", "", "generated"},
		{"**", "", "**"},
		{"a/b/c.md", "a/b/", "c.md"},
	}
	for _, tc := range cases {
		dir, last := splitLastSegment(tc.in)
		assert.Equal(t, tc.dir, dir, "dir for %q", tc.in)
		assert.Equal(t, tc.lastPart, last, "last for %q", tc.in)
	}
}

func TestScopeExcludeToMarkdown(t *testing.T) {
	exts := []string{".md", ".markdown"}
	cases := []struct {
		name    string
		pattern string
		want    []string
	}{
		{
			name:    "recursive tree",
			pattern: "demo/**",
			want:    []string{"demo/**/*.md", "demo/**/*.markdown"},
		},
		{
			name:    "recursive tree with intermediate glob",
			pattern: "internal/rules/*/bad/**",
			want: []string{
				"internal/rules/*/bad/**/*.md",
				"internal/rules/*/bad/**/*.markdown",
			},
		},
		{
			name:    "single level glob",
			pattern: "vendor/*",
			want:    []string{"vendor/*.md", "vendor/*.markdown"},
		},
		{
			// A gitignore-style trailing-slash directory must not
			// produce a `build//**/*.md` double slash.
			name:    "trailing slash directory",
			pattern: "build/",
			want:    []string{"build/**/*.md", "build/**/*.markdown"},
		},
		{
			name:    "bare recursive glob",
			pattern: "**",
			want:    []string{"**/*.md", "**/*.markdown"},
		},
		{
			name:    "already markdown md",
			pattern: "internal/rules/*/bad.md",
			want:    []string{"internal/rules/*/bad.md"},
		},
		{
			name:    "already markdown markdown",
			pattern: "notes/draft.markdown",
			want:    []string{"notes/draft.markdown"},
		},
		{
			name:    "bare directory name treated as a tree",
			pattern: "generated",
			want:    []string{"generated/**/*.md", "generated/**/*.markdown"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, scopeExcludeToMarkdown(tc.pattern, exts))
		})
	}
}

func TestScopeExcludeToMarkdown_AlwaysMarkdownScoped(t *testing.T) {
	// The safety invariant: every emitted exclude ends in a Markdown
	// extension, so a derived -merge line can never match a
	// non-Markdown file — regardless of the ignore pattern's shape.
	exts := []string{".md", ".markdown"}
	patterns := []string{
		"demo/**", "vendor/*", "src", "a/b/c", "weird/*.txt", "**", "x/",
	}
	for _, p := range patterns {
		for _, got := range scopeExcludeToMarkdown(p, exts) {
			hasMarkdownExt := false
			for _, ext := range exts {
				if strings.HasSuffix(got, ext) {
					hasMarkdownExt = true
					break
				}
			}
			assert.True(t, hasMarkdownExt,
				"exclude %q derived from ignore %q must end in a Markdown extension", got, p)
		}
	}
}

func TestGlobsFromConfig_NilConfig(t *testing.T) {
	got, skipped := GlobsFromConfig(nil)
	assert.Equal(t, DefaultIncludes(), got.Include)
	assert.Empty(t, got.Exclude)
	assert.Empty(t, skipped)
}

func TestGlobsFromConfig_TranslatesIgnore(t *testing.T) {
	cfg := &config.Config{Ignore: []string{"demo/**", "vendor/**"}}
	got, skipped := GlobsFromConfig(cfg)
	assert.Equal(t, DefaultIncludes(), got.Include)
	// Each recursive ignore tree is scoped to the Markdown include
	// extensions so the -merge lines never disable git's merge for
	// non-Markdown files in that tree (issue #750).
	assert.Equal(t, []string{
		"demo/**/*.md", "demo/**/*.markdown",
		"vendor/**/*.md", "vendor/**/*.markdown",
	}, got.Exclude)
	assert.Empty(t, skipped, "representable patterns must not be reported as skipped")
}

func TestGlobsFromConfig_KeepsMarkdownScopedIgnoreVerbatim(t *testing.T) {
	// An ignore pattern that already targets a specific Markdown
	// extension can only affect Markdown, so its -merge line is safe
	// as-is and must not be doubled or rewritten.
	cfg := &config.Config{Ignore: []string{"vendor/*.md", "notes/draft.markdown"}}
	got, skipped := GlobsFromConfig(cfg)
	assert.Equal(t, []string{"vendor/*.md", "notes/draft.markdown"}, got.Exclude)
	assert.Empty(t, skipped)
}

func TestGlobsFromConfig_MixedIgnoreShapes(t *testing.T) {
	// A mix of a recursive tree, a single-level glob, an already-
	// Markdown pattern, and a bare directory name — each scoped so
	// every emitted exclude ends in a Markdown extension.
	cfg := &config.Config{Ignore: []string{
		"demo/**",
		"vendor/*",
		"internal/rules/*/bad.md",
		"generated",
	}}
	got, _ := GlobsFromConfig(cfg)
	assert.Equal(t, []string{
		"demo/**/*.md", "demo/**/*.markdown",
		"vendor/*.md", "vendor/*.markdown",
		"internal/rules/*/bad.md",
		"generated/**/*.md", "generated/**/*.markdown",
	}, got.Exclude)
}

func TestGlobsFromConfig_IsolatesIgnoreSlice(t *testing.T) {
	// Mutating the returned Exclude slice must not corrupt the
	// config the caller passed in.
	cfg := &config.Config{Ignore: []string{"demo/**"}}
	got, _ := GlobsFromConfig(cfg)
	got.Exclude[0] = "mutated"
	assert.Equal(t, []string{"demo/**"}, cfg.Ignore)
}

func TestLoadGlobs_MissingConfig(t *testing.T) {
	dir := t.TempDir()
	got := LoadGlobs(dir)
	assert.Equal(t, DefaultIncludes(), got.Include)
	assert.Empty(t, got.Exclude)
}

func TestLoadGlobs_ReadsIgnorePatterns(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"),
		[]byte("ignore:\n  - \"demo/**\"\n  - \"vendor/**\"\n"), 0644))
	got := LoadGlobs(dir)
	assert.Equal(t, DefaultIncludes(), got.Include)
	assert.Equal(t, []string{
		"demo/**/*.md", "demo/**/*.markdown",
		"vendor/**/*.md", "vendor/**/*.markdown",
	}, got.Exclude)
}

func TestLoadGlobs_UnparseableConfigFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"),
		[]byte("not: [valid: yaml\n"), 0644))
	got := LoadGlobs(dir)
	assert.Equal(t, DefaultIncludes(), got.Include)
	assert.Empty(t, got.Exclude,
		"unparseable config must fall back to no exclusions, not error")
}

func TestRenderManagedBlock_IncludeAndExclude(t *testing.T) {
	got := RenderManagedBlock(Globs{
		Include: []string{"*.md", "*.markdown"},
		Exclude: []string{"demo/**", "vendor/*.md"},
	})
	// Excludes are emitted as `merge=text`, not `-merge`: the ignore-
	// derived paths are Markdown (well-defined text-merge semantics) and
	// carry no generated sections, so falling back to git's built-in
	// 3-way text merge avoids the binary-style conflicts `-merge` forces
	// (issue #755). Last-match-wins still turns the mdsmith driver off.
	expected := "# BEGIN mdsmith merge-driver\n" +
		"*.md merge=mdsmith\n" +
		"*.markdown merge=mdsmith\n" +
		"demo/** merge=text\n" +
		"vendor/*.md merge=text\n" +
		"# END mdsmith merge-driver\n"
	assert.Equal(t, expected, got)
}

func TestExtractGlobs_ParsesMergeTextExcludes(t *testing.T) {
	// The current render form: excludes carry `merge=text`. ExtractGlobs
	// must read them back as excludes so a freshly written block round-
	// trips and drift detection does not report perpetual drift.
	content := "# BEGIN mdsmith merge-driver\n" +
		"*.md merge=mdsmith\n" +
		"demo/** merge=text\n" +
		"vendor/*.md merge=text\n" +
		"# END mdsmith merge-driver\n"
	got, ok := ExtractGlobs(content)
	require.True(t, ok)
	assert.Equal(t, []string{"*.md"}, got.Include)
	assert.Equal(t, []string{"demo/**", "vendor/*.md"}, got.Exclude)
}

func TestExtractGlobs_ParsesLegacyDashMergeExcludes(t *testing.T) {
	// Backward compatibility: a `.gitattributes` written by an older
	// mdsmith (or by the pinned CI baseline) uses `-merge` for excludes.
	// ExtractGlobs must still read those as excludes so an unmigrated
	// committed block extracts to the same glob set as the merge=text
	// form and does not read as drift.
	content := "# BEGIN mdsmith merge-driver\n" +
		"*.md merge=mdsmith\n" +
		"demo/** -merge\n" +
		"vendor/*.md -merge\n" +
		"# END mdsmith merge-driver\n"
	got, ok := ExtractGlobs(content)
	require.True(t, ok)
	assert.Equal(t, []string{"*.md"}, got.Include)
	assert.Equal(t, []string{"demo/**", "vendor/*.md"}, got.Exclude)
}

func TestRenderManagedBlock_EmptyGlobs(t *testing.T) {
	got := RenderManagedBlock(Globs{})
	expected := "# BEGIN mdsmith merge-driver\n" +
		"# END mdsmith merge-driver\n"
	assert.Equal(t, expected, got)
}

func TestExtractGlobs_NoManagedBlock(t *testing.T) {
	got, ok := ExtractGlobs("*.txt text\n")
	assert.False(t, ok)
	assert.Empty(t, got.Include)
	assert.Empty(t, got.Exclude)
}

func TestExtractGlobs_RoundTripsRender(t *testing.T) {
	original := Globs{
		Include: []string{"*.md", "*.markdown"},
		Exclude: []string{"demo/**", "vendor/*.md"},
	}
	rendered := RenderManagedBlock(original)
	got, ok := ExtractGlobs(rendered)
	require.True(t, ok)
	assert.Equal(t, original.Include, got.Include)
	assert.Equal(t, original.Exclude, got.Exclude)
}

func TestExtractGlobs_IgnoresCommentsAndBlankLinesInBlock(t *testing.T) {
	content := "# BEGIN mdsmith merge-driver\n" +
		"\n" +
		"# inline comment inside the block\n" +
		"*.md merge=mdsmith\n" +
		"# END mdsmith merge-driver\n"
	got, ok := ExtractGlobs(content)
	require.True(t, ok)
	assert.Equal(t, []string{"*.md"}, got.Include)
	assert.Empty(t, got.Exclude)
}

func TestExtractGlobs_IgnoresUnknownAttributes(t *testing.T) {
	// A line inside the managed block that is not a merge=mdsmith,
	// merge=text, or -merge assignment must be ignored, not counted
	// as a glob.
	content := "# BEGIN mdsmith merge-driver\n" +
		"*.md merge=mdsmith\n" +
		"*.txt text\n" +
		"# END mdsmith merge-driver\n"
	got, ok := ExtractGlobs(content)
	require.True(t, ok)
	assert.Equal(t, []string{"*.md"}, got.Include)
	assert.Empty(t, got.Exclude)
}

func TestExtractGlobs_BlockWithoutTrailingNewline(t *testing.T) {
	// strings.Split on a trailing newline produces an empty last
	// element; make sure ExtractGlobs handles content that does NOT
	// end with a newline.
	content := "# BEGIN mdsmith merge-driver\n" +
		"*.md merge=mdsmith\n" +
		"# END mdsmith merge-driver"
	got, ok := ExtractGlobs(content)
	require.True(t, ok)
	assert.Equal(t, []string{"*.md"}, got.Include)
}

func TestGlobsEqual(t *testing.T) {
	a := Globs{Include: []string{"*.md"}, Exclude: []string{"demo/**"}}
	b := Globs{Include: []string{"*.md"}, Exclude: []string{"demo/**"}}
	assert.True(t, GlobsEqual(a, b))

	// Different include length.
	assert.False(t, GlobsEqual(a, Globs{Include: []string{"*.md", "*.markdown"}, Exclude: a.Exclude}))
	// Different exclude length.
	assert.False(t, GlobsEqual(a, Globs{Include: a.Include}))
	// Same length, different include order (last-match-wins makes
	// this a real behaviour change).
	assert.False(t, GlobsEqual(
		Globs{Include: []string{"a", "b"}},
		Globs{Include: []string{"b", "a"}},
	))
	// Same length, different exclude content.
	assert.False(t, GlobsEqual(
		Globs{Include: []string{"*.md"}, Exclude: []string{"demo/**"}},
		Globs{Include: []string{"*.md"}, Exclude: []string{"vendor/**"}},
	))
}

func TestHasMdsmithMergeDriver(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, exec.Command("git", "init", dir).Run())
	assert.False(t, HasMdsmithMergeDriver(dir))

	require.NoError(t, exec.Command(
		"git", "-C", dir, "config", "merge.mdsmith.driver",
		"mdsmith merge-driver run %O %A %B %P",
	).Run())
	assert.True(t, HasMdsmithMergeDriver(dir))
}

func TestIsRepresentableGitattributesPattern(t *testing.T) {
	cases := []struct {
		pattern string
		want    bool
	}{
		{"", false},
		{"*.md", true},
		{"docs/**", true},
		{"!docs/*.md", false},
		{"with space.md", false},
		{"with\ttab.md", false},
		{"with\nnewline.md", false},
		{"with\rcr.md", false},
	}
	for _, tc := range cases {
		t.Run(tc.pattern, func(t *testing.T) {
			assert.Equal(t, tc.want, isRepresentableGitattributesPattern(tc.pattern))
		})
	}
}

func TestGlobsFromConfig_DropsUnrepresentablePatterns(t *testing.T) {
	cfg := &config.Config{Ignore: []string{
		"demo/**",
		"!docs/*.md", // negation: skipped
		"with space", // whitespace: skipped
		"vendor/**",
	}}
	got, skipped := GlobsFromConfig(cfg)
	assert.Equal(t, []string{
		"demo/**/*.md", "demo/**/*.markdown",
		"vendor/**/*.md", "vendor/**/*.markdown",
	}, got.Exclude,
		"only representable patterns survive the validation filter, each scoped to Markdown")
	assert.Equal(t, []string{"!docs/*.md", "with space"}, skipped,
		"dropped patterns are returned in input order so callers can warn")
}

func TestExtractGlobs_SkipsSingleFieldLines(t *testing.T) {
	// A managed-block line with only a pattern (no attribute) is
	// not a valid merge=mdsmith, merge=text, or -merge assignment;
	// ExtractGlobs must skip it instead of treating the lone token
	// as a glob.
	content := "# BEGIN mdsmith merge-driver\n" +
		"orphan-token\n" +
		"*.md merge=mdsmith\n" +
		"# END mdsmith merge-driver\n"
	got, ok := ExtractGlobs(content)
	require.True(t, ok)
	assert.Equal(t, []string{"*.md"}, got.Include)
	assert.Empty(t, got.Exclude)
}

func TestWriteGitattributes_AtomicWriteFails_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	orig := atomicWriteFn
	t.Cleanup(func() { atomicWriteFn = orig })
	atomicWriteFn = func(string, []byte, os.FileMode) error {
		return fmt.Errorf("mock write failure")
	}

	err := WriteGitattributes(path, Globs{Include: []string{"a.md"}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mock write failure")
}

func TestAtomicWriteGitattributes_CreateTempFails_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	orig := createTempFn
	t.Cleanup(func() { createTempFn = orig })
	createTempFn = func(string, string) (*os.File, error) {
		return nil, fmt.Errorf("mock createtemp failure")
	}

	err := atomicWriteGitattributes(path, []byte("content"), 0o644)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mock createtemp failure")
}

func TestAtomicWriteGitattributes_WriteFails_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	origCreate := createTempFn
	t.Cleanup(func() { createTempFn = origCreate })
	// Return a closed file so Write fails.
	createTempFn = func(dir, pattern string) (*os.File, error) {
		f, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		_ = f.Close()
		return f, nil
	}

	err := atomicWriteGitattributes(path, []byte("content"), 0o644)
	require.Error(t, err)
}

func TestAtomicWriteGitattributes_SyncFails_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	orig := syncTempFn
	t.Cleanup(func() { syncTempFn = orig })
	syncTempFn = func(*os.File) error {
		return fmt.Errorf("mock sync failure")
	}

	err := atomicWriteGitattributes(path, []byte("content"), 0o644)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mock sync failure")
}

func TestAtomicWriteGitattributes_CloseFails_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	orig := closeTempFn
	t.Cleanup(func() { closeTempFn = orig })
	closeTempFn = func(*os.File) error {
		return fmt.Errorf("mock close failure")
	}

	err := atomicWriteGitattributes(path, []byte("content"), 0o644)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mock close failure")
}

func TestAtomicWriteGitattributes_ChmodFails_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	orig := chmodFile
	t.Cleanup(func() { chmodFile = orig })
	chmodFile = func(string, os.FileMode) error {
		return fmt.Errorf("mock chmod failure")
	}

	err := atomicWriteGitattributes(path, []byte("content"), 0o644)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mock chmod failure")
}

func TestAtomicWriteGitattributes_TargetIsDirectory_ReturnsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("not reliable on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	require.NoError(t, os.Mkdir(target, 0o755))

	err := atomicWriteGitattributes(target, []byte("content"), 0o644)
	require.Error(t, err)
}

func TestAtomicWriteGitattributes_LstatNonENOENTError_ReturnsError(t *testing.T) {
	orig := lstatFile
	t.Cleanup(func() { lstatFile = orig })
	lstatFile = func(string) (os.FileInfo, error) {
		return nil, fmt.Errorf("mock lstat failure")
	}

	err := atomicWriteGitattributes("/any/path", []byte("content"), 0o644)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mock lstat failure")
}

func TestAtomicWriteGitattributes_FstatFails_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")
	require.NoError(t, os.WriteFile(path, []byte("existing"), 0o644))

	orig := fstatFn
	t.Cleanup(func() { fstatFn = orig })
	fstatFn = func(*os.File) (os.FileInfo, error) {
		return nil, fmt.Errorf("mock fstat failure")
	}

	err := atomicWriteGitattributes(path, []byte("content"), 0o644)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mock fstat failure")
}

func TestAtomicWriteGitattributes_LstatFdMismatch_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")
	other := filepath.Join(dir, "other")
	require.NoError(t, os.WriteFile(path, []byte("existing"), 0o644))
	require.NoError(t, os.WriteFile(other, []byte("other"), 0o644))

	otherInfo, err := os.Lstat(other)
	require.NoError(t, err)

	// Inject lstatFile to return info for 'other' (different inode than path).
	orig := lstatFile
	t.Cleanup(func() { lstatFile = orig })
	lstatFile = func(string) (os.FileInfo, error) {
		return otherInfo, nil
	}

	err = atomicWriteGitattributes(path, []byte("content"), 0o644)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "changed since lstat")
}

func TestWriteGitattributes_LstatNonENOENTError_ReturnsError(t *testing.T) {
	orig := lstatFile
	t.Cleanup(func() { lstatFile = orig })
	lstatFile = func(string) (os.FileInfo, error) {
		return nil, fmt.Errorf("mock lstat failure")
	}
	err := WriteGitattributes("/any/path", Globs{Include: []string{"*.md"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lstat")
}

func TestWriteGitattributesFile_LstatNonENOENTError_ReturnsError(t *testing.T) {
	orig := lstatFile
	t.Cleanup(func() { lstatFile = orig })
	lstatFile = func(string) (os.FileInfo, error) {
		return nil, fmt.Errorf("mock lstat failure")
	}
	err := writeGitattributesFile("/any/path", "content")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lstat")
}

func TestWriteGitattributes_ReadFileFails_ReturnsWrappedError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")
	require.NoError(t, os.WriteFile(path, []byte("existing\n"), 0o644))

	orig := readFile
	t.Cleanup(func() { readFile = orig })
	readFile = func(string) ([]byte, error) {
		return nil, fmt.Errorf("mock read failure")
	}

	err := WriteGitattributes(path, Globs{Include: []string{"*.md"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading")
}

func TestWriteGitattributes_RejectsDirectory(t *testing.T) {
	// .gitattributes is a directory — the Lstat guard must reject it
	// before any read or write is attempted.
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")
	require.NoError(t, os.Mkdir(path, 0o755))

	err := WriteGitattributes(path, Globs{Include: []string{"*.md"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a regular file")
}
