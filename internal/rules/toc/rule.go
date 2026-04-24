// Package toc implements MDS038, the <?toc?> generated-section directive
// that emits a nested heading list linked to GitHub-style anchors.
package toc

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/rule"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks and fixes <?toc?>...<?/toc?> generated sections.
type Rule struct {
	engine *gensection.Engine
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS038" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "toc" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// RuleID implements gensection.Directive.
func (r *Rule) RuleID() string { return "MDS038" }

// RuleName implements gensection.Directive.
func (r *Rule) RuleName() string { return "toc" }

func (r *Rule) getEngine() *gensection.Engine {
	if r.engine == nil {
		r.engine = gensection.NewEngine(r)
	}
	return r.engine
}

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	return r.getEngine().Check(f)
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	return r.getEngine().Fix(f)
}

// Validate implements gensection.Directive.
func (r *Rule) Validate(filePath string, line int,
	params map[string]string, _ map[string]gensection.ColumnConfig,
) []lint.Diagnostic {
	_, _, diags := parseLevels(filePath, line, params)
	return diags
}

// Generate implements gensection.Directive.
func (r *Rule) Generate(f *lint.File, filePath string, line int,
	params map[string]string, _ map[string]gensection.ColumnConfig,
) (string, []lint.Diagnostic) {
	minLevel, maxLevel, diags := parseLevels(filePath, line, params)
	if len(diags) > 0 {
		return "", diags
	}

	items := mdtext.CollectTOCItems(f.AST, f.Source)

	var sb strings.Builder
	// stack tracks heading levels of included ancestors for depth computation.
	stack := make([]int, 0, 8)

	for _, item := range items {
		if item.Level < minLevel || item.Level > maxLevel {
			continue
		}
		// Pop ancestors at same or deeper level to find nesting depth.
		for len(stack) > 0 && stack[len(stack)-1] >= item.Level {
			stack = stack[:len(stack)-1]
		}
		depth := len(stack)
		stack = append(stack, item.Level)

		indent := strings.Repeat("  ", depth)
		// Escape special characters in link text to avoid breaking Markdown syntax.
		escapedText := escapeLinkText(item.Text)
		sb.WriteString(fmt.Sprintf("%s- [%s](#%s)\n", indent, escapedText, item.Anchor))
	}

	content := sb.String()
	if content == "" {
		return "", nil
	}
	// Wrap with blank lines so the list satisfies MDS014 (blank-line-around-lists).
	return "\n" + content + "\n", nil
}

// parseLevels parses and validates min-level / max-level params.
// Defaults: min-level=2, max-level=6.
func parseLevels(
	filePath string, line int, params map[string]string,
) (minLevel, maxLevel int, diags []lint.Diagnostic) {
	minLevel = 2
	maxLevel = 6

	if v, ok := params["min-level"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 6 {
			return 0, 0, []lint.Diagnostic{makeDiag(filePath, line,
				`"min-level" must be an integer between 1 and 6`)}
		}
		minLevel = n
	}

	if v, ok := params["max-level"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 6 {
			return 0, 0, []lint.Diagnostic{makeDiag(filePath, line,
				`"max-level" must be an integer between 1 and 6`)}
		}
		maxLevel = n
	}

	if maxLevel < minLevel {
		return 0, 0, []lint.Diagnostic{makeDiag(filePath, line,
			`"max-level" must be >= "min-level"`)}
	}

	return minLevel, maxLevel, nil
}

func makeDiag(filePath string, line int, msg string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     filePath,
		Line:     line,
		Column:   1,
		RuleID:   "MDS038",
		RuleName: "toc",
		Severity: lint.Error,
		Message:  msg,
	}
}

// escapeLinkText escapes special characters in link text that would break
// Markdown link syntax: backslash, opening bracket, closing bracket.
func escapeLinkText(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "[", "\\[")
	s = strings.ReplaceAll(s, "]", "\\]")
	return s
}
