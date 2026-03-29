package catalog

import (
	"fmt"
	"io/fs"
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
	return r.getEngine().Check(f)
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
	entries := buildCatalogEntries(f, params)

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
// (from a YAML list).
func validateGlob(filePath string, line int, params map[string]string) []lint.Diagnostic {
	glob, hasGlob := params["glob"]
	if !hasGlob {
		return []lint.Diagnostic{makeDiag(filePath, line,
			`generated section directive missing required "glob" parameter`)}
	}
	for _, pattern := range splitGlobs(glob) {
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
	if key == "" || strings.ContainsAny(key, " \t") {
		return []lint.Diagnostic{makeDiag(filePath, line,
			fmt.Sprintf("generated section directive has invalid sort value %q", sortVal))}
	}
	return nil
}

// buildCatalogEntries resolves glob matches, reads front matter, and
// returns sorted file entries for the catalog directive.
func buildCatalogEntries(f *lint.File, params map[string]string) []fileEntry {
	seen := make(map[string]bool)
	var files []string
	for _, pattern := range splitGlobs(params["glob"]) {
		matches, err := doublestar.Glob(f.FS, pattern)
		if err != nil {
			continue
		}
		for _, m := range matches {
			if seen[m] {
				continue
			}
			info, err := fs.Stat(f.FS, m)
			if err != nil || info.IsDir() {
				continue
			}
			seen[m] = true
			files = append(files, m)
		}
	}

	sortKey, descending := parseSort(params)
	_, hasRow := params["row"]
	needFM := hasRow || (sortKey != "path" && sortKey != "filename")

	entries := make([]fileEntry, 0, len(files))
	for _, path := range files {
		fields := map[string]any{"filename": path}
		if needFM {
			for k, v := range readFrontMatter(f.FS, path) {
				fields[k] = v
			}
		}
		entries = append(entries, fileEntry{fields: fields})
	}

	sortEntries(entries, sortKey, descending)
	return entries
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
			// Not a valid CUE path; try direct map lookup.
			return fieldinterp.Stringify(entry.fields[key])
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
// Returns nil if no front matter is found or on any error.
func readFrontMatter(fsys fs.FS, path string) map[string]any {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil
	}

	prefix, _ := lint.StripFrontMatter(data)
	if prefix == nil {
		return nil
	}

	// Extract the YAML between --- delimiters.
	s := string(prefix)
	s = strings.TrimPrefix(s, "---\n")
	idx := strings.Index(s, "---\n")
	if idx < 0 {
		return nil
	}
	yamlStr := s[:idx]

	var raw map[string]any
	if err := yaml.Unmarshal([]byte(yamlStr), &raw); err != nil {
		return nil
	}

	return raw
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
