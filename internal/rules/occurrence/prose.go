package occurrence

import (
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// proseParagraph is one counted paragraph: the 1-based source line it
// starts on, its plain text with inline code spans removed, and that
// text lowercased when the rule matches case-insensitively (else Text).
type proseParagraph struct {
	Line  int
	Text  string
	Lower string
	end   int // end offset of Text in the joined extraction buffer
}

// prose is the counting input for one Check: the paragraphs the rule
// counts and, for section scope, the headings that bound sections.
type prose struct {
	paras    []proseParagraph
	headings []astutil.SectionHeading
}

// collectProse walks the AST once and returns the paragraphs the rule
// counts, in document order: every paragraph outside a table (fenced
// and indented code never parse as paragraphs), the same selection as
// astutil.CollectSectionParagraphs. With wantHeadings it also returns
// every heading, as astutil.CollectSectionHeadings would. All texts
// are extracted into one buffer and sliced from a single string, and
// lowercasing runs once on that string, so the cost is a fixed handful
// of allocations whatever the paragraph count. Only MDS060 reads this
// view, so it is built per Check rather than memoized on the File.
func collectProse(f *lint.File, lower, wantHeadings bool) prose {
	w := proseWalker{
		f:            f,
		buf:          make([]byte, 0, len(f.Source)),
		paras:        make([]proseParagraph, 0, f.AST.ChildCount()),
		wantHeadings: wantHeadings,
	}
	w.walk(f.AST)
	paras := w.paras
	if len(paras) > 0 {
		joined := string(w.buf)
		lowered := joined
		if lower {
			lowered = strings.ToLower(joined)
		}
		start := 0
		for i := range paras {
			end := paras[i].end
			paras[i].Text = joined[start:end]
			switch {
			case !lower:
				paras[i].Lower = paras[i].Text
			case len(lowered) == len(joined):
				paras[i].Lower = lowered[start:end]
			default:
				// Lowercasing changed a byte length (e.g. U+0130), so
				// the joined offsets no longer apply to lowered.
				paras[i].Lower = strings.ToLower(paras[i].Text)
			}
			start = end
		}
	}
	return prose{paras: paras, headings: w.headings}
}

// proseWalker accumulates collectProse's single AST pass.
type proseWalker struct {
	f            *lint.File
	buf          []byte
	paras        []proseParagraph
	headings     []astutil.SectionHeading
	wantHeadings bool
}

// walk visits n's block children in document order, recording each
// non-table paragraph's prose text and, when wanted, each heading.
func (w *proseWalker) walk(n ast.Node) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *ast.Paragraph:
			if astutil.IsTable(v, w.f) {
				continue
			}
			w.buf = appendProse(w.buf, v, w.f.Source)
			w.paras = append(w.paras, proseParagraph{
				Line: astutil.ParagraphLine(v, w.f), end: len(w.buf),
			})
		case *ast.Heading:
			if w.wantHeadings {
				w.headings = append(w.headings, astutil.SectionHeading{
					Level: v.Level, Line: astutil.HeadingLine(v, w.f),
				})
			}
		default:
			w.walk(c)
		}
	}
}

// appendProse appends n's readable text to buf. It mirrors
// mdtext.ExtractPlainText — text and string nodes, link and emphasis
// text, image alt text — except that an inline code span is not prose
// and contributes a single space, so the words on either side cannot
// splice into a false match.
func appendProse(buf []byte, n ast.Node, source []byte) []byte {
	switch v := n.(type) {
	case *ast.Text:
		buf = append(buf, v.Segment.Value(source)...)
		if v.SoftLineBreak() || v.HardLineBreak() {
			buf = append(buf, ' ')
		}
		return buf
	case *ast.String:
		return append(buf, v.Value...)
	case *ast.CodeSpan:
		return append(buf, ' ')
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		buf = appendProse(buf, c, source)
	}
	return buf
}
