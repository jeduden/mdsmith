package generatedsection

import (
	"bytes"
	"path/filepath"
	"strings"
	"text/template"
)

// fileEntry holds the template fields for a single matched file.
type fileEntry struct {
	fields map[string]string
}

// renderTemplate renders header + row-per-file + footer. Each section is
// terminated by \n; if the value already ends with \n, no extra is added.
func renderTemplate(params map[string]string, entries []fileEntry) (string, error) {
	var buf strings.Builder

	header := params["header"]
	row := params["row"]
	footer := params["footer"]

	if header != "" {
		buf.WriteString(ensureTrailingNewline(header))
	}

	tmpl, err := template.New("row").Option("missingkey=zero").Parse(row)
	if err != nil {
		return "", err
	}

	for _, entry := range entries {
		var rowBuf bytes.Buffer
		if err := tmpl.Execute(&rowBuf, entry.fields); err != nil {
			return "", err
		}
		buf.WriteString(ensureTrailingNewline(rowBuf.String()))
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
		path := entry.fields["filename"]
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

// ensureTrailingNewline appends \n if s does not already end with \n.
func ensureTrailingNewline(s string) string {
	if strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}
