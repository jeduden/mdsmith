package firstlineheading

import (
	"fmt"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/placeholders"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{Level: 1})
}

// Rule checks that the first line of the file is a heading of the configured level.
type Rule struct {
	Level        int      // expected heading level (default: 1)
	Placeholders []string // placeholder tokens to treat as opaque
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS004" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "first-line-heading" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "heading" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	level := r.Level
	if level == 0 {
		level = 1
	}

	missingMsg := fmt.Sprintf("first line should be a level %d heading", level)

	if len(f.Source) == 0 {
		return r.diag(f, missingMsg)
	}

	firstChild := f.AST.FirstChild()
	if firstChild == nil {
		return r.diag(f, missingMsg)
	}

	heading, ok := firstChild.(*ast.Heading)
	if !ok {
		return r.diag(f, missingMsg)
	}

	if headingLine(heading, f) != 1 {
		return r.diag(f, fmt.Sprintf("first line should be a level %d heading, found blank line", level))
	}

	if heading.Level != level {
		// If the heading text matches a configured placeholder token,
		// treat it as opaque and suppress the level diagnostic.
		text := astutil.HeadingText(heading, f.Source)
		if placeholders.ContainsBodyToken(text, r.Placeholders) {
			return nil
		}
		return r.diag(f, fmt.Sprintf("first heading should be level %d, got %d", level, heading.Level))
	}

	return nil
}

// diag returns a single-element diagnostic slice for a line-1 issue.
func (r *Rule) diag(f *lint.File, msg string) []lint.Diagnostic {
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     1,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  msg,
	}}
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "level":
			n, ok := settings.ToInt(v)
			if !ok {
				return fmt.Errorf("first-line-heading: level must be an integer, got %T", v)
			}
			if n < 1 || n > 6 {
				return fmt.Errorf("first-line-heading: level must be 1-6, got %d", n)
			}
			r.Level = n
		case "placeholders":
			toks, ok := settings.ToStringSlice(v)
			if !ok {
				return fmt.Errorf("first-line-heading: placeholders must be a list of strings, got %T", v)
			}
			if err := placeholders.Validate(toks); err != nil {
				return fmt.Errorf("first-line-heading: %w", err)
			}
			r.Placeholders = toks
		default:
			return fmt.Errorf("first-line-heading: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"level":        1,
		"placeholders": []string{},
	}
}

// ListMergeMode implements rule.ListMerger: placeholders concatenate
// across config layers; other keys (none today) replace by default.
func (r *Rule) ListMergeMode(key string) rule.ListMergeMode {
	if key == "placeholders" {
		return rule.ListAppend
	}
	return rule.ListReplace
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.ListMerger   = (*Rule)(nil)
)

func headingLine(heading *ast.Heading, f *lint.File) int {
	lines := heading.Lines()
	if lines.Len() > 0 {
		return f.LineOfOffset(lines.At(0).Start)
	}
	for c := heading.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			return f.LineOfOffset(t.Segment.Start)
		}
	}
	// Empty headings (e.g. "# \n") have no text segments.
	// Detect whether the first line is blank (only spaces/tabs
	// before a newline). Markdown treats such lines as blank,
	// so a heading on the following line starts on line 2.
	for i := 0; i < len(f.Source); i++ {
		b := f.Source[i]
		if b == ' ' || b == '\t' {
			continue
		}
		if b == '\n' || b == '\r' {
			return 2
		}
		return 1
	}
	return 1
}
