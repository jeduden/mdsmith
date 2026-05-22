// Package tablefmt answers "what does the canonical aligned form of a
// pipe-table look like for these source lines?" Callers ask the package
// to format a string of markdown, to spot non-conforming tables in a
// parsed line list, or to rewrite the source bytes in place.
package tablefmt

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/mattn/go-runewidth"
)

// Violation describes a single table whose source formatting differs
// from the canonical layout produced by this package.
type Violation struct {
	StartLine int    // 1-based line number of the table's first row
	Message   string // diagnostic message including the first differing row
}

// Config controls how tables are formatted.
//
// Pad is the number of spaces on each side of cell content. Values
// below 0 fall back to 1.
//
// SeparatorStyle picks how the separator row is rendered: SeparatorSpaced
// writes `| --- | --- |` (the form spelled out by the GFM specification
// example), SeparatorCompact writes `|---|---|`. The zero value is
// SeparatorSpaced so a caller that builds a Config{Pad: 1} without
// touching the style field gets the spec-leaning default.
type Config struct {
	Pad            int
	SeparatorStyle SeparatorStyle
}

// SeparatorStyle selects the rendering of the separator row.
type SeparatorStyle int

const (
	// SeparatorSpaced writes `| --- | --- |`. Zero value so the
	// default-constructed Config picks this layout.
	SeparatorSpaced SeparatorStyle = iota
	// SeparatorCompact writes `|---|---|`. Dashes fill the cell area
	// with no whitespace around them.
	SeparatorCompact
)

// ParseSeparatorStyle converts a config value (typically a YAML scalar)
// to a SeparatorStyle, returning a rule-scoped error if v is not one
// of the supported string forms. ruleName is the rule's config key
// (e.g. "table-format", "catalog") and prefixes the error so multiple
// rules can share this helper without losing diagnostic context.
func ParseSeparatorStyle(v any, ruleName string) (SeparatorStyle, error) {
	s, ok := v.(string)
	if !ok {
		return 0, fmt.Errorf("%s: separator-style must be a string, got %T", ruleName, v)
	}
	switch s {
	case "spaced":
		return SeparatorSpaced, nil
	case "compact":
		return SeparatorCompact, nil
	default:
		return 0, fmt.Errorf("%s: separator-style must be %q or %q, got %q", ruleName, "spaced", "compact", s)
	}
}

// FormatString formats all markdown tables in s with the given padding
// and returns the result. Padding less than 0 falls back to 1 (one space
// of padding on each side of cell content). Uses the default spaced
// separator style; callers that need to choose a style call
// FormatStringWithConfig instead.
func FormatString(s string, pad int) string {
	return FormatStringWithConfig(s, Config{Pad: pad})
}

// FormatStringWithConfig formats all markdown tables in s with cfg.
func FormatStringWithConfig(s string, cfg Config) string {
	source := []byte(s)
	lines := bytes.Split(source, []byte("\n"))
	tables := findTables(lines, nil)
	if len(tables) == 0 {
		return s
	}
	return string(rebuildWithFormattedTables(lines, tables, normalizeConfig(cfg)))
}

// Violations returns the formatting violations found in lines. codeLines
// maps 1-based line numbers known to sit inside a fenced or indented
// code block; those lines are skipped.
func Violations(lines [][]byte, codeLines map[int]bool, cfg Config) []Violation {
	tables := findTables(lines, codeLines)
	cfg = normalizeConfig(cfg)

	var out []Violation
	for _, tbl := range tables {
		formatted := formatTable(tbl, cfg)
		if tableEqual(tbl, formatted) {
			continue
		}
		out = append(out, Violation{
			StartLine: tbl.startLine,
			Message:   tableDiffMessage(tbl, formatted),
		})
	}
	return out
}

// FormatLines rewrites every table found in source with canonical
// formatting, preserving everything else. lines must be the result of
// splitting source on newlines (i.e. f.Lines from internal/lint).
func FormatLines(source []byte, lines [][]byte, codeLines map[int]bool, cfg Config) []byte {
	tables := findTables(lines, codeLines)
	if len(tables) == 0 {
		out := make([]byte, len(source))
		copy(out, source)
		return out
	}
	return rebuildWithFormattedTables(lines, tables, normalizeConfig(cfg))
}

// rebuildWithFormattedTables returns the source bytes implied by lines,
// with each non-conforming table replaced by its canonical layout.
//
// Splice by line index (not bytes.Replace) so identical tables earlier
// in the file — including table-shaped text inside skipped code blocks
// — do not get rewritten in place of the parsed target. formatTable
// preserves row count, so each replacement is one-line-per-line.
//
// CRLF preservation: lint.NewFile keeps the trailing `\r` on each line
// of a CRLF document. formatTable rebuilds rewritten rows without `\r`,
// so a naive splice mixes endings (rewritten rows bare-LF, surrounding
// rows CRLF). When any source line carries a trailing `\r`, re-append
// it to every formatted row.
func rebuildWithFormattedTables(lines [][]byte, tables []table, cfg Config) []byte {
	work := make([][]byte, len(lines))
	copy(work, lines)

	crlf := false
	for _, l := range lines {
		if len(l) > 0 && l[len(l)-1] == '\r' {
			crlf = true
			break
		}
	}

	for _, tbl := range tables {
		formatted := formatTable(tbl, cfg)
		if tableEqual(tbl, formatted) {
			continue
		}
		start := tbl.startLine - 1 // 0-based
		for j, newLine := range formatted.rawLines {
			if crlf {
				newLine = append(newLine, '\r')
			}
			work[start+j] = newLine
		}
	}

	return bytes.Join(work, []byte("\n"))
}

func normalizeConfig(cfg Config) Config {
	if cfg.Pad < 0 {
		cfg.Pad = 1
	}
	return cfg
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
// lines inside fenced or indented code blocks. The byte-check
// shortcut (`bytes.IndexByte(line, '|') < 0`) avoids a tryParseTable
// call — and the `string(line)` allocation that detectPrefix would
// otherwise pay — on every non-pipe line. On real corpora most
// lines have no `|`, so the per-Check overhead drops to the cost of
// scanning the lines that could plausibly start a table.
func findTables(lines [][]byte, codeLines map[int]bool) []table {
	var tables []table
	i := 0
	for i < len(lines) {
		lineNum := i + 1 // 1-based
		if codeLines[lineNum] {
			i++
			continue
		}
		if bytes.IndexByte(lines[i], '|') < 0 {
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
func formatTable(tbl table, cfg Config) table {
	if len(tbl.rows) < 2 {
		return tbl
	}

	numCols := len(tbl.rows[0].cells)
	normalizedRows := normalizeRows(tbl.rows, numCols)
	aligns := separatorAlignments(normalizedRows)
	colWidths := computeColWidths(normalizedRows, numCols, aligns, cfg)
	padding := strings.Repeat(" ", cfg.Pad)

	var formattedLines [][]byte
	var formattedRows []row
	for _, r := range normalizedRows {
		var line strings.Builder
		line.WriteString(tbl.prefix)
		line.WriteByte('|')
		if r.isSeparator {
			writeSeparatorRow(&line, r.alignments, colWidths, numCols, cfg)
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

// separatorAlignments returns the per-column alignment list taken
// from the first separator row, or nil if the table has none.
func separatorAlignments(rows []row) []align {
	for _, r := range rows {
		if r.isSeparator {
			return r.alignments
		}
	}
	return nil
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

// computeColWidths returns the max display width per column. Each
// column is widened to fit a separator with at least three hyphens for
// its alignment — :--- / ---: / :---: — which is the cross-flavor floor
// (markdown-it, pandoc, and others reject :--, --:, :-:). Without
// aligns the table is treated as alignNone and the floor is three
// hyphens per cell.
func computeColWidths(rows []row, numCols int, aligns []align, cfg Config) []int {
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
		a := alignNone
		if j < len(aligns) {
			a = aligns[j]
		}
		if m := minSeparatorWidth(a, cfg); widths[j] < m {
			widths[j] = m
		}
	}
	return widths
}

// minSeparatorWidth returns the smallest colWidth that lets
// writeSeparatorRow render the cell with at least three hyphens for
// the given alignment under cfg.
//
// Spaced style writes exactly colWidth characters of separator content
// per cell. Compact style writes colWidth + 2*cfg.Pad characters,
// because pad spaces are absorbed into the dash run. The result is
// also clamped to 3 — the original min — so previously-compact tables
// keep their column widths.
func minSeparatorWidth(a align, cfg Config) int {
	const dashesNeeded = 3
	var cellChars int
	switch a {
	case alignLeft, alignRight:
		cellChars = dashesNeeded + 1
	case alignCenter:
		cellChars = dashesNeeded + 2
	default:
		cellChars = dashesNeeded
	}
	if cfg.SeparatorStyle == SeparatorCompact {
		cellChars -= 2 * cfg.Pad
	}
	if cellChars < 3 {
		cellChars = 3
	}
	return cellChars
}

// writeSeparatorRow writes the separator row dashes into line.
//
// SeparatorSpaced (default): `| --- | --- |`, with `pad` spaces between
// the pipe and the dashes/colons on each side. Cells in the alignment
// arms still place the colon at the edge of the content area:
// `| :--- | ---: | :---: |`.
//
// SeparatorCompact: `|---|---|` — dashes fill the cell area, alignment
// colons sit flush against the pipes.
func writeSeparatorRow(line *strings.Builder, aligns []align, colWidths []int, numCols int, cfg Config) {
	for len(aligns) < numCols {
		aligns = append(aligns, alignNone)
	}
	padding := strings.Repeat(" ", cfg.Pad)
	for j := 0; j < numCols; j++ {
		switch cfg.SeparatorStyle {
		case SeparatorCompact:
			writeCompactSeparatorCell(line, aligns[j], colWidths[j], cfg.Pad)
		default:
			writeSpacedSeparatorCell(line, aligns[j], colWidths[j], padding)
		}
		line.WriteByte('|')
	}
}

// writeSpacedSeparatorCell renders a cell whose dashes are wrapped in
// `padding` spaces, leaving room for an alignment colon at each edge of
// the dash run.
func writeSpacedSeparatorCell(line *strings.Builder, a align, colWidth int, padding string) {
	line.WriteString(padding)
	switch a {
	case alignLeft:
		line.WriteByte(':')
		line.WriteString(strings.Repeat("-", colWidth-1))
	case alignRight:
		line.WriteString(strings.Repeat("-", colWidth-1))
		line.WriteByte(':')
	case alignCenter:
		line.WriteByte(':')
		line.WriteString(strings.Repeat("-", colWidth-2))
		line.WriteByte(':')
	default:
		line.WriteString(strings.Repeat("-", colWidth))
	}
	line.WriteString(padding)
}

// writeCompactSeparatorCell renders a cell whose dashes fill the entire
// cell width (pad spaces are absorbed into the dash run).
func writeCompactSeparatorCell(line *strings.Builder, a align, colWidth, pad int) {
	totalWidth := colWidth + pad*2
	switch a {
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
