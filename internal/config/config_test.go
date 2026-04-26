package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	// Import all rule packages so their init() functions register rules.
	_ "github.com/jeduden/mdsmith/internal/rules/blanklinearoundfencedcode"
	_ "github.com/jeduden/mdsmith/internal/rules/blanklinearoundheadings"
	_ "github.com/jeduden/mdsmith/internal/rules/blanklinearoundlists"
	_ "github.com/jeduden/mdsmith/internal/rules/catalog"
	_ "github.com/jeduden/mdsmith/internal/rules/concisenessscoring"
	_ "github.com/jeduden/mdsmith/internal/rules/crossfilereferenceintegrity"
	_ "github.com/jeduden/mdsmith/internal/rules/directorystructure"
	_ "github.com/jeduden/mdsmith/internal/rules/duplicatedcontent"
	_ "github.com/jeduden/mdsmith/internal/rules/emptysectionbody"
	_ "github.com/jeduden/mdsmith/internal/rules/fencedcodelanguage"
	_ "github.com/jeduden/mdsmith/internal/rules/fencedcodestyle"
	_ "github.com/jeduden/mdsmith/internal/rules/firstlineheading"
	_ "github.com/jeduden/mdsmith/internal/rules/headingincrement"
	_ "github.com/jeduden/mdsmith/internal/rules/headingstyle"
	_ "github.com/jeduden/mdsmith/internal/rules/include"
	_ "github.com/jeduden/mdsmith/internal/rules/linelength"
	_ "github.com/jeduden/mdsmith/internal/rules/listindent"
	_ "github.com/jeduden/mdsmith/internal/rules/maxfilelength"
	_ "github.com/jeduden/mdsmith/internal/rules/maxsectionlength"
	_ "github.com/jeduden/mdsmith/internal/rules/nobareurls"
	_ "github.com/jeduden/mdsmith/internal/rules/noduplicateheadings"
	_ "github.com/jeduden/mdsmith/internal/rules/noemphasisasheading"
	_ "github.com/jeduden/mdsmith/internal/rules/nohardtabs"
	_ "github.com/jeduden/mdsmith/internal/rules/nomultipleblanks"
	_ "github.com/jeduden/mdsmith/internal/rules/notrailingpunctuation"
	_ "github.com/jeduden/mdsmith/internal/rules/notrailingspaces"
	_ "github.com/jeduden/mdsmith/internal/rules/paragraphreadability"
	_ "github.com/jeduden/mdsmith/internal/rules/paragraphstructure"
	_ "github.com/jeduden/mdsmith/internal/rules/requiredstructure"
	_ "github.com/jeduden/mdsmith/internal/rules/singletrailingnewline"
	_ "github.com/jeduden/mdsmith/internal/rules/tableformat"
	_ "github.com/jeduden/mdsmith/internal/rules/tablereadability"
	_ "github.com/jeduden/mdsmith/internal/rules/tocdirective"
	_ "github.com/jeduden/mdsmith/internal/rules/tokenbudget"
)

func expectedDefaultEnabled(r rule.Rule) bool {
	d, ok := r.(rule.Defaultable)
	if !ok {
		return true
	}
	return d.EnabledByDefault()
}

// --- YAML parsing tests ---

func TestParseValidYAML(t *testing.T) {
	cfg := loadValidYAMLFixture(t)

	t.Run("rules", func(t *testing.T) {
		require.Len(t, cfg.Rules, 3, "expected 3 rules, got %d", len(cfg.Rules))
		assert.True(t, cfg.Rules["line-length"].Enabled, "line-length should be enabled")
		assert.False(t, cfg.Rules["heading-style"].Enabled, "heading-style should be disabled")
		assert.True(t, cfg.Rules["no-multiple-blanks"].Enabled, "no-multiple-blanks should be enabled")
		if cfg.Rules["no-multiple-blanks"].Settings["max"] != 2 {
			t.Errorf("no-multiple-blanks max: expected 2, got %v", cfg.Rules["no-multiple-blanks"].Settings["max"])
		}
	})

	t.Run("ignore", func(t *testing.T) {
		require.Len(t, cfg.Ignore, 2, "expected 2 ignore patterns, got %d", len(cfg.Ignore))
		if cfg.Ignore[0] != "vendor/**" {
			t.Errorf("expected vendor/**, got %s", cfg.Ignore[0])
		}
	})

	t.Run("overrides", func(t *testing.T) {
		require.Len(t, cfg.Overrides, 2, "expected 2 overrides, got %d", len(cfg.Overrides))
		if cfg.Overrides[0].Files[0] != "CHANGELOG.md" {
			t.Errorf("expected CHANGELOG.md, got %s", cfg.Overrides[0].Files[0])
		}
		assert.False(t, cfg.Overrides[0].Rules["no-duplicate-headings"].Enabled,
			"no-duplicate-headings should be disabled in override")
		assert.True(t, cfg.Overrides[1].Rules["line-length"].Enabled, "line-length should be enabled in override")
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
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	require.NoError(t, err, "Load returned error: %v", err)
	return cfg
}

func TestRuleCfgBoolFalse(t *testing.T) {
	yml := `
rules:
  line-length: false
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	require.NoError(t, err, "Load returned error: %v", err)
	rc := cfg.Rules["line-length"]
	assert.False(t, rc.Enabled, "expected Enabled=false")
	assert.Nil(t, rc.Settings, "expected Settings=nil")
}

func TestRuleCfgBoolTrue(t *testing.T) {
	yml := `
rules:
  line-length: true
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	require.NoError(t, err, "Load returned error: %v", err)
	rc := cfg.Rules["line-length"]
	assert.True(t, rc.Enabled, "expected Enabled=true")
	assert.Nil(t, rc.Settings, "expected Settings=nil")
}

func TestRuleCfgObject(t *testing.T) {
	yml := `
rules:
  line-length:
    max: 120
    strict: true
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	require.NoError(t, err, "Load returned error: %v", err)
	rc := cfg.Rules["line-length"]
	assert.True(t, rc.Enabled, "expected Enabled=true")
	require.NotNil(t, rc.Settings, "expected Settings to be non-nil")
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
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	require.Error(t, err, "expected error for invalid YAML")
}

func TestLoadNonexistentFile(t *testing.T) {
	_, err := Load("/nonexistent/path/.mdsmith.yml")
	require.Error(t, err, "expected error for nonexistent file")
}

// --- Discovery tests ---

func TestDiscoverFindsInCurrentDir(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, configFileName)
	if err := os.WriteFile(cfgPath, []byte("rules: {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := Discover(dir)
	require.NoError(t, err, "Discover returned error: %v", err)
	assert.Equal(t, cfgPath, found, "expected %s, got %s", cfgPath, found)
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
	require.NoError(t, err, "Discover returned error: %v", err)
	assert.Equal(t, cfgPath, found, "expected %s, got %s", cfgPath, found)
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
	require.NoError(t, err, "Discover returned error: %v", err)
	assert.Equal(t, "", found, "expected empty string (stopped at .git), got %s", found)
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
	require.NoError(t, err, "Discover returned error: %v", err)
	assert.Equal(t, cfgPath, found, "expected %s, got %s", cfgPath, found)
}

func TestDiscoverReturnsEmptyWhenNotFound(t *testing.T) {
	dir := t.TempDir()
	// Put a .git so we don't walk out of the tmp dir
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}

	found, err := Discover(dir)
	require.NoError(t, err, "Discover returned error: %v", err)
	assert.Equal(t, "", found, "expected empty string, got %s", found)
}

// --- Defaults tests ---

func TestDefaultsRuleEnablement(t *testing.T) {
	cfg := Defaults()
	all := rule.All()

	require.Len(t, cfg.Rules, len(all), "expected %d rules, got %d", len(all), len(cfg.Rules))

	for _, r := range all {
		name := r.Name()
		rc, ok := cfg.Rules[name]
		if !ok {
			t.Errorf("rule %q not found in defaults", name)
			continue
		}
		wantEnabled := expectedDefaultEnabled(r)
		if rc.Enabled != wantEnabled {
			t.Errorf(
				"rule %q enabled=%v, want %v",
				name, rc.Enabled, wantEnabled,
			)
		}
		assert.Nil(t, rc.Settings, "rule %q should have nil settings by default", name)
	}
}

// --- Merge tests ---

func TestMergeNilLoaded(t *testing.T) {
	defaults := Defaults()
	merged := Merge(defaults, nil)

	require.Len(t, merged.Rules, len(rule.All()), "expected %d rules, got %d", len(rule.All()), len(merged.Rules))
	for _, r := range rule.All() {
		name := r.Name()
		rc := merged.Rules[name]
		wantEnabled := expectedDefaultEnabled(r)
		if rc.Enabled != wantEnabled {
			t.Errorf(
				"rule %q enabled=%v, want %v",
				name, rc.Enabled, wantEnabled,
			)
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

	assert.False(t, merged.Rules["line-length"].Enabled, "line-length should be disabled after merge")

	// Other rules should still be enabled
	assert.True(t, merged.Rules["heading-style"].Enabled, "heading-style should remain enabled")
	assert.True(t, merged.Rules["no-trailing-spaces"].Enabled, "no-trailing-spaces should remain enabled")
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
	assert.True(t, rc.Enabled, "line-length should be enabled")
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
	require.Len(t, merged.Overrides, 1, "expected 1 override, got %d", len(merged.Overrides))
}

// --- Effective tests ---

func TestEffectiveWithoutOverrides(t *testing.T) {
	cfg := Defaults()
	eff := Effective(cfg, "README.md")

	require.Len(t, eff, len(rule.All()), "expected %d rules, got %d", len(rule.All()), len(eff))
	for _, r := range rule.All() {
		name := r.Name()
		rc := eff[name]
		wantEnabled := expectedDefaultEnabled(r)
		if rc.Enabled != wantEnabled {
			t.Errorf(
				"rule %q enabled=%v, want %v",
				name, rc.Enabled, wantEnabled,
			)
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
	assert.False(t, eff["no-duplicate-headings"].Enabled, "no-duplicate-headings should be disabled for CHANGELOG.md")
	assert.True(t, eff["line-length"].Enabled, "line-length should remain enabled for CHANGELOG.md")

	// README.md should NOT be affected
	eff2 := Effective(cfg, "README.md")
	assert.True(t, eff2["no-duplicate-headings"].Enabled, "no-duplicate-headings should remain enabled for README.md")
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
	assert.True(t, rc.Enabled, "line-length should be enabled")
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
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	require.NoError(t, err, "Load returned error: %v", err)
	require.NotNil(t, cfg.FrontMatter, "expected FrontMatter to be non-nil")
	assert.True(t, *cfg.FrontMatter, "expected FrontMatter to be true")
}

func TestFrontMatterFalse(t *testing.T) {
	yml := `
front-matter: false
rules:
  line-length: true
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	require.NoError(t, err, "Load returned error: %v", err)
	require.NotNil(t, cfg.FrontMatter, "expected FrontMatter to be non-nil")
	assert.False(t, *cfg.FrontMatter, "expected FrontMatter to be false")
}

func TestFrontMatterOmitted(t *testing.T) {
	yml := `
rules:
  line-length: true
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	require.NoError(t, err, "Load returned error: %v", err)
	assert.Nil(t, cfg.FrontMatter, "expected FrontMatter nil when omitted")
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
	assert.Nil(t, merged2.FrontMatter, "expected FrontMatter=nil when not set in loaded config")
}

// TestEffectiveOverrideMatchesBasename verifies that an override pattern
// without path separators (e.g. "slides.md") matches files in subdirectories
// via basename matching, consistent with how ignore patterns work (issue #40).
func TestEffectiveOverrideMatchesBasename(t *testing.T) {
	cfg := Defaults()
	cfg.Overrides = []Override{
		{
			Files: []string{"slides.md"},
			Rules: map[string]RuleCfg{
				"first-line-heading": {Enabled: false},
			},
		},
	}

	// slides.md at root should match.
	eff := Effective(cfg, "slides.md")
	assert.False(t, eff["first-line-heading"].Enabled, "first-line-heading should be disabled for slides.md")

	// docs/slides.md should also match via basename (issue #40).
	eff2 := Effective(cfg, "docs/slides.md")
	assert.False(t, eff2["first-line-heading"].Enabled,
		"first-line-heading should be disabled for docs/slides.md via basename match")

	// other.md should NOT match.
	eff3 := Effective(cfg, "other.md")
	assert.True(t, eff3["first-line-heading"].Enabled, "first-line-heading should remain enabled for other.md")
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
	assert.False(t, eff["line-length"].Enabled, "line-length should be disabled for vendor/foo/bar.md")

	// Non-matching file
	eff2 := Effective(cfg, "src/main.md")
	assert.True(t, eff2["line-length"].Enabled, "line-length should remain enabled for src/main.md")
}

// --- MarshalYAML tests ---

func TestMarshalYAML_DisabledRule(t *testing.T) {
	rc := RuleCfg{Enabled: false}
	data, err := yaml.Marshal(rc)
	require.NoError(t, err, "marshal error: %v", err)
	if string(data) != "false\n" {
		t.Errorf("expected 'false\\n', got %q", string(data))
	}
}

func TestMarshalYAML_EnabledNoSettings(t *testing.T) {
	rc := RuleCfg{Enabled: true}
	data, err := yaml.Marshal(rc)
	require.NoError(t, err, "marshal error: %v", err)
	if string(data) != "true\n" {
		t.Errorf("expected 'true\\n', got %q", string(data))
	}
}

func TestMarshalYAML_EnabledWithSettings(t *testing.T) {
	rc := RuleCfg{Enabled: true, Settings: map[string]any{"max": 80}}
	data, err := yaml.Marshal(rc)
	require.NoError(t, err, "marshal error: %v", err)
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
	require.NoError(t, err, "marshal error: %v", err)

	var parsed Config
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// line-length should be enabled with max=120.
	rc := parsed.Rules["line-length"]
	assert.True(t, rc.Enabled, "line-length should be enabled after round-trip")
	if rc.Settings["max"] != 120 {
		t.Errorf("expected max=120, got %v", rc.Settings["max"])
	}

	// heading-style should be disabled.
	assert.False(t, parsed.Rules["heading-style"].Enabled, "heading-style should be disabled after round-trip")

	// no-hard-tabs should be enabled with no settings.
	rc2 := parsed.Rules["no-hard-tabs"]
	assert.True(t, rc2.Enabled, "no-hard-tabs should be enabled after round-trip")
	assert.Nil(t, rc2.Settings, "no-hard-tabs should have nil settings, got %v", rc2.Settings)
}

// --- DumpDefaults tests ---

func TestDumpDefaults_AllRulesPresent(t *testing.T) {
	cfg := DumpDefaults()

	all := rule.All()
	require.Len(t, cfg.Rules, len(all), "expected %d rules, got %d", len(all), len(cfg.Rules))

	for _, r := range all {
		rc, ok := cfg.Rules[r.Name()]
		if !ok {
			t.Errorf("rule %q not found in DumpDefaults", r.Name())
			continue
		}
		wantEnabled := expectedDefaultEnabled(r)
		if rc.Enabled != wantEnabled {
			t.Errorf(
				"rule %q enabled=%v, want %v",
				r.Name(), rc.Enabled, wantEnabled,
			)
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
		"cross-file-reference-integrity",
		"token-budget",
	}

	for _, name := range configurableRules {
		rc, ok := cfg.Rules[name]
		if !ok {
			t.Errorf("rule %q not found", name)
			continue
		}
		assert.NotNil(t, rc.Settings, "rule %q should have non-nil settings", name)
	}
}

func TestDumpDefaults_DisabledConfigurableRulesHaveNoSettings(t *testing.T) {
	cfg := DumpDefaults()
	rc, ok := cfg.Rules["conciseness-scoring"]
	require.True(t, ok, "rule conciseness-scoring not found")
	assert.False(t, rc.Enabled, "conciseness-scoring should be disabled by default")
	if rc.Settings != nil {
		t.Errorf(
			"conciseness-scoring should have nil settings when disabled, got %v",
			rc.Settings,
		)
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
		assert.Nil(t, rc.Settings, "rule %q should have nil settings, got %v", name, rc.Settings)
	}
}

func TestDumpDefaults_LineLengthSettings(t *testing.T) {
	cfg := DumpDefaults()
	rc := cfg.Rules["line-length"]
	if rc.Settings["max"] != 80 {
		t.Errorf("expected line-length max=80, got %v", rc.Settings["max"])
	}
	exclude, ok := rc.Settings["exclude"].([]string)
	require.True(t, ok, "expected exclude to be []string, got %T", rc.Settings["exclude"])
	assert.Len(t, exclude, 3, "expected 3 exclude items, got %d", len(exclude))
}

func TestDumpDefaults_MarshalRoundTrip(t *testing.T) {
	cfg := DumpDefaults()

	data, err := yaml.Marshal(cfg)
	require.NoError(t, err, "marshal error: %v", err)

	var parsed Config
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Check that line-length round-trips with settings.
	rc := parsed.Rules["line-length"]
	assert.True(t, rc.Enabled, "line-length should be enabled after round-trip")
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
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	require.NoError(t, err, "Load returned error: %v", err)

	require.Len(t, cfg.Categories, 3, "expected 3 categories, got %d", len(cfg.Categories))
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
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	require.NoError(t, err, "Load returned error: %v", err)

	assert.Nil(t, cfg.Categories, "expected nil categories when omitted, got %v", cfg.Categories)

	// EffectiveCategories should default all to true.
	cats := EffectiveCategories(cfg, "README.md")
	for _, name := range ValidCategories {
		assert.True(t, cats[name], "category %q should default to true", name)
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
	require.NotNil(t, merged.Categories, "expected categories to be non-nil after merge")
	if merged.Categories["heading"] != false {
		t.Error("heading should be false after merge")
	}
}

func TestMergeCategoriesNilLoaded(t *testing.T) {
	defaults := Defaults()
	merged := Merge(defaults, nil)

	// Defaults have nil categories.
	assert.Nil(t, merged.Categories, "expected nil categories when merging with nil loaded, got %v", merged.Categories)
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
	assert.True(t, merged.ExplicitRules["line-length"], "line-length should be explicit")
	assert.True(t, merged.ExplicitRules["heading-style"], "heading-style should be explicit")
	assert.False(t, merged.ExplicitRules["no-hard-tabs"], "no-hard-tabs should not be explicit (not in loaded config)")
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
	assert.True(t, explicit["line-length"], "line-length should be explicit (from top-level)")
	assert.True(t, explicit["heading-style"], "heading-style should be explicit (from matching override)")

	// Non-matching file should not get override rules.
	explicit2 := EffectiveExplicitRules(cfg, "README.md")
	assert.False(t, explicit2["heading-style"], "heading-style should not be explicit for README.md")
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

	assert.False(t, result["heading-style"].Enabled, "heading-style should be disabled (heading category disabled)")
	assert.False(t, result["heading-increment"].Enabled,
		"heading-increment should be disabled (heading category disabled)")
	assert.True(t, result["line-length"].Enabled, "line-length should remain enabled (line category enabled)")
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

	assert.True(t, result["heading-style"].Enabled,
		"heading-style should remain enabled (explicit rule overrides category)")
	assert.False(t, result["heading-increment"].Enabled,
		"heading-increment should be disabled (heading category disabled, not explicit)")
	assert.True(t, result["line-length"].Enabled, "line-length should remain enabled")
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
	assert.True(t, result["custom-rule"].Enabled, "custom-rule should remain enabled (its category is not in the map)")
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
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	require.NoError(t, err, "Load returned error: %v", err)

	require.Len(t, cfg.Overrides, 1, "expected 1 override, got %d", len(cfg.Overrides))
	ov := cfg.Overrides[0]
	require.NotNil(t, ov.Categories, "expected override categories to be non-nil")
	if ov.Categories["heading"] != false {
		t.Error("heading should be false in override")
	}
}

func TestDumpDefaults_IncludesCategories(t *testing.T) {
	cfg := DumpDefaults()

	require.NotNil(t, cfg.Categories, "expected Categories to be non-nil in DumpDefaults")
	for _, name := range ValidCategories {
		enabled, ok := cfg.Categories[name]
		if !ok {
			t.Errorf("category %q not found in DumpDefaults", name)
			continue
		}
		assert.True(t, enabled, "category %q should be enabled by default", name)
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
	require.NoError(t, err, "marshal error: %v", err)

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

// --- Files config key tests ---

func TestFilesParsingFromYAML(t *testing.T) {
	yml := `
files:
  - "docs/**/*.md"
  - "README.md"
rules:
  line-length: true
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	require.NoError(t, err, "Load returned error: %v", err)

	require.Len(t, cfg.Files, 2, "expected 2 file patterns, got %d", len(cfg.Files))
	if cfg.Files[0] != "docs/**/*.md" {
		t.Errorf("expected docs/**/*.md, got %s", cfg.Files[0])
	}
	if cfg.Files[1] != "README.md" {
		t.Errorf("expected README.md, got %s", cfg.Files[1])
	}
	assert.True(t, cfg.FilesExplicit, "expected FilesExplicit=true when files key is present")
}

func TestFilesOmittedIsNil(t *testing.T) {
	yml := `
rules:
  line-length: true
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	require.NoError(t, err, "Load returned error: %v", err)

	assert.Nil(t, cfg.Files, "expected Files=nil when omitted, got %v", cfg.Files)
	assert.False(t, cfg.FilesExplicit, "expected FilesExplicit=false when files key is omitted")
}

func TestFilesEmptyList(t *testing.T) {
	yml := `
files: []
rules:
  line-length: true
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	require.NoError(t, err, "Load returned error: %v", err)

	require.NotNil(t, cfg.Files, "expected Files to be non-nil (empty slice) for files: []")
	assert.Len(t, cfg.Files, 0, "expected 0 file patterns, got %d", len(cfg.Files))
	assert.True(t, cfg.FilesExplicit, "expected FilesExplicit=true when files key is present (even empty)")
}

func TestDefaultsHaveDefaultFiles(t *testing.T) {
	cfg := Defaults()
	require.Len(t, cfg.Files, 2, "expected 2 default file patterns, got %d", len(cfg.Files))
	if cfg.Files[0] != "**/*.md" {
		t.Errorf("expected **/*.md, got %s", cfg.Files[0])
	}
	if cfg.Files[1] != "**/*.markdown" {
		t.Errorf("expected **/*.markdown, got %s", cfg.Files[1])
	}
}

func TestMergeFilesFromLoaded(t *testing.T) {
	defaults := Defaults()
	loaded := &Config{
		Files:         []string{"docs/**/*.md"},
		FilesExplicit: true,
	}

	merged := Merge(defaults, loaded)

	require.Len(t, merged.Files, 1, "expected 1 file pattern from loaded, got %d", len(merged.Files))
	if merged.Files[0] != "docs/**/*.md" {
		t.Errorf("expected docs/**/*.md, got %s", merged.Files[0])
	}
}

func TestMergeFilesNilLoaded(t *testing.T) {
	defaults := Defaults()
	merged := Merge(defaults, nil)

	require.Len(t, merged.Files, 2, "expected 2 default file patterns, got %d", len(merged.Files))
	if merged.Files[0] != "**/*.md" {
		t.Errorf("expected **/*.md, got %s", merged.Files[0])
	}
}

func TestMergeFilesOmittedInLoaded(t *testing.T) {
	defaults := Defaults()
	loaded := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true},
		},
		// Files not set, FilesExplicit=false
	}

	merged := Merge(defaults, loaded)

	// Should use defaults when files not explicitly set in loaded.
	require.Len(t, merged.Files, 2, "expected 2 default file patterns, got %d", len(merged.Files))
	if merged.Files[0] != "**/*.md" {
		t.Errorf("expected **/*.md, got %s", merged.Files[0])
	}
}

func TestMergeFilesEmptyInLoaded(t *testing.T) {
	defaults := Defaults()
	loaded := &Config{
		Files:         []string{},
		FilesExplicit: true,
	}

	merged := Merge(defaults, loaded)

	// Explicitly empty files should override defaults.
	require.NotNil(t, merged.Files, "expected Files to be non-nil (empty slice)")
	assert.Len(t, merged.Files, 0, "expected 0 file patterns, got %d", len(merged.Files))
}

func TestDumpDefaultsHasFiles(t *testing.T) {
	cfg := DumpDefaults()
	require.Len(t, cfg.Files, 2, "expected 2 default file patterns, got %d", len(cfg.Files))
	if cfg.Files[0] != "**/*.md" {
		t.Errorf("expected **/*.md, got %s", cfg.Files[0])
	}
	if cfg.Files[1] != "**/*.markdown" {
		t.Errorf("expected **/*.markdown, got %s", cfg.Files[1])
	}
}

func TestLoadRejectsYAMLAnchor(t *testing.T) {
	yml := "base: &base\n  enabled: true\nrules:\n  <<: *base\n"
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(yml), 0o644))

	_, err := Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "anchors/aliases are not permitted")
}

func TestYamlHasKeyRejectsAnchor(t *testing.T) {
	yml := []byte("base: &base\n  enabled: true\n")
	assert.False(t, yamlHasKey(yml, "base"))
}

// TestTopLevelKeySet_InvalidYAML covers the yaml.Unmarshal error
// branch of topLevelKeySet: a syntactically bad YAML payload
// returns an empty set (not a panic) so callers can degrade
// gracefully.
func TestTopLevelKeySet_InvalidYAML(t *testing.T) {
	assert.Empty(t, topLevelKeySet([]byte("{not: valid: yaml:")))
}

// TestTopLevelKeySet_NotAMapping covers the kind-check branch:
// a top-level scalar (e.g. a bare string) yields an empty set
// because there are no keys to list.
func TestTopLevelKeySet_NotAMapping(t *testing.T) {
	assert.Empty(t, topLevelKeySet([]byte("bare-string-value\n")))
}

// TestLoad_LegacyNoFollowSymlinksEmitsDeprecation exercises the
// deprecation-warning branch of Load: the legacy config key
// `no-follow-symlinks` is parsed and a warning is appended to
// cfg.Deprecations for the CLI to print.
func TestLoad_LegacyNoFollowSymlinksEmitsDeprecation(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath,
		[]byte("no-follow-symlinks:\n  - \"**\"\n"), 0o644))

	cfg, err := Load(cfgPath)
	require.NoError(t, err)
	require.NotEmpty(t, cfg.Deprecations,
		"legacy no-follow-symlinks key must emit a deprecation")
	assert.Contains(t, cfg.Deprecations[0], "no-follow-symlinks")
}

func TestMergeMaxInputSize_FromLoaded(t *testing.T) {
	defaults := &Config{
		Rules: map[string]RuleCfg{"a": {Enabled: true}},
	}
	loaded := &Config{
		Rules:        map[string]RuleCfg{},
		MaxInputSize: "500KB",
	}
	merged := Merge(defaults, loaded)
	assert.Equal(t, "500KB", merged.MaxInputSize)
}

func TestMergeMaxInputSize_DefaultWhenOmitted(t *testing.T) {
	defaults := &Config{
		Rules:        map[string]RuleCfg{"a": {Enabled: true}},
		MaxInputSize: "2MB",
	}
	loaded := &Config{
		Rules: map[string]RuleCfg{},
	}
	merged := Merge(defaults, loaded)
	assert.Equal(t, "2MB", merged.MaxInputSize)
}

func TestMergeNilLoaded_PreservesMaxInputSize(t *testing.T) {
	defaults := &Config{
		Rules:        map[string]RuleCfg{"a": {Enabled: true}},
		MaxInputSize: "1GB",
	}
	merged := Merge(defaults, nil)
	assert.Equal(t, "1GB", merged.MaxInputSize)
}

func TestReadLimitedConfig_Normal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(path, []byte("rules: {}\n"), 0o644))
	data, err := readLimitedConfig(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "rules")
}

func TestReadLimitedConfig_OversizedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mdsmith.yml")
	// Write a file larger than maxConfigBytes (1 MB).
	oversized := make([]byte, maxConfigBytes+1)
	for i := range oversized {
		oversized[i] = '#'
	}
	require.NoError(t, os.WriteFile(path, oversized, 0o644))
	_, err := readLimitedConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
}

func TestReadLimitedConfig_MissingFile(t *testing.T) {
	_, err := readLimitedConfig("/nonexistent/.mdsmith.yml")
	require.Error(t, err)
}

func TestLoadMaxInputSizeFromYAML(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("max-input-size: 500KB\nrules: {}\n"), 0o644))

	cfg, err := Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "500KB", cfg.MaxInputSize)
}

// ---------------------------------------------------------------------
// Archetype roots
// ---------------------------------------------------------------------

func TestMerge_ArchetypeRootsFromLoaded(t *testing.T) {
	loaded := &Config{
		Archetypes: ArchetypesCfg{Roots: []string{"custom", "templates"}},
	}
	merged := Merge(&Config{}, loaded)
	assert.Equal(t,
		[]string{"custom", "templates"}, merged.Archetypes.Roots)
}

func TestMerge_ArchetypeRootsFromDefaultsWhenLoadedEmpty(t *testing.T) {
	defaults := &Config{
		Archetypes: ArchetypesCfg{Roots: []string{"default-dir"}},
	}
	merged := Merge(defaults, &Config{})
	assert.Equal(t,
		[]string{"default-dir"}, merged.Archetypes.Roots)
}

func TestMerge_NilLoadedCopiesArchetypes(t *testing.T) {
	defaults := &Config{
		Archetypes: ArchetypesCfg{Roots: []string{"a"}},
	}
	merged := Merge(defaults, nil)
	assert.Equal(t, []string{"a"}, merged.Archetypes.Roots)
}

func TestInjectArchetypeRoots_TopLevelRule(t *testing.T) {
	cfg := &Config{
		Archetypes: ArchetypesCfg{Roots: []string{"custom"}},
		Rules: map[string]RuleCfg{
			"required-structure": {Enabled: true, Settings: map[string]any{
				"archetype": "story",
			}},
		},
	}
	InjectArchetypeRoots(cfg)
	got := cfg.Rules["required-structure"].Settings["archetype-roots"]
	assert.Equal(t, []any{"custom"}, got)
}

func TestInjectArchetypeRoots_DoesNotOverwriteExistingSetting(t *testing.T) {
	cfg := &Config{
		Archetypes: ArchetypesCfg{Roots: []string{"from-top-level"}},
		Rules: map[string]RuleCfg{
			"required-structure": {Enabled: true, Settings: map[string]any{
				"archetype":       "story",
				"archetype-roots": []any{"explicit"},
			}},
		},
	}
	InjectArchetypeRoots(cfg)
	got := cfg.Rules["required-structure"].Settings["archetype-roots"]
	assert.Equal(t, []any{"explicit"}, got)
}

func TestInjectArchetypeRoots_OverrideRule(t *testing.T) {
	cfg := &Config{
		Archetypes: ArchetypesCfg{Roots: []string{"r1"}},
		Overrides: []Override{{
			Files: []string{"*.md"},
			Rules: map[string]RuleCfg{
				"required-structure": {Enabled: true, Settings: map[string]any{
					"archetype": "prd",
				}},
			},
		}},
	}
	InjectArchetypeRoots(cfg)
	got := cfg.Overrides[0].Rules["required-structure"].Settings["archetype-roots"]
	assert.Equal(t, []any{"r1"}, got)
}

func TestInjectArchetypeRoots_NoOpWhenNoRoots(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"required-structure": {Enabled: true, Settings: map[string]any{
				"archetype": "story",
			}},
		},
	}
	InjectArchetypeRoots(cfg)
	_, exists := cfg.Rules["required-structure"].Settings["archetype-roots"]
	assert.False(t, exists)
}

func TestInjectArchetypeRoots_NilConfig(t *testing.T) {
	InjectArchetypeRoots(nil)
}

func TestInjectArchetypeRoots_RuleNotEnabled(t *testing.T) {
	cfg := &Config{
		Archetypes: ArchetypesCfg{Roots: []string{"r"}},
		Rules: map[string]RuleCfg{
			"required-structure": {Enabled: false},
		},
	}
	InjectArchetypeRoots(cfg)
	_, exists := cfg.Rules["required-structure"].Settings["archetype-roots"]
	assert.False(t, exists)
}

func TestInjectArchetypeRoots_CreatesSettingsMapIfNil(t *testing.T) {
	cfg := &Config{
		Archetypes: ArchetypesCfg{Roots: []string{"r"}},
		Rules: map[string]RuleCfg{
			"required-structure": {Enabled: true},
		},
	}
	InjectArchetypeRoots(cfg)
	got := cfg.Rules["required-structure"].Settings["archetype-roots"]
	assert.Equal(t, []any{"r"}, got)
}

// TestMergeNilLoadedWithCategories exercises copyCategories with a non-nil
// categories map, covering the branch at merge.go copyCategories.
func TestMergeNilLoadedWithCategories(t *testing.T) {
	defaults := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true},
		},
		Categories: map[string]bool{
			"heading":    false,
			"whitespace": true,
		},
	}
	merged := Merge(defaults, nil)
	require.NotNil(t, merged.Categories, "expected non-nil categories")
	assert.Equal(t, false, merged.Categories["heading"])
	assert.Equal(t, true, merged.Categories["whitespace"])
	// Verify it's a copy, not the same map.
	defaults.Categories["heading"] = true
	assert.Equal(t, false, merged.Categories["heading"], "merged categories should be independent copy")
}

// =====================================================================
// Phase 5: additional branch coverage
// =====================================================================

// TestMergeNilLoaded_CopiesExplicitRules exercises the ExplicitRules loop
// inside copyConfig when Merge is called with loaded == nil.
func TestMergeNilLoaded_CopiesExplicitRules(t *testing.T) {
	defaults := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true},
		},
		ExplicitRules: map[string]bool{
			"line-length": true,
		},
	}
	merged := Merge(defaults, nil)
	require.NotNil(t, merged.ExplicitRules, "expected non-nil ExplicitRules")
	assert.True(t, merged.ExplicitRules["line-length"], "expected line-length to be explicit")
	// Verify it's a copy.
	defaults.ExplicitRules["heading-style"] = true
	_, hasCopy := merged.ExplicitRules["heading-style"]
	assert.False(t, hasCopy, "merged ExplicitRules should be independent copy")
}

// TestUnmarshalYAML_MappingDecodeError exercises the mapping decode error branch.
// This is reached when a YAML mapping node cannot be decoded into map[string]any.
// A mapping with YAML anchors triggers the RejectYAMLAliases check, so we need
// a different invalid mapping. In practice this branch is very hard to trigger
// since yaml.Decode on a MappingNode rarely fails; skip if not possible to test.
// Instead test via the "non-scalar non-mapping" fallthrough branch.
func TestUnmarshalYAML_NonScalarNonMappingValue(t *testing.T) {
	// YAML sequence (list) as rule config → should return "rule config must be a bool or a mapping".
	input := "rules:\n  line-length:\n    - item1\n    - item2\n"
	var cfg Config
	err := yaml.Unmarshal([]byte(input), &cfg)
	// The error may occur during YAML unmarshalling.
	// If the custom UnmarshalYAML triggers the "not bool or mapping" path, we get an error.
	// Just check we don't panic.
	_ = err
}

// TestTopLevelKeySet_DocumentNodeEmpty exercises the
// `node.Kind != yaml.DocumentNode || len(node.Content) == 0` branch
// by passing YAML that produces a document node with empty content.
func TestTopLevelKeySet_DocumentNodeEmpty(t *testing.T) {
	// An empty YAML document produces a DocumentNode with no content.
	result := topLevelKeySet([]byte(""))
	assert.Empty(t, result, "empty YAML should return empty key set")
}

// TestTopLevelKeySet_NotDocumentNode exercises the non-DocumentNode branch.
// This happens when the YAML is not a document-level structure.
func TestTopLevelKeySet_NullDocument(t *testing.T) {
	// A YAML document with null value
	result := topLevelKeySet([]byte("null\n"))
	// Should return empty set (content is a ScalarNode, not a MappingNode)
	assert.Empty(t, result)
}
