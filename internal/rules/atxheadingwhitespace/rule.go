package atxheadingwhitespace

import (
	"bytes"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks ATX heading whitespace and indentation.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS064" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "atx-heading-whitespace" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "heading" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	codeLines := lint.CollectCodeBlockLines(f)
	piLines := lint.CollectPIBlockLines(f)
	var diags []lint.Diagnostic
	for i, rawLine := range f.Lines {
		// Candidate gate before the per-line set lookups: checkLine
		// only acts on lines whose first non-blank byte is '#'.
		// Running its own prefix test here removes the two map probes
		// for every prose line, including indented ones.
		leading := leadingSpaces(rawLine)
		if leading >= len(rawLine) || rawLine[leading] != '#' {
			continue
		}
		lineNum := i + 1
		if lint.InCodeOrPI(codeLines, piLines, lineNum) {
			continue
		}
		diags = append(diags, r.checkLine(f.Path, lineNum, rawLine)...)
	}
	return diags
}

func (r *Rule) checkLine(path string, lineNum int, line []byte) []lint.Diagnostic {
	var diags []lint.Diagnostic

	leading := leadingSpaces(line)
	rest := line[leading:]
	if len(rest) == 0 || rest[0] != '#' {
		return nil
	}

	level := 0
	for level < len(rest) && rest[level] == '#' {
		level++
	}
	if level > 6 {
		return nil
	}

	after := rest[level:]

	// A '#' run immediately followed by a digit on an indented line is almost
	// certainly an issue/PR reference (#22, #288) in a soft-wrapped list-item
	// continuation, not a malformed ATX heading. At column 1 the same pattern
	// (#1Heading, ##22Title) IS a missing-space defect and is flagged normally.
	if leading > 0 && len(after) > 0 && after[0] >= '0' && after[0] <= '9' {
		return nil
	}

	if leading > 0 {
		diags = append(diags, r.diag(path, lineNum, 1, "heading must start at column 1"))
	}

	if len(bytes.TrimRight(after, " \t\r")) == 0 {
		return diags
	}

	if after[0] != ' ' {
		diags = append(diags, r.diag(path, lineNum, leading+level+1, "missing space after # in heading"))
	} else if leadingSpaces(after) > 1 {
		diags = append(diags, r.diag(path, lineNum, leading+level+2, "multiple spaces or tabs after # in heading"))
	}

	diags = append(diags, r.checkClosingATX(path, lineNum, leading, level, after)...)
	return diags
}

func (r *Rule) checkClosingATX(path string, lineNum, leading, level int, after []byte) []lint.Diagnostic {
	trimmed := bytes.TrimRight(after, " \t\r")
	if len(trimmed) == 0 || trimmed[len(trimmed)-1] != '#' {
		return nil
	}

	hashStart := len(trimmed)
	for hashStart > 0 && trimmed[hashStart-1] == '#' {
		hashStart--
	}
	if hashStart == 0 {
		return nil // content is all hashes; no closing-suffix defect
	}

	spaceEnd := hashStart
	for spaceEnd > 0 && (trimmed[spaceEnd-1] == ' ' || trimmed[spaceEnd-1] == '\t') {
		spaceEnd--
	}
	spacesBeforeHash := hashStart - spaceEnd

	// Only treat trailing # as a closing ATX marker when preceded by whitespace
	// (CommonMark requirement). No preceding space means the # is content (e.g. "# C#").
	if spacesBeforeHash == 0 {
		return nil
	}

	switch spacesBeforeHash {
	case 1:
		return []lint.Diagnostic{r.diag(path, lineNum, leading+level+spaceEnd+1,
			"heading has closing # marker")}
	default:
		return []lint.Diagnostic{r.diag(path, lineNum, leading+level+spaceEnd+1,
			"multiple spaces before closing # in heading")}
	}
}

func (r *Rule) diag(path string, line, col int, msg string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     path,
		Line:     line,
		Column:   col,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  msg,
	}
}

// Fix implements rule.FixableRule.
func (r *Rule) Fix(f *lint.File) []byte {
	codeLines := lint.CollectCodeBlockLines(f)
	piLines := lint.CollectPIBlockLines(f)
	result := make([][]byte, 0, len(f.Lines))
	for i, rawLine := range f.Lines {
		lineNum := i + 1
		if lint.InCodeOrPI(codeLines, piLines, lineNum) {
			result = append(result, rawLine)
			continue
		}
		if diags := r.checkLine("", lineNum, rawLine); len(diags) > 0 {
			result = append(result, normalizeLine(rawLine))
		} else {
			// Unchanged line: append slice header only, no string copy.
			result = append(result, rawLine)
		}
	}
	return bytes.Join(result, []byte{'\n'})
}

func normalizeLine(line []byte) []byte {
	leading := leadingSpaces(line)
	rest := line[leading:]

	level := 0
	for level < len(rest) && rest[level] == '#' {
		level++
	}
	if level == 0 || level > 6 {
		// No change needed: return the original slice, no allocation.
		return line
	}

	// Preserve a trailing \r so CRLF files don't get mixed line endings when
	// only some lines are rewritten.
	hasCR := len(line) > 0 && line[len(line)-1] == '\r'

	content := extractContent(string(rest[level:]))

	// Pre-size: level '#' chars + optional " " + content + optional '\r'.
	n := level
	if content != "" {
		n += 1 + len(content)
	}
	if hasCR {
		n++
	}
	buf := make([]byte, 0, n)
	for i := 0; i < level; i++ {
		buf = append(buf, '#')
	}
	if content != "" {
		buf = append(buf, ' ')
		buf = append(buf, content...)
	}
	if hasCR {
		buf = append(buf, '\r')
	}
	return buf
}

// extractContent strips leading/trailing whitespace and any closing ATX suffix
// from everything after the opening hashes. A trailing run of '#' is only
// treated as a closing marker when preceded by whitespace; otherwise it is
// part of the content (e.g. "C#" in "# C#").
func extractContent(after string) string {
	s := strings.TrimSpace(after)
	if s == "" {
		return ""
	}
	hashStart := len(s)
	for hashStart > 0 && s[hashStart-1] == '#' {
		hashStart--
	}
	if hashStart == len(s) {
		return s // no trailing hashes
	}
	if hashStart == 0 {
		return "" // content is all hashes (empty heading with closing hashes)
	}
	// Trailing hashes not preceded by whitespace are content, not a closing marker.
	if s[hashStart-1] != ' ' && s[hashStart-1] != '\t' {
		return s
	}
	return strings.TrimRight(s[:hashStart], " \t")
}

// leadingSpaces returns the number of leading space or tab bytes in b.
func leadingSpaces(b []byte) int {
	n := 0
	for n < len(b) && (b[n] == ' ' || b[n] == '\t') {
		n++
	}
	return n
}

var _ rule.FixableRule = (*Rule)(nil)

// FixTitle implements rule.QuickFixTitler.
func (r *Rule) FixTitle() string { return "Fix heading spacing" }
