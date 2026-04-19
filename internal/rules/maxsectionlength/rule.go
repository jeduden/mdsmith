package maxsectionlength

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule enforces per-section line-count limits.
//
// Section length is the count of lines from the heading line up to (but not
// including) the next heading line of any level, or the end of file. Nested
// subsections are therefore measured separately from their parent.
type Rule struct {
	// Max is the default per-section line limit. Zero means no global limit.
	Max int
	// PerLevel overrides Max for a specific heading level (1-6).
	PerLevel map[int]int
	// PerHeading overrides Max and PerLevel when the heading text matches.
	// The first pattern that matches wins.
	PerHeading []HeadingPattern
}

// HeadingPattern is a regex pattern matched against heading text with an
// associated maximum section length.
type HeadingPattern struct {
	Pattern string
	Regex   *regexp.Regexp
	Max     int
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS036" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "max-section-length" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "heading" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f.AST == nil {
		return nil
	}
	headings := collectHeadings(f)
	if len(headings) == 0 {
		return nil
	}

	totalLines := len(f.Lines)
	if totalLines > 0 && len(f.Lines[totalLines-1]) == 0 {
		totalLines--
	}

	var diags []lint.Diagnostic
	for i, h := range headings {
		end := totalLines
		if i+1 < len(headings) {
			end = headings[i+1].line - 1
		}
		length := end - h.line + 1
		max := r.resolveMax(h)
		if max <= 0 {
			continue
		}
		if length <= max {
			continue
		}

		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     h.line,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message: fmt.Sprintf(
				"section %q too long (%d > %d)",
				h.label(), length, max,
			),
		})
	}
	return diags
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "max":
			n, ok := toInt(v)
			if !ok {
				return fmt.Errorf(
					"max-section-length: max must be an integer, got %T", v,
				)
			}
			if n < 0 {
				return fmt.Errorf(
					"max-section-length: max must be non-negative, got %d", n,
				)
			}
			r.Max = n
		case "per-level":
			m, err := parsePerLevel(v)
			if err != nil {
				return err
			}
			r.PerLevel = m
		case "per-heading":
			patterns, err := parsePerHeading(v)
			if err != nil {
				return err
			}
			r.PerHeading = patterns
		default:
			return fmt.Errorf(
				"max-section-length: unknown setting %q", k,
			)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable. All keys are returned so
// re-applying defaults (e.g., test cleanup) fully clears PerLevel and
// PerHeading, not just Max.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"max":         0,
		"per-level":   map[string]any{},
		"per-heading": []any{},
	}
}

func (r *Rule) resolveMax(h heading) int {
	for _, p := range r.PerHeading {
		if p.Regex != nil && p.Regex.MatchString(h.text) {
			return p.Max
		}
	}
	if m, ok := r.PerLevel[h.level]; ok {
		return m
	}
	return r.Max
}

type heading struct {
	level int
	text  string
	line  int
}

func (h heading) label() string {
	return strings.Repeat("#", h.level) + " " + h.text
}

func collectHeadings(f *lint.File) []heading {
	var out []heading
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		out = append(out, heading{
			level: h.Level,
			text:  strings.TrimSpace(mdtext.ExtractPlainText(h, f.Source)),
			line:  headingLine(h, f),
		})
		return ast.WalkSkipChildren, nil
	})
	sort.Slice(out, func(i, j int) bool {
		return out[i].line < out[j].line
	})
	return out
}

func headingLine(h *ast.Heading, f *lint.File) int {
	if lines := h.Lines(); lines.Len() > 0 {
		return f.LineOfOffset(lines.At(0).Start)
	}
	for c := h.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			return f.LineOfOffset(t.Segment.Start)
		}
	}
	return 1
}

func parsePerLevel(v any) (map[int]int, error) {
	raw, err := asStringMap(v)
	if err != nil {
		return nil, fmt.Errorf(
			"max-section-length: per-level %w", err,
		)
	}
	out := make(map[int]int, len(raw))
	for k, val := range raw {
		level, err := strconv.Atoi(k)
		if err != nil {
			return nil, fmt.Errorf(
				"max-section-length: per-level key %q must be an integer 1-6", k,
			)
		}
		if level < 1 || level > 6 {
			return nil, fmt.Errorf(
				"max-section-length: per-level key %d out of range (1-6)", level,
			)
		}
		n, ok := toInt(val)
		if !ok {
			return nil, fmt.Errorf(
				"max-section-length: per-level[%d] must be an integer, got %T",
				level, val,
			)
		}
		if n < 0 {
			return nil, fmt.Errorf(
				"max-section-length: per-level[%d] must be non-negative", level,
			)
		}
		out[level] = n
	}
	return out, nil
}

func parsePerHeading(v any) ([]HeadingPattern, error) {
	items, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf(
			"max-section-length: per-heading must be a list, got %T", v,
		)
	}
	settings := make([]patternSetting, 0, len(items))
	for i, item := range items {
		m, err := asStringMap(item)
		if err != nil {
			return nil, fmt.Errorf(
				"max-section-length: per-heading[%d] %w", i, err,
			)
		}
		pattern, _ := m["pattern"].(string)
		if pattern == "" {
			return nil, fmt.Errorf(
				"max-section-length: per-heading[%d].pattern must be a non-empty string",
				i,
			)
		}
		max, ok := toInt(m["max"])
		if !ok {
			return nil, fmt.Errorf(
				"max-section-length: per-heading[%d].max must be an integer", i,
			)
		}
		if max < 0 {
			return nil, fmt.Errorf(
				"max-section-length: per-heading[%d].max must be non-negative", i,
			)
		}
		settings = append(settings, patternSetting{Pattern: pattern, Max: max})
	}
	return compilePatterns(settings)
}

type patternSetting struct {
	Pattern string
	Max     int
}

func compilePatterns(in []patternSetting) ([]HeadingPattern, error) {
	out := make([]HeadingPattern, 0, len(in))
	for _, p := range in {
		re, err := regexp.Compile(p.Pattern)
		if err != nil {
			return nil, fmt.Errorf(
				"max-section-length: invalid pattern %q: %w", p.Pattern, err,
			)
		}
		out = append(out, HeadingPattern{
			Pattern: p.Pattern,
			Regex:   re,
			Max:     p.Max,
		})
	}
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

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		if n != float64(int(n)) {
			return 0, false
		}
		return int(n), true
	}
	return 0, false
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
)
