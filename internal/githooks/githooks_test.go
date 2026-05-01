package githooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/testutil"
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

func TestExtractHookFiles_IgnoresCommentLines(t *testing.T) {
	// A commented-out example must not produce a managed-file entry.
	content := "#!/bin/sh\n" +
		"# example: mdsmith fix -- 'commented.md'\n" +
		"mdsmith fix -- 'real.md'\n"
	assert.Equal(t, []string{"real.md"}, ExtractHookFiles(content))
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

func TestDiscoverFiles_IgnoresDirectivesInsideFencedCode(t *testing.T) {
	dir := t.TempDir()
	// docs file shows a directive only inside a fenced code block,
	// e.g. as a documentation example. mdsmith does not parse such
	// markers, so DiscoverFiles must skip the file.
	docs := "# Generating Content\n\n" +
		"```markdown\n" +
		"<?catalog glob: plan/*.md ?>\n" +
		"<?/catalog?>\n" +
		"```\n"
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "guide.md"),
		[]byte(docs), 0o644))

	// real.md has the directive at document root.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real.md"),
		[]byte("# Real\n\n<?catalog?>\n<?/catalog?>\n"), 0o644))

	got := DiscoverFiles(dir, 1024*1024)
	assert.Equal(t, []string{"real.md"}, got)
}

func TestDiscoverFiles_FenceWithTrailingTextStillEncloses(t *testing.T) {
	dir := t.TempDir()
	// A line of `````` characters followed by non-whitespace is NOT a
	// closing fence in CommonMark. The marker on the next line must
	// remain inside the fenced block and so must NOT count.
	content := "# x\n\n" +
		"```sh\n" +
		"```not-a-closing-fence\n" +
		"<?catalog?>\n" +
		"<?/catalog?>\n" +
		"```\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.md"),
		[]byte(content), 0o644))

	got := DiscoverFiles(dir, 1024*1024)
	assert.Empty(t, got, "marker inside fence with trailing-text line must not count")
}

func TestDiscoverFiles_IgnoresDirectivesInIndentedCodeBlocks(t *testing.T) {
	dir := t.TempDir()
	// Indented (4-space) and tab-indented blocks are CommonMark
	// indented code blocks; mdsmith's PI parser refuses them too.
	// A line with up to three leading spaces followed by a tab is
	// also an indented block, so the directive markers there must
	// not count either.
	indented := "# Examples\n\n" +
		"    <?catalog glob: plan/*.md ?>\n" +
		"    <?/catalog?>\n\n" +
		"\t<?include file: x.md ?>\n" +
		"\t<?/include?>\n\n" +
		"   \t<?toc?>\n" +
		"   \t<?/toc?>\n"
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "guide.md"),
		[]byte(indented), 0o644))

	// real.md has the directive at column 0.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real.md"),
		[]byte("# Real\n\n<?catalog?>\n<?/catalog?>\n"), 0o644))

	got := DiscoverFiles(dir, 1024*1024)
	assert.Equal(t, []string{"real.md"}, got)
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
	testutil.SkipIfSymlinkUnsupported(t)
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

func TestDiscoverFiles_EmptyWhenNoDirectives(t *testing.T) {
	dir := t.TempDir()
	// Plain markdown file with no directives — DiscoverFiles must
	// return an empty slice rather than the install-time fallback.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plain.md"),
		[]byte("# Plain\n\nNo directives here.\n"), 0o644))

	got := DiscoverFiles(dir, 1024*1024)
	assert.Empty(t, got)
}

func TestDiscoverFilesForInstall_FallsBackOnEmpty(t *testing.T) {
	dir := t.TempDir()
	got := DiscoverFilesForInstall(dir, 1024*1024)
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

func TestWriteGitattributes_CreatesNewFileWithManagedBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")
	files := []string{"a.md", "b.md"}

	err := WriteGitattributes(path, files)
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

	files := []string{"test.md"}
	err = WriteGitattributes(path, files)
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

	files := []string{"new.md", "other.md"}
	err = WriteGitattributes(path, files)
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

	err = WriteGitattributes(path, []string{"new.md"})
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

func TestWriteGitattributes_StripsStaleMdsmithEntriesWhenNoBlockExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	initial := "*.txt text eol=lf\n" +
		"stale1.md merge=mdsmith\n" +
		"stale2.md merge=mdsmith\n"
	err := os.WriteFile(path, []byte(initial), 0644)
	require.NoError(t, err)

	err = WriteGitattributes(path, []string{"new.md"})
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

	err := WriteGitattributes(path, []string{})
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

	files := []string{"test.md"}
	err = WriteGitattributes(path, files)
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	expected := "*.txt text eol=lf\n" +
		"# BEGIN mdsmith merge-driver\n" +
		"test.md merge=mdsmith\n" +
		"# END mdsmith merge-driver\n"
	assert.Equal(t, expected, string(content))
}
