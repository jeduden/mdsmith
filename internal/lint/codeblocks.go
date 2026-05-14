package lint

import "github.com/yuin/goldmark/ast"

// CollectPIBlockLines walks the AST and returns a set of 1-based line numbers
// that belong to processing-instruction blocks, including the opening <?...
// line and the closing ?> line.
func CollectPIBlockLines(f *File) map[int]bool {
	lines := map[int]bool{}
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		pi, ok := n.(*ProcessingInstruction)
		if !ok {
			return ast.WalkContinue, nil
		}
		segs := pi.Lines()
		for i := 0; i < segs.Len(); i++ {
			lines[f.LineOfOffset(segs.At(i).Start)] = true
		}
		if pi.HasClosure() {
			lines[f.LineOfOffset(pi.ClosureLine.Start)] = true
		}
		return ast.WalkContinue, nil
	})
	return lines
}

// CollectCodeBlockLines walks the AST and returns a set of 1-based line
// numbers that belong to fenced code blocks (including fence lines) or
// indented code blocks.
func CollectCodeBlockLines(f *File) map[int]bool {
	lines := map[int]bool{}

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch cb := n.(type) {
		case *ast.FencedCodeBlock:
			addFencedCodeBlockLines(f, cb, lines)
		case *ast.CodeBlock:
			addBlockLines(f, cb, lines)
		}

		return ast.WalkContinue, nil
	})

	return lines
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
