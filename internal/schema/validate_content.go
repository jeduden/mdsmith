package schema

import (
	"fmt"
	"strings"
	"sync"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// ValidateContent runs the content-entry walker for every scope in
// sch that declares `content:`. A content entry constrains the
// shape of an AST node (code block, table, list, paragraph) inside
// the matched section's body. Diagnostics surface alongside the
// heading-tree results emitted by Validate.
//
// docFM is the document's parsed front matter; it threads through
// to the per-scope walker so `\#(fmvar(...))` matchers resolve when
// pairing a scope with its heading.
//
// The function does its own document parse with the table extension
// enabled — lint.NewFile's parser is CommonMark-only, so GFM tables
// would otherwise appear as paragraphs. The parse is skipped when
// the schema declares no content entries anywhere.
func ValidateContent(
	f *lint.File, sch *Schema, docFM map[string]any, mkDiag MakeDiag,
) []lint.Diagnostic {
	if sch == nil || !anyScopeHasContent(sch.Sections) {
		return nil
	}
	rootLevel := sch.EffectiveRootLevel()
	heads := skipContentBelow(ExtractDocHeadings(f), rootLevel)
	root := parseWithTableExt(f.Source)
	blocks := topLevelBlocks(f, root)

	var diags []lint.Diagnostic
	claimed := make(map[int]bool)
	walkContentScopes(
		f, sch.Sections, heads, rootLevel, 1, len(f.Lines)+1,
		claimed, blocks, docFM, mkDiag, &diags,
	)
	return diags
}

// anyScopeHasContent reports whether any scope (or nested scope) in
// scopes declares a `content:` list. Used to short-circuit the
// content pass on schemas that need no AST-block checks.
func anyScopeHasContent(scopes []Scope) bool {
	for _, sc := range scopes {
		if len(sc.Content) > 0 {
			return true
		}
		if anyScopeHasContent(sc.Sections) {
			return true
		}
	}
	return false
}

// skipContentBelow filters headings the same way the heading walker
// does: anything shallower than rootLevel is dropped so the content
// walker's section-boundary math stays consistent with the heading
// pass.
func skipContentBelow(heads []DocHeading, rootLevel int) []DocHeading {
	out := make([]DocHeading, 0, len(heads))
	for _, h := range heads {
		if h.Level >= rootLevel {
			out = append(out, h)
		}
	}
	return out
}

// contentParserPool reuses GFM-enabled goldmark parsers across
// ValidateContent calls. parser.Parser is only safe to reuse
// sequentially within a single goroutine — the LSP server's lint
// pipeline (and any future caller running passes in parallel)
// can run multiple ValidateContent invocations concurrently, so
// the pool hands each goroutine its own parser instance. Mirrors
// internal/lsp/index/build.go's parserPool.
var contentParserPool = sync.Pool{
	New: func() any {
		return goldmark.New(
			goldmark.WithExtensions(extension.Table),
			goldmark.WithParserOptions(
				parser.WithBlockParsers(lint.PIBlockParserPrioritized()),
			),
		).Parser()
	},
}

// parseWithTableExt re-parses source with a CommonMark + Table parser
// so the content walker can recognise GFM tables as *extast.Table
// rather than fallback paragraphs. The PI block parser is registered
// alongside the table extension so `<?include?>`, `<?catalog?>`, and
// other directives parse as ProcessingInstruction blocks — the same
// shape lint.NewParser produces — instead of HTML blocks that would
// shadow surrounding content and confuse the walker's match loop.
func parseWithTableExt(source []byte) ast.Node {
	p := contentParserPool.Get().(parser.Parser)
	defer contentParserPool.Put(p)
	return p.Parse(text.NewReader(source))
}

// topLevelBlocks returns the document's top-level block children in
// source order, annotated with their 1-based starting line. Headings
// are intentionally included: they bound section ranges but the walker
// filters them out per scope.
//
// blockLine reports 0 for the corner case where goldmark exposes no
// position (an empty fenced block with no info string and no content
// lines). A second pass back-fills those entries from neighbouring
// siblings — `next - 1` when the following block has a known line,
// else `prev + 1`, else 1 — so blocksInRange's [startLine, endLine)
// filter still places the block inside its enclosing section and
// diagnostic anchors never land at line 0.
func topLevelBlocks(f *lint.File, root ast.Node) []contentBlock {
	var out []contentBlock
	for c := root.FirstChild(); c != nil; c = c.NextSibling() {
		out = append(out, contentBlock{node: c, line: blockLine(f, c)})
	}
	return backfillBlockLines(out)
}

// backfillBlockLines replaces every contentBlock whose line is < 1
// with a position inferred from siblings. The inference walks each
// gap separately so a run of position-less blocks all settle into
// the same section without colliding with the surrounding heading
// boundaries.
func backfillBlockLines(blocks []contentBlock) []contentBlock {
	for i := range blocks {
		if blocks[i].line >= 1 {
			continue
		}
		blocks[i].line = inferBlockLine(blocks, i)
	}
	return blocks
}

func inferBlockLine(blocks []contentBlock, i int) int {
	// Prefer the next sibling with a known line, minus one — keeps
	// the block strictly before any heading that follows.
	for j := i + 1; j < len(blocks); j++ {
		if blocks[j].line >= 1 {
			if blocks[j].line > 1 {
				return blocks[j].line - 1
			}
			return blocks[j].line
		}
	}
	// Otherwise inherit the previous sibling's line + 1 so the block
	// lands strictly inside its section rather than on the heading.
	for j := i - 1; j >= 0; j-- {
		if blocks[j].line >= 1 {
			return blocks[j].line + 1
		}
	}
	return 1
}

// contentBlock pairs a top-level block AST node with its 1-based
// source line. Caching the line up front keeps section-range
// filtering cheap.
type contentBlock struct {
	node ast.Node
	line int
}

// blockLine returns the 1-based source line of the first visible
// token of a block-level AST node. Fenced code blocks route through
// lint.FindFencedOpenLine so the result anchors at the opening
// fence — info string when present, first-content-line-minus-one
// otherwise — matching the line numbers the rest of the engine
// reports for the same nodes. Other block kinds fall back to the
// first Lines() segment, then to a descendant scan for empty
// containers.
//
// Returns 0 when goldmark exposes no position for n (the truly-
// empty fenced block with no info string and no content). Callers
// that anchor diagnostics or filter by section line range must
// route through topLevelBlocks, which back-fills these unknown
// positions from sibling blocks so the missing-position case never
// surfaces as a line-0 diagnostic.
func blockLine(f *lint.File, n ast.Node) int {
	if fcb, ok := n.(*ast.FencedCodeBlock); ok {
		return lint.FindFencedOpenLine(f, fcb)
	}
	if n.Lines().Len() > 0 {
		return f.LineOfOffset(n.Lines().At(0).Start)
	}
	line := 0
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || c == n {
			return ast.WalkContinue, nil
		}
		if c.Lines().Len() > 0 {
			line = f.LineOfOffset(c.Lines().At(0).Start)
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return line
}

// walkContentScopes mirrors scope_rules.walkScopes: pair each scope
// with a doc heading, compute its body line range, and invoke the
// content validator. Diagnostics for matched scopes are appended to
// *diags.
//
// claimed tracks heading indices that have been paired with a scope.
// parentStart / parentEnd bound the parent section's content range so
// a nested walk doesn't drift outside it.
func walkContentScopes(
	f *lint.File, scopes []Scope, heads []DocHeading,
	expectedLevel, parentStart, parentEnd int,
	claimed map[int]bool, blocks []contentBlock,
	docFM map[string]any, mkDiag MakeDiag, diags *[]lint.Diagnostic,
) {
	for i, sc := range scopes {
		if isSlotMatcher(sc.Matcher) {
			continue
		}
		if sc.Preamble {
			end := firstContentHeadingLine(heads, expectedLevel, parentStart, parentEnd)
			// Anchor preamble diagnostics at parentStart (line 1 for
			// the root preamble) rather than parentStart-1: a line-0
			// diagnostic has no source location and confuses editor
			// jump-to-line. The validator labels the parent with
			// "preamble" instead of formatHeading so an empty
			// sc.Heading does not render as a bare `## `.
			runContent(f, sc, parentStart, expectedLevel, parentStart, end, blocks, mkDiag, diags)
			continue
		}
		// ScopeRunIndices applies the structural validator's
		// run + yield semantics: contiguous matches only, with
		// broad-and-after-min yielding to later named scopes.
		for _, matched := range ScopeRunIndices(
			scopes, i, heads, expectedLevel, parentStart, parentEnd, claimed, docFM) {
			dh := heads[matched]
			claimed[matched] = true
			end := contentScopeEndLine(heads, matched, dh.Level, parentEnd)
			runContent(f, sc, dh.Line, dh.Level, dh.Line+1, end, blocks, mkDiag, diags)
			if len(sc.Sections) > 0 {
				walkContentScopes(
					f, sc.Sections, heads, expectedLevel+1, dh.Line, end,
					claimed, blocks, docFM, mkDiag, diags,
				)
			}
		}
	}
}

// runContent invokes the content-entry walker when sc declares a
// non-empty Content list. The body range is [startLine, endLine);
// sectionLine is where "missing required" diagnostics anchor.
func runContent(
	f *lint.File, sc Scope, sectionLine, sectionLevel int,
	startLine, endLine int, blocks []contentBlock,
	mkDiag MakeDiag, diags *[]lint.Diagnostic,
) {
	if len(sc.Content) == 0 {
		return
	}
	nodes := blocksInRange(blocks, startLine, endLine)
	*diags = append(*diags,
		validateContentEntries(
			f, sc, sectionLine, sectionLevel, nodes, mkDiag)...)
}

// blocksInRange returns the contentBlock entries whose start line is
// in [startLine, endLine), with heading nodes filtered out. The
// remaining blocks are the section's body in source order.
func blocksInRange(blocks []contentBlock, startLine, endLine int) []contentBlock {
	var out []contentBlock
	for _, b := range blocks {
		if b.line < startLine || b.line >= endLine {
			continue
		}
		if _, isHeading := b.node.(*ast.Heading); isHeading {
			continue
		}
		out = append(out, b)
	}
	return out
}

// contentScopeEndLine returns the exclusive end-line of a section
// matched at heads[matched]. The boundary level is the matched
// heading's own level so siblings at the same level terminate the
// range and ancestors at a shallower level also terminate it; deeper
// headings stay inside.
func contentScopeEndLine(
	heads []DocHeading, matched, boundaryLevel, parentEnd int,
) int {
	for j := matched + 1; j < len(heads); j++ {
		if heads[j].Line >= parentEnd {
			break
		}
		if heads[j].Level <= boundaryLevel {
			return heads[j].Line
		}
	}
	return parentEnd
}

// firstContentHeadingLine returns the line of the first heading at
// expectedLevel within the parent window, or parentEnd when none
// exists. Used to size a preamble's content range — preamble runs
// from parentStart up to (but not including) the first listed
// section at this level.
func firstContentHeadingLine(
	heads []DocHeading, expectedLevel, parentStart, parentEnd int,
) int {
	for _, h := range heads {
		if h.Line < parentStart || h.Line >= parentEnd {
			continue
		}
		if h.Level == expectedLevel {
			return h.Line
		}
	}
	return parentEnd
}

// validateContentEntries pairs each content entry with one of the
// section's body blocks. Mirrors the heading-tree walker's claim /
// out-of-order / unlisted-slot semantics; the only kind-specific
// behaviour lives inside nodeMatchesKind and shapeDiags.
//
// sectionLine is the heading line the "missing required" diagnostic
// anchors at; sectionLevel + scopeHeading feed the formatting helper
// used to name the parent section in error text.
func validateContentEntries(
	f *lint.File, sc Scope, sectionLine, sectionLevel int,
	nodes []contentBlock, mkDiag MakeDiag,
) []lint.Diagnostic {
	var diags []lint.Diagnostic
	w := contentWalker{
		f:            f,
		sc:           sc,
		sectionLine:  sectionLine,
		sectionLevel: sectionLevel,
		nodes:        nodes,
		mkDiag:       mkDiag,
		claimed:      make(map[int]bool, len(sc.Content)),
	}
	w.run(&diags)
	return diags
}

// contentWalker holds the running state of a single content-entry
// match pass: the current node index, which entries are already
// claimed out-of-order, and whether an `unlisted` slot is currently
// open so trailing non-matching nodes are tolerated.
type contentWalker struct {
	f            *lint.File
	sc           Scope
	sectionLine  int
	sectionLevel int
	nodes        []contentBlock
	mkDiag       MakeDiag

	nodeIdx    int
	claimed    map[int]bool
	allowExtra bool
}

func (w *contentWalker) run(diags *[]lint.Diagnostic) {
	for i, entry := range w.sc.Content {
		if entry.Kind == ContentKindUnlisted {
			w.allowExtra = true
			continue
		}
		if w.claimed[i] {
			continue
		}
		w.matchEntry(i, entry, diags)
	}
	w.flushTrailing(diags)
}

// matchEntry advances nodeIdx looking for a node that matches the
// entry's kind. Intervening nodes are either claimed as an
// out-of-order match for a later listed entry, flagged as unexpected
// (closed scope, no open slot), or silently consumed. On loop exit
// w.claimed[i] reports whether the entry was paired with a node; an
// unclaimed required entry emits a "missing required" diagnostic
// anchored at the section's heading line.
func (w *contentWalker) matchEntry(
	i int, entry ContentEntry, diags *[]lint.Diagnostic,
) {
	for w.nodeIdx < len(w.nodes) {
		n := w.nodes[w.nodeIdx]
		if nodeMatchesKind(entry.Kind, n.node) {
			*diags = append(*diags, shapeDiags(w.f, entry, n, w.mkDiag)...)
			w.claimed[i] = true
			w.nodeIdx++
			w.allowExtra = false
			return
		}
		if ooIdx := w.findLaterEntry(i+1, n.node); ooIdx >= 0 {
			if !entry.Required {
				return
			}
			ooEntry := w.sc.Content[ooIdx]
			*diags = append(*diags, w.mkDiag(
				w.f.Path, n.line,
				fmt.Sprintf("content %q out of order: expected after %q",
					describeNode(w.f, n.node), describeEntry(entry))))
			*diags = append(*diags, shapeDiags(w.f, ooEntry, n, w.mkDiag)...)
			w.claimed[ooIdx] = true
			w.nodeIdx++
			continue
		}
		if !w.allowExtra && w.sc.Closed {
			*diags = append(*diags, w.mkDiag(
				w.f.Path, n.line,
				fmt.Sprintf("unexpected content %q inside %s (expected %q)",
					describeNode(w.f, n.node),
					scopeLabel(w.sc, w.sectionLevel),
					describeEntry(entry))))
		}
		w.nodeIdx++
	}
	if !w.claimed[i] && entry.Required {
		*diags = append(*diags, w.mkDiag(
			w.f.Path, w.sectionLine,
			fmt.Sprintf("missing required content %q inside %s",
				describeEntry(entry),
				scopeLabel(w.sc, w.sectionLevel))))
	}
}

// findLaterEntry returns the index of the first unclaimed listed
// entry at or after startIdx whose kind matches n, or -1 when none
// exists. Unlisted slots are skipped — they never claim a specific
// node by kind.
func (w *contentWalker) findLaterEntry(startIdx int, n ast.Node) int {
	for j := startIdx; j < len(w.sc.Content); j++ {
		if w.claimed[j] {
			continue
		}
		e := w.sc.Content[j]
		if e.Kind == ContentKindUnlisted {
			continue
		}
		if nodeMatchesKind(e.Kind, n) {
			return j
		}
	}
	return -1
}

// flushTrailing handles body blocks left after the last entry was
// processed. In a closed scope with no open `unlisted` slot, each
// trailing block produces an unexpected-content diagnostic; otherwise
// they are tolerated silently.
func (w *contentWalker) flushTrailing(diags *[]lint.Diagnostic) {
	if w.allowExtra || !w.sc.Closed {
		return
	}
	for w.nodeIdx < len(w.nodes) {
		n := w.nodes[w.nodeIdx]
		*diags = append(*diags, w.mkDiag(
			w.f.Path, n.line,
			fmt.Sprintf("unexpected content %q inside %s",
				describeNode(w.f, n.node),
				scopeLabel(w.sc, w.sectionLevel))))
		w.nodeIdx++
	}
}

// nodeMatchesKind returns true when n is the AST shape named by kind.
// The match is shape-only — `code-block` matches any fenced code
// block regardless of its info string; sub-shape constraints are
// reported by shapeDiags after the slot is claimed.
func nodeMatchesKind(kind string, n ast.Node) bool {
	switch kind {
	case ContentKindCodeBlock:
		_, ok := n.(*ast.FencedCodeBlock)
		return ok
	case ContentKindTable:
		_, ok := n.(*extast.Table)
		return ok
	case ContentKindList:
		_, ok := n.(*ast.List)
		return ok
	case ContentKindParagraph:
		_, ok := n.(*ast.Paragraph)
		return ok
	}
	return false
}

// shapeDiags emits sub-shape diagnostics for a claimed match. A
// code-block whose language differs from the entry's required Lang,
// a table whose header row differs from the required Columns, or a
// list whose order/item count violates the entry's constraints
// produces a diagnostic here. The slot itself is already considered
// claimed — these are kind-specific refinements layered on top.
func shapeDiags(
	f *lint.File, entry ContentEntry, b contentBlock, mkDiag MakeDiag,
) []lint.Diagnostic {
	switch entry.Kind {
	case ContentKindCodeBlock:
		return codeBlockShapeDiags(f, entry, b, mkDiag)
	case ContentKindTable:
		return tableShapeDiags(f, entry, b, mkDiag)
	case ContentKindList:
		return listShapeDiags(f, entry, b, mkDiag)
	}
	return nil
}

// codeBlockShapeDiags is only invoked after nodeMatchesKind has
// confirmed b.node is *ast.FencedCodeBlock; the direct type
// assertion would panic on a programming error, which is preferable
// to a silent no-op.
func codeBlockShapeDiags(
	f *lint.File, entry ContentEntry, b contentBlock, mkDiag MakeDiag,
) []lint.Diagnostic {
	if entry.Lang == "" {
		return nil
	}
	fcb := b.node.(*ast.FencedCodeBlock)
	lang := string(fcb.Language(f.Source))
	if lang == entry.Lang {
		return nil
	}
	return []lint.Diagnostic{mkDiag(f.Path, b.line,
		fmt.Sprintf("code block language %q does not match required %q",
			lang, entry.Lang))}
}

// tableShapeDiags relies on nodeMatchesKind to have confirmed the
// table type, mirroring codeBlockShapeDiags' contract.
func tableShapeDiags(
	f *lint.File, entry ContentEntry, b contentBlock, mkDiag MakeDiag,
) []lint.Diagnostic {
	if len(entry.Columns) == 0 {
		return nil
	}
	tbl := b.node.(*extast.Table)
	cols := tableHeaderColumns(tbl, f.Source)
	if stringSlicesEqual(cols, entry.Columns) {
		return nil
	}
	return []lint.Diagnostic{mkDiag(f.Path, b.line,
		fmt.Sprintf("table headers %v do not match required %v",
			cols, entry.Columns))}
}

// listShapeDiags relies on nodeMatchesKind to have confirmed the
// list type, mirroring codeBlockShapeDiags' contract.
func listShapeDiags(
	f *lint.File, entry ContentEntry, b contentBlock, mkDiag MakeDiag,
) []lint.Diagnostic {
	lst := b.node.(*ast.List)
	var diags []lint.Diagnostic
	if entry.OrderedSet && lst.IsOrdered() != entry.Ordered {
		diags = append(diags, mkDiag(f.Path, b.line,
			fmt.Sprintf("list ordered=%v does not match required ordered=%v",
				lst.IsOrdered(), entry.Ordered)))
	}
	count := listItemCount(lst)
	if entry.MinItems > 0 && count < entry.MinItems {
		diags = append(diags, mkDiag(f.Path, b.line,
			fmt.Sprintf("list has %d items, required at least %d",
				count, entry.MinItems)))
	}
	if entry.MaxItems > 0 && count > entry.MaxItems {
		diags = append(diags, mkDiag(f.Path, b.line,
			fmt.Sprintf("list has %d items, required at most %d",
				count, entry.MaxItems)))
	}
	return diags
}

// listItemCount returns the number of immediate ListItem children of
// l. Nested lists are not counted.
func listItemCount(l *ast.List) int {
	count := 0
	for c := l.FirstChild(); c != nil; c = c.NextSibling() {
		if _, ok := c.(*ast.ListItem); ok {
			count++
		}
	}
	return count
}

// tableHeaderColumns extracts the text content of the first row of
// tbl — typically a *extast.TableHeader holding *extast.TableCell
// children. Cell labels are extracted via mdtext.ExtractPlainText so
// inline code spans, smart-quote/autolink *ast.String nodes, and
// other inline extensions round-trip through the header text the
// same way they do everywhere else in the engine.
func tableHeaderColumns(tbl *extast.Table, source []byte) []string {
	header := tbl.FirstChild()
	if header == nil {
		return nil
	}
	var cells []string
	for c := header.FirstChild(); c != nil; c = c.NextSibling() {
		if _, ok := c.(*extast.TableCell); !ok {
			continue
		}
		cells = append(cells, strings.TrimSpace(mdtext.ExtractPlainText(c, source)))
	}
	return cells
}

// stringSlicesEqual reports whether a and b have identical length
// and equal elements in order. Used for table-column comparison
// where order matters.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// scopeLabel renders the parent scope as the short string used in
// content diagnostics — `## Heading` for listed scopes, `preamble`
// for `heading: null`. The preamble case is the reason this helper
// exists: formatHeading would render an empty heading text as a
// bare `## ` and bury the user in confusion.
func scopeLabel(sc Scope, level int) string {
	if sc.Preamble {
		return "preamble"
	}
	return formatHeading(level, sc.Heading)
}

// describeEntry renders a content entry as the short string used in
// diagnostic text — `"code-block lang=yaml"`, `"table columns=[…]"`,
// `"list ordered=true min-items=2"`, etc. The format is stable so
// docs and fixtures can pin against it. Only the four listed entry
// kinds are rendered; `unlisted` entries never reach this function
// because matchEntry and findLaterEntry skip them before any
// diagnostic is formatted.
func describeEntry(e ContentEntry) string {
	switch e.Kind {
	case ContentKindCodeBlock:
		if e.Lang != "" {
			return fmt.Sprintf("code-block lang=%s", e.Lang)
		}
		return "code-block"
	case ContentKindTable:
		if len(e.Columns) > 0 {
			return fmt.Sprintf("table columns=%v", e.Columns)
		}
		return "table"
	case ContentKindList:
		parts := []string{"list"}
		if e.OrderedSet {
			parts = append(parts, fmt.Sprintf("ordered=%v", e.Ordered))
		}
		if e.MinItems > 0 {
			parts = append(parts, fmt.Sprintf("min-items=%d", e.MinItems))
		}
		if e.MaxItems > 0 {
			parts = append(parts, fmt.Sprintf("max-items=%d", e.MaxItems))
		}
		return strings.Join(parts, " ")
	}
	return "paragraph"
}

// describeNode renders an AST node as the short string used in
// diagnostic text — matches the kind names used in entry
// descriptions so error pairs read coherently. A code block carries
// its language when present; a list carries its ordered flag.
func describeNode(f *lint.File, n ast.Node) string {
	switch x := n.(type) {
	case *ast.FencedCodeBlock:
		lang := string(x.Language(f.Source))
		if lang != "" {
			return fmt.Sprintf("code-block lang=%s", lang)
		}
		return "code-block"
	case *extast.Table:
		return "table"
	case *ast.List:
		return fmt.Sprintf("list ordered=%v", x.IsOrdered())
	case *ast.Paragraph:
		return "paragraph"
	}
	return n.Kind().String()
}
