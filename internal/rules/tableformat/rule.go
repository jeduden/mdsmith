package tableformat

import (
	"fmt"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/jeduden/mdsmith/internal/rules/tablefmt"
)

func init() {
	rule.Register(&Rule{Pad: 1, SeparatorStyle: tablefmt.SeparatorSpaced})
}

// Rule checks that markdown tables are formatted with consistent
// column widths, cell padding, and a chosen separator style.
type Rule struct {
	Pad            int // spaces on each side of cell content
	SeparatorStyle tablefmt.SeparatorStyle
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS025" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "table-format" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "table" }

// GetPad returns the current pad setting.
func (r *Rule) GetPad() int { return r.Pad }

// GetSeparatorStyle returns the active separator style.
func (r *Rule) GetSeparatorStyle() tablefmt.SeparatorStyle { return r.SeparatorStyle }

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "pad":
			n, ok := settings.ToInt(v)
			if !ok {
				return fmt.Errorf("table-format: pad must be an integer, got %T", v)
			}
			if n < 0 {
				return fmt.Errorf("table-format: pad must be non-negative, got %d", n)
			}
			r.Pad = n
		case "separator-style":
			style, err := parseSeparatorStyle(v)
			if err != nil {
				return err
			}
			r.SeparatorStyle = style
		default:
			return fmt.Errorf("table-format: unknown setting %q", k)
		}
	}
	return nil
}

func parseSeparatorStyle(v any) (tablefmt.SeparatorStyle, error) {
	s, ok := v.(string)
	if !ok {
		return 0, fmt.Errorf("table-format: separator-style must be a string, got %T", v)
	}
	switch s {
	case "spaced":
		return tablefmt.SeparatorSpaced, nil
	case "compact":
		return tablefmt.SeparatorCompact, nil
	default:
		return 0, fmt.Errorf("table-format: separator-style must be \"spaced\" or \"compact\", got %q", s)
	}
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"pad":             1,
		"separator-style": "spaced",
	}
}

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	codeLines := lint.CollectCodeBlockLines(f)
	var diags []lint.Diagnostic
	for _, v := range tablefmt.Violations(f.Lines, codeLines, r.config()) {
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     v.StartLine,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  v.Message,
		})
	}
	return diags
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	codeLines := lint.CollectCodeBlockLines(f)
	return tablefmt.FormatLines(f.Source, f.Lines, codeLines, r.config())
}

func (r *Rule) config() tablefmt.Config {
	return tablefmt.Config{Pad: r.Pad, SeparatorStyle: r.SeparatorStyle}
}

var _ rule.FixableRule = (*Rule)(nil)
var _ rule.Configurable = (*Rule)(nil)
