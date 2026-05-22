package tablereadability

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/settings"
)

const (
	defaultMaxColumns          = 8
	defaultMaxRows             = 30
	defaultMaxWordsPerCell     = 30
	defaultMaxColumnWidthRatio = 60.0
)

func init() {
	rule.Register(&Rule{
		MaxColumns:          defaultMaxColumns,
		MaxRows:             defaultMaxRows,
		MaxWordsPerCell:     defaultMaxWordsPerCell,
		MaxColumnWidthRatio: defaultMaxColumnWidthRatio,
	})
}

// Rule checks markdown tables for readability limits.
type Rule struct {
	MaxColumns          int
	MaxRows             int
	MaxWordsPerCell     int
	MaxColumnWidthRatio float64
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS026" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "table-readability" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "table" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	// Cheap early-exit: any GFM table row contains `|`. Skipping
	// files with no pipe byte avoids the AST-walk for code lines
	// and the per-line prefix-detection pass, which together are
	// the rule's dominant per-Check allocator on table-free files
	// (the common case in most repos).
	if bytes.IndexByte(f.Source, '|') < 0 {
		return nil
	}

	maxColumns := positiveIntOrDefault(r.MaxColumns, defaultMaxColumns)
	maxRows := positiveIntOrDefault(r.MaxRows, defaultMaxRows)
	maxWordsPerCell := positiveIntOrDefault(r.MaxWordsPerCell, defaultMaxWordsPerCell)
	maxRatio := positiveFloatOrDefault(r.MaxColumnWidthRatio, defaultMaxColumnWidthRatio)

	codeLines := lint.CollectCodeBlockLines(f)
	tables := findTables(f.Lines, codeLines)
	if len(tables) == 0 {
		return nil
	}

	var diags []lint.Diagnostic
	for _, tbl := range tables {
		if cols := tbl.columnCount(); cols > maxColumns {
			diags = append(diags, makeDiag(
				f,
				tbl.startLine,
				fmt.Sprintf("table has too many columns (%d > %d)", cols, maxColumns),
			))
		}

		if rows := tbl.dataRowCount(); rows > maxRows {
			diags = append(diags, makeDiag(
				f,
				tbl.startLine,
				fmt.Sprintf("table has too many rows (%d > %d)", rows, maxRows),
			))
		}

		if words, line, col := tbl.maxCellWords(); words > maxWordsPerCell {
			msg := fmt.Sprintf("table cell has too many words (%d > %d)", words, maxWordsPerCell)
			if header := tbl.columnHeader(col); header != "" {
				msg += fmt.Sprintf(" in column %q", header)
			}
			diags = append(diags, makeDiag(
				f,
				line,
				msg,
			))
		}

		if ratio := tbl.columnWidthRatio(); ratio > maxRatio {
			diags = append(diags, makeDiag(
				f,
				tbl.startLine,
				fmt.Sprintf("table has high column width ratio (%.2f > %.2f)", ratio, maxRatio),
			))
		}
	}

	return diags
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "max-columns":
			n, ok := settings.ToInt(v)
			if !ok {
				return fmt.Errorf("table-readability: max-columns must be an integer, got %T", v)
			}
			if n <= 0 {
				return fmt.Errorf("table-readability: max-columns must be > 0, got %d", n)
			}
			r.MaxColumns = n
		case "max-rows":
			n, ok := settings.ToInt(v)
			if !ok {
				return fmt.Errorf("table-readability: max-rows must be an integer, got %T", v)
			}
			if n <= 0 {
				return fmt.Errorf("table-readability: max-rows must be > 0, got %d", n)
			}
			r.MaxRows = n
		case "max-words-per-cell":
			n, ok := settings.ToInt(v)
			if !ok {
				return fmt.Errorf("table-readability: max-words-per-cell must be an integer, got %T", v)
			}
			if n <= 0 {
				return fmt.Errorf("table-readability: max-words-per-cell must be > 0, got %d", n)
			}
			r.MaxWordsPerCell = n
		case "max-column-width-ratio":
			n, ok := settings.ToFloat(v)
			if !ok {
				return fmt.Errorf("table-readability: max-column-width-ratio must be a number, got %T", v)
			}
			if n <= 0 {
				return fmt.Errorf("table-readability: max-column-width-ratio must be > 0, got %.2f", n)
			}
			r.MaxColumnWidthRatio = n
		default:
			return fmt.Errorf("table-readability: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"max-columns":            defaultMaxColumns,
		"max-rows":               defaultMaxRows,
		"max-words-per-cell":     defaultMaxWordsPerCell,
		"max-column-width-ratio": defaultMaxColumnWidthRatio,
	}
}

func makeDiag(f *lint.File, line int, msg string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     f.Path,
		Line:     line,
		Column:   1,
		RuleID:   "MDS026",
		RuleName: "table-readability",
		Severity: lint.Warning,
		Message:  msg,
	}
}

func positiveIntOrDefault(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func positiveFloatOrDefault(value, fallback float64) float64 {
	if value <= 0 {
		return fallback
	}
	return value
}

type table struct {
	startLine int
	rows      []tableRow
}

type tableRow struct {
	line        int
	cells       []string
	isSeparator bool
}

var separatorRe = regexp.MustCompile(`^:?-+:?$`)

func findTables(lines [][]byte, codeLines map[int]bool) []table {
	var tables []table
	for i := 0; i < len(lines); {
		if codeLines[i+1] {
			i++
			continue
		}

		// Lines that cannot contain a `|` cannot start a table; the
		// guard avoids a per-line detectPrefix allocation. detectPrefix
		// returns "" for non-pipe lines anyway, but only after a
		// `string(line)` conversion that was the dominant per-Check
		// allocator on file-with-no-tables corpora before plan 195.
		if bytes.IndexByte(lines[i], '|') < 0 {
			i++
			continue
		}

		tbl, end := tryParseTable(lines, i, codeLines)
		if tbl == nil {
			i++
			continue
		}

		tables = append(tables, *tbl)
		i = end
	}
	return tables
}

func tryParseTable(lines [][]byte, start int, codeLines map[int]bool) (*table, int) {
	if start+1 >= len(lines) {
		return nil, start
	}

	prefix := detectPrefix(lines[start])
	header := stripPrefix(lines[start], prefix)
	if !isTableRow(header) {
		return nil, start
	}

	if codeLines[start+2] {
		return nil, start
	}
	separator := stripPrefix(lines[start+1], prefix)
	if !isTableRow(separator) {
		return nil, start
	}
	sepCells := splitRow(separator)
	if !isSeparatorRow(sepCells) {
		return nil, start
	}

	rows := []tableRow{
		{line: start + 1, cells: splitRow(header)},
		{line: start + 2, cells: sepCells, isSeparator: true},
	}

	end := start + 2
	for end < len(lines) {
		if codeLines[end+1] {
			break
		}
		content := stripPrefix(lines[end], prefix)
		if !isTableRow(content) {
			break
		}
		rows = append(rows, tableRow{line: end + 1, cells: splitRow(content)})
		end++
	}

	return &table{startLine: start + 1, rows: rows}, end
}

func (t table) columnCount() int {
	maxColumns := 0
	for _, row := range t.rows {
		if row.isSeparator {
			continue
		}
		if len(row.cells) > maxColumns {
			maxColumns = len(row.cells)
		}
	}
	return maxColumns
}

func (t table) dataRowCount() int {
	count := 0
	for idx, row := range t.rows {
		if idx == 0 || row.isSeparator {
			continue
		}
		count++
	}
	return count
}

func (t table) maxCellWords() (int, int, int) {
	maxWords := 0
	maxLine := t.startLine
	maxCol := 0
	for _, row := range t.rows {
		if row.isSeparator {
			continue
		}
		for col, cell := range row.cells {
			wc := len(strings.Fields(cell))
			if wc > maxWords {
				maxWords = wc
				maxLine = row.line
				maxCol = col
			}
		}
	}
	return maxWords, maxLine, maxCol
}

func (t table) columnHeader(col int) string {
	if len(t.rows) == 0 || col >= len(t.rows[0].cells) {
		return ""
	}
	return strings.TrimSpace(t.rows[0].cells[col])
}

func (t table) columnWidthRatio() float64 {
	columns := t.columnCount()
	if columns == 0 {
		return 0
	}

	sums := make([]float64, columns)
	counts := make([]float64, columns)

	for _, row := range t.rows {
		if row.isSeparator {
			continue
		}
		for col := 0; col < columns; col++ {
			cell := ""
			if col < len(row.cells) {
				cell = strings.TrimSpace(row.cells[col])
			}
			sums[col] += float64(utf8.RuneCountInString(cell))
			counts[col]++
		}
	}

	minAverage := math.MaxFloat64
	maxAverage := 0.0
	for col := 0; col < columns; col++ {
		if counts[col] == 0 {
			continue
		}
		avg := sums[col] / counts[col]
		if avg < minAverage {
			minAverage = avg
		}
		if avg > maxAverage {
			maxAverage = avg
		}
	}

	if minAverage == math.MaxFloat64 || maxAverage == 0 {
		return 0
	}
	if minAverage == 0 {
		return math.Inf(1)
	}

	return maxAverage / minAverage
}

var (
	blockquoteSpace = []byte("> ")
	blockquoteOnly  = []byte(">")
)

// detectPrefix returns the GFM-extension blockquote prefix that
// nests this line's table (e.g. `> ` or `> > `). For a non-nested
// table line it returns "" without allocating; for a nested table
// it returns the prefix as a string so stripPrefix can match by
// equality. The hot path is the non-nested case, so the fast
// shortcut (no `>` byte at start ⇒ return "") avoids the byte-
// scanner loop entirely. Pre-plan 195 the helper allocated
// `string(line)` plus a strings.Builder per call on every line in
// the file; the byte-scanner replacement allocates only when a
// blockquote prefix is actually present.
func detectPrefix(line []byte) string {
	// Skip leading ASCII space. The original code used `strings.TrimLeft`
	// for the space-prefix shape; ASCII space matches GFM's blockquote
	// indent grammar without the Unicode TrimLeft cost.
	start := 0
	for start < len(line) && line[start] == ' ' {
		start++
	}
	if start >= len(line) || line[start] != '>' {
		// Non-blockquote line. The original code's fallback returned
		// the leading-spaces prefix only when followed by `|`; this is
		// the dominant case for table rows in unnested context, so
		// preserving that string allocation costs one alloc per
		// matched table line — paid only when we then proceed to
		// parse the table.
		idx := bytes.IndexByte(line, '|')
		if idx <= 0 {
			return ""
		}
		candidate := line[:idx]
		if len(bytes.TrimSpace(candidate)) > 0 {
			return ""
		}
		return string(candidate)
	}

	// Nested-blockquote path. Walk through `> ` (or bare `>` followed
	// by `>`) segments. The accumulated prefix is rare in real docs;
	// allocating only here keeps the non-nested hot path free of any
	// per-call allocation.
	var prefix []byte
	remaining := line
	for {
		s := 0
		for s < len(remaining) && remaining[s] == ' ' {
			s++
		}
		indent := remaining[:s]
		trimmed := remaining[s:]

		switch {
		case bytes.HasPrefix(trimmed, blockquoteSpace):
			prefix = append(prefix, indent...)
			prefix = append(prefix, blockquoteSpace...)
			remaining = trimmed[2:]
		case bytes.HasPrefix(trimmed, blockquoteOnly) &&
			(len(trimmed) == 1 || trimmed[1] == '>'):
			prefix = append(prefix, indent...)
			prefix = append(prefix, '>')
			remaining = trimmed[1:]
		default:
			// Reaching the default branch means we already consumed at
			// least one blockquote segment in a prior iteration (the
			// outer guard required `line[0] == '>'`, and both inner
			// cases write to prefix), so prefix is non-empty here.
			return string(prefix)
		}
	}
}

func stripPrefix(line []byte, prefix string) []byte {
	if prefix == "" {
		return line
	}
	s := string(line)
	if !strings.HasPrefix(s, prefix) {
		return line
	}
	return []byte(s[len(prefix):])
}

func isTableRow(content []byte) bool {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) < 2 {
		return false
	}
	return trimmed[0] == '|' && trimmed[len(trimmed)-1] == '|'
}

// splitRow splits a row's interior (after outer-pipe strip and
// whitespace trim) into per-cell strings. The byte-based form
// replaces the original `strings.Builder`-driven path so that:
//
//   - the input avoids one `string(...)` conversion per row;
//   - the result slice is sized exactly from the unescaped `|`
//     count, eliminating per-row capacity grows;
//   - each cell allocates exactly one string (`string(...)` on a
//     bytes.TrimSpace sub-slice), rather than one Builder.String()
//     plus a separate strings.TrimSpace.
//
// `\|` is the GFM escape for a literal pipe inside a cell; the
// scanner preserves the backslash in the cell text so consumers
// see the input as the user wrote it.
func splitRow(row []byte) []string {
	row = bytes.TrimSpace(row)
	if len(row) > 0 && row[0] == '|' {
		row = row[1:]
	}
	if len(row) > 0 && row[len(row)-1] == '|' {
		row = row[:len(row)-1]
	}

	// Pre-size cells: count unescaped `|` separators + 1.
	cellCount := 1
	for i := 0; i < len(row); i++ {
		if row[i] == '\\' && i+1 < len(row) && row[i+1] == '|' {
			i++
			continue
		}
		if row[i] == '|' {
			cellCount++
		}
	}
	cells := make([]string, 0, cellCount)

	start := 0
	for i := 0; i <= len(row); i++ {
		if i < len(row) && row[i] == '\\' && i+1 < len(row) && row[i+1] == '|' {
			i++
			continue
		}
		if i == len(row) || row[i] == '|' {
			cells = append(cells, string(bytes.TrimSpace(row[start:i])))
			start = i + 1
		}
	}
	return cells
}

func isSeparatorRow(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		if !separatorRe.MatchString(strings.TrimSpace(cell)) {
			return false
		}
	}
	return true
}

var _ rule.Configurable = (*Rule)(nil)
