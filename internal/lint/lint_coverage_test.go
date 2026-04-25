package lint

import (
	"os"
	"path/filepath"
	"runtime"
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
	// Use a unique extension unlikely to appear in any ancestor .gitignore.
	path := filepath.Join(dir, t.Name()+"-not-ignored.mdsmith-test-unique")
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

func TestNewGitignoreMatcher_UnreadableGitignore(t *testing.T) {
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

	// NewGitignoreMatcher should not panic; it silently skips unreadable files.
	m := NewGitignoreMatcher(dir)
	require.NotNil(t, m)

	// Rules from the readable root .gitignore should still be active.
	logFile := filepath.Join(dir, "test.log")
	assert.True(t, m.IsIgnored(logFile, false), "*.log rule from root .gitignore should still apply")
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
	_, err := walkDir(filepath.Join(t.TempDir(), "no-such-dir"), false, false)
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

// --- SetRootDir tests ---

// TestSetRootDir_SetsRootDirAndRootFS verifies that SetRootDir sets both the
// RootDir string and initialises RootFS as an os.DirFS rooted at that directory.
func TestSetRootDir_SetsRootDirAndRootFS(t *testing.T) {
	dir := t.TempDir()
	f := &File{}
	f.SetRootDir(dir)

	assert.Equal(t, dir, f.RootDir)
	require.NotNil(t, f.RootFS, "RootFS must be non-nil after SetRootDir")

	// Write a sentinel file and verify RootFS can open it, which confirms
	// that RootFS is rooted at dir and not at some other location.
	sentinel := "sentinel.md"
	require.NoError(t, os.WriteFile(filepath.Join(dir, sentinel), []byte("# hi"), 0o644))
	fh, err := f.RootFS.Open(sentinel)
	require.NoError(t, err, "RootFS must be able to open files under dir")
	_ = fh.Close()
}

// --- NewGitignoreMatcher ancestor .gitignore tests ---

// TestNewGitignoreMatcher_AncestorGitignore verifies that rules from a
// .gitignore file in an ancestor directory (above the root passed to
// NewGitignoreMatcher) are collected and applied.
func TestNewGitignoreMatcher_AncestorGitignore(t *testing.T) {
	// Build: /parent/.gitignore (contains *.ancestor)
	//        /parent/child/           <- root passed to NewGitignoreMatcher
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	require.NoError(t, os.MkdirAll(child, 0o755))

	ancestorGitignore := filepath.Join(parent, ".gitignore")
	require.NoError(t, os.WriteFile(ancestorGitignore, []byte("*.ancestor\n"), 0o644))

	m := NewGitignoreMatcher(child)
	require.NotNil(t, m)

	// A file in the child dir that matches the ancestor pattern should be ignored.
	matchedFile := filepath.Join(child, "test.ancestor")
	assert.True(t, m.IsIgnored(matchedFile, false),
		"file matching an ancestor .gitignore pattern must be ignored")

	// A file that does not match must not be ignored.
	otherFile := filepath.Join(child, "test.md")
	assert.False(t, m.IsIgnored(otherFile, false),
		"file not matching the ancestor pattern must not be ignored")
}

// --- addDirFiles error path ---

// TestAddDirFiles_NonexistentDir exercises the error return path of addDirFiles
// when the target directory does not exist.
func TestAddDirFiles_NonexistentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	err := addDirFiles(dir, DefaultResolveOpts(), func(_ string) {})
	assert.Error(t, err, "addDirFiles must propagate walkDir errors")
}

// --- isDescendantOf unreachable-Rel branch (via filepath.Rel on Windows
//     volume-root mismatch; on POSIX we use the closest equivalent) ---

// TestIsDescendantOf_RelError exercises the filepath.Rel-error branch of
// isDescendantOf. filepath.Rel returns an error when the two paths reside on
// different Windows drive letters; on POSIX the function never returns an
// error for absolute paths, so we verify the happy-path variant that hits the
// same coverage statement via the `rel == "."` guard (already covered by the
// existing SelfReturnsFalse test).  This additional test strengthens the
// assertion that sibling paths are correctly classified.
func TestIsDescendantOf_Sibling(t *testing.T) {
	assert.False(t, isDescendantOf("/a/sibling", "/a/other"),
		"sibling directory must not be a descendant")
	assert.True(t, isDescendantOf("/a/other/child", "/a/other"),
		"child must be a descendant of parent")
}

// --- isGitignored: Abs-error branch ---
// filepath.Abs virtually never returns an error on POSIX (only in the
// unreachable os.Getwd failure scenario), so we verify the positive/negative
// paths that are reachable without simulating a Getwd failure.
func TestIsGitignored_DirectorySentinel(t *testing.T) {
	dir := t.TempDir()
	gitignorePath := filepath.Join(dir, ".gitignore")
	require.NoError(t, os.WriteFile(gitignorePath, []byte("logs/\n"), 0o644))

	m := NewGitignoreMatcher(dir)
	logsDir := filepath.Join(dir, "logs")
	// isGitignored for a directory entry.
	assert.True(t, isGitignored(m, logsDir, true),
		"directory matching dir-only pattern must be ignored")
	assert.False(t, isGitignored(m, logsDir, false),
		"file with same name must not be ignored by dir-only pattern")
}
