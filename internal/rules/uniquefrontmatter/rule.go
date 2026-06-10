// Package uniquefrontmatter implements MDS069: within a configured
// include/exclude glob scope, no two files may carry the same value
// in a named front-matter field. The first holder in ascending path
// order stays clean; every later file gets one diagnostic naming the
// field, the value, and the first holder. With no include globs
// configured the rule reports nothing, so it ships enabled and inert.
package uniquefrontmatter

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/globpath"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/settings"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that every file matching the include globs (minus the
// exclude globs) holds a distinct value in the Field front-matter
// key. Files without the key are skipped.
type Rule struct {
	Field   string
	Include []string
	Exclude []string
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS069" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "unique-frontmatter" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "structural" }

// pathEntry records one in-scope file's field value, the 1-based
// file line of the field, and — when the value repeats an earlier
// path's — that first path. firstPath is empty on the first holder.
type pathEntry struct {
	value     string
	line      int
	firstPath string
}

// scopeIndex maps every in-scope file that carries the field to its
// pathEntry. Built once per run and shared read-only across Check
// goroutines, so lookups stay allocation-free.
type scopeIndex struct {
	byPath map[string]pathEntry
}

// Check implements rule.Rule. The host file is flagged when an
// earlier file in path order already holds its value.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if r.Field == "" || len(r.Include) == 0 {
		return nil
	}
	e, ok := r.index(f).byPath[path.Clean(f.Path)]
	if !ok || e.firstPath == "" {
		return nil
	}
	// e.line is the raw file line of the field; rules emit
	// body-coordinate lines and lint.File.AdjustDiagnostics adds
	// LineOffset back, so subtract it here (the host file is the
	// flagged file, so its offset is the right one).
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     e.line - f.LineOffset,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Message: fmt.Sprintf(
			"front-matter %q: value %s already used by %s",
			r.Field, e.value, e.firstPath),
	}}
}

// index returns the scope index, built at most once per run via the
// RunCache when wired (one build shared by every host file), else
// once per File via the per-File memo (unit tests, struct-literal
// callers). The key encodes the rule's whole scope so two
// differently-configured layers never share an index.
func (r *Rule) index(f *lint.File) *scopeIndex {
	key := strings.Join(append(append(
		[]string{"MDS069", r.Field}, r.Include...), r.Exclude...), "\x00")
	build := func() any { return r.buildIndex(f) }
	var v any
	if f.RunCache != nil {
		v = f.RunCache.UniqueFieldIndex(key, build)
	} else {
		v = f.Memo(key, build)
	}
	return v.(*scopeIndex)
}

// buildIndex enumerates the include globs against the workspace FS
// (RootFS when wired, else the file's FS), drops exclude matches,
// and records each field-bearing file's value in ascending path
// order so "first holder" is deterministic.
func (r *Rule) buildIndex(f *lint.File) *scopeIndex {
	idx := &scopeIndex{byPath: map[string]pathEntry{}}
	fsys := f.RootFS
	if fsys == nil {
		fsys = f.FS
	}
	if fsys == nil {
		return idx
	}

	seen := map[string]struct{}{}
	var paths []string
	for _, pat := range r.Include {
		matches, err := doublestar.Glob(fsys, pat)
		if err != nil {
			// Patterns are validated in ApplySettings; a walk error
			// here means the FS, not the config — skip the pattern.
			continue
		}
		for _, m := range matches {
			m = path.Clean(m)
			if _, dup := seen[m]; dup {
				continue
			}
			if globpath.MatchAny(r.Exclude, m) {
				continue
			}
			seen[m] = struct{}{}
			paths = append(paths, m)
		}
	}
	sort.Strings(paths)

	firstByValue := make(map[string]string, len(paths))
	for _, p := range paths {
		value, line, ok := r.fieldValue(fsys, p, f.MaxInputBytes)
		if !ok {
			continue
		}
		entry := pathEntry{value: value, line: line}
		if first, dup := firstByValue[value]; dup {
			entry.firstPath = first
		} else {
			firstByValue[value] = p
		}
		idx.byPath[p] = entry
	}
	return idx
}

// fieldValue reads p's front matter and returns the field's value as
// a string plus the field's 1-based file line. ok is false when the
// file is unreadable, has no front matter, fails to parse, or lacks
// the field — all of which mean "not a uniqueness participant", not
// an error: this rule owns uniqueness, MDS020 owns well-formedness.
func (r *Rule) fieldValue(
	fsys fs.FS, p string, maxBytes int64,
) (string, int, bool) {
	data, err := bytelimit.ReadFSFileLimited(fsys, p, maxBytes)
	if err != nil {
		return "", 0, false
	}
	prefix, _ := lint.StripFrontMatter(data)
	if prefix == nil {
		return "", 0, false
	}
	fields, err := lint.ParseFrontMatterFields(prefix)
	if err != nil {
		return "", 0, false
	}
	v, ok := fields[r.Field]
	if !ok || v == nil {
		return "", 0, false
	}
	return fmt.Sprintf("%v", v), fieldLine(prefix, r.Field), true
}

// fieldLine returns the 1-based file line of the first front-matter
// line that sets field. The prefix starts at the file's first line
// (the opening ---), so a line index inside the prefix is the file
// line. Falls back to 1 when the textual scan misses (flow-style or
// folded mappings).
func fieldLine(prefix []byte, field string) int {
	line := 1
	for len(prefix) > 0 {
		l := prefix
		if i := bytes.IndexByte(prefix, '\n'); i >= 0 {
			l = prefix[:i]
			prefix = prefix[i+1:]
		} else {
			prefix = nil
		}
		if bytes.HasPrefix(l, []byte(field)) &&
			len(l) > len(field) && l[len(field)] == ':' {
			return line
		}
		line++
	}
	return 1
}

// ApplySettings implements rule.Configurable.
//
// include and exclude are list settings and replace wholesale across
// config layers (rule.MergeReplace, the default): a kind that sets
// include starts from scratch rather than appending to an earlier
// layer's globs. A bool-only layer (unique-frontmatter: false) still
// toggles enabled without erasing these.
func (r *Rule) ApplySettings(s map[string]any) error {
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
				"unique-frontmatter: field must be a string, got %T", v)
		}
		r.Field = fv
		return nil
	case "include":
		return applyList(&r.Include, "include", v)
	case "exclude":
		return applyList(&r.Exclude, "exclude", v)
	}
	return fmt.Errorf("unique-frontmatter: unknown setting %q", key)
}

func applyList(target *[]string, name string, v any) error {
	list, ok := settings.ToStringSlice(v)
	if !ok {
		return fmt.Errorf(
			"unique-frontmatter: %s must be a list of strings, got %T",
			name, v)
	}
	*target = list
	return nil
}

func (r *Rule) validateGlobs() error {
	for _, pat := range r.Include {
		if !doublestar.ValidatePattern(pat) {
			return fmt.Errorf("unique-frontmatter: invalid glob %q", pat)
		}
	}
	for _, pat := range r.Exclude {
		if !doublestar.ValidatePattern(pat) {
			return fmt.Errorf("unique-frontmatter: invalid glob %q", pat)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"field":   r.Field,
		"include": append([]string{}, r.Include...),
		"exclude": append([]string{}, r.Exclude...),
	}
}
