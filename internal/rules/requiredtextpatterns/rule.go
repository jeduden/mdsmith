// Package requiredtextpatterns implements MDS057, which flags
// heading-bounded sections whose body text does not match a configured
// regex.
package requiredtextpatterns

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/jeduden/mdsmith/internal/rules/settings"
)

func init() {
	rule.Register(&Rule{})
}

// Rule flags heading-bounded sections whose plain text does not match a
// configured regular expression. Walks every heading in the document
// and, for each one, gathers the prose under the heading (paragraphs,
// including those in nested sub-sections) and tests each configured
// pattern against the gathered text. A failing pattern emits one
// diagnostic anchored at the heading line so the per-scope override
// from plan 146 keeps only diagnostics that fall inside the configured
// scope's line range.
type Rule struct {
	Patterns []Pattern
}

// Pattern is a compiled required-text pattern with optional message
// and skip-indices.
type Pattern struct {
	Source      string
	Regex       *regexp.Regexp
	Message     string
	SkipIndices []int
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS057" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "required-text-patterns" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "prose" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f.AST == nil || len(r.Patterns) == 0 {
		return nil
	}
	headings := astutil.CollectSectionHeadings(f)
	if len(headings) == 0 {
		return nil
	}
	// Pre-materialise text once per file: SectionBody runs per
	// heading and would otherwise re-extract the same paragraph
	// once per containing section (h1 > h2 > h3 = 3x). The
	// memoized variant restores the plan-196 pre-image cost.
	paragraphs := astutil.CollectSectionParagraphsWithText(f)

	totalLines := len(f.Lines)
	if totalLines > 0 && len(f.Lines[totalLines-1]) == 0 {
		totalLines--
	}

	var diags []lint.Diagnostic
	for i, h := range headings {
		end := astutil.SectionEnd(headings, i, totalLines)
		body := astutil.SectionBody(paragraphs, f.Source, h.Line, end)
		for _, p := range r.Patterns {
			if p.Regex.MatchString(body) {
				continue
			}
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     h.Line,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  formatMessage(p),
			})
		}
	}
	return diags
}

func formatMessage(p Pattern) string {
	if p.Message != "" {
		return fmt.Sprintf("required text missing: %s", p.Message)
	}
	return fmt.Sprintf("required text pattern not matched: %s", p.Source)
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "patterns":
			ps, err := parsePatterns(v)
			if err != nil {
				return err
			}
			r.Patterns = ps
		default:
			return fmt.Errorf(
				"required-text-patterns: unknown setting %q", k,
			)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"patterns": []any{},
	}
}

func parsePatterns(v any) ([]Pattern, error) {
	items, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf(
			"required-text-patterns: patterns must be a list, got %T", v,
		)
	}
	out := make([]Pattern, 0, len(items))
	for i, item := range items {
		m, err := asStringMap(item)
		if err != nil {
			return nil, fmt.Errorf(
				"required-text-patterns: patterns[%d] %w", i, err,
			)
		}
		patternStr, _ := m["pattern"].(string)
		if patternStr == "" {
			return nil, fmt.Errorf(
				"required-text-patterns: patterns[%d].pattern must be a non-empty string",
				i,
			)
		}
		re, err := regexp.Compile(patternStr)
		if err != nil {
			return nil, fmt.Errorf(
				"required-text-patterns: patterns[%d].pattern invalid: %w", i, err,
			)
		}
		msg, _ := m["message"].(string)
		skip, err := parseSkipIndices(m["skip-indices"])
		if err != nil {
			return nil, fmt.Errorf(
				"required-text-patterns: patterns[%d].skip-indices %w", i, err,
			)
		}
		out = append(out, Pattern{
			Source:      patternStr,
			Regex:       re,
			Message:     msg,
			SkipIndices: skip,
		})
	}
	return out, nil
}

func parseSkipIndices(v any) ([]int, error) {
	if v == nil {
		return nil, nil
	}
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("must be a list, got %T", v)
	}
	out := make([]int, 0, len(list))
	for i, item := range list {
		n, ok := settings.ToInt(item)
		if !ok {
			return nil, fmt.Errorf("entry %d must be an integer, got %T", i, item)
		}
		out = append(out, n)
	}
	sort.Ints(out)
	return out, nil
}

func asStringMap(v any) (map[string]any, error) {
	switch m := v.(type) {
	case map[string]any:
		return m, nil
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			out[fmt.Sprint(k)] = val
		}
		return out, nil
	}
	return nil, fmt.Errorf("must be a map, got %T", v)
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
)
