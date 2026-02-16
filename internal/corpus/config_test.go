package corpus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_DefaultsAndRootNormalization(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	repoDir := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	path := writeConfigFile(t, dir, `
collected_at: 2026-02-16
license_allowlist:
  - MIT
sources:
  - name: seed
    repository: github.com/acme/seed
    root: repo
    commit_sha: abc123
    license: MIT
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.DatasetVersion != "v0" {
		t.Fatalf("DatasetVersion = %q, want v0", cfg.DatasetVersion)
	}
	if cfg.Seed != defaultSeed {
		t.Fatalf("Seed = %d, want %d", cfg.Seed, defaultSeed)
	}
	if cfg.MinWords != defaultMinWords {
		t.Fatalf("MinWords = %d, want %d", cfg.MinWords, defaultMinWords)
	}
	if cfg.Sources[0].Root != repoDir {
		t.Fatalf("source root = %q, want %q", cfg.Sources[0].Root, repoDir)
	}
	if len(cfg.Sources[0].Include) != 2 {
		t.Fatalf("include count = %d, want 2", len(cfg.Sources[0].Include))
	}
}

func TestLoadConfig_InvalidDate(t *testing.T) {
	t.Parallel()

	path := writeConfigFile(t, t.TempDir(), `
collected_at: 02-16-2026
license_allowlist:
  - MIT
sources:
  - name: seed
    repository: github.com/acme/seed
    root: .
    commit_sha: abc123
    license: MIT
`)

	_, err := LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "collected_at must use YYYY-MM-DD") {
		t.Fatalf("expected collected_at error, got %v", err)
	}
}

func TestLoadConfig_DisallowedLicense(t *testing.T) {
	t.Parallel()

	path := writeConfigFile(t, t.TempDir(), `
collected_at: 2026-02-16
license_allowlist:
  - MIT
sources:
  - name: seed
    repository: github.com/acme/seed
    root: .
    commit_sha: abc123
    license: Apache-2.0
`)

	_, err := LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "not allowlisted") {
		t.Fatalf("expected allowlist error, got %v", err)
	}
}

func TestLoadConfig_UnknownBalanceCategory(t *testing.T) {
	t.Parallel()

	path := writeConfigFile(t, t.TempDir(), `
collected_at: 2026-02-16
license_allowlist:
  - MIT
balance:
  made-up:
    min: 0.1
    max: 0.2
sources:
  - name: seed
    repository: github.com/acme/seed
    root: .
    commit_sha: abc123
    license: MIT
`)

	_, err := LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "unknown balance category") {
		t.Fatalf("expected unknown category error, got %v", err)
	}
}

func writeConfigFile(t *testing.T, dir string, content string) string {
	t.Helper()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
