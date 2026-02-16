package corpus

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourceAllowed(t *testing.T) {
	t.Parallel()

	allow := map[string]bool{"MIT": true}
	policy := QualityPolicy{MinStars: 10, MinRecentCommits90D: 2, RequireCI: true}
	base := SourceConfig{
		License: "MIT",
		Quality: SourceQuality{Stars: 10, RecentCommits90D: 2, HasCI: true},
	}

	ok, reason := sourceAllowed(policy, allow, base)
	if !ok || reason != "" {
		t.Fatalf("expected allowed source, got ok=%v reason=%q", ok, reason)
	}

	blocked, got := sourceAllowed(policy, allow, SourceConfig{License: "Apache-2.0", Quality: base.Quality})
	if blocked || got != "license" {
		t.Fatalf("expected license block, got ok=%v reason=%q", blocked, got)
	}
}

func TestIsGenerated(t *testing.T) {
	t.Parallel()

	if !isGenerated("docs/generated/file.md", "# x") {
		t.Fatal("expected generated path to be filtered")
	}
	if !isGenerated("docs/file.md", "Code generated. Do not edit.") {
		t.Fatal("expected generated marker to be filtered")
	}
	if isGenerated("docs/file.md", "# Human content") {
		t.Fatal("did not expect ordinary content to be generated")
	}
}

func TestIsLowSignal(t *testing.T) {
	t.Parallel()

	if !isLowSignal("tiny", 5, 10) {
		t.Fatal("expected short content to be low signal")
	}
	if isLowSignal("word word word word word", 3, 5) {
		t.Fatal("did not expect content to be low signal")
	}
}

func TestTokenSet(t *testing.T) {
	t.Parallel()

	set := tokenSet("Hello, world! hello\n`and` world")
	if _, ok := set["hello"]; !ok {
		t.Fatal("expected hello token")
	}
	if _, ok := set["world"]; !ok {
		t.Fatal("expected world token")
	}
	if _, ok := set["and"]; !ok {
		t.Fatal("expected and token")
	}
}

func TestCompileGlobPatterns(t *testing.T) {
	t.Parallel()

	patterns, err := compileGlobPatterns([]string{"docs/**/*.md"})
	if err != nil {
		t.Fatalf("compileGlobPatterns: %v", err)
	}
	if !matchesAny(patterns, "docs/a/b.md") {
		t.Fatal("expected glob to match")
	}

	_, err = compileGlobPatterns([]string{"["})
	if err == nil {
		t.Fatal("expected invalid glob error")
	}
}

func TestSourceMarkdownFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "docs/keep.md"), "# keep")
	mustWrite(t, filepath.Join(root, "docs/skip.md"), "# skip")
	mustWrite(t, filepath.Join(root, "notes.txt"), "x")

	source := SourceConfig{
		Name:    "seed",
		Root:    root,
		Include: []string{"docs/*.md"},
		Exclude: []string{"docs/skip.md"},
	}

	files, err := sourceMarkdownFiles(source)
	if err != nil {
		t.Fatalf("sourceMarkdownFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(files))
	}
	if filepath.Base(files[0]) != "keep.md" {
		t.Fatalf("kept file = %s, want keep.md", filepath.Base(files[0]))
	}
}

func mustWrite(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
