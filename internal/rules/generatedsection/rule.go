package generatedsection

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
	"gopkg.in/yaml.v3"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that generated sections match their directive output.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "TM019" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "generated-section" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f.FS == nil {
		return nil
	}

	pairs, diags := findMarkerPairs(f)
	for _, mp := range pairs {
		pairDiags := r.checkPair(f, mp)
		diags = append(diags, pairDiags...)
	}
	return diags
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	if f.FS == nil {
		return f.Source
	}

	pairs, _ := findMarkerPairs(f)

	// Work backwards to preserve line numbers.
	for i := len(pairs) - 1; i >= 0; i-- {
		mp := pairs[i]
		expected, ok := r.generateContent(f, mp)
		if !ok {
			continue
		}

		f.Source = replaceContent(f, mp, expected)
		f.Lines = splitLines(f.Source)
	}

	return f.Source
}

// checkPair checks a single marker pair and returns diagnostics.
func (r *Rule) checkPair(f *lint.File, mp markerPair) []lint.Diagnostic {
	dir, diags := parseDirective(f, mp)
	if dir == nil || len(diags) > 0 {
		return diags
	}

	valDiags := r.validateDirective(f, mp, dir)
	if len(valDiags) > 0 {
		return valDiags
	}

	expected, genDiags := r.resolveCatalogWithDiags(f, mp, dir)
	if len(genDiags) > 0 {
		return genDiags
	}

	actual := extractContent(f, mp)
	if actual != expected {
		return []lint.Diagnostic{{
			File:     f.Path,
			Line:     mp.startLine,
			Column:   1,
			RuleID:   "TM019",
			RuleName: "generated-section",
			Severity: lint.Error,
			Message:  "generated section is out of date",
		}}
	}

	return nil
}

// generateContent generates the expected content for a marker pair.
// Returns the content and true if generation succeeded, or empty and
// false if there were validation errors or generation errors.
func (r *Rule) generateContent(f *lint.File, mp markerPair) (string, bool) {
	dir, diags := parseDirective(f, mp)
	if dir == nil || len(diags) > 0 {
		return "", false
	}

	moreDiags := r.validateDirective(f, mp, dir)
	if len(moreDiags) > 0 {
		return "", false
	}

	return r.resolveCatalog(f, dir)
}

// validateDirective validates a parsed directive's parameters.
func (r *Rule) validateDirective(f *lint.File, mp markerPair, dir *directive) []lint.Diagnostic {
	if dir.name != "catalog" {
		return []lint.Diagnostic{makeDiag(f.Path, mp.startLine,
			fmt.Sprintf("unknown generated section directive %q", dir.name))}
	}
	return validateCatalogDirective(f.Path, mp.startLine, dir)
}

// validateCatalogDirective validates parameters specific to the catalog directive.
func validateCatalogDirective(filePath string, line int, dir *directive) []lint.Diagnostic {
	_, hasRow := dir.params["row"]
	_, hasHeader := dir.params["header"]
	_, hasFooter := dir.params["footer"]

	if (hasHeader || hasFooter) && !hasRow {
		return []lint.Diagnostic{makeDiag(filePath, line,
			`generated section template missing required "row" key`)}
	}
	if hasRow && strings.TrimSpace(dir.params["row"]) == "" {
		return []lint.Diagnostic{makeDiag(filePath, line,
			`generated section directive has empty "row" value`)}
	}

	if diags := validateGlob(filePath, line, dir.params); len(diags) > 0 {
		return diags
	}

	var diags []lint.Diagnostic
	if sortVal, hasSort := dir.params["sort"]; hasSort {
		diags = append(diags, validateSort(filePath, line, sortVal)...)
	}
	if hasRow {
		if _, err := parseRowTemplate(dir.params["row"]); err != nil {
			diags = append(diags, makeDiag(filePath, line,
				fmt.Sprintf("generated section has invalid template: %v", err)))
		}
	}
	return diags
}

// validateGlob validates the glob parameter and returns diagnostics on failure.
func validateGlob(filePath string, line int, params map[string]string) []lint.Diagnostic {
	glob, hasGlob := params["glob"]
	if !hasGlob {
		return []lint.Diagnostic{makeDiag(filePath, line,
			`generated section directive missing required "glob" parameter`)}
	}
	if glob == "" {
		return []lint.Diagnostic{makeDiag(filePath, line,
			`generated section directive has empty "glob" parameter`)}
	}
	if filepath.IsAbs(glob) {
		return []lint.Diagnostic{makeDiag(filePath, line,
			"generated section directive has absolute glob path")}
	}
	if containsDotDot(glob) {
		return []lint.Diagnostic{makeDiag(filePath, line,
			`generated section directive has glob pattern with ".." path traversal`)}
	}
	if !doublestar.ValidatePattern(glob) {
		return []lint.Diagnostic{makeDiag(filePath, line,
			fmt.Sprintf("generated section directive has invalid glob pattern: %s", glob))}
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

// resolveCatalogWithDiags generates content and returns diagnostics on failure.
func (r *Rule) resolveCatalogWithDiags(f *lint.File, mp markerPair, dir *directive) (string, []lint.Diagnostic) {
	entries := buildCatalogEntries(f, dir)

	_, hasRow := dir.params["row"]
	content, err := renderCatalogContent(dir, entries, hasRow)
	if err != nil {
		return "", []lint.Diagnostic{makeDiag(f.Path, mp.startLine,
			fmt.Sprintf("generated section template execution failed: %v", err))}
	}
	return content, nil
}

// resolveCatalog generates content for the catalog directive.
// Returns the content and true, or empty and false on error.
func (r *Rule) resolveCatalog(f *lint.File, dir *directive) (string, bool) {
	entries := buildCatalogEntries(f, dir)

	_, hasRow := dir.params["row"]
	content, err := renderCatalogContent(dir, entries, hasRow)
	if err != nil {
		return "", false
	}
	return content, true
}

// buildCatalogEntries resolves glob matches, reads front matter, and
// returns sorted file entries for the catalog directive.
func buildCatalogEntries(f *lint.File, dir *directive) []fileEntry {
	matches, err := doublestar.Glob(f.FS, dir.params["glob"])
	if err != nil {
		return nil
	}

	var files []string
	for _, m := range matches {
		info, err := fs.Stat(f.FS, m)
		if err != nil || info.IsDir() {
			continue
		}
		files = append(files, m)
	}

	sortKey, descending := parseSort(dir.params)
	_, hasRow := dir.params["row"]
	needFM := hasRow || (sortKey != "path" && sortKey != "filename")

	entries := make([]fileEntry, 0, len(files))
	for _, path := range files {
		fields := map[string]string{"filename": path}
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
func renderCatalogContent(dir *directive, entries []fileEntry, hasRow bool) (string, error) {
	if len(entries) == 0 {
		return renderEmpty(dir.params), nil
	}
	if !hasRow {
		return renderMinimal(entries), nil
	}
	return renderTemplate(dir.params, entries, dir.columns)
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
			pi := strings.ToLower(entries[i].fields["filename"])
			pj := strings.ToLower(entries[j].fields["filename"])
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
		return entry.fields["filename"]
	case "filename":
		return filepath.Base(entry.fields["filename"])
	default:
		return entry.fields[key]
	}
}

// readFrontMatter reads a file's YAML front matter and returns it as
// a string map. Non-string values are converted via fmt.Sprintf.
// Returns nil if no front matter is found or on any error.
func readFrontMatter(fsys fs.FS, path string) map[string]string {
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

	result := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			result[k] = s
		} else {
			result[k] = fmt.Sprintf("%v", v)
		}
	}
	return result
}

// extractContent returns the content between markers as a string.
func extractContent(f *lint.File, mp markerPair) string {
	if mp.contentFrom > mp.contentTo {
		return ""
	}
	// contentFrom and contentTo are 1-based line numbers.
	var lines []string
	for i := mp.contentFrom - 1; i <= mp.contentTo-1 && i < len(f.Lines); i++ {
		lines = append(lines, string(f.Lines[i]))
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// replaceContent replaces the content between markers with new content.
func replaceContent(f *lint.File, mp markerPair, content string) []byte {
	var result []byte

	// Lines before content.
	for i := 0; i < mp.contentFrom-1 && i < len(f.Lines); i++ {
		result = append(result, f.Lines[i]...)
		result = append(result, '\n')
	}

	// New content.
	result = append(result, []byte(content)...)

	// Lines from end marker onward.
	for i := mp.endLine - 1; i < len(f.Lines); i++ {
		result = append(result, f.Lines[i]...)
		if i < len(f.Lines)-1 {
			result = append(result, '\n')
		}
	}

	return result
}

// splitLines splits source into lines (like bytes.Split but returns [][]byte).
func splitLines(source []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range source {
		if b == '\n' {
			lines = append(lines, source[start:i])
			start = i + 1
		}
	}
	lines = append(lines, source[start:])
	return lines
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

// parseRowTemplate parses a row template string.
// The missingkey=zero option ensures missing map keys produce empty strings
// rather than "<no value>".
func parseRowTemplate(row string) (*template.Template, error) {
	return template.New("row").Option("missingkey=zero").Parse(row)
}
