package nomultipleblanks

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
)

func init() {
	rule.Register(&Rule{Max: 1})
}

// Rule checks that there are no more than Max consecutive blank lines.
// Default Max is 1 (no consecutive blank lines allowed).
type Rule struct {
	Max int // maximum allowed consecutive blank lines (default: 1)
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "TM008" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-multiple-blanks" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "whitespace" }

// isBlank returns true if the line contains only whitespace.
func isBlank(line []byte) bool {
	return len(bytes.TrimSpace(line)) == 0
}

// maxBlanks returns the effective maximum.
func (r *Rule) maxBlanks() int {
	if r.Max <= 0 {
		return 1
	}
	return r.Max
}

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	codeLines := lint.CollectCodeBlockLines(f)
	var diags []lint.Diagnostic
	max := r.maxBlanks()
	consecutiveBlanks := 0
	for i, line := range f.Lines {
		lineNum := i + 1
		if codeLines[lineNum] {
			consecutiveBlanks = 0
			continue
		}
		if isBlank(line) {
			consecutiveBlanks++
			if consecutiveBlanks > max {
				diags = append(diags, lint.Diagnostic{
					File:     f.Path,
					Line:     lineNum,
					Column:   1,
					RuleID:   r.ID(),
					RuleName: r.Name(),
					Severity: lint.Warning,
					Message:  "multiple consecutive blank lines",
				})
			}
		} else {
			consecutiveBlanks = 0
		}
	}
	return diags
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	codeLines := lint.CollectCodeBlockLines(f)
	max := r.maxBlanks()
	var result []string
	consecutiveBlanks := 0
	for i, line := range f.Lines {
		lineNum := i + 1
		if codeLines[lineNum] {
			result = append(result, string(line))
			consecutiveBlanks = 0
			continue
		}
		if isBlank(line) {
			consecutiveBlanks++
			if consecutiveBlanks > max {
				continue
			}
		} else {
			consecutiveBlanks = 0
		}
		result = append(result, string(line))
	}
	return []byte(strings.Join(result, "\n"))
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "max":
			n, ok := toInt(v)
			if !ok {
				return fmt.Errorf("no-multiple-blanks: max must be an integer, got %T", v)
			}
			r.Max = n
		default:
			return fmt.Errorf("no-multiple-blanks: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"max": 1,
	}
}

// toInt converts a value to int.
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	case int64:
		return int(n), true
	}
	return 0, false
}

var _ rule.Configurable = (*Rule)(nil)
