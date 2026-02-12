package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/tidymark/internal/rule"
	"gopkg.in/yaml.v3"

	// Import all rule packages so their init() functions register rules.
	_ "github.com/jeduden/tidymark/internal/rules/blanklinearoundfencedcode"
	_ "github.com/jeduden/tidymark/internal/rules/blanklinearoundheadings"
	_ "github.com/jeduden/tidymark/internal/rules/blanklinearoundlists"
	_ "github.com/jeduden/tidymark/internal/rules/catalog"
	_ "github.com/jeduden/tidymark/internal/rules/fencedcodelanguage"
	_ "github.com/jeduden/tidymark/internal/rules/fencedcodestyle"
	_ "github.com/jeduden/tidymark/internal/rules/firstlineheading"
	_ "github.com/jeduden/tidymark/internal/rules/headingincrement"
	_ "github.com/jeduden/tidymark/internal/rules/headingstyle"
	_ "github.com/jeduden/tidymark/internal/rules/linelength"
	_ "github.com/jeduden/tidymark/internal/rules/listindent"
	_ "github.com/jeduden/tidymark/internal/rules/nobareurls"
	_ "github.com/jeduden/tidymark/internal/rules/noduplicateheadings"
	_ "github.com/jeduden/tidymark/internal/rules/noemphasisasheading"
	_ "github.com/jeduden/tidymark/internal/rules/nohardtabs"
	_ "github.com/jeduden/tidymark/internal/rules/nomultipleblanks"
	_ "github.com/jeduden/tidymark/internal/rules/notrailingpunctuation"
	_ "github.com/jeduden/tidymark/internal/rules/notrailingspaces"
	_ "github.com/jeduden/tidymark/internal/rules/singletrailingnewline"
	_ "github.com/jeduden/tidymark/internal/rules/tableformat"
)

// --- YAML parsing tests ---

func TestParseValidYAML(t *testing.T) {
	cfg := loadValidYAMLFixture(t)

	t.Run("rules", func(t *testing.T) {
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
	})

	t.Run("ignore", func(t *testing.T) {
		if len(cfg.Ignore) != 2 {
			t.Fatalf("expected 2 ignore patterns, got %d", len(cfg.Ignore))
		}
		if cfg.Ignore[0] != "vendor/**" {
			t.Errorf("expected vendor/**, got %s", cfg.Ignore[0])
		}
	})

	t.Run("overrides", func(t *testing.T) {
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
	})
}

func loadValidYAMLFixture(t *testing.T) *Config {
	t.Helper()
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
	return cfg
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
		"catalog",
		"required-structure",
		"include",
		"table-format",
	}

	if len(cfg.Rules) != 22 {
		t.Fatalf("expected 22 rules, got %d", len(cfg.Rules))
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

	if len(merged.Rules) != 22 {
		t.Fatalf("expected 22 rules, got %d", len(merged.Rules))
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

	if len(eff) != 22 {
		t.Fatalf("expected 22 rules, got %d", len(eff))
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

// --- MarshalYAML tests ---

func TestMarshalYAML_DisabledRule(t *testing.T) {
	rc := RuleCfg{Enabled: false}
	data, err := yaml.Marshal(rc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if string(data) != "false\n" {
		t.Errorf("expected 'false\\n', got %q", string(data))
	}
}

func TestMarshalYAML_EnabledNoSettings(t *testing.T) {
	rc := RuleCfg{Enabled: true}
	data, err := yaml.Marshal(rc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if string(data) != "true\n" {
		t.Errorf("expected 'true\\n', got %q", string(data))
	}
}

func TestMarshalYAML_EnabledWithSettings(t *testing.T) {
	rc := RuleCfg{Enabled: true, Settings: map[string]any{"max": 80}}
	data, err := yaml.Marshal(rc)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	// Should serialize as the map, not as "true".
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if m["max"] != 80 {
		t.Errorf("expected max=80, got %v", m["max"])
	}
}

func TestMarshalYAML_RoundTrip(t *testing.T) {
	original := &Config{
		Rules: map[string]RuleCfg{
			"line-length":   {Enabled: true, Settings: map[string]any{"max": 120}},
			"heading-style": {Enabled: false},
			"no-hard-tabs":  {Enabled: true},
		},
	}

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed Config
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// line-length should be enabled with max=120.
	rc := parsed.Rules["line-length"]
	if !rc.Enabled {
		t.Error("line-length should be enabled after round-trip")
	}
	if rc.Settings["max"] != 120 {
		t.Errorf("expected max=120, got %v", rc.Settings["max"])
	}

	// heading-style should be disabled.
	if parsed.Rules["heading-style"].Enabled {
		t.Error("heading-style should be disabled after round-trip")
	}

	// no-hard-tabs should be enabled with no settings.
	rc2 := parsed.Rules["no-hard-tabs"]
	if !rc2.Enabled {
		t.Error("no-hard-tabs should be enabled after round-trip")
	}
	if rc2.Settings != nil {
		t.Errorf("no-hard-tabs should have nil settings, got %v", rc2.Settings)
	}
}

// --- DumpDefaults tests ---

func TestDumpDefaults_AllRulesPresent(t *testing.T) {
	cfg := DumpDefaults()

	all := rule.All()
	if len(cfg.Rules) != len(all) {
		t.Fatalf("expected %d rules, got %d", len(all), len(cfg.Rules))
	}

	for _, r := range all {
		rc, ok := cfg.Rules[r.Name()]
		if !ok {
			t.Errorf("rule %q not found in DumpDefaults", r.Name())
			continue
		}
		if !rc.Enabled {
			t.Errorf("rule %q should be enabled", r.Name())
		}
	}
}

func TestDumpDefaults_ConfigurableRulesHaveSettings(t *testing.T) {
	cfg := DumpDefaults()

	// These rules should have settings.
	configurableRules := []string{
		"line-length",
		"heading-style",
		"first-line-heading",
		"no-multiple-blanks",
		"fenced-code-style",
		"list-indent",
	}

	for _, name := range configurableRules {
		rc, ok := cfg.Rules[name]
		if !ok {
			t.Errorf("rule %q not found", name)
			continue
		}
		if rc.Settings == nil {
			t.Errorf("rule %q should have non-nil settings", name)
		}
	}
}

func TestDumpDefaults_NonConfigurableRulesHaveNoSettings(t *testing.T) {
	cfg := DumpDefaults()

	// These rules should NOT have settings.
	nonConfigurableRules := []string{
		"heading-increment",
		"no-duplicate-headings",
		"no-trailing-spaces",
		"no-hard-tabs",
		"single-trailing-newline",
		"fenced-code-language",
		"no-bare-urls",
		"blank-line-around-headings",
		"blank-line-around-lists",
		"blank-line-around-fenced-code",
		"no-trailing-punctuation-in-heading",
		"no-emphasis-as-heading",
		"catalog",
	}

	for _, name := range nonConfigurableRules {
		rc, ok := cfg.Rules[name]
		if !ok {
			t.Errorf("rule %q not found", name)
			continue
		}
		if rc.Settings != nil {
			t.Errorf("rule %q should have nil settings, got %v", name, rc.Settings)
		}
	}
}

func TestDumpDefaults_LineLengthSettings(t *testing.T) {
	cfg := DumpDefaults()
	rc := cfg.Rules["line-length"]
	if rc.Settings["max"] != 80 {
		t.Errorf("expected line-length max=80, got %v", rc.Settings["max"])
	}
	exclude, ok := rc.Settings["exclude"].([]string)
	if !ok {
		t.Fatalf("expected exclude to be []string, got %T", rc.Settings["exclude"])
	}
	if len(exclude) != 3 {
		t.Errorf("expected 3 exclude items, got %d", len(exclude))
	}
}

func TestDumpDefaults_MarshalRoundTrip(t *testing.T) {
	cfg := DumpDefaults()

	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed Config
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Check that line-length round-trips with settings.
	rc := parsed.Rules["line-length"]
	if !rc.Enabled {
		t.Error("line-length should be enabled after round-trip")
	}
	if rc.Settings["max"] != 80 {
		t.Errorf("expected max=80 after round-trip, got %v", rc.Settings["max"])
	}
}

// --- Categories tests ---

func TestLoadCategoriesFromYAML(t *testing.T) {
	yml := `
categories:
  heading: false
  whitespace: true
  code: false
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

	if len(cfg.Categories) != 3 {
		t.Fatalf("expected 3 categories, got %d", len(cfg.Categories))
	}
	if cfg.Categories["heading"] != false {
		t.Error("heading should be false")
	}
	if cfg.Categories["whitespace"] != true {
		t.Error("whitespace should be true")
	}
	if cfg.Categories["code"] != false {
		t.Error("code should be false")
	}
}

func TestCategoriesOmittedDefaultToTrue(t *testing.T) {
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

	if cfg.Categories != nil {
		t.Errorf("expected nil categories when omitted, got %v", cfg.Categories)
	}

	// EffectiveCategories should default all to true.
	cats := EffectiveCategories(cfg, "README.md")
	for _, name := range ValidCategories {
		if !cats[name] {
			t.Errorf("category %q should default to true", name)
		}
	}
}

func TestMergeCategories(t *testing.T) {
	defaults := Defaults()
	loaded := &Config{
		Categories: map[string]bool{
			"heading": false,
		},
	}

	merged := Merge(defaults, loaded)
	if merged.Categories == nil {
		t.Fatal("expected categories to be non-nil after merge")
	}
	if merged.Categories["heading"] != false {
		t.Error("heading should be false after merge")
	}
}

func TestMergeCategoriesNilLoaded(t *testing.T) {
	defaults := Defaults()
	merged := Merge(defaults, nil)

	// Defaults have nil categories.
	if merged.Categories != nil {
		t.Errorf("expected nil categories when merging with nil loaded, got %v", merged.Categories)
	}
}

func TestMergeCategoriesBothSet(t *testing.T) {
	defaults := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true},
		},
		Categories: map[string]bool{
			"heading": true,
			"code":    true,
		},
	}
	loaded := &Config{
		Categories: map[string]bool{
			"heading": false,
			"list":    false,
		},
	}

	merged := Merge(defaults, loaded)
	if merged.Categories["heading"] != false {
		t.Error("heading should be false (overridden by loaded)")
	}
	if merged.Categories["code"] != true {
		t.Error("code should remain true from defaults")
	}
	if merged.Categories["list"] != false {
		t.Error("list should be false from loaded")
	}
}

func TestMergeTracksExplicitRules(t *testing.T) {
	defaults := Defaults()
	loaded := &Config{
		Rules: map[string]RuleCfg{
			"line-length":   {Enabled: true},
			"heading-style": {Enabled: false},
		},
	}

	merged := Merge(defaults, loaded)
	if !merged.ExplicitRules["line-length"] {
		t.Error("line-length should be explicit")
	}
	if !merged.ExplicitRules["heading-style"] {
		t.Error("heading-style should be explicit")
	}
	if merged.ExplicitRules["no-hard-tabs"] {
		t.Error("no-hard-tabs should not be explicit (not in loaded config)")
	}
}

func TestEffectiveCategoriesTopLevel(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true},
		},
		Categories: map[string]bool{
			"heading": false,
		},
	}

	cats := EffectiveCategories(cfg, "README.md")
	if cats["heading"] != false {
		t.Error("heading should be false")
	}
	// Other categories should default to true.
	if cats["whitespace"] != true {
		t.Error("whitespace should default to true")
	}
	if cats["code"] != true {
		t.Error("code should default to true")
	}
}

func TestEffectiveCategoriesOverride(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true},
		},
		Categories: map[string]bool{
			"heading": true,
		},
		Overrides: []Override{
			{
				Files: []string{"CHANGELOG.md"},
				Categories: map[string]bool{
					"heading": false,
				},
			},
		},
	}

	// CHANGELOG.md should have heading disabled via override.
	cats := EffectiveCategories(cfg, "CHANGELOG.md")
	if cats["heading"] != false {
		t.Error("heading should be false for CHANGELOG.md")
	}

	// README.md should keep heading enabled.
	cats2 := EffectiveCategories(cfg, "README.md")
	if cats2["heading"] != true {
		t.Error("heading should be true for README.md")
	}
}

func TestEffectiveExplicitRulesFromOverrides(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true},
		},
		ExplicitRules: map[string]bool{
			"line-length": true,
		},
		Overrides: []Override{
			{
				Files: []string{"docs/**"},
				Rules: map[string]RuleCfg{
					"heading-style": {Enabled: true},
				},
			},
		},
	}

	explicit := EffectiveExplicitRules(cfg, "docs/guide.md")
	if !explicit["line-length"] {
		t.Error("line-length should be explicit (from top-level)")
	}
	if !explicit["heading-style"] {
		t.Error("heading-style should be explicit (from matching override)")
	}

	// Non-matching file should not get override rules.
	explicit2 := EffectiveExplicitRules(cfg, "README.md")
	if explicit2["heading-style"] {
		t.Error("heading-style should not be explicit for README.md")
	}
}

func TestApplyCategoriesDisablesRulesInCategory(t *testing.T) {
	rules := map[string]RuleCfg{
		"heading-style":     {Enabled: true},
		"heading-increment": {Enabled: true},
		"line-length":       {Enabled: true},
	}
	categories := map[string]bool{
		"heading": false,
		"line":    true,
	}
	ruleCategory := func(name string) string {
		switch name {
		case "heading-style", "heading-increment":
			return "heading"
		case "line-length":
			return "line"
		}
		return ""
	}
	explicit := map[string]bool{}

	result := ApplyCategories(rules, categories, ruleCategory, explicit)

	if result["heading-style"].Enabled {
		t.Error("heading-style should be disabled (heading category disabled)")
	}
	if result["heading-increment"].Enabled {
		t.Error("heading-increment should be disabled (heading category disabled)")
	}
	if !result["line-length"].Enabled {
		t.Error("line-length should remain enabled (line category enabled)")
	}
}

func TestApplyCategoriesExplicitRuleOverridesCategory(t *testing.T) {
	rules := map[string]RuleCfg{
		"heading-style":     {Enabled: true},
		"heading-increment": {Enabled: true},
		"line-length":       {Enabled: true},
	}
	categories := map[string]bool{
		"heading": false,
	}
	ruleCategory := func(name string) string {
		switch name {
		case "heading-style", "heading-increment":
			return "heading"
		case "line-length":
			return "line"
		}
		return ""
	}
	// heading-style is explicitly set by user.
	explicit := map[string]bool{
		"heading-style": true,
	}

	result := ApplyCategories(rules, categories, ruleCategory, explicit)

	if !result["heading-style"].Enabled {
		t.Error("heading-style should remain enabled (explicit rule overrides category)")
	}
	if result["heading-increment"].Enabled {
		t.Error("heading-increment should be disabled (heading category disabled, not explicit)")
	}
	if !result["line-length"].Enabled {
		t.Error("line-length should remain enabled")
	}
}

func TestApplyCategoriesUnknownCategoryIsNotDisabled(t *testing.T) {
	rules := map[string]RuleCfg{
		"custom-rule": {Enabled: true},
	}
	categories := map[string]bool{
		"heading": false,
	}
	ruleCategory := func(name string) string {
		return "custom"
	}
	explicit := map[string]bool{}

	result := ApplyCategories(rules, categories, ruleCategory, explicit)

	// "custom" category is not in the categories map, so no disable.
	if !result["custom-rule"].Enabled {
		t.Error("custom-rule should remain enabled (its category is not in the map)")
	}
}

func TestLoadCategoriesInOverrides(t *testing.T) {
	yml := `
rules:
  line-length: true
overrides:
  - files:
      - "docs/**"
    categories:
      heading: false
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

	if len(cfg.Overrides) != 1 {
		t.Fatalf("expected 1 override, got %d", len(cfg.Overrides))
	}
	ov := cfg.Overrides[0]
	if ov.Categories == nil {
		t.Fatal("expected override categories to be non-nil")
	}
	if ov.Categories["heading"] != false {
		t.Error("heading should be false in override")
	}
}

func TestDumpDefaults_IncludesCategories(t *testing.T) {
	cfg := DumpDefaults()

	if cfg.Categories == nil {
		t.Fatal("expected Categories to be non-nil in DumpDefaults")
	}
	for _, name := range ValidCategories {
		enabled, ok := cfg.Categories[name]
		if !ok {
			t.Errorf("category %q not found in DumpDefaults", name)
			continue
		}
		if !enabled {
			t.Errorf("category %q should be enabled by default", name)
		}
	}
}

func TestCategoriesMarshalRoundTrip(t *testing.T) {
	original := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true},
		},
		Categories: map[string]bool{
			"heading":    false,
			"whitespace": true,
		},
	}

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed Config
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if parsed.Categories["heading"] != false {
		t.Error("heading should be false after round-trip")
	}
	if parsed.Categories["whitespace"] != true {
		t.Error("whitespace should be true after round-trip")
	}
}
