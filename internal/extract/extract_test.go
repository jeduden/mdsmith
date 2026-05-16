package extract

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func doc(t *testing.T, body string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("doc.md", []byte(body))
	require.NoError(t, err)
	return f
}

func litScope(text string) schema.Scope {
	return schema.Scope{Heading: text, Matcher: &schema.Matcher{Regex: text}}
}

func run(t *testing.T, body string, sch *schema.Schema, fm map[string]any) (any, []lint.Diagnostic) {
	t.Helper()
	f := doc(t, body)
	mt := schema.BuildMatchTree(f, sch, fm)
	return Extract(f, sch, mt)
}

func TestExtract_LiteralAndFrontmatter(t *testing.T) {
	sch := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{litScope("Goal")}}
	got, diags := run(t, "## Goal\n\nbody\n", sch, map[string]any{"id": "x"})
	require.Empty(t, diags)
	root := got.(map[string]any)
	assert.Equal(t, map[string]any{"id": "x"}, root["frontmatter"])
	assert.Contains(t, root, "goal")
}

func TestExtract_Nested(t *testing.T) {
	parent := litScope("Steps")
	parent.Sections = []schema.Scope{litScope("First")}
	sch := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{parent}}
	got, diags := run(t, "## Steps\n\n### First\n\nx\n", sch, nil)
	require.Empty(t, diags)
	root := got.(map[string]any)
	steps := root["steps"].(map[string]any)
	assert.Contains(t, steps, "first")
}

func TestExtract_NoHeadingSectionHoists(t *testing.T) {
	pre := schema.Scope{
		Preamble: true,
		Content:  []schema.ContentEntry{{Kind: schema.ContentKindParagraph, Required: true}},
	}
	sch := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{pre, litScope("Goal")}}
	got, diags := run(t, "lead in text\n\n## Goal\n\nbody\n", sch, nil)
	require.Empty(t, diags)
	root := got.(map[string]any)
	// Preamble content hoists into the root, no wrapper key.
	assert.Equal(t, "lead in text", root["text"])
	assert.NotContains(t, root, "preamble")
}

func TestExtract_OptionalOmitted(t *testing.T) {
	opt := schema.Scope{
		Heading: "Extra",
		Matcher: &schema.Matcher{Regex: "Extra", Repeat: schema.Repeat{Set: true, Min: 0, Max: 1}},
	}
	sch := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{litScope("Goal"), opt}}
	got, diags := run(t, "## Goal\n\nbody\n", sch, nil)
	require.Empty(t, diags)
	root := got.(map[string]any)
	assert.Contains(t, root, "goal")
	// Optional section that did not match is an array key only if it
	// matched; here it is omitted entirely (repeat-set but zero
	// occurrences => no group).
	assert.NotContains(t, root, "extra")
}

func TestExtract_RepeatingWithDigits(t *testing.T) {
	rep := schema.Scope{
		Heading: "Step {n}",
		Matcher: &schema.Matcher{Regex: `Step \#(digits)`, Repeat: schema.Repeat{Set: true, Min: 1}},
	}
	sch := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{rep}}
	got, diags := run(t, "## Step 1\n\na\n\n## Step 2\n\nb\n", sch, nil)
	require.Empty(t, diags)
	root := got.(map[string]any)
	arr := root["step"].([]any)
	require.Len(t, arr, 2)
	assert.Equal(t, "1", arr[0].(map[string]any)["n"])
	assert.Equal(t, "2", arr[1].(map[string]any)["n"])
}

func TestExtract_ContentKinds(t *testing.T) {
	sc := schema.Scope{
		Heading: "Goal",
		Matcher: &schema.Matcher{Regex: "Goal"},
		Content: []schema.ContentEntry{
			{Kind: schema.ContentKindParagraph, Required: true},
			{Kind: schema.ContentKindCodeBlock, Required: true},
			{Kind: schema.ContentKindList, Required: true},
			{Kind: schema.ContentKindTable, Required: true},
		},
	}
	sch := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{sc}}
	body := "## Goal\n\nintro para\n\n```go\nx := 1\n```\n\n- one\n- two\n\n" +
		"| A | B |\n| - | - |\n| 1 | 2 |\n"
	got, diags := run(t, body, sch, nil)
	require.Empty(t, diags)
	goal := got.(map[string]any)["goal"].(map[string]any)
	assert.Equal(t, "intro para", goal["text"])
	assert.Equal(t, "x := 1", goal["code"])
	assert.Equal(t, []any{"one", "two"}, goal["items"])
	rows := goal["rows"].([]any)
	require.Len(t, rows, 1)
	assert.Equal(t, map[string]any{"A": "1", "B": "2"}, rows[0])
}

// An absent optional entry must not consume a later required
// entry's node (Copilot review on matchtree.go:229).
func TestExtract_OptionalContentDoesNotEatRequired(t *testing.T) {
	sc := schema.Scope{
		Heading: "Goal",
		Matcher: &schema.Matcher{Regex: "Goal"},
		Content: []schema.ContentEntry{
			{Kind: schema.ContentKindParagraph, Required: false},
			{Kind: schema.ContentKindCodeBlock, Required: true},
		},
	}
	sch := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{sc}}
	got, diags := run(t, "## Goal\n\n```\nx := 1\n```\n", sch, nil)
	require.Empty(t, diags)
	goal := got.(map[string]any)["goal"].(map[string]any)
	assert.Equal(t, "x := 1", goal["code"])
	assert.NotContains(t, goal, "text")
}

// An empty column header is skipped by the duplicate-detection
// scan (it never collides) and the cell falls back to col-N.
func TestExtract_EmptyTableHeader(t *testing.T) {
	sc := schema.Scope{
		Heading: "Goal",
		Matcher: &schema.Matcher{Regex: "Goal"},
		Content: []schema.ContentEntry{{Kind: schema.ContentKindTable, Required: true}},
	}
	sch := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{sc}}
	body := "## Goal\n\n| A |  |\n| - | - |\n| 1 | 2 |\n"
	got, diags := run(t, body, sch, nil)
	require.Empty(t, diags)
	rows := got.(map[string]any)["goal"].(map[string]any)["rows"].([]any)
	require.Len(t, rows, 1)
	assert.Equal(t, map[string]any{"A": "1", "col-2": "2"}, rows[0])
}

// Duplicate table column headers collide as row-object keys; the
// projector must report it rather than silently keep only the last
// cell.
func TestExtract_DuplicateTableHeaderCollides(t *testing.T) {
	sc := schema.Scope{
		Heading: "Goal",
		Matcher: &schema.Matcher{Regex: "Goal"},
		Content: []schema.ContentEntry{{Kind: schema.ContentKindTable, Required: true}},
	}
	sch := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{sc}}
	body := "## Goal\n\n| A | A |\n| - | - |\n| 1 | 2 |\n"
	_, diags := run(t, body, sch, nil)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message, "A")
}

func TestExtract_SiblingCollision(t *testing.T) {
	// Two distinct sibling scopes whose headings slugify to the same
	// key ("Goal" and "Goal!" both → "goal").
	other := schema.Scope{Heading: "Goal!", Matcher: &schema.Matcher{Regex: "Goal!"}}
	sch := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{litScope("Goal"), other}}
	_, diags := run(t, "## Goal\n\na\n\n## Goal!\n\nb\n", sch, nil)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message, "goal")
}

func TestExtract_MultipleCodeBlocksSuffix(t *testing.T) {
	sc := schema.Scope{
		Heading: "Goal",
		Matcher: &schema.Matcher{Regex: "Goal"},
		Content: []schema.ContentEntry{
			{Kind: schema.ContentKindCodeBlock, Required: true},
			{Kind: schema.ContentKindCodeBlock, Required: true},
		},
	}
	sch := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{sc}}
	body := "## Goal\n\n```\nfirst\n```\n\n```\nsecond\n```\n"
	got, diags := run(t, body, sch, nil)
	require.Empty(t, diags)
	goal := got.(map[string]any)["goal"].(map[string]any)
	assert.Equal(t, "first", goal["code"])
	assert.Equal(t, "second", goal["code-2"])
}
