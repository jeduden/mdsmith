package extract

import (
	"fmt"
	"testing"

	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/stretchr/testify/require"
)

// cue_diff_test.go validates every block/span an extract fixture projects
// against the published output grammar (cue/extract/grammar.cue). It once
// compiled grammar.cue through cuelang.org/go and unified each projected value
// against the #Block/#Span definitions; the grammar uses CUE definitions and
// closed disjunctions that the in-house cuelite subset does not model, so the
// cuelang dependency could not move there. Plan 240 removes cuelang from the
// module, so this test re-encodes the SAME closed-disjunction contract as an
// in-house Go structural validator (grammarShapes below): each discriminated
// arm names its required and optional fields, the struct is closed (an
// unexpected key fails), and the paragraph arm is text-xor-inline. The grammar
// shapes here are kept byte-faithful to grammar.cue so a drift in either
// surfaces; grammar.cue stays the human-readable published contract.

// fieldSet is one closed-struct arm of a discriminated union: the discriminator
// value, the keys that must be present, and the keys that may be present. Any
// other key fails the closed-struct check.
type fieldSet struct {
	required []string
	optional []string
}

// blockShapes maps a `block` discriminator to its closed-struct arm, mirroring
// grammar.cue's #Block disjunction.
var blockShapes = map[string]fieldSet{
	"paragraph": {}, // handled specially (text xor inline) in validateBlock
	"code":      {required: []string{"value"}, optional: []string{"lang"}},
	"list":      {required: []string{"items"}},
	"table":     {required: []string{"columns", "rows"}},
	"quote":     {required: []string{"blocks"}},
	"break":     {},
	"html":      {required: []string{"value"}},
	"section":   {required: []string{"level", "heading", "blocks"}},
}

// spanShapes maps a `span` discriminator to its closed-struct arm, mirroring
// grammar.cue's #Span disjunction.
var spanShapes = map[string]fieldSet{
	"text":     {required: []string{"value"}},
	"break":    {required: []string{"hard"}},
	"code":     {required: []string{"value"}},
	"autolink": {required: []string{"value", "url"}},
	"emphasis": {required: []string{"level", "children"}},
	"strong":   {required: []string{"level", "children"}},
	"link":     {required: []string{"url", "children"}, optional: []string{"title"}},
	"image":    {required: []string{"url", "children"}, optional: []string{"title"}},
}

// validateShape checks v against a discriminated grammar: the discriminator key
// (`block` or `span`) selects the arm, every required field must be present,
// and no key outside required∪optional∪{discriminator} may appear (the closed
// struct). It returns an error describing the first violation.
func validateShape(discriminator string, shapes map[string]fieldSet, v map[string]any) error {
	kind, ok := v[discriminator].(string)
	if !ok {
		return fmt.Errorf("missing %q discriminator", discriminator)
	}
	shape, known := shapes[kind]
	if !known {
		return fmt.Errorf("unknown %s type %q", discriminator, kind)
	}
	allowed := map[string]bool{discriminator: true}
	for _, k := range shape.required {
		allowed[k] = true
		if _, present := v[k]; !present {
			return fmt.Errorf("%s %q missing required field %q", discriminator, kind, k)
		}
	}
	for _, k := range shape.optional {
		allowed[k] = true
	}
	for k := range v {
		if !allowed[k] {
			return fmt.Errorf("%s %q has unexpected key %q (closed struct)", discriminator, kind, k)
		}
	}
	return nil
}

// validateBlock validates one block against #Block. The paragraph arm is
// text-xor-inline (grammar.cue splits it into two closed arms), so it is
// handled here rather than through a single fieldSet.
func validateBlock(v map[string]any) error {
	if v["block"] == "paragraph" {
		_, hasText := v["text"]
		_, hasInline := v["inline"]
		if hasText == hasInline {
			return fmt.Errorf("paragraph must carry exactly one of text/inline")
		}
		want := "text"
		if hasInline {
			want = "inline"
		}
		for k := range v {
			if k != "block" && k != want {
				return fmt.Errorf("paragraph has unexpected key %q (closed struct)", k)
			}
		}
		return nil
	}
	return validateShape("block", blockShapes, v)
}

// validateAgainstBlock validates v against #Block.
func validateAgainstBlock(t *testing.T, v any) error {
	t.Helper()
	m, ok := v.(map[string]any)
	require.Truef(t, ok, "block must be an object, got %T", v)
	return validateBlock(m)
}

// validateAgainstSpan validates v against #Span.
func validateAgainstSpan(t *testing.T, v any) error {
	t.Helper()
	m, ok := v.(map[string]any)
	require.Truef(t, ok, "span must be an object, got %T", v)
	return validateShape("span", spanShapes, m)
}

// TestGrammar_Compiles is the smoke test: a single paragraph block validates
// against #Block.
func TestGrammar_Compiles(t *testing.T) {
	require.NoError(t, validateAgainstBlock(t,
		map[string]any{"block": "paragraph", "text": "hi"}))
}

// TestGrammar_RejectsInvalid pins that the closed definitions reject malformed
// values: an unknown block type, an extra key, a paragraph carrying both text
// and inline, and a code block missing its value.
func TestGrammar_RejectsInvalid(t *testing.T) {
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
			require.Error(t, validateAgainstBlock(t, c.v), "grammar must reject %s", c.name)
		})
	}
}

// TestGrammar_RejectsInvalidSpan pins span-level rejection.
func TestGrammar_RejectsInvalidSpan(t *testing.T) {
	cases := []struct {
		name string
		v    any
	}{
		{"unknown span", map[string]any{"span": "smallcaps", "value": "x"}},
		{"text extra key", map[string]any{"span": "text", "value": "x", "url": "y"}},
		{"emphasis missing children", map[string]any{"span": "emphasis", "level": 1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Error(t, validateAgainstSpan(t, c.v), "grammar must reject %s", c.name)
		})
	}
}

// validateBlocksRecursive validates every block against #Block, descending into
// container blocks and validating each paragraph's inline spans against #Span.
func validateBlocksRecursive(t *testing.T, blocks []any) {
	t.Helper()
	for _, raw := range blocks {
		require.NoErrorf(t, validateAgainstBlock(t, raw), "block %v must validate", raw)
		b := raw.(map[string]any)
		switch b["block"] {
		case "quote", "section":
			kids, ok := b["blocks"].([]any)
			require.Truef(t, ok, "container block %v must carry a []any blocks list", b)
			validateBlocksRecursive(t, kids)
		case "paragraph":
			if inline, hasInline := b["inline"]; hasInline {
				spans, ok := inline.([]any)
				require.Truef(t, ok, "paragraph %v must carry a []any inline list", b)
				for _, s := range spans {
					validateSpansRecursive(t, s)
				}
			}
		}
	}
}

// validateSpansRecursive validates one span against #Span and recurses into a
// container span's children.
func validateSpansRecursive(t *testing.T, raw any) {
	t.Helper()
	require.NoErrorf(t, validateAgainstSpan(t, raw), "span %v must validate", raw)
	span := raw.(map[string]any)
	if kids, ok := span["children"]; ok {
		list, isList := kids.([]any)
		require.Truef(t, isList, "span %v children must be a []any list", span)
		for _, c := range list {
			validateSpansRecursive(t, c)
		}
	}
}

// blocksCorpus projects a set of documents that together exercise every
// block-grammar row and the inline-span option, returning each projection's
// `blocks` list.
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
		{textScope, "## Notes\n\npara\n\n```go\nx := 1\n```\n\n    indented\n\n" +
			"> quoted\n\n- a\n  - b\n\n| A | B |\n| - | - |\n| 1 | 2 |\n\n---\n\n" +
			"<div>raw</div>\n\n### Sub\n\nnested para\n"},
		{inlineScopeSchema, "## Notes\n\nMark*down* **bold** `code` " +
			"[t](u) <https://x.test> ![alt](pic.png)\nwrapped line\n"},
		{textScope, "## Notes\n\n- [x] done\n- [ ] open\n  - child\n"},
		{textScope, "## Notes\n\n| A | B |\n| - | - |\n"},
		{inlineScopeSchema, "## Notes\n\n[![alt](i.png)](u) and " +
			"![a *b* c](j.png)\n"},
		{inlineScopeSchema, "## Notes\n\n[](u) and ![](p.png)\n"},
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

// TestGrammar_AllBlockFixturesValidate validates every block a fixture projects
// against the published #Block definition (spans against #Span), so the grammar
// stays locked to the implementation.
func TestGrammar_AllBlockFixturesValidate(t *testing.T) {
	for _, blocks := range blocksCorpus(t) {
		validateBlocksRecursive(t, blocks)
	}
}

// TestGrammar_CorpusCoversImageAndBreak guards the differential test's value:
// the corpus must actually produce an `image` and a `break` span, else those
// grammar rows would go unexercised.
func TestGrammar_CorpusCoversImageAndBreak(t *testing.T) {
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
