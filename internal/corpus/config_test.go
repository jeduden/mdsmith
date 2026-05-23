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

func TestLoadConfig_RejectsYAMLAnchor(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	writeFile(t, path, "base: &base\n  min_words: 30\nsources: *base\n")

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for YAML anchor, got nil")
	}
	if !strings.Contains(err.Error(), "anchors/aliases are not permitted") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMergeLocalOverrides_RejectsAnchor(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	cfgPath := filepath.Join(dir, "config.yml")
	writeFile(t, cfgPath, `
collected_at: 2026-02-16
license_allowlist:
  - MIT
sources:
  - name: seed
    url: https://example.com/seed.git
`)

	localPath := filepath.Join(dir, "config.local.yml")
	writeFile(t, localPath, "base: &base\n  name: evil\nsources:\n  - <<: *base\n")

	_, err := LoadConfig(cfgPath)
	if err == nil {
		t.Fatal("expected error for local override anchor")
	}
	if !strings.Contains(err.Error(), "anchors/aliases are not permitted") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- validateConfigHeader ---

// TestValidateConfigHeader pins every guard the helper enforces.
// LoadConfig drives the happy path; each error branch — missing
// collected_at, malformed date, sub-1 min_words / min_chars /
// qa_sample_limit, out-of-range test_fraction, empty license
// list, empty sources — needs a direct unit pin. The case table
// is the natural shape; funlen exception applies.
//
//nolint:funlen // table-driven validator test
func TestValidateConfigHeader(t *testing.T) {
	t.Parallel()
	// Cheap, complete baseline; each case below mutates one field.
	base := func() Config {
		return Config{
			CollectedAt:      "2026-01-02",
			MinWords:         1,
			MinChars:         1,
			TestFraction:     0.2,
			QASampleLimit:    1,
			LicenseAllowlist: []string{"MIT"},
			Sources:          []SourceConfig{{Name: "x"}},
		}
	}
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantSub string
	}{
		{name: "missing collected_at",
			mutate:  func(c *Config) { c.CollectedAt = "" },
			wantSub: "collected_at is required"},
		{name: "malformed date",
			mutate:  func(c *Config) { c.CollectedAt = "not-a-date" },
			wantSub: "collected_at must use YYYY-MM-DD"},
		{name: "zero min_words",
			mutate:  func(c *Config) { c.MinWords = 0 },
			wantSub: "min_words"},
		{name: "negative min_chars",
			mutate:  func(c *Config) { c.MinChars = -1 },
			wantSub: "min_chars"},
		{name: "test_fraction zero",
			mutate:  func(c *Config) { c.TestFraction = 0 },
			wantSub: "test_fraction"},
		{name: "test_fraction one",
			mutate:  func(c *Config) { c.TestFraction = 1 },
			wantSub: "test_fraction"},
		{name: "qa_sample_limit zero",
			mutate:  func(c *Config) { c.QASampleLimit = 0 },
			wantSub: "qa_sample_limit"},
		{name: "empty license list",
			mutate:  func(c *Config) { c.LicenseAllowlist = nil },
			wantSub: "license_allowlist"},
		{name: "empty sources",
			mutate:  func(c *Config) { c.Sources = nil },
			wantSub: "at least one source"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := base()
			c.mutate(&cfg)
			err := validateConfigHeader(cfg)
			if err == nil {
				t.Fatalf("expected error for %s", c.name)
			}
			if !strings.Contains(err.Error(), c.wantSub) {
				t.Errorf("err %q must contain %q", err, c.wantSub)
			}
		})
	}
	t.Run("happy path", func(t *testing.T) {
		if err := validateConfigHeader(base()); err != nil {
			t.Errorf("unexpected error on valid config: %v", err)
		}
	})
}

// --- validateSource ---

// TestValidateSource pins every error branch the per-source
// validator enforces: missing name / repo / root / commit /
// license, license not in allowlist, duplicate name. LoadConfig
// only drives the happy path on a hand-crafted YAML; the
// per-field errors were uncovered.
func TestValidateSource(t *testing.T) {
	t.Parallel()
	allow := map[string]struct{}{"MIT": {}}
	base := SourceConfig{
		Name: "seed", Repository: "github.com/x/y", Root: "docs",
		CommitSHA: "deadbeef", License: "MIT",
	}
	cases := []struct {
		name    string
		mutate  func(*SourceConfig)
		seen    map[string]struct{}
		wantSub string
	}{
		{name: "missing name",
			mutate:  func(s *SourceConfig) { s.Name = "  " },
			wantSub: "name is required"},
		{name: "duplicate name",
			mutate:  func(*SourceConfig) {},
			seen:    map[string]struct{}{"seed": {}},
			wantSub: "duplicate source name"},
		{name: "missing repository",
			mutate:  func(s *SourceConfig) { s.Repository = "" },
			wantSub: "repository is required"},
		{name: "missing root",
			mutate:  func(s *SourceConfig) { s.Root = "" },
			wantSub: "root is required"},
		{name: "missing commit_sha",
			mutate:  func(s *SourceConfig) { s.CommitSHA = "" },
			wantSub: "commit_sha is required"},
		{name: "missing license",
			mutate:  func(s *SourceConfig) { s.License = "" },
			wantSub: "license is required"},
		{name: "license not allowlisted",
			mutate:  func(s *SourceConfig) { s.License = "GPL-3.0" },
			wantSub: "not allowlisted"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := base
			c.mutate(&src)
			seen := c.seen
			if seen == nil {
				seen = map[string]struct{}{}
			}
			err := validateSource(src, allow, seen)
			if err == nil {
				t.Fatalf("expected error for %s", c.name)
			}
			if !strings.Contains(err.Error(), c.wantSub) {
				t.Errorf("err %q must contain %q", err, c.wantSub)
			}
		})
	}
	t.Run("happy path", func(t *testing.T) {
		seen := map[string]struct{}{}
		if err := validateSource(base, allow, seen); err != nil {
			t.Errorf("unexpected error on valid source: %v", err)
		}
	})
}
