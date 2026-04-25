package crossfilereferenceintegrity

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gast "github.com/yuin/goldmark/ast"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/require"
)

func TestRuleMetadata(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS027" {
		t.Fatalf("ID = %q, want MDS027", r.ID())
	}
	if r.Name() != "cross-file-reference-integrity" {
		t.Fatalf("Name = %q, want cross-file-reference-integrity", r.Name())
	}
	if r.Category() != "link" {
		t.Fatalf("Category = %q, want link", r.Category())
	}
}

func TestCheck_MissingTargetFile(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\nSee [missing](missing.md).\n")

	f := newLintFile(t, sourcePath)
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	require.Contains(t, diags[0].Message, "missing.md", "message = %q, want to include missing.md", diags[0].Message)
}

func TestCheck_MissingHeadingAnchor(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "guide.md")
	sourcePath := filepath.Join(dir, "doc.md")

	writeFile(t, targetPath, "# Guide\n\n## Intro\n")
	writeFile(t, sourcePath, "# Doc\n\nSee [guide](guide.md#missing).\n")

	f := newLintFile(t, sourcePath)
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	require.Contains(t, diags[0].Message, "guide.md#missing",
		"message = %q, want to include guide.md#missing", diags[0].Message)
}

func TestCheck_ValidRelativeAndLocalAnchors(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "guide.md")
	sourcePath := filepath.Join(dir, "doc.md")

	writeFile(t, targetPath, "# Guide\n\n## Install\n")
	writeFile(t, sourcePath, strings.Join([]string{
		"# Doc",
		"",
		"See [guide](guide.md#install).",
		"",
		"Jump [down](#local-anchor).",
		"",
		"## Local Anchor",
		"",
	}, "\n"))

	f := newLintFile(t, sourcePath)
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %v", len(diags), diagMessages(diags))
}

func TestCheck_RelativeDotDotPath(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "guide.md")
	subDir := filepath.Join(dir, "nested")
	sourcePath := filepath.Join(subDir, "doc.md")

	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, targetPath, "# Guide\n")
	writeFile(t, sourcePath, "# Doc\n\nSee [guide](../guide.md).\n")

	f := newLintFile(t, sourcePath)
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %v", len(diags), diagMessages(diags))
}

func TestCheck_DefaultSkipsNonMarkdownTargets(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")

	writeFile(t, sourcePath, "# Doc\n\n![Logo](missing.png)\n\nSee [asset](missing.png).\n")

	f := newLintFile(t, sourcePath)
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %v", len(diags), diagMessages(diags))
}

func TestCheck_StrictChecksNonMarkdownTargets(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")

	writeFile(t, sourcePath, "# Doc\n\nSee [asset](missing.png).\n")

	f := newLintFile(t, sourcePath)
	r := &Rule{Strict: true}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
}

func TestCheck_IncludeExcludePatterns(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")

	writeFile(t, sourcePath, strings.Join([]string{
		"# Doc",
		"",
		"- [main](docs/missing.md)",
		"- [private](docs/private/secret.md)",
		"",
	}, "\n"))

	f := newLintFile(t, sourcePath)
	r := &Rule{
		Strict:  true,
		Include: []string{"docs/**"},
		Exclude: []string{"docs/private/**"},
	}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %v", len(diags), diagMessages(diags))
	require.Contains(t, diags[0].Message, "docs/missing.md",
		"message = %q, want to include docs/missing.md", diags[0].Message)
}

func TestApplySettings_InvalidValues(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]any
	}{
		{
			name:     "unknown setting",
			settings: map[string]any{"unknown": true},
		},
		{
			name:     "bad strict type",
			settings: map[string]any{"strict": "true"},
		},
		{
			name:     "bad include type",
			settings: map[string]any{"include": true},
		},
		{
			name:     "bad include item type",
			settings: map[string]any{"include": []any{"docs/**", 123}},
		},
		{
			name:     "bad include glob",
			settings: map[string]any{"include": []any{"["}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &Rule{}
			if err := r.ApplySettings(tc.settings); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestApplySettings_ValidValues(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"strict":  true,
		"include": []any{"docs/**"},
		"exclude": []any{"docs/private/**"},
	})
	require.NoError(t, err, "ApplySettings returned error: %v", err)

	require.True(t, r.Strict, "expected strict=true")
	if len(r.Include) != 1 || r.Include[0] != "docs/**" {
		t.Fatalf("unexpected include: %v", r.Include)
	}
	if len(r.Exclude) != 1 || r.Exclude[0] != "docs/private/**" {
		t.Fatalf("unexpected exclude: %v", r.Exclude)
	}
}

func TestCheck_NoFS(t *testing.T) {
	f, err := lint.NewFile("stdin.md", []byte("# Doc\n\nSee [x](missing.md)\n"))
	require.NoError(t, err)

	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_PathTraversalAboveRootSkipped(t *testing.T) {
	// Create a dedicated temp parent containing both RootDir and a sibling
	// file outside RootDir so all filesystem effects remain test-scoped.
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	sub := filepath.Join(root, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	// Create a file outside root but still under the test-scoped temp parent.
	outside := filepath.Join(parent, "outside.md")
	writeFile(t, outside, "# Outside\n")

	// Create a source file linking to the outside file.
	sourcePath := filepath.Join(sub, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\nSee [escape](../../outside.md).\n")

	f := newLintFile(t, sourcePath)
	f.RootDir = root

	diags := (&Rule{}).Check(f)
	// The link traverses above RootDir, so it should be silently skipped
	// (not reported as broken).
	require.Len(t, diags, 0,
		"links above RootDir should be silently skipped, got: %v",
		diagMessages(diags))
}

func TestCheck_PathWithinRootWorks(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	targetPath := filepath.Join(root, "target.md")
	writeFile(t, targetPath, "# Target\n")

	sourcePath := filepath.Join(sub, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\nSee [target](../target.md).\n")

	f := newLintFile(t, sourcePath)
	f.RootDir = root

	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 0,
		"links within RootDir should work, got: %v",
		diagMessages(diags))
}

func newLintFile(t *testing.T, path string) *lint.File {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	f, err := lint.NewFile(path, data)
	require.NoError(t, err)
	f.FS = os.DirFS(filepath.Dir(path))
	return f
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func diagMessages(diags []lint.Diagnostic) []string {
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	return msgs
}

// --- resolveAbsRoot / isWithinRoot unit tests ---

func TestResolveAbsRoot_Empty(t *testing.T) {
	require.Equal(t, "", resolveAbsRoot(""))
}

func TestResolveAbsRoot_ValidDir(t *testing.T) {
	dir := t.TempDir()
	got := resolveAbsRoot(dir)
	require.NotEmpty(t, got)
	require.True(t, filepath.IsAbs(got))
}

func TestResolveAbsRoot_NonexistentDir(t *testing.T) {
	// EvalSymlinks fails for nonexistent paths; should fall back to Abs.
	got := resolveAbsRoot("/tmp/nonexistent-resolve-test-dir")
	require.NotEmpty(t, got)
	require.True(t, filepath.IsAbs(got))
}

func TestResolveAbsRoot_Symlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	require.NoError(t, os.Mkdir(real, 0o755))
	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(real, link))

	resolved := resolveAbsRoot(link)
	// Should resolve through the symlink to the real dir.
	realAbs, _ := filepath.Abs(real)
	require.Equal(t, realAbs, resolved)
}

func TestIsWithinRoot_InsideDir(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "child.md")
	writeFile(t, child, "# Child\n")

	root := resolveAbsRoot(dir)
	require.True(t, isWithinRoot(root, child))
}

func TestIsWithinRoot_OutsideDir(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "root")
	require.NoError(t, os.Mkdir(dir, 0o755))
	outside := filepath.Join(parent, "outside.md")
	writeFile(t, outside, "# Outside\n")

	root := resolveAbsRoot(dir)
	require.False(t, isWithinRoot(root, outside))
}

func TestIsWithinRoot_SymlinkEscape(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "root")
	require.NoError(t, os.Mkdir(dir, 0o755))
	outside := filepath.Join(parent, "escape-target.md")
	writeFile(t, outside, "# Escaped\n")

	// Create a symlink inside the root that points outside.
	link := filepath.Join(dir, "link.md")
	require.NoError(t, os.Symlink(outside, link))

	root := resolveAbsRoot(dir)
	require.False(t, isWithinRoot(root, link),
		"symlink pointing outside root should be rejected")
}

func TestLinkEscapesRoot_NoPath(t *testing.T) {
	f := &lint.File{Path: "standalone.md"}
	// No directory separator in Path → resolveTargetOSPath returns false.
	require.False(t, linkEscapesRoot(f, "../escape.md", "/some/root"))
}

func TestResolveTargetFile_RejectsOutsideRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	sub := filepath.Join(root, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	outside := filepath.Join(parent, "outside.md")
	writeFile(t, outside, "# Outside\n")

	f := &lint.File{
		Path: filepath.Join(sub, "doc.md"),
		FS:   os.DirFS(sub),
	}
	resolvedRoot := resolveAbsRoot(root)

	_, ok := resolveTargetFile(f, "../../outside.md", resolvedRoot)
	require.False(t, ok, "should reject link resolving outside root")
}

func TestResolveTargetFile_AllowsInsideRoot(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	writeFile(t, filepath.Join(root, "target.md"), "# Target\n")

	f := &lint.File{
		Path: filepath.Join(sub, "doc.md"),
		FS:   os.DirFS(sub),
	}
	resolvedRoot := resolveAbsRoot(root)

	_, ok := resolveTargetFile(f, "../target.md", resolvedRoot)
	require.True(t, ok, "should allow link within root")
}

// TestResolveTargetFile_MaxInputBytes verifies that the read closure
// returned by resolveTargetFile honors f.MaxInputBytes — an oversized
// target file should produce a "file too large" error, which callers
// surface via unreadableTargetDiag instead of a misleading "not found".
func TestResolveTargetFile_MaxInputBytes(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.md")
	writeFile(t, target, "# Target\n\n"+strings.Repeat("x", 200))

	f := &lint.File{
		Path:          filepath.Join(root, "doc.md"),
		FS:            os.DirFS(root),
		MaxInputBytes: 50,
	}
	resolvedRoot := resolveAbsRoot(root)

	tgt, ok := resolveTargetFile(f, "target.md", resolvedRoot)
	require.True(t, ok, "expected target resolution to succeed")

	_, err := tgt.read()
	require.Error(t, err, "expected file too large error")
	require.Contains(t, err.Error(), "file too large")
}

func TestUnreadableTargetDiag(t *testing.T) {
	r := &Rule{}
	err := errors.New("file too large (200 bytes, max 50)")
	d := unreadableTargetDiag("doc.md", 5, 10, r, "target.md", err)
	require.Equal(t, "doc.md", d.File)
	require.Equal(t, 5, d.Line)
	require.Equal(t, 10, d.Column)
	require.Contains(t, d.Message, "cannot read link target")
	require.Contains(t, d.Message, "file too large")
}

func TestIsWithinRoot_RelativeTarget(t *testing.T) {
	// When f.Path is relative, resolveTargetOSPath can return a relative
	// path. isWithinRoot must convert to absolute via Abs (using CWD)
	// and then compare to the root. A relative path that happens to
	// resolve outside root must be rejected.
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	require.NoError(t, os.Mkdir(root, 0o755))

	absRoot := resolveAbsRoot(root)
	// "some/relative.md" resolves to CWD/some/relative.md which is
	// outside the temp root — must be rejected (not silently allowed).
	require.False(t, isWithinRoot(absRoot, "some/relative.md"),
		"relative path resolving outside root via CWD should be rejected")
}

func TestIsWithinRoot_RelativeTargetOutside(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	require.NoError(t, os.Mkdir(root, 0o755))
	writeFile(t, filepath.Join(parent, "outside.md"), "# Outside\n")

	absRoot := resolveAbsRoot(root)
	// "../outside.md" is relative and points outside root.
	require.False(t, isWithinRoot(absRoot, "../outside.md"),
		"relative path outside root should be rejected")
}

func TestIsWithinRoot_NonexistentTarget(t *testing.T) {
	dir := t.TempDir()
	root := resolveAbsRoot(dir)
	// Nonexistent file inside root — EvalSymlinks fails, falls back to Clean.
	require.True(t, isWithinRoot(root, filepath.Join(dir, "nonexistent.md")))
}

func TestIsWithinRoot_NonexistentOutside(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "root")
	require.NoError(t, os.Mkdir(dir, 0o755))
	root := resolveAbsRoot(dir)
	require.False(t, isWithinRoot(root, filepath.Join(parent, "nonexistent.md")))
}

// =====================================================================
// Phase 4 coverage: DefaultSettings
// =====================================================================

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	require.Equal(t, false, ds["strict"])
	include, ok := ds["include"].([]string)
	require.True(t, ok)
	require.Len(t, include, 0)
	exclude, ok := ds["exclude"].([]string)
	require.True(t, ok)
	require.Len(t, exclude, 0)
}

// =====================================================================
// Phase 4 coverage: configDiag (via invalid glob in Include field)
// =====================================================================

func TestCheck_InvalidIncludeGlobReturnsConfigDiag(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\nSee [link](file.md).\n")

	f := newLintFile(t, sourcePath)
	// Bypass ApplySettings by setting Include directly to an invalid glob.
	r := &Rule{Include: []string{"["}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	require.Contains(t, diags[0].Message, "invalid rule settings")
	require.Equal(t, "MDS027", diags[0].RuleID)
}

// =====================================================================
// Phase 4 coverage: parseTarget edge cases
// =====================================================================

func TestParseTarget_AnchorOnly(t *testing.T) {
	target, ok := parseTarget("#section")
	require.True(t, ok)
	require.Equal(t, "#section", target.Raw)
	require.Equal(t, "section", target.Anchor)
	require.True(t, target.LocalAnchor)
	require.Equal(t, "", target.Path)
}

func TestParseTarget_Empty(t *testing.T) {
	_, ok := parseTarget("")
	require.False(t, ok)
}

func TestParseTarget_ProtocolRelative(t *testing.T) {
	_, ok := parseTarget("//example.com/path")
	require.False(t, ok)
}

func TestParseTarget_AbsoluteURL(t *testing.T) {
	_, ok := parseTarget("https://example.com/path")
	require.False(t, ok)
}

func TestParseTarget_PathWithAnchor(t *testing.T) {
	target, ok := parseTarget("guide.md#intro")
	require.True(t, ok)
	require.Equal(t, "guide.md", target.Path)
	require.Equal(t, "intro", target.Anchor)
	require.False(t, target.LocalAnchor)
}

func TestParseTarget_EncodedPath(t *testing.T) {
	target, ok := parseTarget("my%20file.md")
	require.True(t, ok)
	// url.Parse decodes percent-encoded characters in the path.
	require.Equal(t, "my file.md", target.Path)
}

// =====================================================================
// Phase 4 coverage: toStringSlice edge cases
// =====================================================================

func TestToStringSlice_MixedTypes(t *testing.T) {
	_, ok := toStringSlice([]any{"valid", 123})
	require.False(t, ok)
}

func TestToStringSlice_NonSlice(t *testing.T) {
	_, ok := toStringSlice("not a slice")
	require.False(t, ok)
}

// =====================================================================
// Phase 4 coverage: anchor-only local ref validated against self anchors
// =====================================================================

func TestCheck_AnchorOnlyLinkMissingHeading(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	// Link to #missing which doesn't exist in the doc.
	writeFile(t, sourcePath, "# Doc\n\nSee [here](#missing).\n")

	f := newLintFile(t, sourcePath)
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	require.Contains(t, diags[0].Message, "#missing")
}

// =====================================================================
// Additional coverage: anchorsForFile cache hit
// =====================================================================

// TestAnchorsForFile_CacheHit exercises the early-return path in anchorsForFile
// when the result is already present in the cache.
func TestAnchorsForFile_CacheHit(t *testing.T) {
	// Pre-populate the cache with a known anchor set.
	cached := map[string]bool{"intro": true}
	cache := map[string]map[string]bool{
		"mykey": cached,
	}

	tf := targetFile{
		cacheKey: "mykey",
		read: func() ([]byte, error) {
			// Should never be called when the cache is hit.
			t.Fatal("read() must not be called on a cache hit")
			return nil, nil
		},
	}

	result, err := anchorsForFile(tf, cache)
	require.NoError(t, err)
	require.True(t, result["intro"], "cache hit must return the pre-populated anchors")
}

// TestAnchorsForFile_ReadError exercises the read() error path in anchorsForFile.
func TestAnchorsForFile_ReadError(t *testing.T) {
	cache := map[string]map[string]bool{}
	readErr := errors.New("simulated read error")
	tf := targetFile{
		cacheKey: "errkey",
		read: func() ([]byte, error) {
			return nil, readErr
		},
	}

	_, err := anchorsForFile(tf, cache)
	require.Error(t, err)
	require.Equal(t, readErr, err)
}

// =====================================================================
// Additional coverage: appendNodeText with *ast.String
// =====================================================================

// TestAppendNodeText_AstString exercises the *ast.String branch of appendNodeText.
// ast.String is created by the typographer extension and other paragraph
// transforms; we construct one directly to keep the test self-contained.
func TestAppendNodeText_AstString(t *testing.T) {
	strNode := gast.NewString([]byte("hello"))

	var b strings.Builder
	appendNodeText(&b, strNode, nil)
	require.Equal(t, "hello", b.String())
}

// =====================================================================
// Additional coverage: resolveTargetFile FS-only path
// =====================================================================

// TestResolveTargetFile_FSOnlyPath exercises the fallback branch in
// resolveTargetFile where the OS path lookup fails (because f.Path has no
// directory component) but the FS contains the target file.
func TestResolveTargetFile_FSOnlyPath(t *testing.T) {
	dir := t.TempDir()
	targetContent := []byte("# Target\n")
	writeFile(t, filepath.Join(dir, "target.md"), string(targetContent))

	// f.Path has no directory separator, so resolveTargetOSPath returns (_, false).
	// The FS lookup succeeds because dir contains target.md.
	f := &lint.File{
		Path: "doc.md", // no separator → resolveTargetOSPath returns false
		FS:   os.DirFS(dir),
	}

	tf, ok := resolveTargetFile(f, "target.md", "")
	require.True(t, ok, "expected target resolution via FS to succeed")

	data, err := tf.read()
	require.NoError(t, err)
	require.Equal(t, targetContent, data)
}

// TestResolveTargetFile_FSPathAbsoluteReturnsNotFound exercises the
// `strings.HasPrefix(fsPath, "/")` branch that returns false.
func TestResolveTargetFile_FSPathAbsoluteReturnsNotFound(t *testing.T) {
	dir := t.TempDir()
	f := &lint.File{
		Path: "doc.md", // no separator → resolveTargetOSPath returns false
		FS:   os.DirFS(dir),
	}
	// A linkPath that normalises to an absolute-like path (leading /).
	// normalizeLinkPath(filepath.FromSlash("/absolute")) → "/" → "."? No,
	// actually linkPath must be pre-normalized by the caller as a relative
	// path.  To hit the fsPath=="" branch, we pass a linkPath of "." which
	// normalizeLinkPath would return "" for. We call resolveTargetFile
	// directly with a clean "." which results in fsPath="" after TrimPrefix.
	// Actually: normalizeLinkPath cleans to ".", and "." → "" is handled by
	// checkLink; here we call resolveTargetFile directly with the already-
	// normalised form. TrimPrefix("./", "./") removes "./" leaving "".
	_, ok := resolveTargetFile(f, "./", "")
	require.False(t, ok, "fsPath='' after TrimPrefix must return not found")
}

// =====================================================================
// Additional coverage: toStringSlice with []string type
// =====================================================================

// TestToStringSlice_StringSlice exercises the []string case in toStringSlice,
// which is the branch reached when YAML is already decoded to []string (e.g.
// by the settings layer).
func TestToStringSlice_StringSlice(t *testing.T) {
	input := []string{"docs/**", "src/**"}
	result, ok := toStringSlice(input)
	require.True(t, ok)
	require.Equal(t, input, result)
	// Verify it's a copy, not the original slice.
	result[0] = "changed"
	require.Equal(t, "docs/**", input[0], "toStringSlice must return a copy")
}

// =====================================================================
// Additional coverage: Check with link to FS-resolved target having anchor
// =====================================================================

// TestCheck_FSResolvesTargetWithAnchor exercises the path where the target
// file is found via the FS (not OS path) and has a matching anchor.  This
// also exercises the anchorsForFile success path in integration.
func TestCheck_FSResolvesTargetWithAnchor(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "guide.md")
	sourcePath := filepath.Join(dir, "doc.md")

	writeFile(t, targetPath, "# Guide\n\n## Setup\n")
	writeFile(t, sourcePath, "# Doc\n\nSee [guide](guide.md#setup).\n")

	f := newLintFile(t, sourcePath)
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 0,
		"link to existing anchor must not produce diagnostics, got: %v", diagMessages(diags))
}

// TestCheck_FSResolvesTargetAndAnchorCachedOnSecondLink exercises the
// anchorsForFile cache hit path by checking the same target file twice
// (two links to guide.md) in a single Check call.
func TestCheck_FSResolvesTargetAnchorCachedOnSecondLink(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "guide.md")
	sourcePath := filepath.Join(dir, "doc.md")

	writeFile(t, targetPath, "# Guide\n\n## Setup\n")
	// Two links to the same target: second call hits anchorsForFile cache.
	writeFile(t, sourcePath, "# Doc\n\nFirst [link](guide.md#setup).\n\nSecond [link](guide.md#setup).\n")

	f := newLintFile(t, sourcePath)
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 0,
		"both links to existing anchor must be clean, got: %v", diagMessages(diags))
}

// TestCheck_AbsoluteURLLinkSkipped exercises the parseTarget-returns-false
// branch in checkLink: absolute-URL links are silently skipped.
func TestCheck_AbsoluteURLLinkSkipped(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\nSee [external](https://example.com/path).\n")

	f := newLintFile(t, sourcePath)
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 0, "absolute URL links must produce no diagnostics, got: %v", diagMessages(diags))
}

// TestCheck_AbsoluteFilepathLinkSkipped exercises the filepath.IsAbs(linkPath)
// branch in checkLink which returns nil for absolute-path links.
func TestCheck_AbsoluteFilepathLinkSkipped(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	// An absolute path link – these are skipped regardless of whether the
	// file exists.
	writeFile(t, sourcePath, "# Doc\n\nSee [root](/etc/nonexistent.md).\n")

	f := newLintFile(t, sourcePath)
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 0, "absolute-path links must be skipped, got: %v", diagMessages(diags))
}
