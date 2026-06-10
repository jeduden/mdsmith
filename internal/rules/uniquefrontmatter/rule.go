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
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/jeduden/mdsmith/internal/bytelimit"
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
// exclude globs) holds a distinct value in the Field front-matter
// key. Files without the key are skipped.
type Rule struct {
	Field   string
	Include []string
	Exclude []string

	// scopeKey caches the RunCache key for the configured scope.
	// ApplySettings recomputes it; struct-literal callers (unit
	// tests) leave it empty and index derives it per call.
	scopeKey string
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS069" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "unique-frontmatter" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "structural" }

// pathEntry records one flagged file's field value, the 1-based
// file line of the field, and the first path holding the value.
type pathEntry struct {
	value     string
	line      int
	firstPath string
}

// scopeIndex maps each in-scope file whose value repeats an earlier
// path's to the data its diagnostic needs. Files with unique values
// are not stored — Check treats a missing entry as clean — so the
// index size tracks the number of violations, not the workspace.
// Built once per run and read-only afterwards, so concurrent Check
// goroutines share it without locks.
type scopeIndex struct {
	byPath map[string]pathEntry

	// rootDir and cwd (both absolute, captured at build time)
	// define the key space. With a rootDir, every byPath key is an
	// absolute native path and a relative host resolves against
	// cwd — the engine hands rules CWD-relative host paths, and
	// discovery from a subdirectory yields paths the workspace
	// root never sees, so root-relative keying cannot work.
	// Without a rootDir (fixtures, struct-literal tests) keys stay
	// as cleaned glob output and hosts match by cleaned f.Path.
	// rootDir plus the glob lists also let RunCache.Invalidate
	// decide whether an edited path could change this index.
	rootDir string
	cwd     string
	include []string
	exclude []string
}

// hostKey returns f.Path in the index's key space; see the
// scopeIndex comment. An unresolvable host returns "" and misses
// the index, the fail-open posture MDS027 uses for unmappable
// paths.
func (s *scopeIndex) hostKey(f *lint.File) string {
	p := f.Path
	if s.rootDir == "" {
		return path.Clean(p)
	}
	if !filepath.IsAbs(p) {
		if s.cwd == "" {
			return ""
		}
		p = filepath.Join(s.cwd, p)
	}
	return filepath.Clean(p)
}

// MatchesInvalidatedPath implements lint.ScopeInvalidator. In-root
// paths match by the scope's globs. Paths the matcher cannot relate
// to the root — Rel errors and dot-dot escapes, e.g. a macOS /tmp
// workspace root receiving /private/tmp document paths — return
// true: dropping the index too eagerly costs one rebuild, while
// keeping it stale would freeze the rule's verdicts for the rest
// of the editing session.
func (s *scopeIndex) MatchesInvalidatedPath(absPath string) bool {
	if s.rootDir == "" {
		return true
	}
	rel, err := filepath.Rel(s.rootDir, absPath)
	if err != nil || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return true
	}
	rel = filepath.ToSlash(rel)
	if globpath.MatchAny(s.exclude, rel) {
		return false
	}
	return globpath.MatchAny(s.include, rel)
}

// Check implements rule.Rule. The host file is flagged when an
// earlier file in path order already holds its value.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if r.Field == "" || len(r.Include) == 0 {
		return nil
	}
	idx := r.index(f)
	e, ok := idx.byPath[idx.hostKey(f)]
	if !ok {
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
// RunCache when wired (one build shared by every host file). The
// per-File memo fallback is test-only scaffolding: the engine
// always wires a RunCache, and without one the index rebuilds once
// per host file. The key encodes the rule's whole scope so two
// differently-configured layers never share an index.
func (r *Rule) index(f *lint.File) *scopeIndex {
	key := r.scopeKey
	if key == "" {
		key = scopeKeyFor(r.Field, r.Include, r.Exclude)
	}
	build := func() any { return r.buildIndex(f) }
	var v any
	if f.RunCache != nil {
		v = f.RunCache.UniqueFieldIndex(key, build)
	} else {
		v = f.Memo(key, build)
	}
	return v.(*scopeIndex)
}

// scopeKeyFor derives the cache key for one configured scope.
// ApplySettings interns the result on the rule so configured runs
// pay no per-Check key allocation.
func scopeKeyFor(field string, include, exclude []string) string {
	parts := make([]string, 0, 2+len(include)+len(exclude))
	parts = append(parts, "MDS069", field)
	parts = append(parts, include...)
	parts = append(parts, exclude...)
	return strings.Join(parts, "\x00")
}

// buildIndex enumerates the include globs against the workspace FS
// (RootFS when wired, else the file's FS), drops exclude matches,
// and records duplicate holders in ascending path order so "first
// holder" is deterministic. In the LSP the workspace FS reads
// as-saved disk state, the same view every cross-file rule gets;
// unsaved buffer edits land in the index after save.
func (r *Rule) buildIndex(f *lint.File) *scopeIndex {
	idx := &scopeIndex{
		byPath:  map[string]pathEntry{},
		include: r.Include,
		exclude: r.Exclude,
	}
	fsys := f.RootFS
	if fsys == nil {
		fsys = f.FS
	}
	if fsys == nil {
		return idx
	}
	if f.RootDir != "" {
		if abs, err := filepath.Abs(f.RootDir); err == nil {
			idx.rootDir = abs
		}
		if cwd, err := os.Getwd(); err == nil {
			idx.cwd = cwd
		}
	}

	paths := r.scopePaths(fsys)

	firstByValue := make(map[string]string, len(paths))
	for _, p := range paths {
		value, line, ok := r.fieldValue(fsys, p, f.MaxInputBytes)
		if !ok {
			continue
		}
		key := p
		if idx.rootDir != "" {
			key = filepath.Join(idx.rootDir, filepath.FromSlash(p))
		}
		if first, dup := firstByValue[value]; dup {
			// firstPath stays the workspace-relative form: it is
			// message text, not a key.
			idx.byPath[key] = pathEntry{
				value: value, line: line, firstPath: first,
			}
		} else {
			firstByValue[value] = p
		}
	}
	return idx
}

// scopePaths enumerates the include globs against fsys — skipping
// directories, symlinks, duplicate matches, and exclude matches —
// and returns the survivors in ascending path order so "first
// holder" is deterministic.
func (r *Rule) scopePaths(fsys fs.FS) []string {
	seen := map[string]struct{}{}
	var paths []string
	for _, pat := range r.Include {
		// WithNoFollow keeps the walk out of symlinked directories
		// and reports symlinked files as symlinks, which the type
		// check skips: front matter outside the workspace must not
		// join the uniqueness scope. Discovery denies symlinks the
		// same way (plan 84). One upstream caveat: a symlink in a
		// pattern's literal prefix (before the first meta
		// character) is still followed. Walk errors leave the
		// pattern's partial matches in place — pattern syntax was
		// already validated in ApplySettings.
		_ = doublestar.GlobWalk(fsys, pat,
			func(m string, d fs.DirEntry) error {
				if d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
					return nil
				}
				m = path.Clean(m)
				if _, dup := seen[m]; dup {
					return nil
				}
				if globpath.MatchAny(r.Exclude, m) {
					return nil
				}
				seen[m] = struct{}{}
				paths = append(paths, m)
				return nil
			}, doublestar.WithNoFollow())
	}
	sort.Strings(paths)
	return paths
}

// fieldValue reads p's front matter and returns the field's scalar
// text and 1-based file line. ok is false when the file is
// unreadable, has no front matter, fails to parse, or the field is
// absent, null, or non-scalar — all meaning "not a uniqueness
// participant", not an error: this rule owns uniqueness, MDS020
// owns well-formedness.
//
// The shared FrontMatter RunCache slot is deliberately not used:
// that slot's value type belongs to the catalog rule, and a second
// writer storing a different shape under the same path key would
// poison whichever rule builds second.
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
	delim := []byte("---\n")
	body := bytes.TrimSuffix(bytes.TrimPrefix(prefix, delim), delim)
	doc, err := yamlutil.UnmarshalNodeSafe(body)
	if err != nil {
		return "", 0, false
	}
	// The body starts at file line 2 (line 1 is the opening ---),
	// so the node walk shifts its 1-based body lines by one.
	return yamlutil.TopLevelScalarField(&doc, r.Field, 1)
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
	if err := r.validateGlobs(); err != nil {
		return err
	}
	r.scopeKey = scopeKeyFor(r.Field, r.Include, r.Exclude)
	return nil
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
	for _, pats := range [][]string{r.Include, r.Exclude} {
		for _, pat := range pats {
			if !doublestar.ValidatePattern(pat) {
				return fmt.Errorf(
					"unique-frontmatter: invalid glob %q", pat)
			}
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"field":   "",
		"include": []string{},
		"exclude": []string{},
	}
}
