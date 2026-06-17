package headingincrement

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

// LineCapable implements rule.LineCapable. With no placeholder tokens
// configured the per-heading verdict reads only heading levels, which the
// Layer 0 block scan supplies, so the rule can lint the parse-skipped File.
// A configured placeholder list makes the rule read heading inline text
// (placeholders.ContainsBodyToken), which lives in the AST, so it falls
// back to the parse path.
func (r *Rule) LineCapable() bool { return len(r.Placeholders) == 0 }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f != nil && f.AST == nil {
		return r.checkNilAST(f)
	}
	var diags []lint.Diagnostic
	prevLevel := 0

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		heading, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}

		level := heading.Level

		// Check if this heading's text matches a configured placeholder.
		// Placeholder headings skip the increment diagnostic but still
		// update prevLevel so subsequent headings track correctly.
		isPlaceholder := len(r.Placeholders) > 0 &&
			placeholders.ContainsBodyToken(astutil.HeadingText(heading, f.Source), r.Placeholders)

		if prevLevel == 0 {
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
		} else if level > prevLevel+1 && !isPlaceholder {
			line := astutil.HeadingLine(heading, f)
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     line,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message: "heading level incremented from " + strconv.Itoa(prevLevel) +
					" to " + strconv.Itoa(level) +
					" (expected " + strconv.Itoa(prevLevel+1) + ")",
			})
		}

		prevLevel = level
		return ast.WalkContinue, nil
	})

	return diags
}

// checkNilAST is the parse-skip path: it walks the Layer 0 block scan's
// heading spans (top-level only — the gate excludes container-nested
// headings) and replicates the AST path's prevLevel sequence exactly,
// reading levels from each heading's source line. It runs only with empty
// placeholders (LineCapable gates the skip); a placeholder-configured rule
// would need heading inline text, so it returns nil defensively if ever
// reached with one.
func (r *Rule) checkNilAST(f *lint.File) []lint.Diagnostic {
	if len(r.Placeholders) > 0 {
		return nil
	}
	var diags []lint.Diagnostic
	prevLevel := 0
	// The gate (layer0SkipEligible) excludes any file that may hold a block
	// quote or list, so every heading span here is top-level (Depth 0); the
	// scanner never tags a heading span with a nesting depth.
	for _, span := range lint.Layer0(f).BlockSpans {
		level := 0
		switch span.Kind {
		case lint.BlockATXHeading:
			level = atxLevelFromLine(f.Lines[span.Start-1])
		case lint.BlockSetextHeading:
			level = setextLevelFromSpan(f, span)
		default:
			continue
		}
		if prevLevel == 0 {
			if level > 1 {
				diags = append(diags, lint.Diagnostic{
					File:     f.Path,
					Line:     span.Start,
					Column:   1,
					RuleID:   r.ID(),
					RuleName: r.Name(),
					Severity: lint.Warning,
					Message:  "first heading level should be 1, got " + strconv.Itoa(level),
				})
			}
		} else if level > prevLevel+1 {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     span.Start,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message: "heading level incremented from " + strconv.Itoa(prevLevel) +
					" to " + strconv.Itoa(level) +
					" (expected " + strconv.Itoa(prevLevel+1) + ")",
			})
		}
		prevLevel = level
	}
	return diags
}

// atxLevelFromLine returns the ATX heading level (number of leading `#`,
// 1–6) for a line the Layer 0 scan classified BlockATXHeading. Up to three
// leading spaces are skipped first, matching goldmark's ATX parse.
func atxLevelFromLine(line []byte) int {
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

// setextLevelFromSpan returns the level of a BlockSetextHeading span: 1 when
// its underline (the span's last line) starts with `=`, 2 when it starts
// with `-`. Up to three leading spaces are skipped, matching the scanner's
// isSetextUnderline.
func setextLevelFromSpan(f *lint.File, span lint.BlockSpan) int {
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
	_ rule.LineCapable  = (*Rule)(nil)
)
