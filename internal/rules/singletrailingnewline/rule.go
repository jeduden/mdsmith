package singletrailingnewline

import (
	"bytes"

	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that a file ends with exactly one trailing newline.
type Rule struct{}

func (r *Rule) ID() string   { return "TM009" }
func (r *Rule) Name() string { return "single-trailing-newline" }

func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	src := f.Source

	// Empty file: report diagnostic
	if len(src) == 0 {
		return []lint.Diagnostic{{
			File:     f.Path,
			Line:     1,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  "file must end with a single newline",
		}}
	}

	// File doesn't end with newline
	if src[len(src)-1] != '\n' {
		return []lint.Diagnostic{{
			File:     f.Path,
			Line:     len(f.Lines),
			Column:   len(f.Lines[len(f.Lines)-1]) + 1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  "file must end with a single newline",
		}}
	}

	// Check for multiple trailing newlines
	trimmed := bytes.TrimRight(src, "\n")
	trailingCount := len(src) - len(trimmed)
	if trailingCount > 1 {
		// Report diagnostic on the line after the last content line
		return []lint.Diagnostic{{
			File:     f.Path,
			Line:     len(f.Lines),
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  "file must end with a single newline",
		}}
	}

	return nil
}

func (r *Rule) Fix(f *lint.File) []byte {
	src := f.Source

	// Empty file: return a single newline
	if len(src) == 0 {
		return []byte("\n")
	}

	trimmed := bytes.TrimRight(src, "\n")
	return append(trimmed, '\n')
}
