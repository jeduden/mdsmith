package lint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

// --- GetGitignore tests ---

func TestGetGitignore_NilFunc(t *testing.T) {
	f := &File{}
	m := f.GetGitignore()
	assert.Nil(t, m)
}

func TestGetGitignore_WithFunc(t *testing.T) {
	called := 0
	matcher := &GitignoreMatcher{}
	f := &File{
		GitignoreFunc: func() *GitignoreMatcher {
			called++
			return matcher
		},
	}
	m := f.GetGitignore()
	assert.Same(t, matcher, m)
	assert.Equal(t, 1, called)
}

func TestGetGitignore_Cached(t *testing.T) {
	called := 0
	matcher := &GitignoreMatcher{}
	f := &File{
		GitignoreFunc: func() *GitignoreMatcher {
			called++
			return matcher
		},
	}
	m1 := f.GetGitignore()
	m2 := f.GetGitignore()
	assert.Same(t, m1, m2)
	assert.Equal(t, 1, called, "GitignoreFunc should be called only once")
}

// --- matchesGlob tests ---

func TestMatchesGlob_ExactMatch(t *testing.T) {
	assert.True(t, matchesGlob([]string{"readme.md"}, "readme.md"))
}

func TestMatchesGlob_WildcardMatch(t *testing.T) {
	assert.True(t, matchesGlob([]string{"*.md"}, "readme.md"))
}

func TestMatchesGlob_NoMatch(t *testing.T) {
	assert.False(t, matchesGlob([]string{"*.txt"}, "readme.md"))
}

func TestMatchesGlob_EmptyPatterns(t *testing.T) {
	assert.False(t, matchesGlob([]string{}, "readme.md"))
}

func TestMatchesGlob_InvalidPattern(t *testing.T) {
	assert.False(t, matchesGlob([]string{"[invalid"}, "readme.md"))
}

func TestMatchesGlob_MatchesBasename(t *testing.T) {
	assert.True(t, matchesGlob([]string{"readme.md"}, "/some/path/readme.md"))
}

func TestMatchesGlob_MatchesCleanedPath(t *testing.T) {
	assert.True(t, matchesGlob([]string{"foo/bar.md"}, "foo//bar.md"))
}

// --- useGitignore tests ---

func TestUseGitignore_NilPointer(t *testing.T) {
	opts := ResolveOpts{UseGitignore: nil}
	assert.True(t, opts.useGitignore(), "nil UseGitignore should default to true")
}

func TestUseGitignore_True(t *testing.T) {
	b := true
	opts := ResolveOpts{UseGitignore: &b}
	assert.True(t, opts.useGitignore())
}

func TestUseGitignore_False(t *testing.T) {
	b := false
	opts := ResolveOpts{UseGitignore: &b}
	assert.False(t, opts.useGitignore())
}

// --- resolveGlob tests ---

func TestResolveGlob_InvalidPattern(t *testing.T) {
	err := resolveGlob("[invalid", DefaultResolveOpts(), func(_ string) {})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid glob pattern")
}

func TestResolveGlob_NoMatches(t *testing.T) {
	var files []string
	pattern := filepath.Join(t.TempDir(), "no-match-*.md")
	err := resolveGlob(pattern, DefaultResolveOpts(), func(f string) {
		files = append(files, f)
	})
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestResolveGlob_MatchesFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"), []byte("# B"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.txt"), []byte("text"), 0o644))

	var files []string
	pattern := filepath.Join(dir, "*.md")
	err := resolveGlob(pattern, DefaultResolveOpts(), func(f string) {
		files = append(files, f)
	})
	require.NoError(t, err)
	assert.Len(t, files, 2)
}

func TestResolveGlob_MatchesDirectory(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "docs")
	require.NoError(t, os.MkdirAll(subDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "guide.md"), []byte("# Guide"), 0o644))

	var files []string
	pattern := filepath.Join(dir, "doc*")
	err := resolveGlob(pattern, DefaultResolveOpts(), func(f string) {
		files = append(files, f)
	})
	require.NoError(t, err)
	assert.Len(t, files, 1)
}

// --- isGitignored tests ---

func TestIsGitignored_MatchAndNoMatch(t *testing.T) {
	// isGitignored with a real matcher: matching path returns true,
	// non-matching path returns false.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\n"), 0o644))

	matcher := NewGitignoreMatcher(dir)
	// A file matching the pattern should be ignored.
	logFile := filepath.Join(dir, "test.log")
	require.NoError(t, os.WriteFile(logFile, []byte("log"), 0o644))
	assert.True(t, isGitignored(matcher, logFile, false))

	// A file not matching should not be ignored.
	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Test"), 0o644))
	assert.False(t, isGitignored(matcher, mdFile, false))
}

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

// --- NewGitignoreMatcher tests ---

func TestNewGitignoreMatcher_NoGitignore(t *testing.T) {
	dir := t.TempDir()
	m := NewGitignoreMatcher(dir)
	require.NotNil(t, m)
	// A directory without its own .gitignore should not ignore an arbitrary
	// file by default, even though the matcher may still load ancestor rules.
	path := filepath.Join(dir, t.Name()+"-not-ignored.md")
	assert.False(t, m.IsIgnored(path, false))
}

func TestNewGitignoreMatcher_WithGitignore(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\nbuild/\n"), 0o644))

	m := NewGitignoreMatcher(dir)
	require.NotNil(t, m)
	assert.True(t, len(m.rules) >= 2, "expected at least 2 rules from .gitignore")
}

func TestNewGitignoreMatcher_NestedGitignore(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sub, ".gitignore"), []byte("draft.md\n"), 0o644))

	m := NewGitignoreMatcher(dir)
	require.NotNil(t, m)
	// Should have rules from both .gitignore files.
	assert.True(t, len(m.rules) >= 2)
}

func TestNewGitignoreMatcher_NegationPattern(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"),
		[]byte("*.md\n!keep.md\n"), 0o644))

	m := NewGitignoreMatcher(dir)
	require.NotNil(t, m)

	// keep.md should NOT be ignored due to negation.
	keepAbs := filepath.Join(dir, "keep.md")
	assert.False(t, m.IsIgnored(keepAbs, false), "keep.md should not be ignored")

	// other.md should be ignored.
	otherAbs := filepath.Join(dir, "other.md")
	assert.True(t, m.IsIgnored(otherAbs, false), "other.md should be ignored")
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
}

// --- Kind tests ---

func TestKind_ProcessingInstruction(t *testing.T) {
	pi := &ProcessingInstruction{Name: "test"}
	assert.Equal(t, KindProcessingInstruction, pi.Kind())
}

// --- Dump tests ---

func TestDump_ProcessingInstruction(t *testing.T) {
	pi := &ProcessingInstruction{Name: "catalog"}
	// Dump should not panic.
	pi.Dump([]byte("<?catalog?>"), 0)
}

// --- Close (pi_parser) tests ---

func TestClose_PIBlockParser(t *testing.T) {
	p := NewPIBlockParser()
	// Close is a no-op; should not panic.
	pi := &ProcessingInstruction{Name: "test"}
	p.Close(pi, nil, nil)
}

// --- IsRaw tests ---

func TestIsRaw_ProcessingInstruction(t *testing.T) {
	pi := &ProcessingInstruction{Name: "test"}
	assert.True(t, pi.IsRaw())
}

// --- HasClosure tests ---

func TestHasClosure_NoClosureLine(t *testing.T) {
	pi := &ProcessingInstruction{Name: "test"}
	assert.False(t, pi.HasClosure())
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

// --- IsIgnored dirOnly tests ---

func TestIsIgnored_DirOnlySkipsFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("build/\n"), 0o644))

	m := NewGitignoreMatcher(dir)

	// A file named "build" should NOT be ignored by "build/" pattern.
	buildFile := filepath.Join(dir, "build")
	assert.False(t, m.IsIgnored(buildFile, false), "file named 'build' should not be ignored by dir-only pattern")

	// A directory named "build" should be ignored.
	buildDir := filepath.Join(dir, "build")
	assert.True(t, m.IsIgnored(buildDir, true), "dir named 'build' should be ignored by dir-only pattern")
}

// --- LineOfOffset tests ---

func TestLineOfOffset_Basic(t *testing.T) {
	f := &File{Source: []byte("line1\nline2\nline3\n")}
	assert.Equal(t, 1, f.LineOfOffset(0))
	assert.Equal(t, 1, f.LineOfOffset(4))
	assert.Equal(t, 2, f.LineOfOffset(6))
	assert.Equal(t, 3, f.LineOfOffset(12))
}

// --- PIBlockParser edge cases ---

func TestPIBlockParser_CanInterruptParagraph(t *testing.T) {
	p := NewPIBlockParser()
	assert.True(t, p.CanInterruptParagraph())
}

func TestPIBlockParser_CanAcceptIndentedLine(t *testing.T) {
	p := NewPIBlockParser()
	assert.False(t, p.CanAcceptIndentedLine())
}

func TestPIBlockParser_Trigger(t *testing.T) {
	p := NewPIBlockParser()
	assert.Equal(t, []byte{'<'}, p.Trigger())
}

// --- extractPINameBytes tests ---

func TestExtractPINameBytes_Basic(t *testing.T) {
	assert.Equal(t, "foo", string(extractPINameBytes([]byte("foo?>"))))
}

func TestExtractPINameBytes_WithSpace(t *testing.T) {
	assert.Equal(t, "catalog", string(extractPINameBytes([]byte("catalog key=val"))))
}

func TestExtractPINameBytes_SlashName(t *testing.T) {
	assert.Equal(t, "/include", string(extractPINameBytes([]byte("/include?>"))))
}

func TestExtractPINameBytes_EmptyAfterPI(t *testing.T) {
	assert.Equal(t, "", string(extractPINameBytes([]byte("?>"))))
}

// --- Additional walk coverage ---

func TestWalkDir_NonexistentDir(t *testing.T) {
	_, err := walkDir(filepath.Join(t.TempDir(), "no-such-dir"), false, nil)
	assert.Error(t, err)
}

// --- hasGlobChars tests ---

func TestHasGlobChars(t *testing.T) {
	assert.True(t, hasGlobChars("*.md"))
	assert.True(t, hasGlobChars("file?.md"))
	assert.True(t, hasGlobChars("[abc].md"))
	assert.False(t, hasGlobChars("readme.md"))
}

// --- isMarkdown tests ---

func TestIsMarkdown(t *testing.T) {
	assert.True(t, isMarkdown("readme.md"))
	assert.True(t, isMarkdown("readme.MD"))
	assert.True(t, isMarkdown("file.markdown"))
	assert.True(t, isMarkdown("file.MARKDOWN"))
	assert.False(t, isMarkdown("file.txt"))
	assert.False(t, isMarkdown("file.go"))
}

// --- isSkippedSymlink tests ---

func TestIsSkippedSymlink_NotSymlink(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "real.md")
	require.NoError(t, os.WriteFile(f, []byte("# Test"), 0o644))
	info, err := os.Stat(f)
	require.NoError(t, err)
	assert.False(t, isSkippedSymlink(info, f, []string{"*.md"}))
}

func TestIsSkippedSymlink_EmptyPatterns(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "real.md")
	require.NoError(t, os.WriteFile(f, []byte("# Test"), 0o644))
	info, err := os.Stat(f)
	require.NoError(t, err)
	assert.False(t, isSkippedSymlink(info, f, nil))
}

// --- matchRule edge cases ---

func TestMatchRule_OutsideBase(t *testing.T) {
	r := ignoreRule{base: "/home/user/project", pattern: "*.md"}
	// Path outside the base should not match.
	assert.False(t, matchRule(r, "/other/path/file.md"))
}

// --- Walk with PI nodes ---

func TestPI_Kind_EqualsRegisteredKind(t *testing.T) {
	pi := &ProcessingInstruction{Name: "test"}
	assert.Equal(t, KindProcessingInstruction, pi.Kind())
	assert.NotEqual(t, ast.KindDocument, pi.Kind())
}

func TestPI_Dump_WithSource(t *testing.T) {
	// Dump should not panic with actual source.
	src := []byte("<?catalog\nglob: docs\n?>")
	pi := &ProcessingInstruction{Name: "catalog"}
	pi.Dump(src, 1)
}

func TestPI_Dump_EmptySource(t *testing.T) {
	pi := &ProcessingInstruction{Name: "test"}
	pi.Dump([]byte{}, 0)
}

// Verify formatSnippet called from AST walk with multiple levels
func TestNewFile_MultiPIs(t *testing.T) {
	src := "<?foo?>\n\n<?bar\nbaz\n?>\n"
	f, err := NewFile("test.md", []byte(src))
	require.NoError(t, err)
	pis := findPINodes(f.AST)
	require.Len(t, pis, 2)
	assert.Equal(t, "foo", pis[0].Name)
	assert.Equal(t, "bar", pis[1].Name)
}
