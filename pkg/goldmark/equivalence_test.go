package goldmark_test

// The equivalence harness diffs the arena and non-arena parser
// paths on the same input. Two Markdown stacks are built — one
// with the per-parse arena (the canonical fork), one with
// parser.WithNoArena() — and the rendered HTML plus a structural
// summary of the AST are compared for every fixture in the
// corpora below. Plan 198's task 6 calls for this harness as the
// gate on every arena change.
//
// Coverage strategy: the comprehensive corpora in
// comprehensive_test.go and markdown_test.go already exercise the
// rare branches; the harness piggy-backs on them by re-using their
// raw Markdown strings. The CI workflow also runs every test
// under `-tags goldmark_upstream`, which forces newArenaForParse
// to return nil — orthogonal to WithNoArena but a second axis of
// equivalence.

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

// equivalencePair returns two configured Markdown stacks: one with
// the per-parse arena (the canonical path) and one explicitly
// opted out via WithNoArena. Both share the same extensions and
// renderer options so any output difference is attributable to the
// arena, not configuration drift.
func equivalencePair() (withArena, withoutArena goldmark.Markdown) {
	common := []goldmark.Option{
		goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.TaskList),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	}
	withArena = goldmark.New(common...)
	withoutArena = goldmark.New(append(common, goldmark.WithParserOptions(parser.WithNoArena()))...)
	return withArena, withoutArena
}

// summarizeAST walks the tree and produces a deterministic string
// that captures kind, position, and (for *ast.Text) the rendered
// segment text plus the publicly-readable flag bits (Raw,
// SoftLineBreak, HardLineBreak). The flags matter because two ASTs
// can render to identical HTML for a given input but disagree on a
// flag whose effect is only visible under different content — e.g.
// a Raw difference is invisible if the segment has no escapable
// bytes. Including the flags makes the harness a true
// AST-equivalence gate, not just a rendered-HTML gate. (ast.Text
// does not expose an IsCode accessor in the fork, so the Code flag
// stays out of the summary.)
func summarizeAST(root ast.Node, source []byte) string {
	var b strings.Builder
	var walk func(n ast.Node, depth int)
	walk = func(n ast.Node, depth int) {
		fmt.Fprintf(&b, "%s%s pos=%d", strings.Repeat("  ", depth), n.Kind(), n.Pos())
		if t, ok := n.(*ast.Text); ok {
			fmt.Fprintf(&b, " text=%q seg=%d:%d raw=%v soft=%v hard=%v",
				string(t.Segment.Value(source)),
				t.Segment.Start, t.Segment.Stop,
				t.IsRaw(),
				t.SoftLineBreak(), t.HardLineBreak())
		}
		b.WriteByte('\n')
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c, depth+1)
		}
	}
	walk(root, 0)
	return b.String()
}

// equivalenceCorpus is the set of inputs the harness diffs through
// both paths. Each entry is a name + source. Add entries here when
// a new failure mode is discovered.
var equivalenceCorpus = []struct {
	name string
	src  string
}{
	{"empty", ""},
	{"single-paragraph", "Hello world\n"},
	{"two-paragraphs", "First paragraph.\n\nSecond paragraph.\n"},
	{"headings", "# H1\n\n## H2\n\n### H3\n"},
	{"setext-heading", "Title\n=====\n\nSubtitle\n--------\n"},
	{"emphasis-and-strong", "Some *italic* and **bold** and ***both*** and ~~strike~~.\n"},
	{"code-span", "Use `fmt.Sprintf` for formatting.\n"},
	{"link-inline", "See [the docs](https://example.com).\n"},
	{"link-ref", "See [the docs][ref].\n\n[ref]: https://example.com\n"},
	{"autolink", "Visit <https://example.com> today.\n"},
	{"blockquote", "> First\n> Second\n>\n> Third\n"},
	{"list-tight", "- One\n- Two\n- Three\n"},
	{"list-loose", "- One\n\n- Two\n\n- Three\n"},
	{"ordered-list", "1. First\n2. Second\n3. Third\n"},
	{"fenced-code", "```go\nfunc main() {}\n```\n"},
	{"indented-code", "    code line one\n    code line two\n"},
	{"thematic-break", "Above\n\n---\n\nBelow\n"},
	{"table", "| A | B |\n|---|---|\n| 1 | 2 |\n| 3 | 4 |\n"},
	{"task-list", "- [ ] todo\n- [x] done\n"},
	{"strikethrough", "Use ~~old~~ new style.\n"},
	{"nested-emphasis", "*nested **bold** inside* and **strong *em* inside**.\n"},
	{"raw-html", "<div class=\"x\">\ninline <em>html</em> inside\n</div>\n"},
	{"escaped-chars", `Brackets \[escaped\] and backslash \\ here.` + "\n"},
	{"long-paragraph", strings.Repeat("Lorem ipsum dolor sit amet. ", 80) + "\n"},
	{"many-paragraphs", strings.Repeat("A short paragraph here.\n\n", 30)},
	{"deep-nesting", "> > > > triple nest\n>\n> > > out one\n"},
}

// TestEquivalence_ArenaVsNoArena_HTML diffs rendered HTML for the
// corpus. A regression in arena allocation that affects rendering
// (e.g. a Segment with the wrong range, a Text node carrying the
// wrong segment) surfaces as an HTML diff.
func TestEquivalence_ArenaVsNoArena_HTML(t *testing.T) {
	withArena, withoutArena := equivalencePair()
	for _, tc := range equivalenceCorpus {
		t.Run(tc.name, func(t *testing.T) {
			var arenaOut, plainOut bytes.Buffer
			if err := withArena.Convert([]byte(tc.src), &arenaOut); err != nil {
				t.Fatalf("arena Convert: %v", err)
			}
			if err := withoutArena.Convert([]byte(tc.src), &plainOut); err != nil {
				t.Fatalf("plain Convert: %v", err)
			}
			if !bytes.Equal(arenaOut.Bytes(), plainOut.Bytes()) {
				t.Errorf("HTML diverged.\nArena:\n%s\nPlain:\n%s", arenaOut.String(), plainOut.String())
			}
		})
	}
}

// TestEquivalence_ArenaVsNoArena_AST diffs the AST structural
// summary. Renders are byte-identical doesn't imply ASTs are —
// e.g. internal node ordering or position fields could drift
// without affecting HTML — so the AST check is the strictest gate.
func TestEquivalence_ArenaVsNoArena_AST(t *testing.T) {
	withArena, withoutArena := equivalencePair()
	for _, tc := range equivalenceCorpus {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.src)
			arenaAST := withArena.Parser().Parse(text.NewReader(src))
			plainAST := withoutArena.Parser().Parse(text.NewReader(src))
			arenaSummary := summarizeAST(arenaAST, src)
			plainSummary := summarizeAST(plainAST, src)
			if arenaSummary != plainSummary {
				t.Errorf("AST diverged.\nArena:\n%s\nPlain:\n%s", arenaSummary, plainSummary)
			}
		})
	}
}

// TestEquivalence_ComprehensiveCorpus diffs the long
// comprehensiveCorpus string (defined in comprehensive_test.go)
// through both paths. It covers far more shapes than the small
// corpus above and is the most rigorous check in the harness.
func TestEquivalence_ComprehensiveCorpus(t *testing.T) {
	withArena, withoutArena := equivalencePair()
	src := []byte(comprehensiveCorpus)
	var arenaOut, plainOut bytes.Buffer
	if err := withArena.Convert(src, &arenaOut); err != nil {
		t.Fatalf("arena Convert: %v", err)
	}
	if err := withoutArena.Convert(src, &plainOut); err != nil {
		t.Fatalf("plain Convert: %v", err)
	}
	if !bytes.Equal(arenaOut.Bytes(), plainOut.Bytes()) {
		t.Errorf("HTML diverged on comprehensive corpus")
	}
	arenaAST := withArena.Parser().Parse(text.NewReader(src))
	plainAST := withoutArena.Parser().Parse(text.NewReader(src))
	arenaSummary := summarizeAST(arenaAST, src)
	plainSummary := summarizeAST(plainAST, src)
	if arenaSummary != plainSummary {
		t.Errorf("AST diverged on comprehensive corpus")
	}
}

// TestEquivalence_ReuseParserSurvivesPriorAST guards against the
// hazard that plan 198's risk section called out: with a pooled
// parser, a second Parse on the same parser could clobber the
// first AST when an arena was reused. The canonical fork allocates
// a fresh arena per Parse so the first AST stays readable after
// the second Parse — this test enforces that invariant by reading
// both ASTs after both parses complete.
func TestEquivalence_ReuseParserSurvivesPriorAST(t *testing.T) {
	md := goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.TaskList))
	p := md.Parser()
	srcA := []byte("# Heading A\n\nBody of doc A.\n")
	srcB := []byte("# Heading B\n\nBody of doc B.\n")

	astA := p.Parse(text.NewReader(srcA))
	astB := p.Parse(text.NewReader(srcB))

	headingTextA := readFirstHeadingText(astA, srcA)
	headingTextB := readFirstHeadingText(astB, srcB)

	if headingTextA != "Heading A" {
		t.Errorf("doc A heading text corrupted by doc B parse: got %q", headingTextA)
	}
	if headingTextB != "Heading B" {
		t.Errorf("doc B heading text wrong: got %q", headingTextB)
	}
}

// readFirstHeadingText returns the rendered text of the first
// heading in root, scanning siblings of the document root.
func readFirstHeadingText(root ast.Node, source []byte) string {
	for c := root.FirstChild(); c != nil; c = c.NextSibling() {
		if c.Kind() != ast.KindHeading {
			continue
		}
		var b strings.Builder
		for tc := c.FirstChild(); tc != nil; tc = tc.NextSibling() {
			if t, ok := tc.(*ast.Text); ok {
				b.Write(t.Segment.Value(source))
			}
		}
		return b.String()
	}
	return ""
}
