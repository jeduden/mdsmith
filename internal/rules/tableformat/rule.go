package tableformat

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/settings"
)

func init() {
	rule.Register(&Rule{Pad: 1})
}

// Rule checks that markdown tables are formatted with consistent
// column widths and padding (prettier-style).
type Rule struct {
	Pad int // spaces on each side of cell content
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS025" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "table-format" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "table" }

// GetPad returns the current pad setting.
func (r *Rule) GetPad() int { return r.Pad }

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "pad":
			n, ok := settings.ToInt(v)
			if !ok {
				return fmt.Errorf("table-format: pad must be an integer, got %T", v)
			}
			if n < 0 {
				return fmt.Errorf("table-format: pad must be non-negative, got %d", n)
			}
			r.Pad = n
		default:
			return fmt.Errorf("table-format: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"pad": 1,
	}
}

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	codeLines := lint.CollectCodeBlockLines(f)
	tables := findTables(f.Lines, codeLines)
	pad := r.Pad
	if pad < 0 {
		pad = 1
	}

	var diags []lint.Diagnostic
	for _, tbl := range tables {
		formatted := formatTable(tbl, pad)
		if !tableEqual(tbl, formatted) {
			msg := tableDiffMessage(tbl, formatted)
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     tbl.startLine,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  msg,
			})
		}
	}
	return diags
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	codeLines := lint.CollectCodeBlockLines(f)
	tables := findTables(f.Lines, codeLines)
	pad := r.Pad
	if pad < 0 {
		pad = 1
	}

	if len(tables) == 0 {
		result := make([]byte, len(f.Source))
		copy(result, f.Source)
		return result
	}

	// Apply replacements in reverse order to preserve offsets.
	result := make([]byte, len(f.Source))
	copy(result, f.Source)

	for i := len(tables) - 1; i >= 0; i-- {
		tbl := tables[i]
		formatted := formatTable(tbl, pad)
		if tableEqual(tbl, formatted) {
			continue
		}

		// Build replacement bytes from formatted lines.
		var replacement bytes.Buffer
		for j, line := range formatted.rawLines {
			replacement.Write(line)
			if j < len(formatted.rawLines)-1 {
				replacement.WriteByte('\n')
			}
		}

		// Build original bytes range.
		var original bytes.Buffer
		for j, line := range tbl.rawLines {
			original.Write(line)
			if j < len(tbl.rawLines)-1 {
				original.WriteByte('\n')
			}
		}

		// Find and replace in result.
		result = bytes.Replace(result, original.Bytes(), replacement.Bytes(), 1)
	}

	return result
}

// table represents a parsed markdown table with its source location.
type table struct {
	startLine int      // 1-based line number of the first row
	rawLines  [][]byte // raw source lines (including prefix)
	prefix    string   // blockquote/list prefix (e.g. "> ", "  ")
	rows      []row    // parsed rows (header, separator, data)
}

// row is a single table row with its cells.
type row struct {
	cells       []string // trimmed cell contents
	isSeparator bool     // true for the separator row (|---|---|)
	alignments  []align  // alignment per column (only for separator row)
}

// align represents column alignment in a table.
type align int

const (
	alignNone   align = iota
	alignLeft         // :---
	alignCenter       // :---:
	alignRight        // ---:
)

// separatorRe matches a table separator row cell content.
var separatorRe = regexp.MustCompile(`^:?-+:?$`)

// findTables scans file lines for contiguous table blocks, skipping
// lines inside fenced or indented code blocks.
func findTables(lines [][]byte, codeLines map[int]bool) []table {
	var tables []table
	i := 0
	for i < len(lines) {
		lineNum := i + 1 // 1-based
		if codeLines[lineNum] {
			i++
			continue
		}
		tbl, end := tryParseTable(lines, i, codeLines)
		if tbl != nil {
			tables = append(tables, *tbl)
			i = end
		} else {
			i++
		}
	}
	return tables
}

// tryParseTable attempts to parse a table starting at line index start.
// Returns the table and the index of the line after the table, or nil if
// no table starts here. A valid table must have at least a header row and
// a separator row.
func tryParseTable(lines [][]byte, start int, codeLines map[int]bool) (*table, int) {
	if start >= len(lines) {
		return nil, start
	}

	prefix := detectPrefix(lines[start])
	content := stripPrefix(lines[start], prefix)

	// First line must look like a table row.
	if !isTableRow(content) {
		return nil, start
	}

	// Need at least 2 lines (header + separator).
	if start+1 >= len(lines) || codeLines[start+2] {
		return nil, start
	}

	sepContent := stripPrefix(lines[start+1], prefix)
	if !isTableRow(sepContent) {
		return nil, start
	}

	sepCells := splitRow(string(sepContent))
	if !isSeparatorRow(sepCells) {
		return nil, start
	}

	// Collect all table rows.
	var rawLines [][]byte
	var rows []row

	// Header row.
	headerCells := splitRow(string(content))
	rawLines = append(rawLines, lines[start])
	rows = append(rows, row{cells: headerCells})

	// Separator row.
	aligns := parseAlignments(sepCells)
	rawLines = append(rawLines, lines[start+1])
	rows = append(rows, row{cells: sepCells, isSeparator: true, alignments: aligns})

	// Data rows.
	end := start + 2
	for end < len(lines) {
		if codeLines[end+1] { // end is 0-based, codeLines is 1-based
			break
		}
		rowContent := stripPrefix(lines[end], prefix)
		if !isTableRow(rowContent) {
			break
		}
		dataCells := splitRow(string(rowContent))
		rawLines = append(rawLines, lines[end])
		rows = append(rows, row{cells: dataCells})
		end++
	}

	return &table{
		startLine: start + 1, // 1-based
		rawLines:  rawLines,
		prefix:    prefix,
		rows:      rows,
	}, end
}

// detectPrefix extracts the blockquote or list prefix from a line.
func detectPrefix(line []byte) string {
	s := string(line)

	// Check for blockquote prefix: optional spaces + "> "
	// Support nested blockquotes: "> > "
	var prefix strings.Builder
	remaining := s
	for {
		trimmed := strings.TrimLeft(remaining, " ")
		indent := remaining[:len(remaining)-len(trimmed)]
		if strings.HasPrefix(trimmed, "> ") {
			prefix.WriteString(indent)
			prefix.WriteString("> ")
			remaining = trimmed[2:]
			continue
		}
		if strings.HasPrefix(trimmed, ">") && (len(trimmed) == 1 || trimmed[1] == '>') {
			prefix.WriteString(indent)
			prefix.WriteString(">")
			remaining = trimmed[1:]
			continue
		}
		break
	}
	if prefix.Len() > 0 {
		return prefix.String()
	}

	// Check for list item indentation (spaces only before a |).
	idx := strings.Index(s, "|")
	if idx > 0 {
		potentialPrefix := s[:idx]
		if strings.TrimSpace(potentialPrefix) == "" {
			return potentialPrefix
		}
	}

	return ""
}

// stripPrefix removes the detected prefix from a line.
func stripPrefix(line []byte, prefix string) []byte {
	if prefix == "" {
		return line
	}
	s := string(line)
	if strings.HasPrefix(s, prefix) {
		return []byte(s[len(prefix):])
	}
	return line
}

// isTableRow returns true if content looks like a table row (starts and
// ends with a pipe character, allowing trailing whitespace).
func isTableRow(content []byte) bool {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) < 2 {
		return false
	}
	return trimmed[0] == '|' && trimmed[len(trimmed)-1] == '|'
}

// splitRow splits a table row into cell contents. Leading and trailing
// pipes are removed. Escaped pipes (\|) inside cells are preserved.
func splitRow(row string) []string {
	row = strings.TrimSpace(row)

	// Remove leading and trailing pipe.
	if len(row) > 0 && row[0] == '|' {
		row = row[1:]
	}
	if len(row) > 0 && row[len(row)-1] == '|' {
		row = row[:len(row)-1]
	}

	// Split on unescaped pipes.
	var cells []string
	var current strings.Builder
	for i := 0; i < len(row); i++ {
		if row[i] == '\\' && i+1 < len(row) && row[i+1] == '|' {
			current.WriteString(`\|`)
			i++ // skip the pipe
			continue
		}
		if row[i] == '|' {
			cells = append(cells, strings.TrimSpace(current.String()))
			current.Reset()
			continue
		}
		current.WriteByte(row[i])
	}
	cells = append(cells, strings.TrimSpace(current.String()))

	return cells
}

// isSeparatorRow returns true if all cells match the separator pattern.
func isSeparatorRow(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		if !separatorRe.MatchString(cell) {
			return false
		}
	}
	return true
}

// parseAlignments extracts alignment from separator row cells.
func parseAlignments(cells []string) []align {
	aligns := make([]align, len(cells))
	for i, cell := range cells {
		cell = strings.TrimSpace(cell)
		left := strings.HasPrefix(cell, ":")
		right := strings.HasSuffix(cell, ":")
		switch {
		case left && right:
			aligns[i] = alignCenter
		case right:
			aligns[i] = alignRight
		case left:
			aligns[i] = alignLeft
		default:
			aligns[i] = alignNone
		}
	}
	return aligns
}

// displayWidth returns the raw display width of a cell's content
// in a monospace terminal/editor, accounting for wide Unicode
// characters (emoji, CJK) but preserving markdown syntax as-is
// so that column delimiters align in source text.
func displayWidth(s string) int {
	return runewidth.StringWidth(s)
}

// formatTable produces a formatted version of a table with aligned columns.
func formatTable(tbl table, pad int) table {
	if len(tbl.rows) < 2 {
		return tbl
	}

	numCols := len(tbl.rows[0].cells)
	normalizedRows := normalizeRows(tbl.rows, numCols)
	colWidths := computeColWidths(normalizedRows, numCols)
	padding := strings.Repeat(" ", pad)

	var formattedLines [][]byte
	var formattedRows []row
	for _, r := range normalizedRows {
		var line strings.Builder
		line.WriteString(tbl.prefix)
		line.WriteByte('|')
		if r.isSeparator {
			writeSeparatorRow(&line, r.alignments, colWidths, numCols, pad)
		} else {
			writeDataRow(&line, r, colWidths, numCols, padding)
		}
		formattedLines = append(formattedLines, []byte(line.String()))
		formattedRows = append(formattedRows, r)
	}

	return table{
		startLine: tbl.startLine,
		rawLines:  formattedLines,
		prefix:    tbl.prefix,
		rows:      formattedRows,
	}
}

// normalizeRows ensures all rows have exactly numCols cells.
func normalizeRows(rows []row, numCols int) []row {
	out := make([]row, len(rows))
	for i, r := range rows {
		cells := make([]string, numCols)
		copy(cells, r.cells)
		out[i] = row{
			cells:       cells,
			isSeparator: r.isSeparator,
			alignments:  r.alignments,
		}
	}
	return out
}

// computeColWidths returns the max display width per column, with a
// minimum of 3 (to fit separator dashes).
func computeColWidths(rows []row, numCols int) []int {
	widths := make([]int, numCols)
	for _, r := range rows {
		if r.isSeparator {
			continue
		}
		for j := 0; j < numCols && j < len(r.cells); j++ {
			if w := displayWidth(r.cells[j]); w > widths[j] {
				widths[j] = w
			}
		}
	}
	for j := range widths {
		if widths[j] < 3 {
			widths[j] = 3
		}
	}
	return widths
}

// writeSeparatorRow writes the separator row dashes into line.
func writeSeparatorRow(line *strings.Builder, aligns []align, colWidths []int, numCols, pad int) {
	// Extend alignments to match column count.
	for len(aligns) < numCols {
		aligns = append(aligns, alignNone)
	}
	for j := 0; j < numCols; j++ {
		totalWidth := colWidths[j] + pad*2
		switch aligns[j] {
		case alignLeft:
			line.WriteByte(':')
			line.WriteString(strings.Repeat("-", totalWidth-1))
		case alignRight:
			line.WriteString(strings.Repeat("-", totalWidth-1))
			line.WriteByte(':')
		case alignCenter:
			line.WriteByte(':')
			line.WriteString(strings.Repeat("-", totalWidth-2))
			line.WriteByte(':')
		default:
			line.WriteString(strings.Repeat("-", totalWidth))
		}
		line.WriteByte('|')
	}
}

// writeDataRow writes a data row with padded cells into line.
func writeDataRow(line *strings.Builder, r row, colWidths []int, numCols int, padding string) {
	for j := 0; j < numCols; j++ {
		line.WriteString(padding)
		cell := ""
		if j < len(r.cells) {
			cell = r.cells[j]
		}
		w := displayWidth(cell)
		line.WriteString(cell)
		line.WriteString(strings.Repeat(" ", colWidths[j]-w))
		line.WriteString(padding)
		line.WriteByte('|')
	}
}

// tableDiffMessage builds a diagnostic message that includes the first
// row that differs between the original and formatted table, so the user
// can see what the expected formatting looks like.
func tableDiffMessage(original, formatted table) string {
	for i := range original.rawLines {
		if i >= len(formatted.rawLines) {
			break
		}
		if !bytes.Equal(original.rawLines[i], formatted.rawLines[i]) {
			return fmt.Sprintf(
				"table is not formatted; row %d: expected %q",
				i+1, string(formatted.rawLines[i]),
			)
		}
	}
	return "table is not formatted"
}

// tableEqual compares two tables line by line.
func tableEqual(a, b table) bool {
	if len(a.rawLines) != len(b.rawLines) {
		return false
	}
	for i := range a.rawLines {
		if !bytes.Equal(a.rawLines[i], b.rawLines[i]) {
			return false
		}
	}
	return true
}

// FormatString formats all markdown tables in s with the given padding
// and returns the result. This is used by other rules (e.g. MDS019) that
// generate table content and need it to comply with MDS025.
func FormatString(s string, pad int) string {
	source := []byte(s)
	lines := bytes.Split(source, []byte("\n"))
	tables := findTables(lines, nil)
	if len(tables) == 0 {
		return s
	}

	result := make([]byte, len(source))
	copy(result, source)

	for i := len(tables) - 1; i >= 0; i-- {
		tbl := tables[i]
		formatted := formatTable(tbl, pad)
		if tableEqual(tbl, formatted) {
			continue
		}

		var replacement bytes.Buffer
		for j, line := range formatted.rawLines {
			replacement.Write(line)
			if j < len(formatted.rawLines)-1 {
				replacement.WriteByte('\n')
			}
		}

		var original bytes.Buffer
		for j, line := range tbl.rawLines {
			original.Write(line)
			if j < len(tbl.rawLines)-1 {
				original.WriteByte('\n')
			}
		}

		result = bytes.Replace(result, original.Bytes(), replacement.Bytes(), 1)
	}

	return string(result)
}

var _ rule.FixableRule = (*Rule)(nil)
var _ rule.Configurable = (*Rule)(nil)
