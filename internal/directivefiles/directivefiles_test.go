package directivefiles

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/testsymlink"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Register the directive-bearing rules so DiscoverFiles can find
	// real catalog/include/toc markers in test fixtures.
	_ "github.com/jeduden/mdsmith/internal/rules/catalog"
	_ "github.com/jeduden/mdsmith/internal/rules/include"
	_ "github.com/jeduden/mdsmith/internal/rules/toc"
)

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
	testsymlink.SkipIfSymlinkUnsupported(t)
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

func TestDiscoverFilesForInstall_ReturnsRealFilesWhenPresent(t *testing.T) {
	// When the repo already has directive-bearing files, the
	// install-time fallback ("PLAN.md", "README.md") must not
	// replace or augment the real discovered list.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "real.md"),
		[]byte("# real\n\n<?catalog?>\n<?/catalog?>\n"), 0o644))

	got := DiscoverFilesForInstall(dir, 1024*1024)
	assert.Equal(t, []string{"real.md"}, got)
}

func TestDiscoverFiles_NonexistentRepoRootYieldsNoFiles(t *testing.T) {
	// filepath.Walk invokes the callback once with a non-nil err when
	// it cannot stat the root; DiscoverFiles must swallow that error
	// and return an empty list rather than panicking or propagating it.
	got := DiscoverFiles(filepath.Join(t.TempDir(), "does-not-exist"), 1024*1024)
	assert.Empty(t, got)
}

func TestDiscoverFiles_SkipsFilesOverMaxBytes(t *testing.T) {
	// A file whose content ReadFileLimited refuses (over maxBytes)
	// must be skipped rather than surfacing an error.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "big.md"),
		[]byte("# big\n\n<?catalog?>\n<?/catalog?>\n"), 0o644))

	got := DiscoverFiles(dir, 4)
	assert.Empty(t, got, "a file larger than maxBytes must be skipped, not surfaced")
}

func TestHasDirectiveMarker_ShortBacktickRunIsNotAFence(t *testing.T) {
	// Fewer than three backticks never opens a fence per CommonMark,
	// so a directive marker on the very next line still counts.
	content := []byte("``\n<?catalog?>\n")
	assert.True(t, hasDirectiveMarker(content, []string{"catalog"}))
}

func TestHasDirectiveMarker_ShortClosingRunDoesNotCloseFence(t *testing.T) {
	// A closing run shorter than the opening run does not close the
	// fence (CommonMark requires the closer to be at least as long as
	// the opener), so the marker that follows the short "close" stays
	// hidden inside the still-open fence.
	content := []byte("````\n```\n<?catalog?>\n````\n")
	assert.False(t, hasDirectiveMarker(content, []string{"catalog"}))
}

func TestHasDirectiveMarker_ClosingFenceWithLeadingIndentation(t *testing.T) {
	// A closing fence may be preceded by up to three spaces of
	// indentation per CommonMark and still close the block.
	content := []byte("```\ninside\n   ```\n<?catalog?>\n")
	assert.True(t, hasDirectiveMarker(content, []string{"catalog"}))
}

func TestHasDirectiveMarker_ClosingFenceWithTrailingWhitespace(t *testing.T) {
	// A closing fence line may be followed by trailing whitespace
	// (spaces/tabs) and still close the block; the marker after it
	// must then count as real, top-level content.
	content := []byte("```\ninside\n```  \n<?catalog?>\n")
	assert.True(t, hasDirectiveMarker(content, []string{"catalog"}))
}
