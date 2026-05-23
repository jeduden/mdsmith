package goldmark_test

// A comprehensive Markdown corpus that exercises rare branches
// across the parser, renderer, and extension surfaces in a single
// Convert call.  Each section targets specific uncovered paths.

import (
	"bytes"
	"strings"
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

func TestCorpus_VariedShapes(t *testing.T) {
	// Mass corpus driving rare parser/renderer paths.
	cases := []string{
		// Various heading levels.
		"# H1\n## H2\n### H3\n#### H4\n##### H5\n###### H6\n",
		// Empty heading.
		"#\n",
		"#  \n",
		// Closing-hash heading variants.
		"# Heading #\n",
		"## Heading ##\n",
		"### Heading\\#\n",
		// Setext both forms.
		"H1\n===\n",
		"H2\n---\n",
		// Multi-line paragraph.
		"line 1\nline 2\nline 3\n",
		// Paragraph with trailing soft line breaks.
		"first  \nsecond  \nthird\n",
		// Hard line breaks (backslash + newline).
		"first\\\nsecond\\\nthird\n",
		// Code spans with various backtick counts.
		"`single`\n``double tick`s` end``\n`a` `b` `c`\n",
		// Reference link variants.
		"[full][r]\n[collapsed][]\n[shortcut]\n\n[r]: /r\n[collapsed]: /c\n[shortcut]: /s\n",
		// Reference definitions with titles.
		"[ref]\n\n[ref]: /url \"quoted title\"\n[ref2]: /url2 'single title'\n[ref3]: /url3 (paren title)\n",
		// Multi-line reference definitions.
		"[ref]\n\n[ref]:\n  /url\n  \"multi-line title\"\n",
		// Link with angle-bracket URL.
		"[x](</url with space>)\n",
		// Indented code blocks.
		"    code1\n    code2\n",
		// Indented code preceded by paragraph.
		"para\n\n    code\n",
		// Fenced code, no info, no body.
		"```\n```\n",
		// Fenced code with very long info string.
		"```" + strings.Repeat("a", 100) + "\nbody\n```\n",
		// HTML block conditions.
		"<table>\nbody\n</table>\n",
		"<!--\ncomment\n-->\n",
		"<![CDATA[content]]>\n",
		// Inline HTML.
		"text <span class=\"x\">inline</span> end\n",
		// Bare URLs (autolink).
		"<https://example.com/path?q=v&r=s>\n",
		// Block quote with leading spaces (allowed).
		"   > quoted\n",
		// Lists with various markers.
		"* star\n+ plus\n- dash\n",
		"1) paren ordered\n2) more\n",
		// Tight list interleaved.
		"- one\n- two\n  - nested\n  - more nested\n- three\n",
	}
	md := goldmark.New(goldmark.WithRendererOptions(html.WithUnsafe()))
	for i, src := range cases {
		var buf bytes.Buffer
		if err := md.Convert([]byte(src), &buf); err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
	}
}

func TestCorpus_DeepEdgeCases(t *testing.T) {
	// Last attempts at coverage edges.
	cases := []string{
		// Code block with tab + content following indented body.
		"    \t tab inside indented code\n",
		// Reference def with 999-char label (boundary).
		"[" + strings.Repeat("x", 999) + "]\n\n[" + strings.Repeat("x", 999) + "]: /url\n",
		// Reference def with 1000-char label (over boundary).
		"[" + strings.Repeat("y", 1000) + "]\n\n[" + strings.Repeat("y", 1000) + "]: /url\n",
		// Mixed tight/loose lists.
		"- a\n- b\n\n- new loose item\n\n- another loose\n",
		// Setext heading mid-list.
		"- list item\n\nH1\n===\n",
		// Heading with HTML inline.
		"# Heading with <span>HTML</span>\n",
		// Link inside link reference label (should NOT nest).
		"[outer[inner]label][ref]\n\n[ref]: /url\n",
		// Image alt with link.
		"![alt [linked alt](/l)](/img.png)\n",
		// Trailing backslash on last line.
		"text ending with backslash\\\n",
		// Newlines in different places.
		"para 1\n\n\n\npara 2 (multiple blanks)\n",
		// Indented blockquote.
		"   > quoted\n   > continued\n",
		// Empty fenced block info.
		"```   \nbody\n```\n",
		// Code fence using tildes.
		"~~~~~~\nlong fence\n~~~~~~\n",
		// Fence open then closed with different char count.
		"```\nbody\n``` extra after close\n",
		// Image with title.
		`![alt](/img.png "title")` + "\n",
		// Link with empty title.
		`[x](/u "")` + "\n",
		// Link destination containing parens.
		`[x](/u\(escaped\))` + "\n",
		// Underscore inside word (no emphasis).
		"foo_bar_baz qux\n",
		// Mixed underscores and asterisks.
		"_a *b_ c*\n",
		// HR with stars vs dashes vs underscores.
		"***\n",
		"---\n",
		"___\n",
		"* * *\n",
		"- - -\n",
		"_ _ _\n",
	}
	md := goldmark.New()
	for i, src := range cases {
		var buf bytes.Buffer
		if err := md.Convert([]byte(src), &buf); err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
	}
}

func TestCorpus_CRLFLineEndings(t *testing.T) {
	// Drive the \\r\\n line-break branches in parseBlock.
	cases := []string{
		"line one\r\nline two\r\n",
		"text  \r\nhard break\r\n",                          // [space][space]\r\n
		"text\\\r\nbackslash break\r\n",                     // \\\r\n
	}
	md := goldmark.New()
	for i, src := range cases {
		var buf bytes.Buffer
		if err := md.Convert([]byte(src), &buf); err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
	}
}

func TestCorpus_FootnoteAndSetextEdgeCases(t *testing.T) {
	cases := []string{
		// Multiple footnotes referenced multiple times.
		"see[^a][^b][^a][^c]\n\n[^a]: A body\n[^b]: B body\n[^c]: C body\n",
		// Footnote with nested formatting in body.
		"see[^x]\n\n[^x]: body with *emph* and **bold** and `code`\n",
		// Footnote definition body with multiple paragraphs.
		"see[^p]\n\n[^p]: first paragraph\n\n    second paragraph\n",
		// Setext heading at document start.
		"H1\n===\n",
		// Setext h1 immediately after h2.
		"H2\n---\nH1 below\n===\n",
		// Setext heading interrupting blockquote.
		"> Title\n> ====\n",
		// ATX heading with attributes followed by setext underline (rare).
		"# Title {#id}\n===\n",
	}
	md := goldmark.New(
		goldmark.WithExtensions(extension.Footnote),
		goldmark.WithParserOptions(parser.WithAutoHeadingID(), parser.WithAttribute()),
	)
	for i, src := range cases {
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
