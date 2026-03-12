package corpus

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuild_HappyPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeCorpusFile(t, filepath.Join(root, "docs", "reference.md"), "# API Reference\n\nword word word word word word")
	writeCorpusFile(t, filepath.Join(root, "docs", "copy.md"), "# API Reference\n\nword word word word word word")
	writeCorpusFile(t, filepath.Join(root, "docs", "guide.md"), "# Guide\n\nword word word word word word")

	cfg := &Config{
		DatasetVersion:   "v2026-02-16",
		CollectedAt:      "2026-02-16",
		MinWords:         5,
		MinChars:         10,
		TestFraction:     0.2,
		QASampleLimit:    10,
		LicenseAllowlist: []string{"MIT"},
		Sources: []SourceConfig{{
			Name:       "seed",
			Repository: "github.com/acme/seed",
			Root:       root,
			CommitSHA:  "abc123",
			License:    "MIT",
		}},
	}

	result, err := Build(cfg, t.TempDir())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if result.Report.DatasetVersion != "v2026-02-16" {
		t.Fatalf("DatasetVersion = %q, want v2026-02-16", result.Report.DatasetVersion)
	}
	if result.Report.FilesCollected != 3 {
		t.Fatalf("FilesCollected = %d, want 3", result.Report.FilesCollected)
	}
	if result.Report.FilesDeduped != 1 {
		t.Fatalf("FilesDeduped = %d, want 1", result.Report.FilesDeduped)
	}
	if result.Report.FilesKept != 2 {
		t.Fatalf("FilesKept = %d, want 2", result.Report.FilesKept)
	}
	if len(result.Manifest) != 2 {
		t.Fatalf("manifest len = %d, want 2", len(result.Manifest))
	}
	if len(result.QASample) == 0 {
		t.Fatal("expected non-empty QA sample")
	}
	if len(result.QASample) > 10 {
		t.Fatalf("qa sample len = %d, want <= 10", len(result.QASample))
	}
}

func TestBuild_ErrorPath(t *testing.T) {
	t.Parallel()

	_, err := Build(nil, t.TempDir())
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func writeCorpusFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
