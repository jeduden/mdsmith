package duplicatedcontent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuleIdentity(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS037", r.ID())
	assert.Equal(t, "duplicated-content", r.Name())
	assert.Equal(t, "content", r.Category())
}

func TestRuleRegistered(t *testing.T) {
	r := rule.ByID("MDS037")
	assert.NotNil(t, r, "MDS037 must be registered via init()")
}

func longParagraph(seed string) string {
	return strings.Repeat(seed+" ", 12)
}

func TestCheck_DetectsDuplicateAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	p := longParagraph("the quick brown fox jumps over the lazy dog")
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n\n"+p+"\n")
	writeFile(t, filepath.Join(dir, "b.md"), "# B\n\n"+p+"\n")

	f := newLintFileWithRoot(t, filepath.Join(dir, "a.md"), dir)

	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "b.md")
	assert.Equal(t, "MDS037", diags[0].RuleID)
	assert.Equal(t, lint.Warning, diags[0].Severity)
}

func TestCheck_IgnoresShortParagraphs(t *testing.T) {
	dir := t.TempDir()
	p := "Short paragraph under the min-chars threshold."
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n\n"+p+"\n")
	writeFile(t, filepath.Join(dir, "b.md"), "# B\n\n"+p+"\n")

	f := newLintFileWithRoot(t, filepath.Join(dir, "a.md"), dir)

	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_IgnoresUniqueParagraphs(t *testing.T) {
	dir := t.TempDir()
	p1 := longParagraph("unique paragraph one with enough length")
	p2 := longParagraph("different paragraph two with enough length")
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n\n"+p1+"\n")
	writeFile(t, filepath.Join(dir, "b.md"), "# B\n\n"+p2+"\n")

	f := newLintFileWithRoot(t, filepath.Join(dir, "a.md"), dir)

	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_NormalizesWhitespaceAndCase(t *testing.T) {
	dir := t.TempDir()
	p := longParagraph("the quick brown fox jumps over the lazy dog")
	// Same paragraph, different case and reflowed with extra spaces.
	p2 := strings.ReplaceAll(strings.ToUpper(p), " ", "   ")
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n\n"+p+"\n")
	writeFile(t, filepath.Join(dir, "b.md"), "# B\n\n"+p2+"\n")

	f := newLintFileWithRoot(t, filepath.Join(dir, "a.md"), dir)

	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "b.md")
}

func TestCheck_ReportsLineOfDuplicateInSelf(t *testing.T) {
	dir := t.TempDir()
	p := longParagraph("the quick brown fox jumps over the lazy dog")
	writeFile(t, filepath.Join(dir, "a.md"),
		"# A\n\nintro paragraph that is also quite long but unique "+
			strings.Repeat("really unique content ", 10)+"\n\n"+p+"\n")
	writeFile(t, filepath.Join(dir, "b.md"), "# B\n\n"+p+"\n")

	f := newLintFileWithRoot(t, filepath.Join(dir, "a.md"), dir)

	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	// Duplicate is at the 3rd paragraph block (line 5 in a.md).
	assert.Equal(t, 5, diags[0].Line)
}

func TestCheck_SkipsSelfFile(t *testing.T) {
	dir := t.TempDir()
	p := longParagraph("the quick brown fox jumps over the lazy dog")
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n\n"+p+"\n\n"+p+"\n")

	f := newLintFileWithRoot(t, filepath.Join(dir, "a.md"), dir)

	diags := (&Rule{}).Check(f)
	// An identical paragraph within the same file is not a cross-file
	// duplicate — this rule only flags matches in other files.
	assert.Empty(t, diags)
}

func TestCheck_HonorsExcludePattern(t *testing.T) {
	dir := t.TempDir()
	p := longParagraph("the quick brown fox jumps over the lazy dog")
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n\n"+p+"\n")
	writeFile(t, filepath.Join(dir, "ignored.md"), "# Ignored\n\n"+p+"\n")

	f := newLintFileWithRoot(t, filepath.Join(dir, "a.md"), dir)

	r := &Rule{Exclude: []string{"ignored.md"}}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_HonorsIncludePattern(t *testing.T) {
	dir := t.TempDir()
	p := longParagraph("the quick brown fox jumps over the lazy dog")
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n\n"+p+"\n")
	writeFile(t, filepath.Join(dir, "scoped.md"), "# Scoped\n\n"+p+"\n")
	writeFile(t, filepath.Join(dir, "other.md"), "# Other\n\n"+p+"\n")

	f := newLintFileWithRoot(t, filepath.Join(dir, "a.md"), dir)

	r := &Rule{Include: []string{"scoped.md"}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "scoped.md")
}

func TestCheck_NoFSIsNoop(t *testing.T) {
	src := []byte("# A\n\n" + longParagraph("the quick brown fox") + "\n")
	f, err := lint.NewFile("a.md", src)
	require.NoError(t, err)
	// FS and RootFS intentionally left nil.
	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func newLintFileWithRoot(t *testing.T, path, root string) *lint.File {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	f, err := lint.NewFile(path, data)
	require.NoError(t, err)
	f.FS = os.DirFS(filepath.Dir(path))
	f.SetRootDir(root)
	return f
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}
