// Package release: coverage.go renders the peer-linter coverage
// matrix from each rule README's front matter into the single
// canonical page at docs/research/markdownlint-coverage/README.md.
//
// Source of truth: the `markdownlint:`, `rumdl:`, `mado:`, and
// `panache:` blocks in each
// internal/rules/MDS###-<rule-name>/README.md (matched by the
// embed glob `MDS*/README.md`). The generator never reads
// upstream tool repositories — defaults and rule IDs live in
// the rule READMEs and are updated by hand when peers ship a
// new release.
package release

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/jeduden/mdsmith/internal/rules"
)

// CoverageMatrixFile is the path, relative to the repo root, of
// the generated coverage matrix.
const CoverageMatrixFile = "docs/research/markdownlint-coverage/README.md"

// categoryOrder controls the section order in the rendered page.
// Categories absent from this list appear alphabetically at the
// end so a new mdsmith category doesn't silently disappear.
var categoryOrder = []string{
	"heading",
	"list",
	"whitespace",
	"code",
	"link",
	"table",
	"prose",
	"structural",
	"directive",
	"accessibility",
	"line",
}

// categoryTitle maps a `category:` front-matter value to the
// section heading text used in the rendered page.
var categoryTitle = map[string]string{
	"accessibility": "Accessibility",
	"code":          "Code blocks and code spans",
	"directive":     "Generated sections (directives)",
	"heading":       "Headings",
	"line":          "Line length",
	"link":          "Links and references",
	"list":          "Lists",
	"prose":         "Prose and readability",
	"structural":    "Structure and cross-file",
	"table":         "Tables",
	"whitespace":    "Whitespace, blank lines, tabs",
}

// RenderCoverageMatrix builds the complete page body from a
// sorted slice of rule metadata. Output is byte-stable across
// runs given the same input.
func RenderCoverageMatrix(rs []rules.RuleInfo) string {
	var buf bytes.Buffer

	buf.WriteString("---\n")
	buf.WriteString("summary: >-\n")
	buf.WriteString("  Per-tool, per-rule coverage matrix: each mdsmith rule\n")
	buf.WriteString("  alongside its analog in markdownlint, rumdl, mado,\n")
	buf.WriteString("  and panache, with the upstream default-enabled state\n")
	buf.WriteString("  per peer.\n")
	buf.WriteString("---\n")
	buf.WriteString("# Peer-linter coverage matrix\n\n")
	buf.WriteString("For every mdsmith rule, the analog rule in each peer\n")
	buf.WriteString("Markdown linter (markdownlint, rumdl, mado, panache)\n")
	buf.WriteString("with the peer's upstream default-enabled state. This\n")
	buf.WriteString("page is generated from each rule README's front matter\n")
	buf.WriteString("by `mdsmith-release sync-coverage-matrix`; do not edit\n")
	buf.WriteString("it by hand.\n\n")
	buf.WriteString("Cell legend:\n\n")
	buf.WriteString("- ✅ implemented, enabled by default upstream\n")
	buf.WriteString("- ⚪ implemented, off by default upstream\n")
	buf.WriteString("- (partial) — covers only part of the named rule\n")
	buf.WriteString("- — no analog rule\n\n")

	grouped := groupByCategory(rs)
	cats := orderedCategories(grouped)
	for i, cat := range cats {
		rsInCat := grouped[cat]
		mdsmithOnly := categoryIsMdsmithOnly(rsInCat)
		title := categoryTitle[cat]
		if title == "" {
			title = unknownCategoryTitle(cat)
		}
		if mdsmithOnly {
			title += " (mdsmith-only)"
		}
		fmt.Fprintf(&buf, "## %s\n\n", title)
		if mdsmithOnly {
			renderMdsmithOnlyTable(&buf, rsInCat)
		} else {
			renderPeerTable(&buf, rsInCat)
		}
		if i < len(cats)-1 {
			buf.WriteByte('\n')
		}
	}

	return buf.String()
}

// renderPeerTable emits the five-column table used for mdsmith
// rules that have at least one peer-linter analog somewhere in
// the category. Column widths are sized to the widest cell so
// the output passes the MDS025 table-format rule without a
// downstream `mdsmith fix` pass.
func renderPeerTable(buf *bytes.Buffer, rs []rules.RuleInfo) {
	headers := []string{"mdsmith", "markdownlint", "rumdl", "mado", "panache"}
	rows := make([][]string, 0, len(rs))
	for _, r := range rs {
		rows = append(rows, []string{
			renderMdsmithCell(r),
			renderPeerCell(r.Markdownlint),
			renderPeerCell(r.Rumdl),
			renderPeerCell(r.Mado),
			renderPeerCell(r.Panache),
		})
	}
	writePaddedTable(buf, headers, rows)
}

// renderMdsmithOnlyTable emits the two-column table used for
// categories whose rules have no peer-linter analog (typically
// the mdsmith-only directives and project-level checks).
func renderMdsmithOnlyTable(buf *bytes.Buffer, rs []rules.RuleInfo) {
	headers := []string{"mdsmith", "What it adds"}
	rows := make([][]string, 0, len(rs))
	for _, r := range rs {
		rows = append(rows, []string{
			renderMdsmithCell(r),
			r.Description,
		})
	}
	writePaddedTable(buf, headers, rows)
}

// writePaddedTable emits a GFM table whose columns are padded
// to the widest cell in the column, matching the format MDS025
// (table-format) auto-fixes to. Cell content is not modified —
// only trailing space padding is inserted.
func writePaddedTable(buf *bytes.Buffer, headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = runewidth.StringWidth(h)
	}
	for _, row := range rows {
		for i, c := range row {
			if w := runewidth.StringWidth(c); w > widths[i] {
				widths[i] = w
			}
		}
	}
	// Header row.
	for i, h := range headers {
		if i == 0 {
			buf.WriteByte('|')
		}
		buf.WriteByte(' ')
		buf.WriteString(h)
		buf.WriteString(strings.Repeat(" ", widths[i]-runewidth.StringWidth(h)))
		buf.WriteString(" |")
	}
	buf.WriteByte('\n')
	// Separator row.
	for i := range headers {
		if i == 0 {
			buf.WriteByte('|')
		}
		buf.WriteByte(' ')
		buf.WriteString(strings.Repeat("-", widths[i]))
		buf.WriteString(" |")
	}
	buf.WriteByte('\n')
	// Body rows.
	for _, row := range rows {
		for i, c := range row {
			if i == 0 {
				buf.WriteByte('|')
			}
			buf.WriteByte(' ')
			buf.WriteString(c)
			buf.WriteString(strings.Repeat(" ", widths[i]-runewidth.StringWidth(c)))
			buf.WriteString(" |")
		}
		buf.WriteByte('\n')
	}
}

// renderMdsmithCell prints the mdsmith ID and kebab-case name as
// a Markdown link to the rule's README, with a "not-ready" tag
// on rules still in development.
func renderMdsmithCell(r rules.RuleInfo) string {
	link := fmt.Sprintf("[%s](../../../internal/rules/%s-%s/README.md) %s",
		r.ID, r.ID, r.Name, r.Name)
	if r.Status != "ready" {
		link += " (not-ready)"
	}
	return link
}

// renderPeerCell renders a list of peer-linter mappings as one
// comma-joined cell. A nil/empty list renders as the long-dash
// "no analog" marker.
func renderPeerCell(ms []rules.RuleMapping) string {
	if len(ms) == 0 {
		return "—"
	}
	parts := make([]string, 0, len(ms))
	for _, m := range ms {
		mark := "✅"
		if !m.Default {
			mark = "⚪"
		}
		part := fmt.Sprintf("%s %s %s", m.ID, mark, m.Name)
		if m.Partial {
			part += " (partial)"
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ", ")
}

// groupByCategory buckets rules by their `category:` front-matter
// value. The "" key catches rules whose READMEs are missing a
// category — surfaced as a separate section so the gap is loud
// rather than silent.
func groupByCategory(rs []rules.RuleInfo) map[string][]rules.RuleInfo {
	out := make(map[string][]rules.RuleInfo)
	for _, r := range rs {
		out[r.Category] = append(out[r.Category], r)
	}
	for k := range out {
		sort.Slice(out[k], func(i, j int) bool {
			return out[k][i].ID < out[k][j].ID
		})
	}
	return out
}

// orderedCategories returns category keys in the canonical order
// (categoryOrder first; unknown categories sorted alphabetically
// at the end).
func orderedCategories(grouped map[string][]rules.RuleInfo) []string {
	seen := make(map[string]bool, len(categoryOrder))
	result := make([]string, 0, len(grouped))
	for _, cat := range categoryOrder {
		if _, ok := grouped[cat]; ok {
			result = append(result, cat)
			seen[cat] = true
		}
	}
	var extras []string
	for cat := range grouped {
		if !seen[cat] {
			extras = append(extras, cat)
		}
	}
	sort.Strings(extras)
	result = append(result, extras...)
	return result
}

// unknownCategoryTitle renders a section heading for a
// category value that isn't in categoryTitle. An empty string
// is rendered as a loud "Uncategorized" label that points
// contributors at the missing front matter; any other unknown
// value is rendered with its first ASCII letter upper-cased.
func unknownCategoryTitle(cat string) string {
	if cat == "" {
		return "Uncategorized (category missing from rule README front matter)"
	}
	return strings.ToUpper(cat[:1]) + cat[1:]
}

// categoryIsMdsmithOnly reports whether every rule in the slice
// has zero peer-linter mappings — i.e. the section is entirely
// mdsmith-original and should render with the simpler
// "What it adds" table.
func categoryIsMdsmithOnly(rs []rules.RuleInfo) bool {
	for _, r := range rs {
		if len(r.Markdownlint) > 0 || len(r.Rumdl) > 0 ||
			len(r.Mado) > 0 || len(r.Panache) > 0 {
			return false
		}
	}
	return true
}

// listRules is a package-level seam so tests stub the rule
// loader without driving the real embed.FS. Production uses
// the real loader.
var listRules = rules.ListRules

// ApplyCoverageMatrix re-renders the coverage page from each
// rule README's front matter and writes it to disk under root.
// Returns (true, nil) when the on-disk file actually changed.
func ApplyCoverageMatrix(root string) (bool, error) {
	rs, err := listRules()
	if err != nil {
		return false, fmt.Errorf("loading rule metadata: %w", err)
	}
	want := RenderCoverageMatrix(rs)
	path := filepath.Join(root, CoverageMatrixFile)
	have, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("reading existing matrix: %w", err)
	}
	if string(have) == want {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("creating output dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
		return false, fmt.Errorf("writing coverage matrix: %w", err)
	}
	return true, nil
}

// CheckCoverageMatrix renders the expected page content from
// the rule READMEs and compares to the file on disk under root.
// Returns ("", nil) when the file is in sync, a non-empty drift
// message and nil error when the file content differs from the
// generator's output, and an empty string with a non-nil error
// when the loader or the file read fails.
func CheckCoverageMatrix(root string) (string, error) {
	rs, err := listRules()
	if err != nil {
		return "", fmt.Errorf("loading rule metadata: %w", err)
	}
	want := RenderCoverageMatrix(rs)
	path := filepath.Join(root, CoverageMatrixFile)
	have, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading existing matrix: %w", err)
	}
	if string(have) == want {
		return "", nil
	}
	return formatCoverageDrift(string(have), want), nil
}

// formatCoverageDrift surfaces the first divergent line so a CI
// failure points at where the file fell out of sync without
// dumping the whole diff. Full re-render is one command away;
// the goal here is enough signal to act on, not a code review.
func formatCoverageDrift(have, want string) string {
	hLines := strings.Split(have, "\n")
	wLines := strings.Split(want, "\n")
	n := len(hLines)
	if len(wLines) < n {
		n = len(wLines)
	}
	const hint = "run `mdsmith-release sync-coverage-matrix` to regenerate"
	for i := 0; i < n; i++ {
		if hLines[i] != wLines[i] {
			return fmt.Sprintf(
				"coverage matrix drift at line %d:\n"+
					"  on disk:  %q\n"+
					"  expected: %q\n%s",
				i+1, hLines[i], wLines[i], hint)
		}
	}
	return fmt.Sprintf(
		"coverage matrix drift: file has %d lines, expected %d\n%s",
		len(hLines), len(wLines), hint)
}
