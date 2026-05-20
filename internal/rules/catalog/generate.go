package catalog

import (
	"path/filepath"
	"strings"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
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
func renderTemplate(params map[string]string, entries []fileEntry, columns ...map[string]columnConfig) (string, error) {
	var buf strings.Builder

	header := params["header"]
	row := params["row"]
	footer := params["footer"]

	// Build column map from the row template if we have column constraints.
	var cols map[string]columnConfig
	var colMap map[int]string
	if len(columns) > 0 && columns[0] != nil && len(columns[0]) > 0 {
		cols = columns[0]
		colMap = buildColumnMap(row)
	}

	if header != "" {
		buf.WriteString(ensureTrailingNewline(header))
	}

	for _, entry := range entries {
		rendered := fieldinterp.Interpolate(row, entry.fields)

		// Apply column constraints to table rows.
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
func renderMinimal(entries []fileEntry) string {
	var buf strings.Builder
	for _, entry := range entries {
		path := fieldinterp.Stringify(entry.fields["filename"])
		basename := filepath.Base(path)
		buf.WriteString("- [" + basename + "](" + path + ")\n")
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
