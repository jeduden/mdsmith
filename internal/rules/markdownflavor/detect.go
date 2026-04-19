package markdownflavor

import (
	"regexp"

	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"

	"github.com/jeduden/mdsmith/internal/lint"
)

// Finding records one detected feature use.
//
// Line and Column are 1-based and reference the document body
// (already adjusted to match the main lint.File line numbering).
// Start and End bound the feature in f.Source and are used by Fix.
type Finding struct {
	Feature Feature
	Line    int
	Column  int
	Start   int
	End     int
	// Extra carries feature-specific metadata used by Fix (e.g. the
	// {#id} span inside a heading). Nil when not needed.
	Extra any
}

// HeadingIDExtra describes the byte span of a heading-attribute block
// (e.g. "{#custom-id}") inside the original source.
type HeadingIDExtra struct {
	AttrStart int // byte offset of '{'
	AttrEnd   int // byte offset one past '}'
}

// bareURLPattern mirrors goldmark's linkify http/https/ftp URL regex
// closely enough to catch bare URLs in text. Anchors removed so it can
// match anywhere inside a Text segment.
var bareURLPattern = regexp.MustCompile(
	`(?:http|https|ftp)://[-a-zA-Z0-9@:%._+~#=]{1,256}` +
		`\.[a-z]+(?::\d+)?(?:[/#?][-a-zA-Z0-9@:%_+.~#$!?&/=();,'">^{}\[\]` +
		"`" + `]*)?`,
)

// Detect runs every enabled feature detector against f and returns
// findings in document order.
func Detect(f *lint.File) []Finding {
	var out []Finding
	dualDoc := Parser().Parser().Parse(text.NewReader(f.Source))

	out = append(out, detectFromDual(f, dualDoc)...)
	out = append(out, detectBareURLs(f)...)
	return out
}

// detectFromDual walks the dual-parser tree for all extension-based
// features (tables, strikethrough, task lists, footnotes, definition
// lists, heading IDs).
func detectFromDual(f *lint.File, doc ast.Node) []Finding {
	var findings []Finding
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *extast.Table:
			findings = append(findings, blockFinding(f, n, FeatureTables))
			return ast.WalkSkipChildren, nil
		case *extast.TaskCheckBox:
			findings = append(findings, inlineFinding(f, n, FeatureTaskLists))
		case *extast.Strikethrough:
			findings = append(findings, inlineFinding(f, n, FeatureStrikethrough))
		case *extast.FootnoteLink, *extast.Footnote, *extast.FootnoteList:
			findings = append(findings, blockFinding(f, n, FeatureFootnotes))
			return ast.WalkSkipChildren, nil
		case *extast.DefinitionList:
			findings = append(findings, blockFinding(f, n, FeatureDefinitionLists))
			return ast.WalkSkipChildren, nil
		case *ast.Heading:
			if hf, ok := findHeadingID(f, node); ok {
				findings = append(findings, hf)
			}
		}
		return ast.WalkContinue, nil
	})
	return dedupe(findings)
}

// blockFinding reports a block-level feature starting at column 1 of
// the line containing the node's first text descendant.
func blockFinding(f *lint.File, n ast.Node, feat Feature) Finding {
	start, end := nodeByteRange(n)
	lineStart := lineStartOf(f.Source, start)
	line, _ := lineCol(f.Source, lineStart)
	return Finding{Feature: feat, Line: line, Column: 1, Start: lineStart, End: end}
}

// inlineFinding reports an inline feature at its exact source column.
func inlineFinding(f *lint.File, n ast.Node, feat Feature) Finding {
	start, end := nodeByteRange(n)
	line, col := lineCol(f.Source, start)
	return Finding{Feature: feat, Line: line, Column: col, Start: start, End: end}
}

func nodeByteRange(n ast.Node) (int, int) {
	if n.Type() == ast.TypeBlock {
		if lines := n.Lines(); lines != nil && lines.Len() > 0 {
			first := lines.At(0)
			last := lines.At(lines.Len() - 1)
			return first.Start, last.Stop
		}
	}
	start := firstTextStart(n)
	return start, start
}

func lineStartOf(source []byte, offset int) int {
	if offset > len(source) {
		offset = len(source)
	}
	for i := offset - 1; i >= 0; i-- {
		if source[i] == '\n' {
			return i + 1
		}
	}
	return 0
}

func firstTextStart(n ast.Node) int {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			return t.Segment.Start
		}
		if s := firstTextStart(c); s >= 0 {
			return s
		}
	}
	if t, ok := n.(*ast.Text); ok {
		return t.Segment.Start
	}
	return 0
}

// makeFinding converts a byte range to a Finding with line and column
// derived from f.Source.
func makeFinding(f *lint.File, feat Feature, start, end int) Finding {
	line, col := lineCol(f.Source, start)
	return Finding{Feature: feat, Line: line, Column: col, Start: start, End: end}
}

func lineCol(source []byte, offset int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if offset > len(source) {
		offset = len(source)
	}
	line := 1
	lineStart := 0
	for i := 0; i < offset; i++ {
		if source[i] == '\n' {
			line++
			lineStart = i + 1
		}
	}
	return line, offset - lineStart + 1
}

// dedupe collapses consecutive findings of the same feature at the
// same offset (goldmark's extension nodes sometimes nest, e.g. each
// footnote child also carries FootnoteLink).
func dedupe(in []Finding) []Finding {
	if len(in) < 2 {
		return in
	}
	out := in[:1]
	for _, f := range in[1:] {
		last := out[len(out)-1]
		if f.Feature == last.Feature && f.Start == last.Start {
			continue
		}
		out = append(out, f)
	}
	return out
}

// findHeadingID locates the trailing "{#id}" attribute block that the
// goldmark attribute parser consumed. The Heading node's Lines segment
// only covers the inner text, so we scan the raw line in f.Source from
// the segment start forward to the next newline.
func findHeadingID(f *lint.File, h *ast.Heading) (Finding, bool) {
	if h.Attributes() == nil {
		return Finding{}, false
	}
	if _, ok := h.AttributeString("id"); !ok {
		return Finding{}, false
	}
	lines := h.Lines()
	if lines == nil || lines.Len() == 0 {
		return Finding{}, false
	}
	segStart := lines.At(0).Start
	lineEnd := segStart
	for lineEnd < len(f.Source) && f.Source[lineEnd] != '\n' {
		lineEnd++
	}
	// Find the last '{' on the line that introduces the attribute block.
	brace := -1
	for i := lineEnd - 1; i >= segStart; i-- {
		if f.Source[i] == '{' {
			brace = i
			break
		}
	}
	if brace < 0 {
		return Finding{}, false
	}
	attrStart := brace
	attrEnd := lineEnd
	// Trim trailing whitespace so fixes keep tidy line endings.
	for attrEnd > attrStart && f.Source[attrEnd-1] == ' ' {
		attrEnd--
	}
	line, col := lineCol(f.Source, attrStart)
	return Finding{
		Feature: FeatureHeadingIDs,
		Line:    line,
		Column:  col,
		Start:   attrStart,
		End:     attrEnd,
		Extra:   HeadingIDExtra{AttrStart: attrStart, AttrEnd: attrEnd},
	}, true
}

// detectBareURLs scans f.AST (the main CommonMark parse, which has no
// extensions) for bare URL text. Bracketed <url> autolinks are
// recognised by CommonMark and appear as ast.AutoLink, so only true
// bare URLs remain inside Text nodes.
func detectBareURLs(f *lint.File) []Finding {
	var findings []Finding
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		t, ok := n.(*ast.Text)
		if !ok {
			return ast.WalkContinue, nil
		}
		if insideNonBareContext(n) {
			return ast.WalkContinue, nil
		}
		seg := t.Segment
		body := seg.Value(f.Source)
		matches := bareURLPattern.FindAllIndex(body, -1)
		for _, m := range matches {
			start := seg.Start + m[0]
			end := seg.Start + m[1]
			findings = append(findings, makeFinding(f, FeatureBareURLAutolinks, start, end))
		}
		return ast.WalkContinue, nil
	})
	return findings
}

func insideNonBareContext(n ast.Node) bool {
	for p := n.Parent(); p != nil; p = p.Parent() {
		switch p.(type) {
		case *ast.Link, *ast.AutoLink, *ast.CodeSpan, *ast.FencedCodeBlock,
			*ast.CodeBlock:
			return true
		}
	}
	return false
}
