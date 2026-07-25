package main_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitattributesFixture is a .gitattributes body whose `#` comment lines
// would be parsed as ATX headings if linted as Markdown: they sit with
// no surrounding blank lines (blank-line-around-headings would insert
// them) and end in punctuation (MDS017). If the explicit-path guard
// regresses, `fix` rewrites this content and the byte-equality
// assertion fails.
const gitattributesFixture = `# BEGIN mdsmith merge-driver
*.md merge=mdsmith
*.markdown merge=mdsmith
# END mdsmith merge-driver
# Re-enable git 3-way merge for code under the trees mdsmith marks -merge.
# mdsmith derives those -merge lines from the config ignore patterns.
`

// TestFix_ExplicitNonMarkdownFileUnchanged is the regression test for
// issue #759: passing a non-Markdown file explicitly to `mdsmith fix`
// must not rewrite it. The directory walk already skips such files; an
// explicit path must behave the same.
func TestFix_ExplicitNonMarkdownFileUnchanged(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	attrPath := filepath.Join(dir, ".gitattributes")
	require.NoError(t, os.WriteFile(attrPath, []byte(gitattributesFixture), 0o644))

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "fix", ".gitattributes")
	assert.Equal(t, 0, exitCode, "fix on a non-Markdown file should succeed with nothing to do")
	assert.Contains(t, stderr, `skipping ".gitattributes"`,
		"fix should warn that the explicitly named non-Markdown file was skipped, not do nothing silently")

	got, err := os.ReadFile(attrPath)
	require.NoError(t, err)
	assert.Equal(t, gitattributesFixture, string(got),
		"fix must leave a non-Markdown file byte-for-byte unchanged")
}

// TestCheck_ExplicitNonMarkdownFileClean is the check-side counterpart:
// a non-Markdown file named explicitly is skipped (no Markdown content
// diagnostics, clean exit) but reported with a warning rather than a
// silent no-op.
func TestCheck_ExplicitNonMarkdownFileClean(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	attrPath := filepath.Join(dir, ".gitattributes")
	require.NoError(t, os.WriteFile(attrPath, []byte(gitattributesFixture), 0o644))

	stdout, stderr, exitCode := runBinaryInDir(t, dir, "", "check", ".gitattributes")
	assert.Equal(t, 0, exitCode,
		"check on a non-Markdown file should exit clean; stdout=%q stderr=%q", stdout, stderr)
	assert.NotContains(t, stderr, "MDS", "no Markdown diagnostics should be reported")
	assert.Contains(t, stderr, `skipping ".gitattributes"`,
		"check should warn that the explicitly named non-Markdown file was skipped")
}

// TestCheck_NonMarkdownWarningSuppressed verifies the skip warning is a
// text-format, non-quiet affordance only: it must not appear under
// --quiet (non-error output) or --format json (the JSON document shares
// the stderr stream and a prose line would corrupt it).
func TestCheck_NonMarkdownWarningSuppressed(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	attrPath := filepath.Join(dir, ".gitattributes")
	require.NoError(t, os.WriteFile(attrPath, []byte(gitattributesFixture), 0o644))

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"quiet", []string{"check", "--quiet", ".gitattributes"}},
		{"json", []string{"check", "-f", "json", ".gitattributes"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exitCode := runBinaryInDir(t, dir, "", tc.args...)
			assert.Equal(t, 0, exitCode, "should still exit clean; stdout=%q stderr=%q", stdout, stderr)
			assert.NotContains(t, stderr, "skipping",
				"the human skip warning must be suppressed in %s mode", tc.name)
		})
	}
}

// TestCheck_MixedExplicitPathsLintsOnlyMarkdown pins that when both a
// Markdown file and a non-Markdown file are named explicitly, only the
// Markdown one is linted: the non-Markdown file is dropped, but the
// Markdown file's genuine diagnostic still surfaces.
func TestCheck_MixedExplicitPathsLintsOnlyMarkdown(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	attrPath := filepath.Join(dir, ".gitattributes")
	mdPath := filepath.Join(dir, "bad.md")
	require.NoError(t, os.WriteFile(attrPath, []byte(gitattributesFixture), 0o644))
	// A heading ending in a period is MDS017, a default-enabled rule
	// (the issue confirms it fires); used only to prove the Markdown
	// file was actually linted.
	require.NoError(t, os.WriteFile(mdPath, []byte("# Heading.\n"), 0o644))

	stdout, stderr, exitCode := runBinaryInDir(t, dir, "", "check", ".gitattributes", "bad.md")
	assert.Equal(t, 1, exitCode,
		"the Markdown file's diagnostic should still fail the run; stdout=%q stderr=%q", stdout, stderr)
	assert.Contains(t, stderr, "bad.md", "the Markdown file should be the one reported")
	// The non-Markdown file may appear in the skip warning
	// (`skipping ".gitattributes"`), but must never appear as a linted
	// diagnostic — those are anchored as `<path>:<line>:<col>`.
	assert.NotContains(t, stderr, ".gitattributes:",
		"the non-Markdown file must not be linted (no diagnostic anchored to it)")
}
