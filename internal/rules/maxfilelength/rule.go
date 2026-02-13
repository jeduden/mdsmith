package maxfilelength

import (
	"fmt"

	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
)

func init() {
	rule.Register(&Rule{Max: 300})
}

// Rule checks that a file does not exceed a configurable number of lines.
type Rule struct {
	Max int
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "TM022" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "max-file-length" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	max := r.Max
	if max <= 0 {
		max = 300
	}
	lineCount := len(f.Lines)
	// bytes.Split adds an empty trailing element for files ending
	// with newline, so subtract one when the last element is empty.
	if lineCount > 0 && len(f.Lines[lineCount-1]) == 0 {
		lineCount--
	}
	if lineCount > max {
		return []lint.Diagnostic{{
			File:     f.Path,
			Line:     1,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  fmt.Sprintf("file too long (%d > %d)", lineCount, max),
		}}
	}
	return nil
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "max":
			n, ok := toInt(v)
			if !ok {
				return fmt.Errorf(
					"max-file-length: max must be an integer, got %T", v,
				)
			}
			r.Max = n
		default:
			return fmt.Errorf(
				"max-file-length: unknown setting %q", k,
			)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{"max": 300}
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
