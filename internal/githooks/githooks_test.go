package githooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Register the directive-bearing rules so DiscoverFiles can find
	// real catalog/include/toc markers in test fixtures.
	_ "github.com/jeduden/mdsmith/internal/rules/catalog"
	_ "github.com/jeduden/mdsmith/internal/rules/include"
	_ "github.com/jeduden/mdsmith/internal/rules/toc"
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

func TestDiscoverFiles_FindsDirectives(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"README.md":         "# Test\n\n<?catalog?>\n<?/catalog?>\n",
		"docs/guide.md":     "# Guide\n\n<?toc?>\n<?/toc?>\n",
		"plain.md":          "# No directives\n",
		"notes.txt":         "ignored non-markdown",
		".hidden/secret.md": "<?catalog?>\n",
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	got := DiscoverFiles(dir, 1024*1024)
	assert.Contains(t, got, "README.md")
	assert.Contains(t, got, "docs/guide.md", "paths should use forward slashes")
	assert.NotContains(t, got, "plain.md")
	assert.NotContains(t, got, ".hidden/secret.md")
}

func TestDiscoverFiles_IgnoresDirectiveMentionsInProse(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"README.md":      "# Test\n\nUse `<?catalog?>` in generated sections.\n",
		"docs/guide.md":  "# Guide\n\nThis guide mentions `<?toc?>` and `<?/toc?>` inline.\n",
		"docs/real.md":   "# Real\n\n<?include file: \"docs/source.md\"?>\n<?/include?>\n",
		"docs/source.md": "source content\n",
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	got := DiscoverFiles(dir, 1024*1024)
	assert.Contains(t, got, "docs/real.md")
	assert.NotContains(t, got, "README.md")
	assert.NotContains(t, got, "docs/guide.md")
}

func TestDiscoverFiles_SkipsSymlinks(t *testing.T) {
	dir := t.TempDir()
	// A real file with a directive that should be discovered.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real.md"),
		[]byte("# real\n\n<?catalog?>\n<?/catalog?>\n"), 0o644))
	// A symlink whose name ends in .md and would otherwise be read
	// twice (or follow outside the repo). DiscoverFiles must skip
	// it because it is not a regular file.
	target := filepath.Join(dir, "real.md")
	link := filepath.Join(dir, "link.md")
	require.NoError(t, os.Symlink(target, link))

	got := DiscoverFiles(dir, 1024*1024)
	assert.Equal(t, []string{"real.md"}, got)
}

func TestDiscoverFiles_SortedAndDeduplicated(t *testing.T) {
	dir := t.TempDir()
	// Create files with directives in non-alphabetical layout to
	// confirm DiscoverFiles returns a sorted slice.
	for _, name := range []string{"z.md", "a.md", "m.md"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name),
			[]byte("# x\n\n<?catalog?>\n<?/catalog?>\n"), 0o644))
	}
	got := DiscoverFiles(dir, 1024*1024)
	assert.Equal(t, []string{"a.md", "m.md", "z.md"}, got)
}

func TestDiscoverFiles_FallbackOnEmpty(t *testing.T) {
	dir := t.TempDir()
	got := DiscoverFiles(dir, 1024*1024)
	assert.Equal(t, []string{"PLAN.md", "README.md"}, got)
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

	got := ResolveHooksDir(dir)
	gotResolved, _ := filepath.EvalSymlinks(got)
	wantResolved, _ := filepath.EvalSymlinks(filepath.Join(dir, ".git", "hooks"))
	assert.Equal(t, wantResolved, gotResolved)
}

func TestResolveHooksDir_FallbackWhenNotARepo(t *testing.T) {
	dir := t.TempDir()
	// No git init — `git rev-parse` fails so the function falls back
	// to <repoRoot>/.git/hooks.
	got := ResolveHooksDir(dir)
	assert.Equal(t, filepath.Join(dir, ".git", "hooks"), got)
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
