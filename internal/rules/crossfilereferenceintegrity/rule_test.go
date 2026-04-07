package crossfilereferenceintegrity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
