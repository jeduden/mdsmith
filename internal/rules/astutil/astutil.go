package astutil

import (
	"bytes"
	"sort"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/yuin/goldmark/ast"
)

// SectionHeading is a heading discovered by CollectSectionHeadings,
// carrying the level and source line needed to compute a section's
// body range.
type SectionHeading struct {
	Level int
	Line  int
}

// SectionParagraph is a non-table paragraph discovered by
// CollectSectionParagraphs. Line is the 1-based source line; Node is
// the goldmark paragraph node, used by [SectionParagraph.ExtractText]
// to materialise the plain text lazily.
//
// Text is a documented cache: CollectSectionParagraphs no longer
// fills it (plan 196 — most callers do not need the text on every
// paragraph), but test literals can still set it directly without
// building an AST node, and ExtractText prefers the cached value
// when present. Production code reaches the text through
// ExtractText, never the field; the field is kept exported to keep
// existing literals compiling.
//
// HasText flags Text as an authoritative cache, including the
// legitimately-empty case (an image-only paragraph extracts to
// ""). [CollectSectionParagraphsWithText] sets it so per-heading
// SectionBody sweeps hit the cache for every paragraph,
// regardless of whether the extracted text is empty.
type SectionParagraph struct {
	Line    int
	Node    ast.Node
	Text    string
	HasText bool
}

// ExtractText returns the paragraph's plain text. If HasText is
// set the cached Text is returned verbatim (including the empty
// string for image-only paragraphs). Otherwise, a non-empty Text
// short-circuit handles test literals built without an AST node,
// and the final fallback extracts from Node against source.
//
// Precondition: at least one of Text/HasText or Node must be set.
// Calling on a zero-value SectionParagraph (no Text, no Node)
// panics inside [mdtext.ExtractPlainText]'s nil-node dereference.
// Production paragraphs from [CollectSectionParagraphs] always
// have Node set; test literals set Text and hit the shortcut.
func (p SectionParagraph) ExtractText(source []byte) string {
	if p.HasText {
		return p.Text
	}
	if p.Text != "" {
		return p.Text
	}
	return mdtext.ExtractPlainText(p.Node, source)
}

// CollectSectionHeadings returns every heading in the document
// ordered by source line. Used by content rules (MDS057, MDS058)
// that need to walk heading-bounded sections.
func CollectSectionHeadings(f *lint.File) []SectionHeading {
	var out []SectionHeading
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		out = append(out, SectionHeading{
			Level: h.Level,
			Line:  HeadingLine(h, f),
		})
		return ast.WalkSkipChildren, nil
	})
	sort.Slice(out, func(i, j int) bool {
		return out[i].Line < out[j].Line
	})
	return out
}

// CollectSectionParagraphs returns every non-table paragraph with its
// 1-based source line and a reference to its AST node. Goldmark
// parses pipe-delimited tables as paragraphs when the table
// extension is absent; those are filtered so cell text does not
// pollute section bodies.
//
// Memoized per File via lint.File.MemoFile (the *File-passing
// variant of Memo): the AST walk is shared across the prose rules
// (MDS023 paragraph-readability, MDS024 paragraph-structure, MDS057
// required-text-patterns, MDS058 required-mentions). The result is
// a pure function of the immutable AST and Source; the memo lives
// on the per-Check File, so nothing is cached across files or runs.
// Callers treat the slice as read-only.
//
// Plan 196 made the extracted text lazy: the per-paragraph
// [mdtext.ExtractPlainText] call no longer runs in the walk. Rules
// that need the text reach it via
// [SectionParagraph.ExtractText]; paragraph-readability, the
// default-on prose rule, gates on word count alone via
// [mdtext.CountWordsInNode] and only materialises text for
// paragraphs that pass minWords.
//
// The MemoFile variant lets buildSectionParagraphs be a package-
// level function instead of a closure, so the build itself adds no
// per-call allocation beyond what the function body does.
func CollectSectionParagraphs(f *lint.File) []SectionParagraph {
	return f.MemoFile("astutil.sectionParagraphs", buildSectionParagraphs).([]SectionParagraph)
}

// buildSectionParagraphs is the MemoFile-style builder for the
// section-paragraphs memo. Defined at package scope so the value
// passed to MemoFile is a plain function pointer (no closure
// capturing `f`), avoiding the per-call closure allocation a
// `func() any { … }` literal would force.
func buildSectionParagraphs(f *lint.File) any {
	var out []SectionParagraph
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		p, ok := n.(*ast.Paragraph)
		if !ok {
			return ast.WalkContinue, nil
		}
		if IsTable(p, f) {
			return ast.WalkContinue, nil
		}
		out = append(out, SectionParagraph{
			Line: ParagraphLine(p, f),
			Node: p,
		})
		return ast.WalkContinue, nil
	})
	return out
}

// CollectSectionParagraphsWithText returns the same SectionParagraphs
// as [CollectSectionParagraphs] but with Text populated for every
// entry. Memoized per File so MDS057 and MDS058 (and any future rule
// that builds section bodies from paragraph text) share a single
// materialisation even when both rules are enabled — without this,
// each paragraph nested inside multiple overlapping section ranges
// (h1 > h2 > h3) would re-run [mdtext.ExtractPlainText] once per
// containing heading.
//
// Use this when a rule needs paragraph text for every paragraph in
// the file. Do NOT use it from the default-on MDS023
// paragraph-readability rule — that one filters most paragraphs out
// before any text is needed, which is the point of plan 196's lazy
// design. The returned slice is a copy of the
// [CollectSectionParagraphs] memo's slice (with Text filled in);
// callers treat it as read-only.
func CollectSectionParagraphsWithText(f *lint.File) []SectionParagraph {
	return f.MemoFile("astutil.sectionParagraphsWithText", buildSectionParagraphsWithText).([]SectionParagraph)
}

// buildSectionParagraphsWithText materialises Text on every paragraph
// returned by [CollectSectionParagraphs]. Built on top of the
// table-filtered memo so the AST walk runs once even when both memos
// are accessed. The upstream collector guarantees Text is empty on
// every entry, so this builder unconditionally fills it and sets
// HasText so subsequent ExtractText calls hit the cache even when
// the extracted text is legitimately empty.
func buildSectionParagraphsWithText(f *lint.File) any {
	src := CollectSectionParagraphs(f)
	out := make([]SectionParagraph, len(src))
	for i, p := range src {
		out[i] = p
		out[i].Text = mdtext.ExtractPlainText(p.Node, f.Source)
		out[i].HasText = true
	}
	return out
}

// SectionEnd returns the exclusive end line of the section starting
// at headings[i]. The section ends at the first heading at the same
// or shallower level after headings[i], or at totalLines+1 when no
// such heading exists. Nested sub-sections stay inside.
func SectionEnd(headings []SectionHeading, i, totalLines int) int {
	for j := i + 1; j < len(headings); j++ {
		if headings[j].Level <= headings[i].Level {
			return headings[j].Line
		}
	}
	return totalLines + 1
}

// SectionBody concatenates paragraph plain text for paragraphs whose
// start line falls in [start, end). Joins with a space so adjacent
// paragraphs do not appear glued together to a substring/regex
// matcher. The source byte slice is required because
// SectionParagraph's text is materialised lazily through
// [SectionParagraph.ExtractText] (plan 196); callers pass f.Source.
func SectionBody(paragraphs []SectionParagraph, source []byte, start, end int) string {
	var parts []string
	for _, p := range paragraphs {
		if p.Line < start || p.Line >= end {
			continue
		}
		parts = append(parts, p.ExtractText(source))
	}
	return strings.Join(parts, " ")
}

// HeadingLine returns the 1-based source line of a heading node.
// Setext headings expose their line via Lines(); ATX headings are found
// by walking inline descendants until the first text segment. Returns 1
// as a safe fallback.
func HeadingLine(heading *ast.Heading, f *lint.File) int {
	lines := heading.Lines()
	if lines.Len() > 0 {
		return f.LineOfOffset(lines.At(0).Start)
	}

	line := 1
	_ = ast.Walk(heading, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || n == heading {
			return ast.WalkContinue, nil
		}
		t, ok := n.(*ast.Text)
		if !ok {
			return ast.WalkContinue, nil
		}
		line = f.LineOfOffset(t.Segment.Start)
		return ast.WalkStop, nil
	})

	return line
}

// ParagraphLine returns the 1-based source line of a paragraph node.
func ParagraphLine(para *ast.Paragraph, f *lint.File) int {
	lines := para.Lines()
	if lines.Len() > 0 {
		return f.LineOfOffset(lines.At(0).Start)
	}
	return 1
}

// IsTable reports whether a paragraph node is actually a GFM table
// (goldmark parses tables as paragraphs when the table extension is
// absent).  It checks whether the first line starts with "|".
func IsTable(para *ast.Paragraph, f *lint.File) bool {
	lines := para.Lines()
	if lines.Len() == 0 {
		return false
	}
	seg := lines.At(0)
	return bytes.HasPrefix(bytes.TrimSpace(f.Source[seg.Start:seg.Stop]), []byte("|"))
}

// HeadingText returns the plain-text content of a heading by
// recursively extracting all text segments from its children.
func HeadingText(heading *ast.Heading, source []byte) string {
	var buf bytes.Buffer
	for c := heading.FirstChild(); c != nil; c = c.NextSibling() {
		ExtractText(c, source, &buf)
	}
	return buf.String()
}

// ExtractText recursively writes the text content of n and its
// descendants into buf.
func ExtractText(n ast.Node, source []byte, buf *bytes.Buffer) {
	if t, ok := n.(*ast.Text); ok {
		buf.Write(t.Segment.Value(source))
		return
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		ExtractText(c, source, buf)
	}
}
