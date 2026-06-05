package notrailingspaces

import (
	"bytes"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that no line ends with trailing spaces or tabs.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS006" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-trailing-spaces" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "whitespace" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	codeLines := lint.CollectCodeBlockLines(f)
	var diags []lint.Diagnostic
	for i, line := range f.Lines {
		lineNum := i + 1
		if _, ok := codeLines[lineNum]; ok {
			continue
		}
		trimmed := bytes.TrimRight(line, " \t")
		if len(trimmed) < len(line) {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     lineNum,
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

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	codeLines := lint.CollectCodeBlockLines(f)
	result := make([]string, 0, len(f.Lines))
	for i, line := range f.Lines {
		lineNum := i + 1
		if _, ok := codeLines[lineNum]; ok {
			result = append(result, string(line))
			continue
		}
		trimmed := bytes.TrimRight(line, " \t")
		result = append(result, string(trimmed))
	}
	return []byte(strings.Join(result, "\n"))
}

// FixTitle implements rule.QuickFixTitler.
func (r *Rule) FixTitle() string { return "Remove trailing whitespace" }
