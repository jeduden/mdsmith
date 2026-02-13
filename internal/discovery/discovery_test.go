package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatalf("creating directory %s: %v", parent, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func TestDiscover_FindsMDFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "README.md", "# Hello\n")
	writeFile(t, dir, "docs/guide.md", "# Guide\n")
	writeFile(t, dir, "src/main.go", "package main\n")

	files, err := Discover(Options{
		Patterns: []string{"**/*.md"},
		BaseDir:  dir,
	})
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}

	// Should include both .md files.
	found := make(map[string]bool)
	for _, f := range files {
		found[filepath.Base(f)] = true
	}
	if !found["README.md"] {
		t.Error("expected README.md in results")
	}
	if !found["guide.md"] {
		t.Error("expected guide.md in results")
	}
}

func TestDiscover_FindsMarkdownExtension(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "README.md", "# Hello\n")
	writeFile(t, dir, "guide.markdown", "# Guide\n")

	files, err := Discover(Options{
		Patterns: []string{"**/*.md", "**/*.markdown"},
		BaseDir:  dir,
	})
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}
}

func TestDiscover_EmptyPatterns(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "README.md", "# Hello\n")

	files, err := Discover(Options{
		Patterns: []string{},
		BaseDir:  dir,
	})
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("expected 0 files with empty patterns, got %d: %v", len(files), files)
	}
}

func TestDiscover_NilPatterns(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "README.md", "# Hello\n")

	files, err := Discover(Options{
		Patterns: nil,
		BaseDir:  dir,
	})
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("expected 0 files with nil patterns, got %d: %v", len(files), files)
	}
}

func TestDiscover_GitignoreRespected(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "README.md", "# Hello\n")
	writeFile(t, dir, "vendor/lib.md", "# Lib\n")
	writeFile(t, dir, ".gitignore", "vendor/\n")

	files, err := Discover(Options{
		Patterns:     []string{"**/*.md"},
		BaseDir:      dir,
		UseGitignore: true,
	})
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file (vendor ignored), got %d: %v", len(files), files)
	}
	if filepath.Base(files[0]) != "README.md" {
		t.Errorf("expected README.md, got %s", files[0])
	}
}

func TestDiscover_NoGitignoreIncludesAll(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "README.md", "# Hello\n")
	writeFile(t, dir, "vendor/lib.md", "# Lib\n")
	writeFile(t, dir, ".gitignore", "vendor/\n")

	files, err := Discover(Options{
		Patterns:     []string{"**/*.md"},
		BaseDir:      dir,
		UseGitignore: false,
	})
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 files (gitignore disabled), got %d: %v", len(files), files)
	}
}

func TestDiscover_ResultsSorted(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "c.md", "# C\n")
	writeFile(t, dir, "a.md", "# A\n")
	writeFile(t, dir, "b.md", "# B\n")

	files, err := Discover(Options{
		Patterns: []string{"**/*.md"},
		BaseDir:  dir,
	})
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}

	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}

	for i := 1; i < len(files); i++ {
		if files[i] < files[i-1] {
			t.Errorf("results not sorted: %v", files)
			break
		}
	}
}

func TestDiscover_SubdirectoryPattern(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "docs/guide.md", "# Guide\n")
	writeFile(t, dir, "docs/api/ref.md", "# Ref\n")
	writeFile(t, dir, "README.md", "# Hello\n")

	files, err := Discover(Options{
		Patterns: []string{"docs/**/*.md"},
		BaseDir:  dir,
	})
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 files from docs/, got %d: %v", len(files), files)
	}

	// README.md should not be included.
	for _, f := range files {
		if filepath.Base(f) == "README.md" {
			t.Error("README.md should not match docs/**/*.md pattern")
		}
	}
}

func TestDiscover_ExactFilePattern(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "README.md", "# Hello\n")
	writeFile(t, dir, "CHANGELOG.md", "# Changes\n")

	files, err := Discover(Options{
		Patterns: []string{"README.md"},
		BaseDir:  dir,
	})
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d: %v", len(files), files)
	}
	if filepath.Base(files[0]) != "README.md" {
		t.Errorf("expected README.md, got %s", files[0])
	}
}

func TestDiscover_NoDuplicates(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "README.md", "# Hello\n")

	// Multiple patterns that match the same file.
	files, err := Discover(Options{
		Patterns: []string{"**/*.md", "README.md"},
		BaseDir:  dir,
	})
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file (no duplicates), got %d: %v", len(files), files)
	}
}
