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
