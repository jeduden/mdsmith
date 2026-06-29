// Package requiredfrontmatter implements MDS071: every file in the
// configured include/exclude glob scope must carry the named
// front-matter fields, each present and non-empty. It is the per-file
// companion to MDS069 (unique-frontmatter): MDS069 enforces that a
// field's values are distinct across files, while this rule enforces
// that the field exists and holds a value at all. With no fields
// configured the rule reports nothing, so it ships registered and
// inert until a scope names the required keys.
//
// The canonical use is the Open Knowledge Format (OKF): every concept
// document must declare a non-empty `type`, while the reserved
// `index.md` and `log.md` files are excluded. The built-in `okf`
// convention configures the rule for exactly that case.
package requiredfrontmatter

import (
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/jeduden/mdsmith/internal/globpath"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/jeduden/mdsmith/internal/yamlutil"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that every file matching the include globs (minus the
// exclude globs) declares each field in Fields with a present,
// non-empty value in its top-level YAML front matter.
type Rule struct {
	Fields  []string
	Include []string
	Exclude []string
}

// Compile-time interface checks.
var (
	_ rule.Rule         = (*Rule)(nil)
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
)

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS071" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "required-frontmatter" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "structural" }

// EnabledByDefault implements rule.Defaultable. The rule is opt-in:
// requiring a field is a project decision, so nothing fires until a
// convention, kind, or config names the fields.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule. A file in scope is flagged once per
// required field that is absent, null, or empty.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f == nil || len(r.Fields) == 0 {
		return nil
	}
	p := filepath.ToSlash(f.Path)
	if len(r.Include) > 0 && !globpath.MatchAny(r.Include, p) {
		return nil
	}
	if len(r.Exclude) > 0 && globpath.MatchAny(r.Exclude, p) {
		return nil
	}

	// raw is nil when the file has no parseable front matter; the loop
	// below then reports every required field as missing, which is the
	// intended verdict (a non-reserved file with no front matter fails).
	raw := r.docFrontMatter(f)

	// Front-matter diagnostics anchor to file line 1. Rules emit
	// body-coordinate lines and lint.File.AdjustDiagnostics adds
	// LineOffset back, so subtract it here so the final line is 1 even
	// when the engine stripped front matter into f.FrontMatter.
	line := 1 - f.LineOffset

	var diags []lint.Diagnostic
	for _, field := range r.Fields {
		v, present := raw[field]
		switch {
		case !present:
			diags = append(diags, r.diag(f, line, fmt.Sprintf(
				"front-matter %q is required but missing", field)))
		case isEmptyValue(v):
			diags = append(diags, r.diag(f, line, fmt.Sprintf(
				"front-matter %q is required but empty", field)))
		}
	}
	return diags
}

func (r *Rule) diag(f *lint.File, line int, msg string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     f.Path,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Message:  msg,
	}
}

// docFrontMatter returns the file's parsed top-level front matter, or
// nil when none is available. It prefers f.FrontMatter, which the
// engine populates in production. When that is empty — files built via
// lint.NewFile in unit and fixture tests, or a real file with no front
// matter — it falls back to reading the file from the workspace FS so a
// file's own front matter is still visible. An unreadable path yields
// nil, which Check treats as "every field missing".
func (r *Rule) docFrontMatter(f *lint.File) map[string]any {
	fmBytes := f.FrontMatter
	if len(fmBytes) == 0 && f.FS != nil && f.Path != "" {
		if data, err := fs.ReadFile(f.FS, filepath.ToSlash(f.Path)); err == nil {
			fmBytes, _ = lint.StripFrontMatter(data)
		}
	}
	body := extractYAMLBody(fmBytes)
	if len(body) == 0 {
		return nil
	}
	var raw map[string]any
	if err := yamlutil.UnmarshalSafe(body, &raw); err != nil {
		return nil
	}
	return raw
}

// extractYAMLBody strips the surrounding `---` fences from a stored
// front-matter block and returns the YAML body. It handles both a
// trailing `---\n` fence and a bare `---` (no final newline), mirroring
// how the engine and lint.StripFrontMatter store the prefix. A block
// with no recognisable fences is returned unchanged so a bare YAML
// payload still parses.
func extractYAMLBody(fmBlock []byte) []byte {
	body := bytes.TrimPrefix(fmBlock, []byte("---\n"))
	switch {
	case bytes.HasSuffix(body, []byte("---\n")):
		return body[:len(body)-len("---\n")]
	case bytes.HasSuffix(body, []byte("---")):
		return body[:len(body)-len("---")]
	}
	return body
}

// isEmptyValue reports whether a front-matter value counts as empty:
// a null, a blank or whitespace-only string, or an empty list or map.
// Non-empty scalars of any other type (numbers, booleans) satisfy the
// requirement — the rule checks presence, not a specific type.
func isEmptyValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	default:
		return false
	}
}

// ApplySettings implements rule.Configurable.
//
// fields, include, and exclude are list settings and replace wholesale
// across config layers (rule.MergeReplace, the default). field is a
// convenience alias for a single-element fields list; setting both
// field and fields is a config error. A bool-only layer
// (required-frontmatter: false) toggles enabled without erasing these.
func (r *Rule) ApplySettings(s map[string]any) error {
	_, hasField := s["field"]
	_, hasFields := s["fields"]
	if hasField && hasFields {
		return fmt.Errorf(
			"required-frontmatter: set either field or fields, not both")
	}
	for k, v := range s {
		if err := r.applyOne(k, v); err != nil {
			return err
		}
	}
	return r.validateGlobs()
}

func (r *Rule) applyOne(key string, v any) error {
	switch key {
	case "field":
		fv, ok := v.(string)
		if !ok {
			return fmt.Errorf(
				"required-frontmatter: field must be a string, got %T", v)
		}
		if fv == "" {
			r.Fields = nil
			return nil
		}
		r.Fields = []string{fv}
		return nil
	case "fields":
		return applyList(&r.Fields, "fields", v)
	case "include":
		return applyList(&r.Include, "include", v)
	case "exclude":
		return applyList(&r.Exclude, "exclude", v)
	}
	return fmt.Errorf("required-frontmatter: unknown setting %q", key)
}

func applyList(target *[]string, name string, v any) error {
	list, ok := settings.ToStringSlice(v)
	if !ok {
		return fmt.Errorf(
			"required-frontmatter: %s must be a list of strings, got %T",
			name, v)
	}
	*target = list
	return nil
}

func (r *Rule) validateGlobs() error {
	for _, pats := range [][]string{r.Include, r.Exclude} {
		for _, pat := range pats {
			bare := strings.TrimPrefix(pat, "!")
			if !doublestar.ValidatePattern(bare) {
				return fmt.Errorf(
					"required-frontmatter: invalid glob %q", pat)
			}
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"fields":  []string{},
		"include": []string{},
		"exclude": []string{},
	}
}
