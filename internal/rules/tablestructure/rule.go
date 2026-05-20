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
	if before := t.start() - 1; before >= 1 && !isBlankAround(f.Lines[before-1], t.prefix) {
		diags = append(diags, r.diag(f, t.start(), 1,
			"missing blank line before table"))
	}
	if after := t.end() + 1; after <= len(f.Lines) && !isBlankAround(f.Lines[after-1], t.prefix) {
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

	// Match the file's newline style so a CRLF document does not
	// gain a bare-LF blank line (mixed endings); lines are joined
	// with "\n", so a CRLF blank line ends in a lone "\r".
	cr := ""
	if bytes.Contains(f.Source, []byte("\r\n")) {
		cr = "\r"
	}

	blankBefore := map[int]string{}
	blankAfter := map[int]string{}
	for _, t := range tables {
		wantLead, wantTrail := r.expectedStyle(t)
		for _, row := range t.rows {
			if row.leading != wantLead || row.trailing != wantTrail {
				idx := row.lineNum - 1
				lines[idx] = normalizeEdges(lines[idx], t.prefix, wantLead, wantTrail)
			}
		}
		blank := blankLineFor(t.prefix) + cr
		if before := t.start() - 1; before >= 1 && !isBlankAround(f.Lines[before-1], t.prefix) {
			blankBefore[t.start()] = blank
		}
		if after := t.end() + 1; after <= len(f.Lines) && !isBlankAround(f.Lines[after-1], t.prefix) {
			blankAfter[t.end()] = blank
		}
	}

	var out []string
	for i, l := range lines {
		lineNum := i + 1
		if b, ok := blankBefore[lineNum]; ok {
			out = append(out, b)
		}
		out = append(out, l)
		if b, ok := blankAfter[lineNum]; ok {
			out = append(out, b)
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
	if endsWithUnescapedPipe(trimmed) {
		trimmed = trimmed[:len(trimmed)-1]
	}
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

// sharedPrefix returns the row prefix common to the header and
// separator lines, and whether they share one. A table's rows must
// all carry the same prefix (blockquote markers and/or indentation).
func sharedPrefix(header, sep []byte) (string, bool) {
	hp := detectPrefix(header)
	sp := detectPrefix(sep)
	if hp != sp {
		return "", false
	}
	return hp, true
}

// detectPrefix returns the blockquote/indentation prefix of a line:
// a chain of `>` markers (each optionally followed by one space, with
// optional indentation before each), mirroring MDS025's tablefmt so
// the two rules agree on blockquoted tables. When no blockquote marker
// is present it falls back to the run of leading whitespace, which
// covers list-indented and borderless tables.
func detectPrefix(line []byte) string {
	s := string(line)
	var b strings.Builder
	rem := s
	for {
		trimmed := strings.TrimLeft(rem, " ")
		indent := rem[:len(rem)-len(trimmed)]
		switch {
		case strings.HasPrefix(trimmed, "> "):
			b.WriteString(indent)
			b.WriteString("> ")
			rem = trimmed[2:]
		case strings.HasPrefix(trimmed, ">") && (len(trimmed) == 1 || trimmed[1] == '>'):
			b.WriteString(indent)
			b.WriteString(">")
			rem = trimmed[1:]
		default:
			if b.Len() > 0 {
				return b.String()
			}
			n := 0
			for n < len(line) && (line[n] == ' ' || line[n] == '\t') {
				n++
			}
			return string(line[:n])
		}
	}
}

// blankLineFor returns the text of an inserted MD058 blank line for a
// table with the given prefix. Inside a blockquote the separating line
// is the bare marker chain (e.g. `>`), not an empty line, so the
// blockquote is not broken.
func blankLineFor(prefix string) string {
	if strings.Contains(prefix, ">") {
		return strings.TrimRight(prefix, " \t")
	}
	return ""
}

// isBlankAround reports whether line counts as the blank line bounding
// a table with the given prefix: a wholly empty line, or — for a
// blockquoted table — a line that is only blockquote markers.
func isBlankAround(line []byte, prefix string) bool {
	t := bytes.TrimSpace(line)
	if len(t) == 0 {
		return true
	}
	if strings.Contains(prefix, ">") {
		for _, c := range t {
			if c != '>' && c != ' ' && c != '\t' {
				return false
			}
		}
		return true
	}
	return false
}

// rowContent strips the prefix and trailing whitespace/CR, returning
// the bare row text used for pipe and cell analysis.
func rowContent(line []byte, prefix string) string {
	s := strings.TrimPrefix(string(line), prefix)
	return strings.TrimRight(s, " \t\r")
}

func isSeparator(line []byte, prefix string) bool {
	c := rowContent(line, prefix)
	return containsUnescapedPipe(c) && isSeparatorContent(c)
}

func isHeader(line []byte, prefix string) bool {
	c := rowContent(line, prefix)
	if c == "" || !containsUnescapedPipe(c) {
		return false
	}
	if isATXHeading(c) {
		return false
	}
	return !isSeparatorContent(c)
}

// isATXHeading reports whether s has the shape of a CommonMark ATX
// heading: one to six `#` characters followed by a space, tab, or
// end-of-line. A bare `#` at the start (e.g. `#1 | x`) is not a
// heading and must not exclude a candidate from table parsing.
func isATXHeading(s string) bool {
	s = strings.TrimSpace(s)
	n := 0
	for n < len(s) && n < 6 && s[n] == '#' {
		n++
	}
	if n == 0 {
		return false
	}
	if n == len(s) {
		return true // bare hashes, empty heading
	}
	c := s[n]
	return c == ' ' || c == '\t'
}

// containsUnescapedPipe reports whether s contains a `|` that is a
// real delimiter — that is, not escaped by a preceding `\` (with
// backslash parity respected so `\\|` counts as unescaped).
func containsUnescapedPipe(s string) bool {
	escape := false
	for i := 0; i < len(s); i++ {
		switch {
		case escape:
			escape = false
		case s[i] == '\\':
			escape = true
		case s[i] == '|':
			return true
		}
	}
	return false
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
// the given prefix: same prefix, non-blank, and contains at least one
// unescaped pipe (paragraphs whose only pipe is `\|` end the table).
func continuesTable(line []byte, prefix string) bool {
	if isBlank(line) || detectPrefix(line) != prefix {
		return false
	}
	return containsUnescapedPipe(rowContent(line, prefix))
}

// endsWithUnescapedPipe reports whether s ends with a real edge pipe
// rather than an escaped literal `\|`. A trailing `|` is an edge only
// when an even number (including zero) of backslashes precede it.
func endsWithUnescapedPipe(s string) bool {
	if !strings.HasSuffix(s, "|") {
		return false
	}
	bs := 0
	for i := len(s) - 2; i >= 0 && s[i] == '\\'; i-- {
		bs++
	}
	return bs%2 == 0
}

func parseRow(line []byte, lineNum int, prefix string) tableRow {
	c := rowContent(line, prefix)
	// Extra whitespace between the prefix and the first cell — common
	// inside list items and blockquotes with double-space indent —
	// must not hide a real edge pipe; logicalCells already trims, so
	// edge detection mirrors it.
	t := strings.TrimSpace(c)
	lead := strings.HasPrefix(t, "|")
	trail := endsWithUnescapedPipe(t)
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
	if endsWithUnescapedPipe(t) {
		t = t[:len(t)-1]
	}
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

// splitCells splits a row body on unescaped pipes, honoring backslash
// parity: `\|` is a literal pipe, `\\|` is an escaped backslash
// followed by a real delimiter, and so on.
func splitCells(s string) []string {
	var cells []string
	var cur strings.Builder
	escape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case escape:
			cur.WriteByte(c)
			escape = false
		case c == '\\':
			cur.WriteByte(c)
			escape = true
		case c == '|':
			cells = append(cells, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
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
