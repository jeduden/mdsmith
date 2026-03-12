package corpus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_AppliesDefaults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	writeFile(t, path, `
collected_at: 2026-02-16
license_allowlist:
  - MIT
sources:
  - name: seed
    repository: github.com/acme/seed
    root: docs
    commit_sha: abc123
    license: MIT
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.DatasetVersion != "v2026-02-16" {
		t.Fatalf("DatasetVersion = %q, want v2026-02-16", cfg.DatasetVersion)
	}
	if cfg.MinWords != defaultMinWords {
		t.Fatalf("MinWords = %d, want %d", cfg.MinWords, defaultMinWords)
	}
	if cfg.MinChars != defaultMinChars {
		t.Fatalf("MinChars = %d, want %d", cfg.MinChars, defaultMinChars)
	}
	if cfg.TestFraction != defaultTestFraction {
		t.Fatalf("TestFraction = %f, want %f", cfg.TestFraction, defaultTestFraction)
	}
	if cfg.QASampleLimit != defaultQASampleLimit {
		t.Fatalf("QASampleLimit = %d, want %d", cfg.QASampleLimit, defaultQASampleLimit)
	}
}

func TestLoadConfig_MergesLocalOverrides(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	localRoot := filepath.Join(dir, "local-docs")
	if err := os.MkdirAll(localRoot, 0o755); err != nil {
		t.Fatalf("mkdir local root: %v", err)
	}

	configPath := filepath.Join(dir, "config.yml")
	writeFile(t, configPath, `
collected_at: 2026-02-16
license_allowlist:
  - MIT
sources:
  - name: seed
    repository: github.com/acme/seed
    root: docs
    commit_sha: abc123
    license: MIT
`)
	writeFile(t, filepath.Join(dir, "config.local.yml"), "sources:\n  - name: seed\n    root: "+localRoot+"\n")

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Sources[0].Root != localRoot {
		t.Fatalf("merged root = %q, want %q", cfg.Sources[0].Root, localRoot)
	}
	if !cfg.ResolvedFromLocal {
		t.Fatal("ResolvedFromLocal = false, want true")
	}
}

func TestLoadConfig_ValidationErrors(t *testing.T) {
	t.Parallel()

	t.Run("missing date", func(t *testing.T) {
		t.Parallel()
		assertLoadConfigError(t, `
license_allowlist:
  - MIT
sources:
  - name: seed
    repository: github.com/acme/seed
    root: docs
    commit_sha: abc123
    license: MIT
`, "collected_at is required")
	})

	t.Run("bad date format", func(t *testing.T) {
		t.Parallel()
		assertLoadConfigError(t, `
collected_at: 16-02-2026
license_allowlist:
  - MIT
sources:
  - name: seed
    repository: github.com/acme/seed
    root: docs
    commit_sha: abc123
    license: MIT
`, "collected_at must use YYYY-MM-DD")
	})

	t.Run("license not allowlisted", func(t *testing.T) {
		t.Parallel()
		assertLoadConfigError(t, `
collected_at: 2026-02-16
license_allowlist:
  - MIT
sources:
  - name: seed
    repository: github.com/acme/seed
    root: docs
    commit_sha: abc123
    license: Apache-2.0
`, "not allowlisted")
	})
}

func assertLoadConfigError(t *testing.T, config string, wantErr string) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	writeFile(t, path, config)

	_, err := LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("expected error containing %q, got %v", wantErr, err)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}
