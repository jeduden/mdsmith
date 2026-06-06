package requiredstructure

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTranslateLayerSettings_FileSource covers the rule's
// rule.SettingsTranslator implementation: a `schema:` file path
// becomes a one-entry schema-sources list and the legacy key is
// stripped.
func TestTranslateLayerSettings_FileSource(t *testing.T) {
	r := &Rule{}
	out := r.TranslateLayerSettings(map[string]any{
		"schema":       "internal/rules/proto.md",
		"placeholders": []any{"cue-frontmatter"},
	})
	assert.NotContains(t, out, "schema")
	srcs, ok := out["schema-sources"].([]any)
	require.True(t, ok)
	require.Len(t, srcs, 1)
	assert.Equal(t, "internal/rules/proto.md",
		srcs[0].(map[string]any)["file"])
	assert.Contains(t, out, "placeholders",
		"unrelated keys must survive translation")
}

// TestTranslateLayerSettings_InlineSource covers the inline-schema
// branch of the translator.
func TestTranslateLayerSettings_InlineSource(t *testing.T) {
	r := &Rule{}
	out := r.TranslateLayerSettings(map[string]any{
		"inline-schema": map[string]any{
			"sections": []any{map[string]any{"heading": "Goal"}},
		},
	})
	assert.NotContains(t, out, "inline-schema")
	srcs, ok := out["schema-sources"].([]any)
	require.True(t, ok)
	require.Len(t, srcs, 1)
	inl, ok := srcs[0].(map[string]any)["inline"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, inl, "sections")
}

// TestTranslateLayerSettings_NoSchemaKeyPassesThrough covers the
// "no translation applies" path: a settings map with no schema
// keys is returned unchanged (same map identity), so the merge
// layer's no-translation fast path holds.
func TestTranslateLayerSettings_NoSchemaKeyPassesThrough(t *testing.T) {
	r := &Rule{}
	in := map[string]any{"placeholders": []any{"x"}}
	out := r.TranslateLayerSettings(in)
	assert.Equal(t, in["placeholders"], out["placeholders"])
	assert.NotContains(t, out, "schema-sources")
}

// TestTranslateLayerSettings_EmptyValuesStripped covers the
// hadKey-but-no-source path: empty `schema: ""` and empty
// `inline-schema: {}` are stripped without adding a source.
func TestTranslateLayerSettings_EmptyValuesStripped(t *testing.T) {
	r := &Rule{}
	out := r.TranslateLayerSettings(map[string]any{
		"schema":        "",
		"inline-schema": map[string]any{},
	})
	assert.NotContains(t, out, "schema")
	assert.NotContains(t, out, "inline-schema")
	assert.NotContains(t, out, "schema-sources")
}

// TestTranslateLayerSettings_AccumulatesOntoExistingList covers the
// branch where a layer already carries a schema-sources list (e.g.
// a prior translator pass): the new entry appends rather than
// replacing.
func TestTranslateLayerSettings_AccumulatesOntoExistingList(t *testing.T) {
	r := &Rule{}
	out := r.TranslateLayerSettings(map[string]any{
		"schema": "b.md",
		"schema-sources": []any{
			map[string]any{"file": "a.md"},
		},
	})
	srcs, ok := out["schema-sources"].([]any)
	require.True(t, ok)
	require.Len(t, srcs, 2)
	assert.Equal(t, "a.md", srcs[0].(map[string]any)["file"])
	assert.Equal(t, "b.md", srcs[1].(map[string]any)["file"])
}

// TestTranslateLayerSettings_NonStringSchema covers the non-string
// `schema:` value branch of extractSchemaSourceFromSettings.
func TestTranslateLayerSettings_NonStringSchema(t *testing.T) {
	r := &Rule{}
	out := r.TranslateLayerSettings(map[string]any{"schema": 42})
	assert.NotContains(t, out, "schema")
	assert.NotContains(t, out, "schema-sources")
}

// TestTranslateLayerSettings_NonMapInline covers the non-map
// `inline-schema:` value branch.
func TestTranslateLayerSettings_NonMapInline(t *testing.T) {
	r := &Rule{}
	out := r.TranslateLayerSettings(map[string]any{"inline-schema": 7})
	assert.NotContains(t, out, "inline-schema")
	assert.NotContains(t, out, "schema-sources")
}

// TestTranslateLayerSettings_DeepClonesNestedValues verifies the
// translator does not alias the caller's nested maps/slices, so a
// later mutation of the input cannot corrupt the merged config.
func TestTranslateLayerSettings_DeepClonesNestedValues(t *testing.T) {
	r := &Rule{}
	inline := map[string]any{
		"sections": []any{map[string]any{"heading": "Goal"}},
	}
	out := r.TranslateLayerSettings(map[string]any{"inline-schema": inline})
	// Mutate the original nested structures.
	inline["sections"].([]any)[0].(map[string]any)["heading"] = "MUTATED"
	srcs := out["schema-sources"].([]any)
	got := srcs[0].(map[string]any)["inline"].(map[string]any)
	sec := got["sections"].([]any)[0].(map[string]any)
	assert.Equal(t, "Goal", sec["heading"],
		"translator must deep-clone inline schema, not alias it")
}

// TestCloneSettingsDeep_NilAndScalarTypes covers cloneSettingsDeep's
// nil short-circuit and cloneSettingsValue's []string / []int /
// nested-map clone branches. These value shapes appear in
// programmatically-built RuleCfg.Settings (YAML decodes lists as
// []any, but config code and tests construct typed slices).
func TestCloneSettingsDeep_NilAndScalarTypes(t *testing.T) {
	assert.Nil(t, cloneSettingsDeep(nil))

	in := map[string]any{
		"strs":   []string{"a", "b"},
		"ints":   []int{1, 2},
		"nested": map[string]any{"k": []any{"v"}},
		"scalar": 7,
	}
	out := cloneSettingsDeep(in)
	// Mutating the originals must not affect the clone.
	in["strs"].([]string)[0] = "X"
	in["ints"].([]int)[0] = 99
	in["nested"].(map[string]any)["k"] = "changed"
	assert.Equal(t, []string{"a", "b"}, out["strs"])
	assert.Equal(t, []int{1, 2}, out["ints"])
	assert.Equal(t, []any{"v"},
		out["nested"].(map[string]any)["k"])
	assert.Equal(t, 7, out["scalar"])
}

// TestRule_ImplementsSettingsTranslator is a compile-time-ish
// guard that the rule satisfies the interface the merge layer
// type-asserts on.
func TestRule_ImplementsSettingsTranslator(t *testing.T) {
	var _ rule.SettingsTranslator = (*Rule)(nil)
	r := rule.ByName("required-structure")
	require.NotNil(t, r)
	_, ok := r.(rule.SettingsTranslator)
	assert.True(t, ok,
		"required-structure must be discoverable as a SettingsTranslator via the registry")
}

// TestApplySettings_SchemaSourcesList exercises plan 156's primary
// new input: a `schema-sources` list installed by the merge layer.
// The rule loads each entry into Sources and reflects the first
// entry into Schema / InlineSchema only when the list has exactly
// one element.
func TestApplySettings_SchemaSourcesList(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"schema-sources": []any{
			map[string]any{"file": "schemas/a.md"},
			map[string]any{"inline": map[string]any{
				"sections": []any{map[string]any{"heading": "Goal"}},
			}},
		},
	})
	require.NoError(t, err)
	require.Len(t, r.Sources, 2)
	assert.Equal(t, "schemas/a.md", r.Sources[0].File)
	assert.NotNil(t, r.Sources[1].Inline)
	// Multi-source: the single-source fields are not authoritative.
	assert.Empty(t, r.Schema, "Schema must be empty for multi-source configs")
	assert.Nil(t, r.InlineSchema, "InlineSchema must be empty for multi-source configs")
}

func TestApplySettings_SchemaSourcesSingleEntryReflectsLegacyFields(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"schema-sources": []any{
			map[string]any{"file": "schemas/a.md"},
		},
	})
	require.NoError(t, err)
	require.Len(t, r.Sources, 1)
	assert.Equal(t, "schemas/a.md", r.Schema,
		"single file source must reflect into r.Schema for legacy callers")
}

func TestApplySettings_SchemaSourcesSingleInlineReflectsLegacyFields(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"schema-sources": []any{
			map[string]any{"inline": map[string]any{
				"sections": []any{map[string]any{"heading": "Goal"}},
			}},
		},
	})
	require.NoError(t, err)
	require.Len(t, r.Sources, 1)
	require.NotNil(t, r.InlineSchema,
		"single inline source must reflect into r.InlineSchema")
	assert.Empty(t, r.Schema,
		"single inline source must clear r.Schema")
}

func TestApplySettings_SchemaSourcesRejectsBothKeys(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"schema-sources": []any{
			map[string]any{
				"file":   "x.md",
				"inline": map[string]any{"sections": []any{}},
			},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "may set only one")
}

func TestApplySettings_SchemaSourcesRejectsEmptyEntry(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"schema-sources": []any{
			map[string]any{},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must set `file` or `inline`")
}

func TestApplySettings_SchemaSourcesRejectsNonList(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"schema-sources": "not-a-list",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema-sources must be a list")
}

// TestCheck_ComposedSchemasRequireBothSets is acceptance criterion
// #1 of plan 156: a file resolved by two kinds with disjoint
// required sections must fail until both sets are present.
func TestCheck_ComposedSchemasRequireBothSets(t *testing.T) {
	schA := mustInline(t, map[string]any{
		"sections": []any{map[string]any{"heading": "Goal"}},
	})
	schB := mustInline(t, map[string]any{
		"sections": []any{map[string]any{"heading": "Risks"}},
	})

	r := &Rule{Sources: []SchemaSource{{Inline: schA}, {Inline: schB}}}

	// Missing Risks.
	f := newTestFile(t, "doc.md", "# Plan\n\n## Goal\n\nx\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, `## Risks: got <missing>`)

	// Missing Goal.
	f2 := newTestFile(t, "doc.md", "# Plan\n\n## Risks\n\nx\n")
	diags = r.Check(f2)
	expectDiagMsg(t, diags, `## Goal: got <missing>`)

	// Both present — clean.
	f3 := newTestFile(t, "doc.md",
		"# Plan\n\n## Goal\n\nx\n\n## Risks\n\ny\n")
	diags = r.Check(f3)
	assert.Empty(t, diags,
		"file satisfying both kinds' required sections must validate cleanly")
}

// TestCheck_ComposedSchemasComposeFrontmatter is acceptance
// criterion #2: a file resolving to two kinds with disjoint
// required frontmatter keys must fail until both sets are present.
func TestCheck_ComposedSchemasComposeFrontmatter(t *testing.T) {
	schA := mustInline(t, map[string]any{
		"frontmatter": map[string]any{
			"id": `=~"^A-[0-9]+$"`,
		},
	})
	schB := mustInline(t, map[string]any{
		"frontmatter": map[string]any{
			"category": `"alpha" | "beta"`,
		},
	})
	r := &Rule{Sources: []SchemaSource{{Inline: schA}, {Inline: schB}}}

	// Missing category.
	doc := "---\nid: A-1\n---\n# T\n"
	f := newTestFile(t, "doc.md", doc)
	diags := r.Check(f)
	require.NotEmpty(t, diags)
	expectDiagMsg(t, diags, `category: got <missing>`)

	// Has both — clean.
	doc = "---\nid: A-1\ncategory: alpha\n---\n# T\n"
	f = newTestFile(t, "doc.md", doc)
	diags = r.Check(f)
	assert.Empty(t, diags,
		"frontmatter that satisfies both schemas must validate cleanly")
}

// TestCheck_ComposedSchemasMergeSameHeading covers the merge-by-text
// rule: two schemas that share a literal heading combine their
// child sections.
func TestCheck_ComposedSchemasMergeSameHeading(t *testing.T) {
	schA := mustInline(t, map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Meta-Information",
				"sections": []any{
					map[string]any{"heading": "ID"},
				},
			},
		},
	})
	schB := mustInline(t, map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Meta-Information",
				"sections": []any{
					map[string]any{"heading": "Owner"},
				},
			},
		},
	})
	r := &Rule{Sources: []SchemaSource{{Inline: schA}, {Inline: schB}}}

	// Missing Owner under Meta-Information.
	doc := "# T\n\n## Meta-Information\n\n### ID\n\nx\n"
	f := newTestFile(t, "doc.md", doc)
	diags := r.Check(f)
	expectDiagMsg(t, diags, `### Owner: got <missing>`)

	// Both children present.
	doc = "# T\n\n## Meta-Information\n\n### ID\n\nx\n\n### Owner\n\ny\n"
	f = newTestFile(t, "doc.md", doc)
	diags = r.Check(f)
	assert.Empty(t, diags)
}

func mustInline(t *testing.T, m map[string]any) *schema.Schema {
	t.Helper()
	sch, err := schema.ParseInline(m, "test")
	require.NoError(t, err)
	return sch
}

// TestApplySettings_InlineSchemaEmptyMap covers the empty-map
// short-circuit in applyInlineSchemaSetting. An empty
// inline-schema is the merge-clear-then-empty state; it must not
// install a SchemaSource and must not error.
func TestApplySettings_InlineSchemaEmptyMap(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"inline-schema": map[string]any{},
	}))
	assert.Nil(t, r.InlineSchema)
	assert.Empty(t, r.Sources)
}

// TestParseSchemaSources_EntryNotMap covers the non-map entry
// rejection in parseSchemaSources.
func TestParseSchemaSources_EntryNotMap(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"schema-sources": []any{"not-a-map"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a map")
}

// TestParseSchemaSources_FileEntryWrongType covers the non-string
// `file` value rejection.
func TestParseSchemaSources_FileEntryWrongType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"schema-sources": []any{
			map[string]any{"file": 42},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file must be a non-empty string")
}

// TestParseSchemaSources_FileEntryEmpty covers the empty-string
// `file` value rejection.
func TestParseSchemaSources_FileEntryEmpty(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"schema-sources": []any{
			map[string]any{"file": ""},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file must be a non-empty string")
}

// TestParseSchemaSources_InlineEntryWrongType covers the non-map
// `inline` value rejection.
func TestParseSchemaSources_InlineEntryWrongType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"schema-sources": []any{
			map[string]any{"inline": "not-a-map"},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inline must be a non-empty mapping")
}

// TestParseSchemaSources_InlineEntryEmpty covers the empty-map
// `inline` rejection.
func TestParseSchemaSources_InlineEntryEmpty(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"schema-sources": []any{
			map[string]any{"inline": map[string]any{}},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inline must be a non-empty mapping")
}

// TestParseSchemaSources_InlineParseError covers the
// schema.ParseInline error path: an inline-schema with an
// unsupported key surfaces as a wrapped error.
func TestParseSchemaSources_InlineParseError(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"schema-sources": []any{
			map[string]any{"inline": map[string]any{
				"bogus-key": "value",
			}},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown schema key")
}

// TestSettingMergeMode_SchemaSources covers the new MergeAppend
// declaration for the `schema-sources` setting.
func TestSettingMergeMode_SchemaSources(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, rule.MergeAppend,
		r.SettingMergeMode("schema-sources"))
}

// TestIsAnySchemaFile_InlineSourceSkipped covers the
// `src.File == ""` continue branch in isAnySchemaFile: an inline
// source has no file path to match, so the loop moves on without
// claiming the file as a schema.
func TestIsAnySchemaFile_InlineSourceSkipped(t *testing.T) {
	r := &Rule{Sources: []SchemaSource{
		{Inline: mustInline(t, map[string]any{
			"sections": []any{map[string]any{"heading": "X"}},
		})},
	}}
	f := newTestFile(t, "doc.md", "# T\n")
	assert.False(t, r.isAnySchemaFile(f),
		"inline-only sources cannot make the doc its own schema")
}

// TestCheckComposedSources_EmptyInlineSourceSkipped covers the
// `src.Inline.IsEmpty()` continue branch in checkComposedSources:
// an inline source set to an empty schema should not contribute a
// composed entry. The companion non-empty entry still validates.
func TestCheckComposedSources_EmptyInlineSourceSkipped(t *testing.T) {
	r := &Rule{Sources: []SchemaSource{
		{Inline: &schema.Schema{}}, // empty
		{Inline: mustInline(t, map[string]any{
			"sections": []any{map[string]any{
				"heading": "Goal",
			}},
		})},
	}}
	f := newTestFile(t, "doc.md", "# T\n\n## NotGoal\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, `## Goal: got <missing>`)
}

// TestCheckComposedSources_EmptyFileSourceSkipped covers the
// `src.File == ""` branch when iterating sources: a zero-value
// file source is silently skipped.
func TestCheckComposedSources_EmptyFileSourceSkipped(t *testing.T) {
	r := &Rule{Sources: []SchemaSource{
		{File: ""},
		{Inline: mustInline(t, map[string]any{
			"sections": []any{map[string]any{
				"heading": "Goal",
			}},
		})},
	}}
	f := newTestFile(t, "doc.md", "# T\n\n## NotGoal\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, `## Goal: got <missing>`)
}

// TestCheckComposedSources_AllEmptySchemas covers the
// `len(parsed) == 0` early-return branch in checkComposedSources:
// every source is empty / self-referential, so the composed schema
// is nothing and no validation diagnostics fire.
func TestCheckComposedSources_AllEmptySchemas(t *testing.T) {
	r := &Rule{Sources: []SchemaSource{
		{Inline: &schema.Schema{}},
		{Inline: &schema.Schema{}},
	}}
	f := newTestFile(t, "doc.md", "# T\n")
	diags := r.Check(f)
	assert.Empty(t, diags,
		"all-empty composed schemas must produce no diagnostics")
}

// TestCheckComposedSources_ComposeError covers the
// schema.Compose error branch: two inline schemas with conflicting
// filename patterns surface as a "composing schemas" diagnostic.
func TestCheckComposedSources_ComposeError(t *testing.T) {
	a := mustInline(t, map[string]any{"filename": "AAA*.md"})
	b := mustInline(t, map[string]any{"filename": "BBB*.md"})
	r := &Rule{Sources: []SchemaSource{{Inline: a}, {Inline: b}}}
	f := newTestFile(t, "doc.md", "# T\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, "composing schemas")
}

// TestBodySyncDiagnostics_LoadError covers the readSchemaFile
// error branch in bodySyncDiagnostics: a non-existent schema file
// surfaces as a "cannot read schema" diagnostic when the doc has
// frontmatter (otherwise the function short-circuits earlier).
func TestBodySyncDiagnostics_LoadError(t *testing.T) {
	dir := t.TempDir()
	doc := "---\nid: X\nname: y\n---\n# X: y\n"
	docPath := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(docPath, []byte(doc), 0o644))

	r := &Rule{Sources: []SchemaSource{
		{File: "missing.md"},
		// Second source so the rule takes the multi-source path
		// and exercises bodySyncDiagnostics for the first source.
		{Inline: mustInline(t, map[string]any{
			"sections": []any{map[string]any{"heading": "Goal"}},
		})},
	}}
	f, err := lint.NewFileFromSource(docPath, []byte(doc), true)
	require.NoError(t, err)
	f.SetRootDir(dir)
	diags := r.Check(f)
	expectDiagMsg(t, diags, "cannot read schema")
}

// TestBodySyncDiagnostics_ParseError covers the parseSchema error
// branch: a schema with invalid CUE frontmatter surfaces from the
// compose path as a load error, while bodySyncDiagnostics silently
// drops its own duplicated diagnostic.
func TestBodySyncDiagnostics_ParseError(t *testing.T) {
	dir := t.TempDir()
	// Schema with invalid CUE in frontmatter — parseSchema rejects
	// it, so bodySyncDiagnostics swallows the duplicate while
	// parseFileSchemaForCompose also rejects (one diag total).
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.md"),
		[]byte("---\nname: 'not (closed string\n---\n# ?\n"), 0o644))

	doc := "---\nid: X\nname: y\n---\n# T\n"
	docPath := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(docPath, []byte(doc), 0o644))

	r := &Rule{Sources: []SchemaSource{
		{File: "bad.md"},
		{Inline: mustInline(t, map[string]any{
			"sections": []any{map[string]any{"heading": "Goal"}},
		})},
	}}
	f, err := lint.NewFileFromSource(docPath, []byte(doc), true)
	require.NoError(t, err)
	f.SetRootDir(dir)
	diags := r.Check(f)
	// The compose-path reports the parse failure; the
	// body-sync path swallows it (the swallowed branch is what
	// this test covers).
	msgs := make([]string, 0, len(diags))
	for _, d := range diags {
		msgs = append(msgs, d.Message)
	}
	assert.NotEmpty(t, msgs)
}

// TestCheck_ComposedFileSchemas covers the directive-rule-readme +
// rule-readme shape: two file-based proto.md schemas resolve through
// the Sources list, get parsed via schema.ParseFile, and compose at
// check time. Without coverage of this path, the file-source half of
// the merge layer's translation goes unexercised at the rule level.
func TestCheck_ComposedFileSchemas(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "proto-a.md"),
		[]byte("# {id}: {name}\n\n## Config\n\n## ...\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "proto-b.md"),
		[]byte("# {id}: {name}\n\n## Pattern\n"), 0o644))

	doc := "---\nid: X\nname: y\n---\n# X: y\n\n## Config\n\nx\n"
	docPath := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(docPath, []byte(doc), 0o644))

	r := &Rule{Sources: []SchemaSource{
		{File: "proto-a.md"},
		{File: "proto-b.md"},
	}}
	f, err := lint.NewFileFromSource(docPath, []byte(doc), true)
	require.NoError(t, err)
	f.SetRootDir(dir)
	diags := r.Check(f)
	// proto-b requires `## Pattern`, which the doc is missing —
	// composition must surface that diagnostic.
	expectDiagMsg(t, diags, `## Pattern: got <missing>`)
}

// TestCheck_ComposedFileSchemas_BodySyncFires regresses the
// per-source body-sync path. Each file source still runs its legacy
// heading- and body-sync check independently so proto.md
// `# {id}: {name}` + Meta-Information body lines still fire on
// composed configs.
func TestCheck_ComposedFileSchemas_BodySyncFires(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "proto-a.md"),
		[]byte("# {id}: {name}\n\n## Config\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "proto-b.md"),
		[]byte("# {id}: {name}\n\n## Pattern\n"), 0o644))

	// The doc's H1 has the right shape (matches the regex pattern
	// for `{id}: {name}`) but the values don't match the
	// frontmatter — legacy body-sync should flag the mismatch
	// once the H1 has been claimed by the composed schema.
	doc := "---\nid: X\nname: y\n---\n# wrong: title\n\n" +
		"## Config\n\nx\n\n## Pattern\n\np\n"
	docPath := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(docPath, []byte(doc), 0o644))

	r := &Rule{Sources: []SchemaSource{
		{File: "proto-a.md"},
		{File: "proto-b.md"},
	}}
	f, err := lint.NewFileFromSource(docPath, []byte(doc), true)
	require.NoError(t, err)
	f.SetRootDir(dir)
	diags := r.Check(f)
	expectDiagMsg(t, diags, "heading does not match frontmatter")
}

// TestCheck_ComposedFileSchemas_SchemaFileSelfSkipped verifies the
// self-validation guard: when the file being linted is itself one
// of the configured file sources, that source contributes no
// composed entry (so the schema doesn't validate against itself).
func TestCheck_ComposedFileSchemas_SchemaFileSelfSkipped(t *testing.T) {
	dir := t.TempDir()
	// H2-rooted schemas (no H1 wrapper) so both sources agree on
	// RootLevel and the validator doesn't get stuck on an
	// unmatched H1 before reaching the section under test.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "proto-a.md"),
		[]byte("## Config\n"), 0o644))
	// The file being linted IS proto-b.md.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "proto-b.md"),
		[]byte("## Pattern\n"), 0o644))

	docPath := filepath.Join(dir, "proto-b.md")
	src, err := os.ReadFile(docPath)
	require.NoError(t, err)

	r := &Rule{Sources: []SchemaSource{
		{File: "proto-a.md"},
		{File: "proto-b.md"}, // self-reference; should be skipped
	}}
	f, err := lint.NewFileFromSource(docPath, src, true)
	require.NoError(t, err)
	f.SetRootDir(dir)
	diags := r.Check(f)
	// proto-b.md is skipped (self-validation). proto-a requires
	// Config and the doc (proto-b's body) has only `## Pattern`,
	// so the missing-Config diagnostic must surface — proving
	// proto-a still validated while proto-b did not validate
	// against itself.
	expectDiagMsg(t, diags, `## Config: got <missing>`)
}

// TestComposedSchemaForFix_FileSource exercises the file-source
// branch of composedSchemaForFix. Multi-source configs that mix
// inline and file sources must produce a composed schema that
// carries the inline source's Index block. Both sources must
// agree on RootLevel — the file uses an H2-rooted proto.md
// (no `# ?` H1 wrapper) so it matches the inline default of 2.
func TestComposedSchemaForFix_FileSource(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "proto.md"),
		[]byte("## Goal\n"), 0o644))

	doc := "# T\n\n## Goal\n\nx\n"
	docPath := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(docPath, []byte(doc), 0o644))

	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"schema-sources": []any{
			map[string]any{"file": "proto.md"},
			map[string]any{"inline": map[string]any{
				"index": map[string]any{
					"output":  "out.json",
					"include": []any{"headings"},
				},
			}},
		},
	}))
	f, err := lint.NewFileFromSource(docPath, []byte(doc), true)
	require.NoError(t, err)
	f.SetRootDir(dir)
	r.Fix(f)
	_, err = os.Stat(filepath.Join(dir, "out.json"))
	require.NoError(t, err, "composed Index from inline source must write")
}

// TestComposedSchemaForFix_NoSources covers the no-source short
// circuit in composedSchemaForFix.
func TestComposedSchemaForFix_NoSources(t *testing.T) {
	r := &Rule{}
	f := newTestFile(t, "doc.md", "# T\n")
	sch, err := r.composedSchemaForFix(f)
	require.NoError(t, err)
	assert.Nil(t, sch, "no sources should return nil")
}

// TestComposedSchemaForFix_AllEmpty covers the empty-input branch:
// every inline source is empty, file source is empty string.
func TestComposedSchemaForFix_AllEmpty(t *testing.T) {
	r := &Rule{Sources: []SchemaSource{
		{Inline: &schema.Schema{}},
		{File: ""},
	}}
	f := newTestFile(t, "doc.md", "# T\n")
	sch, err := r.composedSchemaForFix(f)
	require.NoError(t, err)
	assert.Nil(t, sch,
		"empty-only sources should produce no composed schema")
}

// TestComposedSchemaForFix_SelfReferentialFile covers the
// self-validation skip path in composedSchemaForFix: when a file
// source points at the file currently being fixed, the source is
// skipped (parseFileSchemaForCompose returns nil).
func TestComposedSchemaForFix_SelfReferentialFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "proto.md"),
		[]byte("# ?\n\n## Goal\n"), 0o644))
	f, err := lint.NewFileFromSource(filepath.Join(dir, "proto.md"),
		[]byte("# ?\n\n## Goal\n"), true)
	require.NoError(t, err)
	f.SetRootDir(dir)

	r := &Rule{Sources: []SchemaSource{{File: "proto.md"}}}
	sch, err := r.composedSchemaForFix(f)
	require.NoError(t, err)
	assert.Nil(t, sch,
		"a single self-referential file source should produce no schema")
}

// TestReflectSingleSource_NoSources covers the early-return when
// Sources is empty (the default path that does nothing).
func TestReflectSingleSource_NoSources(t *testing.T) {
	r := &Rule{Schema: "original.md"}
	r.reflectSingleSource()
	assert.Equal(t, "original.md", r.Schema,
		"empty Sources must not touch Schema")
}

// TestReflectSingleSource_MultipleSources verifies the no-op branch
// when more than one source is configured: Schema and InlineSchema
// stay at their previous values rather than collapsing to the
// first entry.
func TestReflectSingleSource_MultipleSources(t *testing.T) {
	r := &Rule{
		Schema:       "kept",
		InlineSchema: mustInline(t, map[string]any{"sections": []any{}}),
		Sources: []SchemaSource{
			{File: "a.md"},
			{File: "b.md"},
		},
	}
	prevInline := r.InlineSchema
	r.reflectSingleSource()
	assert.Equal(t, "kept", r.Schema)
	assert.Same(t, prevInline, r.InlineSchema)
}

// TestEffectiveSources_EmptyInlineSchemaIgnored covers the
// `r.InlineSchema.IsEmpty() == true` branch in effectiveSources:
// an InlineSchema that's set but empty falls through to the file
// path, then to nil.
func TestEffectiveSources_EmptyInlineSchemaIgnored(t *testing.T) {
	r := &Rule{InlineSchema: &schema.Schema{}}
	assert.Empty(t, r.effectiveSources(),
		"empty InlineSchema must not produce a source")
}

// TestFix_NoOpOnErrorOrNil exercises the three "do nothing" exits
// of Fix: an error from composedSchemaForFix, a nil composed
// schema (no sources), and a composed schema with no Index.
func TestFix_NoOpOnErrorOrNil(t *testing.T) {
	doc := []byte("# T\n")
	// (a) error path: a non-existent file source surfaces an error.
	t.Run("error_from_compose", func(t *testing.T) {
		dir := t.TempDir()
		f, err := lint.NewFileFromSource(filepath.Join(dir, "doc.md"), doc, true)
		require.NoError(t, err)
		f.SetRootDir(dir)
		r := &Rule{Sources: []SchemaSource{
			{File: "missing.md"},
			{Inline: mustInline(t, map[string]any{
				"sections": []any{map[string]any{"heading": "Goal"}},
			})},
		}}
		out := r.Fix(f)
		assert.Equal(t, doc, out)
	})
	// (b) nil schema path: no sources at all.
	t.Run("nil_schema", func(t *testing.T) {
		f, err := lint.NewFileFromSource("doc.md", doc, true)
		require.NoError(t, err)
		r := &Rule{}
		out := r.Fix(f)
		assert.Equal(t, doc, out)
	})
	// (c) no Index: composed schema has no Index block.
	t.Run("no_index_in_composed", func(t *testing.T) {
		f, err := lint.NewFileFromSource("doc.md", doc, true)
		require.NoError(t, err)
		r := &Rule{Sources: []SchemaSource{
			{Inline: mustInline(t, map[string]any{
				"sections": []any{map[string]any{"heading": "Goal"}},
			})},
			{Inline: mustInline(t, map[string]any{
				"sections": []any{map[string]any{"heading": "Risks"}},
			})},
		}}
		out := r.Fix(f)
		assert.Equal(t, doc, out)
	})
	// (d) empty composed schema: parses to nothing.
	t.Run("empty_composed", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "empty.md"), []byte(""), 0o644))
		f, err := lint.NewFileFromSource(filepath.Join(dir, "doc.md"), doc, true)
		require.NoError(t, err)
		f.SetRootDir(dir)
		r := &Rule{Sources: []SchemaSource{{File: "empty.md"}}}
		out := r.Fix(f)
		assert.Equal(t, doc, out)
	})
}

// TestApplySettings_RejectsBothNonEmpty covers the alternate
// arm of the dual-source rejection: path is set and inline is
// EMPTY map. That should NOT error (it's the merge-clear-then-
// reapply state), exercising the false branch of
// `len(inline) == 0` short-circuit.
func TestApplySettings_PathPlusEmptyInlineAllowed(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"schema":        "schemas/x.md",
		"inline-schema": map[string]any{},
	}))
	assert.Equal(t, "schemas/x.md", r.Schema)
}

// TestReflectSingleSource_ZeroValueSource covers the case where
// Sources has exactly one zero-value entry — both switch arms
// evaluate to false, the function falls through, and Schema /
// InlineSchema stay at their prior values.
func TestReflectSingleSource_ZeroValueSource(t *testing.T) {
	r := &Rule{
		Schema:       "kept",
		InlineSchema: mustInline(t, map[string]any{"sections": []any{}}),
		Sources:      []SchemaSource{{}}, // zero-value
	}
	prevInline := r.InlineSchema
	r.reflectSingleSource()
	assert.Equal(t, "kept", r.Schema,
		"zero-value source must not overwrite Schema")
	assert.Same(t, prevInline, r.InlineSchema,
		"zero-value source must not overwrite InlineSchema")
}

// TestReflectSingleSource_SingleFileSourceClearsInline covers the
// `r.Sources[0].Inline != nil` false branch in the reflect
// switch — when the single source is a FILE, the Inline arm is
// not chosen.
func TestReflectSingleSource_SingleFileSourceClearsInline(t *testing.T) {
	r := &Rule{
		InlineSchema: mustInline(t, map[string]any{"sections": []any{}}),
		Sources:      []SchemaSource{{File: "x.md"}},
	}
	r.reflectSingleSource()
	assert.Equal(t, "x.md", r.Schema)
	assert.Nil(t, r.InlineSchema,
		"single file source must clear InlineSchema")
}

// TestCheck_ZeroValueSingleSourceShortCircuits covers the
// fall-through case in Check where Sources has exactly one entry
// but both File and Inline are zero-valued (a defensive guard).
func TestCheck_ZeroValueSingleSourceShortCircuits(t *testing.T) {
	r := &Rule{Sources: []SchemaSource{{}}}
	f := newTestFile(t, "doc.md", "# T\n")
	diags := r.Check(f)
	assert.Empty(t, diags,
		"a zero-value single source must produce no diagnostics")
}

// TestIsSchemaFileAt_EmptyPath covers the empty-path early-return
// in isSchemaFileAt.
func TestIsSchemaFileAt_EmptyPath(t *testing.T) {
	r := &Rule{}
	f := newTestFile(t, "doc.md", "# T\n")
	assert.False(t, r.isSchemaFileAt(f, ""))
}

// TestComposedSchemaForFix_ParseFileError covers the error
// propagation from parseFileSchemaForCompose during Fix: a schema
// file that fails to parse surfaces as an error from
// composedSchemaForFix. The Fix implementation swallows it, so we
// call the helper directly to confirm the error path.
func TestComposedSchemaForFix_ParseFileError(t *testing.T) {
	dir := t.TempDir()
	// Frontmatter with an unknown shortcut name rejects in
	// schema.ParseFile via frontmatterExpr → resolveBareName.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.md"),
		[]byte("---\nname: notashortcut\n---\n# ?\n"), 0o644))

	doc := "# T\n"
	docPath := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(docPath, []byte(doc), 0o644))

	r := &Rule{Sources: []SchemaSource{
		{File: "bad.md"},
		{Inline: mustInline(t, map[string]any{
			"sections": []any{map[string]any{"heading": "Goal"}},
		})},
	}}
	f, err := lint.NewFileFromSource(docPath, []byte(doc), true)
	require.NoError(t, err)
	f.SetRootDir(dir)
	_, err = r.composedSchemaForFix(f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot load schema")
}

// TestCheckComposedSources_EmptyComposedSchemaSkipsValidate covers
// the `composed == nil || composed.IsEmpty()` early-return: when
// every file source parses to an empty schema (e.g. proto files
// with no frontmatter or sections), the composed schema is empty
// and validation is skipped.
func TestCheckComposedSources_EmptyComposedSchemaSkipsValidate(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "empty-a.md"),
		[]byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "empty-b.md"),
		[]byte(""), 0o644))

	doc := "# T\n"
	docPath := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(docPath, []byte(doc), 0o644))

	r := &Rule{Sources: []SchemaSource{
		{File: "empty-a.md"},
		{File: "empty-b.md"},
	}}
	f, err := lint.NewFileFromSource(docPath, []byte(doc), true)
	require.NoError(t, err)
	f.SetRootDir(dir)
	diags := r.Check(f)
	assert.Empty(t, diags,
		"empty composed schema must skip validation")
}

// TestParseFileSchemaForCompose_LoadError covers the schema.ParseFile
// error branch in composedSchemaForFix / checkComposedSources.
func TestParseFileSchemaForCompose_LoadError(t *testing.T) {
	dir := t.TempDir()
	doc := "# T\n"
	docPath := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(docPath, []byte(doc), 0o644))

	r := &Rule{Sources: []SchemaSource{
		{File: "missing-proto.md"},
		{Inline: mustInline(t, map[string]any{
			"sections": []any{map[string]any{"heading": "Goal"}},
		})},
	}}
	f, err := lint.NewFileFromSource(docPath, []byte(doc), true)
	require.NoError(t, err)
	f.SetRootDir(dir)
	diags := r.Check(f)
	expectDiagMsg(t, diags, "cannot load schema")
}
