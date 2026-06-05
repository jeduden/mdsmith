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
	result := make([]string, 0, len(f.Lines))
	for i, rawLine := range f.Lines {
		lineNum := i + 1
		if lint.InCodeOrPI(codeLines, piLines, lineNum) {
			result = append(result, string(rawLine))
			continue
		}
		if diags := r.checkLine("", lineNum, rawLine); len(diags) > 0 {
			result = append(result, normalizeLine(rawLine))
		} else {
			result = append(result, string(rawLine))
		}
	}
	return []byte(strings.Join(result, "\n"))
}

func normalizeLine(line []byte) string {
	leading := leadingSpaces(line)
	rest := line[leading:]

	level := 0
	for level < len(rest) && rest[level] == '#' {
		level++
	}
	if level == 0 || level > 6 {
		return string(line)
	}

	// Preserve a trailing \r so CRLF files don't get mixed line endings when
	// only some lines are rewritten.
	cr := ""
	if len(line) > 0 && line[len(line)-1] == '\r' {
		cr = "\r"
	}

	prefix := strings.Repeat("#", level)
	content := extractContent(string(rest[level:]))
	if content == "" {
		return prefix + cr
	}
	return prefix + " " + content + cr
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
