package githooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilesMatch(t *testing.T) {
	tests := []struct {
		name string
		a    []string
		b    []string
		want bool
	}{
		{"both nil", nil, nil, true},
		{"empty lists", []string{}, []string{}, true},
		{"same files same order", []string{"a", "b"}, []string{"a", "b"}, true},
		{"same files different order", []string{"a", "b"}, []string{"b", "a"}, true},
		{"different lengths", []string{"a"}, []string{"a", "b"}, false},
		{"different files", []string{"a", "b"}, []string{"a", "c"}, false},
		{"one empty", []string{"a"}, []string{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FilesMatch(tt.a, tt.b))
		})
	}
}

func TestExtractHookFiles_DecodesShellQuoteEscapes(t *testing.T) {
	// shellQuote encodes a literal single quote as `'\''` so the
	// filename `a'b.md` is written as `'a'\''b.md'`. The parser
	// must decode that back to the original.
	content := "mdsmith fix -- 'a'\\''b.md'\n" +
		"git add -- 'a'\\''b.md'\n" +
		"mdsmith fix -- 'plain.md'\n"
	got := ExtractHookFiles(content)
	assert.Equal(t, []string{"a'b.md", "plain.md"}, got)
}

func TestExtractHookFiles_QuotedTokens(t *testing.T) {
	content := "#!/bin/sh\n" +
		PreMergeCommitMarker + "\n" +
		"if [ -e 'PLAN.md' ]; then\n" +
		"  '/usr/bin/mdsmith' fix -- 'PLAN.md'\n" +
		"  git add -- 'PLAN.md'\n" +
		"fi\n" +
		"if [ -e 'README.md' ]; then\n" +
		"  '/usr/bin/mdsmith' fix -- 'README.md'\n" +
		"  git add -- 'README.md'\n" +
		"fi\n"
	assert.Equal(t, []string{"PLAN.md", "README.md"}, ExtractHookFiles(content))
}

func TestExtractHookFiles_IgnoresUnquoted(t *testing.T) {
	// `git add -- 'PLAN.md'` does not contain `fix --` so it is
	// ignored. The `fix --` marker must be followed by a quoted token
	// to count.
	content := "mdsmith fix -- not-quoted\n" +
		"mdsmith fix -- 'good.md'\n"
	assert.Equal(t, []string{"good.md"}, ExtractHookFiles(content))
}

func TestExtractHookFiles_OneFilePerLine(t *testing.T) {
	// Multiple quoted tokens on the same line still produce one entry
	// (the first quoted token after `fix --`).
	content := "mdsmith fix -- 'a.md' && git add -- 'a.md'\n"
	assert.Equal(t, []string{"a.md"}, ExtractHookFiles(content))
}

func TestExtractHookFiles_NoMatch(t *testing.T) {
	assert.Nil(t, ExtractHookFiles("#!/bin/sh\necho hi\n"))
}

func TestExtractHookFiles_IgnoresCommentLines(t *testing.T) {
	// A commented-out example must not produce a managed-file entry.
	content := "#!/bin/sh\n" +
		"# example: mdsmith fix -- 'commented.md'\n" +
		"mdsmith fix -- 'real.md'\n"
	assert.Equal(t, []string{"real.md"}, ExtractHookFiles(content))
}

func TestGitRepoRoot(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, exec.Command("git", "init", dir).Run())

	got, err := GitRepoRoot(dir)
	require.NoError(t, err)
	// Resolve symlinks (some platforms expose /tmp via /private/tmp etc).
	wantResolved, _ := filepath.EvalSymlinks(dir)
	gotResolved, _ := filepath.EvalSymlinks(got)
	assert.Equal(t, wantResolved, gotResolved)
}

func TestGitRepoRoot_EmptyDirDefaultsToCWD(t *testing.T) {
	// Empty dir should be treated as ".". When tests run inside the
	// mdsmith repo, this will resolve successfully — so we just check
	// that the call returns without panicking and either succeeds or
	// returns a deterministic error consistent with running git in cwd.
	got, err := GitRepoRoot("")
	if err == nil {
		assert.NotEmpty(t, got)
	}
}

func TestGitRepoRoot_NotARepo(t *testing.T) {
	dir := t.TempDir()
	_, err := GitRepoRoot(dir)
	assert.Error(t, err)
}

func TestResolveHooksDir_Default(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, exec.Command("git", "init", dir).Run())

	// Ask git itself where hooks should live so the test does not
	// hard-code .git/hooks. A developer with a non-default
	// core.hooksPath set globally would otherwise see this test
	// fail even though ResolveHooksDir is correct.
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--git-path", "hooks").Output()
	require.NoError(t, err)
	want := strings.TrimSpace(string(out))
	if !filepath.IsAbs(want) {
		want = filepath.Join(dir, want)
	}

	got := ResolveHooksDir(dir)
	gotResolved, _ := filepath.EvalSymlinks(got)
	wantResolved, _ := filepath.EvalSymlinks(filepath.Clean(want))
	assert.Equal(t, wantResolved, gotResolved)
}

func TestResolveHooksDir_FallbackWhenNotARepo(t *testing.T) {
	dir := t.TempDir()
	// No git init — `git rev-parse` fails so the function falls back
	// to <repoRoot>/.git/hooks.
	got := ResolveHooksDir(dir)
	assert.Equal(t, filepath.Join(dir, ".git", "hooks"), got)
}

func TestNormalizeManagedPath_RelativeForwardSlashes(t *testing.T) {
	got, err := NormalizeManagedPath("/repo", filepath.Join("docs", "guide.md"))
	require.NoError(t, err)
	assert.Equal(t, "docs/guide.md", got)
}

func TestNormalizeManagedPath_AbsoluteResolvesToRelative(t *testing.T) {
	got, err := NormalizeManagedPath("/repo", "/repo/docs/guide.md")
	require.NoError(t, err)
	assert.Equal(t, "docs/guide.md", got)
}

func TestNormalizeManagedPath_RejectsEmpty(t *testing.T) {
	_, err := NormalizeManagedPath("/repo", "   ")
	assert.Error(t, err)
}

func TestNormalizeManagedPath_RejectsWhitespace(t *testing.T) {
	_, err := NormalizeManagedPath("/repo", "doc with space.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "whitespace")
}

func TestNormalizeManagedPath_RejectsGlob(t *testing.T) {
	for _, p := range []string{"docs/*.md", "?ile.md", "alt[abc].md"} {
		_, err := NormalizeManagedPath("/repo", p)
		require.Errorf(t, err, "path %q should be rejected as a glob", p)
		assert.Contains(t, err.Error(), "glob/pathspec")
	}
}

func TestNormalizeManagedPath_AcceptsRepoRootWithSpaces(t *testing.T) {
	// The whitespace check must inspect the repo-relative result,
	// not the raw input, so a repo whose own path contains spaces
	// (e.g. macOS / Windows home dir) accepts an absolute path that
	// resolves to a whitespace-free repo-relative tail.
	got, err := NormalizeManagedPath("/repo with space", "/repo with space/docs/a.md")
	require.NoError(t, err)
	assert.Equal(t, "docs/a.md", got)
}

func TestNormalizeManagedPath_RejectsEscape(t *testing.T) {
	_, err := NormalizeManagedPath("/repo", "../outside.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "escapes")
}

func TestNormalizeManagedPaths_FailFast(t *testing.T) {
	_, err := NormalizeManagedPaths("/repo", []string{"good.md", "bad name.md"})
	assert.Error(t, err)
}

func TestNormalizeManagedPaths_SuccessAll(t *testing.T) {
	got, err := NormalizeManagedPaths("/repo", []string{
		filepath.Join("docs", "a.md"),
		"/repo/b.md",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"docs/a.md", "b.md"}, got)
}

func TestEnableRuleSnippet(t *testing.T) {
	got := EnableRuleSnippet("git-hook-sync")
	assert.Equal(t, "rules:\n  git-hook-sync: true\n", got)
}

func TestFirstQuotedAfter(t *testing.T) {
	tests := []struct {
		line   string
		marker string
		want   string
		ok     bool
	}{
		{"mdsmith fix -- 'a.md'", "fix --", "a.md", true},
		{"mdsmith fix -- '' && true", "fix --", "", false},
		{"mdsmith fix -- not-quoted", "fix --", "", false},
		{"unrelated line", "fix --", "", false},
		{"mdsmith fix -- 'unterminated", "fix --", "", false},
		{"mdsmith fix --", "fix --", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got, ok := firstQuotedAfter(tt.line, tt.marker)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildHookScript_GoldenFile(t *testing.T) {
	got := BuildHookScript("/usr/local/bin/mdsmith")
	golden, err := os.ReadFile(filepath.Join("testdata", "pre-merge-commit.golden.sh"))
	require.NoError(t, err, "missing golden file; regenerate with UPDATE_GOLDEN=1")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		require.NoError(t, os.WriteFile(
			filepath.Join("testdata", "pre-merge-commit.golden.sh"),
			[]byte(got), 0o644))
		return
	}
	assert.Equal(t, string(golden), got,
		"BuildHookScript output differs from testdata/pre-merge-commit.golden.sh; "+
			"run tests with UPDATE_GOLDEN=1 to regenerate")
}

func TestShellQuote_RoundTrip(t *testing.T) {
	assert.Equal(t, "'plain'", shellQuote("plain"))
	assert.Equal(t, `'a'\''b'`, shellQuote("a'b"))
	assert.Equal(t, "'with space'", shellQuote("with space"))
}

func TestHookMatchesCanonical_AcceptsCanonicalScript(t *testing.T) {
	hook := BuildHookScript("/usr/local/bin/mdsmith")
	assert.True(t, HookMatchesCanonical(hook))
}

func TestHookMatchesCanonical_RejectsNonCanonicalHooks(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("testdata", "hooks", "bad"))
	require.NoError(t, err, "testdata/hooks/bad directory must exist")
	require.NotEmpty(t, entries, "testdata/hooks/bad must contain at least one golden file")
	for _, entry := range entries {
		t.Run(entry.Name(), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", "hooks", "bad", entry.Name()))
			require.NoError(t, err)
			assert.False(t, HookMatchesCanonical(string(data)),
				"bad hook %s must be flagged as drifted", entry.Name())
		})
	}
}

func TestHookHasNonCommentLineContaining_IgnoresBlankAndComments(t *testing.T) {
	got := hookHasNonCommentLineContaining(
		"#!/bin/sh\n# example: needle\n\n  needle in real line\n",
		"needle",
	)
	assert.True(t, got)

	got = hookHasNonCommentLineContaining(
		"#!/bin/sh\n# example: needle\n\n",
		"needle",
	)
	assert.False(t, got, "comment-only matches must not satisfy the search")
}
