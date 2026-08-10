package flavor

import (
	"bytes"
	"regexp"
	"sort"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	extast "github.com/jeduden/mdsmith/pkg/goldmark/extension/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"

	"github.com/jeduden/mdsmith/pkg/markdown"
	"github.com/jeduden/mdsmith/pkg/markdown/flavor/ext"
)

// Finding records one detected feature use.
//
// Line and Column are 1-based positions in the parsed document body
// (i.e. doc.Body, not the original source if it carried front matter).
// Callers that present diagnostics relative to a file may need to add
// the front-matter line offset themselves; mdsmith's internal linter
// does this via lint.File.AdjustDiagnostics.
//
// Start and End are best-effort byte anchors in doc.Body. They cover
// the feature span precisely only for features whose Fix needs an
// exact range (heading IDs via Extra, and bare URLs). Other findings
// use convenience anchors: block features widen Start to the start
// of the containing line, and inline extension nodes without a source
// segment emit a zero-length anchor (End == Start). Any future
// rewriter that needs a precise span must recompute it from doc.Body
// rather than trusting End - Start.
type Finding struct {
	Feature Feature
	Line    int
	Column  int
	Start   int
	End     int
	// Extra carries feature-specific metadata used by external
	// rewriters (e.g. the {#id} span inside a heading). Nil when not
	// needed. The only shape currently emitted is HeadingIDExtra,
	// attached to FeatureHeadingIDs findings.
	Extra any
}

// HeadingIDExtra describes the byte span of a heading-attribute block
// (e.g. "{#custom-id}") inside doc.Body. Emitted on every
// FeatureHeadingIDs finding so rewriters can drop the attribute block
// without re-scanning the source.
type HeadingIDExtra struct {
	AttrStart int // byte offset of '{'
	AttrEnd   int // byte offset one past '}'
}

// alertTokenRe matches the exact content of a GitHub Alert marker
// line inside a blockquote (case-sensitive per GFM spec).
var alertTokenRe = regexp.MustCompile(`^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*$`)

// bareURLPattern mirrors goldmark's linkify http/https/ftp URL regex
// closely enough to catch bare URLs in text. Anchors are removed so
// it can match anywhere inside a Text segment. The TLD class accepts
// both upper- and lowercase ASCII so URLs like https://example.COM
// are flagged the same as their lowercase form.
var bareURLPattern = regexp.MustCompile(
	`(?:http|https|ftp)://[-a-zA-Z0-9@:%._+~#=]{1,256}` +
		`\.[a-zA-Z]+(?::\d+)?(?:[/#?][-a-zA-Z0-9@:%_+.~#$!?&/=();,'">^{}\[\]` +
		"`" + `]*)?`,
)

// schemeSeparator is the byte needle every bareURLPattern alternative
// shares — a package-level slice avoids allocating a fresh one on
// every BareURLFindingsInTree Text-node check.
var schemeSeparator = []byte("://")

// Detect runs every feature detector against doc and returns findings
// in document-body order. accept is an optional predicate: when
// non-nil, only features for which accept(feat) returns true are
// detected; whole-file scans are skipped when none of their features
// are accepted. Passing nil accepts every feature.
//
// The dual-parser and bare-URL passes each emit in document order on
// their own, but the two streams must be merged: a bare URL on line 3
// should sort before a footnote definition on line 5 even though
// detectFromDual runs first.
func Detect(doc *markdown.Document, accept func(Feature) bool) []Finding {
	if doc == nil {
		return nil
	}
	keep := func(feat Feature) bool {
		return accept == nil || accept(feat)
	}

	source := doc.Body
	lineCol := newLazyLineCol(source)
	var out []Finding

	if anyDualFeatureAccepted(keep) {
		out = append(out, dualFindings(source, lineCol, keep)...)
	}

	if keep(FeatureBareURLAutolinks) {
		out = append(out, detectBareURLs(source, lineCol, doc.AST)...)
	}

	if keep(FeatureGitHubAlerts) {
		out = append(out, detectGitHubAlerts(source, lineCol, doc.AST)...)
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Start < out[j].Start
	})
	return out
}

// dualFindings runs the dual parser via the package-shared pool,
// walks the resulting AST, and returns the keep-filtered findings.
// The borrow lasts for the parse-plus-walk only — WithSharedParser
// resets the parser's link-ref transformer before returning it to
// the pool so source bytes are not pinned between calls. Returns nil
// (not an empty slice) when no finding passes keep, matching the
// project's allocation-budget rule.
func dualFindings(source []byte, lineCol func(int) (int, int), keep func(Feature) bool) []Finding {
	var out []Finding
	WithSharedParser(func(p parser.Parser) {
		dualDoc := p.Parse(text.NewReader(source))
		for _, fin := range detectFromDual(source, lineCol, dualDoc) {
			if keep(fin.Feature) {
				out = append(out, fin)
			}
		}
	})
	return out
}

// anyDualFeatureAccepted reports whether any feature detected by the
// dual-parser pass is wanted. Lets Detect skip the goldmark re-parse
// when every feature it would detect is already supported by the
// target flavor.
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
func detectFromDual(source []byte, lineCol func(int) (int, int), doc ast.Node) []Finding {
	var findings []Finding
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		fin, status := featureFindingFor(source, lineCol, n)
		if fin != nil {
			findings = append(findings, *fin)
		}
		return status, nil
	})
	return dedupe(findings)
}

// featureFindingFor maps an AST node to at most one Finding plus the
// walk-status to return for the rest of the walk. A nil pointer means
// "no finding for this node".
func featureFindingFor(source []byte, lineCol func(int) (int, int), n ast.Node) (*Finding, ast.WalkStatus) {
	if fin, status, ok := builtinFindingFor(source, lineCol, n); ok {
		return fin, status
	}
	if fin, status, ok := customFindingFor(source, lineCol, n); ok {
		return fin, status
	}
	return nil, ast.WalkContinue
}

// builtinFindingFor handles the six features detected via goldmark's
// built-in extensions plus the heading-ID attribute parser.
func builtinFindingFor(source []byte, lineCol func(int) (int, int), n ast.Node) (*Finding, ast.WalkStatus, bool) {
	switch node := n.(type) {
	case *extast.Table:
		fin := blockFinding(source, lineCol, n, FeatureTables)
		return &fin, ast.WalkSkipChildren, true
	case *extast.TaskCheckBox:
		fin := inlineExtFinding(lineCol, n, FeatureTaskLists)
		return &fin, ast.WalkContinue, true
	case *extast.Strikethrough:
		fin := strikethroughFinding(source, lineCol, n)
		return &fin, ast.WalkContinue, true
	case *extast.FootnoteLink:
		fin := inlineExtFinding(lineCol, n, FeatureFootnotes)
		return &fin, ast.WalkContinue, true
	case *extast.Footnote:
		fin := blockFinding(source, lineCol, n, FeatureFootnotes)
		return &fin, ast.WalkSkipChildren, true
	case *extast.FootnoteList:
		// Walk children so Footnote definitions report their own
		// locations; skip emitting a wrapper finding.
		return nil, ast.WalkContinue, true
	case *extast.DefinitionList:
		fin := blockFinding(source, lineCol, n, FeatureDefinitionLists)
		return &fin, ast.WalkSkipChildren, true
	case *ast.Heading:
		if hf, ok := findHeadingID(source, lineCol, node); ok {
			return &hf, ast.WalkContinue, true
		}
		return nil, ast.WalkContinue, true
	}
	return nil, ast.WalkContinue, false
}

// customFindingFor handles the five features covered by MDS034
// custom extensions: superscript, subscript, math block / inline,
// and abbreviations (both definition and reference).
func customFindingFor(source []byte, lineCol func(int) (int, int), n ast.Node) (*Finding, ast.WalkStatus, bool) {
	switch n.(type) {
	case *ext.SuperscriptNode:
		fin := markerInlineFinding(source, lineCol, n, FeatureSuperscript, '^')
		return &fin, ast.WalkContinue, true
	case *ext.SubscriptNode:
		fin := markerInlineFinding(source, lineCol, n, FeatureSubscript, '~')
		return &fin, ast.WalkContinue, true
	case *ext.MathBlockNode:
		fin := blockFinding(source, lineCol, n, FeatureMathBlock)
		return &fin, ast.WalkSkipChildren, true
	case *ext.MathInlineNode:
		fin := markerInlineFinding(source, lineCol, n, FeatureMathInline, '$')
		return &fin, ast.WalkContinue, true
	case *ext.AbbreviationDefinition:
		fin := blockFinding(source, lineCol, n, FeatureAbbreviations)
		return &fin, ast.WalkSkipChildren, true
	case *ext.AbbreviationReference:
		// The reference carries a child Text with the term's exact
		// source segment, so inlineFinding pulls the real column
		// rather than the enclosing paragraph start.
		fin := inlineFinding(lineCol, n, FeatureAbbreviations)
		return &fin, ast.WalkContinue, true
	}
	return nil, ast.WalkContinue, false
}

// strikethroughFinding backs up past the opening "~~" so the
// diagnostic points at the marker, not at the content character.
func strikethroughFinding(source []byte, lineCol func(int) (int, int), n ast.Node) Finding {
	fin := inlineFinding(lineCol, n, FeatureStrikethrough)
	if fin.Start >= 2 && source[fin.Start-1] == '~' && source[fin.Start-2] == '~' {
		fin.Start -= 2
		fin.Column -= 2
	}
	return fin
}

// markerInlineFinding backs up a single opening marker byte before
// the first text descendant. Used for superscript / subscript /
// inline-math spans where the first child text starts after the
// single-byte marker.
func markerInlineFinding(source []byte, lineCol func(int) (int, int), n ast.Node, feat Feature, marker byte) Finding {
	fin := inlineFinding(lineCol, n, feat)
	if fin.Start >= 1 && source[fin.Start-1] == marker {
		fin.Start--
		fin.Column--
	}
	return fin
}

// blockFinding reports a block-level feature starting at column 1 of
// the line containing the node's first text descendant.
func blockFinding(source []byte, lineCol func(int) (int, int), n ast.Node, feat Feature) Finding {
	start, end := nodeByteRange(n)
	lineStart := lineStartOf(source, start)
	line, _ := lineCol(lineStart)
	return Finding{Feature: feat, Line: line, Column: 1, Start: lineStart, End: end}
}

// inlineExtFinding covers inline extension nodes that expose no
// segment (e.g. FootnoteLink, TaskCheckBox). It uses the first
// ancestor block's first-line position instead of firstTextStart,
// which would return zero for a childless inline.
func inlineExtFinding(lineCol func(int) (int, int), n ast.Node, feat Feature) Finding {
	if p := NearestBlockAncestor(n); p != nil {
		return findingFromBlock(lineCol, p, feat)
	}
	return Finding{Feature: feat, Line: 1, Column: 1}
}

// NearestBlockAncestor walks up from n and returns the first block-
// typed ancestor with non-empty Lines(). Returns nil when no such
// ancestor exists (typically a hand-constructed inline node with no
// surrounding paragraph). Exposed for rewriters that hold an inline
// AST node and need the block context that owns its source position.
func NearestBlockAncestor(n ast.Node) ast.Node {
	for p := n.Parent(); p != nil; p = p.Parent() {
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
func findingFromBlock(lineCol func(int) (int, int), block ast.Node, feat Feature) Finding {
	lines := block.Lines()
	if lines == nil || lines.Len() == 0 {
		return Finding{Feature: feat, Line: 1, Column: 1}
	}
	start := lines.At(0).Start
	line, col := lineCol(start)
	return Finding{Feature: feat, Line: line, Column: col, Start: start, End: start}
}

// inlineFinding reports an inline feature at its exact source column.
func inlineFinding(lineCol func(int) (int, int), n ast.Node, feat Feature) Finding {
	start, end := nodeByteRange(n)
	line, col := lineCol(start)
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
	offset = clampOffset(source, offset)
	return bytes.LastIndexByte(source[:offset], '\n') + 1
}

// clampOffset bounds offset to [0, len(source)] so a caller that
// looks at byte -1 or one past EOF still gets a valid index to slice
// or index with. Shared by lineStartOf and LineCol.
func clampOffset(source []byte, offset int) int {
	if offset < 0 {
		return 0
	}
	if offset > len(source) {
		return len(source)
	}
	return offset
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
// resolved via lineCol.
func makeFinding(lineCol func(int) (int, int), feat Feature, start, end int) Finding {
	line, col := lineCol(start)
	return Finding{Feature: feat, Line: line, Column: col, Start: start, End: end}
}

// isASCIISpace reports whether b is one of the ASCII whitespace bytes
// that can legitimately appear after a heading's attribute block
// before the line's newline.
func isASCIISpace(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\v', '\f':
		return true
	}
	return false
}

// LineCol returns the 1-based (line, column) position of offset
// within source. Out-of-range offsets are clamped so callers that
// look at byte -1 or one past EOF still get a valid position.
// Exposed for rewriters that need to translate byte offsets into
// line numbers when producing line-level edits — a single-lookup use
// case, so it always rescans source directly rather than building a
// lineIndex (which only pays off when reused across many lookups; see
// lineIndex and newLazyLineCol below, used internally by Detect and
// BareURLFindingsInTree).
func LineCol(source []byte, offset int) (line, col int) {
	offset = clampOffset(source, offset)
	// bytes.Count and bytes.LastIndexByte are SIMD-accelerated on
	// amd64; a manual byte-by-byte loop over the same prefix does not
	// vectorize. See docs/development/high-performance-go.md
	// "bytes.IndexByte over a hand-rolled byte loop". lineStart
	// defaults to 0 when no '\n' precedes offset, matching
	// LastIndexByte's -1-not-found sentinel plus 1.
	prefix := source[:offset]
	line = 1 + bytes.Count(prefix, newline)
	lineStart := bytes.LastIndexByte(prefix, '\n') + 1
	return line, offset - lineStart + 1
}

// newline is the single-byte needle LineCol counts with
// bytes.Count — a package-level slice avoids allocating a fresh
// one-element slice literal on every call.
var newline = []byte{'\n'}

// lineIndex is a cached, sorted list of newline byte offsets in a
// source buffer, giving O(log n) lineCol lookups instead of LineCol's
// O(offset) rescan from byte 0 on every call. A single Detect or
// BareURLFindingsInTree run can produce many findings scattered across
// the document; LineCol degrades toward O(n^2) total across them,
// mirroring the anti-pattern internal/lint.File.LineOfOffset was
// rewritten to fix (docs/development/high-performance-go.md "memoize
// per-input computations").
type lineIndex struct {
	source   []byte
	newlines []int
}

// newLineIndex scans source for every '\n' byte and records its
// offset, mirroring internal/lint.File.lineIndex. bytes.Count
// pre-sizes the slice (also SIMD-accelerated) so the append loop never
// regrows; bytes.IndexByte is SIMD-accelerated on amd64 where a
// hand-rolled byte loop is not.
func newLineIndex(source []byte) *lineIndex {
	nl := make([]int, 0, bytes.Count(source, newline))
	for base := 0; ; {
		i := bytes.IndexByte(source[base:], '\n')
		if i < 0 {
			break
		}
		nl = append(nl, base+i)
		base += i + 1
	}
	return &lineIndex{source: source, newlines: nl}
}

// lineCol returns the 1-based (line, column) position of offset,
// matching LineCol's semantics exactly (clamped offset; a newline
// exactly at offset starts the next line).
func (idx *lineIndex) lineCol(offset int) (line, col int) {
	offset = clampOffset(idx.source, offset)
	lo := newlineSearch(idx.newlines, offset)
	line = lo + 1
	start := 0
	if lo > 0 {
		start = idx.newlines[lo-1] + 1
	}
	return line, offset - start + 1
}

// newlineSearch returns the count of entries in nl (a sorted list of
// newline byte offsets) that are strictly less than offset — the
// number of newlines preceding offset. Inlined rather than
// sort.Search, mirroring internal/lint.newlineSearch: sort.Search's
// comparison closure would capture nl and offset and escape to the
// heap.
func newlineSearch(nl []int, offset int) int {
	lo, hi := 0, len(nl)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if nl[mid] >= offset {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo
}

// newLazyLineCol returns a lineCol lookup function that builds and
// caches a lineIndex for source on first use. A call that produces
// zero findings (the common case for a well-formed doc against a
// permissive flavor) never pays for the newline scan; a call that
// produces many findings pays for it once instead of once per finding.
func newLazyLineCol(source []byte) func(offset int) (line, col int) {
	var idx *lineIndex
	return func(offset int) (int, int) {
		if idx == nil {
			idx = newLineIndex(source)
		}
		return idx.lineCol(offset)
	}
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

// FindHeadingID locates the trailing "{#id}" attribute block that the
// goldmark attribute parser consumed on h. The Heading node's Lines
// segment only covers the inner text, so the scan walks the raw line
// in source from the segment start forward to the next newline.
//
// Returns the byte span of the attribute block; the bool is false
// when h is nil, carries no `id` attribute, has no Lines populated,
// or has no `{` on its first line. Used by rewriters that want to
// drop the attribute block without consuming a full Detect run.
func FindHeadingID(source []byte, h *ast.Heading) (HeadingIDExtra, bool) {
	fin, ok := findHeadingID(source, func(offset int) (int, int) { return LineCol(source, offset) }, h)
	if !ok {
		return HeadingIDExtra{}, false
	}
	// findHeadingID is the only writer of Finding.Extra for
	// FeatureHeadingIDs and always stores HeadingIDExtra — the
	// assertion is by contract, not a defensive guard, so the
	// untyped-Extra failure path is not driven by any test.
	return fin.Extra.(HeadingIDExtra), true
}

// findHeadingID locates the trailing "{#id}" attribute block that the
// goldmark attribute parser consumed. The Heading node's Lines segment
// only covers the inner text, so we scan the raw line in source from
// the segment start forward to the next newline.
func findHeadingID(source []byte, lineCol func(int) (int, int), h *ast.Heading) (Finding, bool) {
	if h == nil {
		return Finding{}, false
	}
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
	for lineEnd < len(source) && source[lineEnd] != '\n' {
		lineEnd++
	}
	// Find the last '{' on the line that introduces the attribute block.
	brace := -1
	for i := lineEnd - 1; i >= segStart; i-- {
		if source[i] == '{' {
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
	for attrEnd > attrStart && isASCIISpace(source[attrEnd-1]) {
		attrEnd--
	}
	line, col := lineCol(attrStart)
	return Finding{
		Feature: FeatureHeadingIDs,
		Line:    line,
		Column:  col,
		Start:   attrStart,
		End:     attrEnd,
		Extra:   HeadingIDExtra{AttrStart: attrStart, AttrEnd: attrEnd},
	}, true
}

// detectBareURLs scans cmAST (the CommonMark parse, with no
// extensions) for bare URL text. Bracketed <url> autolinks are
// recognised by CommonMark and appear as ast.AutoLink, so only true
// bare URLs remain inside Text nodes.
func detectBareURLs(source []byte, lineCol func(int) (int, int), cmAST ast.Node) []Finding {
	return bareURLFindingsInTree(source, lineCol, cmAST, 0)
}

// BareURLFindingsInTree returns one FeatureBareURLAutolinks finding per bare
// URL in the Text descendants of root that are not inside a link, autolink, or
// code context. base is added to each Text segment's run-local offsets so a
// caller that re-parsed an inline run in isolation (the parse-skipped path)
// recovers document-absolute positions; pass 0 when root is the document's own
// CommonMark AST. The findings are byte-identical to detectBareURLs by
// construction, so the AST path and the inline-run path agree.
//
// Uses the plain LineCol oracle rather than a lineIndex: markdownflavor's
// parse-skipped path (internal/rules/markdownflavor/rule.go) calls this once
// per lint.InlineBlocks entry — often dozens of calls per file, each with at
// most a handful of matches — so building a full source-length index inside
// every call would cost more than it saves; only Detect's single call
// producing many findings across the whole document (via detectBareURLs
// below, which shares Detect's own lineIndex) benefits from amortizing the
// index build.
func BareURLFindingsInTree(source []byte, root ast.Node, base int) []Finding {
	lineCol := func(offset int) (int, int) { return LineCol(source, offset) }
	return bareURLFindingsInTree(source, lineCol, root, base)
}

// bareURLFindingsInTree is the shared core behind BareURLFindingsInTree and
// detectBareURLs. lineCol lets Detect's callers reuse an already-built
// lineIndex instead of paying for a fresh one per call.
func bareURLFindingsInTree(source []byte, lineCol func(int) (int, int), root ast.Node, base int) []Finding {
	if root == nil {
		return nil
	}
	var findings []Finding
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
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
		body := source[base+seg.Start : base+seg.Stop]
		// Every alternative in bareURLPattern (http/https/ftp) shares the
		// "://" scheme separator, so a Text segment that lacks it can
		// never match. Gate the regex behind this cheap byte scan per
		// docs/development/high-performance-go.md "gate expensive
		// analyzers behind a cheap pre-check" — most prose Text nodes
		// carry no URL at all.
		if !bytes.Contains(body, schemeSeparator) {
			return ast.WalkContinue, nil
		}
		matches := bareURLPattern.FindAllIndex(body, -1)
		for _, m := range matches {
			start := base + seg.Start + m[0]
			end := base + seg.Start + m[1]
			findings = append(findings, makeFinding(lineCol, FeatureBareURLAutolinks, start, end))
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

// detectGitHubAlerts walks cmAST for Blockquote nodes whose first
// paragraph child starts with a GFM alert token (e.g. [!NOTE]).
func detectGitHubAlerts(source []byte, lineCol func(int) (int, int), cmAST ast.Node) []Finding {
	if cmAST == nil {
		return nil
	}
	var findings []Finding
	_ = ast.Walk(cmAST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		bq, ok := n.(*ast.Blockquote)
		if !ok {
			return ast.WalkContinue, nil
		}
		if IsGitHubAlert(bq, source) {
			findings = append(findings, blockFinding(source, lineCol, bq, FeatureGitHubAlerts))
		}
		return ast.WalkContinue, nil
	})
	return findings
}

// IsAlertMarkerLine reports whether line is a GitHub Alert marker line: a
// blockquote line whose content, after stripping the leading `>` markers and
// surrounding whitespace, is one of the five GFM alert tokens. It is the
// line-level counterpart to IsGitHubAlert for callers that work from the
// Layer 0 block scan rather than a parsed Blockquote node.
func IsAlertMarkerLine(line []byte) bool {
	return alertTokenRe.Match(StripBlockquoteMarkers(line))
}

// StripBlockquoteMarkers removes a blockquote line's leading indentation and
// `>` markers (each optionally followed by one space), returning the inner
// content with trailing line terminators trimmed — mirroring how goldmark
// records a quoted paragraph's first content line. Exposed so callers working
// from the Layer 0 block scan can locate a blockquote's first paragraph line.
func StripBlockquoteMarkers(line []byte) []byte {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	for i < len(line) && line[i] == '>' {
		i++
		if i < len(line) && line[i] == ' ' {
			i++
		}
	}
	return bytes.TrimRight(line[i:], "\r\n")
}

// AlertFinding builds a FeatureGitHubAlerts finding anchored at the line
// beginning at lineStart in source — the same anchor blockFinding produces for
// a parsed alert Blockquote, so the AST path and the Layer 0 line path agree.
func AlertFinding(source []byte, lineStart int) Finding {
	line, _ := LineCol(source, lineStart)
	return Finding{Feature: FeatureGitHubAlerts, Line: line, Column: 1, Start: lineStart, End: lineStart}
}

// IsGitHubAlert reports whether bq is a GitHub Alert blockquote: its
// first paragraph child's first line matches one of the five GFM
// alert tokens ([!NOTE], [!TIP], [!IMPORTANT], [!WARNING], [!CAUTION]).
// Returns false when bq is nil, its first child is not a paragraph,
// or the paragraph carries no Lines. Exposed for rewriters that want
// to strip alert markers without running a full Detect.
func IsGitHubAlert(bq *ast.Blockquote, source []byte) bool {
	if bq == nil {
		return false
	}
	para, ok := bq.FirstChild().(*ast.Paragraph)
	if !ok {
		return false
	}
	lines := para.Lines()
	if lines == nil || lines.Len() == 0 {
		return false
	}
	seg := lines.At(0)
	firstLine := bytes.TrimRight(source[seg.Start:seg.Stop], "\r\n")
	return alertTokenRe.Match(firstLine)
}
