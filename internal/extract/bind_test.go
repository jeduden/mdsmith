package extract

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// strPtr returns a pointer to s — used to set the Bind *string field
// from test literals.
func strPtr(s string) *string { return &s }

// boundScope builds a literal-heading scope with an explicit bind
// override. Mirrors the shape an inline schema would parse into.
func boundScope(heading, bind string) schema.Scope {
	sc := litScope(heading)
	sc.Bind = strPtr(bind)
	return sc
}

// TestExtract_BindRenamesKey overrides the default slug with the
// `bind:` value; the projected key is the user's name rather than
// the slugified heading.
func TestExtract_BindRenamesKey(t *testing.T) {
	sch := &schema.Schema{
		RootLevel: 2,
		Sections:  []schema.Scope{boundScope("Goal", "objective")},
	}
	got, diags := run(t, "## Goal\n\nbody\n", sch, nil)
	require.Empty(t, diags)
	root := got.(map[string]any)
	assert.Contains(t, root, "objective")
	assert.NotContains(t, root, "goal")
}

// TestExtract_BindEmptyHoistsChildren ensures `bind: ""` lifts the
// scope's child sections and content into the parent without a
// wrapper key.
func TestExtract_BindEmptyHoistsChildren(t *testing.T) {
	inner := litScope("First")
	outer := litScope("Steps")
	outer.Sections = []schema.Scope{inner}
	outer.Bind = strPtr("") // hoist
	sch := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{outer}}
	got, diags := run(t, "## Steps\n\n### First\n\nx\n", sch, nil)
	require.Empty(t, diags)
	root := got.(map[string]any)
	// `steps` is gone; `first` lives directly on the root.
	assert.NotContains(t, root, "steps")
	assert.Contains(t, root, "first")
}

// TestExtract_BindEmptyHoistsContent confirms the hoist also lifts
// the scope's content entries (paragraph text in this case).
func TestExtract_BindEmptyHoistsContent(t *testing.T) {
	sc := schema.Scope{
		Heading: "Notes",
		Matcher: &schema.Matcher{Regex: "Notes"},
		Bind:    strPtr(""),
		Content: []schema.ContentEntry{
			{Kind: schema.ContentKindParagraph, Required: true},
		},
	}
	sch := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{sc}}
	got, diags := run(t, "## Notes\n\na note\n", sch, nil)
	require.Empty(t, diags)
	root := got.(map[string]any)
	assert.NotContains(t, root, "notes")
	assert.Equal(t, "a note", root["text"])
}

// TestExtract_BindHoistCollisionFlagged surfaces a hoist that would
// silently overwrite a sibling key — the projector reports it as
// a sibling-key collision so the user can adjust the schema.
func TestExtract_BindHoistCollisionFlagged(t *testing.T) {
	inner := litScope("Goal")
	outer := litScope("Wrapper")
	outer.Sections = []schema.Scope{inner}
	outer.Bind = strPtr("")
	other := litScope("Goal")
	sch := &schema.Schema{
		RootLevel: 2,
		Sections:  []schema.Scope{outer, other},
	}
	_, diags := run(t,
		"## Wrapper\n\n### Goal\n\nx\n\n## Goal\n\ny\n",
		sch, nil)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message, "goal")
	// The schema reference must survive as a related location (plan
	// 230 moved it off the message; extract must Emit, not hand-build).
	require.Len(t, diags[0].RelatedLocations, 1,
		"collision diagnostic carries the schema reference")
}

// TestExtract_BindRepeatingHoistRejected covers the edge case where
// a repeating scope is marked `bind: ""`. Two occurrences would
// silently overwrite each other on hoist, so the projector flags
// the collision rather than producing lossy output.
func TestExtract_BindRepeatingHoistRejected(t *testing.T) {
	rep := schema.Scope{
		Heading: "Step {n}",
		Matcher: &schema.Matcher{
			Regex:  `Step \#(digits)`,
			Repeat: schema.Repeat{Set: true, Min: 1},
		},
		Bind: strPtr(""),
	}
	sch := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{rep}}
	_, diags := run(t, "## Step 1\n\na\n\n## Step 2\n\nb\n", sch, nil)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message, "hoist")
}

// TestExtract_ContentBindRenamesKey: a content entry's bind value
// replaces the default `code` / `items` / `rows` / `text` key.
func TestExtract_ContentBindRenamesKey(t *testing.T) {
	sc := schema.Scope{
		Heading: "Examples",
		Matcher: &schema.Matcher{Regex: "Examples"},
		Content: []schema.ContentEntry{
			{Kind: schema.ContentKindParagraph, Required: true, Bind: strPtr("blurb")},
			{Kind: schema.ContentKindCodeBlock, Required: true, Bind: strPtr("snippet")},
		},
	}
	sch := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{sc}}
	body := "## Examples\n\nintro\n\n```go\nx := 1\n```\n"
	got, diags := run(t, body, sch, nil)
	require.Empty(t, diags)
	ex := got.(map[string]any)["examples"].(map[string]any)
	assert.Equal(t, "x := 1", ex["snippet"])
	assert.Equal(t, "intro", ex["blurb"])
	assert.NotContains(t, ex, "code")
	assert.NotContains(t, ex, "text")
}

// TestExtract_BindRepeatingArray confirms a repeating scope keeps
// its array projection under the bound key (not the default slug).
func TestExtract_BindRepeatingArray(t *testing.T) {
	rep := schema.Scope{
		Heading: "Step {n}",
		Matcher: &schema.Matcher{
			Regex:  `Step \#(digits)`,
			Repeat: schema.Repeat{Set: true, Min: 1},
		},
		Bind: strPtr("steps"),
	}
	sch := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{rep}}
	got, diags := run(t, "## Step 1\n\na\n\n## Step 2\n\nb\n", sch, nil)
	require.Empty(t, diags)
	root := got.(map[string]any)
	assert.Contains(t, root, "steps")
	assert.NotContains(t, root, "step")
	arr := root["steps"].([]any)
	require.Len(t, arr, 2)
}

// TestHoistsToParent_NilSafe covers the defensive nil branches:
// hoistsToParent must not panic on a nil match or a match whose
// Scope is nil (the synthetic MatchTree.Root has Scope == nil).
func TestHoistsToParent_NilSafe(t *testing.T) {
	assert.False(t, hoistsToParent(nil))
	assert.False(t, hoistsToParent(&schema.ScopeMatch{}))
}

// TestKeyFor_EmptyBindFallsBackToDefault verifies the defensive
// `*Bind == ""` guard in keyFor: even if a caller bypasses the
// hoistsToParent check, keyFor never emits an empty key.
func TestKeyFor_EmptyBindFallsBackToDefault(t *testing.T) {
	sc := &schema.Scope{
		Heading: "Goal",
		Matcher: &schema.Matcher{Regex: "Goal"},
		Bind:    strPtr(""),
	}
	assert.Equal(t, "goal", keyFor(sc))
}

// TestExtract_BindSurvivesComposition: a file resolving to two
// kinds where one sets `bind:` for a heading uses the bound key in
// the projection.
func TestExtract_BindSurvivesComposition(t *testing.T) {
	a := &schema.Schema{
		RootLevel: 2,
		Sections:  []schema.Scope{boundScope("Goal", "objective")},
	}
	b := &schema.Schema{
		RootLevel: 2,
		Sections:  []schema.Scope{litScope("Goal")},
	}
	composed, err := schema.Compose(a, b)
	require.NoError(t, err)

	f := doc(t, "## Goal\n\nbody\n")
	mt := schema.BuildMatchTree(f, composed, nil)
	got, diags := Extract(f, composed, mt)
	require.Empty(t, diags)
	root := got.(map[string]any)
	assert.Contains(t, root, "objective")
	assert.NotContains(t, root, "goal")
}
