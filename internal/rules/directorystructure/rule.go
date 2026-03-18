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
	Allowed []string
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
	// When no allowed patterns are configured, skip checking.
	// The rule is disabled by default; a user must explicitly configure
	// the allowed list for it to take effect.
	if len(r.Allowed) == 0 {
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
			f.Path, strings.Join(r.Allowed, ", ")),
	}}
}

// isAllowed returns true if the file path matches any allowed pattern.
func (r *Rule) isAllowed(filePath string) bool {
	dir := filepath.Dir(filePath)
	for _, pattern := range r.Allowed {
		// "." means root-level files only.
		if pattern == "." {
			if dir == "." {
				return true
			}
			continue
		}
		g, err := glob.Compile(pattern)
		if err != nil {
			continue
		}
		// Match the full path (minus filename) or the full path including filename.
		if g.Match(filePath) || g.Match(dir) {
			return true
		}
	}
	return false
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "allowed":
			list, ok := v.([]any)
			if !ok {
				return fmt.Errorf("directory-structure: allowed must be a list, got %T", v)
			}
			r.Allowed = make([]string, 0, len(list))
			for _, item := range list {
				s, ok := item.(string)
				if !ok {
					return fmt.Errorf("directory-structure: allowed item must be a string, got %T", item)
				}
				r.Allowed = append(r.Allowed, s)
			}
		default:
			return fmt.Errorf("directory-structure: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{"allowed": []string{}}
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
)
