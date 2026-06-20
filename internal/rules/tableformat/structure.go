package tableformat

// This file folds the structural table checks — MD055 table-pipe-style,
// MD056 table-column-count, MD058 blanks-around-tables — into the same
// rule that formats tables, so a single MDS025 owns table parsing,
// structure, and alignment without a per-pass disagreement.
//
// The format pass (rule.go via tablefmt) still only detects bordered
// tables: it cannot reformat borderless cells without inventing a
// column width. The structure pass below uses a GFM-aware parser
// (header + delimiter + body rows; edge pipes optional, blockquote and
// list-indent prefixes recognised) so MD055/056/058 still apply to
// borderless, mixed-pipe, blockquoted, and indented tables.

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
)

// Pipe-style values for the `style` setting (MD055).
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

// tableRow is one parsed source line belonging to a table.
type tableRow struct {
	lineNum  int  // 1-based line number in f.Lines
	leading  bool // content (prefix stripped) begins with '|'
	trailing bool // content (prefix stripped) ends with '|'
	cells    int  // logical cell count
}

// tableBlock is a contiguous detected GFM table.
type tableBlock struct {
	prefix string // shared blockquote/indentation prefix
	rows   []tableRow
}

func (t tableBlock) start() int { return t.rows[0].lineNum }
func (t tableBlock) end() int   { return t.rows[len(t.rows)-1].lineNum }

// structureDiagnostics returns MD055/MD056/MD058 diagnostics for f
// using the given style. Diagnostics carry the supplied ruleID and
// ruleName so the merged rule emits them under MDS025.
func structureDiagnostics(f *lint.File, style, ruleID, ruleName string) []lint.Diagnostic {
	skip := structureSkipFunc(f)
	tables := findStructureTables(f.Lines, skip)
	diags := make([]lint.Diagnostic, 0, len(tables))
	for _, t := range tables {
		diags = append(diags, checkPipeStyle(f, t, style, ruleID, ruleName)...)
		diags = append(diags, checkColumnCount(f, t, ruleID, ruleName)...)
		diags = append(diags, checkSurroundingBlanks(f, t, ruleID, ruleName)...)
	}
	sort.SliceStable(diags, func(i, j int) bool {
		if diags[i].Line != diags[j].Line {
			return diags[i].Line < diags[j].Line
		}
		return diags[i].Column < diags[j].Column
	})
	return diags
}

// applyStructureFix rewrites f.Source: edge normalization for MD055
// and blank-line insertion for MD058. MD056 column count is never
// auto-rewritten (a missing cell's content is unknown). Callers run
// this before tablefmt's alignment pass so the format pass sees the
// structurally-normalised bytes.
func applyStructureFix(f *lint.File, style string) []byte {
	skip := structureSkipFunc(f)
	tables := findStructureTables(f.Lines, skip)
	if len(tables) == 0 {
		return append([]byte(nil), f.Source...)
	}
	edits := collectStructureEdits(f, tables, style)
	return renderStructureFix(f, edits)
}

// structureEdits records the per-line overrides and blank-line
// insertions the structure fix needs to apply when rebuilding the
// source buffer. Each map is nil when empty so the renderer's
// lookups stay cheap on the common no-op path.
type structureEdits struct {
	modified    map[int]string // 1-based line -> edge-normalised row
	blankBefore map[int]string // 1-based line -> blank inserted before
	blankAfter  map[int]string // 1-based line -> blank inserted after
}

// collectStructureEdits walks the parsed tables and records which
// rows need edge normalisation (MD055) plus where surrounding blank
// lines must be inserted (MD058). MD056 row mismatches are never
// auto-rewritten so they don't appear here.
func collectStructureEdits(f *lint.File, tables []tableBlock, style string) structureEdits {
	// Match the file's newline style so a CRLF document does not gain
	// a bare-LF blank line (mixed endings); lines are emitted with a
	// `\n` separator, so a CRLF blank line ends in a lone `\r`.
	cr := ""
	if bytes.Contains(f.Source, []byte("\r\n")) {
		cr = "\r"
	}
	var e structureEdits
	for _, t := range tables {
		wantLead, wantTrail := expectedStyle(style, t)
		for _, row := range t.rows {
			if row.leading == wantLead && row.trailing == wantTrail {
				continue
			}
			if e.modified == nil {
				e.modified = map[int]string{}
			}
			e.modified[row.lineNum] = normalizeEdges(string(f.Lines[row.lineNum-1]), t.prefix, wantLead, wantTrail)
		}
		blank := blankLineFor(t.prefix) + cr
		if before := t.start() - 1; before >= 1 && !isBlankAround(f.Lines[before-1], t.prefix) {
			if e.blankBefore == nil {
				e.blankBefore = map[int]string{}
			}
			e.blankBefore[t.start()] = blank
		}
		if after := t.end() + 1; after <= len(f.Lines) && !isBlankAround(f.Lines[after-1], t.prefix) {
			if e.blankAfter == nil {
				e.blankAfter = map[int]string{}
			}
			e.blankAfter[t.end()] = blank
		}
	}
	return e
}

// renderStructureFix streams the rebuilt source into a buffer
// pre-sized to the source length, writing untouched rows directly
// from f.Lines (no per-line []byte→string conversion) and pulling
// modified rows from the edits map. blankBefore[K] is suppressed
// when blankAfter[K-1] is already scheduled at the same gap, which
// avoids MDS008 no-multiple-blanks on adjacent tables.
func renderStructureFix(f *lint.File, e structureEdits) []byte {
	var buf bytes.Buffer
	buf.Grow(len(f.Source) + 16)
	first := true
	emitSep := func() {
		if first {
			first = false
			return
		}
		buf.WriteByte('\n')
	}
	for i, line := range f.Lines {
		lineNum := i + 1
		if b, ok := e.blankBefore[lineNum]; ok {
			if _, dup := e.blankAfter[lineNum-1]; !dup {
				emitSep()
				buf.WriteString(b)
			}
		}
		emitSep()
		if mod, ok := e.modified[lineNum]; ok {
			buf.WriteString(mod)
		} else {
			buf.Write(line)
		}
		if b, ok := e.blankAfter[lineNum]; ok {
			emitSep()
			buf.WriteString(b)
		}
	}
	return buf.Bytes()
}

// expectedStyle returns the required (leading, trailing) edge-pipe
// presence for table t under the configured style.
func expectedStyle(style string, t tableBlock) (lead, trail bool) {
	switch style {
	case StyleLeadingAndTrailing:
		return true, true
	case StyleNoLeadingOrTrailing:
		return false, false
	default: // StyleConsistent: infer from the header row.
		return t.rows[0].leading, t.rows[0].trailing
	}
}

func checkPipeStyle(f *lint.File, t tableBlock, style, ruleID, ruleName string) []lint.Diagnostic {
	wantLead, wantTrail := expectedStyle(style, t)
	var diags []lint.Diagnostic
	for _, row := range t.rows {
		if row.leading == wantLead && row.trailing == wantTrail {
			continue
		}
		diags = append(diags, structureDiag(f, row.lineNum, 1, ruleID, ruleName,
			"table pipe style; expected "+describeStyle(wantLead, wantTrail)))
	}
	return diags
}

func checkColumnCount(f *lint.File, t tableBlock, ruleID, ruleName string) []lint.Diagnostic {
	want := t.rows[0].cells
	diags := make([]lint.Diagnostic, 0, len(t.rows)-1)
	for _, row := range t.rows[1:] {
		if row.cells == want {
			continue
		}
		diags = append(diags, structureDiag(f, row.lineNum, 1, ruleID, ruleName,
			fmt.Sprintf("table column count; expected %d, got %d", want, row.cells)))
	}
	if len(diags) == 0 {
		return nil
	}
	return diags
}

func checkSurroundingBlanks(f *lint.File, t tableBlock, ruleID, ruleName string) []lint.Diagnostic {
	var diags []lint.Diagnostic
	if before := t.start() - 1; before >= 1 && !isBlankAround(f.Lines[before-1], t.prefix) {
		diags = append(diags, structureDiag(f, t.start(), 1, ruleID, ruleName,
			"missing blank line before table"))
	}
	if after := t.end() + 1; after <= len(f.Lines) && !isBlankAround(f.Lines[after-1], t.prefix) {
		diags = append(diags, structureDiag(f, t.end(), 1, ruleID, ruleName,
			"missing blank line after table"))
	}
	return diags
}

func structureDiag(f *lint.File, line, col int, ruleID, ruleName, msg string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     f.Path,
		Line:     line,
		Column:   col,
		RuleID:   ruleID,
		RuleName: ruleName,
		Severity: lint.Warning,
		Message:  msg,
	}
}

// normalizeEdges rewrites one table row so its leading/trailing pipe
// presence matches want, preserving the prefix, the inner cell text,
// and a trailing carriage return.
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

// structureSkipFunc returns a predicate reporting whether a 1-based
// line should be ignored by the structure pass: fenced/indented code,
// processing-instruction blocks, and the bodies of include/catalog
// generated sections (the source file owns those bytes).
func structureSkipFunc(f *lint.File) func(int) bool {
	code := lint.CollectCodeBlockLines(f)
	pi := lint.CollectPIBlockLines(f)
	gen := f.GeneratedRanges
	return func(lineNum int) bool {
		if lint.InCodeOrPI(code, pi, lineNum) {
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

// findStructureTables scans lines for GFM pipe tables. A table is a
// delimiter row (cells of dashes with optional colons, at least one
// unescaped pipe) with a non-blank, pipe-bearing header line directly
// above it, followed by zero or more body rows. All rows share one
// prefix (blockquote markers and/or leading whitespace); the table
// ends at a blank line, a skipped line, EOF, or a line that does not
// continue the table.
func findStructureTables(lines [][]byte, skip func(int) bool) []tableBlock {
	var tables []tableBlock
	i := 1 // separator can be at the earliest on line 2 (header above)
	for i < len(lines) {
		sepNum := i + 1 // 1-based line of candidate separator
		hdrNum := sepNum - 1
		if skip(sepNum) || skip(hdrNum) {
			i++
			continue
		}
		// A GFM separator row must contain '|'. Skip lines without one
		// before the more expensive sharedPrefix call, avoiding the
		// string(line) allocations that structureDetectPrefix would pay
		// on every non-pipe line in the file.
		if bytes.IndexByte(lines[i], '|') < 0 {
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
	hp := structureDetectPrefix(header)
	sp := structureDetectPrefix(sep)
	if hp != sp {
		return "", false
	}
	return hp, true
}

// structureDetectPrefix returns the blockquote/indentation prefix of
// a line: a chain of `>` markers (each optionally followed by one
// space, with optional indentation before each), mirroring tablefmt
// so the format and structure passes agree on blockquoted tables.
// When no blockquote marker is present it falls back to the run of
// leading whitespace, which covers list-indented and borderless
// tables. Works entirely in bytes to avoid allocating a string copy
// of line for the common no-blockquote case.
func structureDetectPrefix(line []byte) string {
	var b strings.Builder
	rem := line
	for {
		// Count leading spaces (ASCII space only, matching original).
		i := 0
		for i < len(rem) && rem[i] == ' ' {
			i++
		}
		indent := rem[:i]
		rem = rem[i:]
		switch {
		case len(rem) >= 2 && rem[0] == '>' && rem[1] == ' ':
			b.Write(indent)
			b.WriteString("> ")
			rem = rem[2:]
		case len(rem) >= 1 && rem[0] == '>' && (len(rem) == 1 || rem[1] == '>'):
			b.Write(indent)
			b.WriteByte('>')
			rem = rem[1:]
		default:
			if b.Len() > 0 {
				return b.String()
			}
			// No blockquote: count leading whitespace/tabs from original.
			n := 0
			for n < len(line) && (line[n] == ' ' || line[n] == '\t') {
				n++
			}
			if n == 0 {
				return ""
			}
			return string(line[:n])
		}
	}
}

// blankLineFor returns the text of an inserted MD058 blank line for a
// table with the given prefix. Inside a blockquote the separating
// line is the bare marker chain (e.g. `>`), not an empty line, so
// the blockquote is not broken.
func blankLineFor(prefix string) string {
	if strings.Contains(prefix, ">") {
		return strings.TrimRight(prefix, " \t")
	}
	return ""
}

// isBlankAround reports whether line counts as the blank line
// bounding a table with the given prefix: a wholly empty line, or
// — for a blockquoted table — a line that is only blockquote
// markers.
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
// the bare row bytes used for pipe and cell analysis. Works entirely
// in bytes — no string conversion — so callers avoid a heap alloc on
// every table row in the structure-pass hot path.
func rowContent(line []byte, prefix string) []byte {
	content := line
	// string(line[:len(prefix)]) == prefix is a compiler-optimised
	// comparison that does not allocate a temporary string.
	if len(prefix) > 0 && len(line) >= len(prefix) &&
		string(line[:len(prefix)]) == prefix {
		content = line[len(prefix):]
	}
	return bytes.TrimRight(content, " \t\r")
}

func isSeparator(line []byte, prefix string) bool {
	c := rowContent(line, prefix)
	return containsUnescapedPipe(c) && isSeparatorContent(c)
}

func isHeader(line []byte, prefix string) bool {
	c := rowContent(line, prefix)
	if len(c) == 0 || !containsUnescapedPipe(c) {
		return false
	}
	if isATXHeading(c) {
		return false
	}
	// A header without at least one logical cell ("|", "||", and
	// similar) is not a valid table row. tablefmt also refuses to
	// detect such lines, and accepting them would surface phantom
	// MD055/MD056 diagnostics on prose that happens to sit above a
	// delimiter-shaped line.
	if countCells(c) == 0 {
		return false
	}
	return !isSeparatorContent(c)
}

// isATXHeading reports whether b has the shape of a CommonMark ATX
// heading: one to six `#` characters followed by a space, tab, or
// end-of-line. A bare `#` at the start (e.g. `#1 | x`) is not a
// heading and must not exclude a candidate from table parsing.
func isATXHeading(b []byte) bool {
	b = bytes.TrimSpace(b)
	n := 0
	for n < len(b) && n < 6 && b[n] == '#' {
		n++
	}
	if n == 0 {
		return false
	}
	if n == len(b) {
		return true // bare hashes, empty heading
	}
	c := b[n]
	return c == ' ' || c == '\t'
}

// containsUnescapedPipe reports whether b contains a `|` that is a
// real cell delimiter. A `\|` pair is treated as one escaped-pipe
// literal — matching tablefmt's GFM escape rule. CommonMark's full
// backslash grammar (where `\\|` would be a literal `\` followed by
// an unescaped pipe) is intentionally NOT honored: GitHub's renderer
// doesn't, and the structure pass must agree with tablefmt or the
// two disagree on cell counts for inputs containing `\\|`.
func containsUnescapedPipe(b []byte) bool {
	for i := 0; i < len(b); i++ {
		if b[i] == '\\' && i+1 < len(b) && b[i+1] == '|' {
			i++ // skip the escaped pipe
			continue
		}
		if b[i] == '|' {
			return true
		}
	}
	return false
}

// isSeparatorContent reports whether c (bare row bytes with prefix
// already stripped) consists of valid GFM separator cells (:?-+:?).
// It works entirely in bytes to avoid allocating strings or a cell
// slice, replacing the former regex + logicalCells path.
func isSeparatorContent(c []byte) bool {
	t := bytes.TrimSpace(c)
	// Strip edge pipes (mirrors logicalCells).
	if len(t) > 0 && t[0] == '|' {
		t = t[1:]
	}
	if endsWithUnescapedPipeBytes(t) {
		t = t[:len(t)-1]
	}
	if len(t) == 0 {
		return false
	}
	hasCells := false
	for len(t) > 0 {
		sep := findUnescapedPipe(t)
		var cell []byte
		if sep < 0 {
			cell = bytes.TrimSpace(t)
			t = t[len(t):]
		} else {
			cell = bytes.TrimSpace(t[:sep])
			t = t[sep+1:]
		}
		if len(cell) > 0 {
			if !isSepCellBytes(cell) {
				return false
			}
			hasCells = true
		}
	}
	return hasCells
}

// isSepCellBytes reports whether cell matches the GFM separator cell
// pattern :?-+:? without allocating. Replaces sepCellRe.MatchString.
func isSepCellBytes(cell []byte) bool {
	if len(cell) == 0 {
		return false
	}
	i := 0
	if cell[i] == ':' {
		i++
	}
	if i >= len(cell) || cell[i] != '-' {
		return false
	}
	for i < len(cell) && cell[i] == '-' {
		i++
	}
	if i < len(cell) {
		return cell[i] == ':' && i == len(cell)-1
	}
	return true
}

// findUnescapedPipe returns the index of the first unescaped '|' in b,
// or -1 if none. Mirrors containsUnescapedPipe's escape semantics.
func findUnescapedPipe(b []byte) int {
	for i := 0; i < len(b); i++ {
		if b[i] == '\\' && i+1 < len(b) && b[i+1] == '|' {
			i++
			continue
		}
		if b[i] == '|' {
			return i
		}
	}
	return -1
}

// endsWithUnescapedPipeBytes is the []byte counterpart of
// endsWithUnescapedPipe, used in the structure-pass hot path.
func endsWithUnescapedPipeBytes(b []byte) bool {
	if len(b) == 0 || b[len(b)-1] != '|' {
		return false
	}
	return len(b) < 2 || b[len(b)-2] != '\\'
}

// continuesTable reports whether line is a body row for a table with
// the given prefix: same prefix, non-blank, and contains at least one
// unescaped pipe (paragraphs whose only pipe is `\|` end the table).
func continuesTable(line []byte, prefix string) bool {
	if isBlank(line) || structureDetectPrefix(line) != prefix {
		return false
	}
	return containsUnescapedPipe(rowContent(line, prefix))
}

// NOTE: endsWithUnescapedPipe (string version) is retained for use by
// normalizeEdges in the Fix path. The bytes counterpart above is used
// in the structure-pass Check hot path.

// endsWithUnescapedPipe reports whether s ends with a real edge pipe
// rather than an escaped literal `\|`. A trailing `|` is an edge
// unless it is preceded by exactly one `\` — matching tablefmt's
// GFM escape semantics so the two passes agree.
func endsWithUnescapedPipe(s string) bool {
	if !strings.HasSuffix(s, "|") {
		return false
	}
	return len(s) < 2 || s[len(s)-2] != '\\'
}

func parseRow(line []byte, lineNum int, prefix string) tableRow {
	c := rowContent(line, prefix)
	// Extra whitespace between the prefix and the first cell — common
	// inside list items and blockquotes with double-space indent —
	// must not hide a real edge pipe; edge detection mirrors countCells.
	t := bytes.TrimSpace(c)
	lead := len(t) > 0 && t[0] == '|'
	trail := endsWithUnescapedPipeBytes(t)
	return tableRow{
		lineNum:  lineNum,
		leading:  lead,
		trailing: trail,
		cells:    countCells(c),
	}
}

// countCells returns the logical cell count of a row. An empty row or
// a row consisting of a single bare pipe ("|") has zero cells. A
// bordered row whose interior is all whitespace (e.g. "|  |") has one
// empty cell, not zero.
//
// Works entirely in bytes to avoid allocating a string copy on each
// table row in the structure-pass hot path.
func countCells(content []byte) int {
	t := bytes.TrimSpace(content)
	if len(t) == 0 || (len(t) == 1 && t[0] == '|') {
		return 0
	}
	s := t
	if len(s) > 0 && s[0] == '|' {
		s = s[1:]
	}
	if endsWithUnescapedPipeBytes(s) {
		s = s[:len(s)-1]
	}
	if len(bytes.TrimSpace(s)) == 0 {
		// Bordered row like "|  |" has one empty cell, not zero.
		return 1
	}
	return countUnescapedPipes(s) + 1
}

// countUnescapedPipes counts the number of unescaped '|' characters in b.
// A '\|' pair is treated as one escaped-pipe literal, not a cell delimiter.
func countUnescapedPipes(b []byte) int {
	n := 0
	for i := 0; i < len(b); i++ {
		if b[i] == '\\' && i+1 < len(b) && b[i+1] == '|' {
			i++ // skip escaped pipe
			continue
		}
		if b[i] == '|' {
			n++
		}
	}
	return n
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
