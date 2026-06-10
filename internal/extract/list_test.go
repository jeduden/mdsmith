package extract

import (
	"encoding/json"
	"testing"

	"github.com/jeduden/mdsmith/internal/extract/encode"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"gopkg.in/yaml.v3"
)

// listScope builds a single-section schema whose body is one list
// content entry projected with the given mode (empty for the flat
// default).
func listScope(projection string) *schema.Schema {
	return &schema.Schema{
		RootLevel: 2,
		Sections: []schema.Scope{{
			Heading: "Items",
			Matcher: &schema.Matcher{Regex: "Items"},
			Content: []schema.ContentEntry{{
				Kind:       schema.ContentKindList,
				Required:   true,
				Projection: projection,
			}},
		}},
	}
}

// runList projects body through listScope(projection) and returns the
// `items` value plus any diagnostics.
func runList(t *testing.T, body, projection string) (any, []lint.Diagnostic) {
	t.Helper()
	got, diags := run(t, body, listScope(projection), nil)
	if len(diags) > 0 {
		return nil, diags
	}
	items := got.(map[string]any)["items"].(map[string]any)["items"]
	return items, nil
}

// TestExtract_FlatListNoNestedConcatenation is the plan-244 bugfix
// reproduction: a parent item's own text must not absorb a nested
// child's text. The corrupt behaviour emitted
// "open item with boldnested child"; the fix emits only the parent's
// own inline text ("open item with bold"), children excluded.
func TestExtract_FlatListNoNestedConcatenation(t *testing.T) {
	body := "## Items\n\n" +
		"- [x] done item\n" +
		"- [ ] open item with **bold**\n" +
		"  - nested child\n"
	items, diags := runList(t, body, "")
	require.Empty(t, diags)
	assert.Equal(t, []any{
		"[x] done item",
		"[ ] open item with bold",
	}, items)
}

// TestExtract_FlatListItemOnlyNestedList pins the design decision the
// plan calls out: a top-level item whose only content is a nested
// sub-list has no own text, so flat mode projects it as the empty
// string. The item keeps its slot in the array (order preserved); the
// nested child is excluded, since flat mode emits own text only.
func TestExtract_FlatListItemOnlyNestedList(t *testing.T) {
	body := "## Items\n\n" +
		"- parent\n" +
		"-\n" +
		"  - lonely child\n"
	items, diags := runList(t, body, "")
	require.Empty(t, diags)
	assert.Equal(t, []any{"parent", ""}, items)
}

// TestExtract_TreeListWorkedExample pins the plan-244 tree-mode worked
// example: each item is an object with its own `text`; a task item
// carries a `checked` bool (the `[x]`/`[ ]` marker never leaks into
// `text`); a nesting item carries recursive `children`.
func TestExtract_TreeListWorkedExample(t *testing.T) {
	body := "## Items\n\n" +
		"- [x] done item\n" +
		"- [ ] open item with **bold**\n" +
		"  - nested child\n"
	items, diags := runList(t, body, schema.ProjectionTree)
	require.Empty(t, diags)
	assert.Equal(t, []any{
		map[string]any{"text": "done item", "checked": true},
		map[string]any{
			"text":    "open item with bold",
			"checked": false,
			"children": []any{
				map[string]any{"text": "nested child"},
			},
		},
	}, items)
}

// TestExtract_TreeListPlainItem verifies a non-task, non-nesting item
// projects as just `{text}` — no `checked`, no `children` keys.
func TestExtract_TreeListPlainItem(t *testing.T) {
	items, diags := runList(t, "## Items\n\n- plain item\n", schema.ProjectionTree)
	require.Empty(t, diags)
	assert.Equal(t, []any{map[string]any{"text": "plain item"}}, items)
}

// TestExtract_TreeListTaskMarkerEdges pins task-marker detection to
// match the goldmark task-list extension byte-for-byte: a bare `[x]`
// (empty label) is a checked task with empty text; an unchecked `[ ]`
// with a label strips the marker; and a bracketed word that is not a
// valid marker (`[a]`, `[TODO]`) is left verbatim with no `checked`.
func TestExtract_TreeListTaskMarkerEdges(t *testing.T) {
	cases := []struct {
		name string
		item string
		want map[string]any
	}{
		{"bare checked", "- [x]\n", map[string]any{"text": "", "checked": true}},
		{"bare unchecked", "- [ ]\n", map[string]any{"text": "", "checked": false}},
		{"no space after marker", "- [x]done\n",
			map[string]any{"text": "done", "checked": true}},
		{"non-marker bracket word", "- [a] label\n",
			map[string]any{"text": "[a] label"}},
		{"non-marker uppercase word", "- [TODO] task\n",
			map[string]any{"text": "[TODO] task"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			items, diags := runList(t, "## Items\n\n"+c.item, schema.ProjectionTree)
			require.Empty(t, diags)
			assert.Equal(t, []any{c.want}, items)
		})
	}
}

// TestExtract_TreeListNestedTaskItems verifies `checked` is computed
// per item at any depth: a nested child task carries its own marker.
func TestExtract_TreeListNestedTaskItems(t *testing.T) {
	body := "## Items\n\n" +
		"- [ ] parent task\n" +
		"  - [x] child task\n"
	items, diags := runList(t, body, schema.ProjectionTree)
	require.Empty(t, diags)
	assert.Equal(t, []any{
		map[string]any{
			"text":    "parent task",
			"checked": false,
			"children": []any{
				map[string]any{"text": "child task", "checked": true},
			},
		},
	}, items)
}

// TestExtract_TreeListSameAcrossFormats pins the acceptance criterion
// that YAML and msgpack emit the same tree as JSON — including the
// `checked` bool and nested `children`, the two shapes new to plan
// 244. The full document tree is encoded in each format and decoded
// back; all three must round-trip to an identical structure.
func TestExtract_TreeListSameAcrossFormats(t *testing.T) {
	body := "## Items\n\n" +
		"- [x] done item\n" +
		"- [ ] open item with **bold**\n" +
		"  - nested child\n"
	got, diags := run(t, body, listScope(schema.ProjectionTree), nil)
	require.Empty(t, diags)

	jb, err := encode.Encode(encode.JSON, got)
	require.NoError(t, err)
	var jv any
	require.NoError(t, json.Unmarshal(jb, &jv))

	yb, err := encode.Encode(encode.YAML, got)
	require.NoError(t, err)
	var yv any
	require.NoError(t, yaml.Unmarshal(yb, &yv))

	mb, err := encode.Encode(encode.Msgpack, got)
	require.NoError(t, err)
	var mv any
	require.NoError(t, msgpack.Unmarshal(mb, &mv))

	assert.Equal(t, jv, yv, "YAML tree must equal JSON tree")
	assert.Equal(t, jv, mv, "msgpack tree must equal JSON tree")
}

// TestExtract_TreeListChildrenOnlyWhenNonEmpty verifies `children` is
// present only when an item nests a sub-list, and recursive to depth.
func TestExtract_TreeListChildrenOnlyWhenNonEmpty(t *testing.T) {
	body := "## Items\n\n" +
		"- a\n" +
		"  - b\n" +
		"    - c\n"
	items, diags := runList(t, body, schema.ProjectionTree)
	require.Empty(t, diags)
	assert.Equal(t, []any{
		map[string]any{
			"text": "a",
			"children": []any{
				map[string]any{
					"text": "b",
					"children": []any{
						map[string]any{"text": "c"},
					},
				},
			},
		},
	}, items)
}
