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
	"github.com/yuin/goldmark/ast"
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
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	return rule.WalkNodes(r, f)
}

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
