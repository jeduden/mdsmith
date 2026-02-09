package notrailingspaces

import (
	"bytes"
	"strings"

	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that no line ends with trailing spaces or tabs.
type Rule struct{}

func (r *Rule) ID() string   { return "TM006" }
func (r *Rule) Name() string { return "no-trailing-spaces" }

func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	for i, line := range f.Lines {
		trimmed := bytes.TrimRight(line, " \t")
		if len(trimmed) < len(line) {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     i + 1,
				Column:   len(trimmed) + 1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  "trailing whitespace",
			})
		}
	}
	return diags
}

func (r *Rule) Fix(f *lint.File) []byte {
	var result []string
	for _, line := range f.Lines {
		trimmed := bytes.TrimRight(line, " \t")
		result = append(result, string(trimmed))
	}
	return []byte(strings.Join(result, "\n"))
}
