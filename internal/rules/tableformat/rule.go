package tableformat

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jeduden/tidymark/internal/lint"
	"github.com/jeduden/tidymark/internal/rule"
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
func (r *Rule) ID() string { return "TM021" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "table-format" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "table" }

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "pad":
			n, ok := toInt(v)
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
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     tbl.startLine,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  "table is not formatted",
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

// displayWidth returns the visible width of a cell's content,
// accounting for markdown syntax that is not displayed.
func displayWidth(s string) int {
	visible := stripMarkdown(s)
	return utf8.RuneCountInString(visible)
}

// stripMarkdown removes markdown formatting syntax to get the visible text.
// Handles: images, links, bold, italic, inline code, strikethrough.
func stripMarkdown(s string) string {
	// Process inline code first (to avoid processing markdown inside code).
	s = processInlineCode(s)

	// Process images: ![alt](url) -> alt
	s = stripImages(s)

	// Process links: [text](url) -> text
	s = stripLinks(s)

	// Process bold+italic: ***text*** -> text, ___text___ -> text
	s = stripEmphasis(s, "***", "***")
	s = stripEmphasis(s, "___", "___")

	// Process bold: **text** -> text, __text__ -> text
	s = stripEmphasis(s, "**", "**")
	s = stripEmphasis(s, "__", "__")

	// Process italic: *text* -> text, _text_ -> text
	s = stripEmphasis(s, "*", "*")
	s = stripEmphasis(s, "_", "_")

	// Process strikethrough: ~~text~~ -> text
	s = stripEmphasis(s, "~~", "~~")

	return s
}

// processInlineCode handles inline code spans, returning only
// the visible text (the code content without backticks).
func processInlineCode(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '`' {
			// Count opening backticks.
			start := i
			for i < len(s) && s[i] == '`' {
				i++
			}
			ticks := s[start:i]

			// Find matching closing backticks.
			closeIdx := strings.Index(s[i:], ticks)
			if closeIdx < 0 {
				// No closing — write the backticks literally.
				result.WriteString(ticks)
				continue
			}
			// Write the code content (visible text).
			content := s[i : i+closeIdx]
			result.WriteString(content)
			i += closeIdx + len(ticks)
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}

// stripImages replaces ![alt](url) with alt.
func stripImages(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '!' && s[i+1] == '[' {
			// Find closing ]
			closeIdx := findUnescaped(s[i+2:], ']')
			if closeIdx < 0 {
				result.WriteByte(s[i])
				i++
				continue
			}
			alt := s[i+2 : i+2+closeIdx]
			afterBracket := i + 2 + closeIdx + 1
			// Expect (url) after ]
			if afterBracket < len(s) && s[afterBracket] == '(' {
				parenClose := findUnescaped(s[afterBracket+1:], ')')
				if parenClose >= 0 {
					result.WriteString(alt)
					i = afterBracket + 1 + parenClose + 1
					continue
				}
			}
			result.WriteByte(s[i])
			i++
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}

// stripLinks replaces [text](url) with text.
func stripLinks(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '[' {
			closeIdx := findUnescaped(s[i+1:], ']')
			if closeIdx < 0 {
				result.WriteByte(s[i])
				i++
				continue
			}
			text := s[i+1 : i+1+closeIdx]
			afterBracket := i + 1 + closeIdx + 1
			if afterBracket < len(s) && s[afterBracket] == '(' {
				parenClose := findUnescaped(s[afterBracket+1:], ')')
				if parenClose >= 0 {
					result.WriteString(text)
					i = afterBracket + 1 + parenClose + 1
					continue
				}
			}
			result.WriteByte(s[i])
			i++
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}

// stripEmphasis removes matched open/close markers.
func stripEmphasis(s, open, close string) string {
	var result strings.Builder
	for {
		idx := strings.Index(s, open)
		if idx < 0 {
			result.WriteString(s)
			break
		}
		result.WriteString(s[:idx])
		rest := s[idx+len(open):]
		closeIdx := strings.Index(rest, close)
		if closeIdx < 0 {
			result.WriteString(open)
			s = rest
			continue
		}
		result.WriteString(rest[:closeIdx])
		s = rest[closeIdx+len(close):]
	}
	return result.String()
}

// findUnescaped finds the first occurrence of ch in s that is not preceded
// by a backslash.
func findUnescaped(s string, ch byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == ch && (i == 0 || s[i-1] != '\\') {
			return i
		}
	}
	return -1
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

// toInt converts a value to int. Supports int and float64 (YAML decodes
// numbers as int or float64 depending on context).
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	case int64:
		return int(n), true
	}
	return 0, false
}

var _ rule.FixableRule = (*Rule)(nil)
var _ rule.Configurable = (*Rule)(nil)
