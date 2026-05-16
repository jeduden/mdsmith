package extract

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

// The kind-specific extractors guard their type assertions; the
// projector never feeds them a mismatched node in normal flow, but
// the defensive return must still be exercised.
func TestProjectorHelpers_WrongNodeType(t *testing.T) {
	p := &projector{f: doc(t, "x\n")}
	para := ast.NewParagraph()
	assert.Equal(t, "", p.codeBody(para))
	assert.Nil(t, p.listItems(para))
	assert.Nil(t, p.tableRows(para))
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

func TestSetKey_EmptyKeyIsCollision(t *testing.T) {
	p := &projector{f: doc(t, "x\n"), sch: &schema.Schema{}}
	obj := map[string]any{}
	p.setKey(obj, "", "v")
	assert.NotEmpty(t, p.diags)
	assert.Empty(t, obj)
}

func TestExtract_NilFrontmatterNoKey(t *testing.T) {
	sch := &schema.Schema{RootLevel: 2, Sections: []schema.Scope{litScope("Goal")}}
	got, diags := run(t, "## Goal\n\nx\n", sch, nil)
	assert.Empty(t, diags)
	assert.NotContains(t, got.(map[string]any), "frontmatter")
}
