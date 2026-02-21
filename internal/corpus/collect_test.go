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
