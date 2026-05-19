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
// CollectSectionParagraphs, carrying its 1-based source line and the
// plain text used for section-wide body matches.
type SectionParagraph struct {
	Line int
	Text string
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
// 1-based source line and plain text. Goldmark parses pipe-delimited
// tables as paragraphs when the table extension is absent; those are
// filtered so cell text does not pollute section bodies.
func CollectSectionParagraphs(f *lint.File) []SectionParagraph {
	// Memoized per File: this walks the whole AST and runs
	// ExtractPlainText on every paragraph. On prose-heavy input the
	// two hot default rules (MDS024 paragraph-structure and
	// paragraph-readability) plus the required-* rules each called
	// it, so the walk and per-paragraph extraction ran several times
	// per file. The result is a pure function of the immutable AST
	// and Source; the memo lives on the per-Check File, so nothing
	// is cached across files or runs — the same scope and staleness
	// guarantees as the per-File code-block-line cache. Callers
	// treat the slice as read-only.
	return f.Memo("astutil.sectionParagraphs", func() any {
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
				Text: mdtext.ExtractPlainText(p, f.Source),
			})
			return ast.WalkContinue, nil
		})
		return out
	}).([]SectionParagraph)
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
// matcher.
func SectionBody(paragraphs []SectionParagraph, start, end int) string {
	var parts []string
	for _, p := range paragraphs {
		if p.Line < start || p.Line >= end {
			continue
		}
		parts = append(parts, p.Text)
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
