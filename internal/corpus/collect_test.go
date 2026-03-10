package corpus

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollect_HappyPath(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "docs")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	apiPath := filepath.Join(root, "api.md")
	apiContent := []byte("# API Reference\n\nword word word word word word\n")
	if err := os.WriteFile(apiPath, apiContent, 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "tiny.md"), []byte("small\n"), 0o644); err != nil {
		t.Fatalf("write tiny markdown: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("not markdown"), 0o644); err != nil {
		t.Fatalf("write text file: %v", err)
	}

	cfg := &Config{
		CollectedAt:      "2026-02-16",
		MinWords:         5,
		MinChars:         10,
		LicenseAllowlist: []string{"MIT"},
		Sources: []SourceConfig{{
			Name:       "seed",
			Repository: "github.com/acme/seed",
			Root:       root,
			CommitSHA:  "abc123",
			License:    "MIT",
		}},
	}

	records, err := Collect(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	record := records[0]
	if record.Source != "seed" {
		t.Fatalf("Source = %q, want seed", record.Source)
	}
	if record.Path != "api.md" {
		t.Fatalf("Path = %q, want api.md", record.Path)
	}
	if record.Words < 5 || record.Chars < 10 {
		t.Fatalf("unexpected counts: words=%d chars=%d", record.Words, record.Chars)
	}
	if record.RawContent == "" {
		t.Fatal("RawContent should be populated")
	}
}

func TestCollect_SkipsDisallowedLicense(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "doc.md"), []byte("# Doc\n\nword word word word\n"), 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}

	cfg := &Config{
		CollectedAt:      "2026-02-16",
		MinWords:         1,
		MinChars:         1,
		LicenseAllowlist: []string{"MIT"},
		Sources: []SourceConfig{{
			Name:       "seed",
			Repository: "github.com/acme/seed",
			Root:       root,
			CommitSHA:  "abc123",
			License:    "Apache-2.0",
		}},
	}

	records, err := Collect(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("record count = %d, want 0", len(records))
	}
}

func TestIsGenerated(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path string
		want bool
	}{
		// Should match generated/vendored paths.
		{"vendor/pkg/doc.md", true},
		{"node_modules/lib/README.md", true},
		{"dist/output.md", true},
		{"build/report.md", true},
		{"generated/api.md", true},
		{"gen/schema.md", true},
		{"deep/nested/vendor/pkg/doc.md", true},
		{"src/node_modules/lib/README.md", true},

		// Root-level directories (the leading "/" normalization catches these).
		{"vendor/doc.md", true},
		{"node_modules/doc.md", true},

		// Should NOT match: tokens that appear as substrings of other names.
		{"docs/general.md", false},
		{"src/adventure.md", false},
		{"src/api.md", false},
		{"README.md", false},
		{"docs/guide.md", false},
	}

	for _, tt := range tests {
		if got := isGenerated(tt.path); got != tt.want {
			t.Errorf("isGenerated(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestCollect_SkipsGeneratedDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	// Create a normal markdown file that should be collected.
	docsDir := filepath.Join(root, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	content := []byte("# Guide\n\nword word word word word word\n")
	if err := os.WriteFile(filepath.Join(docsDir, "guide.md"), content, 0o644); err != nil {
		t.Fatalf("write guide.md: %v", err)
	}

	// Create markdown files in generated/vendored directories that should be skipped.
	for _, dir := range []string{"vendor/pkg", "node_modules/lib", "dist", "build", "generated", "gen"} {
		dirPath := filepath.Join(root, dir)
		if err := os.MkdirAll(dirPath, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dirPath, "doc.md"), content, 0o644); err != nil {
			t.Fatalf("write %s/doc.md: %v", dir, err)
		}
	}

	cfg := &Config{
		CollectedAt:      "2026-02-16",
		MinWords:         1,
		MinChars:         1,
		LicenseAllowlist: []string{"MIT"},
		Sources: []SourceConfig{{
			Name:       "seed",
			Repository: "github.com/acme/seed",
			Root:       root,
			CommitSHA:  "abc123",
			License:    "MIT",
		}},
	}

	records, err := Collect(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(records) != 1 {
		paths := make([]string, len(records))
		for i, r := range records {
			paths[i] = r.Path
		}
		t.Fatalf("record count = %d, want 1 (only docs/guide.md); got paths %v", len(records), paths)
	}
	if records[0].Path != "docs/guide.md" {
		t.Fatalf("Path = %q, want docs/guide.md", records[0].Path)
	}
}

func TestCollect_ErrorPath(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		CollectedAt:      "2026-02-16",
		MinWords:         1,
		MinChars:         1,
		LicenseAllowlist: []string{"MIT"},
		Sources: []SourceConfig{{
			Name:       "seed",
			Repository: "github.com/acme/seed",
			Root:       filepath.Join(t.TempDir(), "missing"),
			CommitSHA:  "abc123",
			License:    "MIT",
		}},
	}

	_, err := Collect(cfg, t.TempDir())
	if err == nil {
		t.Fatal("expected resolve error")
	}
}
