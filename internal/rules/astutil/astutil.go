package astutil

import (
	"bytes"
	"cmp"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
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
//
// Node and Text are declared before Line/HasText: buildSectionParagraphs
// allocates one SectionParagraph per document paragraph, so this is
// among the highest-frequency struct constructions in the rule set
// (MDS023, MDS024). Node (an interface) and Text (a string) are the
// struct's only pointer-bearing fields; declaring the plain-int Line
// field ahead of them would force the GC's per-word ptrdata scan to
// cover Line's word too. See sectionparagraph_layout_test.go.
type SectionParagraph struct {
	Node    ast.Node
	Text    string
	Line    int
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
//
// Memoized per File via lint.File.MemoFile, mirroring
// CollectSectionParagraphs: MDS057 and MDS058 both enabled would
// otherwise re-walk the same AST for the same result.
func CollectSectionHeadings(f *lint.File) []SectionHeading {
	return f.MemoFile("astutil.sectionHeadings", buildSectionHeadings).([]SectionHeading)
}

// buildSectionHeadings is the MemoFile-style builder for the
// section-headings memo. Defined at package scope so the value passed
// to MemoFile is a plain function pointer, matching
// buildSectionParagraphs.
func buildSectionHeadings(f *lint.File) any {
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
	sortSectionHeadings(out)
	return out
}

// HeadingNodesMemoKey is CollectHeadingNodes's MemoFile key. Exported
// (rather than inlined at each use) so the shared-walk regression test
// in sharedheadingwalk_bench_test.go — which lives in the external
// astutil_test package because it exercises concrete rule packages
// (headingincrement, noduplicateheadings) that import astutil — can
// pre-seed the same cache entry without duplicating the string.
const HeadingNodesMemoKey = "astutil.headingNodes"

// CollectHeadingNodes returns every *ast.Heading node in the document,
// in source order. Unlike CollectSectionHeadings (which only exposes
// Level and Line), this keeps the node itself so a caller can extract
// heading text via HeadingText.
//
// Memoized per File via lint.File.MemoFile: MDS003 (heading-increment)
// and MDS005 (no-duplicate-headings) both previously ran their own
// full ast.Walk to collect the same headings; sharing one walk here
// means the second rule to run pays a cache hit instead of re-walking
// the tree (docs/development/high-performance-go.md, "memoize
// per-input computations").
func CollectHeadingNodes(f *lint.File) []*ast.Heading {
	return f.MemoFile(HeadingNodesMemoKey, buildHeadingNodes).([]*ast.Heading)
}

// buildHeadingNodes is the MemoFile-style builder for the heading-nodes
// memo. Defined at package scope so the value passed to MemoFile is a
// plain function pointer, matching buildSectionHeadings.
func buildHeadingNodes(f *lint.File) any {
	// Sized on the first heading rather than up front, so a document
	// with none keeps returning nil at zero allocations — the project's
	// "Return nil, not []T{}" convention, and the shape a pre-sized
	// slice would otherwise break.
	//
	// The root's child count is a capacity hint, not a bound: the walk
	// descends into blockquotes and list items, so nested headings can
	// still push past it and regrow. It is simply the cheapest estimate
	// that scales with document length, and it removes the ~log2(n)
	// regrowth steps this once-per-file memo would otherwise pay on
	// every file. See
	// docs/development/high-performance-go.md#allocations.
	var out []*ast.Heading
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		if out == nil {
			out = make([]*ast.Heading, 0, f.AST.ChildCount())
		}
		out = append(out, h)
		return ast.WalkSkipChildren, nil
	})
	return out
}

// sortSectionHeadings orders headings by source line in place.
// slices.SortFunc sorts the concrete SectionHeading values directly,
// unlike sort.Slice, which drives reflect.Swapper under the hood —
// see docs/development/high-performance-go.md's "reflect in hot
// paths" anti-pattern.
func sortSectionHeadings(headings []SectionHeading) {
	slices.SortFunc(headings, func(a, b SectionHeading) int {
		return cmp.Compare(a.Line, b.Line)
	})
}

// CollectSectionParagraphs returns every non-table paragraph with its
// 1-based source line and a reference to its AST node. Goldmark
// parses pipe-delimited tables as paragraphs when the table
// extension is absent; those are filtered so cell text does not
// pollute section bodies.
//
// The result is in ascending Line order — an artifact of the
// depth-first AST walk order combined with lint's parser config,
// which installs no extension (e.g. footnotes) that relocates nodes
// out of document order. SectionBodies depends on this ordering for
// its forward-only cursor; do not pass it a hand-built or
// externally-sorted slice.
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
	// Same shape as buildHeadingNodes: sized on the first paragraph so
	// a document with none stays nil at zero allocations, with the
	// root's child count as the capacity hint.
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
		if out == nil {
			out = make([]SectionParagraph, 0, f.AST.ChildCount())
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
//
// paragraphs must be sorted ascending by Line, as
// [CollectSectionParagraphs] returns them (document order). Callers
// that loop once per heading (MDS057, MDS058) each call this over the
// same paragraph slice with a monotonically non-decreasing start, so a
// binary search bounds the matching range in O(log len(paragraphs))
// instead of a full O(len(paragraphs)) scan per call — turning the
// per-file cost from O(headings * paragraphs) into
// O(headings * log paragraphs).
func SectionBody(paragraphs []SectionParagraph, source []byte, start, end int) string {
	lo := sort.Search(len(paragraphs), func(i int) bool { return paragraphs[i].Line >= start })
	hi := sort.Search(len(paragraphs), func(i int) bool { return paragraphs[i].Line >= end })
	if lo >= hi {
		return ""
	}
	parts := make([]string, 0, hi-lo)
	for _, p := range paragraphs[lo:hi] {
		parts = append(parts, p.ExtractText(source))
	}
	return strings.Join(parts, " ")
}

// SectionBodies returns SectionBody's result for every heading in
// headings, in order — the concatenated plain text of the paragraphs
// each heading's [start, SectionEnd(...)) range covers.
//
// Calling SectionBody once per heading rescans the full paragraphs
// slice from index 0 every time: O(headings × paragraphs). Since
// headings are in document order, each start line is non-decreasing
// across the loop, so a single cursor into paragraphs can advance
// forward-only and never revisit a paragraph it has already passed —
// O(headings + paragraphs) for the skip-ahead work, with the
// per-heading collection cost bounded by that heading's own section
// size (unavoidable: nested headings' bodies legitimately repeat
// their descendants' paragraphs). See
// docs/development/high-performance-go.md "Skip work you don't need".
//
// Trade-off: unlike the per-heading loop it replaces, every returned
// body string stays live for the whole call instead of becoming
// garbage as soon as its heading is processed. On a deeply nested
// document this raises peak live memory roughly to
// heading-depth × prose-size rather than max-section-size. Both
// current callers (MDS057, MDS058) are opt-in rules.
//
// The per-heading `parts` buffer is hoisted out of the loop and
// reused via `parts[:0]` rather than re-declared per heading — a
// per-heading `var parts []string` would regrow from nil every
// iteration (see docs/development/high-performance-go.md "Reuse
// loop-local buffers"), which measured as more allocations overall
// than the per-heading SectionBody loop this function replaces.
//
// Precondition: both headings and paragraphs must already be in
// ascending Line order — [CollectSectionHeadings] sorts its result
// explicitly, and [CollectSectionParagraphs] documents the same
// guarantee. Unlike SectionBody, which tolerates an unordered slice
// (it scans every entry unconditionally), the forward-only cursor
// here relies on both orderings: an out-of-order heading would leave
// `lo` already advanced past paragraphs a later heading needs, and
// an out-of-order paragraph past a heading's end would be silently
// skipped. Either produces a silently truncated body, not a panic.
func SectionBodies(headings []SectionHeading, paragraphs []SectionParagraph, source []byte, totalLines int) []string {
	if len(headings) == 0 {
		return nil
	}
	bodies := make([]string, len(headings))
	lo := 0
	var parts []string
	for i := range headings {
		start := headings[i].Line
		end := SectionEnd(headings, i, totalLines)
		for lo < len(paragraphs) && paragraphs[lo].Line < start {
			lo++
		}
		parts = parts[:0]
		for j := lo; j < len(paragraphs) && paragraphs[j].Line < end; j++ {
			parts = append(parts, paragraphs[j].ExtractText(source))
		}
		bodies[i] = strings.Join(parts, " ")
	}
	return bodies
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

// headingTextPool pools bytes.Buffer values used by HeadingText so
// heading-text extraction in the hot-path rule walk allocates zero
// buffers per call instead of one. Buffers beyond headingTextMaxPooledCap
// are discarded on release to prevent the LSP long-running process from
// retaining an oversized backing array indefinitely (mirrors the cap
// guard in mdtext.ExtractPlainText / extractTextMaxPooledCap).
var headingTextPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// headingTextMaxPooledCap is the maximum buffer capacity returned to
// headingTextPool. Heading text is always short (typically < 200 bytes);
// 4 KiB is a generous cap that prevents any pathological case from
// inflating the pool's steady-state footprint.
const headingTextMaxPooledCap = 4 * 1024

// HeadingText returns the plain-text content of a heading by
// recursively extracting all text segments from its children.
func HeadingText(heading *ast.Heading, source []byte) string {
	buf := headingTextPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer func() {
		if buf.Cap() <= headingTextMaxPooledCap {
			headingTextPool.Put(buf)
		}
	}()
	for c := heading.FirstChild(); c != nil; c = c.NextSibling() {
		ExtractText(c, source, buf)
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

// HeadingTextCached is HeadingText memoized per (f, heading) via
// lint.File.HeadingTextCache. Several default rules read a heading's
// text this way — no-trailing-punctuation and no-duplicate-headings
// for every heading; heading-increment and first-line-heading too, on
// a subset gated by their own rule-specific conditions — so more than
// one can independently call HeadingText for the same heading within
// one Check pass over f; caching lets only the first caller pay for
// the child walk and buf.String() conversion. Cached under base 0,
// matching HeadingTextBase(h, src, 0)'s documented equivalence to
// HeadingText(h, src) — see HeadingTextBaseCached.
func HeadingTextCached(f *lint.File, heading *ast.Heading) string {
	return f.HeadingTextCache(heading, 0, func() string { return HeadingText(heading, f.Source) })
}

// HeadingTextBaseCached is HeadingTextBase memoized per (f, heading,
// base); see HeadingTextCached. base is part of the cache key (not
// just captured by the compute closure), so a heading queried at two
// different bases — which HeadingText/HeadingTextBase would
// themselves disagree on — gets two independent cache entries rather
// than the second caller silently reading the first caller's answer.
func HeadingTextBaseCached(f *lint.File, heading *ast.Heading, base int) string {
	return f.HeadingTextCache(heading, base, func() string { return HeadingTextBase(heading, f.Source, base) })
}

// HeadingTextBase returns the plain-text content of a heading whose
// inline children carry run-local segment offsets, as on the parse-skipped
// path (lint.InlineBlocks re-parses each run in isolation). base is the
// run's start byte offset in source; it is added to every Text segment's
// bounds so the slices index the original document. With base == 0 the
// result is identical to HeadingText, which the AST path uses.
func HeadingTextBase(heading *ast.Heading, source []byte, base int) string {
	buf := headingTextPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer func() {
		if buf.Cap() <= headingTextMaxPooledCap {
			headingTextPool.Put(buf)
		}
	}()
	for c := heading.FirstChild(); c != nil; c = c.NextSibling() {
		extractTextBase(c, source, base, buf)
	}
	return buf.String()
}

// extractTextBase is ExtractText with a base offset added to each Text
// segment's bounds, so it reads run-local segments off the document source.
func extractTextBase(n ast.Node, source []byte, base int, buf *bytes.Buffer) {
	if t, ok := n.(*ast.Text); ok {
		buf.Write(source[base+t.Segment.Start : base+t.Segment.Stop])
		return
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		extractTextBase(c, source, base, buf)
	}
}

// CountLeadingSpaces returns the number of leading space characters
// (ASCII 0x20 only) in line.
func CountLeadingSpaces(line []byte) int {
	return len(line) - len(bytes.TrimLeft(line, " "))
}

// IsBlank reports whether line contains only space and tab characters.
func IsBlank(line []byte) bool {
	return len(bytes.TrimLeft(line, " \t")) == 0
}

// HeadingLineBase returns the 1-based source line of a heading whose inline
// children carry run-local segment offsets (the parse-skipped path). base is
// the run's start byte offset in f.Source. It mirrors HeadingLine exactly with
// every offset shifted by base: prefer heading.Lines().At(0) (setext), else the
// first descendant Text segment, else the constant 1 — so an empty heading with
// no Lines and no Text resolves to line 1 on both paths, not the run's line.
func HeadingLineBase(heading *ast.Heading, f *lint.File, base int) int {
	lines := heading.Lines()
	if lines.Len() > 0 {
		return f.LineOfOffset(base + lines.At(0).Start)
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
		line = f.LineOfOffset(base + t.Segment.Start)
		return ast.WalkStop, nil
	})
	return line
}
