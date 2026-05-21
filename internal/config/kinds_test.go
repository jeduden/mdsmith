package config

import (
	"os"
	"path/filepath"
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
	got := resolveEffectiveKinds(cfg, "file.md", []string{"a"}, nil)
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
	got := resolveEffectiveKinds(cfg, "file.md", []string{"a"}, nil)
	assert.Equal(t, []string{"a", "b"}, got)
}

func TestResolveEffectiveKindsNoAssignmentMatch(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{"a": {}},
		KindAssignment: []KindAssignmentEntry{
			{Files: []string{"docs/*.md"}, Kinds: []string{"a"}},
		},
	}
	got := resolveEffectiveKinds(cfg, "other/file.md", nil, nil)
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
	result := Effective(cfg, "wide/doc.md", nil, nil)
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
	result := Effective(cfg, "wide/special.md", nil, nil)
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
	result := Effective(cfg, "doc.md", []string{"a", "b"}, nil)
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
	result := Effective(cfg, "doc.md", []string{"a", "b"}, nil)
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
	result := EffectiveCategories(cfg, "_partials/foo.md", nil, nil)
	assert.False(t, result["meta"])
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
	result := EffectiveExplicitRules(cfg, "wide/doc.md", nil, nil)
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
	result := EffectiveExplicitRules(cfg, "doc.md", []string{"plan"}, nil)
	assert.True(t, result["paragraph-readability"])
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
	result := Effective(cfg, "doc.md", []string{"nonexistent"}, nil)
	assert.Equal(t, 80, result["line-length"].Settings["max"], "missing kind body is silently skipped")
}

func TestEffectiveExplicitRulesIgnoresMissingKindBody(t *testing.T) {
	cfg := &Config{
		ExplicitRules: map[string]bool{"line-length": true},
		Kinds:         map[string]KindBody{},
	}
	result := EffectiveExplicitRules(cfg, "doc.md", []string{"nonexistent"}, nil)
	assert.True(t, result["line-length"])
	assert.False(t, result["nonexistent"])
}

func TestEffectiveCategoriesIgnoresMissingKindBody(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{},
	}
	result := EffectiveCategories(cfg, "doc.md", []string{"nonexistent"}, nil)
	assert.True(t, result["heading"], "default category still enabled")
}

// --- copyKinds / copyRuleCfg isolation ---

func TestCopyKindsIsolatesSettingsFromSource(t *testing.T) {
	// Verify that mutating the copy's Settings map does not affect the original.
	original := map[string]KindBody{
		"wide": {Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		}},
	}
	copied := copyKinds(original)

	copied["wide"].Rules["line-length"].Settings["max"] = 999

	assert.Equal(t, 80, original["wide"].Rules["line-length"].Settings["max"],
		"mutation of copy's Settings should not affect the original")
}

func TestCopyKindsNilSettingsRemainNil(t *testing.T) {
	original := map[string]KindBody{
		"plan": {Rules: map[string]RuleCfg{
			"no-hard-tabs": {Enabled: true},
		}},
	}
	copied := copyKinds(original)
	assert.Nil(t, copied["plan"].Rules["no-hard-tabs"].Settings)
}

// --- path-pattern ---

func TestKindsPathPatternParsesFromYAML(t *testing.T) {
	yml := `
kinds:
  plan:
    path-pattern: "plan/[0-9][0-9]*_*.md"
  rfc:
    path-pattern: "docs/rfc/RFC-[0-9][0-9][0-9][0-9].md"
`
	cfg := loadFromString(t, yml)
	require.Contains(t, cfg.Kinds, "plan")
	assert.Equal(t, "plan/[0-9][0-9]*_*.md", cfg.Kinds["plan"].PathPattern)
	assert.Equal(t, "docs/rfc/RFC-[0-9][0-9][0-9][0-9].md",
		cfg.Kinds["rfc"].PathPattern)
}

func TestEffectiveRules_PathPatternInstallsListSetting(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"plan": {PathPattern: "plan/[0-9]*_*.md"},
		},
		KindAssignment: []KindAssignmentEntry{
			{Glob: []string{"plan/*.md"}, Kinds: []string{"plan"}},
		},
	}
	eff, _, _ := EffectiveAll(cfg, "plan/01_x.md", nil, nil)
	rs, ok := eff["required-structure"]
	require.True(t, ok, "required-structure rule should be present")
	assert.True(t, rs.Enabled)
	list, ok := rs.Settings["path-patterns"].([]any)
	require.True(t, ok, "path-patterns should be a list")
	require.Len(t, list, 1)
	entry := list[0].(map[string]any)
	assert.Equal(t, "plan", entry["kind"])
	assert.Equal(t, "plan/[0-9]*_*.md", entry["pattern"])
}

func TestEffectiveRules_PathPatternAccumulatesAcrossKinds(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"plan": {PathPattern: "plan/*.md"},
			"rfc":  {PathPattern: "docs/rfc/*.md"},
		},
	}
	eff, _, _ := EffectiveAll(cfg, "anywhere.md", []string{"plan", "rfc"}, nil)
	rs := eff["required-structure"]
	list, _ := rs.Settings["path-patterns"].([]any)
	require.Len(t, list, 2)
	assert.Equal(t, "plan", list[0].(map[string]any)["kind"])
	assert.Equal(t, "rfc", list[1].(map[string]any)["kind"])
}

func TestEffectiveRules_PathPatternAbsentLeavesRuleAlone(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"plan": {Rules: map[string]RuleCfg{"line-length": {Enabled: false}}},
		},
	}
	eff, _, _ := EffectiveAll(cfg, "anywhere.md", []string{"plan"}, nil)
	if rs, ok := eff["required-structure"]; ok {
		_, hasList := rs.Settings["path-patterns"]
		assert.False(t, hasList,
			"path-patterns should be absent when no kind declares one")
	}
}

// TestValidateKinds_RejectsInvalidPathPattern verifies that
// ValidateKinds rejects a kind whose top-level `path-pattern:`
// is not a valid doublestar glob. Without this, commands that
// load config but do not run the required-structure rule
// (e.g. `mdsmith kinds show`) would silently accept and display
// a malformed pattern.
func TestValidateKinds_RejectsInvalidPathPattern(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"plan": {PathPattern: "[unclosed"},
		},
	}
	err := ValidateKinds(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path-pattern")
	assert.Contains(t, err.Error(), "plan")
}

func TestValidateKinds_AcceptsValidPathPattern(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"plan": {PathPattern: "plan/[0-9][0-9]*_*.md"},
		},
	}
	assert.NoError(t, ValidateKinds(cfg))
}

// TestKindsExtendsParsesFromYAML pins the new top-level `extends:`
// key under a kind body. Without explicit coverage a typo in the
// YAML tag would silently drop the field on load.
func TestKindsExtendsParsesFromYAML(t *testing.T) {
	yml := `
kinds:
  rfc-base:
    schema:
      frontmatter:
        id: '=~"^RFC-[0-9]{4}$"'
  rfc-ratified:
    extends: rfc-base
    schema:
      frontmatter:
        status: '"ratified"'
`
	cfg := loadFromString(t, yml)
	require.Contains(t, cfg.Kinds, "rfc-base")
	require.Contains(t, cfg.Kinds, "rfc-ratified")
	assert.Empty(t, cfg.Kinds["rfc-base"].Extends)
	assert.Equal(t, "rfc-base", cfg.Kinds["rfc-ratified"].Extends)
}

// TestValidateKindExtends_AcceptsRootKind covers the trivial path:
// a kind without `extends:` is always valid.
func TestValidateKindExtends_AcceptsRootKind(t *testing.T) {
	kinds := map[string]KindBody{"a": {}}
	assert.NoError(t, validateKindExtends(kinds, "a"))
}

// TestValidateKindExtends_AcceptsValidChain checks a multi-step
// chain that terminates at a root kind.
func TestValidateKindExtends_AcceptsValidChain(t *testing.T) {
	kinds := map[string]KindBody{
		"a": {Extends: "b"},
		"b": {Extends: "c"},
		"c": {},
	}
	assert.NoError(t, validateKindExtends(kinds, "a"))
}

// TestValidateKindExtends_RejectsUnknownParent verifies the parent
// lookup; a typo in `extends:` must surface at load time.
func TestValidateKindExtends_RejectsUnknownParent(t *testing.T) {
	kinds := map[string]KindBody{"a": {Extends: "missing"}}
	err := validateKindExtends(kinds, "a")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "undeclared kind")
}

// TestValidateKindExtends_RejectsCycle covers cycle detection in
// the helper in isolation; the public-API test goes through
// ValidateKinds.
func TestValidateKindExtends_RejectsCycle(t *testing.T) {
	kinds := map[string]KindBody{
		"a": {Extends: "b"},
		"b": {Extends: "a"},
	}
	err := validateKindExtends(kinds, "a")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

// TestValidateKinds_AcceptsExtendsChain covers the happy path for
// schema inheritance (plan 135): a child that names a declared
// parent passes validation.
func TestValidateKinds_AcceptsExtendsChain(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"rfc-base":     {},
			"rfc-ratified": {Extends: "rfc-base"},
		},
	}
	assert.NoError(t, ValidateKinds(cfg))
}

// TestValidateKinds_RejectsExtendsUndeclaredParent verifies that
// `extends:` referencing an unknown kind surfaces as a load-time
// error rather than silently producing an empty parent at merge time.
func TestValidateKinds_RejectsExtendsUndeclaredParent(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"rfc-ratified": {Extends: "missing-base"},
		},
	}
	err := ValidateKinds(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "undeclared kind")
	assert.Contains(t, err.Error(), "missing-base")
}

// TestValidateKinds_RejectsSelfExtends catches the simplest cycle
// (a → a) so the validator surfaces it at config load.
func TestValidateKinds_RejectsSelfExtends(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"a": {Extends: "a"},
		},
	}
	err := ValidateKinds(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
	assert.Contains(t, err.Error(), "a -> a")
}

// TestValidateKinds_RejectsMultiHopExtendsCycle is the multi-hop
// variant: a → b → c → a must surface the full path.
func TestValidateKinds_RejectsMultiHopExtendsCycle(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"a": {Extends: "b"},
			"b": {Extends: "c"},
			"c": {Extends: "a"},
		},
	}
	err := ValidateKinds(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
	assert.Contains(t, err.Error(), " -> ")
}

// TestKindLayerRules_MergesPathPatternWithExistingRules verifies
// that the provenance helper preserves a kind's existing body.Rules
// settings while injecting the synthetic `path-patterns` entry on
// top of them. Without this, a kind that both disables a rule and
// declares a `path-pattern:` would have its `body.Rules` ignored in
// `kinds resolve` / `--explain` output. The `schema:` setting is
// translated to a `schema-sources` entry so the provenance chain
// reflects the deep-merged form rather than the raw user input.
func TestKindLayerRules_MergesPathPatternWithExistingRules(t *testing.T) {
	body := KindBody{
		PathPattern: "plan/*.md",
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: false},
			"required-structure": {Enabled: true,
				Settings: map[string]any{"schema": "plan/proto.md"}},
		},
	}
	out := kindLayerRules("plan", body, nil)
	require.Contains(t, out, "line-length")
	assert.False(t, out["line-length"].Enabled)
	rs := out["required-structure"]
	assert.True(t, rs.Enabled)
	sources, ok := rs.Settings["schema-sources"].([]any)
	require.True(t, ok, "schema-sources must accumulate body.Rules schema source")
	require.Len(t, sources, 1)
	assert.Equal(t, "plan/proto.md",
		sources[0].(map[string]any)["file"],
		"existing required-structure settings must be preserved")
	list := rs.Settings["path-patterns"].([]any)
	require.Len(t, list, 1)
	assert.Equal(t, "plan", list[0].(map[string]any)["kind"])
}

// TestKindLayerRules_MirrorsInlineSchemaAndPathPattern verifies
// that a kind declaring both `schema:` (an inline schema map) and
// `path-pattern:` lands BOTH synthetic settings in the provenance
// layer chain — without this, `kinds resolve` / `--explain` would
// drop the schema source leaf even though effectiveRules applies it.
func TestKindLayerRules_MirrorsInlineSchemaAndPathPattern(t *testing.T) {
	body := KindBody{
		PathPattern: "plan/*.md",
		Schema: map[string]any{
			"sections": []any{
				map[string]any{"heading": "Goal"},
			},
		},
	}
	out := kindLayerRules("plan", body, nil)
	rs := out["required-structure"]
	assert.True(t, rs.Enabled)

	sources, ok := rs.Settings["schema-sources"].([]any)
	require.True(t, ok, "schema-sources must be injected as a list")
	require.Len(t, sources, 1)
	entry := sources[0].(map[string]any)
	inlineMap, ok := entry["inline"].(map[string]any)
	require.True(t, ok, "inline entry must wrap the schema map")
	assert.Contains(t, inlineMap, "sections")

	list, ok := rs.Settings["path-patterns"].([]any)
	require.True(t, ok)
	require.Len(t, list, 1)
	assert.Equal(t, "plan/*.md",
		list[0].(map[string]any)["pattern"])
}

// TestKindLayerRules_TranslatesBodyRulesSchema covers the provenance
// translation of body.Rules' legacy `schema:` setting when the kind
// has neither `KindBody.Schema` (inline map) nor `path-pattern:`.
// The provenance chain must surface `schema-sources` for that case
// too, so explainers don't show a stale `schema:` key.
func TestKindLayerRules_TranslatesBodyRulesSchema(t *testing.T) {
	body := KindBody{
		Rules: map[string]RuleCfg{
			"required-structure": {
				Enabled: true,
				Settings: map[string]any{
					"schema": "plan/proto.md",
				},
			},
		},
	}
	out := kindLayerRules("plan", body, nil)
	rs := out["required-structure"]
	sources, ok := rs.Settings["schema-sources"].([]any)
	require.True(t, ok)
	require.Len(t, sources, 1)
	assert.Equal(t, "plan/proto.md", sources[0].(map[string]any)["file"])
	assert.NotContains(t, rs.Settings, "schema",
		"legacy schema key should be stripped after translation")
}

// TestKindLayerRules_NoTranslationNeededReturnsSameMap exercises the
// fast path: a body whose required-structure entry has no schema
// keys should not allocate a new rules map.
func TestKindLayerRules_NoTranslationNeededReturnsSameMap(t *testing.T) {
	body := KindBody{
		Rules: map[string]RuleCfg{
			"required-structure": {
				Enabled: true,
				Settings: map[string]any{
					"placeholders": []any{"cue-frontmatter"},
				},
			},
		},
	}
	out := kindLayerRules("plan", body, nil)
	// The function returns body.Rules directly in this path because
	// neither body.Schema nor body.PathPattern is set, and the
	// required-structure entry has no schema source to translate.
	assert.Equal(t, body.Rules["required-structure"].Settings["placeholders"],
		out["required-structure"].Settings["placeholders"])
	assert.NotContains(t, out["required-structure"].Settings, "schema-sources")
}

// TestKindLayerRules_BodyRulesInlineSchemaTranslated covers the
// `inline-schema:` translation in body.Rules (parallel to the
// file-path translation above).
func TestKindLayerRules_BodyRulesInlineSchemaTranslated(t *testing.T) {
	body := KindBody{
		Rules: map[string]RuleCfg{
			"required-structure": {
				Enabled: true,
				Settings: map[string]any{
					"inline-schema": map[string]any{
						"sections": []any{
							map[string]any{"heading": "Goal"},
						},
					},
				},
			},
		},
	}
	out := kindLayerRules("plan", body, nil)
	rs := out["required-structure"]
	sources, ok := rs.Settings["schema-sources"].([]any)
	require.True(t, ok)
	require.Len(t, sources, 1)
	inlineMap, ok := sources[0].(map[string]any)["inline"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, inlineMap, "sections")
	assert.NotContains(t, rs.Settings, "inline-schema",
		"legacy inline-schema key should be stripped after translation")
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
