package catalog

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/cuetemplate"
	"github.com/jeduden/mdsmith/internal/fieldinterp"
)

// fileEntry holds the template fields for a single matched file.
// matchPath is the doublestar match path relative to the resolving
// globResolution's fs.FS root — kept alongside the display-form
// fields["filename"] so the include-cycle scan can open the file
// through the same fs the glob walked, even when displayPath
// rewrote the visible path to a "../..." host-relative form.
type fileEntry struct {
	fields    map[string]any
	matchPath string
}

// renderTemplate renders header + row-per-file + footer. Each section is
// terminated by \n; if the value already ends with \n, no extra is added.
// If columns config is provided, column constraints (truncation/wrapping)
// are applied to table rows after template expansion.
//
// The row may be authored as either a placeholder-style `row:`
// (resolved by fieldinterp.Interpolate) or a CUE expression
// `row-expr:` (compiled once via cuetemplate, evaluated against
// each entry's frontmatter map). validateCatalogDirective rejects
// directives that set both forms before this function runs.
func renderTemplate(params map[string]string, entries []fileEntry, columns ...map[string]columnConfig) (string, error) {
	var buf strings.Builder

	header := params["header"]
	row := params["row"]
	rowExpr := strings.TrimSpace(params["row-expr"])
	footer := params["footer"]

	// Defensive: validateCatalogDirective rejects empty
	// row/row-expr before Generate runs, but a direct caller
	// that bypasses validation would otherwise produce blank
	// rows here. Fail loudly instead.
	if strings.TrimSpace(row) == "" && rowExpr == "" {
		return "", fmt.Errorf(
			"renderTemplate called without row or row-expr")
	}

	var rowTpl *cuetemplate.Template
	if rowExpr != "" {
		var err error
		rowTpl, err = cuetemplate.Compile(rowExpr)
		if err != nil {
			return "", fmt.Errorf("compiling row-expr: %w", err)
		}
	}

	// Column constraints are keyed off the placeholder-form row; a
	// CUE-expression row carries no `{field}` columns so column
	// truncation/wrapping does not apply.
	var cols map[string]columnConfig
	var colMap map[int]string
	if rowTpl == nil && len(columns) > 0 && columns[0] != nil && len(columns[0]) > 0 {
		cols = columns[0]
		colMap = buildColumnMap(row)
	}

	if header != "" {
		buf.WriteString(ensureTrailingNewline(header))
	}

	for _, entry := range entries {
		var rendered string
		if rowTpl != nil {
			r, err := rowTpl.Render(entry.fields)
			if err != nil {
				// Display-form filename, matching the rule's other
				// per-entry diagnostics (front-matter read errors,
				// include-cycle reports).
				return "", fmt.Errorf(
					"rendering row-expr for %q: %w",
					fieldinterp.Stringify(entry.fields["filename"]), err)
			}
			rendered = r
		} else {
			rendered = fieldinterp.Interpolate(row, entry.fields)
		}

		// Apply column constraints to placeholder-form rows.
		if cols != nil && colMap != nil {
			rendered = applyColumnConstraints(rendered, cols, colMap)
		}

		buf.WriteString(ensureTrailingNewline(rendered))
	}

	if footer != "" {
		buf.WriteString(ensureTrailingNewline(footer))
	}

	return buf.String(), nil
}

// renderMinimal renders a plain bullet list with basename link text
// and relative path link targets.
//
// Two things are needed together, not either alone: Grow(total) avoids
// the O(entries) reallocations an un-Grow'n Builder pays discovering
// its final size through repeated doubling, and writing each part with
// its own WriteString call (rather than one `+`-concatenated string
// per entry) avoids a further one-allocation-per-entry cost that the
// concatenated string still incurs even once the Builder no longer
// needs to grow — confirmed with testing.AllocsPerRun, since escape
// analysis alone did not predict it. Guideline: "strings.Builder over
// +. Call Grow(n) first if you know the final size" (docs/development/
// high-performance-go.md). filepath.Base and fieldinterp.Stringify are
// cheap, allocation-free lookups for the common string-valued
// "filename" field, so computing them twice per entry to size the
// buffer costs no extra allocations.
func renderMinimal(entries []fileEntry) string {
	const linePrefix, lineMid, lineSuffix = "- [", "](", ")\n"
	total := 0
	for _, entry := range entries {
		path := fieldinterp.Stringify(entry.fields["filename"])
		basename := filepath.Base(path)
		total += len(linePrefix) + len(basename) + len(lineMid) + len(path) + len(lineSuffix)
	}
	var buf strings.Builder
	buf.Grow(total)
	for _, entry := range entries {
		path := fieldinterp.Stringify(entry.fields["filename"])
		basename := filepath.Base(path)
		buf.WriteString(linePrefix)
		buf.WriteString(basename)
		buf.WriteString(lineMid)
		buf.WriteString(path)
		buf.WriteString(lineSuffix)
	}
	return buf.String()
}

// renderEmpty renders the empty fallback text with trailing newline.
func renderEmpty(params map[string]string) string {
	empty := params["empty"]
	if empty == "" {
		return ""
	}
	return ensureTrailingNewline(empty)
}

// ensureTrailingNewline delegates to gensection.EnsureTrailingNewline.
func ensureTrailingNewline(s string) string {
	return gensection.EnsureTrailingNewline(s)
}
