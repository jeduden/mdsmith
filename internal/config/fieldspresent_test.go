package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Files satisfying every listed field with a non-null value get the
// kind. Files missing any field do not.
func TestFieldsPresent_AllFieldsRequired(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{"task": {}},
		KindAssignment: []KindAssignmentEntry{
			{FieldsPresent: []string{"status", "priority", "assignee"}, Kinds: []string{"task"}},
		},
	}

	full := map[string]any{
		"status":   "open",
		"priority": "high",
		"assignee": "alice",
	}
	got := resolveEffectiveKinds(cfg, "anywhere/doc.md", nil, full)
	assert.Equal(t, []string{"task"}, got, "all three fields present → kind matches")

	missing := map[string]any{
		"status":   "open",
		"priority": "high",
	}
	got = resolveEffectiveKinds(cfg, "anywhere/doc.md", nil, missing)
	assert.Empty(t, got, "missing one field → no match")
}

// A field present with a null value does not count as present. The user
// wrote the key but did not fill it in.
func TestFieldsPresent_NullValueDoesNotCount(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{"task": {}},
		KindAssignment: []KindAssignmentEntry{
			{FieldsPresent: []string{"status"}, Kinds: []string{"task"}},
		},
	}

	// `status: null` in YAML decodes to a nil value.
	withNull := map[string]any{"status": nil}
	got := resolveEffectiveKinds(cfg, "doc.md", nil, withNull)
	assert.Empty(t, got, "null value should not count as present")

	withValue := map[string]any{"status": "open"}
	got = resolveEffectiveKinds(cfg, "doc.md", nil, withValue)
	assert.Equal(t, []string{"task"}, got)
}

// An entry combining glob and fields-present matches only files
// satisfying both selectors.
func TestFieldsPresent_AndedWithGlob(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{"plan": {}},
		KindAssignment: []KindAssignmentEntry{
			{
				Glob:          []string{"plan/*.md"},
				FieldsPresent: []string{"id"},
				Kinds:         []string{"plan"},
			},
		},
	}

	withID := map[string]any{"id": 132}

	got := resolveEffectiveKinds(cfg, "plan/132_inline.md", nil, withID)
	assert.Equal(t, []string{"plan"}, got, "both selectors satisfied")

	got = resolveEffectiveKinds(cfg, "docs/api.md", nil, withID)
	assert.Empty(t, got, "glob fails → AND fails")

	got = resolveEffectiveKinds(cfg, "plan/132_inline.md", nil, nil)
	assert.Empty(t, got, "no FM fields → fields-present fails → AND fails")
}

// Entries with no fields-present keep behaving exactly as before:
// glob-only matching.
func TestFieldsPresent_GlobOnlyEntryUnchanged(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{"doc": {}},
		KindAssignment: []KindAssignmentEntry{
			{Glob: []string{"docs/**"}, Kinds: []string{"doc"}},
		},
	}

	got := resolveEffectiveKinds(cfg, "docs/api.md", nil, map[string]any{"unrelated": 1})
	assert.Equal(t, []string{"doc"}, got)

	got = resolveEffectiveKinds(cfg, "plan/132.md", nil, nil)
	assert.Empty(t, got)
}

// Provenance surfaces the matching entry index and selector via the
// new ResolvedKind.Selector field. Includes an entry that does not
// match so the resolver's skip branch runs alongside the matching
// path.
func TestFieldsPresent_ProvenanceCarriesSelector(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{"task": {}, "plan": {}, "draft": {}},
		KindAssignment: []KindAssignmentEntry{
			{FieldsPresent: []string{"status", "assignee"}, Kinds: []string{"task"}},
			// Second entry asks for a field the file doesn't carry —
			// the resolver must skip it without contributing a kind.
			{FieldsPresent: []string{"draft"}, Kinds: []string{"draft"}},
			{Glob: []string{"plan/*.md"}, FieldsPresent: []string{"id"}, Kinds: []string{"plan"}},
		},
	}

	res := ResolveFile(cfg, "plan/9.md", nil, map[string]any{
		"id":       9,
		"status":   "open",
		"assignee": "alice",
	})
	require.Len(t, res.Kinds, 2, "non-matching entry should not contribute a kind")

	// First entry: fields-present only.
	assert.Equal(t, "task", res.Kinds[0].Name)
	assert.Equal(t, KindAssignmentSource("kind-assignment[0]"), res.Kinds[0].Source)
	assert.Equal(t, "fields-present status,assignee", res.Kinds[0].Selector)

	// Third entry matched; second was skipped so the index jumps to 2.
	assert.Equal(t, "plan", res.Kinds[1].Name)
	assert.Equal(t, KindAssignmentSource("kind-assignment[2]"), res.Kinds[1].Source)
	assert.Equal(t, "glob plan/*.md AND fields-present id", res.Kinds[1].Selector)
}

// An entry with neither selector set never matches — an unconditional
// kind assignment is not supported by design.
func TestFieldsPresent_EmptyEntryNeverMatches(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{"task": {}},
		KindAssignment: []KindAssignmentEntry{
			{Kinds: []string{"task"}},
		},
	}

	got := resolveEffectiveKinds(cfg, "doc.md", nil, map[string]any{"status": "open"})
	assert.Empty(t, got)
}

// HasFieldsPresentSelector lets callers (engine, fix, lsp) skip the
// extra FM-mapping decode when no entry uses the selector.
func TestHasFieldsPresentSelector(t *testing.T) {
	assert.False(t, HasFieldsPresentSelector(nil), "nil config")

	assert.False(t, HasFieldsPresentSelector(&Config{
		KindAssignment: []KindAssignmentEntry{
			{Glob: []string{"plan/*.md"}, Kinds: []string{"plan"}},
		},
	}), "glob-only entries")

	assert.True(t, HasFieldsPresentSelector(&Config{
		KindAssignment: []KindAssignmentEntry{
			{Glob: []string{"plan/*.md"}, Kinds: []string{"plan"}},
			{FieldsPresent: []string{"status"}, Kinds: []string{"task"}},
		},
	}), "any entry with fields-present")
}

// NeedsFieldsForFile gates the FM-mapping decode per file path: it only
// returns true when a fields-present entry could actually match this
// file — an entry without a glob always could, an entry with a glob
// only if the path matches it.
func TestNeedsFieldsForFile(t *testing.T) {
	assert.False(t, NeedsFieldsForFile(nil, "any.md"), "nil config")

	assert.False(t, NeedsFieldsForFile(&Config{
		KindAssignment: []KindAssignmentEntry{
			{Glob: []string{"docs/*.md"}, Kinds: []string{"doc"}},
		},
	}, "docs/a.md"), "no fields-present entry → false even when glob matches")

	cfg := &Config{
		KindAssignment: []KindAssignmentEntry{
			{
				Glob:          []string{"plan/*.md"},
				FieldsPresent: []string{"id"},
				Kinds:         []string{"plan"},
			},
		},
	}
	assert.True(t, NeedsFieldsForFile(cfg, "plan/9.md"),
		"glob+fields entry matches path → needs fields")
	assert.False(t, NeedsFieldsForFile(cfg, "docs/api.md"),
		"glob+fields entry does not match path → no need to parse fields")

	cfgUnscoped := &Config{
		KindAssignment: []KindAssignmentEntry{
			{FieldsPresent: []string{"status"}, Kinds: []string{"task"}},
		},
	}
	assert.True(t, NeedsFieldsForFile(cfgUnscoped, "anywhere/x.md"),
		"fields-only entry has no glob, so every path needs the parse")
}
