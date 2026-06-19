package firstlineheading

import (
	"fmt"
	"strconv"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/placeholders"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
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

// LineCapable implements rule.LineCapable. With no placeholder tokens
// configured the verdict reads only the first block's kind, position, and
// heading level — all from the Layer 0 block scan — so the rule can lint
// the parse-skipped File. A configured placeholder list makes the rule
// read the first heading's inline text, which lives in the AST, so it
// falls back to the parse path.
func (r *Rule) LineCapable() bool { return len(r.Placeholders) == 0 }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f != nil && f.AST == nil {
		return r.checkNilAST(f)
	}

	level := r.Level
	if level == 0 {
		level = 1
	}

	if len(f.Source) == 0 {
		return r.diag(f, "first line should be a level "+strconv.Itoa(level)+" heading")
	}

	firstChild := f.AST.FirstChild()
	if firstChild == nil {
		return r.diag(f, "first line should be a level "+strconv.Itoa(level)+" heading")
	}

	heading, ok := firstChild.(*ast.Heading)
	if !ok {
		return r.diag(f, "first line should be a level "+strconv.Itoa(level)+" heading")
	}

	if headingLine(heading, f) != 1 {
		return r.diag(f, "first line should be a level "+strconv.Itoa(level)+" heading, found blank line")
	}

	if heading.Level != level {
		// If the heading text matches a configured placeholder token,
		// treat it as opaque and suppress the level diagnostic.
		text := astutil.HeadingText(heading, f.Source)
		if placeholders.ContainsBodyToken(text, r.Placeholders) {
			return nil
		}
		return r.diag(f, "first heading should be level "+strconv.Itoa(level)+", got "+strconv.Itoa(heading.Level))
	}

	return nil
}

// checkNilAST is the parse-skip path: it reads the first block span from
// the Layer 0 scan and replicates the AST path's verdict exactly. It runs
// only with empty placeholders (LineCapable gates the skip); a
// placeholder-configured rule would need the heading's inline text, so it
// returns nil defensively if ever reached with one.
func (r *Rule) checkNilAST(f *lint.File) []lint.Diagnostic {
	if len(r.Placeholders) > 0 {
		return nil
	}
	level := r.Level
	if level == 0 {
		level = 1
	}

	if len(f.Source) == 0 {
		return r.diag(f, "first line should be a level "+strconv.Itoa(level)+" heading")
	}

	spans := lint.Layer0(f).BlockSpans
	if len(spans) == 0 {
		return r.diag(f, "first line should be a level "+strconv.Itoa(level)+" heading")
	}
	first := spans[0]
	if first.Kind != lint.BlockATXHeading && first.Kind != lint.BlockSetextHeading {
		return r.diag(f, "first line should be a level "+strconv.Itoa(level)+" heading")
	}
	if first.Start != 1 {
		return r.diag(f, "first line should be a level "+strconv.Itoa(level)+" heading, found blank line")
	}
	gotLevel := headingLevelFromSpan(f, first)
	if gotLevel != level {
		return r.diag(f, "first heading should be level "+strconv.Itoa(level)+", got "+strconv.Itoa(gotLevel))
	}
	return nil
}

// headingLevelFromSpan returns the heading level for a Layer 0 heading
// span: the leading-`#` count for ATX, or 1/2 from the setext underline.
func headingLevelFromSpan(f *lint.File, span lint.BlockSpan) int {
	if span.Kind == lint.BlockATXHeading {
		line := f.Lines[span.Start-1]
		i := 0
		for i < len(line) && i < 3 && line[i] == ' ' {
			i++
		}
		level := 0
		for i < len(line) && line[i] == '#' {
			level++
			i++
		}
		return level
	}
	line := f.Lines[span.End-1]
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	if i < len(line) && line[i] == '=' {
		return 1
	}
	return 2
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

// SettingMergeMode implements rule.ListMerger. The placeholder vocabulary
// concatenates across config layers so a kind can extend the inherited
// token list without restating it.
func (r *Rule) SettingMergeMode(key string) rule.MergeMode {
	if key == "placeholders" {
		return rule.MergeAppend
	}
	return rule.MergeReplace
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.ListMerger   = (*Rule)(nil)
	_ rule.LineCapable  = (*Rule)(nil)
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
