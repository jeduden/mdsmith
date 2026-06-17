package headingstyle

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
)

func init() {
	rule.Register(&Rule{Style: "atx"})
}

// Rule checks that all headings use a consistent style (atx or setext).
type Rule struct {
	Style string // "atx" or "setext"
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS002" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "heading-style" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "heading" }

// Check implements rule.Rule. The per-heading logic depends only on a
// heading's style (ATX vs setext) and, for the setext target, its level
// — both readable from the heading's own source line, never the inline
// tree — so the rule is a rule.BlockChecker: on a parsed File it folds
// into the engine's shared AST walk (rule.WalkNodes), and on a parse-
// skipped File (f.AST nil) it reads the Layer 0 block scan instead
// (rule.WalkBlocks). Both resolve the same per-heading verdict, so the
// diagnostics are identical.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f != nil && f.AST == nil {
		return rule.WalkBlocks(r, f)
	}
	return rule.WalkNodes(r, f)
}

// CheckNode implements rule.NodeChecker.
func (r *Rule) CheckNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if !entering {
		return nil
	}
	heading, ok := n.(*ast.Heading)
	if !ok {
		return nil
	}
	return r.verdict(f, isATXHeading(heading, f.Source), heading.Level, headingLine(heading, f))
}

// CheckBlock implements rule.BlockChecker. span.Start is the heading line
// (headingLine on the AST path); the style and ATX level are read from
// that line's bytes to match the AST path's isATXHeading and
// heading.Level exactly, so the verdict is byte-identical.
func (r *Rule) CheckBlock(span lint.BlockSpan, f *lint.File) []lint.Diagnostic {
	line := f.Lines[span.Start-1]
	// Match the AST path's isATXHeading exactly: it tests the first byte of
	// the heading's source line via lineStartsWithHash and does NOT skip
	// indentation. A ≤3-space-indented ATX heading is a BlockATXHeading span
	// (goldmark still parses it as a heading), but MDS002's column-1 test
	// reads it as non-ATX — so isATX keys off the raw first byte, not the
	// span kind, to keep the two paths byte-identical.
	isATX := len(line) > 0 && line[0] == '#'
	level := 0
	if isATX {
		level = atxLevelFromLine(line)
	}
	return r.verdict(f, isATX, level, span.Start)
}

// blockKinds is the static block-kind interest CheckBlock declares via
// rule.BlockChecker; package-level so BlockKinds returns it without
// allocating. ATX and setext headings both map from ast.KindHeading.
var blockKinds = []lint.BlockKind{lint.BlockATXHeading, lint.BlockSetextHeading}

// BlockKinds implements rule.BlockChecker: CheckBlock reacts to both
// heading shapes.
func (r *Rule) BlockKinds() []lint.BlockKind { return blockKinds }

var _ rule.BlockChecker = (*Rule)(nil)

// verdict is the shared per-heading check both the AST (CheckNode) and
// Layer 0 (CheckBlock) paths drive: isATX is the heading's style, level
// its ATX level (only read for the setext target), and line its 1-based
// heading line.
func (r *Rule) verdict(f *lint.File, isATX bool, level, line int) []lint.Diagnostic {
	style := r.Style
	if style == "" {
		style = "atx"
	}
	if style == "atx" && !isATX {
		return r.diag(f, line, "heading style should be atx")
	}
	// setext only supports levels 1 and 2; levels 3-6 must use atx.
	if style == "setext" && isATX && level <= 2 {
		return r.diag(f, line, "heading style should be setext")
	}
	return nil
}

// diag builds the single-element diagnostic slice both paths emit.
func (r *Rule) diag(f *lint.File, line int, msg string) []lint.Diagnostic {
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  msg,
	}}
}

// atxLevelFromLine returns the ATX heading level (number of leading `#`,
// 1–6) of a line the Layer 0 scanner classified BlockATXHeading. Up to
// three leading spaces are skipped first, matching goldmark's ATX parse;
// the span kind guarantees a valid 1–6 run, so no validity check is
// needed here.
func atxLevelFromLine(line []byte) int {
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	level := 0
	for i < len(line) && line[i] == '#' {
		level++
		i++
	}
	return level
}

type replacement struct {
	start, end int
	newText    string
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	style := r.Style
	if style == "" {
		style = "atx"
	}

	result := make([]byte, len(f.Source))
	copy(result, f.Source)

	var replacements []replacement

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		heading, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}

		if rep, ok := buildStyleReplacement(heading, f.Source, style); ok {
			replacements = append(replacements, rep)
		}

		return ast.WalkContinue, nil
	})

	// Pre-size each replacement buffer to avoid the temporary inner slice
	// that append(before, append([]byte(newText), after...)...) would create.
	for i := len(replacements) - 1; i >= 0; i-- {
		rep := replacements[i]
		before := result[:rep.start]
		after := result[rep.end:]
		tmp := make([]byte, 0, rep.start+len(rep.newText)+len(after))
		tmp = append(tmp, before...)
		tmp = append(tmp, rep.newText...)
		tmp = append(tmp, after...)
		result = tmp
	}

	return result
}

// buildStyleReplacement returns a replacement to convert a heading to the target
// style, or false if no conversion is needed.
func buildStyleReplacement(heading *ast.Heading, source []byte, style string) (replacement, bool) {
	isATX := isATXHeading(heading, source)
	hText := headingText(heading, source)
	start, end := headingByteRange(heading, source)

	if style == "atx" && !isATX {
		prefix := strings.Repeat("#", heading.Level)
		return replacement{start: start, end: end, newText: prefix + " " + hText}, true
	}

	if style == "setext" && isATX && heading.Level <= 2 {
		underChar := "="
		if heading.Level == 2 {
			underChar = "-"
		}
		underline := strings.Repeat(underChar, len(hText))
		if len(hText) == 0 {
			underline = strings.Repeat(underChar, 3)
		}
		return replacement{start: start, end: end, newText: hText + "\n" + underline}, true
	}

	return replacement{}, false
}

// isATXHeading checks whether a heading uses ATX style (starts with #).
func isATXHeading(heading *ast.Heading, source []byte) bool {
	lines := heading.Lines()
	if lines.Len() == 0 {
		return isATXHeadingNoLines(heading, source)
	}

	// If Lines() > 0, it could be setext. Check if the source line starts with #.
	seg := lines.At(0)
	return lineStartsWithHash(source, seg.Start)
}

// isATXHeadingNoLines determines ATX style for headings with no Lines() entries,
// using child text nodes to locate the source line.
func isATXHeadingNoLines(heading *ast.Heading, source []byte) bool {
	if heading.FirstChild() == nil {
		return true // no lines, no children - assume atx
	}

	seg := firstTextSegment(heading)
	if seg.Start == 0 && seg.Stop == 0 {
		return true // default to atx if we can't determine
	}

	return lineStartsWithHash(source, seg.Start)
}

// firstTextSegment finds the text.Segment of the first ast.Text node under n.
func firstTextSegment(n ast.Node) text.Segment {
	var seg text.Segment
	_ = ast.Walk(n, func(child ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if t, ok := child.(*ast.Text); ok {
				seg = t.Segment
				return ast.WalkStop, nil
			}
		}
		return ast.WalkContinue, nil
	})
	return seg
}

// lineStartsWithHash returns true if the line containing the byte at offset
// starts with '#'.
func lineStartsWithHash(source []byte, offset int) bool {
	lineStart := offset
	for lineStart > 0 && source[lineStart-1] != '\n' {
		lineStart--
	}
	return lineStart < len(source) && source[lineStart] == '#'
}

func headingLine(heading *ast.Heading, f *lint.File) int {
	lines := heading.Lines()
	if lines.Len() > 0 {
		return f.LineOfOffset(lines.At(0).Start)
	}
	// For ATX headings, find the line via child text nodes
	for c := heading.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			return f.LineOfOffset(t.Segment.Start)
		}
		// Check inline children
		var found int
		_ = ast.Walk(c, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if entering {
				if t, ok := n.(*ast.Text); ok {
					found = f.LineOfOffset(t.Segment.Start)
					return ast.WalkStop, nil
				}
			}
			return ast.WalkContinue, nil
		})
		if found > 0 {
			return found
		}
	}
	return 1
}

func headingByteRange(heading *ast.Heading, source []byte) (int, int) {
	// Find the start of the heading in source
	lines := heading.Lines()
	var start int

	if lines.Len() > 0 {
		start = lines.At(0).Start
	} else {
		// ATX heading - find via children
		for c := heading.FirstChild(); c != nil; c = c.NextSibling() {
			if t, ok := c.(*ast.Text); ok {
				start = t.Segment.Start
				break
			}
		}
	}

	// Go to the start of the line
	for start > 0 && source[start-1] != '\n' {
		start--
	}

	isATX := isATXHeading(heading, source)

	if isATX {
		// ATX heading is a single line
		end := start
		for end < len(source) && source[end] != '\n' {
			end++
		}
		return start, end
	}

	// Setext heading spans multiple lines (text + underline)
	// Find end of text line
	endText := start
	for endText < len(source) && source[endText] != '\n' {
		endText++
	}
	// Skip past newline to underline
	endUnderline := endText + 1
	for endUnderline < len(source) && source[endUnderline] != '\n' {
		endUnderline++
	}
	return start, endUnderline
}

func headingText(heading *ast.Heading, source []byte) string {
	var buf bytes.Buffer
	for c := heading.FirstChild(); c != nil; c = c.NextSibling() {
		extractText(c, source, &buf)
	}
	return buf.String()
}

func extractText(n ast.Node, source []byte, buf *bytes.Buffer) {
	if t, ok := n.(*ast.Text); ok {
		buf.Write(t.Segment.Value(source))
		return
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		extractText(c, source, buf)
	}
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "style":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("heading-style: style must be a string, got %T", v)
			}
			if s != "atx" && s != "setext" {
				return fmt.Errorf("heading-style: invalid style %q (valid: atx, setext)", s)
			}
			r.Style = s
		default:
			return fmt.Errorf("heading-style: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"style": "atx",
	}
}

var _ rule.FixableRule = (*Rule)(nil)
var _ rule.Configurable = (*Rule)(nil)
var _ rule.NodeChecker = (*Rule)(nil)

// FixTitle implements rule.QuickFixTitler.
func (r *Rule) FixTitle() string { return "Convert to configured heading style" }

// enteringKinds is the static node-kind interest CheckNode declares
// via rule.KindScopedChecker; package-level so EnteringKinds returns
// it without allocating.
var enteringKinds = []ast.NodeKind{ast.KindHeading}

// EnteringKinds implements rule.KindScopedChecker: CheckNode only
// reacts to these node kinds, entering visits only.
func (r *Rule) EnteringKinds() []ast.NodeKind { return enteringKinds }

var _ rule.KindScopedChecker = (*Rule)(nil)
