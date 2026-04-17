package catalog

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/fieldinterp"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/tableformat"
	"gopkg.in/yaml.v3"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that generated sections match their directive output.
type Rule struct {
	engine *gensection.Engine
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS019" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "catalog" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// RuleID implements gensection.Directive.
func (r *Rule) RuleID() string { return "MDS019" }

// RuleName implements gensection.Directive.
func (r *Rule) RuleName() string { return "catalog" }

// getEngine lazily initializes and returns the gensection engine.
func (r *Rule) getEngine() *gensection.Engine {
	if r.engine == nil {
		r.engine = gensection.NewEngine(r)
	}
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
	entries, entryDiags := buildCatalogEntries(f, params, filePath, line)
	if len(entryDiags) > 0 {
		return "", entryDiags
	}

	// Check if any matched file includes (directly or indirectly) the
	// catalog-owning file. If so, the catalog body would contain itself.
	if diags := checkCatalogIncludeCycle(f, filePath, line, entries); len(diags) > 0 {
		return "", diags
	}

	_, hasRow := params["row"]
	content, err := renderCatalogContent(params, entries, cols, hasRow)
	if err != nil {
		return "", []lint.Diagnostic{makeDiag(filePath, line,
			fmt.Sprintf("generated section template execution failed: %v", err))}
	}

	// Format tables to comply with MDS025 (table-format) settings.
	content = tableformat.FormatString(content, tableFormatPad())

	return content, nil
}

// tableFormatPad returns the pad setting from the MDS025 (table-format)
// rule, defaulting to 1 if not found.
func tableFormatPad() int {
	r := rule.ByID("MDS025")
	if r == nil {
		return 1
	}
	type padder interface{ GetPad() int }
	if p, ok := r.(padder); ok {
		return p.GetPad()
	}
	return 1
}

// validateCatalogDirective validates parameters specific to the catalog directive.
func validateCatalogDirective(
	filePath string, line int,
	params map[string]string,
	columns map[string]gensection.ColumnConfig,
) []lint.Diagnostic {
	_, hasRow := params["row"]
	_, hasHeader := params["header"]
	_, hasFooter := params["footer"]

	if (hasHeader || hasFooter) && !hasRow {
		return []lint.Diagnostic{makeDiag(filePath, line,
			`generated section template missing required "row" key`)}
	}
	if hasRow && strings.TrimSpace(params["row"]) == "" {
		return []lint.Diagnostic{makeDiag(filePath, line,
			`generated section directive has empty "row" value`)}
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
	if hasRow {
		if err := parseRowTemplate(params["row"]); err != nil {
			diags = append(diags, makeDiag(filePath, line,
				fmt.Sprintf("generated section has invalid template: %v", err)))
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
		if containsDotDot(pattern) {
			return []lint.Diagnostic{makeDiag(filePath, line,
				`generated section directive has glob pattern with ".." path traversal`)}
		}
		if !doublestar.ValidatePattern(pattern) {
			return []lint.Diagnostic{makeDiag(filePath, line,
				fmt.Sprintf("generated section directive has invalid glob pattern: %s", pattern))}
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

// splitIncludeExclude separates glob patterns into include and exclude lists.
// Patterns prefixed with "!" are exclusion patterns (prefix is stripped).
func splitIncludeExclude(glob string) (include, exclude []string) {
	for _, pattern := range splitGlobs(glob) {
		if strings.HasPrefix(pattern, "!") {
			exclude = append(exclude, pattern[1:])
		} else {
			include = append(include, pattern)
		}
	}
	return include, exclude
}

// resolveGlobFS returns the filesystem to use for glob resolution and
// a path prefix for display filenames. When source-dir is set (injected
// by include expansion), globs resolve from that subdirectory of RootFS
// and matched filenames are prefixed relative to the catalog-owning
// file's directory so links work correctly.
func resolveGlobFS(f *lint.File, params map[string]string) (globFS fs.FS, prefix string) {
	sourceDir := params["source-dir"]
	if sourceDir == "" || f.RootFS == nil {
		return f.FS, ""
	}

	sourceDir = path.Clean(sourceDir)
	fileDir := path.Clean(filepath.ToSlash(filepath.Dir(f.Path)))

	// Compute prefix relative to the file's directory so display
	// filenames produce correct links from the including file.
	relPrefix, err := filepath.Rel(fileDir, sourceDir)
	if err != nil {
		return f.FS, ""
	}
	relPrefix = filepath.ToSlash(relPrefix)

	if sourceDir == "." {
		if relPrefix == "." {
			return f.RootFS, ""
		}
		return f.RootFS, relPrefix
	}

	sub, err := fs.Sub(f.RootFS, sourceDir)
	if err != nil {
		return f.FS, ""
	}
	if relPrefix == "." {
		return sub, ""
	}
	return sub, relPrefix
}

// buildCatalogEntries resolves glob matches, reads front matter, and
// returns sorted file entries for the catalog directive. Read errors
// (notably "file too large") are returned as diagnostics attached to
// the directive's file+line. Callers in the Generate path treat any
// returned diagnostic as fatal to avoid producing an incomplete catalog;
// check-only callers (checkInjection, checkCaseMismatches) discard the
// diagnostics because Generate already surfaces them.
func buildCatalogEntries(
	f *lint.File, params map[string]string, filePath string, line int,
) ([]fileEntry, []lint.Diagnostic) {
	globFS, prefix := resolveGlobFS(f, params)
	files := resolveGlobMatchesFrom(globFS, f, params)

	sortKey, descending := parseSort(params)
	_, hasRow := params["row"]
	needFM := hasRow || (sortKey != "path" && sortKey != "filename")

	var diags []lint.Diagnostic
	entries := make([]fileEntry, 0, len(files))
	for _, p := range files {
		displayPath := p
		if prefix != "" {
			displayPath = path.Join(prefix, p)
		}
		fields := map[string]any{"filename": displayPath}
		if needFM {
			fm, err := readFrontMatter(globFS, p, f.MaxInputBytes)
			if err != nil {
				diags = append(diags, makeDiag(filePath, line,
					fmt.Sprintf("cannot read front matter from %q: %v", displayPath, err)))
				continue
			}
			for k, v := range fm {
				fields[k] = v
			}
		}
		entries = append(entries, fileEntry{fields: fields})
	}

	sortEntries(entries, sortKey, descending)
	return entries, diags
}

// resolveGlobMatchesFrom expands include patterns using the given FS,
// filters out exclude and gitignore matches, and returns deduplicated
// file paths.
func resolveGlobMatchesFrom(globFS fs.FS, f *lint.File, params map[string]string) []string {
	includePatterns, excludePatterns := splitIncludeExclude(params["glob"])
	matcher, base := resolveGitignore(f, params)

	seen := make(map[string]bool)
	var files []string
	for _, pattern := range includePatterns {
		matches, err := doublestar.Glob(globFS, pattern)
		if err != nil {
			continue
		}
		for _, m := range matches {
			if seen[m] {
				continue
			}
			info, err := fs.Stat(globFS, m)
			if err != nil || info.IsDir() {
				continue
			}
			if isExcluded(m, excludePatterns) {
				continue
			}
			if matcher != nil && isGitignored(matcher, base, m) {
				continue
			}
			seen[m] = true
			files = append(files, m)
		}
	}
	return files
}

// resolveGitignore returns the gitignore matcher and base directory to use
// for filtering, or (nil, "") if gitignore filtering is disabled.
func resolveGitignore(f *lint.File, params map[string]string) (*lint.GitignoreMatcher, string) {
	if params["gitignore"] == "false" {
		return nil, ""
	}
	matcher := f.GetGitignore()
	if matcher == nil {
		return nil, ""
	}
	baseDir := filepath.Dir(f.Path)
	if sd := params["source-dir"]; sd != "" && f.RootDir != "" {
		sd = path.Clean(sd)
		if !filepath.IsAbs(sd) && !containsDotDot(sd) {
			baseDir = filepath.Join(f.RootDir, sd)
		}
	}
	base, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, ""
	}
	return matcher, base
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
		entries, _ := buildCatalogEntries(f, dir.Params, f.Path, mp.StartLine)
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

// parseSort parses the sort value from params, returning the key and direction.
func parseSort(params map[string]string) (key string, descending bool) {
	sortVal, ok := params["sort"]
	if !ok || sortVal == "" {
		return "path", false
	}

	if strings.HasPrefix(sortVal, "-") {
		return sortVal[1:], true
	}
	return sortVal, false
}

// sortEntries sorts file entries by the given key.
func sortEntries(entries []fileEntry, key string, descending bool) {
	sort.SliceStable(entries, func(i, j int) bool {
		vi := sortValue(entries[i], key)
		vj := sortValue(entries[j], key)

		cmp := strings.Compare(strings.ToLower(vi), strings.ToLower(vj))
		if cmp == 0 {
			// Tiebreaker: path ascending, case-insensitive.
			pi := strings.ToLower(fieldinterp.Stringify(entries[i].fields["filename"]))
			pj := strings.ToLower(fieldinterp.Stringify(entries[j].fields["filename"]))
			return pi < pj
		}

		if descending {
			return cmp > 0
		}
		return cmp < 0
	})
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

// readFrontMatter reads a file's YAML front matter and returns it as
// a map preserving nested structure for CUE path resolution.
// Returns (nil, nil) if no front matter is found or content is
// malformed. Returns (nil, err) when the file itself cannot be read —
// notably when it exceeds the configured max-input-size — so callers
// can surface the failure instead of treating it as "no front matter".
func readFrontMatter(fsys fs.FS, path string, maxBytes int64) (map[string]any, error) {
	data, err := lint.ReadFSFileLimited(fsys, path, maxBytes)
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
	if err := lint.RejectYAMLAliases([]byte(yamlStr)); err != nil {
		return nil, nil
	}
	if err := yaml.Unmarshal([]byte(yamlStr), &raw); err != nil {
		return nil, nil
	}

	return raw, nil
}

// checkCatalogIncludeCycle checks whether any file matched by the catalog
// glob has an include chain that leads back to the catalog-owning file.
// If so, the catalog body would recursively contain itself.
func checkCatalogIncludeCycle(
	f *lint.File, filePath string, line int,
	entries []fileEntry,
) []lint.Diagnostic {
	if f.FS == nil {
		return nil
	}
	// matchedPath from doublestar.Glob is relative to f.FS (the
	// catalog file's directory). filePath may be repo-relative, so
	// normalize the catalog owner to the same FS-relative form.
	catalogFile := filepath.Base(filePath)
	for _, entry := range entries {
		matchedPath := fieldinterp.Stringify(entry.fields["filename"])
		if fileIncludesTarget(f.FS, matchedPath, catalogFile, f.MaxInputBytes) {
			return []lint.Diagnostic{makeDiag(filePath, line,
				fmt.Sprintf(
					"catalog includes %q which includes %q via <?include?>, creating a cycle",
					matchedPath, catalogFile))}
		}
	}
	return nil
}

// fileIncludesTarget checks whether the file at filePath contains
// include directives that (directly or indirectly) reference the
// target file. Uses a visited set to avoid infinite recursion.
func fileIncludesTarget(
	fsys fs.FS, filePath, target string, maxBytes int64,
) bool {
	visited := map[string]bool{filePath: true}
	return scanIncludesForTarget(fsys, filePath, target, visited, 0, maxBytes)
}

// maxIncludeDepth mirrors the include rule's depth limit for consistency.
const maxIncludeDepth = 10

func scanIncludesForTarget(
	fsys fs.FS, filePath, target string,
	visited map[string]bool, depth int, maxBytes int64,
) bool {
	if depth > maxIncludeDepth {
		return false
	}
	data, err := lint.ReadFSFileLimited(fsys, filePath, maxBytes)
	if err != nil {
		return false
	}
	_, content := lint.StripFrontMatter(data)
	f, err := lint.NewFile(filePath, content)
	if err != nil {
		return false
	}
	pairs, _ := gensection.FindMarkerPairs(
		f, "include", "MDS021", "include")
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
		resolved := path.Clean(path.Join(path.Dir(filePath), file))
		if resolved == target {
			return true
		}
		if visited[resolved] {
			continue
		}
		visited[resolved] = true
		found := scanIncludesForTarget(fsys, resolved, target, visited, depth+1, maxBytes)
		delete(visited, resolved)
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
		matched, err := doublestar.Match(pattern, filePath)
		if err == nil && matched {
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
func isGitignored(matcher *lint.GitignoreMatcher, base, matchedPath string) bool {
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

// containsDotDot checks if a glob pattern contains ".." path traversal.
func containsDotDot(pattern string) bool {
	parts := strings.Split(pattern, "/")
	for _, p := range parts {
		if p == ".." {
			return true
		}
	}
	return false
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
		entries, _ := buildCatalogEntries(f, dir.Params, f.Path, mp.StartLine)
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
