package crossfilereferenceintegrity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
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
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if !strings.Contains(diags[0].Message, "missing.md") {
		t.Fatalf("message = %q, want to include missing.md", diags[0].Message)
	}
}

func TestCheck_MissingHeadingAnchor(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "guide.md")
	sourcePath := filepath.Join(dir, "doc.md")

	writeFile(t, targetPath, "# Guide\n\n## Intro\n")
	writeFile(t, sourcePath, "# Doc\n\nSee [guide](guide.md#missing).\n")

	f := newLintFile(t, sourcePath)
	diags := (&Rule{}).Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if !strings.Contains(diags[0].Message, "guide.md#missing") {
		t.Fatalf("message = %q, want to include guide.md#missing", diags[0].Message)
	}
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
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d: %v", len(diags), diagMessages(diags))
	}
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
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d: %v", len(diags), diagMessages(diags))
	}
}

func TestCheck_DefaultSkipsNonMarkdownTargets(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")

	writeFile(t, sourcePath, "# Doc\n\n![Logo](missing.png)\n\nSee [asset](missing.png).\n")

	f := newLintFile(t, sourcePath)
	diags := (&Rule{}).Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d: %v", len(diags), diagMessages(diags))
	}
}

func TestCheck_StrictChecksNonMarkdownTargets(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "doc.md")

	writeFile(t, sourcePath, "# Doc\n\nSee [asset](missing.png).\n")

	f := newLintFile(t, sourcePath)
	r := &Rule{Strict: true}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
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
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %v", len(diags), diagMessages(diags))
	}
	if !strings.Contains(diags[0].Message, "docs/missing.md") {
		t.Fatalf("message = %q, want to include docs/missing.md", diags[0].Message)
	}
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
	if err != nil {
		t.Fatalf("ApplySettings returned error: %v", err)
	}

	if !r.Strict {
		t.Fatal("expected strict=true")
	}
	if len(r.Include) != 1 || r.Include[0] != "docs/**" {
		t.Fatalf("unexpected include: %v", r.Include)
	}
	if len(r.Exclude) != 1 || r.Exclude[0] != "docs/private/**" {
		t.Fatalf("unexpected exclude: %v", r.Exclude)
	}
}

func TestCheck_NoFS(t *testing.T) {
	f, err := lint.NewFile("stdin.md", []byte("# Doc\n\nSee [x](missing.md)\n"))
	if err != nil {
		t.Fatal(err)
	}

	diags := (&Rule{}).Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func newLintFile(t *testing.T, path string) *lint.File {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := lint.NewFile(path, data)
	if err != nil {
		t.Fatal(err)
	}
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
