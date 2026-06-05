package headingincrement

import (
	"fmt"
	"strconv"

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

// Rule checks that heading levels only increment by one.
type Rule struct {
	Placeholders []string // placeholder tokens to treat as opaque
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS003" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "heading-increment" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "heading" }

// Check implements rule.Rule. It delegates to the shared walk so a
// direct caller (the LSP, unit tests) sees the same node stream the
// engine's multiplexed dispatch feeds the visitor.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	return rule.WalkVisitor(r, f)
}

// NewNodeVisitor implements rule.NodeVisitorRule. The visitor carries
// the running prevLevel across the walk's Heading nodes, so the engine
// can fold this rule's traversal into the one shared ast.Walk. A fresh
// visitor per file resets prevLevel to 0 for each document.
func (r *Rule) NewNodeVisitor(_ *lint.File) rule.NodeVisitor {
	return &visitor{rule: r}
}

// visitor is the per-file worker: prevLevel tracks the most recent
// heading level seen so a jump of more than one level is flagged.
type visitor struct {
	rule      *Rule
	prevLevel int
}

// Kinds implements rule.NodeVisitor: only Heading nodes matter.
func (v *visitor) Kinds() []ast.NodeKind { return []ast.NodeKind{ast.KindHeading} }

// VisitNode implements rule.NodeVisitor. It mirrors the original
// ast.Walk callback exactly: placeholder headings skip the increment
// diagnostic but still update prevLevel so subsequent headings track
// correctly.
func (v *visitor) VisitNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if !entering {
		return nil
	}
	heading, ok := n.(*ast.Heading)
	if !ok {
		return nil
	}

	r := v.rule
	level := heading.Level

	// Check if this heading's text matches a configured placeholder.
	// Placeholder headings skip the increment diagnostic but still
	// update prevLevel so subsequent headings track correctly.
	isPlaceholder := len(r.Placeholders) > 0 &&
		placeholders.ContainsBodyToken(astutil.HeadingText(heading, f.Source), r.Placeholders)

	var diags []lint.Diagnostic
	if v.prevLevel == 0 {
		// First heading: should be h1
		if level > 1 && !isPlaceholder {
			line := astutil.HeadingLine(heading, f)
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     line,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  "first heading level should be 1, got " + strconv.Itoa(level),
			})
		}
	} else if level > v.prevLevel+1 && !isPlaceholder {
		line := astutil.HeadingLine(heading, f)
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     line,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message: "heading level incremented from " + strconv.Itoa(v.prevLevel) +
				" to " + strconv.Itoa(level) +
				" (expected " + strconv.Itoa(v.prevLevel+1) + ")",
		})
	}

	v.prevLevel = level
	return diags
}

var _ rule.NodeVisitorRule = (*Rule)(nil)

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "placeholders":
			toks, ok := settings.ToStringSlice(v)
			if !ok {
				return fmt.Errorf("heading-increment: placeholders must be a list of strings, got %T", v)
			}
			if err := placeholders.Validate(toks); err != nil {
				return fmt.Errorf("heading-increment: %w", err)
			}
			r.Placeholders = toks
		default:
			return fmt.Errorf("heading-increment: unknown setting %q", k)
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
)
