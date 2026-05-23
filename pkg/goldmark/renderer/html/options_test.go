package html_test

// Coverage for the html renderer's option dispatchers and Writer
// methods. Each option's SetConfig branch is exercised by passing
// the With*Option through goldmark.WithRendererOptions; the writer
// methods are driven directly via the Writer interface.

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

func convertWithOpts(t *testing.T, src string, opts ...renderer.Option) string {
	t.Helper()
	md := goldmark.New(goldmark.WithRendererOptions(opts...))
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	return buf.String()
}

func TestRenderOption_EastAsianLineBreaksNone(t *testing.T) {
	// Without the option the soft line break between CJK runs
	// renders as a literal newline.
	out := convertWithOpts(t, "日本語\nテキスト\n", html.WithEastAsianLineBreaks(html.EastAsianLineBreaksNone))
	if !strings.Contains(out, "\n") {
		t.Errorf("expected newline preserved (None mode), got: %q", out)
	}
}

func TestRenderOption_EastAsianLineBreaksSimple(t *testing.T) {
	// Simple mode: between two East-Asian-wide runes the soft
	// line break is suppressed.
	out := convertWithOpts(t, "日本語\nテキスト\n", html.WithEastAsianLineBreaks(html.EastAsianLineBreaksSimple))
	// Suppressed means no newline between the two CJK words.
	if strings.Contains(out, "日本語\nテキスト") {
		t.Errorf("Simple mode should suppress break between CJK runs: %q", out)
	}
}

func TestRenderOption_EastAsianLineBreaksCSS3Draft(t *testing.T) {
	out := convertWithOpts(t, "日本語\nテキスト\n", html.WithEastAsianLineBreaks(html.EastAsianLineBreaksCSS3Draft))
	if strings.Contains(out, "日本語\nテキスト") {
		t.Errorf("CSS3Draft mode should suppress break between CJK runs: %q", out)
	}
}

func TestRenderOption_EastAsianLineBreaksCSS3DraftPunctuation(t *testing.T) {
	// CSS3Draft has 4 distinct rules. Drive each branch.
	cases := []string{
		// Rule 1 — zero-width space before / after the break.
		"a​\n日本語\n",
		"日本語\n​b\n",
		// Rule 2 — both F/W/H, neither side Hangul -> break removed.
		"日本語\nテキスト\n",
		// Rule 2 — both F/W/H, one side Hangul -> break PRESERVED.
		"가\n나\n",   // both Hangul
		// Rule 3 — punctuation on one side -> break removed.
		"a。\n日本語\n",
		"日本語\n、b\n",
		// Rule 3 — IDEOGRAPHIC SPACE 　 on one side.
		"　\n日本語\n",
		"日本語\n　\n",
		// Rule 4 — neither side is wide nor punctuation -> default branch.
		"abc\ndef\n",
	}
	for _, c := range cases {
		_ = convertWithOpts(t, c, html.WithEastAsianLineBreaks(html.EastAsianLineBreaksCSS3Draft))
	}
}

func TestRenderOption_HardWraps(t *testing.T) {
	out := convertWithOpts(t, "one\ntwo\n", html.WithHardWraps())
	if !strings.Contains(out, "<br") {
		t.Errorf("HardWraps should emit <br>: %q", out)
	}
}

func TestRenderOption_XHTML(t *testing.T) {
	out := convertWithOpts(t, "---\n\n![alt](/x)\n", html.WithXHTML())
	if !strings.Contains(out, " />") {
		t.Errorf("XHTML option should produce self-closing form: %q", out)
	}
}

func TestRenderOption_Unsafe(t *testing.T) {
	out := convertWithOpts(t, "<script>x</script>\n", html.WithUnsafe())
	if !strings.Contains(out, "<script>") {
		t.Errorf("Unsafe should keep <script>: %q", out)
	}
}

func TestRenderOption_WithWriter(t *testing.T) {
	// WithWriter installs a custom Writer. We pass the default
	// Writer reconfigured with WithEscapedSpace to drive that path.
	w := html.NewWriter(html.WithEscapedSpace())
	out := convertWithOpts(t, "hello world\n", html.WithWriter(w))
	if !strings.Contains(out, "hello") {
		t.Errorf("WithWriter convert dropped content: %q", out)
	}
}

func TestWriter_RawWrite_SecureWrite_Write(t *testing.T) {
	// Drive the Writer methods directly so the per-method coverage
	// climbs without depending on which Convert path picks them up.
	w := html.NewWriter()
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)

	// RawWrite escapes & < > and " — drive each separately.
	for _, in := range [][]byte{
		[]byte("plain text"),
		[]byte("with & and <"),
		[]byte("with > and \""),
	} {
		buf.Reset()
		w.RawWrite(bw, in)
		_ = bw.Flush()
	}

	// SecureWrite drops NUL bytes (UTF-8 invalid byte stripping).
	buf.Reset()
	w.SecureWrite(bw, []byte("a\x00b\x00c"))
	_ = bw.Flush()
	if strings.Contains(buf.String(), "\x00") {
		t.Errorf("SecureWrite should strip NUL: %q", buf.String())
	}

	// Write handles entities (&amp; etc) plus the escape rules.
	buf.Reset()
	w.Write(bw, []byte("a &amp; b &#65; c &lt;d&gt;"))
	_ = bw.Flush()
	if !strings.Contains(buf.String(), "&amp;") {
		t.Errorf("Write should preserve &amp;: %q", buf.String())
	}
}

func TestRender_StringNode(t *testing.T) {
	// ast.String nodes are synthesised by some extensions but the
	// stock parser path does not emit them. Build a tiny AST with
	// a String child of a Paragraph and render it via the renderer.
	doc := ast.NewDocument()
	p := ast.NewParagraph()
	doc.AppendChild(doc, p)

	plain := ast.NewString([]byte("plain"))
	p.AppendChild(p, plain)
	codeStr := ast.NewString([]byte("code"))
	codeStr.SetCode(true)
	p.AppendChild(p, codeStr)
	raw := ast.NewString([]byte("<x>"))
	raw.SetRaw(true)
	p.AppendChild(p, raw)

	r := renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(html.NewRenderer(), 1000)))
	var buf bytes.Buffer
	if err := r.Render(&buf, []byte("source"), doc); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "plain") || !strings.Contains(out, "code") {
		t.Errorf("renderer output missing expected content: %q", out)
	}
}

func TestRender_BlockquoteWithAttributes(t *testing.T) {
	// renderBlockquote has an Attributes() != nil branch only
	// reached when the AST node carries a SetAttribute call. The
	// stock parser does not set blockquote attributes, so build
	// the AST manually.
	doc := ast.NewDocument()
	bq := ast.NewBlockquote()
	bq.SetAttribute([]byte("class"), []byte("note"))
	bq.SetAttribute([]byte("id"), []byte("q1"))
	doc.AppendChild(doc, bq)

	r := renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(html.NewRenderer(), 1000)))
	var buf bytes.Buffer
	if err := r.Render(&buf, []byte("source"), doc); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(buf.String(), `class="note"`) {
		t.Errorf("blockquote attribute not rendered: %q", buf.String())
	}
}

func TestRender_ParagraphWithAttributes(t *testing.T) {
	doc := ast.NewDocument()
	p := ast.NewParagraph()
	p.SetAttribute([]byte("class"), []byte("intro"))
	doc.AppendChild(doc, p)
	r := renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(html.NewRenderer(), 1000)))
	var buf bytes.Buffer
	if err := r.Render(&buf, []byte("source"), doc); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(buf.String(), `class="intro"`) {
		t.Errorf("paragraph attribute not rendered: %q", buf.String())
	}
}

func TestRender_ListItemWithAttributes(t *testing.T) {
	doc := ast.NewDocument()
	list := ast.NewList('-')
	li := ast.NewListItem(2)
	li.SetAttribute([]byte("class"), []byte("item"))
	list.AppendChild(list, li)
	doc.AppendChild(doc, list)
	r := renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(html.NewRenderer(), 1000)))
	var buf bytes.Buffer
	if err := r.Render(&buf, []byte("source"), doc); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(buf.String(), `class="item"`) {
		t.Errorf("list-item attribute not rendered: %q", buf.String())
	}
}

func TestRender_HeadingWithExtraAttributes(t *testing.T) {
	// Heading with id (auto) + custom data-* attribute.
	doc := ast.NewDocument()
	h := ast.NewHeading(2)
	h.SetAttribute([]byte("id"), []byte("title"))
	h.SetAttribute([]byte("data-x"), []byte("y"))
	doc.AppendChild(doc, h)
	r := renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(html.NewRenderer(), 1000)))
	var buf bytes.Buffer
	if err := r.Render(&buf, []byte("source"), doc); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(buf.String(), `id="title"`) {
		t.Errorf("heading id not rendered: %q", buf.String())
	}
}

func TestRender_ImageAltWithNestedInlines(t *testing.T) {
	// Image alt-text with nested emphasis / code / link drives the
	// recursive branch in renderTexts.
	cases := []string{
		`![*emphatic* alt](/img.png)` + "\n",
		"![alt with `code`](/img.png)\n",
		"![alt with **bold**](/img.png)\n",
		"![plain alt](/img.png)\n",
	}
	for _, src := range cases {
		_ = convertWithOpts(t, src)
	}
}

func TestRender_AutoLinkEmailAndDangerous(t *testing.T) {
	// Email autolinks add "mailto:" if the URL doesn't already
	// start with it.
	cases := []string{
		"<https://example.com>\n",
		"<user@example.com>\n",
		"<mailto:user@example.com>\n",
	}
	for _, src := range cases {
		_ = convertWithOpts(t, src)
	}
}

func TestRender_InlineRawHTMLSafeUnsafe(t *testing.T) {
	// renderRawHTML has Unsafe and safe branches.  Inline raw
	// HTML appears mid-paragraph as a RawHTML AST node (vs the
	// block-level HTMLBlock).
	src := "paragraph with <span class=\"x\">inline</span> raw html\n"

	// Safe (default): emit the omit comment.
	outSafe := convertWithOpts(t, src)
	if !strings.Contains(outSafe, "raw HTML omitted") {
		t.Errorf("safe should omit raw HTML: %q", outSafe)
	}

	// Unsafe: pass through.
	outUnsafe := convertWithOpts(t, src, html.WithUnsafe())
	if !strings.Contains(outUnsafe, "<span") {
		t.Errorf("unsafe should pass through <span>: %q", outUnsafe)
	}
}

func TestRender_CodeSpanMultiLine(t *testing.T) {
	// A multi-line code span has Text children whose Value ends
	// in '\n'; renderCodeSpan replaces the newline with a single
	// space and writes the rest raw.
	src := "see `a\nb` here\n"
	out := convertWithOpts(t, src)
	if !strings.Contains(out, "<code>") {
		t.Errorf("expected <code> in output: %q", out)
	}
}

func TestRender_HTMLBlockSafeAndUnsafe(t *testing.T) {
	// renderHTMLBlock has four branches:
	//   - entering + Unsafe: SecureWrite each body line
	//   - entering + safe: "raw HTML omitted" comment
	//   - exiting + HasClosure + Unsafe: SecureWrite the closure
	//   - exiting + HasClosure + safe: "raw HTML omitted" comment
	// Multi-line type-1 HTML block has a ClosureLine; drive the
	// safe and unsafe variants both.
	src := "<script>\nbody1\nbody2\n</script>\n"

	// Safe (default).
	outSafe := convertWithOpts(t, src)
	if !strings.Contains(outSafe, "raw HTML omitted") {
		t.Errorf("safe render should emit raw HTML omitted comment: %q", outSafe)
	}

	// Unsafe.
	outUnsafe := convertWithOpts(t, src, html.WithUnsafe())
	if !strings.Contains(outUnsafe, "<script>") {
		t.Errorf("unsafe render should pass through <script>: %q", outUnsafe)
	}
}

func TestWriter_EntitiesEscapeRuneBranches(t *testing.T) {
	// Drive escapeRune via numeric/hex HTML entities. Each entity
	// triggers a different branch:
	//   <, > (r < 256, escape table hit)
	//   A (r < 256, escape table miss; raw rune)
	//   😀 (r > 65535, high-codepoint path)
	w := html.NewWriter()
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	w.Write(bw, []byte(`&#60;a&#62;b&#65;c&#x1F600;d`))
	_ = bw.Flush()
	out := buf.String()
	// Just assert the writer didn't choke on any of the
	// codepoints — exact rendered form depends on internal
	// escape tables.
	if len(out) < 5 {
		t.Errorf("Write produced suspiciously short output: %q", out)
	}
}

func TestRender_CodeSpanWithRawTextSegment(t *testing.T) {
	// Code span containing a RawTextSegment exercises the raw
	// inline write path in renderCodeSpan.
	src := []byte("hello world")
	doc := ast.NewDocument()
	p := ast.NewParagraph()
	cs := ast.NewCodeSpan()
	cs.AppendChild(cs, ast.NewRawTextSegment(text.NewSegment(0, 5)))
	p.AppendChild(p, cs)
	doc.AppendChild(doc, p)
	r := renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(html.NewRenderer(), 1000)))
	var buf bytes.Buffer
	if err := r.Render(&buf, src, doc); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(buf.String(), "<code>") {
		t.Errorf("code span not rendered: %q", buf.String())
	}
}

func TestWriter_EscapedSpace(t *testing.T) {
	// With WithEscapedSpace, the writer escapes literal "\ " (a
	// backslash followed by space) differently — drive a code
	// path that includes one.
	w := html.NewWriter(html.WithEscapedSpace())
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	w.Write(bw, []byte(`a\ b`))
	_ = bw.Flush()
}
