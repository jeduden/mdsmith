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
