package extract

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// inlineScope builds a single-section schema whose body is one
// paragraph projected as inline spans.
func inlineScope() *schema.Schema {
	return &schema.Schema{
		RootLevel: 2,
		Sections: []schema.Scope{{
			Heading: "Headline",
			Matcher: &schema.Matcher{Regex: "Headline"},
			Content: []schema.ContentEntry{{
				Kind:       schema.ContentKindParagraph,
				Required:   true,
				Projection: schema.ProjectionInline,
			}},
		}},
	}
}

// TestExtract_InlineHeadline pins the worked example from plan 212:
// `Mark*down*, smithed.` projects as text / emphasis / text.
func TestExtract_InlineHeadline(t *testing.T) {
	got, diags := run(t, "## Headline\n\nMark*down*, smithed.\n", inlineScope(), nil)
	require.Empty(t, diags)
	headline := got.(map[string]any)["headline"].(map[string]any)
	spans := headline["inline"].([]any)
	require.Len(t, spans, 3)
	assert.Equal(t, map[string]any{"span": "text", "value": "Mark"}, spans[0])
	em := spans[1].(map[string]any)
	assert.Equal(t, "emphasis", em["span"])
	assert.Equal(t, 1, em["level"])
	assert.Equal(t, []any{map[string]any{"span": "text", "value": "down"}},
		em["children"])
	assert.Equal(t, map[string]any{"span": "text", "value": ", smithed."}, spans[2])
}

// TestExtract_InlineStrongLevel2 verifies `**bold**` becomes a
// strong span at level 2.
func TestExtract_InlineStrongLevel2(t *testing.T) {
	got, diags := run(t, "## Headline\n\n**bold**\n", inlineScope(), nil)
	require.Empty(t, diags)
	spans := got.(map[string]any)["headline"].(map[string]any)["inline"].([]any)
	require.Len(t, spans, 1)
	strong := spans[0].(map[string]any)
	assert.Equal(t, "strong", strong["span"])
	assert.Equal(t, 2, strong["level"])
	assert.Equal(t, []any{map[string]any{"span": "text", "value": "bold"}},
		strong["children"])
}

// TestExtract_InlineCodeSpan verifies a code span emits a leaf with
// its verbatim value.
func TestExtract_InlineCodeSpan(t *testing.T) {
	got, diags := run(t, "## Headline\n\nrun `mdsmith fix` now\n", inlineScope(), nil)
	require.Empty(t, diags)
	spans := got.(map[string]any)["headline"].(map[string]any)["inline"].([]any)
	require.Len(t, spans, 3)
	assert.Equal(t, map[string]any{"span": "text", "value": "run "}, spans[0])
	assert.Equal(t, map[string]any{"span": "code", "value": "mdsmith fix"}, spans[1])
	assert.Equal(t, map[string]any{"span": "text", "value": " now"}, spans[2])
}

// TestExtract_InlineNestedStrongCode pins the plan's nesting example:
// a strong span containing a code span round-trips uniformly.
func TestExtract_InlineNestedStrongCode(t *testing.T) {
	got, diags := run(t, "## Headline\n\n**`mdsmith fix`**\n", inlineScope(), nil)
	require.Empty(t, diags)
	spans := got.(map[string]any)["headline"].(map[string]any)["inline"].([]any)
	require.Len(t, spans, 1)
	strong := spans[0].(map[string]any)
	assert.Equal(t, "strong", strong["span"])
	assert.Equal(t, 2, strong["level"])
	assert.Equal(t, []any{
		map[string]any{"span": "code", "value": "mdsmith fix"},
	}, strong["children"])
}

// TestExtract_InlineLink verifies a `[text](url "title")` link emits
// a container span carrying url, title, and children.
func TestExtract_InlineLink(t *testing.T) {
	got, diags := run(t,
		"## Headline\n\nsee [the **docs**](https://example.com \"Docs\")\n",
		inlineScope(), nil)
	require.Empty(t, diags)
	spans := got.(map[string]any)["headline"].(map[string]any)["inline"].([]any)
	require.Len(t, spans, 2)
	assert.Equal(t, map[string]any{"span": "text", "value": "see "}, spans[0])
	link := spans[1].(map[string]any)
	assert.Equal(t, "link", link["span"])
	assert.Equal(t, "https://example.com", link["url"])
	assert.Equal(t, "Docs", link["title"])
	children := link["children"].([]any)
	require.Len(t, children, 2)
	assert.Equal(t, map[string]any{"span": "text", "value": "the "}, children[0])
	strong := children[1].(map[string]any)
	assert.Equal(t, "strong", strong["span"])
	assert.Equal(t, []any{map[string]any{"span": "text", "value": "docs"}},
		strong["children"])
}

// TestExtract_InlineLinkNoTitle omits the title key when the link has
// no title.
func TestExtract_InlineLinkNoTitle(t *testing.T) {
	got, diags := run(t, "## Headline\n\n[home](https://example.com)\n",
		inlineScope(), nil)
	require.Empty(t, diags)
	spans := got.(map[string]any)["headline"].(map[string]any)["inline"].([]any)
	require.Len(t, spans, 1)
	link := spans[0].(map[string]any)
	assert.Equal(t, "link", link["span"])
	assert.Equal(t, "https://example.com", link["url"])
	assert.NotContains(t, link, "title")
}

// TestExtract_InlineAutolink verifies an `<https://…>` autolink emits
// a leaf with both value and url.
func TestExtract_InlineAutolink(t *testing.T) {
	got, diags := run(t, "## Headline\n\nvisit <https://example.com>\n",
		inlineScope(), nil)
	require.Empty(t, diags)
	spans := got.(map[string]any)["headline"].(map[string]any)["inline"].([]any)
	require.Len(t, spans, 2)
	assert.Equal(t, map[string]any{"span": "text", "value": "visit "}, spans[0])
	auto := spans[1].(map[string]any)
	assert.Equal(t, "autolink", auto["span"])
	assert.Equal(t, "https://example.com", auto["value"])
	assert.Equal(t, "https://example.com", auto["url"])
}

// TestExtract_InlineAutolinkEmail verifies an email autolink emits a
// usable mailto: href in url while value keeps the bare address.
func TestExtract_InlineAutolinkEmail(t *testing.T) {
	got, diags := run(t, "## Headline\n\nmail <jeduden@gmail.com>\n",
		inlineScope(), nil)
	require.Empty(t, diags)
	spans := got.(map[string]any)["headline"].(map[string]any)["inline"].([]any)
	require.Len(t, spans, 2)
	auto := spans[1].(map[string]any)
	assert.Equal(t, "autolink", auto["span"])
	assert.Equal(t, "jeduden@gmail.com", auto["value"])
	assert.Equal(t, "mailto:jeduden@gmail.com", auto["url"])
}

// TestExtract_InlineBreakAfterNonText verifies that a line break
// following a non-text node (here an emphasis) records only the break
// span. goldmark hosts the break flag on a zero-length text node it
// inserts after the non-text node, which must not leak an empty
// {span:text,value:""} into the projection.
func TestExtract_InlineBreakAfterNonText(t *testing.T) {
	got, diags := run(t, "## Headline\n\n*em*\nafter\n", inlineScope(), nil)
	require.Empty(t, diags)
	spans := got.(map[string]any)["headline"].(map[string]any)["inline"].([]any)
	for i, s := range spans {
		if m := s.(map[string]any); m["span"] == "text" {
			assert.NotEqual(t, "", m["value"], "span %d is an empty text span", i)
		}
	}
	require.Len(t, spans, 3)
	assert.Equal(t, "emphasis", spans[0].(map[string]any)["span"])
	assert.Equal(t, map[string]any{"span": "break", "hard": false}, spans[1])
	assert.Equal(t, map[string]any{"span": "text", "value": "after"}, spans[2])
}

// TestExtract_InlineRejectsImage is a hard error: an image has no
// inline-span representation in the mapping table.
func TestExtract_InlineRejectsImage(t *testing.T) {
	_, diags := run(t, "## Headline\n\n![alt](img.png)\n", inlineScope(), nil)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message, "image")
}

// TestExtract_InlineRejectsRawHTML is a hard error: inline raw HTML
// is not in the mapping table.
func TestExtract_InlineRejectsRawHTML(t *testing.T) {
	_, diags := run(t, "## Headline\n\ntext <span>x</span> more\n",
		inlineScope(), nil)
	require.NotEmpty(t, diags)
	assert.Contains(t, diags[0].Message, "raw HTML")
}

// TestExtract_TextAndInlineCoexist verifies that a scope holding a
// `text`-projected paragraph and an `inline`-projected paragraph
// emits two sibling keys (the distinct default keys `text` and
// `inline`) rather than colliding. Content entries are positional, so
// each binds its own paragraph node.
func TestExtract_TextAndInlineCoexist(t *testing.T) {
	sch := &schema.Schema{
		RootLevel: 2,
		Sections: []schema.Scope{{
			Heading: "Headline",
			Matcher: &schema.Matcher{Regex: "Headline"},
			Content: []schema.ContentEntry{
				{Kind: schema.ContentKindParagraph, Required: true,
					Projection: schema.ProjectionText},
				{Kind: schema.ContentKindParagraph, Required: true,
					Projection: schema.ProjectionInline},
			},
		}},
	}
	got, diags := run(t, "## Headline\n\nplain prose.\n\nMark*down*.\n", sch, nil)
	require.Empty(t, diags)
	headline := got.(map[string]any)["headline"].(map[string]any)
	assert.Equal(t, "plain prose.", headline["text"])
	spans := headline["inline"].([]any)
	require.Len(t, spans, 3)
	assert.Equal(t, map[string]any{"span": "text", "value": "Mark"}, spans[0])
}

// TestExtract_InlineSoftBreak verifies a wrapped paragraph
// (`first⏎second`) emits a `break` span with `hard: false` after the
// first text node's span, so the line structure survives projection.
func TestExtract_InlineSoftBreak(t *testing.T) {
	got, diags := run(t, "## Headline\n\nfirst\nsecond\n", inlineScope(), nil)
	require.Empty(t, diags)
	spans := got.(map[string]any)["headline"].(map[string]any)["inline"].([]any)
	require.Len(t, spans, 3)
	assert.Equal(t, map[string]any{"span": "text", "value": "first"}, spans[0])
	assert.Equal(t, map[string]any{"span": "break", "hard": false}, spans[1])
	assert.Equal(t, map[string]any{"span": "text", "value": "second"}, spans[2])
}

// TestExtract_InlineHardBreak verifies a hard line break (two trailing
// spaces before the newline) emits a `break` span with `hard: true`.
func TestExtract_InlineHardBreak(t *testing.T) {
	got, diags := run(t, "## Headline\n\nfirst  \nsecond\n", inlineScope(), nil)
	require.Empty(t, diags)
	spans := got.(map[string]any)["headline"].(map[string]any)["inline"].([]any)
	require.Len(t, spans, 3)
	assert.Equal(t, map[string]any{"span": "text", "value": "first"}, spans[0])
	assert.Equal(t, map[string]any{"span": "break", "hard": true}, spans[1])
	assert.Equal(t, map[string]any{"span": "text", "value": "second"}, spans[2])
}

// TestExtract_InlineBreakInsideContainer verifies a soft break inside
// an emphasis span emits the `break` span among the container's
// children, not at the top level.
func TestExtract_InlineBreakInsideContainer(t *testing.T) {
	got, diags := run(t, "## Headline\n\n*first\nsecond*\n", inlineScope(), nil)
	require.Empty(t, diags)
	spans := got.(map[string]any)["headline"].(map[string]any)["inline"].([]any)
	require.Len(t, spans, 1)
	em := spans[0].(map[string]any)
	assert.Equal(t, "emphasis", em["span"])
	assert.Equal(t, []any{
		map[string]any{"span": "text", "value": "first"},
		map[string]any{"span": "break", "hard": false},
		map[string]any{"span": "text", "value": "second"},
	}, em["children"])
}

// TestExtract_InlineBindResolvesCollision verifies a `bind:` override
// renames an inline projection's default key, so two inline entries
// can coexist without colliding on `inline`.
func TestExtract_InlineBindResolvesCollision(t *testing.T) {
	alt := "headline-spans"
	sch := &schema.Schema{
		RootLevel: 2,
		Sections: []schema.Scope{{
			Heading: "Headline",
			Matcher: &schema.Matcher{Regex: "Headline"},
			Content: []schema.ContentEntry{
				{Kind: schema.ContentKindParagraph, Required: true,
					Projection: schema.ProjectionInline, Bind: &alt},
			},
		}},
	}
	got, diags := run(t, "## Headline\n\nhi\n", sch, nil)
	require.Empty(t, diags)
	headline := got.(map[string]any)["headline"].(map[string]any)
	assert.Contains(t, headline, "headline-spans")
	assert.NotContains(t, headline, "inline")
}

// TestExtract_InlineUnsupportedNamesActualKey verifies the
// unsupported-inline diagnostic names the actual projection key rather
// than the literal "inline" default. A `bind:` override renames the key,
// so an unsupported node (an image) under that entry must surface a
// diagnostic that leads with the bind name — pointing at a field that
// exists in the emitted data. Regression test for the post-rebase review.
func TestExtract_InlineUnsupportedNamesActualKey(t *testing.T) {
	alt := "headline-spans"
	sch := &schema.Schema{
		RootLevel: 2,
		Sections: []schema.Scope{{
			Heading: "Headline",
			Matcher: &schema.Matcher{Regex: "Headline"},
			Content: []schema.ContentEntry{
				{Kind: schema.ContentKindParagraph, Required: true,
					Projection: schema.ProjectionInline, Bind: &alt},
			},
		}},
	}
	_, diags := run(t, "## Headline\n\n![alt](img.png)\n", sch, nil)
	require.NotEmpty(t, diags)
	assert.Truef(t, strings.HasPrefix(diags[0].Message, "headline-spans:"),
		"diagnostic should lead with the bind key, got %q", diags[0].Message)
}

// TestExtract_InlineUnsupportedNamesSuffixedKey verifies the
// unsupported-inline diagnostic carries the -N repeat suffix: a second
// inline paragraph projects under `inline-2`, so an unsupported node
// there must name `inline-2`, not the bare `inline`.
func TestExtract_InlineUnsupportedNamesSuffixedKey(t *testing.T) {
	sch := &schema.Schema{
		RootLevel: 2,
		Sections: []schema.Scope{{
			Heading: "Headline",
			Matcher: &schema.Matcher{Regex: "Headline"},
			Content: []schema.ContentEntry{
				{Kind: schema.ContentKindParagraph, Required: true,
					Projection: schema.ProjectionInline},
				{Kind: schema.ContentKindParagraph, Required: true,
					Projection: schema.ProjectionInline},
			},
		}},
	}
	_, diags := run(t, "## Headline\n\nok\n\n![alt](img.png)\n", sch, nil)
	require.NotEmpty(t, diags)
	assert.Truef(t, strings.HasPrefix(diags[0].Message, "inline-2:"),
		"diagnostic should lead with the suffixed key, got %q", diags[0].Message)
}
