package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseInline_BindUnsetVsExplicit pins the *string round-trip:
// a scope with no `bind:` key parses to Bind == nil, an explicit
// empty bind parses to Bind != nil && *Bind == "", and a non-empty
// bind preserves the user's value. Plan 167.
func TestParseInline_BindUnsetVsExplicit(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": "Default"},
			map[string]any{"heading": "Renamed", "bind": "out"},
			map[string]any{"heading": "Hoisted", "bind": ""},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	require.Len(t, sch.Sections, 3)

	assert.Nil(t, sch.Sections[0].Bind, "unset bind must round-trip to nil")

	require.NotNil(t, sch.Sections[1].Bind)
	assert.Equal(t, "out", *sch.Sections[1].Bind)

	require.NotNil(t, sch.Sections[2].Bind,
		"explicit empty bind must round-trip to a non-nil pointer")
	assert.Equal(t, "", *sch.Sections[2].Bind)
}

func TestParseInline_BindWrongType(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Goal",
			"bind":    42,
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bind must be a string")
}

// `bind:` makes no sense on the preamble: the preamble already
// hoists, and renaming the no-key projection would be misleading.
func TestParseInline_BindRejectedOnPreamble(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": nil,
			"bind":    "intro",
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "preamble")
	assert.Contains(t, err.Error(), "bind")
}

// A slot scope is skipped by the projector, so a bind would be
// unreachable. Reject it at parse time so the override surfaces as
// a config error rather than a silent no-op.
func TestParseInline_BindRejectedOnSlot(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": map[string]any{
				"regex":  ".+",
				"repeat": map[string]any{"min": 0},
			},
			"bind": "anywhere",
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "slot")
	assert.Contains(t, err.Error(), "bind")
}

// A broad matcher (`.+` with a non-zero repeat min) is also skipped
// by the projector. The diagnostic explains why the bind would be
// unreachable.
func TestParseInline_BindRejectedOnBroadMatcher(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": map[string]any{"regex": ".+"},
			"bind":    "anywhere",
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broad-match")
	assert.Contains(t, err.Error(), "unreachable")
}

// Two sibling scopes binding to the same key would collide on the
// projection. Reject at parse time.
func TestParseInline_DuplicateSiblingBindsRejected(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{
			map[string]any{"heading": "Goal", "bind": "result"},
			map[string]any{"heading": "Risks", "bind": "result"},
		},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicates")
	assert.Contains(t, err.Error(), "result")
}

// Two siblings both hoisting (`bind: ""`) are allowed: neither
// produces a key, so they cannot collide on one. (Whether the
// projection itself collides is a separate runtime check.)
func TestParseInline_DuplicateEmptyBindsAllowed(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{
			map[string]any{"heading": "A", "bind": ""},
			map[string]any{"heading": "B", "bind": ""},
		},
	}, "kind x")
	require.NoError(t, err)
}

// Content entries pick up the same `bind:` key. A non-empty value
// renames the default key (code/items/rows/text).
func TestParseInline_ContentBindRenames(t *testing.T) {
	raw := map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{
				map[string]any{"kind": "code-block", "bind": "sample"},
				map[string]any{"kind": "list", "bind": "steps"},
				map[string]any{"kind": "table", "bind": "settings"},
				map[string]any{"kind": "paragraph", "bind": "summary"},
			},
		}},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	entries := sch.Sections[0].Content
	require.Len(t, entries, 4)
	for i, want := range []string{"sample", "steps", "settings", "summary"} {
		require.NotNil(t, entries[i].Bind,
			"entry %d should have a non-nil Bind", i)
		assert.Equal(t, want, *entries[i].Bind)
	}
}

// An empty bind on a content entry has no clean semantics — a
// content entry has no children to hoist into the parent. Reject
// it at parse time so authors restructure the schema instead.
func TestParseInline_ContentBindEmptyRejected(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{
				map[string]any{"kind": "code-block", "bind": ""},
			},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty string is not allowed on a content entry")
}

// `bind:` on a `kind: unlisted` slot is unreachable — the slot
// never projects a key to rename. Reject at parse time.
func TestParseInline_ContentBindOnUnlistedRejected(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{
				map[string]any{"kind": "unlisted", "bind": "anywhere"},
			},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unlisted")
	assert.Contains(t, err.Error(), "bind")
}

func TestParseInline_ContentBindWrongType(t *testing.T) {
	_, err := ParseInline(map[string]any{
		"sections": []any{map[string]any{
			"heading": "Examples",
			"content": []any{
				map[string]any{"kind": "code-block", "bind": 42},
			},
		}},
	}, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bind must be a string")
}
