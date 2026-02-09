package config

import (
	"os"
	"path/filepath"
	"testing"
)

// --- YAML parsing tests ---

func TestParseValidYAML(t *testing.T) {
	yml := `
rules:
  line-length: true
  heading-style: false
  no-multiple-blanks:
    max: 2
ignore:
  - "vendor/**"
  - "node_modules/**"
overrides:
  - files:
      - "CHANGELOG.md"
    rules:
      no-duplicate-headings: false
  - files:
      - "docs/**"
    rules:
      line-length:
        max: 120
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".tidymark.yml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	// Check rules
	if len(cfg.Rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(cfg.Rules))
	}
	if !cfg.Rules["line-length"].Enabled {
		t.Error("line-length should be enabled")
	}
	if cfg.Rules["heading-style"].Enabled {
		t.Error("heading-style should be disabled")
	}
	if !cfg.Rules["no-multiple-blanks"].Enabled {
		t.Error("no-multiple-blanks should be enabled")
	}
	if cfg.Rules["no-multiple-blanks"].Settings["max"] != 2 {
		t.Errorf("no-multiple-blanks max: expected 2, got %v", cfg.Rules["no-multiple-blanks"].Settings["max"])
	}

	// Check ignore
	if len(cfg.Ignore) != 2 {
		t.Fatalf("expected 2 ignore patterns, got %d", len(cfg.Ignore))
	}
	if cfg.Ignore[0] != "vendor/**" {
		t.Errorf("expected vendor/**, got %s", cfg.Ignore[0])
	}

	// Check overrides
	if len(cfg.Overrides) != 2 {
		t.Fatalf("expected 2 overrides, got %d", len(cfg.Overrides))
	}
	if cfg.Overrides[0].Files[0] != "CHANGELOG.md" {
		t.Errorf("expected CHANGELOG.md, got %s", cfg.Overrides[0].Files[0])
	}
	if cfg.Overrides[0].Rules["no-duplicate-headings"].Enabled {
		t.Error("no-duplicate-headings should be disabled in override")
	}
	if !cfg.Overrides[1].Rules["line-length"].Enabled {
		t.Error("line-length should be enabled in override")
	}
	if cfg.Overrides[1].Rules["line-length"].Settings["max"] != 120 {
		t.Errorf("line-length max in override: expected 120, got %v",
			cfg.Overrides[1].Rules["line-length"].Settings["max"])
	}
}

func TestRuleCfgBoolFalse(t *testing.T) {
	yml := `
rules:
  line-length: false
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".tidymark.yml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	rc := cfg.Rules["line-length"]
	if rc.Enabled {
		t.Error("expected Enabled=false")
	}
	if rc.Settings != nil {
		t.Error("expected Settings=nil")
	}
}

func TestRuleCfgBoolTrue(t *testing.T) {
	yml := `
rules:
  line-length: true
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".tidymark.yml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	rc := cfg.Rules["line-length"]
	if !rc.Enabled {
		t.Error("expected Enabled=true")
	}
	if rc.Settings != nil {
		t.Error("expected Settings=nil")
	}
}

func TestRuleCfgObject(t *testing.T) {
	yml := `
rules:
  line-length:
    max: 120
    strict: true
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".tidymark.yml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	rc := cfg.Rules["line-length"]
	if !rc.Enabled {
		t.Error("expected Enabled=true")
	}
	if rc.Settings == nil {
		t.Fatal("expected Settings to be non-nil")
	}
	if rc.Settings["max"] != 120 {
		t.Errorf("expected max=120, got %v", rc.Settings["max"])
	}
	if rc.Settings["strict"] != true {
		t.Errorf("expected strict=true, got %v", rc.Settings["strict"])
	}
}

func TestInvalidYAMLReturnsError(t *testing.T) {
	yml := `
rules:
  line-length: [[[invalid
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".tidymark.yml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadNonexistentFile(t *testing.T) {
	_, err := Load("/nonexistent/path/.tidymark.yml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

// --- Discovery tests ---

func TestDiscoverFindsInCurrentDir(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, configFileName)
	if err := os.WriteFile(cfgPath, []byte("rules: {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if found != cfgPath {
		t.Errorf("expected %s, got %s", cfgPath, found)
	}
}

func TestDiscoverFindsInParentDir(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "subdir")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(parent, configFileName)
	if err := os.WriteFile(cfgPath, []byte("rules: {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := Discover(child)
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if found != cfgPath {
		t.Errorf("expected %s, got %s", cfgPath, found)
	}
}

func TestDiscoverStopsAtGitBoundary(t *testing.T) {
	// Setup: grandparent has config, parent has .git, child is startDir.
	// Discover should NOT find the config above .git.
	grandparent := t.TempDir()
	parent := filepath.Join(grandparent, "repo")
	child := filepath.Join(parent, "src")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}

	// Put .git in parent (the repo root)
	gitDir := filepath.Join(parent, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Put config in grandparent (above .git)
	cfgPath := filepath.Join(grandparent, configFileName)
	if err := os.WriteFile(cfgPath, []byte("rules: {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := Discover(child)
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if found != "" {
		t.Errorf("expected empty string (stopped at .git), got %s", found)
	}
}

func TestDiscoverStopsAtGitBoundaryWithConfigInRepo(t *testing.T) {
	// Config in same dir as .git should be found.
	repoRoot := t.TempDir()
	child := filepath.Join(repoRoot, "src")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}

	gitDir := filepath.Join(repoRoot, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(repoRoot, configFileName)
	if err := os.WriteFile(cfgPath, []byte("rules: {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := Discover(child)
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if found != cfgPath {
		t.Errorf("expected %s, got %s", cfgPath, found)
	}
}

func TestDiscoverReturnsEmptyWhenNotFound(t *testing.T) {
	dir := t.TempDir()
	// Put a .git so we don't walk out of the tmp dir
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}

	found, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if found != "" {
		t.Errorf("expected empty string, got %s", found)
	}
}

// --- Defaults tests ---

func TestDefaultsAllRulesEnabled(t *testing.T) {
	cfg := Defaults()
	expectedRules := []string{
		"line-length",
		"heading-style",
		"heading-increment",
		"first-line-heading",
		"no-duplicate-headings",
		"no-trailing-spaces",
		"no-hard-tabs",
		"no-multiple-blanks",
		"single-trailing-newline",
		"fenced-code-style",
		"fenced-code-language",
		"no-bare-urls",
		"blank-line-around-headings",
		"blank-line-around-lists",
		"blank-line-around-fenced-code",
		"list-indent",
		"no-trailing-punctuation-in-heading",
		"no-emphasis-as-heading",
	}

	if len(cfg.Rules) != 19 {
		t.Fatalf("expected 19 rules, got %d", len(cfg.Rules))
	}

	for _, name := range expectedRules {
		rc, ok := cfg.Rules[name]
		if !ok {
			t.Errorf("rule %q not found in defaults", name)
			continue
		}
		if !rc.Enabled {
			t.Errorf("rule %q should be enabled by default", name)
		}
		if rc.Settings != nil {
			t.Errorf("rule %q should have nil settings by default", name)
		}
	}
}

// --- Merge tests ---

func TestMergeNilLoaded(t *testing.T) {
	defaults := Defaults()
	merged := Merge(defaults, nil)

	if len(merged.Rules) != 19 {
		t.Fatalf("expected 19 rules, got %d", len(merged.Rules))
	}
	for name, rc := range merged.Rules {
		if !rc.Enabled {
			t.Errorf("rule %q should be enabled", name)
		}
	}
}

func TestMergeDisabledRule(t *testing.T) {
	defaults := Defaults()
	loaded := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: false},
		},
	}

	merged := Merge(defaults, loaded)

	if merged.Rules["line-length"].Enabled {
		t.Error("line-length should be disabled after merge")
	}

	// Other rules should still be enabled
	if !merged.Rules["heading-style"].Enabled {
		t.Error("heading-style should remain enabled")
	}
	if !merged.Rules["no-trailing-spaces"].Enabled {
		t.Error("no-trailing-spaces should remain enabled")
	}
}

func TestMergeCustomSettings(t *testing.T) {
	defaults := Defaults()
	loaded := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {
				Enabled:  true,
				Settings: map[string]any{"max": 120},
			},
		},
	}

	merged := Merge(defaults, loaded)

	rc := merged.Rules["line-length"]
	if !rc.Enabled {
		t.Error("line-length should be enabled")
	}
	if rc.Settings["max"] != 120 {
		t.Errorf("expected max=120, got %v", rc.Settings["max"])
	}
}

func TestMergePreservesIgnoreAndOverrides(t *testing.T) {
	defaults := Defaults()
	loaded := &Config{
		Ignore: []string{"vendor/**"},
		Overrides: []Override{
			{
				Files: []string{"CHANGELOG.md"},
				Rules: map[string]RuleCfg{
					"no-duplicate-headings": {Enabled: false},
				},
			},
		},
	}

	merged := Merge(defaults, loaded)

	if len(merged.Ignore) != 1 || merged.Ignore[0] != "vendor/**" {
		t.Errorf("ignore not preserved: %v", merged.Ignore)
	}
	if len(merged.Overrides) != 1 {
		t.Fatalf("expected 1 override, got %d", len(merged.Overrides))
	}
}

// --- Effective tests ---

func TestEffectiveWithoutOverrides(t *testing.T) {
	cfg := Defaults()
	eff := Effective(cfg, "README.md")

	if len(eff) != 19 {
		t.Fatalf("expected 19 rules, got %d", len(eff))
	}
	for name, rc := range eff {
		if !rc.Enabled {
			t.Errorf("rule %q should be enabled", name)
		}
	}
}

func TestEffectiveOverrideAppliesPerFile(t *testing.T) {
	cfg := Defaults()
	cfg.Overrides = []Override{
		{
			Files: []string{"CHANGELOG.md"},
			Rules: map[string]RuleCfg{
				"no-duplicate-headings": {Enabled: false},
			},
		},
	}

	// CHANGELOG.md should have no-duplicate-headings disabled
	eff := Effective(cfg, "CHANGELOG.md")
	if eff["no-duplicate-headings"].Enabled {
		t.Error("no-duplicate-headings should be disabled for CHANGELOG.md")
	}
	if !eff["line-length"].Enabled {
		t.Error("line-length should remain enabled for CHANGELOG.md")
	}

	// README.md should NOT be affected
	eff2 := Effective(cfg, "README.md")
	if !eff2["no-duplicate-headings"].Enabled {
		t.Error("no-duplicate-headings should remain enabled for README.md")
	}
}

func TestEffectiveLaterOverridesWin(t *testing.T) {
	cfg := Defaults()
	cfg.Overrides = []Override{
		{
			Files: []string{"docs/**"},
			Rules: map[string]RuleCfg{
				"line-length": {
					Enabled:  true,
					Settings: map[string]any{"max": 100},
				},
			},
		},
		{
			Files: []string{"docs/api/**"},
			Rules: map[string]RuleCfg{
				"line-length": {
					Enabled:  true,
					Settings: map[string]any{"max": 200},
				},
			},
		},
	}

	// docs/api/foo.md matches both overrides; second should win
	eff := Effective(cfg, "docs/api/foo.md")
	rc := eff["line-length"]
	if !rc.Enabled {
		t.Error("line-length should be enabled")
	}
	if rc.Settings["max"] != 200 {
		t.Errorf("expected max=200 (later override wins), got %v", rc.Settings["max"])
	}
}

func TestFrontMatterParsing(t *testing.T) {
	yml := `
front-matter: true
rules:
  line-length: true
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".tidymark.yml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.FrontMatter == nil {
		t.Fatal("expected FrontMatter to be non-nil")
	}
	if !*cfg.FrontMatter {
		t.Error("expected FrontMatter to be true")
	}
}

func TestFrontMatterFalse(t *testing.T) {
	yml := `
front-matter: false
rules:
  line-length: true
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".tidymark.yml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.FrontMatter == nil {
		t.Fatal("expected FrontMatter to be non-nil")
	}
	if *cfg.FrontMatter {
		t.Error("expected FrontMatter to be false")
	}
}

func TestFrontMatterOmitted(t *testing.T) {
	yml := `
rules:
  line-length: true
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".tidymark.yml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.FrontMatter != nil {
		t.Errorf("expected FrontMatter nil when omitted, got %v", *cfg.FrontMatter)
	}
}

func TestMergeFrontMatter(t *testing.T) {
	defaults := Defaults()

	// Loaded config sets front-matter: false
	fm := false
	loaded := &Config{
		FrontMatter: &fm,
	}

	merged := Merge(defaults, loaded)
	if merged.FrontMatter == nil || *merged.FrontMatter {
		t.Error("expected FrontMatter=false after merge")
	}

	// Loaded config omits front-matter — defaults should apply
	loaded2 := &Config{}
	merged2 := Merge(defaults, loaded2)
	if merged2.FrontMatter != nil {
		t.Error("expected FrontMatter=nil when not set in loaded config")
	}
}

func TestEffectiveGlobPatternMatch(t *testing.T) {
	cfg := Defaults()
	cfg.Overrides = []Override{
		{
			Files: []string{"vendor/**"},
			Rules: map[string]RuleCfg{
				"line-length": {Enabled: false},
			},
		},
	}

	eff := Effective(cfg, "vendor/foo/bar.md")
	if eff["line-length"].Enabled {
		t.Error("line-length should be disabled for vendor/foo/bar.md")
	}

	// Non-matching file
	eff2 := Effective(cfg, "src/main.md")
	if !eff2["line-length"].Enabled {
		t.Error("line-length should remain enabled for src/main.md")
	}
}
