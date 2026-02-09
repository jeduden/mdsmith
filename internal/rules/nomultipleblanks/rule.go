package nomultipleblanks

import (
	"bytes"
	"strings"

	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that there are no consecutive blank lines.
type Rule struct{}

func (r *Rule) ID() string   { return "TM008" }
func (r *Rule) Name() string { return "no-multiple-blanks" }

// isBlank returns true if the line contains only whitespace.
func isBlank(line []byte) bool {
	return len(bytes.TrimSpace(line)) == 0
}

func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	prevBlank := false
	for i, line := range f.Lines {
		blank := isBlank(line)
		if blank && prevBlank {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     i + 1,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  "multiple consecutive blank lines",
			})
		}
		prevBlank = blank
	}
	return diags
}

func (r *Rule) Fix(f *lint.File) []byte {
	var result []string
	prevBlank := false
	for _, line := range f.Lines {
		blank := isBlank(line)
		if blank && prevBlank {
			continue
		}
		result = append(result, string(line))
		prevBlank = blank
	}
	return []byte(strings.Join(result, "\n"))
}
