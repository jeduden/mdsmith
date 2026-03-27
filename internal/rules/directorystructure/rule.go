package directorystructure

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gobwas/glob"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that markdown files exist only in explicitly allowed directories.
type Rule struct {
	Allowed    []string
	configured bool
	matchers   []glob.Glob
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS033" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "directory-structure" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	// When the allowed key was never provided, skip checking.
	// The rule is disabled by default; a user must explicitly configure
	// the allowed list for it to take effect.
	if !r.configured {
		return nil
	}
	if r.isAllowed(f.Path) {
		return nil
	}
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     1,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message: fmt.Sprintf("file %q is not in an allowed directory (allowed: %s)",
			f.Path, formatAllowed(r.Allowed)),
	}}
}

// isAllowed returns true if the file path matches any allowed pattern.
// Patterns are matched against the full file path (e.g., "docs/**" allows
// any file under the docs/ directory).
func (r *Rule) isAllowed(filePath string) bool {
	cleaned := filepath.ToSlash(filepath.Clean(filePath))
	dir := filepath.ToSlash(filepath.Dir(cleaned))
	for i, pattern := range r.Allowed {
		// "." means root-level files only.
		if pattern == "." {
			if dir == "." {
				return true
			}
			continue
		}
		if r.matchers[i].Match(cleaned) {
			return true
		}
	}
	return false
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	// Validate all keys first so we never partially mutate state.
	var patterns []string
	var matchers []glob.Glob
	hasAllowed := false
	for k, v := range settings {
		switch k {
		case "allowed":
			var ok bool
			patterns, ok = toStringSlice(v)
			if !ok {
				return fmt.Errorf("directory-structure: allowed must be a list of strings, got %T", v)
			}
			hasAllowed = true
			matchers = make([]glob.Glob, len(patterns))
			for i, p := range patterns {
				if p == "." {
					continue
				}
				g, err := glob.Compile(p)
				if err != nil {
					return fmt.Errorf("directory-structure: invalid glob pattern %q: %w", p, err)
				}
				matchers[i] = g
			}
		default:
			return fmt.Errorf("directory-structure: unknown setting %q", k)
		}
	}
	// All validation passed — commit state atomically.
	r.Allowed = patterns
	r.matchers = matchers
	r.configured = hasAllowed
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	// No default "allowed" list: by default the rule remains unconfigured/no-op
	return map[string]any{}
}

func formatAllowed(patterns []string) string {
	if len(patterns) == 0 {
		return "(none)"
	}
	return strings.Join(patterns, ", ")
}

func toStringSlice(v any) ([]string, bool) {
	switch values := v.(type) {
	case []string:
		return values, true
	case []any:
		out := make([]string, 0, len(values))
		for _, item := range values {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
)
