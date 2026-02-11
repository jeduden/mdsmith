package linelength

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
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
	Max     int
	Exclude []string
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
		switch k {
		case "max":
			n, ok := toInt(v)
			if !ok {
				return fmt.Errorf("line-length: max must be an integer, got %T", v)
			}
			r.Max = n
		case "exclude":
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
		case "strict":
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
		default:
			return fmt.Errorf("line-length: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"max":     80,
		"exclude": []string{"code-blocks", "tables", "urls"},
	}
}

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	max := r.Max
	if max <= 0 {
		max = 80
	}

	// Build the set of 1-based line numbers inside code blocks.
	codeLines := map[int]bool{}
	if r.isExcluded("code-blocks") {
		codeLines = lint.CollectCodeBlockLines(f)
	}

	// Build the set of 1-based line numbers that are table rows.
	tableLines := map[int]bool{}
	if r.isExcluded("tables") {
		tableLines = collectTableLines(f)
	}

	var diags []lint.Diagnostic
	for i, line := range f.Lines {
		lineNum := i + 1 // 1-based

		if len(line) <= max {
			continue
		}

		// Skip code-block lines when excluded.
		if r.isExcluded("code-blocks") && codeLines[lineNum] {
			continue
		}

		// Skip table lines when excluded.
		if r.isExcluded("tables") && tableLines[lineNum] {
			continue
		}

		// Skip URL-only lines when excluded.
		if r.isExcluded("urls") && urlOnlyRe.MatchString(strings.TrimSpace(string(line))) {
			continue
		}

		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     lineNum,
			Column:   max + 1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  fmt.Sprintf("line too long (%d > %d)", len(line), max),
		})
	}

	return diags
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
