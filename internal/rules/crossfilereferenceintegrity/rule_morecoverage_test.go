package crossfilereferenceintegrity

import (
	"os"
	"path/filepath"
	"testing"

	gast "github.com/yuin/goldmark/ast"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// --- resolveAbsRoot: Abs() fails branch (hard to trigger, but we can cover
//     the EvalSymlinks-succeeds path with a non-existent path that Abs handles)

// --- anchorsForFile: lint.NewFileFromSource error ---

// TestAnchorsForFile_ParseError exercises the error path in anchorsForFile
// when lint.NewFileFromSource returns an error (invalid content is rare but
// the function is exercised by using empty data that parse succeeds on, and
// then via bad content). Actually lint.NewFileFromSource rarely errors.
// Instead we cover collectHeadingAnchors's slug == "" branch.

func TestCollectHeadingAnchors_EmptySlug(t *testing.T) {
	// A heading with non-textual content that produces an empty slug.
	// An empty heading "# " produces an empty slug.
	f, err := lint.NewFile("test.md", []byte("# \n\nSome text.\n"))
	require.NoError(t, err)
	anchors := collectHeadingAnchors(f)
	// Empty slug is skipped, so anchors should be empty.
	require.Empty(t, anchors)
}

// TestCollectHeadingAnchors_DuplicateHeadings exercises the count > 0 branch,
// where the second heading with the same text gets a "-1" suffix in its anchor.
func TestCollectHeadingAnchors_DuplicateHeadings(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("# Intro\n\n## Intro\n"))
	require.NoError(t, err)
	anchors := collectHeadingAnchors(f)
	require.True(t, anchors["intro"], "first heading should be 'intro'")
	require.True(t, anchors["intro-1"], "second heading should be 'intro-1'")
}

// --- ApplySettings: bad exclude type ---

func TestApplySettings_BadExcludeType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"exclude": true})
	require.Error(t, err, "expected error for non-list exclude")
}

// --- ApplySettings: bad exclude glob pattern ---

func TestApplySettings_BadExcludeGlob(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"exclude": []any{"["}})
	require.Error(t, err, "expected error for invalid exclude glob")
}

// --- checkLink: local anchor with empty anchor string ---

func TestCheck_LocalAnchorEmptyFragment(t *testing.T) {
	// A link with only "#" (empty fragment) should be silently skipped.
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\nSee [here](#).\n")

	f := newLintFile(t, sourcePath)
	diags := (&Rule{}).Check(f)
	// Empty anchor skips without a diagnostic.
	require.Len(t, diags, 0, "empty anchor should be silently skipped")
}

// --- checkLink: link with anchor but target not markdown ---

func TestCheck_NonMarkdownLinkWithAnchorSkipped(t *testing.T) {
	// In strict mode, a non-markdown file that exists and has an anchor
	// should still be skipped for the anchor check (non-markdown => no heading check).
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	// The linked file exists.
	writeFile(t, filepath.Join(dir, "image.png"), "fake png data")
	writeFile(t, sourcePath, "# Doc\n\nSee [img](image.png#section).\n")

	f := newLintFile(t, sourcePath)
	r := &Rule{Strict: true}
	diags := r.Check(f)
	// File exists but is not Markdown, so anchor check is skipped.
	require.Len(t, diags, 0, "non-markdown with anchor should skip anchor check")
}

// --- resolveTargetOSPath: sourcePath == "." ---

func TestResolveTargetOSPath_DotSourcePath(t *testing.T) {
	path, ok := resolveTargetOSPath(".", "target.md")
	require.False(t, ok, "sourcePath='.' should return false")
	require.Empty(t, path)
}

// --- parseTarget: path=="" and no opaque, no fragment ---

func TestParseTarget_EmptyPathNoFragment(t *testing.T) {
	// A URL like "?" (query only) has no scheme, host, path, or fragment.
	// url.Parse("?q=1") => {RawQuery: "q=1"} — path == "", fragment == ""
	_, ok := parseTarget("?q=1")
	require.False(t, ok, "URL with only query should return false")
}

// --- normalizeLinkPath: path normalizes to "." ---

func TestNormalizeLinkPath_DotPath(t *testing.T) {
	// A path that normalises to "." should return "".
	result := normalizeLinkPath("./")
	require.Equal(t, "", result)
}

// --- matchesPathFilters: include match succeeds then exclude rejects ---

func TestMatchesPathFilters_IncludeThenExclude(t *testing.T) {
	// Include matches, but exclude also matches — should return false.
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\nSee [link](docs/secret.md).\n")

	f := newLintFile(t, sourcePath)
	r := &Rule{
		Strict:  true,
		Include: []string{"docs/**"},
		Exclude: []string{"docs/**"}, // exclude everything in docs
	}
	diags := r.Check(f)
	// File is excluded, so no diagnostics.
	require.Len(t, diags, 0)
}

// --- linkPosition: offset < 0 branch (no text node in link) ---

func TestCheck_LinkWithNoTextOffset(t *testing.T) {
	// A link that resolves but whose AST link node has no text children
	// would produce offset < 0. This is very hard to construct directly,
	// but we can exercise linkPosition indirectly through a normal check
	// that produces a diagnostic — the diagnostic's line/col is correct.
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\nSee [missing](missing.md).\n")

	f := newLintFile(t, sourcePath)
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	require.Greater(t, diags[0].Line, 0)
	require.Greater(t, diags[0].Column, 0)
}

// --- resolveAbsRoot: EvalSymlinks errors, Abs succeeds ---

func TestResolveAbsRoot_EvalSymlinksError(t *testing.T) {
	// A path that doesn't exist causes EvalSymlinks to fail; fallback to Abs.
	got := resolveAbsRoot("/nonexistent-abc-xyz-123/path")
	require.NotEmpty(t, got)
	require.True(t, filepath.IsAbs(got))
}

// --- checkLink: target.Anchor == "" after resolveTargetFile succeeds ---
// Line 136: if target.Anchor == "" || !isMarkdownPath(linkPath) { return nil }
// Exercise the target.Anchor == "" path: a plain markdown link with no anchor
// that resolves to an existing file should return nil (no diagnostic).
func TestCheck_MarkdownLinkNoAnchor_Passes(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "guide.md")
	sourcePath := filepath.Join(dir, "doc.md")

	writeFile(t, targetPath, "# Guide\n")
	// Link has no anchor — after resolveTargetFile succeeds, we hit the
	// "target.Anchor == ''" branch and return nil.
	writeFile(t, sourcePath, "# Doc\n\nSee [guide](guide.md).\n")

	f := newLintFile(t, sourcePath)
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 0)
}

// --- Check: configDiag for invalid exclude glob ---

func TestCheck_InvalidExcludeGlobReturnsConfigDiag(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\nSee [link](file.md).\n")

	f := newLintFile(t, sourcePath)
	// Bypass ApplySettings by setting Exclude directly to an invalid glob.
	r := &Rule{Exclude: []string{"["}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	require.Contains(t, diags[0].Message, "invalid rule settings")
}

// --- parseTarget: path == "" && u.Opaque != "" ---
// An opaque URI like "mailto:user@example.com" has Opaque set.
func TestParseTarget_PlainRelativePath(t *testing.T) {
	target, ok := parseTarget("guide.md")
	require.True(t, ok)
	require.Equal(t, "guide.md", target.Path)
	require.Empty(t, target.Anchor)
}

// --- matchesPathFilters: include list that does NOT match the path ---

func TestCheck_IncludePatternNoMatch_Skipped(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\nSee [link](other/missing.md).\n")

	f := newLintFile(t, sourcePath)
	// Include only "docs/**"; the link path "other/missing.md" does not match.
	r := &Rule{
		Strict:  true,
		Include: []string{"docs/**"},
	}
	diags := r.Check(f)
	// Not matched by include → skipped, no diagnostics.
	require.Len(t, diags, 0, "link not matching include should be skipped, got: %v", diagMessages(diags))
}

// TestMatchesPathFilters_IncludeNoMatch exercises the `!matched` return false
// branch directly, bypassing the full Check pipeline.
func TestMatchesPathFilters_IncludeNoMatch(t *testing.T) {
	include, err := compileMatchers([]string{"docs/**"})
	require.NoError(t, err)

	// "other/page.md" doesn't match "docs/**"
	result := matchesPathFilters("other/page.md", include, nil)
	require.False(t, result, "path not in include should return false")
}

// --- linkPosition: offset < 0 (no *ast.Text children in the link) ---

func TestLinkPosition_NoTextChildren(t *testing.T) {
	// Build a lint.File and a fake ast.Link with no children.
	// linkPosition calls firstTextOffset, which returns -1 when no ast.Text
	// nodes are found, causing the "if offset < 0 { return 1, 1 }" branch.
	f, err := lint.NewFile("test.md", []byte("# Title\n"))
	require.NoError(t, err)

	linkNode := gast.NewLink()
	// No children → firstTextOffset returns -1 → offset < 0 → return 1, 1

	line, col := linkPosition(f, linkNode)
	require.Equal(t, 1, line)
	require.Equal(t, 1, col)
}

// --- Collect: compute returns an error path in rank.go ---

func TestCheck_UnreadableTargetDiag(t *testing.T) {
	// Create a target file that exists but is too large to read.
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "guide.md")
	sourcePath := filepath.Join(dir, "doc.md")

	// Write more than 50 bytes.
	content := "# Guide\n\n## Setup\n\n" + string(make([]byte, 100))
	writeFile(t, targetPath, content)
	writeFile(t, sourcePath, "# Doc\n\nSee [guide](guide.md#setup).\n")

	data, err := os.ReadFile(sourcePath)
	require.NoError(t, err)
	f, err := lint.NewFile(sourcePath, data)
	require.NoError(t, err)
	f.FS = os.DirFS(filepath.Dir(sourcePath))
	f.MaxInputBytes = 10 // very small limit

	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	require.Contains(t, diags[0].Message, "cannot read link target")
}
