package nohardtabs

import (
	"bytes"
	"strings"

	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that no line contains hard tab characters.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "TM007" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-hard-tabs" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "whitespace" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	codeLines := lint.CollectCodeBlockLines(f)
	var diags []lint.Diagnostic
	for i, line := range f.Lines {
		lineNum := i + 1
		if codeLines[lineNum] {
			continue
		}
		idx := bytes.IndexByte(line, '\t')
		if idx >= 0 {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     lineNum,
				Column:   idx + 1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  "hard tab character",
			})
		}
	}
	return diags
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	codeLines := lint.CollectCodeBlockLines(f)
	var result []string
	for i, line := range f.Lines {
		lineNum := i + 1
		if codeLines[lineNum] {
			result = append(result, string(line))
			continue
		}
		replaced := strings.ReplaceAll(string(line), "\t", "    ")
		result = append(result, replaced)
	}
	return []byte(strings.Join(result, "\n"))
}
