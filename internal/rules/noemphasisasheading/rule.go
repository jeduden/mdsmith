package noemphasisasheading

import (
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/placeholders"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that emphasis/strong emphasis is not used as a heading substitute.
// A paragraph whose only content is emphasis or strong emphasis is flagged.
type Rule struct {
	Placeholders []string // placeholder tokens to treat as opaque
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS018" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-emphasis-as-heading" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "heading" }

// Check implements rule.Rule.
// Check implements rule.Rule. The per-paragraph logic is pure and
// stateless, so it is expressed as CheckNode and the engine can fold
// this rule into one shared AST walk; a direct call still works via
// rule.WalkNodes.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	return rule.WalkNodes(r, f)
}

// CheckNode implements rule.NodeChecker.
func (r *Rule) CheckNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if !entering {
		return nil
	}
	para, ok := n.(*ast.Paragraph)
	if !ok {
		return nil
	}

	// Pipe-tables are row layouts; emphasis inside a cell is intentional
	// inline styling (a row-label stub, a bold flag column), not a
	// heading substitute. Defer to the table-format rule and skip this
	// paragraph. Issue #320.
	if astutil.IsTable(para, f) {
		return nil
	}

	// The paragraph must have exactly one child, and it must be
	// emphasis (a lone *emphasised line* masquerading as a heading).
	firstChild := para.FirstChild()
	if firstChild == nil || firstChild.NextSibling() != nil {
		return nil
	}
	if _, isEmphasis := firstChild.(*ast.Emphasis); !isEmphasis {
		return nil
	}

	// If the emphasis text contains a configured placeholder token,
	// treat it as opaque and suppress the diagnostic.
	if emphasisContainsPlaceholder(firstChild, f.Source, r.Placeholders) {
		return nil
	}

	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     astutil.ParagraphLine(para, f),
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  "emphasis used instead of a heading",
	}}
}

func emphasisContainsPlaceholder(n ast.Node, src []byte, toks []string) bool {
	if len(toks) == 0 {
		return false
	}
	var sb strings.Builder
	found := false
	_ = ast.Walk(n, func(inner ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if t, ok := inner.(*ast.Text); ok {
			sb.Write(t.Segment.Value(src))
			if placeholders.ContainsBodyToken(sb.String(), toks) {
				found = true
				return ast.WalkStop, nil
			}
		}
		return ast.WalkContinue, nil
	})
	return found
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "placeholders":
			toks, ok := settings.ToStringSlice(v)
			if !ok {
				return fmt.Errorf("no-emphasis-as-heading: placeholders must be a list of strings, got %T", v)
			}
			if err := placeholders.Validate(toks); err != nil {
				return fmt.Errorf("no-emphasis-as-heading: %w", err)
			}
			r.Placeholders = toks
		default:
			return fmt.Errorf("no-emphasis-as-heading: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"placeholders": []string{},
	}
}

// SettingMergeMode implements rule.ListMerger.
func (r *Rule) SettingMergeMode(key string) rule.MergeMode {
	if key == "placeholders" {
		return rule.MergeAppend
	}
	return rule.MergeReplace
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.ListMerger   = (*Rule)(nil)
	_ rule.NodeChecker  = (*Rule)(nil)
)
