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

func collectPIBlockLines(f *File) map[int]struct{} {
	lines := map[int]struct{}{}
	collectPIBlockLinesInto(f.AST, f, lines)
	return lines
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

func collectCodeBlockLines(f *File) map[int]struct{} {
	lines := map[int]struct{}{}
	collectCodeBlockLinesInto(f.AST, f, lines)
	return lines
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
	// Determine the opening fence line by looking at the node's info or
	// the first content line. The opening fence is always the line before
	// the first content line (or, when there are no content lines, we find
	// it via the Info segment).
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
// fence. Returns 0 when the block has neither an info string nor any
// content lines — the truly-empty fenced shape that goldmark exposes
// no source position for. Callers must NOT clamp 0 to 1 for section-
// range filtering or diagnostic anchoring: clamping would mis-locate
// the block at the top of the document and silently move any
// diagnostic to a line that has nothing to do with the source. The
// preferred fallback is sibling-derived inference — see
// internal/schema.topLevelBlocks for an implementation that walks
// adjacent blocks to recover a sensible position.
func FindFencedOpenLine(f *File, fcb *ast.FencedCodeBlock) int {
	// If the code block has an info string, walk backwards from it to find
	// the start of the line.
	if fcb.Info != nil {
		return f.LineOfOffset(fcb.Info.Segment.Start)
	}
	// If there are content lines, the opening fence is on the previous line.
	if fcb.Lines().Len() > 0 {
		firstContentLine := f.LineOfOffset(fcb.Lines().At(0).Start)
		if firstContentLine > 1 {
			return firstContentLine - 1
		}
		return 1
	}
	// Empty fenced code block with no info: scan from the node's text position.
	// Fall back to using previous sibling or document start.
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
