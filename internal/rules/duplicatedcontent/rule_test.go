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
	assert.Equal(t, "meta", r.Category())
}

func TestRuleRegistered(t *testing.T) {
	r := rule.ByID("MDS037")
	assert.NotNil(t, r, "MDS037 must be registered via init()")
}

func TestEnabledByDefault_False(t *testing.T) {
	r := &Rule{}
	assert.False(t, r.EnabledByDefault(),
		"duplicated-content is opt-in; default-enabled would flag shared agent-config prose")
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

func TestCheck_NilASTIsNoop(t *testing.T) {
	// An uninitialized File (no parse) must not panic.
	f := &lint.File{Path: "x.md"}
	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_OversizeCorpusFileSkipped(t *testing.T) {
	dir := t.TempDir()
	p := longParagraph("the quick brown fox jumps over the lazy dog")
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n\n"+p+"\n")
	writeFile(t, filepath.Join(dir, "b.md"), "# B\n\n"+p+"\n")

	f := newLintFileWithRoot(t, filepath.Join(dir, "a.md"), dir)
	// MaxInputBytes forces lint.ReadFSFileLimited to reject b.md, so the
	// walker silently skips it and no duplicate is reported.
	f.MaxInputBytes = 1

	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_SameFileMultipleMatchesSortedByLine(t *testing.T) {
	dir := t.TempDir()
	p := longParagraph("the quick brown fox jumps over the lazy dog")
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n\n"+p+"\n")
	// Same paragraph twice in b.md at two different lines.
	writeFile(t, filepath.Join(dir, "b.md"),
		"# B\n\n"+p+"\n\nunrelated content goes here to separate blocks\n\n"+p+"\n")

	f := newLintFileWithRoot(t, filepath.Join(dir, "a.md"), dir)
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 2)
	// Both diagnostics reference b.md; lines ascend.
	assert.Contains(t, diags[0].Message, "b.md:3")
	assert.Contains(t, diags[1].Message, "b.md:")
	assert.NotEqual(t, diags[0].Message, diags[1].Message)
}

func TestCheck_NoFSIsNoop(t *testing.T) {
	src := []byte("# A\n\n" + longParagraph("the quick brown fox") + "\n")
	f, err := lint.NewFile("a.md", src)
	require.NoError(t, err)
	// FS and RootFS intentionally left nil.
	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestApplySettings_MinChars(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"min-chars": 50}))
	assert.Equal(t, 50, r.MinChars)
}

func TestApplySettings_IncludeExclude(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"include": []any{"docs/**"},
		"exclude": []any{"**/draft.md"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"docs/**"}, r.Include)
	assert.Equal(t, []string{"**/draft.md"}, r.Exclude)
}

func TestApplySettings_RejectsUnknownKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknown": 1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown setting")
}

func TestApplySettings_RejectsBadTypes(t *testing.T) {
	r := &Rule{}
	require.Error(t, r.ApplySettings(map[string]any{"min-chars": "oops"}))
	require.Error(t, r.ApplySettings(map[string]any{"min-chars": -1}))
	require.Error(t, r.ApplySettings(map[string]any{"include": "not-a-list"}))
}

func TestApplySettings_RejectsBadGlob(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"include": []any{"[invalid"}})
	require.Error(t, err)
}

func TestDefaultSettings_HasMinChars(t *testing.T) {
	d := (&Rule{}).DefaultSettings()
	assert.Equal(t, defaultMinChars, d["min-chars"])
	assert.Contains(t, d, "include")
	assert.Contains(t, d, "exclude")
}

func TestCheck_ConfigDiagOnBadGlob(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n\n"+longParagraph("xyz")+"\n")

	f := newLintFileWithRoot(t, filepath.Join(dir, "a.md"), dir)
	r := &Rule{Include: []string{"[invalid"}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, lint.Error, diags[0].Severity)
	assert.Contains(t, diags[0].Message, "duplicated-content")
}

func TestCheck_ConfigDiagOnBadExcludeGlob(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n\n"+longParagraph("xyz")+"\n")

	f := newLintFileWithRoot(t, filepath.Join(dir, "a.md"), dir)
	r := &Rule{Exclude: []string{"[invalid"}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, lint.Error, diags[0].Severity)
}

func TestCheck_FallsBackToFSWhenRootFSMissing(t *testing.T) {
	dir := t.TempDir()
	p := longParagraph("the quick brown fox jumps over the lazy dog")
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n\n"+p+"\n")
	writeFile(t, filepath.Join(dir, "b.md"), "# B\n\n"+p+"\n")

	data, err := os.ReadFile(filepath.Join(dir, "a.md"))
	require.NoError(t, err)
	f, err := lint.NewFile(filepath.Join(dir, "a.md"), data)
	require.NoError(t, err)
	f.FS = os.DirFS(dir)
	// RootFS intentionally left nil so resolveCorpus falls back to FS.

	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "b.md")
}

func TestCheck_CorpusSkipsUnparseableFiles(t *testing.T) {
	dir := t.TempDir()
	p := longParagraph("the quick brown fox jumps over the lazy dog")
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n\n"+p+"\n")
	// Non-markdown file in the corpus — must be ignored by the walker.
	writeFile(t, filepath.Join(dir, "readme.txt"), p)

	f := newLintFileWithRoot(t, filepath.Join(dir, "a.md"), dir)
	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_MultipleMatchesSortDeterministically(t *testing.T) {
	dir := t.TempDir()
	p := longParagraph("the quick brown fox jumps over the lazy dog")
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n\n"+p+"\n")
	writeFile(t, filepath.Join(dir, "b.md"), "# B\n\n"+p+"\n")
	writeFile(t, filepath.Join(dir, "c.md"), "# C\n\n"+p+"\n")

	f := newLintFileWithRoot(t, filepath.Join(dir, "a.md"), dir)
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 2)
	assert.Contains(t, diags[0].Message, "b.md")
	assert.Contains(t, diags[1].Message, "c.md")
}

func TestApplySettings_RejectsBadExcludeType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"exclude": "not-a-list"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exclude")
}

func TestApplySettings_RejectsBadExcludeGlob(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"exclude": []any{"[invalid"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exclude")
}

func TestApplySettings_AcceptsConcreteStringSlice(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"include": []string{"docs/**"},
	}))
	assert.Equal(t, []string{"docs/**"}, r.Include)
}

func TestApplySettings_RejectsStringInsideAnySlice(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"include": []any{"ok", 42},
	})
	require.Error(t, err)
}

func TestApplySettings_AcceptsIntegerTypesForMinChars(t *testing.T) {
	for _, v := range []any{int(50), int64(50), float64(50)} {
		r := &Rule{}
		require.NoError(t, r.ApplySettings(map[string]any{"min-chars": v}),
			"value %v (%T) should be accepted", v, v)
		assert.Equal(t, 50, r.MinChars)
	}
}

func TestApplySettings_RejectsFractionalFloat(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"min-chars": 1.5})
	require.Error(t, err)
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
