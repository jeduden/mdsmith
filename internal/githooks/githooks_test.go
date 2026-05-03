package githooks

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
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

func TestDiscoverFiles_RespectsConfigIgnorePatterns(t *testing.T) {
	// Discovery must consult the project's .mdsmith.yml ignore list
	// so the merge-driver assignments and pre-merge-commit hook only
	// reference paths mdsmith would actually process. Without the
	// filter, .gitattributes ends up listing fixture/example files
	// that mdsmith fix skips, so a real merge conflict in those
	// files would invoke the merge driver but fix nothing.
	dir := t.TempDir()
	files := map[string]string{
		".mdsmith.yml": "ignore:\n" +
			"  - \"fixtures/**\"\n" +
			"  - \"vendor/inner/skip.md\"\n",
		"README.md":            "# Test\n\n<?catalog?>\n<?/catalog?>\n",
		"docs/guide.md":        "# Guide\n\n<?toc?>\n<?/toc?>\n",
		"fixtures/bad.md":      "# Bad fixture\n\n<?catalog?>\n<?/catalog?>\n",
		"fixtures/sub/x.md":    "# Sub\n\n<?include file=\"y.md\"?><?/include?>\n",
		"vendor/inner/skip.md": "# Skip\n\n<?toc?>\n<?/toc?>\n",
		"vendor/inner/keep.md": "# Kept\n\n<?catalog?>\n<?/catalog?>\n",
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	got := DiscoverFiles(dir, 1024*1024)

	assert.Contains(t, got, "README.md", "non-ignored top-level file is discovered")
	assert.Contains(t, got, "docs/guide.md", "non-ignored nested file is discovered")
	assert.Contains(t, got, "vendor/inner/keep.md",
		"siblings of an ignored exact path are still discovered")
	assert.NotContains(t, got, "fixtures/bad.md",
		"file matched by `fixtures/**` must be filtered out")
	assert.NotContains(t, got, "fixtures/sub/x.md",
		"file matched by `fixtures/**` must be filtered out (deep)")
	assert.NotContains(t, got, "vendor/inner/skip.md",
		"exact-path ignore must filter that file")
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

func TestDefaultIncludes(t *testing.T) {
	got := DefaultIncludes()
	assert.Equal(t, []string{"*.md", "*.markdown"}, got)
	// Each call must return a fresh slice so callers can mutate it
	// without affecting later callers.
	got[0] = "mutated"
	assert.Equal(t, []string{"*.md", "*.markdown"}, DefaultIncludes())
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
	assert.Equal(t, []string{"demo/**", "vendor/**"}, got.Exclude)
	assert.Empty(t, skipped, "representable patterns must not be reported as skipped")
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
	assert.Equal(t, []string{"demo/**", "vendor/**"}, got.Exclude)
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
	expected := "# BEGIN mdsmith merge-driver\n" +
		"*.md merge=mdsmith\n" +
		"*.markdown merge=mdsmith\n" +
		"demo/** -merge\n" +
		"vendor/*.md -merge\n" +
		"# END mdsmith merge-driver\n"
	assert.Equal(t, expected, got)
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
	// A line inside the managed block that is not a merge=mdsmith
	// or -merge assignment must be ignored, not counted as a glob.
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

func TestBuildHookScript_EmbedsBinaryAndUsesGlobs(t *testing.T) {
	got := BuildHookScript("/usr/local/bin/mdsmith")
	assert.Contains(t, got, "#!/bin/sh")
	assert.Contains(t, got, PreMergeCommitMarker)
	assert.Contains(t, got, "cd \"$(git rev-parse --show-toplevel)\"")
	assert.Contains(t, got, "if ! '/usr/local/bin/mdsmith' fix .; then")
	assert.Contains(t, got, `if [ "$status" -ne 1 ]; then`,
		"hook must propagate exit codes other than 1 (unfixed diagnostics)")
	assert.Contains(t, got, "git diff --name-only -- '*.md' '*.markdown' |")
}

func TestBuildHookScript_QuotesBinaryWithEmbeddedQuote(t *testing.T) {
	got := BuildHookScript("/path/it's/mdsmith")
	assert.Contains(t, got, `if ! '/path/it'\''s/mdsmith' fix .; then`,
		"single quote in path must be encoded as `'\\''` so the shell sees the literal path")
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

func TestHookMatchesCanonical_RejectsMissingChdir(t *testing.T) {
	// Drop the `cd "$(git rev-parse ...)"` line.
	hook := "#!/bin/sh\n" + PreMergeCommitMarker + "\n" +
		"set -e\n" +
		"if ! '/usr/local/bin/mdsmith' fix .; then\n" +
		"  status=$?\n" +
		"  if [ \"$status\" -ne 1 ]; then exit \"$status\"; fi\n" +
		"fi\n" +
		"git diff --name-only -- '*.md' '*.markdown' |\n"
	assert.False(t, HookMatchesCanonical(hook))
}

func TestHookMatchesCanonical_RejectsLegacyFixCommand(t *testing.T) {
	// Old per-file `fix --` style instead of glob-based `fix .`.
	hook := "#!/bin/sh\n" + PreMergeCommitMarker + "\n" +
		"set -e\n" +
		"cd \"$(git rev-parse --show-toplevel)\"\n" +
		"'/usr/local/bin/mdsmith' fix -- 'PLAN.md'\n" +
		"git diff --name-only -- '*.md' '*.markdown' |\n"
	assert.False(t, HookMatchesCanonical(hook))
}

func TestHookMatchesCanonical_RejectsLegacyOrTrueGuard(t *testing.T) {
	// Old `fix . || true` form swallowed every non-zero exit. The new
	// canonical template uses an `if ! ... fix .; then` block so
	// genuine errors propagate. The drift check must reject the
	// permissive legacy form.
	hook := "#!/bin/sh\n" + PreMergeCommitMarker + "\n" +
		"set -e\n" +
		"cd \"$(git rev-parse --show-toplevel)\"\n" +
		"'/usr/local/bin/mdsmith' fix . || true\n" +
		"git diff --name-only -- '*.md' '*.markdown' |\n"
	assert.False(t, HookMatchesCanonical(hook))
}

func TestHookMatchesCanonical_RejectsMissingStagingLine(t *testing.T) {
	hook := "#!/bin/sh\n" + PreMergeCommitMarker + "\n" +
		"set -e\n" +
		"cd \"$(git rev-parse --show-toplevel)\"\n" +
		"if ! '/usr/local/bin/mdsmith' fix .; then\n" +
		"  status=$?\n" +
		"  if [ \"$status\" -ne 1 ]; then exit \"$status\"; fi\n" +
		"fi\n"
	assert.False(t, HookMatchesCanonical(hook))
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
	assert.Equal(t, []string{"demo/**", "vendor/**"}, got.Exclude,
		"only representable patterns survive the validation filter")
	assert.Equal(t, []string{"!docs/*.md", "with space"}, skipped,
		"dropped patterns are returned in input order so callers can warn")
}

func TestHookMatchesCanonical_RejectsMissingStagingPipeline(t *testing.T) {
	hook := "#!/bin/sh\n" + PreMergeCommitMarker + "\n" +
		"set -e\n" +
		"cd \"$(git rev-parse --show-toplevel)\"\n" +
		"if ! '/usr/local/bin/mdsmith' fix .; then\n" +
		"  status=$?\n" +
		"  if [ \"$status\" -ne 1 ]; then exit \"$status\"; fi\n" +
		"fi\n" +
		// Missing the `git diff --name-only ... |` pipeline header.
		"while IFS= read -r f; do git add -- \"$f\"; done\n"
	assert.False(t, HookMatchesCanonical(hook))
}

func TestHookMatchesCanonical_RejectsMissingReadLoop(t *testing.T) {
	hook := "#!/bin/sh\n" + PreMergeCommitMarker + "\n" +
		"set -e\n" +
		"cd \"$(git rev-parse --show-toplevel)\"\n" +
		"if ! '/usr/local/bin/mdsmith' fix .; then\n" +
		"  status=$?\n" +
		"  if [ \"$status\" -ne 1 ]; then exit \"$status\"; fi\n" +
		"fi\n" +
		"git diff --name-only -- '*.md' '*.markdown' | xargs git add --\n"
	assert.False(t, HookMatchesCanonical(hook),
		"a non-canonical staging pipeline (e.g. xargs without the read loop) must be flagged")
}

func TestHookMatchesCanonical_RejectsMissingExitGuard(t *testing.T) {
	// fix .; then is present but the `[ "$status" -ne 1 ]` guard
	// is missing, meaning genuine errors would be swallowed.
	hook := "#!/bin/sh\n" + PreMergeCommitMarker + "\n" +
		"set -e\n" +
		"cd \"$(git rev-parse --show-toplevel)\"\n" +
		"if ! '/usr/local/bin/mdsmith' fix .; then\n" +
		"  true\n" +
		"fi\n" +
		"git diff --name-only -- '*.md' '*.markdown' | while IFS= read -r f; do\n" +
		"  git add -- \"$f\"\ndone\n"
	assert.False(t, HookMatchesCanonical(hook))
}

func TestExtractGlobs_SkipsSingleFieldLines(t *testing.T) {
	// A managed-block line with only a pattern (no attribute) is
	// not a valid merge=mdsmith or -merge assignment; ExtractGlobs
	// must skip it instead of treating the lone token as a glob.
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

	orig := chmodFn
	t.Cleanup(func() { chmodFn = orig })
	chmodFn = func(string, os.FileMode) error {
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

func TestHookMatchesCanonical_RejectsCanonicalLinesInsideComments(t *testing.T) {
	// A drifted hook that mentions the canonical commands inside
	// shell comments must not be treated as canonical. Only
	// non-comment lines satisfy the required-fragment checks.
	hook := "#!/bin/sh\n" + PreMergeCommitMarker + "\n" +
		"# example: cd \"$(git rev-parse --show-toplevel)\"\n" +
		"# example: fix .; then\n" +
		"# example: if [ \"$status\" -ne 1 ]; then\n" +
		"# example: git diff --name-only -- '*.md' '*.markdown' |\n" +
		"# example: while IFS= read -r f; do\n" +
		"echo placeholder\n"
	assert.False(t, HookMatchesCanonical(hook),
		"required commands sitting in comments must not satisfy the drift check")
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
