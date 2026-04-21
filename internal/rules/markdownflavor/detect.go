package markdownflavor

import (
	"regexp"
	"sort"

	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rules/markdownflavor/ext"
)

// Finding records one detected feature use.
//
// Line and Column are 1-based positions within the parsed document
// body in f.Source. The engine's lint.File.AdjustDiagnostics applies
// any front-matter LineOffset later, so detectors and Rule.Check
// must report body-relative positions only.
//
// Start and End are best-effort byte anchors in f.Source. They cover
// the feature span precisely only for features whose Fix needs an
// exact range (currently heading IDs via Extra, and bare URLs).
// Other findings use convenience anchors: block features widen Start
// to the start of the containing line, and inline extension nodes
// without a source segment emit a zero-length anchor (End == Start).
// Any future Fix implementation that needs a precise span must
// recompute it from f.Source rather than trusting End - Start.
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
// match anywhere inside a Text segment. The TLD class accepts both
// upper- and lowercase ASCII so URLs like https://example.COM are
// flagged the same way as their lowercase form.
var bareURLPattern = regexp.MustCompile(
	`(?:http|https|ftp)://[-a-zA-Z0-9@:%._+~#=]{1,256}` +
		`\.[a-zA-Z]+(?::\d+)?(?:[/#?][-a-zA-Z0-9@:%_+.~#$!?&/=();,'">^{}\[\]` +
		"`" + `]*)?`,
)

// Detect runs every feature detector against f and returns findings
// in document order. Use DetectFiltered to skip detectors for
// features the caller is not interested in.
func Detect(f *lint.File) []Finding {
	return DetectFiltered(f, nil)
}

// DetectFiltered is Detect with an optional accept predicate. When
// accept is non-nil, only features for which accept(feat) returns
// true are detected; whole-file scans are skipped when none of their
// features are accepted. Passing nil accepts every feature.
//
// The dual-parser and bare-URL passes each emit in document order
// on their own, but the two streams must be merged: a bare URL on
// line 3 should sort before a footnote definition on line 5 even
// though detectFromDual runs first.
func DetectFiltered(f *lint.File, accept func(Feature) bool) []Finding {
	keep := func(feat Feature) bool {
		return accept == nil || accept(feat)
	}

	var out []Finding

	if anyDualFeatureAccepted(keep) {
		dualDoc := Parser().Parser().Parse(text.NewReader(f.Source))
		for _, fin := range detectFromDual(f, dualDoc) {
			if keep(fin.Feature) {
				out = append(out, fin)
			}
		}
	}

	if keep(FeatureBareURLAutolinks) {
		out = append(out, detectBareURLs(f)...)
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Start < out[j].Start
	})
	return out
}

// anyDualFeatureAccepted reports whether any feature detected by the
// dual-parser pass is wanted. Lets DetectFiltered skip the goldmark
// re-parse when every feature it would detect is already supported
// by the target flavor.
func anyDualFeatureAccepted(keep func(Feature) bool) bool {
	for _, feat := range []Feature{
		FeatureTables, FeatureTaskLists, FeatureStrikethrough,
		FeatureFootnotes, FeatureDefinitionLists, FeatureHeadingIDs,
		FeatureSuperscript, FeatureSubscript,
		FeatureMathBlock, FeatureMathInline, FeatureAbbreviations,
	} {
		if keep(feat) {
			return true
		}
	}
	return false
}

// detectFromDual walks the dual-parser tree for every feature that
// has an AST representation: the six built-in extensions (tables,
// strikethrough, task lists, footnotes, definition lists, heading
// IDs) plus the five MDS034 custom extensions (superscript,
// subscript, math block, math inline, abbreviations).
func detectFromDual(f *lint.File, doc ast.Node) []Finding {
	var findings []Finding
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		fin, status := featureFindingFor(f, n)
		if fin != nil {
			findings = append(findings, *fin)
		}
		return status, nil
	})
	return dedupe(findings)
}

// featureFindingFor maps an AST node to at most one Finding plus
// the walk-status to return for the rest of the walk. A nil pointer
// means "no finding for this node".
func featureFindingFor(f *lint.File, n ast.Node) (*Finding, ast.WalkStatus) {
	if fin, status, ok := builtinFindingFor(f, n); ok {
		return fin, status
	}
	if fin, status, ok := customFindingFor(f, n); ok {
		return fin, status
	}
	return nil, ast.WalkContinue
}

// builtinFindingFor handles the six features detected via goldmark's
// built-in extensions plus the heading-ID attribute parser.
func builtinFindingFor(f *lint.File, n ast.Node) (*Finding, ast.WalkStatus, bool) {
	switch node := n.(type) {
	case *extast.Table:
		fin := blockFinding(f, n, FeatureTables)
		return &fin, ast.WalkSkipChildren, true
	case *extast.TaskCheckBox:
		fin := taskCheckBoxFinding(f, n)
		return &fin, ast.WalkContinue, true
	case *extast.Strikethrough:
		fin := strikethroughFinding(f, n)
		return &fin, ast.WalkContinue, true
	case *extast.FootnoteLink:
		fin := inlineExtFinding(f, n, FeatureFootnotes)
		return &fin, ast.WalkContinue, true
	case *extast.Footnote:
		fin := blockFinding(f, n, FeatureFootnotes)
		return &fin, ast.WalkSkipChildren, true
	case *extast.FootnoteList:
		// Walk children so Footnote definitions report their own
		// locations; skip emitting a wrapper finding.
		return nil, ast.WalkContinue, true
	case *extast.DefinitionList:
		fin := blockFinding(f, n, FeatureDefinitionLists)
		return &fin, ast.WalkSkipChildren, true
	case *ast.Heading:
		if hf, ok := findHeadingID(f, node); ok {
			return &hf, ast.WalkContinue, true
		}
		return nil, ast.WalkContinue, true
	}
	return nil, ast.WalkContinue, false
}

// customFindingFor handles the five features covered by MDS034
// custom extensions: superscript, subscript, math block / inline,
// and abbreviations (both definition and reference).
func customFindingFor(f *lint.File, n ast.Node) (*Finding, ast.WalkStatus, bool) {
	switch n.(type) {
	case *ext.SuperscriptNode:
		fin := markerInlineFinding(f, n, FeatureSuperscript, '^')
		return &fin, ast.WalkContinue, true
	case *ext.SubscriptNode:
		fin := markerInlineFinding(f, n, FeatureSubscript, '~')
		return &fin, ast.WalkContinue, true
	case *ext.MathBlockNode:
		fin := blockFinding(f, n, FeatureMathBlock)
		return &fin, ast.WalkSkipChildren, true
	case *ext.MathInlineNode:
		fin := markerInlineFinding(f, n, FeatureMathInline, '$')
		return &fin, ast.WalkContinue, true
	case *ext.AbbreviationDefinition:
		fin := blockFinding(f, n, FeatureAbbreviations)
		return &fin, ast.WalkSkipChildren, true
	case *ext.AbbreviationReference:
		// The reference carries a child Text with the term's exact
		// source segment, so inlineFinding pulls the real column
		// rather than the enclosing paragraph start.
		fin := inlineFinding(f, n, FeatureAbbreviations)
		return &fin, ast.WalkContinue, true
	}
	return nil, ast.WalkContinue, false
}

// strikethroughFinding backs up past the opening "~~" so the
// diagnostic points at the marker, not at the content character.
func strikethroughFinding(f *lint.File, n ast.Node) Finding {
	fin := inlineFinding(f, n, FeatureStrikethrough)
	if fin.Start >= 2 && f.Source[fin.Start-1] == '~' && f.Source[fin.Start-2] == '~' {
		fin.Start -= 2
		fin.Column -= 2
	}
	return fin
}

// markerInlineFinding backs up a single opening marker byte before
// the first text descendant. Used for superscript / subscript /
// inline-math spans where the first child text starts after the
// single-byte marker.
func markerInlineFinding(f *lint.File, n ast.Node, feat Feature, marker byte) Finding {
	fin := inlineFinding(f, n, feat)
	if fin.Start >= 1 && f.Source[fin.Start-1] == marker {
		fin.Start--
		fin.Column--
	}
	return fin
}

// blockFinding reports a block-level feature starting at column 1 of
// the line containing the node's first text descendant.
func blockFinding(f *lint.File, n ast.Node, feat Feature) Finding {
	start, end := nodeByteRange(n)
	lineStart := lineStartOf(f.Source, start)
	line, _ := lineCol(f.Source, lineStart)
	return Finding{Feature: feat, Line: line, Column: 1, Start: lineStart, End: end}
}

// taskCheckBoxFinding synthesises a Finding for a TaskCheckBox by
// walking up to the nearest block ancestor with line info (TextBlock
// inside the containing ListItem). TaskCheckBox has no source segment
// of its own.
func taskCheckBoxFinding(f *lint.File, n ast.Node) Finding {
	if p := nearestBlockAncestor(n, ast.NodeKind(0)); p != nil {
		return findingFromBlock(f, p, FeatureTaskLists)
	}
	return Finding{Feature: FeatureTaskLists, Line: 1, Column: 1}
}

// inlineExtFinding covers inline extension nodes that expose no
// segment (e.g. FootnoteLink). It uses the first ancestor block's
// first-line position instead of firstTextStart, which would return
// zero for a childless inline.
func inlineExtFinding(f *lint.File, n ast.Node, feat Feature) Finding {
	if p := nearestBlockAncestor(n, ast.NodeKind(0)); p != nil {
		return findingFromBlock(f, p, feat)
	}
	return Finding{Feature: feat, Line: 1, Column: 1}
}

// nearestBlockAncestor walks up from n and returns the first ancestor
// whose kind matches want; when want is 0 the first block-typed
// ancestor with Lines() is returned.
func nearestBlockAncestor(n ast.Node, want ast.NodeKind) ast.Node {
	for p := n.Parent(); p != nil; p = p.Parent() {
		if want != 0 {
			if p.Kind() == want {
				return p
			}
			continue
		}
		if p.Type() != ast.TypeBlock {
			continue
		}
		if lines := p.Lines(); lines != nil && lines.Len() > 0 {
			return p
		}
	}
	return nil
}

// findingFromBlock builds an inline-style finding (exact line/col of
// the block's first line) for features emitted from a block ancestor.
func findingFromBlock(f *lint.File, block ast.Node, feat Feature) Finding {
	lines := block.Lines()
	if lines == nil || lines.Len() == 0 {
		return Finding{Feature: feat, Line: 1, Column: 1}
	}
	start := lines.At(0).Start
	line, col := lineCol(f.Source, start)
	return Finding{Feature: feat, Line: line, Column: col, Start: start, End: start}
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
	if start < 0 {
		start = 0
	}
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

// firstTextStart returns the byte offset of the first descendant Text
// node, or -1 when none exists. The sentinel matters: returning 0 on
// "not found" would point at the start of the file and shift inline
// findings to line 1, column 1.
func firstTextStart(n ast.Node) int {
	if t, ok := n.(*ast.Text); ok {
		return t.Segment.Start
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if s := firstTextStart(c); s >= 0 {
			return s
		}
	}
	return -1
}

// makeFinding converts a byte range to a Finding with line and column
// derived from f.Source.
func makeFinding(f *lint.File, feat Feature, start, end int) Finding {
	line, col := lineCol(f.Source, start)
	return Finding{Feature: feat, Line: line, Column: col, Start: start, End: end}
}

// isASCIISpace reports whether b is one of the ASCII whitespace
// bytes that can legitimately appear after a heading's attribute
// block before the line's newline.
func isASCIISpace(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\v', '\f':
		return true
	}
	return false
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
	// Trim trailing ASCII whitespace so fixes keep tidy line endings
	// even when the heading line ends with a tab or CRLF.
	for attrEnd > attrStart && isASCIISpace(f.Source[attrEnd-1]) {
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
