package linelength

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{
		Max:     80,
		Exclude: []string{"code-blocks", "tables", "urls"},
	})
}

// Rule checks that no line exceeds the configured maximum length.
// Lines matching categories in Exclude are skipped. Valid exclude
// values: "code-blocks", "tables", "urls".
type Rule struct {
	Max          int
	HeadingMax   *int
	CodeBlockMax *int
	Stern        bool
	Exclude      []string
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "TM001" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "line-length" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "line" }

// urlOnlyRe matches a line whose trimmed content is a single URL.
var urlOnlyRe = regexp.MustCompile(`^https?://\S+$`)

// tableLineRe matches a line whose trimmed content starts with a pipe.
var tableLineRe = regexp.MustCompile(`^\s*\|`)

// isExcluded returns true if the given category is in the Exclude list.
func (r *Rule) isExcluded(category string) bool {
	for _, e := range r.Exclude {
		if e == category {
			return true
		}
	}
	return false
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		if err := r.applySetting(k, v); err != nil {
			return err
		}
	}
	return nil
}

func (r *Rule) applySetting(k string, v any) error {
	switch k {
	case "max":
		return r.applyMax(v)
	case "heading-max":
		return r.applyPositiveIntPtr(v, "heading-max", &r.HeadingMax)
	case "code-block-max":
		return r.applyPositiveIntPtr(v, "code-block-max", &r.CodeBlockMax)
	case "stern":
		return r.applyStern(v)
	case "exclude":
		return r.applyExclude(v)
	case "strict":
		return r.applyStrict(v)
	default:
		return fmt.Errorf("line-length: unknown setting %q", k)
	}
}

func (r *Rule) applyMax(v any) error {
	n, ok := toInt(v)
	if !ok {
		return fmt.Errorf("line-length: max must be an integer, got %T", v)
	}
	r.Max = n
	return nil
}

func (r *Rule) applyPositiveIntPtr(v any, name string, target **int) error {
	n, ok := toInt(v)
	if !ok {
		return fmt.Errorf("line-length: %s must be an integer, got %T", name, v)
	}
	if n <= 0 {
		return fmt.Errorf("line-length: %s must be positive, got %d", name, n)
	}
	*target = &n
	return nil
}

func (r *Rule) applyStern(v any) error {
	b, ok := v.(bool)
	if !ok {
		return fmt.Errorf("line-length: stern must be a bool, got %T", v)
	}
	r.Stern = b
	return nil
}

func (r *Rule) applyExclude(v any) error {
	list, ok := toStringSlice(v)
	if !ok {
		return fmt.Errorf("line-length: exclude must be a list of strings, got %T", v)
	}
	for _, item := range list {
		if !isValidExclude(item) {
			return fmt.Errorf("line-length: invalid exclude value %q (valid: code-blocks, tables, urls)", item)
		}
	}
	r.Exclude = list
	return nil
}

func (r *Rule) applyStrict(v any) error {
	b, ok := v.(bool)
	if !ok {
		return fmt.Errorf("line-length: strict must be a bool, got %T", v)
	}
	// Deprecation shim: translate strict to exclude.
	if b {
		r.Exclude = []string{}
	} else {
		r.Exclude = []string{"code-blocks", "tables", "urls"}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"max":     80,
		"exclude": []string{"code-blocks", "tables", "urls"},
		"stern":   false,
	}
}

// lineCategories holds pre-computed line classification maps.
type lineCategories struct {
	code    map[int]bool
	table   map[int]bool
	heading map[int]bool
}

func (r *Rule) buildCategories(f *lint.File) lineCategories {
	lc := lineCategories{
		code:    map[int]bool{},
		table:   map[int]bool{},
		heading: map[int]bool{},
	}
	if r.isExcluded("code-blocks") || r.CodeBlockMax != nil {
		lc.code = lint.CollectCodeBlockLines(f)
	}
	if r.isExcluded("tables") {
		lc.table = collectTableLines(f)
	}
	if r.HeadingMax != nil {
		lc.heading = collectHeadingLines(f)
	}
	return lc
}

// activeMax returns the effective maximum for a line given its categories.
func (r *Rule) activeMax(baseMax int, lc lineCategories, lineNum int) int {
	if lc.heading[lineNum] && r.HeadingMax != nil {
		return *r.HeadingMax
	}
	if lc.code[lineNum] && r.CodeBlockMax != nil {
		return *r.CodeBlockMax
	}
	return baseMax
}

// isSkipped returns true if the line should be excluded from checking.
func (r *Rule) isSkipped(line []byte, lineNum, limit int, lc lineCategories) bool {
	if r.isExcluded("code-blocks") && lc.code[lineNum] {
		return true
	}
	if r.isExcluded("tables") && lc.table[lineNum] {
		return true
	}
	if r.isExcluded("urls") && urlOnlyRe.MatchString(strings.TrimSpace(string(line))) {
		return true
	}
	if r.Stern && !hasSpacePastLimit(line, limit) {
		return true
	}
	return false
}

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	baseMax := r.Max
	if baseMax <= 0 {
		baseMax = 80
	}

	lc := r.buildCategories(f)

	var diags []lint.Diagnostic
	for i, line := range f.Lines {
		lineNum := i + 1
		limit := r.activeMax(baseMax, lc, lineNum)

		if len(line) <= limit {
			continue
		}
		if r.isSkipped(line, lineNum, limit, lc) {
			continue
		}

		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     lineNum,
			Column:   limit + 1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  fmt.Sprintf("line too long (%d > %d)", len(line), limit),
		})
	}

	return diags
}

// hasSpacePastLimit returns true if line contains a space byte at or beyond
// the given limit column (0-indexed byte position).
func hasSpacePastLimit(line []byte, limit int) bool {
	for i := limit; i < len(line); i++ {
		if line[i] == ' ' {
			return true
		}
	}
	return false
}

// collectTableLines returns a set of 1-based line numbers that are table rows.
func collectTableLines(f *lint.File) map[int]bool {
	lines := map[int]bool{}
	for i, line := range f.Lines {
		if tableLineRe.Match(line) {
			lines[i+1] = true
		}
	}
	return lines
}

// collectHeadingLines walks the AST and returns a set of 1-based line numbers
// that are heading lines.
func collectHeadingLines(f *lint.File) map[int]bool {
	lines := map[int]bool{}
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		ln := headingLineNum(h, f)
		if ln > 0 {
			lines[ln] = true
		}
		return ast.WalkContinue, nil
	})
	return lines
}

// headingLineNum returns the 1-based line number of a heading node.
func headingLineNum(h *ast.Heading, f *lint.File) int {
	if h.Lines().Len() > 0 {
		return f.LineOfOffset(h.Lines().At(0).Start)
	}
	// ATX headings may have no Lines(); find line via child text nodes.
	for c := h.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			return f.LineOfOffset(t.Segment.Start)
		}
	}
	return 0
}

// toInt converts a value to int. Supports int and float64 (YAML decodes
// numbers as int or float64 depending on context).
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	case int64:
		return int(n), true
	}
	return 0, false
}

// toStringSlice converts a value to []string. YAML decodes sequences as
// []any with string elements.
func toStringSlice(v any) ([]string, bool) {
	switch s := v.(type) {
	case []string:
		return s, true
	case []any:
		result := make([]string, 0, len(s))
		for _, item := range s {
			str, ok := item.(string)
			if !ok {
				return nil, false
			}
			result = append(result, str)
		}
		return result, true
	}
	return nil, false
}

func isValidExclude(s string) bool {
	return s == "code-blocks" || s == "tables" || s == "urls"
}

var _ rule.Configurable = (*Rule)(nil)
