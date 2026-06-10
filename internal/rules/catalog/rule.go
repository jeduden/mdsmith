package catalog

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/cuetemplate"
	"github.com/jeduden/mdsmith/internal/fieldinterp"
	"github.com/jeduden/mdsmith/internal/gitignore"
	"github.com/jeduden/mdsmith/internal/globpath"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/query"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/jeduden/mdsmith/internal/rules/tablefmt"
	"github.com/jeduden/mdsmith/internal/yamlutil"
)

// numericSortPrefix marks a sort spec whose key value should be
// parsed as an integer before comparison. The prefix follows any
// leading "-" descending marker — `-numeric:id`, not `numeric:-id`.
const numericSortPrefix = "numeric:"

func init() {
	rule.Register(&Rule{Pad: 1, SeparatorStyle: tablefmt.SeparatorSpaced})
}

// Rule checks that generated sections match their directive output.
//
// engineOnce serialises the lazy initialisation of engine: the rule
// is a registered singleton and the LSP server may call Check from
// multiple goroutines, so a plain check-then-set on the engine
// field races. sync.Once gives both writers and readers a single
// happens-before edge.
//
// Pad and SeparatorStyle mirror MDS025 (table-format)'s knobs and
// govern only the tables this rule emits inside `<?catalog?>` bodies.
// Catalog carries its own copies — rather than reading MDS025's
// configured state — because the lint engine clones rules and applies
// settings per file in parallel; a process-global view of MDS025
// would race across workers. Set both rules to the same style when
// you want host-file tables and catalog-generated tables to share a
// canonical.
type Rule struct {
	engineOnce     sync.Once
	engine         *gensection.Engine
	Pad            int
	SeparatorStyle tablefmt.SeparatorStyle
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS019" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "catalog" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "directive" }

// RuleID implements gensection.Directive.
func (r *Rule) RuleID() string { return "MDS019" }

// RuleName implements gensection.Directive.
func (r *Rule) RuleName() string { return "catalog" }

// getEngine lazily initializes and returns the gensection engine.
func (r *Rule) getEngine() *gensection.Engine {
	r.engineOnce.Do(func() {
		r.engine = gensection.NewEngine(r)
	})
	return r.engine
}

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f.FS == nil {
		return nil
	}
	diags := r.getEngine().Check(f)
	// Case-mismatch hints run a separate pass over directives. This
	// re-reads front-matter but avoids coupling hints to the engine's
	// fatal-diagnostic pipeline. Acceptable for typical catalog sizes.
	diags = append(diags, r.checkCaseMismatches(f)...)
	// Injection warnings are non-fatal and must not block generation,
	// so they run as a separate pass outside the engine.
	diags = append(diags, r.checkInjection(f)...)
	return diags
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	if f.FS == nil {
		return f.Source
	}
	return r.getEngine().Fix(f)
}

// Validate implements gensection.Directive.
func (r *Rule) Validate(filePath string, line int,
	params map[string]string, columns map[string]gensection.ColumnConfig,
) []lint.Diagnostic {
	return validateCatalogDirective(filePath, line, params, columns)
}

// Generate implements gensection.Directive.
func (r *Rule) Generate(f *lint.File, filePath string, line int,
	params map[string]string, columns map[string]gensection.ColumnConfig,
) (string, []lint.Diagnostic) {
	cols := fromGensectionColumns(columns)
	// Read errors (e.g. "file too large") are fatal for generation:
	// a partially-rendered catalog would silently hide missing rows,
	// which is worse than failing loudly with a clear diagnostic.
	entries, res, entryDiags := cachedCatalogEntries(f, params, filePath, line)
	if len(entryDiags) > 0 {
		return "", entryDiags
	}

	// Check if any matched file includes (directly or indirectly) the
	// catalog-owning file. If so, the catalog body would contain itself.
	if diags := checkCatalogIncludeCycle(f, filePath, line, entries, res); len(diags) > 0 {
		return "", diags
	}

	hasRow := hasRowTemplate(params)
	content, err := renderCatalogContent(params, entries, cols, hasRow)
	if err != nil {
		return "", []lint.Diagnostic{makeDiag(filePath, line,
			fmt.Sprintf("generated section template execution failed: %v", err))}
	}

	// Defensive guard: refuse to silently empty a previously-populated
	// catalog when the glob matched zero files and no `empty:` fallback
	// applies. Emptying a non-empty section in that case is almost always
	// a misconfiguration (a wrong working directory, a stale glob, or an
	// ignore rule that swept up the whole tree) rather than an intentional
	// edit, so destroying the rows would discard real content. Returning a
	// diagnostic both surfaces the problem in `check` and, via the engine's
	// generation-error path, leaves the body untouched in `fix`. An
	// already-empty body is left alone (no rows to protect); an `empty:`
	// fallback renders non-empty content and so never reaches this branch.
	if len(entries) == 0 && content == "" && sectionBodyNonEmpty(f, line) {
		return "", []lint.Diagnostic{makeDiag(filePath, line,
			"catalog glob matched zero files but the section is non-empty; "+
				"refusing to empty it — check the working directory, the glob, "+
				"and ignore rules, or set an \"empty:\" fallback to clear it intentionally")}
	}

	// Format tables this rule generates using its own pad / separator
	// settings; see Rule's doc comment for why catalog carries its own
	// table-format knobs instead of consulting MDS025.
	content = tablefmt.FormatStringWithConfig(content, tablefmt.Config{
		Pad:            r.Pad,
		SeparatorStyle: r.SeparatorStyle,
	})

	return content, nil
}

// sectionBodyNonEmpty reports whether the catalog section whose start
// marker is on the given 1-based line currently has any non-whitespace
// body content. It powers the destructive-empty guard in Generate: a
// body of only blank lines (or no body at all) carries nothing worth
// protecting, so the guard must not fire for it. Returns false when the
// marker pair cannot be located, matching the conservative "nothing to
// protect" default.
func sectionBodyNonEmpty(f *lint.File, line int) bool {
	pairs, _ := gensection.FindMarkerPairs(f, "catalog", "MDS019", "catalog")
	for _, mp := range pairs {
		if mp.StartLine != line {
			continue
		}
		return strings.TrimSpace(gensection.ExtractContent(f, mp)) != ""
	}
	return false
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "pad":
			n, ok := settings.ToInt(v)
			if !ok {
				return fmt.Errorf("catalog: pad must be an integer, got %T", v)
			}
			if n < 0 {
				return fmt.Errorf("catalog: pad must be non-negative, got %d", n)
			}
			r.Pad = n
		case "separator-style":
			style, err := tablefmt.ParseSeparatorStyle(v, "catalog")
			if err != nil {
				return err
			}
			r.SeparatorStyle = style
		default:
			return fmt.Errorf("catalog: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"pad":             1,
		"separator-style": "spaced",
	}
}

// hasRowTemplate reports whether the directive declared any
// per-file row template — either the placeholder-style `row:`
// or the CUE-expression `row-expr:`. Most catalog paths gate
// "do we render per-file" on this question.
func hasRowTemplate(params map[string]string) bool {
	_, hasRow := params["row"]
	_, hasRowExpr := params["row-expr"]
	return hasRow || hasRowExpr
}

// validateCatalogDirective validates parameters specific to the catalog directive.
func validateCatalogDirective(
	filePath string, line int,
	params map[string]string,
	columns map[string]gensection.ColumnConfig,
) []lint.Diagnostic {
	if diags := validateRowParams(filePath, line, params); len(diags) > 0 {
		return diags
	}
	if diags := validateGlob(filePath, line, params); len(diags) > 0 {
		return diags
	}
	if gitignore, ok := params["gitignore"]; ok {
		if gitignore != "true" && gitignore != "false" {
			return []lint.Diagnostic{makeDiag(filePath, line,
				`generated section directive has invalid "gitignore" value; must be "true" or "false"`)}
		}
	}

	var diags []lint.Diagnostic
	if sortVal, hasSort := params["sort"]; hasSort {
		diags = append(diags, validateSort(filePath, line, sortVal)...)
	}
	diags = append(diags, validateRowExpressions(filePath, line, params)...)
	if whereExpr := strings.TrimSpace(params["where"]); whereExpr != "" {
		if _, err := query.Compile(whereExpr); err != nil {
			diags = append(diags, makeDiag(filePath, line,
				fmt.Sprintf(`generated section directive has invalid "where" expression: %v`, err)))
		}
	}
	return diags
}

// validateRowParams checks the presence/exclusivity/emptiness
// rules for the `row:` and `row-expr:` parameters and the
// header/footer dependency on having some row form declared.
// A non-empty result is a hard failure that short-circuits
// later validation, matching the previous inline behaviour.
func validateRowParams(
	filePath string, line int, params map[string]string,
) []lint.Diagnostic {
	_, hasRow := params["row"]
	_, hasRowExpr := params["row-expr"]
	_, hasHeader := params["header"]
	_, hasFooter := params["footer"]

	if hasRow && hasRowExpr {
		return []lint.Diagnostic{makeDiag(filePath, line,
			`generated section directive sets both "row" and "row-expr"; choose one`)}
	}
	if (hasHeader || hasFooter) && !hasRow && !hasRowExpr {
		return []lint.Diagnostic{makeDiag(filePath, line,
			`generated section template missing required "row" or "row-expr" key`)}
	}
	if hasRow && strings.TrimSpace(params["row"]) == "" {
		return []lint.Diagnostic{makeDiag(filePath, line,
			`generated section directive has empty "row" value`)}
	}
	if hasRowExpr && strings.TrimSpace(params["row-expr"]) == "" {
		return []lint.Diagnostic{makeDiag(filePath, line,
			`generated section directive has empty "row-expr" value`)}
	}
	return nil
}

// validateRowExpressions returns diagnostics for malformed
// `row:` placeholder templates and `row-expr:` CUE
// expressions. Presence/exclusivity is already enforced by
// validateRowParams, so at most one of the two paths runs
// per directive.
func validateRowExpressions(
	filePath string, line int, params map[string]string,
) []lint.Diagnostic {
	var diags []lint.Diagnostic
	if row, hasRow := params["row"]; hasRow {
		if err := parseRowTemplate(row); err != nil {
			diags = append(diags, makeDiag(filePath, line,
				fmt.Sprintf("generated section has invalid template: %v", err)))
		}
	}
	if rowExpr, hasRowExpr := params["row-expr"]; hasRowExpr {
		if _, err := cuetemplate.Compile(
			strings.TrimSpace(rowExpr)); err != nil {
			diags = append(diags, makeDiag(filePath, line,
				fmt.Sprintf(
					`generated section directive has invalid "row-expr" expression: %v`,
					err)))
		}
	}
	return diags
}

// splitGlobs splits a possibly newline-joined glob parameter into individual
// patterns. A single-string glob returns a one-element slice.
func splitGlobs(glob string) []string {
	return strings.Split(glob, "\n")
}

// validateGlob validates the glob parameter and returns diagnostics on failure.
// The glob value may be a single pattern or multiple newline-joined patterns
// (from a YAML list). Patterns prefixed with "!" are exclusion patterns.
//
// Path-traversal escapes and missing-root errors for ".." patterns are
// checked at generation time, where the project root is available; see
// resolveGlobFS.
func validateGlob(filePath string, line int, params map[string]string) []lint.Diagnostic {
	glob, hasGlob := params["glob"]
	if !hasGlob {
		return []lint.Diagnostic{makeDiag(filePath, line,
			`generated section directive missing required "glob" parameter`)}
	}
	hasInclude := false
	for _, raw := range splitGlobs(glob) {
		pattern := raw
		isExclude := strings.HasPrefix(pattern, "!")
		if isExclude {
			pattern = pattern[1:]
		}
		if pattern == "" {
			return []lint.Diagnostic{makeDiag(filePath, line,
				`generated section directive has empty "glob" parameter`)}
		}
		if filepath.IsAbs(pattern) {
			return []lint.Diagnostic{makeDiag(filePath, line,
				"generated section directive has absolute glob path")}
		}
		if !doublestar.ValidatePattern(pattern) {
			return []lint.Diagnostic{makeDiag(filePath, line,
				fmt.Sprintf("generated section directive has invalid glob pattern: %s", pattern))}
		}
		if containsDotDotInsideBraces(pattern) {
			// path.Clean does not expand `{a,b}` alternatives, so a
			// `..` segment inside braces would silently bypass the
			// project-root containment check at resolve time.
			return []lint.Diagnostic{makeDiag(filePath, line,
				`generated section directive has ".." inside brace expansion; rewrite as separate patterns`)}
		}
		if !isExclude {
			hasInclude = true
		}
	}
	if !hasInclude {
		return []lint.Diagnostic{makeDiag(filePath, line,
			`generated section directive "glob" parameter must include at least one non-negated pattern`)}
	}
	return nil
}

// validateSort validates the sort value and returns diagnostics.
func validateSort(filePath string, line int, sortVal string) []lint.Diagnostic {
	if sortVal == "" {
		return []lint.Diagnostic{makeDiag(filePath, line,
			`generated section directive has empty "sort" value`)}
	}
	key := strings.TrimPrefix(sortVal, "-")
	key = strings.TrimPrefix(key, numericSortPrefix)
	if key == "" {
		return []lint.Diagnostic{makeDiag(filePath, line,
			fmt.Sprintf("generated section directive has invalid sort value %q", sortVal))}
	}
	// Built-in sort keys don't need CUE path validation.
	if key == "path" || key == "filename" {
		return nil
	}
	// Front-matter sort keys must be valid CUE paths.
	if fieldinterp.ParseCUEPath(key) == nil {
		return []lint.Diagnostic{makeDiag(filePath, line,
			fmt.Sprintf("generated section directive has invalid sort key %q; "+
				"non-identifier keys must be quoted, e.g. sort: '\"my-key\"'", key))}
	}
	return nil
}

// globResolution describes how a catalog directive's glob is rooted.
// It tells the caller which fs.FS to glob against, how to convert matched
// paths back into display paths for the catalog-owning file, the resolved
// include/exclude pattern lists, and where to anchor gitignore lookups.
type globResolution struct {
	fs            fs.FS
	includes      []string
	excludes      []string
	gitignoreBase string // absolute directory matched paths are relative to; "" disables gitignore filtering
	fileDir       string // catalog-owning file's directory, slash-separated, project-root-relative; "" means at root
	rootRelative  bool   // when true, matches are root-relative and need filepath.Rel for display
	diags         []lint.Diagnostic
}

// displayPath converts a doublestar match into a path relative to the
// catalog-owning file's directory.
func (r globResolution) displayPath(match string) string {
	if !r.rootRelative {
		return match
	}
	base := r.fileDir
	if base == "" {
		base = "."
	}
	rel, err := filepath.Rel(base, match)
	if err != nil {
		return match
	}
	return filepath.ToSlash(rel)
}

// resolveGlobFS resolves the catalog directive's glob scope. When the
// patterns contain ".." segments or when source-dir is set, globs resolve
// against the project root via RootFS; otherwise the catalog-owning file's
// own fs.FS is used. Patterns are rewritten to root-relative form when
// switching to RootFS; an "escapes project root" diagnostic is returned
// when any pattern would resolve outside the root.
func resolveGlobFS(f *lint.File, params map[string]string, filePath string, line int) globResolution {
	rawPatterns := splitGlobs(params["glob"])
	includes, excludes := globpath.SplitIncludeExclude(rawPatterns)
	sourceDir := params["source-dir"]
	hasDotDot := dotDotInPatterns(rawPatterns)

	if sourceDir == "" && !hasDotDot {
		return localFSResolution(f, includes, excludes)
	}
	if f.RootFS == nil {
		if hasDotDot {
			return missingRootDiag(filePath, line)
		}
		return localFSResolution(f, includes, excludes)
	}
	return resolveAgainstProjectRoot(f, sourceDir, hasDotDot, includes, excludes, filePath, line)
}

func missingRootDiag(filePath string, line int) globResolution {
	return globResolution{diags: []lint.Diagnostic{makeDiag(
		filePath, line,
		`generated section directive glob contains ".." but project root is not configured`)}}
}

// outsideRootDiag reports a ".." pattern in a file whose path cannot be
// related to the configured project root (e.g. it lives on a different
// volume, or above the configured RootDir). Distinct from
// missingRootDiag so the user can tell which situation they're in.
func outsideRootDiag(filePath string, line int) globResolution {
	return globResolution{diags: []lint.Diagnostic{makeDiag(
		filePath, line,
		`generated section directive catalog file is outside project root; ".." globs cannot be resolved`)}}
}

// resolveAgainstProjectRoot rewrites include/exclude patterns relative
// to the project root using RootFS. When the source-dir is invalid or
// fileDir cannot be related to the project root, it falls back to the
// file's fs.FS — except for ".." patterns, which still need a project
// root and surface the outside-root diagnostic instead of silently
// matching nothing on a fs.FS that rejects "..".
func resolveAgainstProjectRoot(
	f *lint.File, sourceDir string, hasDotDot bool,
	includes, excludes []string,
	filePath string, line int,
) globResolution {
	fileDir, ok := projectRelFileDir(f)
	if !ok {
		if hasDotDot {
			return outsideRootDiag(filePath, line)
		}
		return localFSResolution(f, includes, excludes)
	}
	baseRel, ok := resolveBaseRel(fileDir, sourceDir)
	if !ok {
		if hasDotDot {
			// The pattern needs root-aware resolution for its ".."
			// segments; an invalid source-dir is ignored so we still
			// catch escapes-root and report them.
			baseRel = fileDir
		} else {
			return localFSResolution(f, includes, excludes)
		}
	}
	resolvedIncludes, ok := resolvePatterns(baseRel, includes)
	if !ok {
		return escapeDiag(filePath, line)
	}
	resolvedExcludes, ok := resolvePatterns(baseRel, excludes)
	if !ok {
		return escapeDiag(filePath, line)
	}
	gitignoreBase := ""
	if f.RootDir != "" {
		if abs, err := filepath.Abs(f.RootDir); err == nil {
			gitignoreBase = abs
		}
	}
	return globResolution{
		fs:            f.RootFS,
		includes:      resolvedIncludes,
		excludes:      resolvedExcludes,
		gitignoreBase: gitignoreBase,
		fileDir:       fileDir,
		rootRelative:  true,
	}
}

// projectRelFileDir returns the catalog-owning file's directory as a
// slash-separated path relative to the project root, or ok=false when
// no relation can be computed (e.g. an absolute file path with no
// configured RootDir, or a file outside the configured root).
//
// The Runner passes file paths through verbatim from the command line,
// so a relative f.Path may be CWD-relative rather than root-relative
// (e.g. running `mdsmith check index.md` from a subdirectory). When
// RootDir is set, the file path is absolutized first so both absolute
// and any flavor of relative input land on the same root-relative
// string. Without RootDir, the path is assumed to already be
// root-relative.
//
// Returns "" for the project root itself.
func projectRelFileDir(f *lint.File) (string, bool) {
	if f.RootDir == "" {
		cleaned := path.Clean(filepath.ToSlash(filepath.Dir(f.Path)))
		if cleaned == "." {
			return "", true
		}
		if filepath.IsAbs(cleaned) {
			return "", false
		}
		return cleaned, true
	}
	rootAbs, err := filepath.Abs(f.RootDir)
	if err != nil {
		return "", false
	}
	fileAbs := f.Path
	if !filepath.IsAbs(fileAbs) {
		fileAbs, err = filepath.Abs(fileAbs)
		if err != nil {
			return "", false
		}
	}
	rel, err := filepath.Rel(rootAbs, filepath.Dir(fileAbs))
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return "", true
	}
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

func escapeDiag(filePath string, line int) globResolution {
	return globResolution{diags: []lint.Diagnostic{makeDiag(
		filePath, line,
		"generated section directive glob escapes project root")}}
}

// containsDotDotInsideBraces reports whether p has a ".." segment inside
// a `{a,b}` brace alternative. doublestar expands braces lazily during
// matching, but path.Clean treats `{..,foo}` as a single opaque segment,
// so such a `..` would slip past the project-root containment check.
// Detecting it lets the validator reject the pattern up front instead of
// silently producing partial matches.
func containsDotDotInsideBraces(p string) bool {
	depth := 0
	n := len(p)
	for i := 0; i < n; i++ {
		switch p[i] {
		case '{':
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		case '.':
			if depth == 0 || i+1 >= n || p[i+1] != '.' {
				continue
			}
			if !braceSegmentBoundary(p, i-1) || !braceSegmentBoundary(p, i+2) {
				continue
			}
			return true
		}
	}
	return false
}

// braceSegmentBoundary reports whether p[i] is at or past a delimiter
// that separates path segments inside a brace expansion: `/`, `,`, `{`,
// or `}`. Out-of-range positions are treated as boundaries so a `..`
// at the very edge of a brace block still matches.
func braceSegmentBoundary(p string, i int) bool {
	if i < 0 || i >= len(p) {
		return true
	}
	c := p[i]
	return c == '/' || c == ',' || c == '{' || c == '}'
}

// dotDotInPatterns reports whether any pattern contains a ".." segment.
func dotDotInPatterns(patterns []string) bool {
	for _, p := range patterns {
		if globpath.ContainsDotDotSegment(strings.TrimPrefix(p, "!")) {
			return true
		}
	}
	return false
}

// localFSResolution builds the fast-path resolution that globs from the
// catalog-owning file's own fs.FS. Used when no source-dir is set and the
// patterns contain no ".." segments, or as a fallback when the project
// root is not configured.
//
// gitignoreBase is left empty when absBaseDir cannot derive an absolute
// path. A relative "" or "." base would otherwise leak into
// absMatchedPath as a non-absolute cache key, breaking the LSP
// invalidation contract (which keys by absolute doc.path). Empty base
// opts the resolution out of both run-cache use and gitignore
// anchoring, matching the legacy pre-cache behavior.
func localFSResolution(f *lint.File, includes, excludes []string) globResolution {
	base := ""
	if abs, ok := absBaseDir(f); ok {
		base = abs
	}
	return globResolution{
		fs:            f.FS,
		includes:      includes,
		excludes:      excludes,
		gitignoreBase: base,
	}
}

// absBaseDir returns the absolute filesystem directory f.FS is rooted
// at — i.e. the absolute equivalent of "where doublestar matches
// resolve from for this file". Three resolution strategies, picked in
// order:
//
//  1. f.Path is already absolute → use filepath.Dir(f.Path) directly.
//  2. f.RootDir is set (the LSP path: doc paths arrive workspace-
//     relative but the workspace root is configured, and f.FS is
//     wired from the document's absolute directory) → anchor at
//     RootDir + Dir(f.Path) so cache keys align with the absolute
//     doc.path the LSP's Server.invalidateCachedRead uses.
//  3. Otherwise → filepath.Abs(Dir(f.Path)), which falls back to the
//     process CWD (matches the legacy CLI behavior).
//
// Without strategy 2 the LSP would key cache entries by /cwd/X.md
// while invalidating by /workspace/X.md, so a didChange / didSave on
// a target would silently miss the cached entry and stale catalog
// bodies would persist.
func absBaseDir(f *lint.File) (string, bool) {
	dir := filepath.Dir(f.Path)
	if filepath.IsAbs(dir) {
		return filepath.Clean(dir), true
	}
	if f.RootDir != "" {
		root, err := filepath.Abs(f.RootDir)
		if err == nil {
			return filepath.Clean(filepath.Join(root, dir)), true
		}
	}
	abs, err := filepath.Abs(dir)
	return filepath.Clean(abs), err == nil
}

// resolveBaseRel returns the project-root-relative directory glob patterns
// resolve against. When sourceDir is set it overrides the file's own
// directory; an absolute or escaping sourceDir signals failure (ok=false).
func resolveBaseRel(fileDir, sourceDir string) (string, bool) {
	if sourceDir == "" {
		return fileDir, true
	}
	sd := path.Clean(sourceDir)
	if sd == "." {
		sd = ""
	}
	if sd == ".." || strings.HasPrefix(sd, "../") || filepath.IsAbs(sd) {
		return "", false
	}
	return sd, true
}

// resolvePatterns rewrites each pattern relative to baseRel; ok is false
// when any pattern would resolve outside the project root.
func resolvePatterns(baseRel string, patterns []string) ([]string, bool) {
	resolved := make([]string, 0, len(patterns))
	for _, p := range patterns {
		r, escapes := globpath.ResolveAgainstRoot(baseRel, p)
		if escapes {
			return nil, false
		}
		resolved = append(resolved, r)
	}
	return resolved, true
}

// catalogEntries holds one buildCatalogEntries result so f.Memo can
// cache the (entries, resolution, diags) triple behind a single key.
// The resolution flows through to the include-cycle scan: catalogs
// resolved via RootFS need the scan to walk through f.RootFS with
// root-relative paths so cross-directory cycles are still detected.
type catalogEntries struct {
	entries []fileEntry
	res     globResolution
	diags   []lint.Diagnostic
}

// cachedCatalogEntries returns buildCatalogEntries' result, computed
// once per directive per Check. A directive is identified by its file
// and start line, which the generate, injection, and case-mismatch
// passes all pass identically for the same marker pair, so without
// this memo every matched file's glob + front-matter read ran three
// times per directive. The result is read-only for every caller
// (entries are already sorted by buildCatalogEntries). The memo lives
// on the per-Check *lint.File, so nothing is cached across files or
// runs.
func cachedCatalogEntries(
	f *lint.File, params map[string]string, filePath string, line int,
) ([]fileEntry, globResolution, []lint.Diagnostic) {
	key := "catalog.entries:" + filePath + "#" + strconv.Itoa(line)
	v := f.Memo(key, func() any {
		e, res, d := buildCatalogEntries(f, params, filePath, line)
		return catalogEntries{entries: e, res: res, diags: d}
	})
	r := v.(catalogEntries)
	return r.entries, r.res, r.diags
}

// buildCatalogEntries resolves glob matches, reads front matter, and
// returns sorted file entries for the catalog directive. Read errors
// (notably "file too large") are returned as diagnostics attached to
// the directive's file+line. Callers in the Generate path treat any
// returned diagnostic as fatal to avoid producing an incomplete catalog;
// check-only callers (checkInjection, checkCaseMismatches) discard the
// diagnostics because Generate already surfaces them. Callers reached
// during Check go through cachedCatalogEntries so the three passes do
// not each rebuild the same directive's entries.
func buildCatalogEntries(
	f *lint.File, params map[string]string, filePath string, line int,
) ([]fileEntry, globResolution, []lint.Diagnostic) {
	res := resolveGlobFS(f, params, filePath, line)
	if len(res.diags) > 0 {
		return nil, res, res.diags
	}
	files := resolveGlobMatchesFrom(res, f, params)

	sortKey, descending, numeric := parseSort(params)
	hasRow := hasRowTemplate(params)
	whereExpr := strings.TrimSpace(params["where"])
	needFM := hasRow || whereExpr != "" || (sortKey != "path" && sortKey != "filename")

	var matcher *query.Matcher
	if whereExpr != "" {
		m, err := query.Compile(whereExpr)
		if err != nil {
			// Validate already reports this; skip filtering rather than
			// silently drop every file when the expression is broken.
			matcher = nil
		} else {
			matcher = m
		}
	}

	var diags []lint.Diagnostic
	entries := make([]fileEntry, 0, len(files))
	for _, p := range files {
		displayPath := res.displayPath(p)
		fields := map[string]any{"filename": displayPath}
		var fm map[string]any
		if needFM {
			var err error
			absPath, _ := absMatchedPath(res, p)
			fm, err = cachedFrontMatter(f, res.fs, p, absPath, f.MaxInputBytes)
			if err != nil {
				diags = append(diags, makeDiag(filePath, line,
					fmt.Sprintf("cannot read front matter from %q: %v", displayPath, err)))
				continue
			}
			for k, v := range fm {
				fields[k] = v
			}
		}
		if matcher != nil && !matcher.Match(fm) {
			continue
		}
		entries = append(entries, fileEntry{fields: fields, matchPath: p})
	}

	sortEntries(entries, sortKey, descending, numeric)
	return entries, res, diags
}

// resolveGlobMatchesFrom expands include patterns using the resolved
// fs.FS, filters out exclude and gitignore matches, and returns
// deduplicated file paths.
func resolveGlobMatchesFrom(res globResolution, f *lint.File, params map[string]string) []string {
	matcher := resolveGitignoreMatcher(f, params)
	base := res.gitignoreBase
	if matcher == nil {
		base = ""
	}

	seen := make(map[string]bool)
	var files []string
	for _, pattern := range res.includes {
		matches, err := doublestar.Glob(res.fs, pattern)
		if err != nil {
			continue
		}
		for _, m := range matches {
			if seen[m] {
				continue
			}
			info, err := fs.Stat(res.fs, m)
			if err != nil || info.IsDir() {
				continue
			}
			if isExcluded(m, res.excludes) {
				continue
			}
			if matcher != nil && base != "" && isGitignored(matcher, base, m) {
				continue
			}
			seen[m] = true
			files = append(files, m)
		}
	}
	return files
}

// resolveGitignoreMatcher returns the gitignore matcher to use for
// filtering, or nil when gitignore filtering is disabled or no matcher
// is available. The absolute base directory matched paths are anchored
// to is supplied separately by resolveGlobFS as part of globResolution.
func resolveGitignoreMatcher(f *lint.File, params map[string]string) *gitignore.Matcher {
	if params["gitignore"] == "false" {
		return nil
	}
	return f.GetGitignore()
}

// checkCatalogInjection warns when interpolated front-matter values contain
// embedded newlines or "](" sequences that could inject Markdown
// structure into the generated catalog section.
func checkCatalogInjection(filePath string, line int, entries []fileEntry) []lint.Diagnostic {
	var diags []lint.Diagnostic
	for _, entry := range entries {
		entryPath := fieldinterp.Stringify(entry.fields["filename"])
		// Iterate keys in sorted order for deterministic diagnostic ordering.
		keys := make([]string, 0, len(entry.fields))
		for k := range entry.fields {
			if k != "filename" {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			val := entry.fields[key]
			s := fieldinterp.Stringify(val)
			if strings.ContainsAny(s, "\n\r") {
				diags = append(diags, lint.Diagnostic{
					File:     filePath,
					Line:     line,
					Column:   1,
					RuleID:   "MDS019",
					RuleName: "catalog",
					Severity: lint.Warning,
					Message: fmt.Sprintf(
						"front-matter field %q in %q contains embedded newlines; "+
							"this may inject unexpected Markdown into the catalog",
						key, entryPath),
				})
			}
			if strings.Contains(s, "](") {
				diags = append(diags, lint.Diagnostic{
					File:     filePath,
					Line:     line,
					Column:   1,
					RuleID:   "MDS019",
					RuleName: "catalog",
					Severity: lint.Warning,
					Message: fmt.Sprintf(
						"front-matter field %q in %q contains \"](\" sequence; "+
							"this may inject a Markdown link into the catalog",
						key, entryPath),
				})
			}
		}
	}
	return diags
}

// checkInjection scans catalog directives for front-matter values that could
// inject Markdown structure. Runs independently of Generate so warnings don't
// block content generation.
func (r *Rule) checkInjection(f *lint.File) []lint.Diagnostic {
	pairs, _ := gensection.FindMarkerPairs(
		f, r.Name(), r.ID(), r.Name(),
	)
	var diags []lint.Diagnostic
	for _, mp := range pairs {
		dir, parseDiags := gensection.ParseDirective(
			f.Path, mp, r.ID(), r.Name(),
		)
		if dir == nil || len(parseDiags) > 0 {
			continue
		}
		// Ignore entry diagnostics here; Generate surfaces them.
		entries, _, _ := cachedCatalogEntries(f, dir.Params, f.Path, mp.StartLine)
		diags = append(diags, checkCatalogInjection(f.Path, mp.StartLine, entries)...)
	}
	return diags
}

// renderCatalogContent renders catalog entries into the final content string.
func renderCatalogContent(
	params map[string]string, entries []fileEntry,
	cols map[string]columnConfig, hasRow bool,
) (string, error) {
	if len(entries) == 0 {
		return renderEmpty(params), nil
	}
	if !hasRow {
		return renderMinimal(entries), nil
	}
	return renderTemplate(params, entries, cols)
}

// parseSort parses the sort value from params, returning the key,
// direction, and whether the value should be compared numerically.
// The `numeric:` prefix opts into integer comparison and may follow
// the descending `-` marker, e.g. `-numeric:id`.
func parseSort(params map[string]string) (key string, descending, numeric bool) {
	sortVal, ok := params["sort"]
	if !ok || sortVal == "" {
		return "path", false, false
	}

	if strings.HasPrefix(sortVal, "-") {
		descending = true
		sortVal = sortVal[1:]
	}
	if strings.HasPrefix(sortVal, numericSortPrefix) {
		numeric = true
		sortVal = sortVal[len(numericSortPrefix):]
	}
	return sortVal, descending, numeric
}

// sortEntries sorts file entries by the given key. When numeric is
// true and every entry's value parses as an int, entries are ordered
// by the integer value; any parse failure falls back to string
// compare for the whole sort so behavior stays predictable when one
// entry's field is missing or malformed.
func sortEntries(entries []fileEntry, key string, descending, numeric bool) {
	useInts := numeric && allParseAsInt(entries, key)

	// Pre-compute lowercase keys once per entry to avoid O(n log n)
	// string allocations inside the comparator.
	type sortable struct {
		entry         fileEntry
		sortKey       string // lowercase primary sort key (empty for numeric mode)
		tiebreakerKey string // lowercase filename for stable tiebreaker
	}
	sortables := make([]sortable, len(entries))
	for i, e := range entries {
		var sk string
		if !useInts {
			sk = strings.ToLower(sortValue(e, key))
		}
		sortables[i] = sortable{
			entry:         e,
			sortKey:       sk,
			tiebreakerKey: strings.ToLower(fieldinterp.Stringify(e.fields["filename"])),
		}
	}

	sort.SliceStable(sortables, func(i, j int) bool {
		var cmp int
		if useInts {
			// Re-parse from the moved entry — SliceStable reorders the
			// slice during sort, so sortables[i].entry is always current.
			ni, _ := parseSortInt(sortables[i].entry, key)
			nj, _ := parseSortInt(sortables[j].entry, key)
			switch {
			case ni < nj:
				cmp = -1
			case ni > nj:
				cmp = 1
			}
		} else {
			cmp = strings.Compare(sortables[i].sortKey, sortables[j].sortKey)
		}
		if cmp == 0 {
			// Tiebreaker: path ascending, case-insensitive.
			return sortables[i].tiebreakerKey < sortables[j].tiebreakerKey
		}

		if descending {
			return cmp > 0
		}
		return cmp < 0
	})

	for i, s := range sortables {
		entries[i] = s.entry
	}
}

// allParseAsInt reports whether every entry's value for key parses
// as an integer. Used to decide whether numeric mode applies before
// sorting begins.
func allParseAsInt(entries []fileEntry, key string) bool {
	for _, e := range entries {
		if _, err := parseSortInt(e, key); err != nil {
			return false
		}
	}
	return true
}

// parseSortInt extracts the entry's sort key as a trimmed string
// and parses it via strconv.Atoi.
func parseSortInt(entry fileEntry, key string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(sortValue(entry, key)))
}

// sortValue returns the sort value for a file entry given a key.
func sortValue(entry fileEntry, key string) string {
	switch key {
	case "path":
		return fieldinterp.Stringify(entry.fields["filename"])
	case "filename":
		return filepath.Base(fieldinterp.Stringify(entry.fields["filename"]))
	default:
		path := fieldinterp.ParseCUEPath(key)
		if path == nil {
			return "" // validated at directive parse time
		}
		val, err := fieldinterp.ResolvePath(entry.fields, path)
		if err != nil {
			return ""
		}
		return val
	}
}

// frontMatterResult holds one readFrontMatter result so f.Memo can
// cache the (map, error) pair behind a single key.
type frontMatterResult struct {
	fm  map[string]any
	err error
}

// cachedFrontMatter returns readFrontMatter's result, computed once
// per matched path per Check, and once per absPath per Run when
// f.RunCache is wired (a target globbed by N host catalogs is then
// read once total, not N times). Several catalog directives in one
// file can also glob overlapping sets — CLAUDE.md carries three over
// docs/** — so the per-Check memo collapses those too. absPath is the
// absolute filesystem path of the matched file; an empty value (e.g.
// no absolute base could be computed) drops back to the per-Check
// memo so struct-literal unit tests still hit the fast path.
func cachedFrontMatter(
	f *lint.File, fsys fs.FS, path, absPath string, maxBytes int64,
) (map[string]any, error) {
	build := func() any {
		fm, err := readFrontMatter(fsys, path, maxBytes)
		return frontMatterResult{fm: fm, err: err}
	}
	var v any
	if f.RunCache != nil && absPath != "" {
		v = f.RunCache.FrontMatter(absPath, build)
	} else {
		v = f.Memo("catalog.fm:"+path, build)
	}
	r := v.(frontMatterResult)
	return r.fm, r.err
}

// absMatchedPath returns the absolute filesystem path of a doublestar
// match m, using the absolute base resolveGlobFS already computed for
// gitignore anchoring. Returns ("", false) when no base is available
// (no RootDir and a non-absolute file path) — the run-scoped cache
// then sits out for that match and the per-Check memo handles it.
func absMatchedPath(res globResolution, m string) (string, bool) {
	if res.gitignoreBase == "" {
		return "", false
	}
	return filepath.Clean(filepath.Join(res.gitignoreBase, m)), true
}

// readFrontMatter reads a file's YAML front matter and returns it as
// a map preserving nested structure for CUE path resolution.
// Returns (nil, nil) if no front matter is found or content is
// malformed. Returns (nil, err) when the file itself cannot be read —
// notably when it exceeds the configured max-input-size — so callers
// can surface the failure instead of treating it as "no front matter".
// Reached during Check via cachedFrontMatter so directives that glob
// overlapping sets do not each re-read the same matched file.
func readFrontMatter(fsys fs.FS, path string, maxBytes int64) (map[string]any, error) {
	data, err := bytelimit.ReadFSFileLimited(fsys, path, maxBytes)
	if err != nil {
		return nil, err
	}

	prefix, _ := lint.StripFrontMatter(data)
	if prefix == nil {
		return nil, nil
	}

	// Extract the YAML between --- delimiters.
	s := string(prefix)
	s = strings.TrimPrefix(s, "---\n")
	idx := strings.Index(s, "---\n")
	if idx < 0 {
		return nil, nil
	}
	yamlStr := s[:idx]

	var raw map[string]any
	if err := yamlutil.UnmarshalSafe([]byte(yamlStr), &raw); err != nil {
		return nil, nil
	}

	return raw, nil
}

// checkCatalogIncludeCycle checks whether any file matched by the catalog
// glob has an include chain that leads back to the catalog-owning file.
// If so, the catalog body would recursively contain itself.
//
// The scan picks its filesystem frame from res: a RootFS-resolved
// catalog (source-dir or ".." patterns) scans through f.RootFS using
// root-relative paths so cross-directory cycles are still detected.
// A local-FS-resolved catalog scans through f.FS using
// host-directory-relative paths, matching the legacy behavior and
// keeping struct-literal unit tests working.
//
// When f.RunCache is set and the scan filesystem's absolute base
// can be derived, the scan switches to an absolute-path traversal
// that shares the include adjacency cache across host files;
// otherwise the legacy fsys-relative scan handles the cycle test.
func checkCatalogIncludeCycle(
	f *lint.File, filePath string, line int,
	entries []fileEntry, res globResolution,
) []lint.Diagnostic {
	if f.FS == nil {
		return nil
	}
	scanFS, catalogFile, scanAbsDir, hasScanAbs := scanFrameFor(f, res, filePath)
	if scanFS == nil {
		return nil
	}
	useAbs := hasScanAbs && f.RunCache != nil
	for _, entry := range entries {
		matchedPath := entry.matchPath
		if matchedPath == "" {
			matchedPath = fieldinterp.Stringify(entry.fields["filename"])
		}
		if matchedIncludesCatalog(f, scanFS, scanAbsDir, useAbs, matchedPath, catalogFile) {
			displayPath := fieldinterp.Stringify(entry.fields["filename"])
			return []lint.Diagnostic{makeDiag(filePath, line,
				fmt.Sprintf(
					"catalog includes %q which includes %q via <?include?>, creating a cycle",
					displayPath, catalogFile))}
		}
	}
	return nil
}

// scanFrameFor returns the (fs, catalog-file-path, abs base, has-abs)
// the cycle scan should use for this directive's resolution. For a
// RootFS-anchored catalog the scan walks through f.RootFS with the
// catalog-owning file expressed root-relative; for a local-FS catalog
// the scan walks through f.FS with the catalog file as its bare
// basename — preserving the legacy behavior whenever rootRelative is
// false (which includes every test that constructs a struct-literal
// File without RootDir).
func scanFrameFor(
	f *lint.File, res globResolution, filePath string,
) (fs.FS, string, string, bool) {
	if !res.rootRelative {
		host, ok := hostAbsDir(f)
		return f.FS, filepath.Base(filePath), host, ok
	}
	if res.fs == nil {
		return nil, "", "", false
	}
	catalog := path.Clean(path.Join(res.fileDir, filepath.Base(filePath)))
	if res.fileDir == "" {
		catalog = filepath.Base(filePath)
	}
	abs, ok := rootAbsDir(f)
	return res.fs, catalog, abs, ok
}

// rootAbsDir returns f.RootDir's absolute path — the directory every
// res.fs-relative path resolves against when the catalog walked the
// project root. Returns ("", false) when no RootDir is set, which
// keeps the run-cache opt-out path consistent with hostAbsDir.
func rootAbsDir(f *lint.File) (string, bool) {
	if f.RootDir == "" {
		return "", false
	}
	abs, err := filepath.Abs(f.RootDir)
	return filepath.Clean(abs), err == nil
}

// matchedIncludesCatalog routes the cycle scan to the absolute-path
// variant when the run cache is in play, falling back to the
// fsys-relative legacy scan otherwise. Pulled out of
// checkCatalogIncludeCycle so each branch reads as one line.
func matchedIncludesCatalog(
	f *lint.File, scanFS fs.FS, scanAbsDir string, useAbs bool,
	matchedPath, catalogFile string,
) bool {
	if useAbs {
		absMatched := filepath.Clean(filepath.Join(scanAbsDir, matchedPath))
		absTarget := filepath.Clean(filepath.Join(scanAbsDir, catalogFile))
		return fileIncludesTargetAbs(f, scanFS, scanAbsDir,
			absMatched, absTarget, f.MaxInputBytes)
	}
	return fileIncludesTarget(f, scanFS, matchedPath, catalogFile, f.MaxInputBytes)
}

// hostAbsDir returns f.FS's absolute filesystem root — the directory
// every fsys-relative path in this Check resolves against. Used to
// translate matched paths into the run cache's absolute keys. Wraps
// absBaseDir so the LSP path (workspace-relative f.Path, absolute
// f.RootDir) lands on the same /workspace/dir base the LSP's
// Server.invalidateCachedRead uses; otherwise cache keys and
// invalidation calls would target different absolute paths and
// edits would not evict stale reads.
func hostAbsDir(f *lint.File) (string, bool) {
	return absBaseDir(f)
}

// fileIncludesTarget checks whether the file at filePath contains
// include directives that (directly or indirectly) reference the
// target file. Uses a visited set to avoid infinite recursion. cf is
// the file being checked; it carries the per-Check memo the include
// adjacency is cached on.
func fileIncludesTarget(
	cf *lint.File, fsys fs.FS, filePath, target string, maxBytes int64,
) bool {
	visited := map[string]bool{filePath: true}
	return scanIncludesForTarget(cf, fsys, filePath, target, visited, 0, maxBytes)
}

// fileIncludesTargetAbs is the absolute-path variant. Used when
// cf.RunCache is set: the cache key is absolute so two host files
// that reach the same target via different fs-relative paths share
// one read. fsys is the scan frame chosen by scanFrameFor — f.FS
// (the host's directory FS) for local-FS-resolved catalogs, or
// res.fs (f.RootFS, the project root) for rootRelative catalogs
// resolved via source-dir or ".." patterns. hostAbsDir is the
// absolute path the fs.FS is anchored at and the directory both
// absFilePath and absTarget live under; read arguments are derived
// by stripping it from the absolute path.
func fileIncludesTargetAbs(
	cf *lint.File, fsys fs.FS, hostAbsDir, absFilePath, absTarget string,
	maxBytes int64,
) bool {
	visited := map[string]bool{absFilePath: true}
	return scanIncludesForTargetAbs(cf, fsys, hostAbsDir,
		absFilePath, absTarget, visited, 0, maxBytes)
}

// maxIncludeDepth mirrors the include rule's depth limit for consistency.
const maxIncludeDepth = 10

// includeTargetsOf returns the resolved paths every <?include?> in
// filePath points at. Reading and fully parsing filePath is the
// include-cycle scan's only expensive step, so the result is memoized
// per Check on cf: a file carrying several catalog directives over an
// overlapping glob (CLAUDE.md has three on docs/**) otherwise re-read
// and re-parsed every matched file once per directive. The adjacency
// is a pure function of the matched file's bytes — independent of the
// cycle target and the DFS visited set — so it is safe to share across
// every directive and recursion within one Check.
//
// The legacy fsys-relative form. The run-cache-aware path uses
// includeTargetsOfAbs so its values stay position-independent and
// shareable across host files.
func includeTargetsOf(
	cf *lint.File, fsys fs.FS, filePath string, maxBytes int64,
) []string {
	v := cf.Memo("catalog.includes:"+filePath, func() any {
		return scanIncludeTargets(fsys, filePath, maxBytes)
	})
	return v.([]string)
}

// scanIncludeTargets reads filePath via fsys, parses its <?include?>
// directives, and returns each directive target as an fsys-relative
// path. Pulled out of includeTargetsOf so the absolute-path variant
// can reuse the same parse without duplicating the read logic.
// lint.NewFile never returns an error (its signature is legacy), so
// the parse is unwrapped — there is no error path to guard.
func scanIncludeTargets(fsys fs.FS, filePath string, maxBytes int64) []string {
	data, err := bytelimit.ReadFSFileLimited(fsys, filePath, maxBytes)
	if err != nil {
		return nil
	}
	_, content := lint.StripFrontMatter(data)
	pf, _ := lint.NewFile(filePath, content)
	pairs, _ := gensection.FindMarkerPairs(
		pf, "include", "MDS021", "include")
	var targets []string
	for _, mp := range pairs {
		dir, diags := gensection.ParseDirective(
			filePath, mp, "MDS021", "include")
		if dir == nil || len(diags) > 0 {
			continue
		}
		file := dir.Params["file"]
		if file == "" {
			continue
		}
		targets = append(targets,
			path.Clean(path.Join(path.Dir(filePath), file)))
	}
	return targets
}

// includeTargetsOfAbs returns the absolute filesystem paths every
// <?include?> in the file at absFilePath resolves to. Cached on
// cf.RunCache (keyed by absFilePath) so two host files whose
// catalogs both glob the file share one read. fsys is the host file's
// f.FS; hostAbsDir is its absolute root.
//
// When absFilePath lies outside hostAbsDir the file is unreadable
// through fsys regardless of its content, so we return nil WITHOUT
// caching: a later host whose hostAbsDir does contain absFilePath
// must be free to parse it fresh. Caching the host-relative
// readability failure here would corrupt the run cache with an
// order-dependent false negative (a real include chain would be
// silently skipped by whichever host hits the entry second).
func includeTargetsOfAbs(
	cf *lint.File, fsys fs.FS, hostAbsDir, absFilePath string, maxBytes int64,
) []string {
	fsRel, ok := relToHost(hostAbsDir, absFilePath)
	if !ok {
		return nil
	}
	return cf.RunCache.Includes(absFilePath, func() []string {
		rels := scanIncludeTargets(fsys, fsRel, maxBytes)
		if len(rels) == 0 {
			return nil
		}
		abs := make([]string, 0, len(rels))
		for _, r := range rels {
			abs = append(abs, filepath.Clean(filepath.Join(hostAbsDir, r)))
		}
		return abs
	})
}

// relToHost converts an absolute path into the form fsys.Open expects:
// a forward-slash path relative to hostAbsDir. Returns ("", false)
// when absPath escapes hostAbsDir (e.g. an include reaches above the
// host file's directory, which the host's os.DirFS would reject
// anyway).
func relToHost(hostAbsDir, absPath string) (string, bool) {
	rel, err := filepath.Rel(hostAbsDir, absPath)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

func scanIncludesForTarget(
	cf *lint.File, fsys fs.FS, filePath, target string,
	visited map[string]bool, depth int, maxBytes int64,
) bool {
	if depth > maxIncludeDepth {
		return false
	}
	for _, resolved := range includeTargetsOf(cf, fsys, filePath, maxBytes) {
		if resolved == target {
			return true
		}
		if visited[resolved] {
			continue
		}
		visited[resolved] = true
		found := scanIncludesForTarget(cf, fsys, resolved, target, visited, depth+1, maxBytes)
		delete(visited, resolved)
		if found {
			return true
		}
	}
	return false
}

// scanIncludesForTargetAbs is the absolute-path variant of
// scanIncludesForTarget. All paths in the recursion (filePath,
// target, visited) are absolute filesystem paths; the include
// adjacency comes from includeTargetsOfAbs, which the run cache
// shares across host files.
func scanIncludesForTargetAbs(
	cf *lint.File, fsys fs.FS, hostAbsDir, absFilePath, absTarget string,
	visited map[string]bool, depth int, maxBytes int64,
) bool {
	if depth > maxIncludeDepth {
		return false
	}
	for _, absResolved := range includeTargetsOfAbs(cf, fsys, hostAbsDir, absFilePath, maxBytes) {
		if absResolved == absTarget {
			return true
		}
		if visited[absResolved] {
			continue
		}
		visited[absResolved] = true
		found := scanIncludesForTargetAbs(cf, fsys, hostAbsDir,
			absResolved, absTarget, visited, depth+1, maxBytes)
		delete(visited, absResolved)
		if found {
			return true
		}
	}
	return false
}

// isExcluded checks whether a file path matches any of the exclude patterns.
func isExcluded(filePath string, patterns []string) bool {
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		if ok, _ := doublestar.Match(pattern, filePath); ok {
			return true
		}
	}
	return false
}

// isGitignored checks whether a glob-matched path is ignored by gitignore
// rules. The matchedPath is relative to the catalog file's directory;
// base is the pre-computed absolute path of that directory. To match
// gitignore semantics for directory-only patterns (e.g. "ignored/"),
// ancestor directories are also checked with isDir=true.
func isGitignored(matcher *gitignore.Matcher, base, matchedPath string) bool {
	abs := filepath.Join(base, matchedPath)

	// Check whether any ancestor directory is ignored (handles dir-only
	// gitignore patterns like "ignored/").
	for dir := filepath.Dir(abs); ; dir = filepath.Dir(dir) {
		if matcher.IsIgnored(dir, true) {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}

	return matcher.IsIgnored(abs, false)
}

// parseRowTemplate validates a row template string containing {field}
// placeholders.
func parseRowTemplate(row string) error {
	return fieldinterp.Validate(row)
}

// checkCaseMismatches scans catalog directives in the file for
// case-mismatched front-matter field references and returns hint
// diagnostics. Runs independently of the Generate/Fix path so
// hints don't block content generation.
func (r *Rule) checkCaseMismatches(f *lint.File) []lint.Diagnostic {
	pairs, _ := gensection.FindMarkerPairs(
		f, r.Name(), r.ID(), r.Name(),
	)
	var diags []lint.Diagnostic
	for _, mp := range pairs {
		dir, parseDiags := gensection.ParseDirective(
			f.Path, mp, r.ID(), r.Name(),
		)
		if dir == nil || len(parseDiags) > 0 {
			continue
		}
		row, hasRow := dir.Params["row"]
		if !hasRow {
			continue
		}
		// Skip entry building when the row only references {filename},
		// since that's a built-in field — no front-matter to mismatch.
		fields := extractPlaceholderFields(row)
		hasNonBuiltin := false
		for _, name := range fields {
			if !strings.EqualFold(name, "filename") {
				hasNonBuiltin = true
				break
			}
		}
		if !hasNonBuiltin {
			continue
		}
		// Ignore entry diagnostics here; Generate surfaces them.
		entries, _, _ := cachedCatalogEntries(f, dir.Params, f.Path, mp.StartLine)
		diags = append(diags, checkFieldCaseMismatches(f.Path, mp.StartLine, row, entries)...)
	}
	return diags
}

// extractPlaceholderFields returns the deduplicated set of field names
// referenced by {field} placeholders in a row template string.
func extractPlaceholderFields(row string) []string {
	all := fieldinterp.Fields(row)
	seen := make(map[string]bool, len(all))
	var fields []string
	for _, name := range all {
		if !seen[name] {
			seen[name] = true
			fields = append(fields, name)
		}
	}
	return fields
}

// checkFieldCaseMismatches checks whether any placeholder field referenced
// in the row template is missing from front-matter but has a case-insensitive
// match. Aggregates all observed casings across entries (including the
// exact template field) so that:
//   - no entry has an exact match + single alternative → "did you mean X?"
//   - mixed casings across files → "inconsistent casing" warning
func checkFieldCaseMismatches(filePath string, line int, row string, entries []fileEntry) []lint.Diagnostic {
	fields := extractPlaceholderFields(row)
	if len(fields) == 0 {
		return nil
	}

	var diags []lint.Diagnostic

	for _, field := range fields {
		if strings.EqualFold(field, "filename") {
			continue
		}

		// Collect all distinct casings of this field across all entries.
		casingsSet := make(map[string]struct{})
		for _, entry := range entries {
			for key := range entry.fields {
				if strings.EqualFold(key, field) {
					casingsSet[key] = struct{}{}
				}
			}
		}

		if len(casingsSet) == 0 {
			continue
		}

		// If the only casing observed is the exact template field, no mismatch
		// (some files may lack the field entirely — that's not a casing issue).
		_, hasExact := casingsSet[field]
		if hasExact && len(casingsSet) == 1 {
			continue
		}

		// Sort for deterministic diagnostics.
		casings := make([]string, 0, len(casingsSet))
		for key := range casingsSet {
			casings = append(casings, key)
		}
		sort.Strings(casings)

		var message string
		switch {
		case len(casings) == 1 && !hasExact:
			// No entry has the exact field; single alternative found.
			message = fmt.Sprintf("field %q not found; did you mean %q?", field, casings[0])
		default:
			// Multiple casings across files — surface the inconsistency.
			quoted := make([]string, len(casings))
			for i, c := range casings {
				quoted[i] = fmt.Sprintf("%q", c)
			}
			message = fmt.Sprintf(
				"field %q has inconsistent casing across matched files: %s",
				field, strings.Join(quoted, ", "),
			)
		}

		diag := makeDiag(filePath, line, message)
		diag.Severity = lint.Warning
		diags = append(diags, diag)
	}
	return diags
}

// FixTitle implements rule.QuickFixTitler.
func (r *Rule) FixTitle() string { return "Regenerate catalog" }
