// Package tablestructure implements MDS060, which gates GFM table
// well-formedness without reformatting cell padding (that is MDS025's
// job). It covers three markdownlint rules:
//
//   - MD055 table-pipe-style: every row's leading/trailing pipe
//     presence must match the configured style. Autofixed.
//   - MD056 table-column-count: every row must have the same cell
//     count as the header. Flagged only — a missing cell's content
//     is unknown, so it is never auto-rewritten.
//   - MD058 blanks-around-tables: a table needs a blank line before
//     and after it. Autofixed by inserting the blank line.
package tablestructure

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

func init() {
	rule.Register(&Rule{Style: StyleConsistent})
}

// Pipe-style values for the MD055 `style` setting.
const (
	// StyleConsistent infers the required edge-pipe shape from the
	// table's header row and holds every other row to it.
	StyleConsistent = "consistent"
	// StyleLeadingAndTrailing requires a leading and a trailing pipe
	// on every row.
	StyleLeadingAndTrailing = "leading_and_trailing"
	// StyleNoLeadingOrTrailing forbids leading and trailing pipes on
	// every row.
	StyleNoLeadingOrTrailing = "no_leading_or_trailing"
)

// Rule checks GFM table pipe style, column count, and surrounding
// blank lines.
type Rule struct {
	// Style is the MD055 pipe style: one of the Style* constants.
	Style string
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS060" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "table-structure" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "table" }

// sepCellRe matches one delimiter-row cell (e.g. `---`, `:--`, `:-:`).
var sepCellRe = regexp.MustCompile(`^:?-+:?$`)

// tableRow is one parsed source line belonging to a table.
type tableRow struct {
	lineNum  int  // 1-based line number in f.Lines
	leading  bool // content (prefix stripped) begins with '|'
	trailing bool // content (prefix stripped) ends with '|'
	cells    int  // logical cell count
}

// tableBlock is a contiguous detected GFM table.
type tableBlock struct {
	prefix string // shared leading-whitespace prefix
	rows   []tableRow
}

func (t tableBlock) start() int { return t.rows[0].lineNum }
func (t tableBlock) end() int   { return t.rows[len(t.rows)-1].lineNum }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	skip := r.skipFunc(f)
	tables := findTables(f.Lines, skip)
	var diags []lint.Diagnostic
	for _, t := range tables {
		diags = append(diags, r.checkPipeStyle(f, t)...)
		diags = append(diags, r.checkColumnCount(f, t)...)
		diags = append(diags, r.checkSurroundingBlanks(f, t)...)
	}
	sort.SliceStable(diags, func(i, j int) bool {
		if diags[i].Line != diags[j].Line {
			return diags[i].Line < diags[j].Line
		}
		return diags[i].Column < diags[j].Column
	})
	return diags
}

// expectedStyle returns the required (leading, trailing) edge-pipe
// presence for table t under the configured style.
func (r *Rule) expectedStyle(t tableBlock) (bool, bool) {
	switch r.Style {
	case StyleLeadingAndTrailing:
		return true, true
	case StyleNoLeadingOrTrailing:
		return false, false
	default: // StyleConsistent: infer from the header row.
		return t.rows[0].leading, t.rows[0].trailing
	}
}

func (r *Rule) checkPipeStyle(f *lint.File, t tableBlock) []lint.Diagnostic {
	wantLead, wantTrail := r.expectedStyle(t)
	var diags []lint.Diagnostic
	for _, row := range t.rows {
		if row.leading == wantLead && row.trailing == wantTrail {
			continue
		}
		diags = append(diags, r.diag(f, row.lineNum, 1,
			"table pipe style; expected "+describeStyle(wantLead, wantTrail)))
	}
	return diags
}

func (r *Rule) checkColumnCount(f *lint.File, t tableBlock) []lint.Diagnostic {
	want := t.rows[0].cells
	var diags []lint.Diagnostic
	for _, row := range t.rows[1:] {
		if row.cells == want {
			continue
		}
		diags = append(diags, r.diag(f, row.lineNum, 1,
			fmt.Sprintf("table column count; expected %d, got %d", want, row.cells)))
	}
	return diags
}

func (r *Rule) checkSurroundingBlanks(f *lint.File, t tableBlock) []lint.Diagnostic {
	var diags []lint.Diagnostic
	if before := t.start() - 1; before >= 1 && !isBlank(f.Lines[before-1]) {
		diags = append(diags, r.diag(f, t.start(), 1,
			"missing blank line before table"))
	}
	if after := t.end() + 1; after <= len(f.Lines) && !isBlank(f.Lines[after-1]) {
		diags = append(diags, r.diag(f, t.end(), 1,
			"missing blank line after table"))
	}
	return diags
}

func (r *Rule) diag(f *lint.File, line, col int, msg string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     f.Path,
		Line:     line,
		Column:   col,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  msg,
	}
}

// Fix implements rule.FixableRule. It normalizes edge pipes (MD055) and
// inserts missing blank lines around tables (MD058). Column-count
// violations (MD056) are never auto-rewritten.
func (r *Rule) Fix(f *lint.File) []byte {
	skip := r.skipFunc(f)
	tables := findTables(f.Lines, skip)
	if len(tables) == 0 {
		return append([]byte(nil), f.Source...)
	}

	lines := make([]string, len(f.Lines))
	for i, l := range f.Lines {
		lines[i] = string(l)
	}

	blankBefore := map[int]bool{}
	blankAfter := map[int]bool{}
	for _, t := range tables {
		wantLead, wantTrail := r.expectedStyle(t)
		for _, row := range t.rows {
			if row.leading != wantLead || row.trailing != wantTrail {
				idx := row.lineNum - 1
				lines[idx] = normalizeEdges(lines[idx], t.prefix, wantLead, wantTrail)
			}
		}
		if before := t.start() - 1; before >= 1 && !isBlank(f.Lines[before-1]) {
			blankBefore[t.start()] = true
		}
		if after := t.end() + 1; after <= len(f.Lines) && !isBlank(f.Lines[after-1]) {
			blankAfter[t.end()] = true
		}
	}

	// Match the file's newline style so a CRLF document does not
	// gain a bare-LF blank line (mixed endings); lines are joined
	// with "\n", so a CRLF blank line is a lone "\r".
	blank := ""
	if bytes.Contains(f.Source, []byte("\r\n")) {
		blank = "\r"
	}

	var out []string
	for i, l := range lines {
		lineNum := i + 1
		if blankBefore[lineNum] {
			out = append(out, blank)
		}
		out = append(out, l)
		if blankAfter[lineNum] {
			out = append(out, blank)
		}
	}
	return []byte(strings.Join(out, "\n"))
}

// normalizeEdges rewrites one table row so its leading/trailing pipe
// presence matches want, preserving the whitespace prefix, the inner
// cell text, and a trailing carriage return.
func normalizeEdges(line, prefix string, wantLead, wantTrail bool) string {
	rest := strings.TrimPrefix(line, prefix)
	cr := ""
	if strings.HasSuffix(rest, "\r") {
		cr = "\r"
		rest = rest[:len(rest)-1]
	}
	trimmed := strings.TrimSpace(rest)
	trimmed = strings.TrimPrefix(trimmed, "|")
	trimmed = strings.TrimSuffix(trimmed, "|")
	trimmed = strings.TrimSpace(trimmed)

	var b strings.Builder
	b.WriteString(prefix)
	if wantLead {
		b.WriteString("| ")
	}
	b.WriteString(trimmed)
	if wantTrail {
		b.WriteString(" |")
	}
	b.WriteString(cr)
	return b.String()
}

// skipFunc returns a predicate reporting whether a 1-based line should
// be ignored: fenced/indented code, processing-instruction blocks, and
// the bodies of include/catalog generated sections (the source file
// owns those bytes, so MDS060 must not flag or rewrite them).
func (r *Rule) skipFunc(f *lint.File) func(int) bool {
	code := lint.CollectCodeBlockLines(f)
	pi := lint.CollectPIBlockLines(f)
	gen := f.GeneratedRanges
	return func(lineNum int) bool {
		if code[lineNum] || pi[lineNum] {
			return true
		}
		for _, gr := range gen {
			if gr.Contains(lineNum) {
				return true
			}
		}
		return false
	}
}

// findTables scans lines for GFM pipe tables. A table is a delimiter
// row (cells of dashes with optional colons, at least one pipe) with a
// non-blank, pipe-bearing header line directly above it, followed by
// zero or more body rows. All rows share one leading-whitespace prefix;
// the table ends at a blank line, a skipped line, EOF, or a line that
// does not continue the table.
func findTables(lines [][]byte, skip func(int) bool) []tableBlock {
	var tables []tableBlock
	i := 1 // separator can be at the earliest on line 2 (header above)
	for i < len(lines) {
		sepNum := i + 1 // 1-based line of candidate separator
		hdrNum := sepNum - 1
		if skip(sepNum) || skip(hdrNum) {
			i++
			continue
		}
		prefix, ok := sharedPrefix(lines[hdrNum-1], lines[sepNum-1])
		if !ok || !isSeparator(lines[sepNum-1], prefix) ||
			!isHeader(lines[hdrNum-1], prefix) {
			i++
			continue
		}

		t := tableBlock{prefix: prefix}
		t.rows = append(t.rows, parseRow(lines[hdrNum-1], hdrNum, prefix))
		t.rows = append(t.rows, parseRow(lines[sepNum-1], sepNum, prefix))

		next := sepNum + 1
		for next <= len(lines) {
			if skip(next) || !continuesTable(lines[next-1], prefix) {
				break
			}
			t.rows = append(t.rows, parseRow(lines[next-1], next, prefix))
			next++
		}
		tables = append(tables, t)
		i = next
	}
	return tables
}

// sharedPrefix returns the leading-whitespace prefix common to the
// header and separator lines, and whether they share one. Blockquote
// (`>`) tables are out of scope: a non-whitespace prefix yields ok =
// false so the candidate is skipped rather than mis-parsed.
func sharedPrefix(header, sep []byte) (string, bool) {
	hp := leadingWhitespace(header)
	sp := leadingWhitespace(sep)
	if hp != sp {
		return "", false
	}
	return hp, true
}

func leadingWhitespace(line []byte) string {
	n := 0
	for n < len(line) && (line[n] == ' ' || line[n] == '\t') {
		n++
	}
	return string(line[:n])
}

// rowContent strips the prefix and trailing whitespace/CR, returning
// the bare row text used for pipe and cell analysis.
func rowContent(line []byte, prefix string) string {
	s := strings.TrimPrefix(string(line), prefix)
	return strings.TrimRight(s, " \t\r")
}

func isSeparator(line []byte, prefix string) bool {
	c := rowContent(line, prefix)
	return strings.Contains(c, "|") && isSeparatorContent(c)
}

func isHeader(line []byte, prefix string) bool {
	c := rowContent(line, prefix)
	if c == "" || !strings.Contains(c, "|") {
		return false
	}
	if strings.HasPrefix(strings.TrimSpace(c), "#") {
		return false // ATX heading, not a table header
	}
	return !isSeparatorContent(c)
}

func isSeparatorContent(c string) bool {
	cells := logicalCells(c)
	if len(cells) == 1 && strings.TrimSpace(cells[0]) == "" {
		return false
	}
	for _, cell := range cells {
		if !sepCellRe.MatchString(strings.TrimSpace(cell)) {
			return false
		}
	}
	return true
}

// continuesTable reports whether line is a body row for a table with
// the given prefix: same whitespace prefix, non-blank, contains a pipe.
func continuesTable(line []byte, prefix string) bool {
	if isBlank(line) || leadingWhitespace(line) != prefix {
		return false
	}
	return strings.Contains(rowContent(line, prefix), "|")
}

func parseRow(line []byte, lineNum int, prefix string) tableRow {
	c := rowContent(line, prefix)
	lead := strings.HasPrefix(c, "|")
	trail := len(c) > 0 && strings.HasSuffix(c, "|")
	return tableRow{
		lineNum:  lineNum,
		leading:  lead,
		trailing: trail,
		cells:    countCells(c),
	}
}

// logicalCells splits a row into its cells, dropping the empty
// segments a leading or trailing pipe would otherwise produce so a
// bordered and a borderless row of the same shape count alike.
func logicalCells(content string) []string {
	t := strings.TrimSpace(content)
	t = strings.TrimPrefix(t, "|")
	t = strings.TrimSuffix(t, "|")
	return splitCells(t)
}

// countCells returns the logical cell count of a row. A row that is
// only edge pipes or empty has no cells.
func countCells(content string) int {
	cells := logicalCells(content)
	if len(cells) == 1 && strings.TrimSpace(cells[0]) == "" {
		t := strings.TrimSpace(content)
		if t == "" || t == "|" {
			return 0
		}
	}
	return len(cells)
}

// splitCells splits a row body on unescaped pipes, preserving `\|`.
func splitCells(s string) []string {
	var cells []string
	var cur strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && s[i+1] == '|' {
			cur.WriteString(`\|`)
			i++
			continue
		}
		if s[i] == '|' {
			cells = append(cells, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(s[i])
	}
	cells = append(cells, cur.String())
	return cells
}

func isBlank(line []byte) bool {
	return len(bytes.TrimSpace(line)) == 0
}

// describeStyle renders an edge-pipe shape for diagnostic messages.
func describeStyle(lead, trail bool) string {
	switch {
	case lead && trail:
		return "leading and trailing pipes"
	case lead:
		return "leading pipe only"
	case trail:
		return "trailing pipe only"
	default:
		return "no leading or trailing pipes"
	}
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "style":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("table-structure: style must be a string, got %T", v)
			}
			switch s {
			case StyleConsistent, StyleLeadingAndTrailing, StyleNoLeadingOrTrailing:
				r.Style = s
			default:
				return fmt.Errorf(
					"table-structure: invalid style %q (valid: %s, %s, %s)",
					s, StyleConsistent, StyleLeadingAndTrailing, StyleNoLeadingOrTrailing)
			}
		default:
			return fmt.Errorf("table-structure: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"style": StyleConsistent,
	}
}

var (
	_ rule.FixableRule  = (*Rule)(nil)
	_ rule.Configurable = (*Rule)(nil)
)
