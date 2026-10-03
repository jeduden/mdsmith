package notrailingspaces

import (
	"bytes"

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
	// Fetched on the first candidate line, saving a map lookup on every
	// other line.
	var codeLines map[int]struct{}
	codeLinesReady := false
	var diags []lint.Diagnostic
	for i, line := range f.Lines {
		lineNum := i + 1
		trimmed := bytes.TrimRight(line, " \t")
		if len(trimmed) < len(line) {
			// Consult the code-line map only for the rare candidate line.
			if !codeLinesReady {
				codeLines, codeLinesReady = lint.CollectCodeBlockLines(f), true
			}
			if _, ok := codeLines[lineNum]; ok {
				continue
			}
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
	result := make([][]byte, 0, len(f.Lines))
	for i, line := range f.Lines {
		lineNum := i + 1
		if _, ok := codeLines[lineNum]; ok {
			result = append(result, line)
			continue
		}
		result = append(result, bytes.TrimRight(line, " \t"))
	}
	return bytes.Join(result, []byte("\n"))
}

// FixTitle implements rule.QuickFixTitler.
func (r *Rule) FixTitle() string { return "Remove trailing whitespace" }
