package extract

import (
	"encoding/json"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueextract "github.com/jeduden/mdsmith/cue/extract"
	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/stretchr/testify/require"
)

// validateAgainst compiles the published grammar, looks up the named
// definition, and reports whether v (a projected block or span)
// unifies with it concretely. A non-nil error means the value is
// outside the documented grammar.
func validateAgainst(t *testing.T, ctx *cue.Context, grammar cue.Value, def string, v any) error {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	data := ctx.CompileBytes(b)
	require.NoError(t, data.Err())
	unified := grammar.LookupPath(cue.ParsePath(def)).Unify(data)
	return unified.Validate(cue.Concrete(true))
}

// grammarValue compiles the embedded grammar.cue once.
func grammarValue(t *testing.T) (*cue.Context, cue.Value) {
	t.Helper()
	ctx := cuecontext.New()
	g := ctx.CompileString(cueextract.Source())
	require.NoError(t, g.Err(), "grammar.cue must compile")
	return ctx, g
}

// TestCUEGrammar_Compiles is the smoke test: the published grammar
// compiles and a single paragraph block validates against #Block.
func TestCUEGrammar_Compiles(t *testing.T) {
	ctx, g := grammarValue(t)
	err := validateAgainst(t, ctx, g, "#Block",
		map[string]any{"block": "paragraph", "text": "hi"})
	require.NoError(t, err)
}

// TestCUEGrammar_RejectsInvalid pins that the closed definitions
// reject malformed values: an unknown block type, an extra key, and a
// paragraph carrying both `text` and `inline` (the disjunction forbids
// it). If any of these validated, the contract would be too loose to
// catch drift.
func TestCUEGrammar_RejectsInvalid(t *testing.T) {
	ctx, g := grammarValue(t)
	cases := []struct {
		name string
		v    any
	}{
		{"unknown block type", map[string]any{"block": "footnote", "value": "x"}},
		{"extra key", map[string]any{"block": "break", "stray": 1}},
		{"paragraph text and inline", map[string]any{
			"block": "paragraph", "text": "x", "inline": []any{},
		}},
		{"code missing value", map[string]any{"block": "code", "lang": "go"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateAgainst(t, ctx, g, "#Block", c.v)
			require.Error(t, err, "grammar must reject %s", c.name)
		})
	}
}

// TestCUEGrammar_RejectsInvalidSpan pins span-level rejection.
func TestCUEGrammar_RejectsInvalidSpan(t *testing.T) {
	ctx, g := grammarValue(t)
	cases := []struct {
		name string
		v    any
	}{
		{"unknown span", map[string]any{"span": "smallcaps", "value": "x"}},
		{"emphasis wrong level", map[string]any{
			"span": "emphasis", "level": 2, "children": []any{},
		}},
		{"text extra key", map[string]any{"span": "text", "value": "x", "url": "y"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateAgainst(t, ctx, g, "#Span", c.v)
			require.Error(t, err, "grammar must reject %s", c.name)
		})
	}
}

// validateBlocksRecursive validates every block in a `blocks` list
// against #Block, descending into container blocks (quote, section)
// and validating each `inline` paragraph's spans against #Span. It is
// the differential check the plan's acceptance criterion names: a
// fixture's projected blocks must all conform to the published
// grammar, so the grammar cannot drift from the walker.
func validateBlocksRecursive(t *testing.T, ctx *cue.Context, g cue.Value, blocks []any) {
	t.Helper()
	for _, raw := range blocks {
		b, ok := raw.(map[string]any)
		require.True(t, ok, "block must be an object, got %T", raw)
		require.NoError(t, validateAgainst(t, ctx, g, "#Block", b),
			"block %v must validate against #Block", b)
		switch b["block"] {
		case "quote", "section":
			kids, ok := b["blocks"].([]any)
			require.True(t, ok,
				"container block %v must carry a []any blocks list", b)
			validateBlocksRecursive(t, ctx, g, kids)
		case "paragraph":
			if inline, hasInline := b["inline"]; hasInline {
				spans, ok := inline.([]any)
				require.True(t, ok,
					"paragraph %v must carry a []any inline list", b)
				for _, s := range spans {
					sp, ok := s.(map[string]any)
					require.True(t, ok, "span must be an object, got %T", s)
					validateSpansRecursive(t, ctx, g, sp)
				}
			}
		}
	}
}

// validateSpansRecursive validates one span against #Span and recurses
// into a container span's children.
func validateSpansRecursive(t *testing.T, ctx *cue.Context, g cue.Value, span map[string]any) {
	t.Helper()
	require.NoError(t, validateAgainst(t, ctx, g, "#Span", span),
		"span %v must validate against #Span", span)
	if kids, ok := span["children"]; ok {
		list, isList := kids.([]any)
		require.True(t, isList,
			"span %v children must be a []any list", span)
		for _, c := range list {
			child, isObj := c.(map[string]any)
			require.True(t, isObj, "child span must be an object, got %T", c)
			validateSpansRecursive(t, ctx, g, child)
		}
	}
}

// blocksCorpus projects a set of documents that together exercise
// every block-grammar row and the inline-span option, returning each
// projection's `blocks` list. The differential test validates the
// whole corpus against the published grammar.
func blocksCorpus(t *testing.T) [][]any {
	t.Helper()
	textScope := blocksScope("Notes")
	inlineScopeSchema := &schema.Schema{
		RootLevel: 2,
		Sections: []schema.Scope{{
			Heading:         "Notes",
			Matcher:         &schema.Matcher{Regex: "Notes"},
			Projection:      schema.ProjectionBlocks,
			BlockParagraphs: schema.ProjectionInline,
		}},
	}
	cases := []struct {
		sch  *schema.Schema
		body string
	}{
		// Every leaf + container block in document order.
		{textScope, "## Notes\n\npara\n\n```go\nx := 1\n```\n\n    indented\n\n" +
			"> quoted\n\n- a\n  - b\n\n| A | B |\n| - | - |\n| 1 | 2 |\n\n---\n\n" +
			"<div>raw</div>\n\n### Sub\n\nnested para\n"},
		// Inline-paragraph option: text, emphasis, strong, code, link,
		// autolink, image, and a soft break.
		{inlineScopeSchema, "## Notes\n\nMark*down* **bold** `code` " +
			"[t](u) <https://x.test> ![alt](pic.png)\nwrapped line\n"},
		// A task list (tree items carry `checked`).
		{textScope, "## Notes\n\n- [x] done\n- [ ] open\n  - child\n"},
		// A header-only table (no body rows) emits `rows: []`; the
		// closed `block_table` arm must accept the empty list, not
		// just populated rows.
		{textScope, "## Notes\n\n| A | B |\n| - | - |\n"},
		// Lenient image spans nested inside container spans: an image
		// linked (image inside a link's `children`) and an image whose
		// alt text carries emphasis (a non-text image child). These pin
		// that block-inline leniency propagates through a container
		// span's recursion, so the image arm's `children: [...#Span]`
		// is validated for a non-trivial child, not just a bare alt.
		{inlineScopeSchema, "## Notes\n\n[![alt](i.png)](u) and " +
			"![a *b* c](j.png)\n"},
		// Childless containers: an empty-text link and an empty-alt
		// image must emit `children: []`, never `children: null` — the
		// null-vs-empty class the contract rejects (the rows:[] case
		// above is the block-level sibling of the same class).
		{inlineScopeSchema, "## Notes\n\n[](u) and ![](p.png)\n"},
		// Empty containers at block level: a bare blockquote and a
		// body-less deeper heading must emit `blocks: []`.
		{textScope, "## Notes\n\n> x\n\n### Only\n"},
	}
	corpus := make([][]any, 0, len(cases))
	for _, c := range cases {
		got, diags := run(t, c.body, c.sch, nil)
		require.Empty(t, diags, "corpus body must project cleanly: %q", c.body)
		notes := got.(map[string]any)["notes"].(map[string]any)
		corpus = append(corpus, notes["blocks"].([]any))
	}
	return corpus
}

// TestCUEGrammar_AllBlockFixturesValidate is the plan-246 acceptance
// criterion: every block a fixture projects validates against the
// published #Block definition (spans against #Span), so the grammar
// stays locked to the implementation.
func TestCUEGrammar_AllBlockFixturesValidate(t *testing.T) {
	ctx, g := grammarValue(t)
	for _, blocks := range blocksCorpus(t) {
		validateBlocksRecursive(t, ctx, g, blocks)
	}
}

// TestCUEGrammar_CorpusCoversImageAndBreak guards the differential
// test's value: it would still pass if the corpus never produced an
// `image` or `break` span (those grammar rows would go unexercised).
// Assert the inline corpus case actually emits both.
func TestCUEGrammar_CorpusCoversImageAndBreak(t *testing.T) {
	corpus := blocksCorpus(t)
	var sawImage, sawBreak bool
	var walk func(blocks []any)
	walk = func(blocks []any) {
		for _, raw := range blocks {
			b := raw.(map[string]any)
			switch b["block"] {
			case "quote", "section":
				walk(b["blocks"].([]any))
			case "paragraph":
				if inline, ok := b["inline"]; ok {
					for _, s := range inline.([]any) {
						switch s.(map[string]any)["span"] {
						case "image":
							sawImage = true
						case "break":
							sawBreak = true
						}
					}
				}
			}
		}
	}
	for _, blocks := range corpus {
		walk(blocks)
	}
	require.True(t, sawImage, "corpus must exercise the image span row")
	require.True(t, sawBreak, "corpus must exercise the break span row")
}
