// Package commandsshowoutput implements MDS066 commands-show-output:
// when every non-blank line of a fenced code block starts with "$ ",
// the block shows commands with no output and the prompt should be
// dropped so the snippet is copy-paste-friendly. Mirrors markdownlint
// MD014.
package commandsshowoutput

import (
	"bytes"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/fencepos"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule flags fenced code blocks whose every non-blank line is a
// shell prompt with no shown output.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS066" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "commands-show-output" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "code" }

// Check implements rule.Rule. The per-block logic is pure and stateless,
// so it is expressed as CheckNode and the engine can fold this rule into
// one shared AST walk; a direct call still works via rule.WalkNodes.
// On a parse-skipped File (f.AST nil) it reads the Layer 0 block scan
// (rule.WalkBlocks).
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f != nil && f.AST == nil {
		return rule.WalkBlocks(r, f)
	}
	return rule.WalkNodes(r, f)
}

// CheckBlock implements rule.BlockChecker for the nil-AST path. It mirrors
// CheckNode: guards on generated range, then delegates to allLinesArePromptsL0.
func (r *Rule) CheckBlock(span lint.BlockSpan, f *lint.File) []lint.Diagnostic {
	if span.Kind != lint.BlockFencedCode {
		return nil
	}
	if inGeneratedRange(f, span.Start) {
		return nil
	}
	if !allLinesArePromptsL0(f, span) {
		return nil
	}
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     span.Start,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  "commands shown with $ prefix but no output",
	}}
}

// allLinesArePromptsL0 reports whether every non-blank content line of the
// fenced block described by span starts with "$ " and at least one such line
// exists. It mirrors allLinesArePrompts but reads from f.Lines using the
// Layer 0 span rather than goldmark segment positions.
func allLinesArePromptsL0(f *lint.File, span lint.BlockSpan) bool {
	// Determine body line range (1-based inclusive).
	// For a closed block: lines span.Start+1 to span.End-1.
	// For an unclosed block: lines span.Start+1 to span.End.
	bodyStart := span.Start + 1
	var bodyEnd int
	if span.Closed {
		bodyEnd = span.End - 1
	} else {
		bodyEnd = span.End
	}
	if bodyStart > bodyEnd {
		return false
	}
	// Determine the fence's leading indent by counting leading spaces on the
	// opening fence line (span.Start is 1-based).
	openLine := f.Lines[span.Start-1]
	indent := 0
	for indent < len(openLine) && openLine[indent] == ' ' {
		indent++
	}
	hasPrompt := false
	for ln := bodyStart; ln <= bodyEnd; ln++ {
		line := f.Lines[ln-1]
		// Strip up to indent leading spaces from the body line to match
		// goldmark's fence-indent stripping.
		stripped := line
		for i := 0; i < indent && len(stripped) > 0 && stripped[0] == ' '; i++ {
			stripped = stripped[1:]
		}
		_, content := splitLeadingWhitespace(stripped)
		content = bytes.TrimRight(content, " \t\r")
		if len(content) == 0 {
			continue
		}
		if !bytes.HasPrefix(content, []byte("$ ")) {
			return false
		}
		hasPrompt = true
	}
	return hasPrompt
}

// blockKinds is the static block-kind interest CheckBlock declares via
// rule.BlockChecker; package-level so BlockKinds returns it without
// allocating.
var blockKinds = []lint.BlockKind{lint.BlockFencedCode}

// BlockKinds implements rule.BlockChecker.
func (r *Rule) BlockKinds() []lint.BlockKind { return blockKinds }

var _ rule.BlockChecker = (*Rule)(nil)

// CheckNode implements rule.NodeChecker.
func (r *Rule) CheckNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if !entering {
		return nil
	}
	fcb, ok := n.(*ast.FencedCodeBlock)
	if !ok {
		return nil
	}
	line := fencepos.OpenLine(f, fcb)
	if inGeneratedRange(f, line) {
		return nil
	}
	if !allLinesArePrompts(f, fcb) {
		return nil
	}
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  "commands shown with $ prefix but no output",
	}}
}

// Fix implements rule.FixableRule. Each offending block has every "$ "
// prefix stripped from its non-blank content lines.
func (r *Rule) Fix(f *lint.File) []byte {
	if f == nil {
		return nil
	}
	if f.AST == nil {
		return f.Source
	}

	rewriteLines := map[int]string{}
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		fcb, ok := n.(*ast.FencedCodeBlock)
		if !ok {
			return ast.WalkContinue, nil
		}
		line := fencepos.OpenLine(f, fcb)
		if inGeneratedRange(f, line) {
			return ast.WalkContinue, nil
		}
		if !allLinesArePrompts(f, fcb) {
			return ast.WalkContinue, nil
		}
		segs := fcb.Lines()
		for i := 0; i < segs.Len(); i++ {
			seg := segs.At(i)
			ln := f.LineOfOffset(seg.Start)
			rewriteLines[ln] = stripPromptAfter(f.Lines[ln-1], contentOffsetInLine(f, seg.Start))
		}
		return ast.WalkContinue, nil
	})

	if len(rewriteLines) == 0 {
		return f.Source
	}

	var buf bytes.Buffer
	buf.Grow(len(f.Source))
	for i, raw := range f.Lines {
		if rewritten, ok := rewriteLines[i+1]; ok {
			buf.WriteString(rewritten)
		} else {
			buf.Write(raw)
		}
		if i < len(f.Lines)-1 {
			buf.WriteByte('\n')
		}
	}
	return buf.Bytes()
}

// allLinesArePrompts reports whether every non-blank content line of
// fcb starts with "$ " (ignoring any container prefix the parser
// stripped — blockquote marker, list-item indent — plus any leading
// whitespace inside the content) and at least one such prompt line
// exists.
func allLinesArePrompts(f *lint.File, fcb *ast.FencedCodeBlock) bool {
	segs := fcb.Lines()
	if segs.Len() == 0 {
		return false
	}
	hasPrompt := false
	for i := 0; i < segs.Len(); i++ {
		seg := segs.At(i)
		ln := f.LineOfOffset(seg.Start)
		line := f.Lines[ln-1]
		contentCol := contentOffsetInLine(f, seg.Start)
		_, content := splitLeadingWhitespace(line[contentCol:])
		stripped := bytes.TrimRight(content, " \t\r")
		if len(stripped) == 0 {
			continue
		}
		if !bytes.HasPrefix(stripped, []byte("$ ")) {
			return false
		}
		hasPrompt = true
	}
	return hasPrompt
}

// stripPromptAfter removes the "$ " prompt from a non-blank content
// line, preserving the parser-stripped container prefix
// (line[:contentCol], e.g. "> " for a blockquote, "  " for a list
// indent) and any leading whitespace inside the content. Blank lines
// and lines without a prompt pass through unchanged.
func stripPromptAfter(line []byte, contentCol int) string {
	if contentCol > len(line) {
		return string(line)
	}
	prefix := line[:contentCol]
	leading, content := splitLeadingWhitespace(line[contentCol:])
	stripped := bytes.TrimRight(content, " \t\r")
	if len(stripped) == 0 {
		return string(line)
	}
	if !bytes.HasPrefix(stripped, []byte("$ ")) {
		return string(line)
	}
	return string(prefix) + string(leading) + string(content[2:])
}

// contentOffsetInLine returns the byte column (0-based) on the line
// containing offset where parser-stripped content begins. The skipped
// prefix can include a blockquote marker ("> "), a list-item indent,
// or both — anything goldmark consumed before exposing the fenced
// block's content via segment positions.
func contentOffsetInLine(f *lint.File, offset int) int {
	lineStart := offset
	for lineStart > 0 && f.Source[lineStart-1] != '\n' {
		lineStart--
	}
	return offset - lineStart
}

// splitLeadingWhitespace splits a line at the first non-whitespace byte,
// returning the leading whitespace and the rest.
func splitLeadingWhitespace(line []byte) (leading, rest []byte) {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return line[:i], line[i:]
}

func inGeneratedRange(f *lint.File, line int) bool {
	for _, gr := range f.GeneratedRanges {
		if gr.Contains(line) {
			return true
		}
	}
	return false
}

var _ rule.FixableRule = (*Rule)(nil)
var _ rule.NodeChecker = (*Rule)(nil)

// FixTitle implements rule.QuickFixTitler.
func (r *Rule) FixTitle() string { return "Strip $ prefixes from commands" }

// enteringKinds is the static node-kind interest CheckNode declares
// via rule.KindScopedChecker; package-level so EnteringKinds returns
// it without allocating.
var enteringKinds = []ast.NodeKind{ast.KindFencedCodeBlock}

// EnteringKinds implements rule.KindScopedChecker: CheckNode only
// reacts to these node kinds, entering visits only.
func (r *Rule) EnteringKinds() []ast.NodeKind { return enteringKinds }

var _ rule.KindScopedChecker = (*Rule)(nil)
