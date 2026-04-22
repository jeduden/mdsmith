package duplicatedcontent

import (
	"errors"
	"io/fs"
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

// errFS wraps an fs.FS and returns err from ReadDir on a specific
// path, so buildCorpusIndex's WalkDir callback receives a non-nil
// error and takes the skip-but-continue branch.
type errFS struct {
	inner   fs.FS
	failOn  string
	failErr error
}

func (e errFS) Open(name string) (fs.File, error) { return e.inner.Open(name) }

func (e errFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == e.failOn {
		return nil, e.failErr
	}
	return fs.ReadDir(e.inner, name)
}

func TestCheck_CorpusWalkSwallowsFSErrors(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	p := longParagraph("the quick brown fox jumps over the lazy dog")
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n\n"+p+"\n")
	writeFile(t, filepath.Join(sub, "b.md"), "# B\n\n"+p+"\n")

	// Build a.md as the current file, then point RootFS at an FS
	// that errors on ReadDir("sub") so the walker is forced down
	// the err != nil branch for that entry.
	data, err := os.ReadFile(filepath.Join(dir, "a.md"))
	require.NoError(t, err)
	f, err := lint.NewFile(filepath.Join(dir, "a.md"), data)
	require.NoError(t, err)
	f.FS = os.DirFS(dir)
	f.RootDir = dir
	f.RootFS = errFS{
		inner:   os.DirFS(dir),
		failOn:  "sub",
		failErr: errors.New("forced walk error"),
	}

	// The rule must not panic or return the error; it silently
	// skips the unreadable subtree. b.md is in sub/ and therefore
	// not found, so no duplicate diagnostic fires.
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
	// Both diagnostics reference b.md; lines ascend from the first
	// duplicate at line 3 to the later duplicate at line 7.
	assert.Contains(t, diags[0].Message, "b.md:3")
	assert.Contains(t, diags[1].Message, "b.md:7")
}

func TestCheck_NoFSIsNoop(t *testing.T) {
	src := []byte("# A\n\n" + longParagraph("the quick brown fox") + "\n")
	f, err := lint.NewFile("a.md", src)
	require.NoError(t, err)
	// FS and RootFS intentionally left nil.
	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_MirrorsFrontMatterModeForCorpusFiles(t *testing.T) {
	// When the engine runs with `front-matter: false`, current-file
	// paragraph line numbers are raw-source coordinates (no offset
	// was added). Corpus files must be parsed in the same mode so
	// the {other}:{line} part of the diagnostic is also in raw
	// coordinates. Without plumbing StripFrontMatter through, the
	// corpus walk would always strip and over-add LineOffset to the
	// reported line, producing an off-by-N bug for files with front
	// matter.
	dir := t.TempDir()
	p := longParagraph("the quick brown fox jumps over the lazy dog")
	fm := "---\ntitle: B\n---\n"
	// b.md carries front matter whose stripping would add 3 to
	// paragraph lines. The paragraph sits at raw line 6.
	writeFile(t, filepath.Join(dir, "b.md"), fm+"\n# B\n\n"+p+"\n")
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n\n"+p+"\n")

	data, err := os.ReadFile(filepath.Join(dir, "a.md"))
	require.NoError(t, err)
	// Parse a.md with stripFrontMatter=false, matching a
	// `front-matter: false` Runner.
	f, err := lint.NewFileFromSource(filepath.Join(dir, "a.md"), data, false)
	require.NoError(t, err)
	f.FS = os.DirFS(dir)
	f.SetRootDir(dir)

	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	// b.md's paragraph sits at raw line 7: front matter lines
	// 1-3, blank line 4, heading line 5 ("# B"), blank line 6,
	// paragraph line 7.
	assert.Contains(t, diags[0].Message, "b.md:7",
		"corpus line must stay in raw-source coords when front-matter stripping is disabled")
}

func TestCheck_StdinWithRootDirDoesNotWalkProject(t *testing.T) {
	// Mirrors `mdsmith check -` (or Runner.RunSource) under a
	// discovered project root: f.FS is nil because the input has
	// no directory context, but RootFS/RootDir get populated for
	// rules that look up project-relative resources. MDS037 is a
	// cross-file rule that cannot meaningfully run against stdin,
	// so it must short-circuit on f.FS == nil instead of silently
	// walking the entire RootFS.
	dir := t.TempDir()
	p := longParagraph("the quick brown fox jumps over the lazy dog")
	// A duplicate sibling in the root would normally fire, but
	// with no FS the rule must emit nothing.
	writeFile(t, filepath.Join(dir, "sibling.md"), "# S\n\n"+p+"\n")

	src := []byte("# Stdin\n\n" + p + "\n")
	f, err := lint.NewFile("stdin.md", src)
	require.NoError(t, err)
	// FS intentionally left nil (stdin); RootFS populated as
	// RunSource would do.
	f.SetRootDir(dir)

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
	assert.Contains(t, diags[0].Message, "include:",
		"diagnostic must name the offending setting list")
}

func TestCheck_ConfigDiagOnBadExcludeGlob(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n\n"+longParagraph("xyz")+"\n")

	f := newLintFileWithRoot(t, filepath.Join(dir, "a.md"), dir)
	r := &Rule{Exclude: []string{"[invalid"}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, lint.Error, diags[0].Severity)
	assert.Contains(t, diags[0].Message, "exclude:",
		"diagnostic must name the offending setting list")
}

func TestCheck_RootFSWithRelativeFilePath(t *testing.T) {
	// Mirrors a normal CLI run: RootDir is absolute (from config
	// discovery) while f.Path is the relative path returned by
	// ResolveFiles (e.g. "./docs/a.md"). resolveCorpus must use the
	// RootFS walk rather than silently falling back to the file's
	// own directory and missing duplicates elsewhere in the tree.
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	guides := filepath.Join(dir, "guides")
	require.NoError(t, os.MkdirAll(docs, 0o755))
	require.NoError(t, os.MkdirAll(guides, 0o755))

	p := longParagraph("the quick brown fox jumps over the lazy dog")
	writeFile(t, filepath.Join(docs, "a.md"), "# A\n\n"+p+"\n")
	writeFile(t, filepath.Join(guides, "b.md"), "# B\n\n"+p+"\n")

	// Run the test from dir so a RootDir-relative f.Path resolves
	// correctly via filepath.Abs inside rootRelative.
	t.Chdir(dir)

	data, err := os.ReadFile(filepath.Join(dir, "docs", "a.md"))
	require.NoError(t, err)
	// Relative path, as ResolveFiles would return.
	relPath := filepath.Join("docs", "a.md")
	f, err := lint.NewFile(relPath, data)
	require.NoError(t, err)
	f.FS = os.DirFS(docs)
	f.SetRootDir(dir)

	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "guides/b.md",
		"RootFS walk should have found the duplicate in a different directory")
}

func TestCheck_CWDIsSubdirOfRootDir(t *testing.T) {
	// Running `mdsmith check a.md` from inside docs/ with a
	// discovered RootDir at the project root: f.Path = "a.md"
	// (CWD-relative, not RootDir-relative). rootRelative must
	// compute docs/a.md via filepath.Abs before filepath.Rel,
	// otherwise the corpus walk would not recognize the current
	// file as self and would report a paragraph as duplicated in
	// itself.
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	require.NoError(t, os.MkdirAll(docs, 0o755))

	p := longParagraph("the quick brown fox jumps over the lazy dog")
	writeFile(t, filepath.Join(docs, "a.md"), "# A\n\n"+p+"\n")

	t.Chdir(docs)

	data, err := os.ReadFile("a.md")
	require.NoError(t, err)
	f, err := lint.NewFile("a.md", data)
	require.NoError(t, err)
	f.FS = os.DirFS(docs)
	f.SetRootDir(dir)

	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags,
		"rule must not flag a file as a duplicate of itself when CWD is a subdir of RootDir")
}

func TestCheck_RootFSRejectsPathEscapingRoot(t *testing.T) {
	// A file whose Path sits outside RootDir (via "../" traversal) must
	// not scan the entire RootFS; resolveCorpus falls through to FS so
	// the walk stays local.
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	outside := filepath.Join(dir, "outside")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))

	p := longParagraph("the quick brown fox jumps over the lazy dog")
	// File under a nested RootDir, but its recorded path escapes via
	// "../outside/dup.md" — which filepath.Rel would also flag.
	writeFile(t, filepath.Join(outside, "dup.md"), "# Dup\n\n"+p+"\n")
	writeFile(t, filepath.Join(sub, "peer.md"), "# Peer\n\n"+p+"\n")

	data, err := os.ReadFile(filepath.Join(outside, "dup.md"))
	require.NoError(t, err)
	// Relative escape path: "../outside/dup.md" against RootDir=sub.
	f, err := lint.NewFile(filepath.Join("..", "outside", "dup.md"), data)
	require.NoError(t, err)
	f.FS = os.DirFS(outside)
	f.SetRootDir(sub)

	diags := (&Rule{}).Check(f)
	// peer.md is under sub/ which is no longer in scope; FS (=outside)
	// only holds dup.md itself. No duplicates reported.
	assert.Empty(t, diags)
}

func TestCheck_DetectsDuplicatesInDotMarkdownFiles(t *testing.T) {
	// The linter's file discovery accepts both .md and .markdown;
	// the rule's corpus walk must also see .markdown siblings so
	// they are not silently excluded from duplicate detection.
	dir := t.TempDir()
	p := longParagraph("the quick brown fox jumps over the lazy dog")
	writeFile(t, filepath.Join(dir, "a.markdown"), "# A\n\n"+p+"\n")
	writeFile(t, filepath.Join(dir, "b.markdown"), "# B\n\n"+p+"\n")

	f := newLintFileWithRoot(t, filepath.Join(dir, "a.markdown"), dir)
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "b.markdown")
}

func TestCheck_PrunesGitAndNodeModulesUnconditionally(t *testing.T) {
	// .git and node_modules hold no relevant Markdown and blow up the
	// walk; the rule must skip them without any exclude config.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "node_modules"), 0o755))

	p := longParagraph("the quick brown fox jumps over the lazy dog")
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n\n"+p+"\n")
	writeFile(t, filepath.Join(dir, ".git", "HEAD.md"), "# Git\n\n"+p+"\n")
	writeFile(t, filepath.Join(dir, "node_modules", "dup.md"), "# Mod\n\n"+p+"\n")

	f := newLintFileWithRoot(t, filepath.Join(dir, "a.md"), dir)
	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags,
		"duplicates under .git/ and node_modules/ must be pruned by default")
}

func TestCheck_ExcludeSubtreePattern_PrunesWalk(t *testing.T) {
	// The README example uses patterns like "docs/generated/**"
	// for pruning generated directory subtrees. fs.WalkDir yields
	// the directory path without a trailing slash ("docs/generated"),
	// so the raw glob does not match; shouldSkipDir must also try a
	// trailing-slash form so the pattern fires at the directory
	// boundary and the subtree is skipped.
	dir := t.TempDir()
	generated := filepath.Join(dir, "docs", "generated")
	require.NoError(t, os.MkdirAll(generated, 0o755))

	p := longParagraph("the quick brown fox jumps over the lazy dog")
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n\n"+p+"\n")
	writeFile(t, filepath.Join(generated, "dup.md"), "# Gen\n\n"+p+"\n")

	f := newLintFileWithRoot(t, filepath.Join(dir, "a.md"), dir)
	r := &Rule{Exclude: []string{"docs/generated/**"}}
	diags := r.Check(f)
	assert.Empty(t, diags,
		"subtree exclude pattern must prune docs/generated at the directory boundary")
}

func TestCheck_ExcludeDirectoryPattern_PrunesWalk(t *testing.T) {
	// Exclude patterns that match a directory must prune the walk
	// with fs.SkipDir so large trees like .git/ or vendor/ are not
	// traversed on every check. Verified indirectly: an unreadable
	// file inside the excluded subtree would otherwise cause the
	// walker to surface an error; here we use a duplicate that
	// would normally fire but must not be reached.
	dir := t.TempDir()
	vendor := filepath.Join(dir, "vendor")
	require.NoError(t, os.MkdirAll(vendor, 0o755))

	p := longParagraph("the quick brown fox jumps over the lazy dog")
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n\n"+p+"\n")
	writeFile(t, filepath.Join(vendor, "b.md"), "# B\n\n"+p+"\n")

	f := newLintFileWithRoot(t, filepath.Join(dir, "a.md"), dir)
	r := &Rule{Exclude: []string{"vendor"}}
	diags := r.Check(f)
	assert.Empty(t, diags,
		"excluded directory 'vendor' must prune the walk, not just filter its files")
}

func TestCheck_BasenameExcludePatternMatchesAcrossDirs(t *testing.T) {
	// Consistent with MDS027: a basename pattern ("draft.md") excludes
	// the file regardless of which directory the walker finds it in.
	dir := t.TempDir()
	sub := filepath.Join(dir, "nested")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	p := longParagraph("the quick brown fox jumps over the lazy dog")
	writeFile(t, filepath.Join(dir, "a.md"), "# A\n\n"+p+"\n")
	writeFile(t, filepath.Join(sub, "draft.md"), "# Draft\n\n"+p+"\n")

	f := newLintFileWithRoot(t, filepath.Join(dir, "a.md"), dir)
	r := &Rule{Exclude: []string{"draft.md"}}
	diags := r.Check(f)
	assert.Empty(t, diags,
		"basename-only exclude pattern should hide nested/draft.md")
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

func TestApplySettings_TruncatesFractionalFloat(t *testing.T) {
	// settings.ToInt truncates toward zero, matching the rest of the
	// codebase, so 1.5 becomes 1 rather than being rejected.
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"min-chars": 1.5}))
	assert.Equal(t, 1, r.MinChars)
}

func TestApplySettings_RejectsZeroMinChars(t *testing.T) {
	// Check treats MinChars == 0 as unset and falls back to the
	// default, so an explicit 0 in config would be silently ignored;
	// ApplySettings must reject it rather than letting it pass.
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"min-chars": 0})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "min-chars must be > 0")
}

func TestRootRelative_RelErrorOnRelativeRoot(t *testing.T) {
	// filepath.Rel cannot compute a relative path when one argument
	// is absolute and the other is relative (same logic fires on
	// Windows cross-volume paths). rootRelative must surface that
	// as ok=false so resolveCorpus falls through to f.FS scope.
	got, ok := rootRelative("relative_root", "/abs/path")
	assert.False(t, ok)
	assert.Empty(t, got)
}

func TestCheck_SkipsIncludeGeneratedSection(t *testing.T) {
	// A paragraph inside an <?include?> generated section must not be
	// flagged as a duplicate, even when the same text appears in
	// another corpus file (it's the source of the inclusion).
	dir := t.TempDir()
	p := longParagraph("the quick brown fox jumps over the lazy dog")

	// source.md contains the paragraph that gets included.
	writeFile(t, filepath.Join(dir, "source.md"), "# Source\n\n"+p+"\n")

	// host.md includes source.md; the generated body holds the same paragraph.
	host := "# Host\n\n" +
		"<?include\nfile: source.md\n?>\n" +
		p + "\n" +
		"<?/include?>\n"
	writeFile(t, filepath.Join(dir, "host.md"), host)

	f := newLintFileWithRoot(t, filepath.Join(dir, "host.md"), dir)
	diags := (&Rule{MinChars: 10}).Check(f)
	assert.Empty(t, diags,
		"paragraph inside <?include?> body must not be flagged as a duplicate")
}

func TestCheck_SkipsCatalogGeneratedSection(t *testing.T) {
	// A paragraph inside a <?catalog?> generated section must not be
	// flagged even when the same text appears in another corpus file.
	dir := t.TempDir()
	p := longParagraph("the quick brown fox jumps over the lazy dog")

	writeFile(t, filepath.Join(dir, "source.md"), "# Source\n\n"+p+"\n")

	host := "# Host\n\n" +
		"<?catalog\nglob: \"*.md\"\n?>\n" +
		p + "\n" +
		"<?/catalog?>\n"
	writeFile(t, filepath.Join(dir, "host.md"), host)

	f := newLintFileWithRoot(t, filepath.Join(dir, "host.md"), dir)
	diags := (&Rule{MinChars: 10}).Check(f)
	assert.Empty(t, diags,
		"paragraph inside <?catalog?> body must not be flagged as a duplicate")
}

func TestCheck_DuplicateOutsideGeneratedSectionStillFires(t *testing.T) {
	// A paragraph outside any generated section must still be flagged
	// when it appears verbatim in another corpus file.
	dir := t.TempDir()
	p := longParagraph("the quick brown fox jumps over the lazy dog")

	// both.md has the paragraph before the generated section.
	both := "# Both\n\n" +
		p + "\n\n" +
		"<?include\nfile: source.md\n?>\n" +
		"generated content goes here and is definitely not the same\n" +
		"<?/include?>\n"
	writeFile(t, filepath.Join(dir, "both.md"), both)
	writeFile(t, filepath.Join(dir, "other.md"), "# Other\n\n"+p+"\n")

	f := newLintFileWithRoot(t, filepath.Join(dir, "both.md"), dir)
	diags := (&Rule{MinChars: 10}).Check(f)
	require.Len(t, diags, 1,
		"real duplicate outside generated section must still fire")
	assert.Contains(t, diags[0].Message, "other.md")
}

func TestCheck_CorpusSkipsIncludeGeneratedSection(t *testing.T) {
	// When a corpus file contains an <?include?> generated section,
	// the paragraphs inside it must not be indexed. Otherwise a host
	// file checking its own (non-generated) paragraph against the
	// corpus would find a false match in the corpus file's generated body.
	dir := t.TempDir()
	p := longParagraph("the quick brown fox jumps over the lazy dog")

	// corpus-host.md has the same paragraph inside a generated section.
	corpusHost := "# CorpusHost\n\n" +
		"<?include\nfile: source.md\n?>\n" +
		p + "\n" +
		"<?/include?>\n"
	writeFile(t, filepath.Join(dir, "corpus-host.md"), corpusHost)

	// current.md has the paragraph as real content.
	writeFile(t, filepath.Join(dir, "current.md"), "# Current\n\n"+p+"\n")

	f := newLintFileWithRoot(t, filepath.Join(dir, "current.md"), dir)
	diags := (&Rule{MinChars: 10}).Check(f)
	assert.Empty(t, diags,
		"paragraph inside corpus file's generated section must not be indexed")
}

func TestGeneratedRanges_EmptyFile(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("# Hello\n"))
	require.NoError(t, err)
	assert.Empty(t, generatedRanges(f))
}

func TestGeneratedRanges_SingleIncludePair(t *testing.T) {
	src := "<?include\nfile: x.md\n?>\nsome content\n<?/include?>\n"
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	ranges := generatedRanges(f)
	require.Len(t, ranges, 1)
	// Range must cover "some content\n" but not the PI markers.
	contentStart := strings.Index(src, "some content")
	contentEnd := strings.Index(src, "<?/include?>")
	assert.Equal(t, contentStart, ranges[0][0])
	assert.Equal(t, contentEnd, ranges[0][1])
}

func TestGeneratedRanges_MultiplePairs(t *testing.T) {
	src := "<?include\nfile: a.md\n?>\ncontent a\n<?/include?>\n" +
		"<?catalog\nglob: \"*.md\"\n?>\ncontent b\n<?/catalog?>\n"
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	ranges := generatedRanges(f)
	assert.Len(t, ranges, 2)
}

func TestGeneratedRanges_NestedSameNamePair(t *testing.T) {
	// An inner <?include?> inside an outer <?include?> body must not
	// prematurely close the outer range. The outer range must span all
	// content between the outer open and outer close markers.
	src := "<?include\nfile: outer.md\n?>\n" +
		"before inner\n" +
		"<?include\nfile: inner.md\n?>\n" +
		"inner content\n" +
		"<?/include?>\n" +
		"after inner\n" +
		"<?/include?>\n"
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	ranges := generatedRanges(f)
	require.Len(t, ranges, 1, "nested pair must produce exactly one outer range")
	// The range must cover everything from after the outer open to before
	// the outer close, so both "before inner" and "after inner" are inside it.
	beforeInnerOffset := strings.Index(src, "before inner")
	afterInnerOffset := strings.Index(src, "after inner")
	outerCloseOffset := strings.Index(src, "<?/include?>\n")
	// Find the *last* <?/include?> — that's the outer close.
	lastCloseOffset := strings.LastIndex(src, "<?/include?>")
	assert.LessOrEqual(t, ranges[0][0], beforeInnerOffset,
		"range start must be before 'before inner'")
	assert.Greater(t, ranges[0][1], afterInnerOffset,
		"range end must be after 'after inner'")
	assert.Equal(t, lastCloseOffset, ranges[0][1],
		"range end must point to the outer <?/include?> marker")
	_ = outerCloseOffset // suppress unused warning
}

func TestCheck_SkipsNestedIncludeGeneratedSection(t *testing.T) {
	// When the outer generated body contains a nested <?include?> pair,
	// paragraphs appearing AFTER the inner pair (but still inside the
	// outer body) must also be skipped.
	dir := t.TempDir()
	p := longParagraph("the quick brown fox jumps over the lazy dog")

	writeFile(t, filepath.Join(dir, "source.md"), "# Source\n\n"+p+"\n")

	host := "# Host\n\n" +
		"<?include\nfile: outer.md\n?>\n" +
		"before inner paragraph is filler content not a real paragraph\n\n" +
		"<?include\nfile: inner.md\n?>\n" +
		p + "\n" +
		"<?/include?>\n" +
		p + "\n" +
		"<?/include?>\n"
	writeFile(t, filepath.Join(dir, "host.md"), host)

	f := newLintFileWithRoot(t, filepath.Join(dir, "host.md"), dir)
	diags := (&Rule{MinChars: 10}).Check(f)
	assert.Empty(t, diags,
		"paragraph after inner nested <?include?> but inside outer must not be flagged")
}

func TestGeneratedRanges_NilAST(t *testing.T) {
	f := &lint.File{}
	assert.Empty(t, generatedRanges(f))
}

func TestRootRelative_AbsErrorWhenCWDIsRemoved(t *testing.T) {
	// filepath.Abs errors when os.Getwd fails, which happens when
	// the process's current directory was removed underneath it.
	// Exercise that path to pin the behavior: rootRelative returns
	// ok=false instead of producing a garbage selfName.
	dir, err := os.MkdirTemp("", "mds037-abs-err-")
	require.NoError(t, err)
	t.Chdir(dir)
	require.NoError(t, os.Remove(dir))

	got, ok := rootRelative("/some/root", "a.md")
	assert.False(t, ok)
	assert.Empty(t, got)
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
