package lint

import (
	"github.com/jeduden/mdsmith/internal/piparser"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// CollectPIBlockLines returns a set of 1-based line numbers that belong
// to processing-instruction blocks, including the opening <?... line and
// the closing ?> line. The walk is computed once per File and cached;
// the returned map is shared read-only and must not be mutated. The
// atomic.Bool + mutex memo avoids the once.Do closure box (see the
// File.piBlockLines field comment).
func CollectPIBlockLines(f *File) map[int]struct{} {
	if f.piBlockLinesDone.Load() {
		return f.piBlockLines
	}
	f.piBlockLinesMu.Lock()
	defer f.piBlockLinesMu.Unlock()
	if !f.piBlockLinesDone.Load() {
		defer f.piBlockLinesDone.Store(true)
		f.piBlockLines = collectPIBlockLines(f)
	}
	return f.piBlockLines
}

// collectPIBlockLines computes the PI line set. It prefers an
// already-computed Layer 0 scan, falls back to the AST walk when the tree
// is present, and computes Layer 0 on demand when the parse was skipped
// (f.AST == nil). The three paths produce a byte-identical set; the Layer
// 0 equivalence harness gates that invariant across the corpus.
func collectPIBlockLines(f *File) map[int]struct{} {
	if f.layer0Done.Load() {
		return f.layer0.PIBlockLines
	}
	if f.AST != nil {
		lines := map[int]struct{}{}
		collectPIBlockLinesInto(f.AST, f, lines)
		return lines
	}
	return Layer0(f).PIBlockLines
}

// collectPIBlockLinesInto descends node n via recursion (not
// ast.Walk) so the per-File memo build sheds the closure box
// ast.Walk would otherwise allocate. The helper closes over
// nothing.
func collectPIBlockLinesInto(n ast.Node, f *File, lines map[int]struct{}) {
	if n == nil {
		return
	}
	if pi, ok := n.(*piparser.ProcessingInstruction); ok {
		segs := pi.Lines()
		for i := 0; i < segs.Len(); i++ {
			lines[f.LineOfOffset(segs.At(i).Start)] = struct{}{}
		}
		if pi.HasClosure() {
			lines[f.LineOfOffset(pi.ClosureLine.Start)] = struct{}{}
		}
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		collectPIBlockLinesInto(c, f, lines)
	}
}

// InCodeOrPI reports whether the 1-based line is present in codeLines or
// piLines. It checks codeLines first and returns early, so the piLines
// lookup is skipped for a line already known to sit inside a code block —
// preserving the short-circuit of the original `codeLines[l] || piLines[l]`
// expression while keeping each call site to a single statement.
func InCodeOrPI(codeLines, piLines map[int]struct{}, line int) bool {
	if _, ok := codeLines[line]; ok {
		return true
	}
	_, ok := piLines[line]
	return ok
}

// CollectCodeBlockLines returns a set of 1-based line numbers that
// belong to fenced code blocks (including fence lines) or indented code
// blocks. The walk is computed once per File and cached; the returned
// map is shared read-only and must not be mutated. The atomic.Bool +
// mutex memo avoids the once.Do closure box (see the
// File.codeBlockLines field comment).
func CollectCodeBlockLines(f *File) map[int]struct{} {
	// Flat Layer-0 path (plan 2606142147): when the File was built by the
	// engine's parse-skip path it carries a flat classifier and no AST, so
	// serve the code-block set the classifier already computed. The
	// equivalence gate pins this byte-identical to the AST walk below.
	if f.lineClass != nil {
		return f.lineClass.CodeBlockLines()
	}
	if f.codeBlockLinesDone.Load() {
		return f.codeBlockLines
	}
	f.codeBlockLinesMu.Lock()
	defer f.codeBlockLinesMu.Unlock()
	if !f.codeBlockLinesDone.Load() {
		defer f.codeBlockLinesDone.Store(true)
		f.codeBlockLines = collectCodeBlockLines(f)
	}
	return f.codeBlockLines
}

// collectCodeBlockLines computes the code-block line set. It prefers an
// already-computed Layer 0 scan, falls back to the AST walk when the tree
// is present, and computes Layer 0 on demand when the parse was skipped
// (f.AST == nil). The three paths produce a byte-identical set; the Layer
// 0 equivalence harness gates that invariant across the corpus.
func collectCodeBlockLines(f *File) map[int]struct{} {
	if f.layer0Done.Load() {
		return f.layer0.CodeBlockLines
	}
	if f.AST != nil {
		lines := map[int]struct{}{}
		collectCodeBlockLinesInto(f.AST, f, lines)
		return lines
	}
	return Layer0(f).CodeBlockLines
}

// collectCodeBlockLinesInto descends node n via recursion (no
// closure box) and folds every fenced or indented code block's
// content lines into the supplied set. Matches the previous
// ast.Walk shape byte-for-byte; the helper closes over nothing.
func collectCodeBlockLinesInto(n ast.Node, f *File, lines map[int]struct{}) {
	if n == nil {
		return
	}
	switch cb := n.(type) {
	case *ast.FencedCodeBlock:
		addFencedCodeBlockLines(f, cb, lines)
	case *ast.CodeBlock:
		addBlockLines(f, cb, lines)
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		collectCodeBlockLinesInto(c, f, lines)
	}
}

// addFencedCodeBlockLines marks the opening fence line, all content lines,
// and the closing fence line.
func addFencedCodeBlockLines(f *File, fcb *ast.FencedCodeBlock, set map[int]struct{}) {
	openLine := FindFencedOpenLine(f, fcb)
	if openLine > 0 {
		set[openLine] = struct{}{}
	}

	// Content lines from the code block's segments.
	segs := fcb.Lines()
	lastContentLine := 0
	for i := 0; i < segs.Len(); i++ {
		seg := segs.At(i)
		ln := f.LineOfOffset(seg.Start)
		set[ln] = struct{}{}
		if ln > lastContentLine {
			lastContentLine = ln
		}
	}

	// Closing fence line is the line after the last content line.
	// If there are no content lines, the closing fence is the line after
	// the opening fence.
	closeLine := 0
	if lastContentLine > 0 {
		closeLine = lastContentLine + 1
	} else if openLine > 0 {
		closeLine = openLine + 1
	}
	if closeLine > 0 && closeLine <= len(f.Lines) {
		set[closeLine] = struct{}{}
	}
}

// FindFencedOpenLine returns the 1-based line number of the opening
// fence. The parser records the opener's offset as the node position,
// so the line holding it is the opening fence line for every parsed
// block — with or without an info string or content, at top level or
// inside a list item or block quote (fencepos.OpenLineRange reads the
// same position). Returns 0 only for a node without a position inside
// the source (one built by hand, not parsed). Callers must NOT clamp 0
// to 1 for section-range filtering or diagnostic anchoring: clamping
// would mis-locate the block at the top of the document. See
// internal/schema.topLevelBlocks for the sibling-derived fallback.
func FindFencedOpenLine(f *File, fcb *ast.FencedCodeBlock) int {
	if p := fcb.Pos(); p >= 0 && p < len(f.Source) {
		return f.LineOfOffset(p)
	}
	return 0
}

// addBlockLines marks all content lines of an indented code block.
func addBlockLines(f *File, cb *ast.CodeBlock, set map[int]struct{}) {
	segs := cb.Lines()
	for i := 0; i < segs.Len(); i++ {
		seg := segs.At(i)
		ln := f.LineOfOffset(seg.Start)
		set[ln] = struct{}{}
	}
}
