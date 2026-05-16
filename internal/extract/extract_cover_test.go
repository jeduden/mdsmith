package extract

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/stretchr/testify/assert"
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
