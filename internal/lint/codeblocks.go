package lint

import "github.com/yuin/goldmark/ast"

// CollectPIBlockLines returns a set of 1-based line numbers that belong
// to processing-instruction blocks, including the opening <?... line and
// the closing ?> line. The walk is computed once per File and cached;
// the returned map is shared read-only and must not be mutated. The
// atomic.Bool + mutex memo avoids the once.Do closure box (see the
// File.piBlockLines field comment).
func CollectPIBlockLines(f *File) map[int]bool {
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

func collectPIBlockLines(f *File) map[int]bool {
	lines := map[int]bool{}
	collectPIBlockLinesInto(f.AST, f, lines)
	return lines
}

// collectPIBlockLinesInto descends node n via recursion (not
// ast.Walk) so the per-File memo build sheds the closure box
// ast.Walk would otherwise allocate. The helper closes over
// nothing.
func collectPIBlockLinesInto(n ast.Node, f *File, lines map[int]bool) {
	if n == nil {
		return
	}
	if pi, ok := n.(*ProcessingInstruction); ok {
		segs := pi.Lines()
		for i := 0; i < segs.Len(); i++ {
			lines[f.LineOfOffset(segs.At(i).Start)] = true
		}
		if pi.HasClosure() {
			lines[f.LineOfOffset(pi.ClosureLine.Start)] = true
		}
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		collectPIBlockLinesInto(c, f, lines)
	}
}

// CollectCodeBlockLines returns a set of 1-based line numbers that
// belong to fenced code blocks (including fence lines) or indented code
// blocks. The walk is computed once per File and cached; the returned
// map is shared read-only and must not be mutated. The atomic.Bool +
// mutex memo avoids the once.Do closure box (see the
// File.codeBlockLines field comment).
func CollectCodeBlockLines(f *File) map[int]bool {
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

func collectCodeBlockLines(f *File) map[int]bool {
	lines := map[int]bool{}
	collectCodeBlockLinesInto(f.AST, f, lines)
	return lines
}

// collectCodeBlockLinesInto descends node n via recursion (no
// closure box) and folds every fenced or indented code block's
// content lines into the supplied set. Matches the previous
// ast.Walk shape byte-for-byte; the helper closes over nothing.
func collectCodeBlockLinesInto(n ast.Node, f *File, lines map[int]bool) {
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
func addFencedCodeBlockLines(f *File, fcb *ast.FencedCodeBlock, set map[int]bool) {
	// Determine the opening fence line by looking at the node's info or
	// the first content line. The opening fence is always the line before
	// the first content line (or, when there are no content lines, we find
	// it via the Info segment).
	openLine := FindFencedOpenLine(f, fcb)
	if openLine > 0 {
		set[openLine] = true
	}

	// Content lines from the code block's segments.
	segs := fcb.Lines()
	lastContentLine := 0
	for i := 0; i < segs.Len(); i++ {
		seg := segs.At(i)
		ln := f.LineOfOffset(seg.Start)
		set[ln] = true
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
		set[closeLine] = true
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
func addBlockLines(f *File, cb *ast.CodeBlock, set map[int]bool) {
	segs := cb.Lines()
	for i := 0; i < segs.Len(); i++ {
		seg := segs.At(i)
		ln := f.LineOfOffset(seg.Start)
		set[ln] = true
	}
}
