package tablereadability

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"strconv"
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
				"table has too many columns ("+strconv.Itoa(cols)+" > "+strconv.Itoa(maxColumns)+")",
			))
		}

		if rows := tbl.dataRowCount(); rows > maxRows {
			diags = append(diags, makeDiag(
				f,
				tbl.startLine,
				"table has too many rows ("+strconv.Itoa(rows)+" > "+strconv.Itoa(maxRows)+")",
			))
		}

		if words, line, col := tbl.maxCellWords(); words > maxWordsPerCell {
			msg := "table cell has too many words (" + strconv.Itoa(words) + " > " + strconv.Itoa(maxWordsPerCell) + ")"
			if header := tbl.columnHeader(col); header != "" {
				msg += " in column " + strconv.Quote(header)
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

// tableRow holds one parsed source row. cells are sub-slices into
// the source line (after outer-pipe strip and trim), so building a
// row pays one slice allocation rather than one per cell — the
// dominant per-Check allocator on table-bearing files (plan 195
// task 2). Consumers convert to string only when emitting a
// diagnostic message, never on the hot count/width paths.
type tableRow struct {
	line        int
	cells       [][]byte
	isSeparator bool
}

var separatorRe = regexp.MustCompile(`^:?-+:?$`)

func findTables(lines [][]byte, codeLines map[int]struct{}) []table {
	var tables []table
	for i := 0; i < len(lines); {
		if _, ok := codeLines[i+1]; ok {
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

func tryParseTable(lines [][]byte, start int, codeLines map[int]struct{}) (*table, int) {
	if start+1 >= len(lines) {
		return nil, start
	}

	prefix := detectPrefix(lines[start])
	header := stripPrefix(lines[start], prefix)
	if !isTableRow(header) {
		return nil, start
	}

	if _, ok := codeLines[start+2]; ok {
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
		if _, ok := codeLines[end+1]; ok {
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
			wc := countWords(cell)
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
	return string(bytes.TrimSpace(t.rows[0].cells[col]))
}

// countWords returns the count of whitespace-delimited fields in b
// without allocating. Returns the same number as
// `len(strings.Fields(string(b)))` but scans bytes directly so it
// stays on the per-Check alloc budget. Pure ASCII text takes the
// fast path via asciiSpace; multibyte runes decode via
// utf8.DecodeRune and look up the local isUnicodeSpace table (see
// that function for the enumerated whitespace runes). The local
// table avoids pulling in the unicode package for a path the rule
// only hits on rare multi-byte cells.
func countWords(b []byte) int {
	words := 0
	inWord := false
	for i := 0; i < len(b); {
		c := b[i]
		var size int
		isSpace := false
		switch {
		case c < utf8.RuneSelf:
			size = 1
			isSpace = asciiSpace(c)
		default:
			r, sz := utf8.DecodeRune(b[i:])
			size = sz
			isSpace = isUnicodeSpace(r)
		}
		if isSpace {
			inWord = false
		} else if !inWord {
			words++
			inWord = true
		}
		i += size
	}
	return words
}

func asciiSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\v' || b == '\f' || b == '\r'
}

// isUnicodeSpace mirrors unicode.IsSpace for the runes strings.Fields
// treats as whitespace. Inlined to avoid importing unicode just for
// the rare multi-byte cell.
func isUnicodeSpace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	case 0x85, 0xA0, 0x1680:
		return true
	}
	if r >= 0x2000 && r <= 0x200A {
		return true
	}
	switch r {
	case 0x2028, 0x2029, 0x202F, 0x205F, 0x3000:
		return true
	}
	return false
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
			var cell []byte
			if col < len(row.cells) {
				cell = bytes.TrimSpace(row.cells[col])
			}
			sums[col] += float64(utf8.RuneCount(cell))
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

// detectPrefix returns the prefix that nests this line's table —
// blockquote (`> ` or `> > `), bare leading indentation (`  `
// before a `|`), or empty string for unindented non-blockquote
// lines. The unindented non-blockquote case is the hot path and
// allocates 0: the fast shortcut at the top returns "" before
// the byte-scanner loop runs. Other cases allocate one string
// per call:
//
//   - Blockquote-nested lines allocate from `prefix []byte`
//     accumulated across iterations.
//   - Indented non-blockquote lines (e.g. `  | a | b |`) allocate
//     for the leading-indentation prefix returned via
//     `string(candidate)`.
//
// Pre-plan-195 the helper allocated `string(line)` plus a
// strings.Builder per call on every line. The byte-scanner
// replacement keeps the unindented non-blockquote case
// allocation-free; indented and nested table lines still
// allocate one prefix string each.
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
			// outer guard required `line[start] == '>'` after skipping
			// leading ASCII spaces, and both inner cases write to
			// prefix), so prefix is non-empty here.
			return string(prefix)
		}
	}
}

func stripPrefix(line []byte, prefix string) []byte {
	if prefix == "" {
		return line
	}
	if len(line) < len(prefix) {
		return line
	}
	for i := 0; i < len(prefix); i++ {
		if line[i] != prefix[i] {
			return line
		}
	}
	return line[len(prefix):]
}

func isTableRow(content []byte) bool {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) < 2 {
		return false
	}
	return trimmed[0] == '|' && trimmed[len(trimmed)-1] == '|'
}

// splitRow splits a row's interior (after outer-pipe strip and
// whitespace trim) into per-cell byte slices. Each cell is a
// sub-slice of row, so the per-row cost is one slice header
// allocation regardless of cell count — the cell bytes themselves
// alias into the source line. The pre-pass sizes the cells slice
// exactly from the unescaped `|` count so the result slice never
// grows.
//
// `\|` is the GFM escape for a literal pipe inside a cell; the
// scanner preserves the backslash in the cell text so consumers
// see the input as the user wrote it.
func splitRow(row []byte) [][]byte {
	row = bytes.TrimSpace(row)
	if len(row) > 0 && row[0] == '|' {
		row = row[1:]
	}
	if len(row) > 0 && row[len(row)-1] == '|' {
		row = row[:len(row)-1]
	}

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
	cells := make([][]byte, 0, cellCount)

	start := 0
	for i := 0; i <= len(row); i++ {
		if i < len(row) && row[i] == '\\' && i+1 < len(row) && row[i+1] == '|' {
			i++
			continue
		}
		if i == len(row) || row[i] == '|' {
			cells = append(cells, bytes.TrimSpace(row[start:i]))
			start = i + 1
		}
	}
	return cells
}

func isSeparatorRow(cells [][]byte) bool {
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		if !separatorRe.Match(bytes.TrimSpace(cell)) {
			return false
		}
	}
	return true
}

var _ rule.Configurable = (*Rule)(nil)
