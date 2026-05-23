// Package requiredmentions implements MDS058, which flags
// heading-bounded sections whose body text does not contain every
// configured substring.
package requiredmentions

import (
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/jeduden/mdsmith/internal/rules/settings"
)

func init() {
	rule.Register(&Rule{})
}

// Rule flags heading-bounded sections that do not mention every
// configured substring at least once. Walks every heading and tests
// each entry in `mentions:` against the section's prose (including
// nested sub-sections). A missing mention emits one diagnostic at the
// section's heading line so the per-scope override from plan 146 can
// retain it via line-range filtering.
type Rule struct {
	Mentions []string
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS058" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "required-mentions" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "prose" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f.AST == nil || len(r.Mentions) == 0 {
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
		for _, m := range r.Mentions {
			if m == "" {
				continue
			}
			if strings.Contains(body, m) {
				continue
			}
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     h.Line,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message: fmt.Sprintf(
					"section is missing required mention %q", m,
				),
			})
		}
	}
	return diags
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "mentions":
			ss, ok := settings.ToStringSlice(v)
			if !ok {
				return fmt.Errorf(
					"required-mentions: mentions must be a list of strings, got %T",
					v,
				)
			}
			r.Mentions = ss
		default:
			return fmt.Errorf(
				"required-mentions: unknown setting %q", k,
			)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"mentions": []string{},
	}
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
)
