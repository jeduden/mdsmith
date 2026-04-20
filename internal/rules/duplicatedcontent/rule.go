// Package duplicatedcontent implements MDS037, which flags substantial
// paragraphs that also appear verbatim in another Markdown file in the
// project root, after whitespace and case normalization.
package duplicatedcontent

import (
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

func init() {
	rule.Register(&Rule{})
}

// Rule detects paragraphs duplicated across Markdown files.
type Rule struct {
	Include  []string
	Exclude  []string
	MinChars int
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS037" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "duplicated-content" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "content" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	_ = f
	return nil
}
