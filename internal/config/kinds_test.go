package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- YAML parsing ---

func TestKindsParsesFromYAML(t *testing.T) {
	yml := `
kinds:
  plan:
    rules:
      line-length: false
      paragraph-readability: false
  proto:
    rules:
      paragraph-readability: false
    categories:
      meta: false
kind-assignment:
  - files: ["plan/[0-9]*_*.md"]
    kinds: [plan]
  - files: ["**/proto.md"]
    kinds: [proto]
`
	cfg := loadFromString(t, yml)

	require.NotNil(t, cfg.Kinds)
	require.Contains(t, cfg.Kinds, "plan")
	require.Contains(t, cfg.Kinds, "proto")

	planKind := cfg.Kinds["plan"]
	assert.False(t, planKind.Rules["line-length"].Enabled)
	assert.False(t, planKind.Rules["paragraph-readability"].Enabled)

	protoKind := cfg.Kinds["proto"]
	assert.False(t, protoKind.Rules["paragraph-readability"].Enabled)
	assert.False(t, protoKind.Categories["meta"])

	require.Len(t, cfg.KindAssignment, 2)
	assert.Equal(t, []string{"plan/[0-9]*_*.md"}, cfg.KindAssignment[0].Files)
	assert.Equal(t, []string{"plan"}, cfg.KindAssignment[0].Kinds)
}

// --- ValidateKinds ---

func TestValidateKindsAcceptsDeclaredKinds(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"plan": {Rules: map[string]RuleCfg{"line-length": {Enabled: false}}},
		},
		KindAssignment: []KindAssignmentEntry{
			{Files: []string{"plan/*.md"}, Kinds: []string{"plan"}},
		},
	}
	assert.NoError(t, ValidateKinds(cfg))
}

func TestValidateKindsRejectsUndeclaredKindInAssignment(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{},
		KindAssignment: []KindAssignmentEntry{
			{Files: []string{"plan/*.md"}, Kinds: []string{"unknown-kind"}},
		},
	}
	err := ValidateKinds(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "undeclared kind")
	assert.Contains(t, err.Error(), "unknown-kind")
}

func TestLoadRejectsUndeclaredKindInAssignment(t *testing.T) {
	yml := `
kind-assignment:
  - files: ["plan/*.md"]
    kinds: [no-such-kind]
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(yml), 0o644))

	_, err := Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "undeclared kind")
	assert.Contains(t, err.Error(), "no-such-kind")
}

func TestValidateFrontMatterKindsRejectsUndeclared(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"plan": {},
		},
	}
	err := ValidateFrontMatterKinds(cfg, "docs/foo.md", []string{"plan", "ghost"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "docs/foo.md")
	assert.Contains(t, err.Error(), "ghost")
}

func TestValidateFrontMatterKindsAcceptsDeclared(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"plan":  {},
			"proto": {},
		},
	}
	assert.NoError(t, ValidateFrontMatterKinds(cfg, "docs/foo.md", []string{"plan", "proto"}))
}

// --- resolveEffectiveKinds ---

func TestResolveEffectiveKindsFrontMatterFirst(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"a": {},
			"b": {},
			"c": {},
		},
		KindAssignment: []KindAssignmentEntry{
			{Files: []string{"*.md"}, Kinds: []string{"b", "c"}},
		},
	}
	got := resolveEffectiveKinds(cfg, "file.md", []string{"a"})
	assert.Equal(t, []string{"a", "b", "c"}, got)
}

func TestResolveEffectiveKindsDeduplicates(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"a": {},
			"b": {},
		},
		KindAssignment: []KindAssignmentEntry{
			// "a" already in front matter — should not appear again.
			{Files: []string{"*.md"}, Kinds: []string{"a", "b"}},
		},
	}
	got := resolveEffectiveKinds(cfg, "file.md", []string{"a"})
	assert.Equal(t, []string{"a", "b"}, got)
}

func TestResolveEffectiveKindsNoAssignmentMatch(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{"a": {}},
		KindAssignment: []KindAssignmentEntry{
			{Files: []string{"docs/*.md"}, Kinds: []string{"a"}},
		},
	}
	got := resolveEffectiveKinds(cfg, "other/file.md", nil)
	assert.Empty(t, got)
}

// --- Effective with kinds ---

func TestEffectiveKindOverridesTopLevelRule(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
		Kinds: map[string]KindBody{
			"wide": {Rules: map[string]RuleCfg{
				"line-length": {Enabled: true, Settings: map[string]any{"max": 200}},
			}},
		},
		KindAssignment: []KindAssignmentEntry{
			{Files: []string{"wide/*.md"}, Kinds: []string{"wide"}},
		},
	}
	result := Effective(cfg, "wide/doc.md", nil)
	assert.Equal(t, 200, result["line-length"].Settings["max"])
}

func TestEffectiveGlobOverrideBeatsKind(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
		Kinds: map[string]KindBody{
			"wide": {Rules: map[string]RuleCfg{
				"line-length": {Enabled: true, Settings: map[string]any{"max": 200}},
			}},
		},
		KindAssignment: []KindAssignmentEntry{
			{Files: []string{"wide/*.md"}, Kinds: []string{"wide"}},
		},
		Overrides: []Override{
			{
				Files: []string{"wide/special.md"},
				Rules: map[string]RuleCfg{
					"line-length": {Enabled: true, Settings: map[string]any{"max": 120}},
				},
			},
		},
	}
	result := Effective(cfg, "wide/special.md", nil)
	assert.Equal(t, 120, result["line-length"].Settings["max"])
}

func TestEffectiveTwoKindsMergeInListOrder(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length":           {Enabled: true},
			"paragraph-readability": {Enabled: true},
		},
		Kinds: map[string]KindBody{
			"a": {Rules: map[string]RuleCfg{
				"line-length": {Enabled: false},
			}},
			"b": {Rules: map[string]RuleCfg{
				"line-length": {Enabled: true, Settings: map[string]any{"max": 200}},
			}},
		},
	}
	// Front matter: kinds: [a, b] — b comes later and wins on line-length.
	result := Effective(cfg, "doc.md", []string{"a", "b"})
	assert.True(t, result["line-length"].Enabled)
	assert.Equal(t, 200, result["line-length"].Settings["max"])
}

func TestEffectiveConflictLaterKindWins(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
		Kinds: map[string]KindBody{
			"a": {Rules: map[string]RuleCfg{
				"line-length": {Enabled: true, Settings: map[string]any{"max": 100}},
			}},
			"b": {Rules: map[string]RuleCfg{
				"line-length": {Enabled: true, Settings: map[string]any{"max": 150}},
			}},
		},
	}
	// kinds: [a, b] — b's config replaces a's entirely.
	result := Effective(cfg, "doc.md", []string{"a", "b"})
	assert.Equal(t, 150, result["line-length"].Settings["max"])
}

func TestEffectiveCategoriesWithKinds(t *testing.T) {
	cfg := &Config{
		Categories: map[string]bool{"meta": true},
		Kinds: map[string]KindBody{
			"fragment": {Categories: map[string]bool{"meta": false}},
		},
		KindAssignment: []KindAssignmentEntry{
			{Files: []string{"_partials/*.md"}, Kinds: []string{"fragment"}},
		},
	}
	result := EffectiveCategories(cfg, "_partials/foo.md", nil)
	assert.False(t, result["meta"])
}

// --- No hardcoded kind names in rule code (grep test) ---

func TestNoHardcodedKindNamesInConfig(t *testing.T) {
	// Scan non-test Go source files in the config and engine packages and
	// assert that none contain `kindName == "`, which would indicate a
	// hardcoded kind-name branch in code that uses the "kindName" loop
	// variable. Rules and engine code must treat all kind names uniformly.
	dirs := []string{
		".",
		"../../internal/engine",
	}
	pattern := regexp.MustCompile(`kindName\s*==\s*"`)
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			require.NoError(t, err)
			assert.False(t, pattern.Match(data),
				"file %s/%s contains a hardcoded kind-name branch", dir, e.Name())
		}
	}
}

// --- Merge preserves kinds ---

func TestMergePreservesKinds(t *testing.T) {
	defaults := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true},
		},
	}
	loaded := &Config{
		Kinds: map[string]KindBody{
			"plan": {Rules: map[string]RuleCfg{"line-length": {Enabled: false}}},
		},
		KindAssignment: []KindAssignmentEntry{
			{Files: []string{"plan/*.md"}, Kinds: []string{"plan"}},
		},
	}
	merged := Merge(defaults, loaded)
	require.Contains(t, merged.Kinds, "plan")
	require.Len(t, merged.KindAssignment, 1)
}

// --- EffectiveExplicitRules with kinds ---

func TestEffectiveExplicitRulesIncludesKindRules(t *testing.T) {
	cfg := &Config{
		ExplicitRules: map[string]bool{"no-hard-tabs": true},
		Kinds: map[string]KindBody{
			"wide": {Rules: map[string]RuleCfg{
				"line-length": {Enabled: true, Settings: map[string]any{"max": 200}},
			}},
		},
		KindAssignment: []KindAssignmentEntry{
			{Files: []string{"wide/*.md"}, Kinds: []string{"wide"}},
		},
	}
	result := EffectiveExplicitRules(cfg, "wide/doc.md", nil)
	assert.True(t, result["no-hard-tabs"], "top-level explicit rule should be present")
	assert.True(t, result["line-length"], "kind rule should be marked explicit")
}

func TestEffectiveExplicitRulesFrontMatterKinds(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"plan": {Rules: map[string]RuleCfg{
				"paragraph-readability": {Enabled: false},
			}},
		},
	}
	result := EffectiveExplicitRules(cfg, "doc.md", []string{"plan"})
	assert.True(t, result["paragraph-readability"])
}

// --- InjectArchetypeRoots with kinds ---

func TestInjectArchetypeRootsInjectsIntoKinds(t *testing.T) {
	cfg := &Config{
		Archetypes: ArchetypesCfg{Roots: []string{"archetypes"}},
		Kinds: map[string]KindBody{
			"plan": {Rules: map[string]RuleCfg{
				"required-structure": {Enabled: true},
			}},
		},
	}
	InjectArchetypeRoots(cfg)
	roots := cfg.Kinds["plan"].Rules["required-structure"].Settings["archetype-roots"]
	require.NotNil(t, roots)
	arr, ok := roots.([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"archetypes"}, arr)
}

func TestInjectArchetypeRootsSkipsKindWithExistingRoots(t *testing.T) {
	existing := []any{"custom-root"}
	cfg := &Config{
		Archetypes: ArchetypesCfg{Roots: []string{"archetypes"}},
		Kinds: map[string]KindBody{
			"plan": {Rules: map[string]RuleCfg{
				"required-structure": {
					Enabled:  true,
					Settings: map[string]any{"archetype-roots": existing},
				},
			}},
		},
	}
	InjectArchetypeRoots(cfg)
	roots := cfg.Kinds["plan"].Rules["required-structure"].Settings["archetype-roots"]
	assert.Equal(t, existing, roots, "existing roots should not be overwritten")
}

// --- Defensive: kind present in effective list but missing from cfg.Kinds ---
// These paths are unreachable in validated configs but the code handles them.

func TestEffectiveIgnoresMissingKindBody(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
		Kinds:          map[string]KindBody{},
		KindAssignment: []KindAssignmentEntry{
			// Directly exercise the resolveEffectiveKinds path with a name that
			// exists in assignment but not in Kinds (bypassing ValidateKinds).
		},
	}
	// Inject a stale kind name via front-matter (bypasses LoadKinds validation).
	result := Effective(cfg, "doc.md", []string{"nonexistent"})
	assert.Equal(t, 80, result["line-length"].Settings["max"], "missing kind body is silently skipped")
}

func TestEffectiveExplicitRulesIgnoresMissingKindBody(t *testing.T) {
	cfg := &Config{
		ExplicitRules: map[string]bool{"line-length": true},
		Kinds:         map[string]KindBody{},
	}
	result := EffectiveExplicitRules(cfg, "doc.md", []string{"nonexistent"})
	assert.True(t, result["line-length"])
	assert.False(t, result["nonexistent"])
}

func TestEffectiveCategoriesIgnoresMissingKindBody(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{},
	}
	result := EffectiveCategories(cfg, "doc.md", []string{"nonexistent"})
	assert.True(t, result["heading"], "default category still enabled")
}

// --- helpers ---

func loadFromString(t *testing.T, yml string) *Config {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(yml), 0o644))
	cfg, err := Load(cfgPath)
	require.NoError(t, err)
	return cfg
}
