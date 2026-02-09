package fencedcodestyle

import (
	"bytes"

	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func init() {
	rule.Register(&Rule{Style: "backtick"})
}

// Rule checks that fenced code blocks use a consistent fence style.
// Default style is "backtick". Set Style to "tilde" for tilde fences.
type Rule struct {
	Style string // "backtick" or "tilde"
}

func (r *Rule) ID() string   { return "TM010" }
func (r *Rule) Name() string { return "fenced-code-style" }

func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic

	ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		fcb, ok := n.(*ast.FencedCodeBlock)
		if !ok {
			return ast.WalkContinue, nil
		}

		openStart, _ := fenceOpenLineRange(f.Source, fcb)
		if openStart >= len(f.Source) {
			return ast.WalkContinue, nil
		}

		fenceChar := fenceCharAt(f.Source, openStart)
		if fenceChar == 0 {
			return ast.WalkContinue, nil
		}

		wantChar := r.wantChar()
		if fenceChar != wantChar {
			line := f.LineOfOffset(openStart)
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     line,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  "fenced code block should use " + r.Style + " style",
			})
		}

		return ast.WalkContinue, nil
	})

	return diags
}

func (r *Rule) Fix(f *lint.File) []byte {
	type fenceRange struct {
		openStart, openEnd   int
		closeStart, closeEnd int
	}
	var ranges []fenceRange

	ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		fcb, ok := n.(*ast.FencedCodeBlock)
		if !ok {
			return ast.WalkContinue, nil
		}

		openStart, openEnd := fenceOpenLineRange(f.Source, fcb)
		if openStart >= len(f.Source) {
			return ast.WalkContinue, nil
		}

		fenceChar := fenceCharAt(f.Source, openStart)
		if fenceChar == 0 {
			return ast.WalkContinue, nil
		}

		wantChar := r.wantChar()
		if fenceChar != wantChar {
			closeStart, closeEnd := fenceCloseLineRange(f.Source, fcb, openEnd)
			ranges = append(ranges, fenceRange{
				openStart: openStart, openEnd: openEnd,
				closeStart: closeStart, closeEnd: closeEnd,
			})
		}

		return ast.WalkContinue, nil
	})

	if len(ranges) == 0 {
		return f.Source
	}

	wantChar := r.wantChar()
	result := make([]byte, 0, len(f.Source))
	prev := 0
	for _, fr := range ranges {
		result = append(result, f.Source[prev:fr.openStart]...)
		result = append(result, replaceFenceChars(f.Source[fr.openStart:fr.openEnd], wantChar)...)
		result = append(result, f.Source[fr.openEnd:fr.closeStart]...)
		result = append(result, replaceFenceChars(f.Source[fr.closeStart:fr.closeEnd], wantChar)...)
		prev = fr.closeEnd
	}
	result = append(result, f.Source[prev:]...)
	return result
}

func (r *Rule) wantChar() byte {
	if r.Style == "tilde" {
		return '~'
	}
	return '`'
}

// fenceCharAt returns the fence character at the given position, skipping leading spaces.
func fenceCharAt(src []byte, pos int) byte {
	for pos < len(src) && src[pos] == ' ' {
		pos++
	}
	if pos < len(src) && (src[pos] == '`' || src[pos] == '~') {
		return src[pos]
	}
	return 0
}

// replaceFenceChars replaces backtick or tilde chars in a fence line with the target char,
// preserving count, leading spaces, and any info string.
func replaceFenceChars(line []byte, targetChar byte) []byte {
	result := make([]byte, len(line))
	copy(result, line)
	i := 0
	// Skip leading spaces
	for i < len(result) && result[i] == ' ' {
		i++
	}
	// Replace fence characters
	for i < len(result) && (result[i] == '`' || result[i] == '~') {
		result[i] = targetChar
		i++
	}
	return result
}

// fenceOpenLineRange returns the byte range [start, end) of the opening fence line (without trailing newline).
func fenceOpenLineRange(src []byte, fcb *ast.FencedCodeBlock) (int, int) {
	if fcb.Info != nil {
		// Walk back from info start to find line start
		lineStart := fcb.Info.Segment.Start
		for lineStart > 0 && src[lineStart-1] != '\n' {
			lineStart--
		}
		// Line end is the end of the info segment (there may be trailing space)
		lineEnd := fcb.Info.Segment.Stop
		for lineEnd < len(src) && src[lineEnd] != '\n' {
			lineEnd++
		}
		return lineStart, lineEnd
	}
	if fcb.Lines().Len() > 0 {
		firstContentStart := fcb.Lines().At(0).Start
		// Walk backwards past the newline ending the opening fence line
		pos := firstContentStart
		if pos > 0 && src[pos-1] == '\n' {
			pos--
		}
		lineEnd := pos
		lineStart := pos
		for lineStart > 0 && src[lineStart-1] != '\n' {
			lineStart--
		}
		return lineStart, lineEnd
	}
	// Empty code block with no info - scan from previous sibling or start of file
	searchStart := 0
	if prev := fcb.PreviousSibling(); prev != nil {
		searchStart = lastByteOfNodeStop(src, prev)
	}
	pos := searchStart
	for pos < len(src) {
		lineStart := pos
		lineEnd := pos
		for lineEnd < len(src) && src[lineEnd] != '\n' {
			lineEnd++
		}
		line := bytes.TrimLeft(src[lineStart:lineEnd], " ")
		if bytes.HasPrefix(line, []byte("```")) || bytes.HasPrefix(line, []byte("~~~")) {
			return lineStart, lineEnd
		}
		if lineEnd >= len(src) {
			break
		}
		pos = lineEnd + 1
	}
	return len(src), len(src)
}

// fenceCloseLineRange returns the byte range [start, end) of the closing fence line (without trailing newline).
func fenceCloseLineRange(src []byte, fcb *ast.FencedCodeBlock, openEnd int) (int, int) {
	var closingStart int
	if fcb.Lines().Len() > 0 {
		lastLine := fcb.Lines().At(fcb.Lines().Len() - 1)
		closingStart = lastLine.Stop
	} else {
		closingStart = openEnd
		if closingStart < len(src) && src[closingStart] == '\n' {
			closingStart++
		}
	}
	closingEnd := closingStart
	for closingEnd < len(src) && src[closingEnd] != '\n' {
		closingEnd++
	}
	return closingStart, closingEnd
}

func lastByteOfNodeStop(src []byte, n ast.Node) int {
	if block, ok := n.(interface{ Lines() *text.Segments }); ok {
		lines := block.Lines()
		if lines.Len() > 0 {
			return lines.At(lines.Len() - 1).Stop
		}
	}
	return 0
}

// FenceOpenLine returns the 1-based line number of the opening fence.
func FenceOpenLine(f *lint.File, fcb *ast.FencedCodeBlock) int {
	start, _ := fenceOpenLineRange(f.Source, fcb)
	return f.LineOfOffset(start)
}

// FenceCloseLine returns the 1-based line number of the closing fence.
func FenceCloseLine(f *lint.File, fcb *ast.FencedCodeBlock) int {
	_, openEnd := fenceOpenLineRange(f.Source, fcb)
	start, _ := fenceCloseLineRange(f.Source, fcb, openEnd)
	return f.LineOfOffset(start)
}

// FenceOpenLineRange is an exported wrapper for tests.
func FenceOpenLineRange(src []byte, fcb *ast.FencedCodeBlock) (int, int) {
	return fenceOpenLineRange(src, fcb)
}

// FenceCloseLineRange is an exported wrapper for tests.
func FenceCloseLineRange(src []byte, fcb *ast.FencedCodeBlock, openEnd int) (int, int) {
	return fenceCloseLineRange(src, fcb, openEnd)
}

// FenceLines returns a helper for getting the line ranges used by other rules.
func FenceLines(src []byte, fcb *ast.FencedCodeBlock) (openStart, openEnd, closeStart, closeEnd int) {
	openStart, openEnd = fenceOpenLineRange(src, fcb)
	closeStart, closeEnd = fenceCloseLineRange(src, fcb, openEnd)
	return
}
