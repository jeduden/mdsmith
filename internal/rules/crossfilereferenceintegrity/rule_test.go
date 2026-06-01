package crossfilereferenceintegrity

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"

	"github.com/stretchr/testify/assert"
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

// TestCheck_FrontMatterLineNumbersBodyRelative verifies that MDS027
// emits body-relative line numbers when the source has front matter.
// The engine adds f.LineOffset to every diagnostic in CheckRules, so
// link.Line must NOT already include that offset — otherwise the line
// is shifted twice and points past the end of the file.
func TestCheck_FrontMatterLineNumbersBodyRelative(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	// 3 lines of front matter + body. The broken link is on body line 3.
	source := "---\ntitle: x\n---\n# Doc\n\nSee [missing](missing.md).\n"
	writeFile(t, sourcePath, source)

	data, err := os.ReadFile(sourcePath)
	require.NoError(t, err)
	// Use NewFileFromSource (the same shape the engine uses for files
	// with front matter): strips the prefix, records LineOffset=3.
	f, err := lint.NewFileFromSource(sourcePath, data, true)
	require.NoError(t, err)
	f.FS = os.DirFS(filepath.Dir(sourcePath))

	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 1)
	// Body-relative: link is on body line 3 ("See [missing]…"), not
	// file-relative 6. The engine adds LineOffset later.
	require.Equal(t, 3, diags[0].Line,
		"diagnostic must use body-relative coordinates; engine adds f.LineOffset")
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
	target, ok := linkgraph.ParseTarget("#section")
	require.True(t, ok)
	require.Equal(t, "#section", target.Raw)
	require.Equal(t, "section", target.Anchor)
	require.True(t, target.LocalAnchor)
	require.Equal(t, "", target.Path)
}

func TestParseTarget_Empty(t *testing.T) {
	_, ok := linkgraph.ParseTarget("")
	require.False(t, ok)
}

func TestParseTarget_ProtocolRelative(t *testing.T) {
	_, ok := linkgraph.ParseTarget("//example.com/path")
	require.False(t, ok)
}

func TestParseTarget_AbsoluteURL(t *testing.T) {
	_, ok := linkgraph.ParseTarget("https://example.com/path")
	require.False(t, ok)
}

func TestParseTarget_PathWithAnchor(t *testing.T) {
	target, ok := linkgraph.ParseTarget("guide.md#intro")
	require.True(t, ok)
	require.Equal(t, "guide.md", target.Path)
	require.Equal(t, "intro", target.Anchor)
	require.False(t, target.LocalAnchor)
}

func TestParseTarget_EncodedPath(t *testing.T) {
	target, ok := linkgraph.ParseTarget("my%20file.md")
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
	cached := map[string]struct{}{"intro": {}}
	cache := map[string]map[string]struct{}{
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

	result, err := anchorsForFile(nil, tf, cache)
	require.NoError(t, err)
	require.Contains(t, result, "intro", "cache hit must return the pre-populated anchors")
}

// TestAnchorsForFile_ReadError exercises the read() error path in anchorsForFile.
func TestAnchorsForFile_ReadError(t *testing.T) {
	cache := map[string]map[string]struct{}{}
	readErr := errors.New("simulated read error")
	tf := targetFile{
		cacheKey: "errkey",
		read: func() ([]byte, error) {
			return nil, readErr
		},
	}

	_, err := anchorsForFile(nil, tf, cache)
	require.Error(t, err)
	require.Equal(t, readErr, err)
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

// TestResolveTargetFile_EmptyFSPathReturnsNotFound exercises the early
// return when TrimPrefix("./", "./") leaves fsPath empty.
func TestResolveTargetFile_EmptyFSPathReturnsNotFound(t *testing.T) {
	dir := t.TempDir()
	f := &lint.File{
		Path: "doc.md", // no separator → resolveTargetOSPath returns false
		FS:   os.DirFS(dir),
	}
	// "./" becomes an empty fsPath after TrimPrefix("./", "./"), which
	// hits the fsPath=="" early-return branch.
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

// --- Placeholder tests ---

func TestCheck_Placeholder_VarTokenInLink_Suppressed(t *testing.T) {
	// A link whose destination contains a var-token placeholder should
	// not be flagged when var-token is configured.
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "test.md")
	writeFile(t, srcPath, "# Title\n\n[link]({base}/docs/page.md)\n")
	f := newLintFile(t, srcPath)
	r := &Rule{Placeholders: []string{"var-token"}}
	diags := r.Check(f)
	require.Empty(t, diags, "var-token in link destination should suppress diagnostic")
}

func TestCheck_Placeholder_VarTokenInLink_EmptyPlaceholders(t *testing.T) {
	// Without placeholders, a link with a placeholder in the destination is
	// flagged as a broken link (the path doesn't exist).
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "test.md")
	writeFile(t, srcPath, "# Title\n\n[link]({base}/docs/page.md)\n")
	f := newLintFile(t, srcPath)
	r := &Rule{Placeholders: []string{}}
	diags := r.Check(f)
	require.Len(t, diags, 1, "broken link without placeholders should be flagged")
}

func TestApplySettings_Placeholders_CrossFile(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"placeholders": []any{"var-token"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"var-token"}, r.Placeholders)
}

func TestApplySettings_Placeholders_UnknownToken_CrossFile(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"placeholders": []any{"bad"}})
	require.Error(t, err)
}

func TestApplySettings_Placeholders_NonList_CrossFile(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"placeholders": "not-a-list"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "list of strings")
}

func TestDefaultSettings_CrossFile(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	require.Equal(t, []string{}, ds["placeholders"])
}

func TestSettingMergeMode_CrossFileReferenceIntegrity(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, rule.MergeAppend, r.SettingMergeMode("placeholders"))
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("include"))
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("unknown"))
}

// =====================================================================
// G1: validate-images
// =====================================================================

func TestCheck_ValidateImages_FlagsMissingTarget(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\n![diagram](missing.png)\n")

	f := newLintFile(t, sourcePath)
	r := &Rule{Links: LinksConfig{ValidateImages: true}}
	diags := r.Check(f)
	require.Len(t, diags, 1, "missing image target must be flagged")
	require.Contains(t, diags[0].Message, "missing.png")
}

func TestCheck_ValidateImages_SilentWhenOff(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\n![diagram](missing.png)\n")

	f := newLintFile(t, sourcePath)
	r := &Rule{Links: LinksConfig{ValidateImages: false}}
	diags := r.Check(f)
	require.Len(t, diags, 0, "image target must be silent when validate-images is off")
}

func TestCheck_ValidateImages_ExistingTarget(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, filepath.Join(dir, "logo.png"), "fake png")
	writeFile(t, sourcePath, "# Doc\n\n![logo](logo.png)\n")

	f := newLintFile(t, sourcePath)
	r := &Rule{Links: LinksConfig{ValidateImages: true}}
	diags := r.Check(f)
	require.Len(t, diags, 0, "existing image target must produce no diagnostic")
}

func TestCheck_ValidateImages_ReferenceStyle(t *testing.T) {
	// Reference-style image whose target file is missing.
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\n![alt][img]\n\n[img]: missing.png\n")

	f := newLintFile(t, sourcePath)
	r := &Rule{Links: LinksConfig{ValidateImages: true}}
	diags := r.Check(f)
	require.Len(t, diags, 1, "broken reference-style image must be flagged")
	require.Contains(t, diags[0].Message, "missing.png")
}

// TestCheck_ValidateImages_BypassesStrictMode verifies that
// validate-images checks image targets regardless of strict mode —
// images are intentional assets unlike arbitrary non-markdown links.
func TestCheck_ValidateImages_BypassesStrictMode(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\n![logo](missing.png)\n")

	f := newLintFile(t, sourcePath)
	// strict=false but validate-images=true: image must still be checked.
	r := &Rule{Strict: false, Links: LinksConfig{ValidateImages: true}}
	diags := r.Check(f)
	require.Len(t, diags, 1, "validate-images must check .png even with strict=false")
}

// =====================================================================
// G2: site-root for absolute paths
// =====================================================================

func TestCheck_SiteRoot_ResolvesExistingDir(t *testing.T) {
	siteRoot := t.TempDir()
	// Create the target directory that the absolute link points to.
	targetDir := filepath.Join(siteRoot, "docs", "rules", "MDS027")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))

	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\nSee [rule](/docs/rules/MDS027/).\n")

	f := newLintFile(t, sourcePath)
	r := &Rule{Links: LinksConfig{SiteRoot: siteRoot}}
	diags := r.Check(f)
	require.Len(t, diags, 0, "absolute link resolving to existing dir must produce no diagnostic")
}

func TestCheck_SiteRoot_FlagsMissingDir(t *testing.T) {
	siteRoot := t.TempDir()
	// Do NOT create the target directory.

	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\nSee [rule](/docs/rules/missing/).\n")

	f := newLintFile(t, sourcePath)
	r := &Rule{Links: LinksConfig{SiteRoot: siteRoot}}
	diags := r.Check(f)
	require.Len(t, diags, 1, "absolute link to nonexistent path must be flagged")
	require.Contains(t, diags[0].Message, "/docs/rules/missing/")
}

func TestCheck_SiteRoot_UnsetPreservesShortCircuit(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\nSee [root](/docs/rules/MDS027/).\n")

	f := newLintFile(t, sourcePath)
	// No site-root configured: absolute paths are silently skipped.
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "absolute path without site-root must be silently skipped")
}

// =====================================================================
// G3: validate-reference-style
// =====================================================================

func TestCheck_ValidateRefStyle_FlagsBrokenDef(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	// Definition points to a file that does not exist.
	writeFile(t, sourcePath, "# Doc\n\n[a]: ./missing.md\n\nSee [a].\n")

	f := newLintFile(t, sourcePath)
	r := &Rule{Links: LinksConfig{ValidateReferenceStyle: true}}
	diags := r.Check(f)
	require.Len(t, diags, 1, "broken reference-style target must be flagged")
	require.Contains(t, diags[0].Message, "missing.md")
}

func TestCheck_ValidateRefStyle_SilentWhenOff(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\n[a]: ./missing.md\n\nSee [a].\n")

	f := newLintFile(t, sourcePath)
	r := &Rule{Links: LinksConfig{ValidateReferenceStyle: false}}
	diags := r.Check(f)
	require.Len(t, diags, 0, "reference-style target must be silent when validate-reference-style is off")
}

func TestCheck_ValidateRefStyle_ExistingTarget(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "guide.md")
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, targetPath, "# Guide\n")
	writeFile(t, sourcePath, "# Doc\n\n[guide]: guide.md\n\nSee [guide].\n")

	f := newLintFile(t, sourcePath)
	r := &Rule{Links: LinksConfig{ValidateReferenceStyle: true}}
	diags := r.Check(f)
	require.Len(t, diags, 0, "valid reference-style target must produce no diagnostic")
}

// =====================================================================
// Edge-case coverage: normalized empty path and site-root slash-only
// =====================================================================

// TestCheck_DotLinkPathNormalizesEmpty ensures that a link whose target
// normalizes to "" (e.g. "[text](.)") is silently ignored rather than
// flagged as a broken link.
func TestCheck_DotLinkPathNormalizesEmpty(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	// "." normalizes to "." → normalizeLinkPath returns ""
	writeFile(t, sourcePath, "# Doc\n\nSee [here](.).\n")

	f := newLintFile(t, sourcePath)
	diags := (&Rule{}).Check(f)
	require.Len(t, diags, 0, "link whose path normalizes to empty must be silently ignored")
}

// TestCheck_SiteRootSlashOnlyLinkIgnored ensures that an absolute link
// consisting only of "/" is silently ignored when site-root is set,
// because stripping the leading "/" leaves an empty rel path.
func TestCheck_SiteRootSlashOnlyLinkIgnored(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\nSee [root](/).\n")

	f := newLintFile(t, sourcePath)
	r := &Rule{Links: LinksConfig{SiteRoot: dir}}
	diags := r.Check(f)
	require.Len(t, diags, 0, "site-absolute link consisting of only '/' must be silently ignored")
}

// =====================================================================
// ApplySettings: links sub-block
// =====================================================================

func TestApplySettings_Links_ValidValues(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{
			"site-root":                "/srv/site",
			"validate-images":          false,
			"validate-reference-style": false,
		},
	})
	require.NoError(t, err)
	require.Equal(t, "/srv/site", r.Links.SiteRoot)
	require.False(t, r.Links.ValidateImages)
	require.False(t, r.Links.ValidateReferenceStyle)
}

func TestApplySettings_Links_DefaultsViaApply(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(r.DefaultSettings())
	require.NoError(t, err)
	require.Equal(t, "", r.Links.SiteRoot)
	require.True(t, r.Links.ValidateImages)
	require.True(t, r.Links.ValidateReferenceStyle)
}

func TestApplySettings_Links_InvalidType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"links": "not-a-map"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "links must be a map")
}

func TestApplySettings_Links_InvalidSiteRootType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{"site-root": 42},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "links.site-root")
}

func TestApplySettings_Links_InvalidValidateImagesType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{"validate-images": "yes"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "links.validate-images")
}

func TestApplySettings_Links_InvalidValidateRefStyleType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{"validate-reference-style": 1},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "links.validate-reference-style")
}

func TestApplySettings_Links_UnknownKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{"unknown-key": true},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown links setting")
}

// TestApplySettings_Links_ToleratesMDS068Keys verifies the shared
// `links:` block stays usable when both MDS027 and MDS068 are
// configured from the same YAML body. MDS068's keys (`style`,
// `external-skip`) reach MDS027 too; they must parse without
// erroring even though MDS027 ignores them at runtime.
func TestApplySettings_Links_ToleratesMDS068Keys(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{
			"site-root":     "/srv",
			"style":         map[string]any{"path": "relative"},
			"external-skip": []any{"^https?://localhost"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "/srv", r.Links.SiteRoot)
}

func TestDefaultSettings_Links(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	links, ok := ds["links"].(map[string]any)
	require.True(t, ok, "DefaultSettings must include a links map")
	require.Equal(t, "", links["site-root"])
	require.Equal(t, true, links["validate-images"])
	require.Equal(t, true, links["validate-reference-style"])
}

func TestDefaultSettings_Wikilinks(t *testing.T) {
	ds := (&Rule{}).DefaultSettings()
	require.Equal(t, false, ds["wikilinks"])
	require.Equal(t, "obsidian", ds["wikilink-style"])
}

func TestCheck_Wikilinks_DisabledByDefault(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\nSee [[Missing]].\n")

	f := newLintFile(t, sourcePath)
	diags := (&Rule{}).Check(f)
	require.Empty(t, diags, "wikilinks must not be flagged when wikilinks=false")
}

func TestCheck_Wikilinks_UnresolvedTarget(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\nSee [[Missing Page]].\n")

	f := newLintFileWithRoot(t, sourcePath, dir)
	r := &Rule{Wikilinks: true}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	require.Contains(t, diags[0].Message, "Missing Page")
	require.Contains(t, diags[0].Message, "not found in workspace")
}

func TestCheck_Wikilinks_ResolvedTarget(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, filepath.Join(dir, "present.md"), "# Present\n")
	writeFile(t, sourcePath, "# Doc\n\nSee [[Present]].\n")

	f := newLintFileWithRoot(t, sourcePath, dir)
	r := &Rule{Wikilinks: true}
	diags := r.Check(f)
	require.Empty(t, diags, "wikilinks to existing files must not be flagged")
}

func TestCheck_Wikilinks_BrokenAnchor(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, filepath.Join(dir, "notes.md"), "# Notes\n\n## Current Heading\n")
	writeFile(t, sourcePath, "# Doc\n\nSee [[Notes#Old Heading]].\n")

	f := newLintFileWithRoot(t, sourcePath, dir)
	r := &Rule{Wikilinks: true}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	require.Contains(t, diags[0].Message, "Old Heading")
	require.Contains(t, diags[0].Message, "notes.md")
}

func TestCheck_Wikilinks_ResolvedAnchor(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, filepath.Join(dir, "notes.md"), "# Notes\n\n## Existing\n")
	writeFile(t, sourcePath, "# Doc\n\nSee [[Notes#Existing]].\n")

	f := newLintFileWithRoot(t, sourcePath, dir)
	r := &Rule{Wikilinks: true}
	diags := r.Check(f)
	require.Empty(t, diags, "matching wikilink anchors must not be flagged")
}

func TestCheck_Wikilinks_AliasIgnoredForResolution(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, filepath.Join(dir, "page.md"), "# Page\n")
	writeFile(t, sourcePath, "# Doc\n\nSee [[Page|Alias]].\n")

	f := newLintFileWithRoot(t, sourcePath, dir)
	r := &Rule{Wikilinks: true}
	diags := r.Check(f)
	require.Empty(t, diags, "alias must not affect resolution of the target stem")
}

func TestCheck_Wikilinks_EmbedAnyFileType(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "image.png"), []byte("\x89PNG\r\n"), 0o644))
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\nLook ![[image.png]].\n")

	f := newLintFileWithRoot(t, sourcePath, dir)
	r := &Rule{Wikilinks: true}
	diags := r.Check(f)
	require.Empty(t, diags, "embed must resolve any extension")
}

func TestCheck_Wikilinks_PlaceholderAppliesToAnchor(t *testing.T) {
	// wikilinkSuppressed builds "target#anchor" before testing the
	// placeholder set, so a placeholder hit in the anchor must
	// suppress the diagnostic the same way a hit in the target
	// would. Covers the `wl.Anchor != ""` branch of the helper.
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\nSee [[Page#{topic}]] there.\n")

	f := newLintFileWithRoot(t, sourcePath, dir)
	r := &Rule{Wikilinks: true, Placeholders: []string{"var-token"}}
	diags := r.Check(f)
	require.Empty(t, diags, "placeholder anchor must suppress the diagnostic")
}

func TestCheck_Wikilinks_RunCacheSharedIndex(t *testing.T) {
	// f.RunCache makes wikilinkIndexForRoot build a shared
	// WikilinkIndex; the resolver then takes the index branch
	// instead of per-call fs.WalkDir. Asserting "two Check calls
	// on different host files emit the same diagnostic" exercises
	// wikilinkIndexForRoot, wikilinkCacheKey, and the index path
	// in the resolver.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "present.md"), []byte("# P\n"), 0o644))
	docA := filepath.Join(dir, "a.md")
	docB := filepath.Join(dir, "b.md")
	writeFile(t, docA, "# A\n\n[[present]] [[missing]]\n")
	writeFile(t, docB, "# B\n\n[[present]] [[missing]]\n")

	cache := lint.NewRunCache()
	r := &Rule{Wikilinks: true}

	for _, p := range []string{docA, docB} {
		f := newLintFileWithRoot(t, p, dir)
		f.RunCache = cache
		diags := r.Check(f)
		require.Len(t, diags, 1, "missing wikilink must surface once per host file")
		require.Contains(t, diags[0].Message, "missing")
	}
}

func TestWikilinkIndexForRoot_EmptyKeyReturnsNil(t *testing.T) {
	// A file with RootFS set but RootDir empty hits the
	// wikilinkCacheKey == "" branch and short-circuits without
	// touching the RunCache. The resolver then falls back to the
	// per-Check walk.
	dir := t.TempDir()
	f := &lint.File{
		RootFS:   os.DirFS(dir),
		RunCache: lint.NewRunCache(),
	}
	got := wikilinkIndexForRoot(f, f.RootFS)
	assert.Nil(t, got)
}

func TestWikilinkIndexForRoot_NilRunCacheReturnsNil(t *testing.T) {
	dir := t.TempDir()
	f := &lint.File{RootDir: dir, RootFS: os.DirFS(dir)}
	assert.Nil(t, wikilinkIndexForRoot(f, f.RootFS),
		"no RunCache → no per-run index")
}

func TestWikilinkIndexForRoot_NilRootReturnsNil(t *testing.T) {
	f := &lint.File{RootDir: "/tmp/x", RunCache: lint.NewRunCache()}
	assert.Nil(t, wikilinkIndexForRoot(f, nil),
		"nil root → no per-run index")
}

func TestWikilinkCacheKey_FallbackOnUnknownDir(t *testing.T) {
	// filepath.Abs only errors when os.Getwd fails — unreachable in
	// the test harness — so this asserts the empty-RootDir branch
	// and the happy path. The error fallback stays defensive code.
	assert.Equal(t, "", wikilinkCacheKey(&lint.File{}))
	dir := t.TempDir()
	got := wikilinkCacheKey(&lint.File{RootDir: dir})
	assert.NotEmpty(t, got, "RootDir must produce a non-empty cache key")
}

func TestCheck_Wikilinks_PlaceholderSuppresses(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\nSee [[{topic}]] for context.\n")

	f := newLintFileWithRoot(t, sourcePath, dir)
	r := &Rule{
		Wikilinks:    true,
		Placeholders: []string{"var-token"},
	}
	diags := r.Check(f)
	require.Empty(t, diags, "placeholder targets must be skipped")
}

func TestCheck_Wikilinks_PlaceholderInAliasDoesNotSuppress(t *testing.T) {
	// MDS027's placeholder filter applies to link destinations, not
	// link text. A wikilink whose alias contains a placeholder must
	// still resolve the underlying target — a missing target stays
	// a diagnostic. Mirrors the standard-link behaviour where only
	// the URL half is checked for placeholder tokens.
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\nSee [[MissingPage|{topic}]] alias.\n")

	f := newLintFileWithRoot(t, sourcePath, dir)
	r := &Rule{
		Wikilinks:    true,
		Placeholders: []string{"var-token"},
	}
	diags := r.Check(f)
	require.Len(t, diags, 1, "alias-only placeholder must not mask a missing target")
	require.Contains(t, diags[0].Message, "MissingPage")
}

func TestCheck_Wikilinks_CodeBlockSkipped(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\n```\n[[Missing]]\n```\n")

	f := newLintFileWithRoot(t, sourcePath, dir)
	r := &Rule{Wikilinks: true}
	diags := r.Check(f)
	require.Empty(t, diags, "wikilinks inside fenced code must not be flagged")
}

func TestApplySettings_Wikilinks(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"wikilinks": true}))
	require.True(t, r.Wikilinks)

	err := r.ApplySettings(map[string]any{"wikilinks": "yes"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "wikilinks must be a bool")
}

func TestApplySettings_WikilinkStyle(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"wikilink-style": "obsidian"}))
	require.Equal(t, "obsidian", r.WikilinkStyle)

	err := r.ApplySettings(map[string]any{"wikilink-style": "foam"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not supported")

	// Non-string types are rejected before the value check runs;
	// reaching the "must be a string" branch keeps the error
	// taxonomy honest.
	err = r.ApplySettings(map[string]any{"wikilink-style": 123})
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be a string")
}

func newLintFileWithRoot(t *testing.T, path, root string) *lint.File {
	t.Helper()
	f := newLintFile(t, path)
	f.RootDir = root
	f.RootFS = os.DirFS(root)
	return f
}

func TestWikilinkRaw_Variants(t *testing.T) {
	cases := map[string]struct {
		wl   linkgraph.WikiLink
		want string
	}{
		"bare":         {linkgraph.WikiLink{Target: "Page"}, "[[Page]]"},
		"anchor":       {linkgraph.WikiLink{Target: "Page", Anchor: "Sec"}, "[[Page#Sec]]"},
		"alias":        {linkgraph.WikiLink{Target: "Page", Alias: "X"}, "[[Page|X]]"},
		"embed":        {linkgraph.WikiLink{Target: "img.png", Embed: true}, "![[img.png]]"},
		"anchor+alias": {linkgraph.WikiLink{Target: "Page", Anchor: "S", Alias: "X"}, "[[Page#S|X]]"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, wikilinkRaw(tc.wl))
		})
	}
}

func TestEffectiveWikilinkStyle(t *testing.T) {
	assert.Equal(t, "obsidian", (&Rule{}).effectiveWikilinkStyle())
	assert.Equal(t, "obsidian", (&Rule{WikilinkStyle: "obsidian"}).effectiveWikilinkStyle())
	// Set non-default via direct field write — ApplySettings rejects
	// unknown values, but the helper must still mirror whatever is
	// configured.
	assert.Equal(t, "custom", (&Rule{WikilinkStyle: "custom"}).effectiveWikilinkStyle())
}

func TestWikilinkRoot_Fallbacks(t *testing.T) {
	dir := t.TempDir()
	f := &lint.File{}
	assert.Nil(t, wikilinkRoot(f))

	f.RootDir = dir
	require.NotNil(t, wikilinkRoot(f))

	mfs := os.DirFS(dir)
	f.RootFS = mfs
	assert.Equal(t, fs.FS(mfs), wikilinkRoot(f))

	f.RootFS = nil
	f.RootDir = ""
	f.FS = mfs
	assert.Equal(t, fs.FS(mfs), wikilinkRoot(f))
}

func TestCheck_Wikilinks_RootMissing(t *testing.T) {
	// f.FS is nil → the rule short-circuits before calling
	// checkWikilinks; verify the top-level Check has the early return.
	f := &lint.File{Source: []byte("[[X]]\n")}
	diags := (&Rule{Wikilinks: true}).Check(f)
	assert.Nil(t, diags)
}

func TestResolver_UnknownStyleNoop(t *testing.T) {
	// A Rule constructed with an unknown style bypasses ApplySettings'
	// validation. The resolver must treat it as "no resolution" rather
	// than silently falling back to the Obsidian algorithm.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "page.md"), []byte{}, 0o644))
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\n[[page]]\n")
	f := newLintFileWithRoot(t, sourcePath, dir)
	r := &Rule{Wikilinks: true, WikilinkStyle: "foam-not-supported"}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	require.Contains(t, diags[0].Message, "not found in workspace")
}

func TestCheck_Wikilinks_UnreadableTarget(t *testing.T) {
	// MaxInputBytes set to a value below the target file's size makes
	// the anchor read fail; the rule must surface an unreadable-target
	// diagnostic rather than crash.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"),
		[]byte("# Notes\n\n## A\n## B\n## C\n"), 0o644))
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\n[[Notes#A]]\n")
	f := newLintFileWithRoot(t, sourcePath, dir)
	f.MaxInputBytes = 5
	r := &Rule{Wikilinks: true}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "cannot read wikilink target")
}

func TestWikilinkAnchorsForTarget_CacheHit(t *testing.T) {
	cache := map[string]map[string]struct{}{
		"wikilink:notes.md": {"x": {}},
	}
	f := &lint.File{}
	got, err := wikilinkAnchorsForTarget(f, nil, "notes.md", cache)
	require.NoError(t, err)
	assert.Contains(t, got, "x")
}

func TestWorkspaceRelativeSource_NoRootDir(t *testing.T) {
	f := &lint.File{Path: "/abs/path/doc.md"}
	got := workspaceRelativeSource(f)
	assert.Equal(t, "/abs/path/doc.md", got)
}

func TestWorkspaceRelativeSource_RelativePathAnchoredToRoot(t *testing.T) {
	// The LSP passes workspace-relative f.Path values (e.g.
	// "docs/notes.md"). Without explicit anchoring, filepath.Abs
	// would join against the process cwd — producing a path
	// outside RootDir that filepath.Rel would either reject or
	// turn into "../../<cwd>/docs/notes.md". The helper anchors
	// to RootDir first so the returned value is always the
	// expected workspace-relative form.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0o755))
	f := &lint.File{Path: "docs/notes.md", RootDir: dir}
	got := workspaceRelativeSource(f)
	assert.Equal(t, "docs/notes.md", got)
}

func TestWorkspaceRelativeSource_AbsolutePathUnderRoot(t *testing.T) {
	// CLI callers pass absolute paths. Those must collapse to
	// the workspace-relative form too.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0o755))
	f := &lint.File{
		Path:    filepath.Join(dir, "docs", "notes.md"),
		RootDir: dir,
	}
	got := workspaceRelativeSource(f)
	assert.Equal(t, "docs/notes.md", got)
}

func TestCheck_Wikilinks_ResolutionCachedPerTarget(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "page.md"), []byte("# P\n"), 0o644))
	sourcePath := filepath.Join(dir, "doc.md")
	writeFile(t, sourcePath, "# Doc\n\n[[page]] [[page]] [[page]] [[page]] [[page]]\n")

	counting := &walkCountingFS{inner: os.DirFS(dir)}

	f := newLintFile(t, sourcePath)
	f.RootDir = dir
	f.RootFS = counting

	r := &Rule{Wikilinks: true}
	diags := r.Check(f)
	require.Empty(t, diags)
	// One workspace walk does an Open(".") for Stat and again for the
	// recursion entry — two opens per walk. Five identical wikilinks
	// without caching would walk five times (10 opens); with the cache
	// they collapse to one walk (≤ 2 opens). Bound by the no-cache
	// floor to prove the cache is consulted.
	require.LessOrEqual(t, counting.rootOpens, 2,
		"resolver must memoize per target; got %d Open(\".\") calls", counting.rootOpens)
}

// walkCountingFS wraps an fs.FS and tallies Open(".") calls — the
// entry point fs.WalkDir uses for the root. Each ResolveWikiLink call
// triggers exactly two such opens (one for Stat, one for the walk).
// The test below uses this to prove the per-target cache reduces N
// repeated wikilinks down to a single walk.
type walkCountingFS struct {
	inner     fs.FS
	rootOpens int
}

func (w *walkCountingFS) Open(name string) (fs.File, error) {
	if name == "." {
		w.rootOpens++
	}
	return w.inner.Open(name)
}
