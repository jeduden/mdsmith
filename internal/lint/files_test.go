package lint

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestResolveFiles_SingleFile(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	if err := os.WriteFile(mdFile, []byte("# Hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := ResolveFiles([]string{mdFile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0] != mdFile {
		t.Errorf("expected %q, got %q", mdFile, files[0])
	}
}

func TestResolveFiles_NonMarkdownFile(t *testing.T) {
	dir := t.TempDir()
	txtFile := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(txtFile, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Non-markdown files are still returned when given explicitly as args.
	files, err := ResolveFiles([]string{txtFile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
}

func TestResolveFiles_Directory(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create markdown files at various levels.
	for _, name := range []string{
		filepath.Join(dir, "a.md"),
		filepath.Join(dir, "b.markdown"),
		filepath.Join(dir, "c.txt"), // should be excluded
		filepath.Join(subDir, "d.md"),
	} {
		if err := os.WriteFile(name, []byte("# Test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	files, err := ResolveFiles([]string{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should find a.md, b.markdown, sub/d.md (not c.txt).
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d: %v", len(files), files)
	}

	// Check that all returned files are markdown.
	for _, f := range files {
		ext := filepath.Ext(f)
		if ext != ".md" && ext != ".markdown" {
			t.Errorf("unexpected non-markdown file: %s", f)
		}
	}
}

func TestResolveFiles_GlobPattern(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"a.md", "b.md", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("# Test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	pattern := filepath.Join(dir, "*.md")
	files, err := ResolveFiles([]string{pattern})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}
}

func TestResolveFiles_NonexistentPath(t *testing.T) {
	_, err := ResolveFiles([]string{"/nonexistent/path/file.md"})
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}

func TestResolveFiles_EmptyArgs(t *testing.T) {
	files, err := ResolveFiles([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(files))
	}
}

func TestResolveFiles_NilArgs(t *testing.T) {
	files, err := ResolveFiles(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(files))
	}
}

func TestResolveFiles_Deduplicated(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	if err := os.WriteFile(mdFile, []byte("# Hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Pass the same file twice.
	files, err := ResolveFiles([]string{mdFile, mdFile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file (deduplicated), got %d", len(files))
	}
}

func TestResolveFiles_Sorted(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"z.md", "a.md", "m.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("# Test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	files, err := ResolveFiles([]string{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sort.StringsAreSorted(files) {
		t.Errorf("expected sorted files, got %v", files)
	}
}

func TestResolveFiles_MarkdownExtension(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "doc.markdown")
	if err := os.WriteFile(mdFile, []byte("# Hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := ResolveFiles([]string{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if filepath.Ext(files[0]) != ".markdown" {
		t.Errorf("expected .markdown extension, got %s", filepath.Ext(files[0]))
	}
}

func TestResolveFiles_GlobMatchingDirectory(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "docs")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "guide.md"), []byte("# Guide"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Glob that matches a directory should recurse into it.
	pattern := filepath.Join(dir, "doc*")
	files, err := ResolveFiles([]string{pattern})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d: %v", len(files), files)
	}
}
