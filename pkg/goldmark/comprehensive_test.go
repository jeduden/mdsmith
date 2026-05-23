package goldmark_test

// A comprehensive Markdown corpus that exercises rare branches
// across the parser, renderer, and extension surfaces in a single
// Convert call.  Each section targets specific uncovered paths.

import (
	"bytes"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

const comprehensiveCorpus = `# Heading 1 with {#explicit-id .first-class .second}

Setext H1
=========

Setext H2
---------

A paragraph followed by a *softline*
break and a **strong** with ~~strike~~.

A paragraph with ` + "`code span`" + ` and \\` + "`escaped backtick`" + `.

> Blockquote with > nested >> deeper.
>
> Second paragraph in blockquote.

- Tight list one
- Tight list two

- Loose list one

- Loose list two

1. Ordered
2. ` + "`code`" + ` in list
3. **bold** in list

Indented code:

    func hello() {
        return "world"
    }

` + "```go" + `
fenced code with info
` + "```" + `

Image: ![alt **bold**](/img.png "title")
Link: [text](/url "title")
Auto: <https://example.com>
Email: <user@example.com>

| col1 | col2 | col3 |
| :--- | :--: | ---: |
| a    | b    | c    |
| ` + "`x|y`" + ` | d | e |

Task list:

- [x] done
- [ ] todo
- [X] uppercase done

Definition list:

term1
:   def1

term2
:   def2 with a paragraph
:   another def

Footnote ref[^1] and another[^a].

[^1]: footnote one body.
[^a]: footnote a body
    with continuation indent.

---

Multi-line setext heading
content
=========================

[ref-link][ref-key] and another [ref-key] shortcut.

[ref-key]: /ref-url "ref title"
`

func TestComprehensiveCorpus_RareSyntax(t *testing.T) {
	// Drive rarely-hit branches via uncommon Markdown shapes.
	src := `
> > > triple nested blockquote
> > continuing two
> continuing one
back at root

99999. nine-digit ordered list start

   - 3-space indented bullet (still a list)

1)  ordered with parens

* * *

___

***

Setext with attribute {#sattr .scls}
=====================================

Indented setext
   ===========

	tab-indented code block line
	another tab line

` + "```" + `
fenced empty info
` + "```" + `

` + "~~~yaml" + `
tilde fence with info
` + "~~~" + `

| h |
|---|
| a |
| b\|c |
| d |
` + "[^trailing^]: trailing-special label\n[^trailing^]\n"
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.Footnote,
			extension.Table,
			extension.Strikethrough,
		),
		goldmark.WithParserOptions(parser.WithAutoHeadingID(), parser.WithAttribute()),
	)
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
}

func TestCorpus_EdgeShapes(t *testing.T) {
	// Inputs designed to drive remaining branches: degenerate
	// shapes (empty, whitespace-only, EOF in unusual places),
	// boundary cases (very long lines, deeply indented content),
	// and unusual character combinations.
	cases := []string{
		"",
		" ",
		"\n",
		"\n\n\n",
		"  \n  \n",
		"\t\n\t\n",
		// Indented code blocks with tab + space mixes.
		"    line1\n      line2 (4+2 indent)\n",
		// Reference definition spanning lines.
		"[ref]: /url\n  \"title spanning\nmultiple lines\"\n[ref]\n",
		// Code block immediately followed by content.
		"    code\nparagraph after\n",
		// Tight list of 1.
		"- one\n",
		// Empty bullet at end.
		"- one\n-\n",
		// Bare URL in autolink.
		"<http://example.org/?x=1&y=2>\n",
		// Email at start of line.
		"<a@b.co>\n",
		// Backslash escapes.
		"\\*not emphasis\\* \\\\ \\[ \\] \\(  \\) \\#\n",
		// Hard line break (two trailing spaces).
		"line one  \nline two\n",
		// Backslash hard line break.
		"line one\\\nline two\n",
		// Numeric reference + named.
		"&amp; &#65; &#x41; &unknownentity;\n",
	}
	for i, src := range cases {
		md := goldmark.New(goldmark.WithRendererOptions(html.WithUnsafe()))
		var buf bytes.Buffer
		if err := md.Convert([]byte(src), &buf); err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
	}
}

func TestComprehensiveCorpus(t *testing.T) {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.Footnote,
			extension.DefinitionList,
			extension.Strikethrough,
			extension.Table,
			extension.TaskList,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			parser.WithAttribute(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
			html.WithUnsafe(),
		),
	)
	var buf bytes.Buffer
	if err := md.Convert([]byte(comprehensiveCorpus), &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("comprehensive corpus produced empty output")
	}
}
