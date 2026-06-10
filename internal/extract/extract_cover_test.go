package extract

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
)

// The kind-specific extractors guard their type assertions; the
// projector never feeds them a mismatched node in normal flow, but
// the defensive return must still be exercised.
func TestProjectorHelpers_WrongNodeType(t *testing.T) {
	p := &projector{f: doc(t, "x\n")}
	para := ast.NewParagraph()
	assert.Equal(t, "", p.codeBody(para))
	assert.Nil(t, p.listItems(para))
	assert.Nil(t, p.listTree(para))
	assert.Nil(t, p.tableRows(para))
	cols, rows := p.tableRowsPositional(para)
	assert.Nil(t, cols)
	assert.Nil(t, rows)
}

// The table-extension parser pads short body rows to the header
// width, so tableRowsPositional's own padding branch is unreachable
// from Markdown source; drive it by stripping cells off a matched
// table's row.
func TestTableRowsPositional_PadsHandBuiltShortRow(t *testing.T) {
	f := doc(t, "## Data\n\n| A | B | C |\n| - | - | - |\n| x | y | z |\n")
	sc := schema.Scope{
		Heading: "Data",
		Matcher: &schema.Matcher{Regex: "Data"},
		Content: []schema.ContentEntry{
			{Kind: schema.ContentKindTable, Required: true},
		},
	}
	sch := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{sc}}
	mt := schema.BuildMatchTree(f, sch, nil)
	require.NotNil(t, mt)
	require.NotEmpty(t, mt.Root.Children)
	content := mt.Root.Children[0].Content
	require.NotEmpty(t, content)
	tbl, ok := content[0].Node.(*extast.Table)
	require.True(t, ok)

	var body *extast.TableRow
	for r := tbl.FirstChild(); r != nil; r = r.NextSibling() {
		if rr, isRow := r.(*extast.TableRow); isRow {
			body = rr
		}
	}
	require.NotNil(t, body)
	body.RemoveChild(body, body.LastChild())
	body.RemoveChild(body, body.LastChild())

	p := &projector{f: f}
	cols, rows := p.tableRowsPositional(tbl)
	assert.Equal(t, []any{"A", "B", "C"}, cols)
	require.Len(t, rows, 1)
	assert.Equal(t, []any{"x", "", ""}, rows[0])
}

func TestKeyFor_FallbackToHeadingSlug(t *testing.T) {
	// Empty literal stem, no fmvar fields → slug of the raw
	// heading label.
	sc := &schema.Scope{Heading: "Weird Name", Matcher: &schema.Matcher{Regex: ""}}
	assert.Equal(t, "weird-name", keyFor(sc))
}

// A repeating scope whose heading is only an fmvar placeholder
// keys by the placeholder name (keyFor's fmvar fallback).
func TestKeyFor_FmvarFallback(t *testing.T) {
	rep := schema.Scope{
		Heading: "{id}",
		Matcher: &schema.Matcher{
			Regex:  `\#(fmvar(id))`,
			Repeat: schema.Repeat{Set: true, Min: 1},
		},
	}
	sch := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{rep}}
	got, diags := run(t, "## RFC-1\n\nbody\n", sch, map[string]any{"id": "RFC-1"})
	require.Empty(t, diags)
	arr := got.(map[string]any)["id"].([]any)
	require.Len(t, arr, 1)
	assert.Equal(t, "RFC-1", arr[0].(map[string]any)["id"])
}

// Two matches of a non-repeating scope collide on the same key
// (projectChildren's len(group) > 1 branch). The match tree's
// in-order matcher claims only one occurrence, so this is driven
// with a hand-built tree.
func TestExtract_DuplicateNonRepeatingCollision(t *testing.T) {
	sc := litScope("Goal")
	sch := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{sc}}
	mt := &schema.MatchTree{Root: &schema.ScopeMatch{
		Children: []*schema.ScopeMatch{
			{Scope: &sch.Sections[0], Heading: schema.DocHeading{Text: "Goal"}},
			{Scope: &sch.Sections[0], Heading: schema.DocHeading{Text: "Goal"}},
		},
	}}
	_, diags := Extract(doc(t, "## Goal\n"), sch, mt)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message, "goal")
}

// Collision diagnostics go straight to the CLI without
// AdjustDiagnostics, so their Line must be an absolute positive
// value even for a front-matter-stripped file (LineOffset > 0).
func TestExtract_CollisionDiagnosticLineIsPositive(t *testing.T) {
	src := []byte("---\nid: x\n---\n## Goal\n")
	f, err := lint.NewFileFromSource("doc.md", src, true)
	require.NoError(t, err)
	require.Positive(t, f.LineOffset)

	sc := litScope("Goal")
	sch := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{sc}}
	mt := &schema.MatchTree{Root: &schema.ScopeMatch{
		Children: []*schema.ScopeMatch{
			{Scope: &sch.Sections[0], Heading: schema.DocHeading{Text: "Goal"}},
			{Scope: &sch.Sections[0], Heading: schema.DocHeading{Text: "Goal"}},
		},
	}}
	_, diags := Extract(f, sch, mt)
	require.NotEmpty(t, diags)
	assert.Equal(t, 1, diags[0].Line)
}

func TestIsRepeating(t *testing.T) {
	assert.False(t, isRepeating(nil))
	assert.False(t, isRepeating(&schema.Scope{}))
	assert.False(t, isRepeating(&schema.Scope{Matcher: &schema.Matcher{}}))
	assert.True(t, isRepeating(&schema.Scope{
		Matcher: &schema.Matcher{Repeat: schema.Repeat{Set: true}},
	}))
}

// A ContentMatch whose kind is none of the four projected shapes
// (defensive: collectContent never emits one) must be ignored
// without panicking — this drives the switch's no-match arm.
func TestProjectContent_UnknownKindIgnored(t *testing.T) {
	f := doc(t, "para\n")
	para := f.AST.FirstChild()
	p := &projector{f: f}
	obj := map[string]any{}
	p.projectContent([]schema.ContentMatch{
		{Entry: &schema.ContentEntry{Kind: "bogus"}, Node: para, Line: 1},
	}, obj)
	assert.Empty(t, obj)
	assert.Empty(t, p.diags)
}

func TestSetKey_EmptyKeyIsCollision(t *testing.T) {
	p := &projector{f: doc(t, "x\n"), sch: &schema.Schema{}}
	obj := map[string]any{}
	p.setKey(obj, "", "v")
	assert.NotEmpty(t, p.diags)
	assert.Empty(t, obj)
}

func TestExtract_NilFrontmatterEmptyObject(t *testing.T) {
	sch := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{litScope("Goal")}}
	got, diags := run(t, "## Goal\n\nx\n", sch, nil)
	assert.Empty(t, diags)
	// The root always carries a `frontmatter` object, empty when
	// the document has no front matter.
	assert.Equal(t, map[string]any{}, got.(map[string]any)["frontmatter"])
}
