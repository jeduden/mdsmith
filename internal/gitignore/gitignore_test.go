package gitignore

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- trimTrailingWhitespace tests ---

func TestTrimTrailingWhitespace_NoWhitespace(t *testing.T) {
	assert.Equal(t, "hello", trimTrailingWhitespace("hello"))
}

func TestTrimTrailingWhitespace_TrailingSpaces(t *testing.T) {
	assert.Equal(t, "hello", trimTrailingWhitespace("hello   "))
}

func TestTrimTrailingWhitespace_TrailingTabs(t *testing.T) {
	assert.Equal(t, "hello", trimTrailingWhitespace("hello\t\t"))
}

func TestTrimTrailingWhitespace_EscapedSpace(t *testing.T) {
	// Backslash before trailing space preserves one space.
	assert.Equal(t, "hello ", trimTrailingWhitespace("hello\\  "))
}

func TestTrimTrailingWhitespace_EmptyString(t *testing.T) {
	assert.Equal(t, "", trimTrailingWhitespace(""))
}

func TestTrimTrailingWhitespace_AllWhitespace(t *testing.T) {
	assert.Equal(t, "", trimTrailingWhitespace("   "))
}

// --- relTo tests ---

// TestRelTo_AgreesWithFilepathRel pins the fast prefix-strip path to
// filepath.Rel's answer for the absolute, cleaned inputs the matcher
// sees, including paths outside the base.
func TestRelTo_AgreesWithFilepathRel(t *testing.T) {
	cases := []struct{ base, path string }{
		{"/repo", "/repo/file.md"},
		{"/repo", "/repo/sub/dir/file.md"},
		{"/repo", "/repo"},
		{"/repo/sub", "/repo/other/file.md"},
		{"/repo/sub", "/repo/subdir/file.md"},
		{"/", "/file.md"},
		{"/a/b", "/a/bc/d"},
	}
	for _, tc := range cases {
		want, wantErr := filepath.Rel(tc.base, tc.path)
		got, gotErr := relTo(tc.base, tc.path)
		assert.Equal(t, wantErr == nil, gotErr == nil, "err for (%q, %q)", tc.base, tc.path)
		if wantErr == nil {
			assert.Equal(t, want, got, "rel for (%q, %q)", tc.base, tc.path)
		}
	}
}

// --- NewMatcher tests ---

func TestNewMatcher_NoGitignore(t *testing.T) {
	dir := t.TempDir()
	m := NewMatcher(dir)
	require.NotNil(t, m)
	// Use a unique extension unlikely to appear in any ancestor .gitignore.
	path := filepath.Join(dir, t.Name()+"-not-ignored.mdsmith-test-unique")
	assert.False(t, m.IsIgnored(path, false))
}

func TestNewMatcher_WithGitignore(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\nbuild/\n"), 0o644))

	m := NewMatcher(dir)
	require.NotNil(t, m)
	assert.True(t, len(m.rules) >= 2, "expected at least 2 rules from .gitignore")
}

func TestNewMatcher_NestedGitignore(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sub, ".gitignore"), []byte("draft.md\n"), 0o644))

	m := NewMatcher(dir)
	require.NotNil(t, m)
	// Should have rules from both .gitignore files.
	assert.True(t, len(m.rules) >= 2)
}

func TestNewMatcher_UnreadableGitignore(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission test not reliable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("permission test not reliable as root")
	}
	dir := t.TempDir()
	// A valid .gitignore in the root so we have something to match against.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\n"), 0o644))

	// A subdirectory with an unreadable .gitignore (chmod 000).
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	bad := filepath.Join(sub, ".gitignore")
	require.NoError(t, os.WriteFile(bad, []byte("*.tmp\n"), 0o644))
	require.NoError(t, os.Chmod(bad, 0o000))
	defer func() { _ = os.Chmod(bad, 0o644) }()

	// NewMatcher should not panic; it silently skips unreadable files.
	m := NewMatcher(dir)
	require.NotNil(t, m)

	// Rules from the readable root .gitignore should still be active.
	logFile := filepath.Join(dir, "test.log")
	assert.True(t, m.IsIgnored(logFile, false), "*.log rule from root .gitignore should still apply")
}

func TestNewMatcher_NegationPattern(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"),
		[]byte("*.md\n!keep.md\n"), 0o644))

	m := NewMatcher(dir)
	require.NotNil(t, m)

	// keep.md should NOT be ignored due to negation.
	keepAbs := filepath.Join(dir, "keep.md")
	assert.False(t, m.IsIgnored(keepAbs, false), "keep.md should not be ignored")

	// other.md should be ignored.
	otherAbs := filepath.Join(dir, "other.md")
	assert.True(t, m.IsIgnored(otherAbs, false), "other.md should be ignored")
}

// --- Matcher.IsIgnored tests ---

func TestMatcher_IsIgnored_DirOnly(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("build/\n"), 0o644))

	m := NewMatcher(dir)

	// A file named "build" should NOT be ignored by "build/" pattern.
	buildFile := filepath.Join(dir, "build")
	assert.False(t, m.IsIgnored(buildFile, false), "file named 'build' should not be ignored by dir-only pattern")

	// A directory named "build" should be ignored.
	buildDir := filepath.Join(dir, "build")
	assert.True(t, m.IsIgnored(buildDir, true), "dir named 'build' should be ignored by dir-only pattern")
}

// --- matchGitignorePattern tests ---

func TestMatchGitignorePattern_Simple(t *testing.T) {
	assert.True(t, matchGitignorePattern("*.md", "readme.md"))
	assert.False(t, matchGitignorePattern("*.md", "readme.txt"))
}

func TestMatchGitignorePattern_Doublestar(t *testing.T) {
	assert.True(t, matchGitignorePattern("**/*.md", "sub/readme.md"))
	assert.True(t, matchGitignorePattern("**/*.md", "a/b/c.md"))
}

func TestMatchGitignorePattern_ExactMatch(t *testing.T) {
	assert.True(t, matchGitignorePattern("readme.md", "readme.md"))
	assert.False(t, matchGitignorePattern("readme.md", "other.md"))
}

// --- matchDoublestar tests ---

func TestMatchDoublestar_LeadingDoublestar(t *testing.T) {
	assert.True(t, matchDoublestar("**/*.md", "readme.md"))
	assert.True(t, matchDoublestar("**/*.md", "sub/readme.md"))
	assert.True(t, matchDoublestar("**/*.md", "a/b/c.md"))
}

func TestMatchDoublestar_TrailingDoublestar(t *testing.T) {
	assert.True(t, matchDoublestar("docs/**", "docs/readme.md"))
	assert.True(t, matchDoublestar("docs/**", "docs/sub/file.md"))
	assert.True(t, matchDoublestar("docs/**", "docs"))
}

func TestMatchDoublestar_MiddleDoublestar(t *testing.T) {
	assert.True(t, matchDoublestar("a/**/b.md", "a/b.md"))
	// Middle ** with single intermediate dir: prefix "a" matches pathParts[:1]="a",
	// suffix "b.md" must match pathParts[1:]="sub/b.md" which it doesn't via
	// filepath.Match. This is a known limitation of the simple ** implementation.
	// Verify the zero-depth case works.
	assert.True(t, matchDoublestar("docs/**/readme.md", "docs/readme.md"))
}

func TestMatchDoublestar_JustDoublestar(t *testing.T) {
	assert.True(t, matchDoublestar("**", "anything"))
	assert.True(t, matchDoublestar("**", "a/b/c"))
}

func TestMatchDoublestar_LeadingSlashDoublestar(t *testing.T) {
	assert.True(t, matchDoublestar("/**/*.md", "readme.md"))
	assert.True(t, matchDoublestar("/**/*.md", "sub/readme.md"))
}

func TestMatchDoublestar_TrailingSlashDoublestar(t *testing.T) {
	assert.True(t, matchDoublestar("docs/**/", "docs/sub"))
	assert.True(t, matchDoublestar("docs/**/", "docs"))
}

func TestMatchDoublestar_MultipleDoublestars(t *testing.T) {
	// Pattern with multiple ** falls back to simple matching.
	assert.True(t, matchDoublestar("a/**/b/**/c", "a/x/b/y/c"))
}

func TestMatchDoublestar_NoMatch(t *testing.T) {
	assert.False(t, matchDoublestar("docs/**/*.md", "src/file.md"))
	// Leading ** with no matching tail: exercises the return-false path in
	// matchLeadingDoublestar after the tail-advancing loop exhausts all slashes.
	assert.False(t, matchDoublestar("**/*.md", "readme.txt"))
	assert.False(t, matchDoublestar("**/foo.md", "bar/baz.txt"))
}

func TestMatchDoublestar_MiddleDoublestar_SuffixOnly(t *testing.T) {
	// When ** expands to zero segments and the path starts with the suffix,
	// matchMiddleDoublestar returns true via the pre-loop check.
	// This is the same behavior as the original (i=0 guard skipped prefix check).
	assert.True(t, matchDoublestar("a/**/b.md", "b.md"))
}

// --- parseGitignoreFile tests ---

func TestParseGitignoreFile_Comments(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	content := "# This is a comment\n\n*.log\n# Another comment\nbuild/\n"
	require.NoError(t, os.WriteFile(gi, []byte(content), 0o644))

	rules, err := parseGitignoreFile(gi)
	require.NoError(t, err)
	assert.Len(t, rules, 2) // *.log and build/
}

func TestParseGitignoreFile_Negation(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	content := "*.md\n!keep.md\n"
	require.NoError(t, os.WriteFile(gi, []byte(content), 0o644))

	rules, err := parseGitignoreFile(gi)
	require.NoError(t, err)
	require.Len(t, rules, 2)
	assert.False(t, rules[0].negate)
	assert.True(t, rules[1].negate)
}

func TestParseGitignoreFile_DirOnly(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	content := "build/\n"
	require.NoError(t, os.WriteFile(gi, []byte(content), 0o644))

	rules, err := parseGitignoreFile(gi)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.True(t, rules[0].dirOnly)
}

func TestParseGitignoreFile_LeadingSlash(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	content := "/build\n"
	require.NoError(t, os.WriteFile(gi, []byte(content), 0o644))

	rules, err := parseGitignoreFile(gi)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.True(t, rules[0].hasSlash)
	assert.Equal(t, "build", rules[0].pattern)
}

func TestParseGitignoreFile_SlashInMiddle(t *testing.T) {
	dir := t.TempDir()
	gi := filepath.Join(dir, ".gitignore")
	content := "sub/dir\n"
	require.NoError(t, os.WriteFile(gi, []byte(content), 0o644))

	rules, err := parseGitignoreFile(gi)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.True(t, rules[0].hasSlash)
}

func TestParseGitignoreFile_Nonexistent(t *testing.T) {
	_, err := parseGitignoreFile(filepath.Join(t.TempDir(), "no-such/.gitignore"))
	assert.Error(t, err)
}

// --- matchRule edge cases ---

func TestMatchRule_OutsideBase(t *testing.T) {
	r := ignoreRule{base: "/home/user/project", pattern: "*.md"}
	// Path outside the base should not match.
	assert.False(t, matchRule(r, "/other/path/file.md"))
}

// --- NewMatcher walk pruning ---

func TestNewMatcher_SkipsDotGitContents(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git", "info"), 0o755))
	// A pattern file inside .git must not be collected: git never
	// applies .gitignore files from inside the metadata directory.
	require.NoError(t, os.WriteFile(filepath.Join(root, ".git", "info", ".gitignore"),
		[]byte("*.md\n"), 0o644))

	m := NewMatcher(root)
	require.NotNil(t, m)
	assert.False(t, m.IsIgnored(filepath.Join(root, "doc.md"), false),
		"a .gitignore inside .git must not contribute rules")
}

func TestNewMatcher_PrunesIgnoredDirectories(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"),
		[]byte("vendor/\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "vendor", "dep"), 0o755))
	// Git cannot re-include below an excluded directory, so rules in
	// there are inert; the walk must not descend (or collect) them.
	require.NoError(t, os.WriteFile(filepath.Join(root, "vendor", "dep", ".gitignore"),
		[]byte("*.md\n"), 0o644))

	m := NewMatcher(root)
	require.NotNil(t, m)
	// The matcher's contract is per-path: consumers test ancestor
	// directories themselves (catalog's isGitignored, the workspace
	// walk), so the vendor/ verdict is what they consult.
	assert.True(t, m.IsIgnored(filepath.Join(root, "vendor"), true))
	assert.False(t, m.IsIgnored(filepath.Join(root, "doc.md"), false),
		"a .gitignore under an ignored directory must not contribute rules")
}

func TestNewMatcher_StillReadsNestedGitignores(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", ".gitignore"),
		[]byte("draft.md\n"), 0o644))

	m := NewMatcher(root)
	require.NotNil(t, m)
	assert.True(t, m.IsIgnored(filepath.Join(root, "docs", "draft.md"), false))
	assert.False(t, m.IsIgnored(filepath.Join(root, "docs", "kept.md"), false))
}
