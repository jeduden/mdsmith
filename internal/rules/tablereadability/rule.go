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
)

const (
	defaultMaxColumns             = 8
	defaultMaxRows                = 30
	defaultMaxWordsPerCell        = 30
	defaultMaxColumnWidthVariance = 60.0
)

func init() {
	rule.Register(&Rule{
		MaxColumns:             defaultMaxColumns,
		MaxRows:                defaultMaxRows,
		MaxWordsPerCell:        defaultMaxWordsPerCell,
		MaxColumnWidthVariance: defaultMaxColumnWidthVariance,
	})
}

// Rule checks markdown tables for readability limits.
type Rule struct {
	MaxColumns             int
	MaxRows                int
	MaxWordsPerCell        int
	MaxColumnWidthVariance float64
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS026" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "table-readability" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "table" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	maxColumns := positiveIntOrDefault(r.MaxColumns, defaultMaxColumns)
	maxRows := positiveIntOrDefault(r.MaxRows, defaultMaxRows)
	maxWordsPerCell := positiveIntOrDefault(r.MaxWordsPerCell, defaultMaxWordsPerCell)
	maxVariance := positiveFloatOrDefault(r.MaxColumnWidthVariance, defaultMaxColumnWidthVariance)

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

		if words, line := tbl.maxCellWords(); words > maxWordsPerCell {
			diags = append(diags, makeDiag(
				f,
				line,
				fmt.Sprintf("table cell has too many words (%d > %d)", words, maxWordsPerCell),
			))
		}

		if variance := tbl.columnWidthVariance(); variance > maxVariance {
			diags = append(diags, makeDiag(
				f,
				tbl.startLine,
				fmt.Sprintf("table has high column width variance (%.2f > %.2f)", variance, maxVariance),
			))
		}
	}

	return diags
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "max-columns":
			n, ok := toInt(v)
			if !ok {
				return fmt.Errorf("table-readability: max-columns must be an integer, got %T", v)
			}
			if n <= 0 {
				return fmt.Errorf("table-readability: max-columns must be > 0, got %d", n)
			}
			r.MaxColumns = n
		case "max-rows":
			n, ok := toInt(v)
			if !ok {
				return fmt.Errorf("table-readability: max-rows must be an integer, got %T", v)
			}
			if n <= 0 {
				return fmt.Errorf("table-readability: max-rows must be > 0, got %d", n)
			}
			r.MaxRows = n
		case "max-words-per-cell":
			n, ok := toInt(v)
			if !ok {
				return fmt.Errorf("table-readability: max-words-per-cell must be an integer, got %T", v)
			}
			if n <= 0 {
				return fmt.Errorf("table-readability: max-words-per-cell must be > 0, got %d", n)
			}
			r.MaxWordsPerCell = n
		case "max-column-width-variance":
			n, ok := toFloat(v)
			if !ok {
				return fmt.Errorf("table-readability: max-column-width-variance must be a number, got %T", v)
			}
			if n <= 0 {
				return fmt.Errorf("table-readability: max-column-width-variance must be > 0, got %.2f", n)
			}
			r.MaxColumnWidthVariance = n
		default:
			return fmt.Errorf("table-readability: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"max-columns":               defaultMaxColumns,
		"max-rows":                  defaultMaxRows,
		"max-words-per-cell":        defaultMaxWordsPerCell,
		"max-column-width-variance": defaultMaxColumnWidthVariance,
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
	sepCells := splitRow(string(separator))
	if !isSeparatorRow(sepCells) {
		return nil, start
	}

	rows := []tableRow{
		{line: start + 1, cells: splitRow(string(header))},
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
		rows = append(rows, tableRow{line: end + 1, cells: splitRow(string(content))})
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

func (t table) maxCellWords() (int, int) {
	maxWords := 0
	maxLine := t.startLine
	for _, row := range t.rows {
		if row.isSeparator {
			continue
		}
		for _, cell := range row.cells {
			wc := len(strings.Fields(cell))
			if wc > maxWords {
				maxWords = wc
				maxLine = row.line
			}
		}
	}
	return maxWords, maxLine
}

func (t table) columnWidthVariance() float64 {
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

func detectPrefix(line []byte) string {
	s := string(line)

	var prefix strings.Builder
	remaining := s
	for {
		trimmed := strings.TrimLeft(remaining, " ")
		indent := remaining[:len(remaining)-len(trimmed)]

		switch {
		case strings.HasPrefix(trimmed, "> "):
			prefix.WriteString(indent)
			prefix.WriteString("> ")
			remaining = trimmed[2:]
		case strings.HasPrefix(trimmed, ">") && (len(trimmed) == 1 || trimmed[1] == '>'):
			prefix.WriteString(indent)
			prefix.WriteString(">")
			remaining = trimmed[1:]
		default:
			if prefix.Len() > 0 {
				return prefix.String()
			}
			idx := strings.Index(s, "|")
			if idx <= 0 {
				return ""
			}
			candidate := s[:idx]
			if strings.TrimSpace(candidate) == "" {
				return candidate
			}
			return ""
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

func splitRow(row string) []string {
	row = strings.TrimSpace(row)

	if len(row) > 0 && row[0] == '|' {
		row = row[1:]
	}
	if len(row) > 0 && row[len(row)-1] == '|' {
		row = row[:len(row)-1]
	}

	var cells []string
	var current strings.Builder
	for i := 0; i < len(row); i++ {
		if row[i] == '\\' && i+1 < len(row) && row[i+1] == '|' {
			current.WriteString(`\\|`)
			i++
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

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

var _ rule.Configurable = (*Rule)(nil)
