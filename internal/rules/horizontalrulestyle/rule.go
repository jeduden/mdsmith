package horizontalrulestyle

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	rulesettings "github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{
		Style:             "dash",
		Length:            3,
		RequireBlankLines: true,
	})
}

// Rule checks that horizontal rules use a consistent delimiter style.
type Rule struct {
	Style             string // "dash", "asterisk", or "underscore"
	Length            int    // exact number of delimiter characters required
	RequireBlankLines bool   // blank lines required before/after
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS044" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "horizontal-rule-style" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "whitespace" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule. The per-thematic-break logic is pure and
// stateless, so it is expressed as CheckNode and the engine can fold
// this rule into one shared AST walk; a direct call still works via
// rule.WalkNodes.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	return rule.WalkNodes(r, f)
}

// CheckNode implements rule.NodeChecker.
func (r *Rule) CheckNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if !entering {
		return nil
	}
	tb, ok := n.(*ast.ThematicBreak)
	if !ok {
		return nil
	}
	line := thematicBreakLine(tb, f)
	return r.checkHR(f, line)
}

func (r *Rule) checkHR(f *lint.File, line int) []lint.Diagnostic {
	lineIdx := line - 1
	if lineIdx < 0 || lineIdx >= len(f.Lines) {
		return nil
	}

	rawLine := string(bytes.TrimRight(f.Lines[lineIdx], "\n\r"))
	_, token := splitHRLine(rawLine)
	delimiter, count, hasSpaces := parseHR(token)

	diags := make([]lint.Diagnostic, 0, 4)
	diags = append(diags, r.checkDelimiter(f, line, delimiter)...)
	diags = append(diags, r.checkSpaces(f, line, hasSpaces)...)
	diags = append(diags, r.checkLength(f, line, count)...)
	diags = append(diags, r.checkBlankLines(f, line)...)
	return diags
}

func (r *Rule) checkDelimiter(f *lint.File, line int, delimiter rune) []lint.Diagnostic {
	if delimiter == delimChar(r.Style) {
		return nil
	}
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  fmt.Sprintf("horizontal rule uses %s; configured style is %s", styleName(delimiter), r.Style),
	}}
}

func (r *Rule) checkSpaces(f *lint.File, line int, hasSpaces bool) []lint.Diagnostic {
	if !hasSpaces {
		return nil
	}
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  "horizontal rule has internal spaces",
	}}
}

func (r *Rule) checkLength(f *lint.File, line int, count int) []lint.Diagnostic {
	if count == r.Length {
		return nil
	}
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  fmt.Sprintf("horizontal rule has length %d; configured length is %d", count, r.Length),
	}}
}

func (r *Rule) checkBlankLines(f *lint.File, line int) []lint.Diagnostic {
	if !r.RequireBlankLines {
		return nil
	}
	var diags []lint.Diagnostic
	if line > 1 && !isBlankLine(f.Lines, line-2) {
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     line,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  "horizontal rule needs a blank line above",
		})
	}
	if !isBlankLine(f.Lines, line) {
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     line,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  "horizontal rule needs a blank line below",
		})
	}
	return diags
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	// before/after map line numbers to the blank-line string to insert
	// (includes any container prefix such as "> " for blockquotes).
	insertBefore, insertAfter, replacements := r.collectChanges(f)

	if len(insertBefore) == 0 && len(insertAfter) == 0 && len(replacements) == 0 {
		return f.Source
	}

	var result []string
	for i, line := range f.Lines {
		lineNum := i + 1
		if blank, ok := insertBefore[lineNum]; ok {
			// Avoid double blank: if we just inserted after the previous line,
			// that already provides the blank line above.
			if _, prev := insertAfter[lineNum-1]; !prev {
				result = append(result, blank)
			}
		}
		if repl, ok := replacements[lineNum]; ok {
			result = append(result, repl)
		} else {
			result = append(result, string(line))
		}
		if blank, ok := insertAfter[lineNum]; ok {
			result = append(result, blank)
		}
	}

	return []byte(strings.Join(result, "\n"))
}

func (r *Rule) collectChanges(f *lint.File) (before, after map[int]string, replacements map[int]string) {
	before = make(map[int]string)
	after = make(map[int]string)
	replacements = make(map[int]string)

	canonical := strings.Repeat(string(delimChar(r.Style)), r.Length)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		tb, ok := n.(*ast.ThematicBreak)
		if !ok {
			return ast.WalkContinue, nil
		}
		line := thematicBreakLine(tb, f)
		lineIdx := line - 1
		if lineIdx < 0 || lineIdx >= len(f.Lines) {
			return ast.WalkContinue, nil
		}

		// Only replace the line when the token differs from canonical, preserving any leading prefix.
		rawLine := string(bytes.TrimRight(f.Lines[lineIdx], "\n\r"))
		prefix, token := splitHRLine(rawLine)
		if token != canonical {
			replacements[line] = prefix + canonical
		}

		if r.RequireBlankLines {
			// blankLine is the blank line to insert — include the container
			// prefix (e.g. "> ") so the blank line stays inside the blockquote.
			blankLine := strings.TrimRight(prefix, " \t")
			if line > 1 && !isBlankLine(f.Lines, line-2) {
				before[line] = blankLine
			}
			if !isBlankLine(f.Lines, line) {
				after[line] = blankLine
			}
		}

		return ast.WalkContinue, nil
	})

	return before, after, replacements
}

// splitHRLine splits a raw source line into its leading prefix (blockquote
// markers, list indentation, up-to-3-space indent) and the thematic-break
// token itself. For example "> ---" returns ("> ", "---").
func splitHRLine(rawLine string) (prefix, token string) {
	idx := strings.IndexAny(rawLine, "-*_")
	if idx < 0 {
		return rawLine, ""
	}
	return rawLine[:idx], strings.TrimSpace(rawLine[idx:])
}

// parseHR returns the delimiter character, the count of delimiter chars,
// and whether internal spaces are present.
func parseHR(token string) (delimiter rune, count int, hasSpaces bool) {
	for _, ch := range token {
		if ch == '-' || ch == '*' || ch == '_' {
			delimiter = ch
			break
		}
	}
	for _, ch := range token {
		switch ch {
		case delimiter:
			count++
		case ' ', '\t':
			hasSpaces = true
		}
	}
	return delimiter, count, hasSpaces
}

func delimChar(style string) rune {
	switch style {
	case "asterisk":
		return '*'
	case "underscore":
		return '_'
	default:
		return '-'
	}
}

func styleName(delimiter rune) string {
	switch delimiter {
	case '*':
		return "asterisk"
	case '_':
		return "underscore"
	default:
		return "dash"
	}
}

// thematicBreakLine returns the 1-based source line of a ThematicBreak node.
func thematicBreakLine(tb *ast.ThematicBreak, f *lint.File) int {
	lines := tb.Lines()
	if lines.Len() > 0 {
		return f.LineOfOffset(lines.At(0).Start)
	}
	return f.LineOfOffset(tb.Pos())
}

func isBlankLine(lines [][]byte, idx int) bool {
	if idx < 0 || idx >= len(lines) {
		return true
	}
	// A line containing only blockquote markers (">") and whitespace is
	// considered blank — e.g. "> " is an empty blockquote continuation.
	stripped := bytes.TrimLeft(lines[idx], "> \t")
	return len(bytes.TrimSpace(stripped)) == 0
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "style":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("horizontal-rule-style: style must be a string, got %T", v)
			}
			if s != "dash" && s != "asterisk" && s != "underscore" {
				return fmt.Errorf("horizontal-rule-style: invalid style %q (valid: dash, asterisk, underscore)", s)
			}
			r.Style = s
		case "length":
			n, ok := rulesettings.ToInt(v)
			if !ok {
				return fmt.Errorf("horizontal-rule-style: length must be an integer, got %T", v)
			}
			if n < 3 {
				return fmt.Errorf("horizontal-rule-style: length must be at least 3, got %d", n)
			}
			r.Length = n
		case "require-blank-lines":
			b, ok := v.(bool)
			if !ok {
				return fmt.Errorf("horizontal-rule-style: require-blank-lines must be a boolean, got %T", v)
			}
			r.RequireBlankLines = b
		default:
			return fmt.Errorf("horizontal-rule-style: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"style":               "dash",
		"length":              3,
		"require-blank-lines": true,
	}
}

var _ rule.FixableRule = (*Rule)(nil)
var _ rule.Configurable = (*Rule)(nil)
var _ rule.Defaultable = (*Rule)(nil)
var _ rule.NodeChecker = (*Rule)(nil)

// FixTitle implements rule.QuickFixTitler.
func (r *Rule) FixTitle() string { return "Normalize horizontal rule" }
