// Package forbiddenparagraphstarts implements MDS055, which flags
// paragraphs whose plain text begins with any configured prefix.
package forbiddenparagraphstarts

import (
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule flags paragraphs whose plain text begins with one of the
// configured prefixes. The prefix match is case-sensitive on the
// trimmed paragraph text.
type Rule struct {
	Starts []string
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS055" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "forbidden-paragraph-starts" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "prose" }

// EnabledByDefault implements rule.Defaultable. MDS055 is opt-in;
// teams enable it for prose rules ("avoid starting paragraphs with
// 'We'") or for per-section overrides through a schema.
func (r *Rule) EnabledByDefault() bool { return false }

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
	if f.AST == nil || len(r.Starts) == 0 {
		return nil
	}
	para, ok := n.(*ast.Paragraph)
	if !ok {
		return nil
	}
	if astutil.IsTable(para, f) {
		return nil
	}
	text := strings.TrimLeft(mdtext.ExtractPlainText(para, f.Source), " \t")
	for _, prefix := range r.Starts {
		if prefix == "" {
			continue
		}
		if strings.HasPrefix(text, prefix) {
			return []lint.Diagnostic{{
				File:     f.Path,
				Line:     astutil.ParagraphLine(para, f),
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message: fmt.Sprintf(
					"paragraph starts with forbidden prefix %q",
					prefix,
				),
			}}
		}
	}
	return nil
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "starts":
			ss, ok := settings.ToStringSlice(v)
			if !ok {
				return fmt.Errorf(
					"forbidden-paragraph-starts: starts must be a list of strings, got %T",
					v,
				)
			}
			r.Starts = ss
		default:
			return fmt.Errorf(
				"forbidden-paragraph-starts: unknown setting %q", k,
			)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"starts": []string{},
	}
}

// SettingMergeMode implements rule.ListMerger. The "starts" list
// appends across config layers so a project that pins a convention
// (e.g. no-llm-tells) and then adds its own forbidden openers unions
// the two lists rather than replacing the convention's.
func (r *Rule) SettingMergeMode(key string) rule.MergeMode {
	if key == "starts" {
		return rule.MergeAppend
	}
	return rule.MergeReplace
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
	_ rule.ListMerger   = (*Rule)(nil)
	_ rule.NodeChecker  = (*Rule)(nil)
)
