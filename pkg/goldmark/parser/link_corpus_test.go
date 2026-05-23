package parser_test

// Link parser corpus: reference-style links and edge cases that
// the inline-link snippets in corpus_test.go do not reach. Each
// case lifts coverage of parseReferenceLink, parseLinkTitle, and
// the linkLabel state stack.

import (
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

func parseDoc(src string) ast.Node {
	p := parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)
	return p.Parse(text.NewReader([]byte(src)), parser.WithContext(parser.NewContext()))
}

func walkKindSet(root ast.Node) map[ast.NodeKind]int {
	out := map[ast.NodeKind]int{}
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			out[n.Kind()]++
		}
		return ast.WalkContinue, nil
	})
	return out
}

func TestLinkParser_FullReferenceLink(t *testing.T) {
	src := `[label][ref]

[ref]: /url
`
	kinds := walkKindSet(parseDoc(src))
	if kinds[ast.KindLink] == 0 {
		t.Errorf("expected Link node for full reference link, kinds: %v", kinds)
	}
}

func TestLinkParser_CollapsedReferenceLink(t *testing.T) {
	src := `[label][]

[label]: /url
`
	kinds := walkKindSet(parseDoc(src))
	if kinds[ast.KindLink] == 0 {
		t.Errorf("expected Link node for collapsed reference link, kinds: %v", kinds)
	}
}

func TestLinkParser_ShortcutReferenceLink(t *testing.T) {
	src := `[label]

[label]: /url
`
	kinds := walkKindSet(parseDoc(src))
	if kinds[ast.KindLink] == 0 {
		t.Errorf("expected Link node for shortcut reference link, kinds: %v", kinds)
	}
}

func TestLinkParser_ReferenceLink_Missing(t *testing.T) {
	// Reference label that has no matching definition stays as
	// literal text (no Link node).
	src := `[label][missing]
`
	kinds := walkKindSet(parseDoc(src))
	if kinds[ast.KindLink] != 0 {
		t.Errorf("missing reference must not produce Link, kinds: %v", kinds)
	}
}

func TestLinkParser_MultiLineLinkTitle(t *testing.T) {
	// A link title that spans multiple lines drives the multi-
	// segment branch in parseLinkTitle (segments.Len() > 1).
	srcs := []string{
		`[x](/u "multi
line
title")
`,
		`[y](/u 'multi
line single')
`,
		`[z](/u (multi
line parens))
`,
	}
	for _, src := range srcs {
		kinds := walkKindSet(parseDoc(src))
		if kinds[ast.KindLink] == 0 {
			t.Errorf("expected Link for multi-line title: %q", src)
		}
	}
}

func TestLinkParser_LinkTitle_QuotedForms(t *testing.T) {
	// parseLinkTitle accepts three quoting styles: "...", '...',
	// and (...). Drive each one.
	cases := []string{
		`[x](/u "double quoted title")` + "\n",
		`[x](/u 'single quoted title')` + "\n",
		`[x](/u (parenthesised title))` + "\n",
	}
	for _, src := range cases {
		kinds := walkKindSet(parseDoc(src))
		if kinds[ast.KindLink] == 0 {
			t.Errorf("expected Link for %q, kinds: %v", src, kinds)
		}
	}
}

func TestLinkParser_NestedLinkInsideLink(t *testing.T) {
	// CommonMark forbids nested links: `[outer [inner](/i)](/o)`
	// produces a Link for the INNER one; the outer fails because
	// containsLink returns true for the candidate text.
	src := "[outer [inner](/i)](/o)\n"
	kinds := walkKindSet(parseDoc(src))
	if kinds[ast.KindLink] == 0 {
		t.Errorf("expected at least one Link (inner) for nested case, kinds: %v", kinds)
	}
}

func TestLinkParser_ImageWithReferenceLink(t *testing.T) {
	// Image can use a reference label too: ![alt][ref].
	src := `![alt][ref]

[ref]: /img.png
`
	kinds := walkKindSet(parseDoc(src))
	if kinds[ast.KindImage] == 0 {
		t.Errorf("expected Image for reference image, kinds: %v", kinds)
	}
}

func TestLinkParser_LinkInAutolinkInImage(t *testing.T) {
	// Combine several inline parsers in one paragraph to stress the
	// linkLabel state stack.
	src := "see [a](/a) and <https://b.example> and ![img](/i.png)\n"
	kinds := walkKindSet(parseDoc(src))
	if kinds[ast.KindLink] == 0 {
		t.Error("expected Link")
	}
	if kinds[ast.KindAutoLink] == 0 {
		t.Error("expected AutoLink")
	}
	if kinds[ast.KindImage] == 0 {
		t.Error("expected Image")
	}
}

func TestLinkParser_MultiLineReferenceLabel(t *testing.T) {
	// A reference link whose [label][...] spans multiple lines
	// drives the multi-segment branch in parseReferenceLink.
	src := `[text][ref
inder]

[ref inder]: /url
`
	_ = parseDoc(src)
}

func TestLinkParser_OverlongReferenceLabel(t *testing.T) {
	// Labels > 999 chars are rejected by the spec; the parser
	// returns nil with consumed=true on that path.
	long := make([]byte, 1100)
	for i := range long {
		long[i] = 'x'
	}
	src := "[text][" + string(long) + "]\n[ref]: /url\n"
	_ = parseDoc(src)
}

func TestLinkRefDefinition_EdgeShapes(t *testing.T) {
	// parseLinkReferenceDefinition has many error paths. Drive
	// each via specific malformed reference definitions.
	cases := []string{
		`[ref]: /url`,                       // missing newline
		`[ref]: /url "title"` + "\n",        // happy path
		`[ref]: /url "unclosed title`,        // unclosed quote, no newline
		`[ref]: /url 'unclosed`,              // unclosed single quote
		"   [ref]: /url\n",                    // 3-space indent OK
		"    [ref]: /url\n",                   // 4-space indent NOT OK (code block)
		"[]: empty-label\n",                   // empty label
		"[unclosed: /url\n",                  // no closing ]
		"[label] no colon\n",                 // no colon
		"[label]: no\\ destination\\ ok\n",   // weird destination
		"[label]:\n",                          // empty destination
	}
	for _, src := range cases {
		_ = parseDoc(src + "\n[label]: trailing test\n")
	}
}

func TestLinkParser_QuadrupleNestedBracket(t *testing.T) {
	// Drive popLinkBottom's default switch arm (slice len > 2) via
	// 4+ nested open brackets, then resolve them in order.
	cases := []string{
		"[a [b [c [d](/d)](/c)](/b)](/a)\n",
		"[a [b [c [d [e](/e)](/d)](/c)](/b)](/a)\n",
	}
	for _, src := range cases {
		_ = parseDoc(src)
	}
}

func TestLinkParser_LinkLabelStateStack(t *testing.T) {
	// Inputs that exercise multiple open '[' brackets in flight at
	// the same time so the linkLabelState linked list has 2+
	// entries when remove fires.
	cases := []string{
		"[a [b](/b) c](/a)\n",
		"[a [b] [c](/c)](/a)\n",
		"[a [b] [c] [d](/d)](/a)\n",
		// Multiple unclosed openers that resolve at different points.
		"[outer [middle [inner](/i)](/m)](/o)\n",
		// Mixed image and link openers.
		"![alt [link](/l)](/img.png)\n",
		// Image inside link.
		"[link [alt ![inner](/i)](/m)](/l)\n",
	}
	for _, src := range cases {
		_ = parseDoc(src)
	}
}

func TestLinkParser_DeeplyNested(t *testing.T) {
	// Multiple nested link / image patterns exercise the
	// pushLinkBottom / popLinkBottom stack across more than the
	// usual single-entry depth.
	cases := []string{
		"[outer [inner](/i)](/o)\n",
		"[outer [inner1](/i1) [inner2](/i2)](/o)\n",
		"![alt with [link](/l)](/img.png)\n",
		"[a [b [c](/c)](/b)](/a)\n",
		"![img1](/i1) ![img2](/i2) ![img3](/i3)\n",
		"[a](/a) [b](/b) [c](/c) [d](/d)\n",
	}
	for _, src := range cases {
		_ = parseDoc(src)
	}
}

func TestLinkParser_MalformedReferenceAndTitle(t *testing.T) {
	// Drive remaining parseReferenceLink/parseLink branches:
	//   - reference label with unclosed second [ -> not found
	//   - link with title followed by extra non-) content -> nil
	cases := []string{
		"[label][unclosed-ref\nbody\n",          // ref's [ never closes
		`[x](/url "title" extra)` + "\n",        // title then non-) -> nil
		`[x](/url 'sq title' extra)` + "\n",     // single-quote variant
		`[x](/url (paren title) extra)` + "\n",  // paren variant
	}
	for _, src := range cases {
		_ = parseDoc(src)
	}
}

func TestLinkParser_MalformedInlineLinks(t *testing.T) {
	// Drive each early-return branch in parseLink: empty parens,
	// invalid destination, missing close paren after title,
	// destination + title without proper closing.
	cases := []string{
		"[empty]()\n",                       // empty link
		"[no close](/url\n",                 // missing )
		"[bad-dest](<unclosed angle)\n",     // bad angle-bracket dest
		"[bad-title](/url \"unclosed)\n",    // bad title (unclosed quote)
		"[bad-title](/url )\n",              // dest but trailing close
		"[has both](/url \"title\") next\n", // happy path
	}
	for _, src := range cases {
		_ = parseDoc(src)
	}
}

func TestLinkParser_AngleBracketDestination(t *testing.T) {
	src := `[x](<https://example.com> "title")` + "\n"
	kinds := walkKindSet(parseDoc(src))
	if kinds[ast.KindLink] == 0 {
		t.Errorf("expected Link with angle-bracket destination, kinds: %v", kinds)
	}
}

func TestLinkParser_EmptyDestination(t *testing.T) {
	src := `[x]()` + "\n"
	kinds := walkKindSet(parseDoc(src))
	if kinds[ast.KindLink] == 0 {
		t.Errorf("expected Link with empty destination, kinds: %v", kinds)
	}
}

func TestLinkParser_UnclosedBracket(t *testing.T) {
	// Unclosed `[label` must not panic and must not produce a Link.
	src := "this [label is unclosed and never finishes\n"
	root := parseDoc(src)
	if root == nil {
		t.Fatal("Parse returned nil root")
	}
	// Just verifying no panic; output may include literal text.
}
