package html_test

// Comprehensive HTML rendering corpus. Drives every node-renderer
// function in html.go through goldmark.Convert. Each snippet
// exercises a different render path; the test asserts the output
// contains a marker substring proving the renderer ran.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

func TestRenderCorpus_AllNodeTypes(t *testing.T) {
	cases := []struct {
		name   string
		src    string
		want   []string
		opts   []renderer.Option
		exts   []goldmark.Extender
	}{
		// Block-level renderers.
		{name: "Heading", src: "# H1\n## H2\n### H3\n#### H4\n##### H5\n###### H6\n",
			want: []string{"<h1>", "<h2>", "<h6>"}},
		{name: "Paragraph", src: "first paragraph\n\nsecond paragraph\n",
			want: []string{"<p>", "</p>"}},
		{name: "Blockquote", src: "> quoted\n",
			want: []string{"<blockquote>"}},
		{name: "ThematicBreak", src: "before\n\n---\n\nafter\n",
			want: []string{"<hr"}},
		{name: "ThematicBreak-default", src: "---\n",
			want: []string{"<hr>"}},
		{name: "CodeBlock-indented", src: "    code\n    more code\n",
			want: []string{"<pre><code>"}},
		{name: "CodeBlock-fenced-info", src: "```go\nfn()\n```\n",
			want: []string{"<pre><code class=\"language-go\""}},
		{name: "List-unordered", src: "- one\n- two\n",
			want: []string{"<ul>", "<li>"}},
		{name: "List-ordered", src: "1. one\n2. two\n",
			want: []string{"<ol>", "<li>"}},
		{name: "List-ordered-start", src: "5. five\n6. six\n",
			want: []string{`<ol start="5">`}},
		// Inline renderers.
		{name: "Emphasis", src: "*em* and **strong**\n",
			want: []string{"<em>", "<strong>"}},
		{name: "CodeSpan", src: "use `c` here\n",
			want: []string{"<code>"}},
		{name: "Link", src: "see [text](/url \"title\")\n",
			want: []string{`<a href="/url" title="title">`}},
		{name: "Link-no-title", src: "see [text](/url)\n",
			want: []string{`<a href="/url">`}},
		{name: "Image", src: "see ![alt](/img.png \"title\")\n",
			want: []string{`<img src="/img.png"`, `alt="alt"`, `title="title"`}},
		{name: "AutoLink-url", src: "<https://example.com>\n",
			want: []string{`<a href="https://example.com">`}},
		{name: "AutoLink-email", src: "<a@example.com>\n",
			want: []string{`<a href="mailto:a@example.com">`}},
		{name: "RawHTML-inline", src: "see <span>X</span>\n", opts: []renderer.Option{html.WithUnsafe()},
			want: []string{"<span>", "</span>"}},
		{name: "RawHTML-block-unsafe", src: "<div>\nblock\n</div>\n", opts: []renderer.Option{html.WithUnsafe()},
			want: []string{"<div>"}},
		// Safety: dangerous URLs get blanked (href="" rather than the input).
		{name: "DangerousURL-javascript", src: "[x](javascript:alert(1))\n",
			want: []string{`<a href="">`}},
		// Hard line break (two trailing spaces).
		{name: "HardBreak-spaces", src: "first  \nsecond\n",
			want: []string{"<br"}},
		// Hard line break via backslash.
		{name: "HardBreak-backslash", src: "first\\\nsecond\n",
			want: []string{"<br"}},
		// Extensions wired through render.
		{name: "Table", src: "| h |\n|---|\n| c |\n", exts: []goldmark.Extender{extension.Table},
			want: []string{"<table>", "<th>", "<td>"}},
		{name: "Table-align-left", src: "| h |\n|:--|\n| c |\n", exts: []goldmark.Extender{extension.Table},
			want: []string{"text-align:left"}},
		{name: "Strikethrough", src: "this is ~~struck~~ out\n", exts: []goldmark.Extender{extension.Strikethrough},
			want: []string{"<del>"}},
		{name: "TaskList", src: "- [x] done\n- [ ] todo\n", exts: []goldmark.Extender{extension.TaskList},
			want: []string{"checkbox"}},
		{name: "DefinitionList", src: "Term\n: Def\n", exts: []goldmark.Extender{extension.DefinitionList},
			want: []string{"<dl>", "<dt>", "<dd>"}},
		{name: "Footnote", src: "x[^1]\n\n[^1]: body\n", exts: []goldmark.Extender{extension.Footnote},
			want: []string{`class="footnote-ref"`, `class="footnotes"`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			optsList := []goldmark.Option{}
			if len(tc.exts) > 0 {
				optsList = append(optsList, goldmark.WithExtensions(tc.exts...))
			}
			if len(tc.opts) > 0 {
				optsList = append(optsList, goldmark.WithRendererOptions(tc.opts...))
			}
			md := goldmark.New(optsList...)
			var buf bytes.Buffer
			if err := md.Convert([]byte(tc.src), &buf); err != nil {
				t.Fatalf("Convert: %v", err)
			}
			out := buf.String()
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("rendered output missing %q\ngot: %s", w, out)
				}
			}
		})
	}
}

func TestRender_XHTMLMode(t *testing.T) {
	// XHTML mode emits self-closing tags with `/>`.
	r := renderer.NewRenderer(
		renderer.WithNodeRenderers(util.Prioritized(html.NewRenderer(html.WithXHTML()), 1000)),
	)
	md := goldmark.New(goldmark.WithRenderer(r))
	var buf bytes.Buffer
	if err := md.Convert([]byte("---\n\n![alt](/x)\n"), &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !strings.Contains(buf.String(), " />") {
		t.Errorf("XHTML mode missing self-closing form: %s", buf.String())
	}
}

func TestRender_HardWraps(t *testing.T) {
	r := renderer.NewRenderer(
		renderer.WithNodeRenderers(util.Prioritized(html.NewRenderer(html.WithHardWraps()), 1000)),
	)
	md := goldmark.New(goldmark.WithRenderer(r))
	var buf bytes.Buffer
	if err := md.Convert([]byte("first\nsecond\n"), &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !strings.Contains(buf.String(), "<br") {
		t.Errorf("HardWraps did not emit <br>: %s", buf.String())
	}
}

func TestRender_Unsafe(t *testing.T) {
	// Without WithUnsafe, raw HTML is escaped.
	mdSafe := goldmark.New()
	var bufSafe bytes.Buffer
	if err := mdSafe.Convert([]byte("<script>x</script>\n"), &bufSafe); err != nil {
		t.Fatalf("safe Convert: %v", err)
	}
	if strings.Contains(bufSafe.String(), "<script>") {
		t.Error("safe mode should escape <script>")
	}
	// With WithUnsafe, raw HTML passes through.
	r := renderer.NewRenderer(
		renderer.WithNodeRenderers(util.Prioritized(html.NewRenderer(html.WithUnsafe()), 1000)),
	)
	mdUnsafe := goldmark.New(goldmark.WithRenderer(r))
	var bufUnsafe bytes.Buffer
	if err := mdUnsafe.Convert([]byte("<script>x</script>\n"), &bufUnsafe); err != nil {
		t.Fatalf("unsafe Convert: %v", err)
	}
	if !strings.Contains(bufUnsafe.String(), "<script>") {
		t.Error("unsafe mode should keep <script>")
	}
}
