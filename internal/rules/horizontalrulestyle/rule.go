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
	RequireBlankLines bool   // whether blank lines are required before/after
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS044" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "horizontal-rule-style" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "whitespace" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		tb, ok := n.(*ast.ThematicBreak)
		if !ok {
			return ast.WalkContinue, nil
		}

		line := nodeLineNumber(tb, f)
		diags = append(diags, r.checkHorizontalRule(f, line)...)

		return ast.WalkContinue, nil
	})

	return diags
}

func (r *Rule) checkHorizontalRule(f *lint.File, line int) []lint.Diagnostic {
	var diags []lint.Diagnostic

	lineIdx := line - 1
	if lineIdx < 0 || lineIdx >= len(f.Lines) {
		return diags
	}

	rawLine := string(bytes.TrimRight(f.Lines[lineIdx], "\n\r"))
	_, lineContent := extractHRContent(rawLine)
	delimiter, count, hasSpaces := parseHorizontalRule(lineContent)

	diags = append(diags, r.checkDelimiter(f, line, delimiter)...)
	diags = append(diags, r.checkSpaces(f, line, hasSpaces)...)
	diags = append(diags, r.checkLength(f, line, count)...)
	diags = append(diags, r.checkBlankLines(f, line)...)

	return diags
}

func (r *Rule) checkDelimiter(f *lint.File, line int, delimiter rune) []lint.Diagnostic {
	expectedDelim := delimiterChar(r.Style)
	if delimiter != expectedDelim {
		actualStyle := styleFromDelimiter(delimiter)
		return []lint.Diagnostic{{
			File:     f.Path,
			Line:     line,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  fmt.Sprintf("horizontal rule uses %s; configured style is %s", actualStyle, r.Style),
		}}
	}
	return nil
}

func (r *Rule) checkSpaces(f *lint.File, line int, hasSpaces bool) []lint.Diagnostic {
	if hasSpaces {
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
	return nil
}

func (r *Rule) checkLength(f *lint.File, line int, count int) []lint.Diagnostic {
	if count != r.Length {
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
	return nil
}

func (r *Rule) checkBlankLines(f *lint.File, line int) []lint.Diagnostic {
	if !r.RequireBlankLines {
		return nil
	}

	var diags []lint.Diagnostic

	// Check line above
	if line > 1 {
		prevLineIdx := line - 2
		if prevLineIdx >= 0 && prevLineIdx < len(f.Lines) {
			if !isBlank(f.Lines[prevLineIdx]) {
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
		}
	}

	// Check line below
	nextLineIdx := line
	if nextLineIdx < len(f.Lines) {
		if !isBlank(f.Lines[nextLineIdx]) {
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
	}

	return diags
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	insertBeforeLine, insertAfterLine, replacements := collectHorizontalRuleChanges(f, r)

	if len(insertBeforeLine) == 0 && len(insertAfterLine) == 0 && len(replacements) == 0 {
		return f.Source
	}

	var result []string
	for i, line := range f.Lines {
		lineNum := i + 1
		if insertBeforeLine[lineNum] {
			result = append(result, "")
		}

		if repl, ok := replacements[lineNum]; ok {
			result = append(result, repl)
		} else {
			result = append(result, string(line))
		}

		if insertAfterLine[lineNum] {
			result = append(result, "")
		}
	}

	return []byte(strings.Join(result, "\n"))
}

// collectHorizontalRuleChanges walks the AST and returns:
// - sets of line numbers needing blank line insertions
// - map of line numbers to replacement content
func collectHorizontalRuleChanges(
	f *lint.File, r *Rule,
) (beforeSet, afterSet map[int]bool, replacements map[int]string) {
	beforeSet = make(map[int]bool)
	afterSet = make(map[int]bool)
	replacements = make(map[int]string)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		tb, ok := n.(*ast.ThematicBreak)
		if !ok {
			return ast.WalkContinue, nil
		}

		line := nodeLineNumber(tb, f)
		lineIdx := line - 1
		if lineIdx < 0 || lineIdx >= len(f.Lines) {
			return ast.WalkContinue, nil
		}

		// Build canonical replacement, preserving any leading prefix
		rawLine := string(bytes.TrimRight(f.Lines[lineIdx], "\n\r"))
		prefix, token := extractHRContent(rawLine)
		canonical := strings.Repeat(string(delimiterChar(r.Style)), r.Length)
		if token != canonical {
			replacements[line] = prefix + canonical
		}

		// Check if blank lines are needed
		if r.RequireBlankLines {
			if line > 1 {
				prevLineIdx := line - 2
				if prevLineIdx >= 0 && prevLineIdx < len(f.Lines) && !isBlank(f.Lines[prevLineIdx]) {
					beforeSet[line] = true
				}
			}

			nextLineIdx := line
			if nextLineIdx < len(f.Lines) && !isBlank(f.Lines[nextLineIdx]) {
				afterSet[line] = true
			}
		}

		return ast.WalkContinue, nil
	})

	// Deduplicate: if inserting after line N and before line N+1,
	// the after-insertion already provides the needed blank line.
	for n := range afterSet {
		if beforeSet[n+1] {
			delete(beforeSet, n+1)
		}
	}

	return beforeSet, afterSet, replacements
}

// extractHRContent splits a raw source line into its leading prefix
// (blockquote markers, list indentation, spaces) and the thematic-break
// token itself. For example "> ---" returns ("> ", "---").
func extractHRContent(rawLine string) (prefix, token string) {
	idx := strings.IndexAny(rawLine, "-*_")
	if idx < 0 {
		return rawLine, ""
	}
	return rawLine[:idx], strings.TrimSpace(rawLine[idx:])
}

// parseHorizontalRule analyzes a horizontal rule line and returns:
// - the delimiter character used
// - the count of delimiter characters
// - whether internal spaces exist
func parseHorizontalRule(line string) (delimiter rune, count int, hasSpaces bool) {
	// Determine the delimiter
	for _, r := range line {
		if r == '-' || r == '*' || r == '_' {
			delimiter = r
			break
		}
	}

	// Count delimiters and check for spaces
	for _, char := range line {
		switch char {
		case delimiter:
			count++
		case ' ', '\t':
			hasSpaces = true
		}
	}

	return delimiter, count, hasSpaces
}

// delimiterChar returns the character for a given style name.
func delimiterChar(style string) rune {
	switch style {
	case "dash":
		return '-'
	case "asterisk":
		return '*'
	case "underscore":
		return '_'
	default:
		return '-'
	}
}

// styleFromDelimiter returns the style name for a delimiter character.
func styleFromDelimiter(delimiter rune) string {
	switch delimiter {
	case '-':
		return "dash"
	case '*':
		return "asterisk"
	case '_':
		return "underscore"
	default:
		return "unknown"
	}
}

// nodeLineNumber finds the line number of a ThematicBreak node.
func nodeLineNumber(tb *ast.ThematicBreak, f *lint.File) int {
	// ThematicBreak nodes don't populate Lines(), but they have a Pos field
	// that gives the byte offset
	lines := tb.Lines()
	if lines.Len() > 0 {
		return f.LineOfOffset(lines.At(0).Start)
	}
	// Use the Pos field as the byte offset
	return f.LineOfOffset(tb.Pos())
}

func isBlank(line []byte) bool {
	return len(bytes.TrimSpace(line)) == 0
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "style":
			styleStr, ok := v.(string)
			if !ok {
				return fmt.Errorf("horizontal-rule-style: style must be a string, got %T", v)
			}
			if styleStr != "dash" && styleStr != "asterisk" && styleStr != "underscore" {
				return fmt.Errorf(
					"horizontal-rule-style: invalid style %q (valid: dash, asterisk, underscore)",
					styleStr,
				)
			}
			r.Style = styleStr
		case "length":
			intVal, ok := rulesettings.ToInt(v)
			if !ok {
				return fmt.Errorf("horizontal-rule-style: length must be an integer, got %T", v)
			}
			if intVal < 3 {
				return fmt.Errorf("horizontal-rule-style: length must be at least 3, got %d", intVal)
			}
			r.Length = intVal
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
