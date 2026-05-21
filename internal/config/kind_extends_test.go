package config

import (
	"errors"
	"testing"

	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveKindInlineSchema_NoSchemaReturnsNil covers a kind that
// declares neither a schema nor an extends parent: the resolver
// reports no inline schema rather than synthesising an empty map.
func TestResolveKindInlineSchema_NoSchemaReturnsNil(t *testing.T) {
	kinds := map[string]KindBody{"plan": {}}
	out, err := ResolveKindInlineSchema(kinds, "plan")
	require.NoError(t, err)
	assert.Nil(t, out)
}

// TestResolveKindInlineSchema_NoExtendsNormalisesShortcuts pins
// the post-review behaviour: every chain (even a single-layer
// one) runs through MergeRawMap so bare-name shortcuts expand.
// Without this, `kinds show` would render `date` for a
// shortcut-bearing schema instead of the canonical CUE regex.
func TestResolveKindInlineSchema_NoExtendsNormalisesShortcuts(t *testing.T) {
	body := KindBody{Schema: map[string]any{"frontmatter": map[string]any{
		"d": "date",
	}}}
	kinds := map[string]KindBody{"plan": body}
	out, err := ResolveKindInlineSchema(kinds, "plan")
	require.NoError(t, err)
	fm := out["frontmatter"].(map[string]any)
	expr, ok := fm["d"].(string)
	require.True(t, ok)
	assert.Contains(t, expr, "=~",
		"single-layer schema should still pass through the normaliser")
}

// TestResolveKindInlineSchema_ParentChildMerge is the headline
// behaviour: child's frontmatter keys are added, parent's keys
// flow through, and child sections wholly replace parent's.
func TestResolveKindInlineSchema_ParentChildMerge(t *testing.T) {
	kinds := map[string]KindBody{
		"rfc-base": {Schema: map[string]any{
			"frontmatter": map[string]any{
				"id": `=~"^RFC-[0-9]{4}$"`,
			},
			"sections": []any{
				map[string]any{"heading": "Context"},
			},
		}},
		"rfc-ratified": {
			Extends: "rfc-base",
			Schema: map[string]any{
				"frontmatter": map[string]any{
					"status": `"ratified"`,
				},
				"sections": []any{
					map[string]any{"heading": "Decision"},
				},
			},
		},
	}
	out, err := ResolveKindInlineSchema(kinds, "rfc-ratified")
	require.NoError(t, err)
	fm := out["frontmatter"].(map[string]any)
	assert.Contains(t, fm, "id", "parent frontmatter survives")
	assert.Contains(t, fm, "status", "child frontmatter is added")
	secs := out["sections"].([]any)
	require.Len(t, secs, 1)
	assert.Equal(t, "Decision", secs[0].(map[string]any)["heading"],
		"child sections wholly replace parent's")
}

// TestResolveKindInlineSchema_FrontmatterUnifies verifies the CUE
// refinement path: child narrows a parent disjunction; the unified
// expression joins with `&`.
func TestResolveKindInlineSchema_FrontmatterUnifies(t *testing.T) {
	kinds := map[string]KindBody{
		"base": {Schema: map[string]any{"frontmatter": map[string]any{
			"status": `"open" | "closed" | "ratified"`,
		}}},
		"child": {
			Extends: "base",
			Schema: map[string]any{"frontmatter": map[string]any{
				"status": `"ratified"`,
			}},
		},
	}
	out, err := ResolveKindInlineSchema(kinds, "child")
	require.NoError(t, err)
	fm := out["frontmatter"].(map[string]any)
	assert.Contains(t, fm["status"], "&")
}

// TestResolveKindInlineSchema_MergesConflictExpressionsWithoutError
// covers the post-refactor contract: ResolveKindInlineSchema is
// structural-only — it merges the two expressions with `&` but
// does not CUE-validate the result. Use ValidateKindInlineSchema
// (or ValidateKinds at load time) to surface the conflict.
func TestResolveKindInlineSchema_MergesConflictExpressionsWithoutError(t *testing.T) {
	kinds := map[string]KindBody{
		"base": {Schema: map[string]any{"frontmatter": map[string]any{
			"status": `"open" | "closed"`,
		}}},
		"child": {
			Extends: "base",
			Schema: map[string]any{"frontmatter": map[string]any{
				"status": `int`,
			}},
		},
	}
	out, err := ResolveKindInlineSchema(kinds, "child")
	require.NoError(t, err)
	fm := out["frontmatter"].(map[string]any)
	assert.Contains(t, fm["status"], "&",
		"merged expression keeps the conflict for the load-time validator")
}

// TestValidateKindInlineSchema_NamesKindOnConflict pins the
// load-time validator: an unsatisfiable child surfaces as an
// UnsatisfiableKeyError wrapped with the child kind name.
func TestValidateKindInlineSchema_NamesKindOnConflict(t *testing.T) {
	kinds := map[string]KindBody{
		"base": {Schema: map[string]any{"frontmatter": map[string]any{
			"status": `"open" | "closed"`,
		}}},
		"child": {
			Extends: "base",
			Schema: map[string]any{"frontmatter": map[string]any{
				"status": `int`,
			}},
		},
	}
	err := ValidateKindInlineSchema(kinds, "child")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "child")
	var keyErr *schema.UnsatisfiableKeyError
	require.True(t, errors.As(err, &keyErr))
	assert.Equal(t, "status", keyErr.Key)
}

// TestValidateKindInlineSchema_NoErrorWhenChainIsCompatible
// confirms the validator passes a refinement (child narrowing
// parent's disjunction).
func TestValidateKindInlineSchema_NoErrorWhenChainIsCompatible(t *testing.T) {
	kinds := map[string]KindBody{
		"base": {Schema: map[string]any{"frontmatter": map[string]any{
			"status": `"open" | "closed"`,
		}}},
		"child": {
			Extends: "base",
			Schema: map[string]any{"frontmatter": map[string]any{
				"status": `"open"`,
			}},
		},
	}
	assert.NoError(t, ValidateKindInlineSchema(kinds, "child"))
}

// TestValidateKindInlineSchema_NoExtendsReturnsNil covers the
// fast-path: a kind without `extends:` has no merge to validate.
func TestValidateKindInlineSchema_NoExtendsReturnsNil(t *testing.T) {
	kinds := map[string]KindBody{
		"a": {Schema: map[string]any{"frontmatter": map[string]any{"x": "string"}}},
	}
	assert.NoError(t, ValidateKindInlineSchema(kinds, "a"))
}

// TestValidateKindInlineSchema_PropagatesCycleError verifies the
// validator surfaces structural errors from the resolver.
func TestValidateKindInlineSchema_PropagatesCycleError(t *testing.T) {
	kinds := map[string]KindBody{
		"a": {Extends: "b", Schema: map[string]any{"filename": "a.md"}},
		"b": {Extends: "a", Schema: map[string]any{"filename": "b.md"}},
	}
	err := ValidateKindInlineSchema(kinds, "a")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

// TestResolveKindInlineSchema_ParentOnlySchema covers the path
// where the child carries no inline schema but the parent does: the
// resolver returns the parent's schema directly.
func TestResolveKindInlineSchema_ParentOnlySchema(t *testing.T) {
	kinds := map[string]KindBody{
		"parent": {Schema: map[string]any{"filename": "RFC-*.md"}},
		"child":  {Extends: "parent"},
	}
	out, err := ResolveKindInlineSchema(kinds, "child")
	require.NoError(t, err)
	assert.Equal(t, "RFC-*.md", out["filename"])
}

// TestResolveKindInlineSchema_MultiHopChain verifies that the
// resolver walks more than one extends hop.
func TestResolveKindInlineSchema_MultiHopChain(t *testing.T) {
	kinds := map[string]KindBody{
		"a": {Schema: map[string]any{"frontmatter": map[string]any{"a": `string`}}},
		"b": {Extends: "a", Schema: map[string]any{"frontmatter": map[string]any{"b": `string`}}},
		"c": {Extends: "b", Schema: map[string]any{"frontmatter": map[string]any{"c": `string`}}},
	}
	out, err := ResolveKindInlineSchema(kinds, "c")
	require.NoError(t, err)
	fm := out["frontmatter"].(map[string]any)
	assert.Contains(t, fm, "a")
	assert.Contains(t, fm, "b")
	assert.Contains(t, fm, "c")
}

// TestResolveKindInlineSchema_CycleDefenseInDepth re-detects a
// cycle even when ValidateKinds was bypassed; without this, a
// caller that constructed a kinds map directly would hang.
func TestResolveKindInlineSchema_CycleDefenseInDepth(t *testing.T) {
	kinds := map[string]KindBody{
		"a": {Extends: "b", Schema: map[string]any{"filename": "a.md"}},
		"b": {Extends: "a", Schema: map[string]any{"filename": "b.md"}},
	}
	_, err := ResolveKindInlineSchema(kinds, "a")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

// TestResolveKindInlineSchema_UndeclaredParentDefenseInDepth catches
// the same gap for a typoed parent name when ValidateKinds was
// skipped.
func TestResolveKindInlineSchema_UndeclaredParentDefenseInDepth(t *testing.T) {
	kinds := map[string]KindBody{
		"child": {Extends: "missing"},
	}
	_, err := ResolveKindInlineSchema(kinds, "child")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "undeclared")
}

// TestKindExtendsChain_Root reports a kind with no parent as a
// single-element chain.
func TestKindExtendsChain_Root(t *testing.T) {
	kinds := map[string]KindBody{"a": {}}
	chain := KindExtendsChain(kinds, "a")
	assert.Equal(t, []string{"a"}, chain)
}

// TestKindExtendsChain_MultiLevel walks the chain child-first.
func TestKindExtendsChain_MultiLevel(t *testing.T) {
	kinds := map[string]KindBody{
		"a": {Extends: "b"},
		"b": {Extends: "c"},
		"c": {},
	}
	chain := KindExtendsChain(kinds, "a")
	assert.Equal(t, []string{"a", "b", "c"}, chain)
}

// TestKindExtendsChain_StopsOnCycleDefensively avoids an infinite
// loop when called on a malformed kinds map.
func TestKindExtendsChain_StopsOnCycleDefensively(t *testing.T) {
	kinds := map[string]KindBody{
		"a": {Extends: "b"},
		"b": {Extends: "a"},
	}
	chain := KindExtendsChain(kinds, "a")
	assert.Equal(t, []string{"a", "b"}, chain)
}

// TestExtendsChainSchemas_DropsEmptyLayers ensures intermediate
// kinds without an inline schema do not contribute an empty
// MergeRawMap call.
func TestExtendsChainSchemas_DropsEmptyLayers(t *testing.T) {
	kinds := map[string]KindBody{
		"a": {Schema: map[string]any{"filename": "a.md"}},
		"b": {Extends: "a"},
		"c": {Extends: "b", Schema: map[string]any{"frontmatter": map[string]any{"x": "string"}}},
	}
	chain, err := extendsChainSchemas(kinds, "c")
	require.NoError(t, err)
	require.Len(t, chain, 2, "kind b has no schema and drops out")
	assert.Equal(t, "a", chain[0].kind, "root layer comes first")
	assert.Equal(t, "c", chain[1].kind, "child layer comes last")
}

// TestResolvedInlineSchema_FallbackOnError exercises the defensive
// branch in resolvedInlineSchema: when the resolver errors (e.g.
// because kinds has been mutated post-validation to introduce a
// cycle), the merge layer falls back to the kind's own body.Schema
// rather than dropping the schema source entirely.
func TestResolvedInlineSchema_FallbackOnError(t *testing.T) {
	kinds := map[string]KindBody{
		"a": {Extends: "b", Schema: map[string]any{"filename": "a.md"}},
		"b": {Extends: "a", Schema: map[string]any{"filename": "b.md"}},
	}
	out := resolvedInlineSchema(kinds, "a", kinds["a"])
	require.NotNil(t, out)
	assert.Equal(t, "a.md", out["filename"],
		"defensive fallback uses the kind's own schema when the chain errors")
}

// TestResolvedInlineSchema_NoExtendsReturnsBodySchema covers the
// fast-path branch: no extends → no resolver call, return body
// schema unchanged.
func TestResolvedInlineSchema_NoExtendsReturnsBodySchema(t *testing.T) {
	body := KindBody{Schema: map[string]any{"filename": "x.md"}}
	out := resolvedInlineSchema(map[string]KindBody{"a": body}, "a", body)
	assert.Equal(t, "x.md", out["filename"])
}

// TestResolveLayerInlineSchema_NilKindsMapFallback covers the
// provenance helper's nil-kinds fallback: when callers (unit tests
// that don't construct a kinds map) pass nil, the function returns
// the kind's own schema.
func TestResolveLayerInlineSchema_NilKindsMapFallback(t *testing.T) {
	body := KindBody{
		Extends: "missing",
		Schema:  map[string]any{"filename": "x.md"},
	}
	out := resolveLayerInlineSchema("a", body, nil)
	assert.Equal(t, "x.md", out["filename"])
}

// TestResolveLayerInlineSchema_NoExtendsReturnsBodySchema covers
// the early-return branch when the kind has no extends.
func TestResolveLayerInlineSchema_NoExtendsReturnsBodySchema(t *testing.T) {
	body := KindBody{Schema: map[string]any{"filename": "x.md"}}
	out := resolveLayerInlineSchema("a", body, map[string]KindBody{"a": body})
	assert.Equal(t, "x.md", out["filename"])
}

// TestResolveLayerInlineSchema_WithExtendsCallsResolver verifies the
// provenance helper invokes the resolver when extends is set.
func TestResolveLayerInlineSchema_WithExtendsCallsResolver(t *testing.T) {
	kinds := map[string]KindBody{
		"base": {Schema: map[string]any{
			"frontmatter": map[string]any{"id": "string"},
		}},
		"child": {Extends: "base", Schema: map[string]any{
			"frontmatter": map[string]any{"status": `"ratified"`},
		}},
	}
	out := resolveLayerInlineSchema("child", kinds["child"], kinds)
	require.NotNil(t, out)
	fm := out["frontmatter"].(map[string]any)
	assert.Contains(t, fm, "id", "parent frontmatter flows through")
	assert.Contains(t, fm, "status", "child frontmatter is included")
}

// TestResolveLayerInlineSchema_DefensiveFallbackOnResolverError
// confirms the helper does not panic when the kinds map is
// malformed; it falls back to the kind's own schema.
func TestResolveLayerInlineSchema_DefensiveFallbackOnResolverError(t *testing.T) {
	kinds := map[string]KindBody{
		"a": {Extends: "b", Schema: map[string]any{"filename": "a.md"}},
		"b": {Extends: "a", Schema: map[string]any{"filename": "b.md"}},
	}
	out := resolveLayerInlineSchema("a", kinds["a"], kinds)
	assert.Equal(t, "a.md", out["filename"])
}

// TestKindExtendsChain_UnknownIntermediateBreaksChain covers the
// defensive branch in KindExtendsChain that stops when a parent
// name is not declared in `kinds`. The chain returned still
// contains the unknown name so callers see the dangling pointer in
// the audit trail rather than an empty result.
func TestKindExtendsChain_UnknownIntermediateBreaksChain(t *testing.T) {
	kinds := map[string]KindBody{
		"a": {Extends: "missing"},
	}
	chain := KindExtendsChain(kinds, "a")
	assert.Equal(t, []string{"a", "missing"}, chain)
}

// TestValidateKinds_RejectsExtendsFrontmatterConflict covers the
// second pass in ValidateKinds: a well-formed extends chain whose
// frontmatter cannot unify surfaces at config load.
func TestValidateKinds_RejectsExtendsFrontmatterConflict(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"base": {Schema: map[string]any{
				"frontmatter": map[string]any{"x": "int"},
			}},
			"child": {Extends: "base", Schema: map[string]any{
				"frontmatter": map[string]any{"x": "string"},
			}},
		},
	}
	err := ValidateKinds(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "child")
}

// TestEffectiveRules_ExtendsResolvedInSchemaSources is the
// integration-level acceptance check for plan 135 inline kinds:
// when a kind extends a parent, the schema-sources entry installed
// on `required-structure` carries the merged inline schema.
func TestEffectiveRules_ExtendsResolvedInSchemaSources(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"rfc-base": {Schema: map[string]any{
				"frontmatter": map[string]any{
					"id": `=~"^RFC-[0-9]{4}$"`,
				},
			}},
			"rfc-ratified": {
				Extends: "rfc-base",
				Schema: map[string]any{
					"frontmatter": map[string]any{
						"status": `"ratified"`,
					},
				},
			},
		},
	}
	eff, _, _ := EffectiveAll(cfg, "anywhere.md", []string{"rfc-ratified"}, nil)
	rs, ok := eff["required-structure"]
	require.True(t, ok, "extends must surface required-structure")
	sources, ok := rs.Settings["schema-sources"].([]any)
	require.True(t, ok, "schema-sources must be installed by the merge layer")
	require.Len(t, sources, 1, "extends collapses to one merged source")
	inline, ok := sources[0].(map[string]any)["inline"].(map[string]any)
	require.True(t, ok)
	fm := inline["frontmatter"].(map[string]any)
	assert.Contains(t, fm, "id", "parent frontmatter survives in merged source")
	assert.Contains(t, fm, "status", "child frontmatter survives in merged source")
}

// TestEffectiveRules_ExtendsParentSchemaWithoutChild covers the
// path where the child kind has no inline schema of its own but
// extends a parent that does: the parent's schema must still be
// installed via the child kind's merge layer.
func TestEffectiveRules_ExtendsParentSchemaWithoutChild(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"base":  {Schema: map[string]any{"filename": "RFC-*.md"}},
			"child": {Extends: "base"},
		},
	}
	eff, _, _ := EffectiveAll(cfg, "anywhere.md", []string{"child"}, nil)
	rs := eff["required-structure"]
	sources := rs.Settings["schema-sources"].([]any)
	require.Len(t, sources, 1)
	inline := sources[0].(map[string]any)["inline"].(map[string]any)
	assert.Equal(t, "RFC-*.md", inline["filename"])
}
