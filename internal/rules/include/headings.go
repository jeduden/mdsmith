package include

import (
	"strings"
)

// adjustHeadings shifts all heading levels in content so that the minimum
// heading level becomes parentLevel+1. If parentLevel is 0 or the computed
// shift is <= 0, content is returned unchanged.
func adjustHeadings(content string, parentLevel int) string {
	if parentLevel <= 0 {
		return content
	}

	lines := strings.Split(content, "\n")

	// First pass: find the minimum heading level (source top level).
	minLevel := findMinHeadingLevel(lines)
	if minLevel == 0 {
		// No headings found.
		return content
	}

	shift := parentLevel - minLevel + 1
	if shift <= 0 {
		return content
	}

	// Second pass: apply the shift.
	result := applyShift(lines, shift)

	return strings.Join(result, "\n")
}

// adjustHeadingsByOffset shifts every heading level in content by offset,
// clamped to the 1..6 range. An offset of 0 returns content unchanged.
// Unlike adjustHeadings, the shift is uniform and source-absolute: it does
// not read a parent heading level and may move headings up (negative
// offset), so it calls applyShift directly without the "skip when shift
// <= 0" guard that parent-relative nesting needs.
func adjustHeadingsByOffset(content string, offset int) string {
	if offset == 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	return strings.Join(applyShift(lines, offset), "\n")
}

// adjustHeadingsToLevel shifts headings so the shallowest one becomes the
// given target level (1..6), reusing findMinHeadingLevel and applyShift. The
// shift may be negative (promotion); levels are clamped to 1..6. Content with
// no heading, or already at the target, is returned unchanged. Unlike
// adjustHeadings this carries no parent-relative guards: the target is
// stated outright, so an explicit promotion is allowed.
func adjustHeadingsToLevel(content string, target int) string {
	lines := strings.Split(content, "\n")
	minLevel := findMinHeadingLevel(lines)
	if minLevel == 0 {
		return content
	}
	shift := target - minLevel
	if shift == 0 {
		return content
	}
	return strings.Join(applyShift(lines, shift), "\n")
}

// findMinHeadingLevel scans lines and returns the minimum heading level found,
// ignoring lines inside fenced code blocks, HTML blocks, and processing
// instructions. Returns 0 if no headings are found.
func findMinHeadingLevel(lines []string) int {
	minLevel := 0
	var scan headingScan

	for _, line := range lines {
		level, _ := scan.step(line)
		if level > 0 && (minLevel == 0 || level < minLevel) {
			minLevel = level
		}
	}

	return minLevel
}

// setextLevel returns 1 when line is a setext h1 underline (`=` run),
// 2 when it is a setext h2 underline (`-` run), and 0 otherwise: up to
// three spaces, a run of one character, then only whitespace. It reads
// the bytes directly and bails on the first non-matching byte, since it
// runs on every line outside a fence.
func setextLevel(line string) int {
	i := leadingSpaces(line)
	if i > 3 || i >= len(line) {
		return 0
	}
	c := line[i]
	if c != '=' && c != '-' {
		return 0
	}
	for i < len(line) && line[i] == c {
		i++
	}
	for ; i < len(line); i++ {
		switch line[i] {
		case ' ', '\t', '\n', '\f', '\r':
		default:
			return 0
		}
	}
	if c == '=' {
		return 1
	}
	return 2
}

// atxHeading reports an ATX heading line as goldmark reads one: up to
// three spaces of indentation, one to six '#', then whitespace or the
// line end. It returns the heading level (0 when line is no ATX
// heading) and the indentation's byte count.
func atxHeading(line string) (level, indent int) {
	indent = leadingSpaces(line)
	if indent > 3 {
		return 0, 0
	}
	n := indent
	for n < len(line) && line[n] == '#' {
		n++
	}
	level = n - indent
	if level == 0 || level > 6 {
		return 0, 0
	}
	if n < len(line) {
		switch line[n] {
		case ' ', '\t', '\n', '\r':
		default:
			return 0, 0
		}
	}
	return level, indent
}

// leadingSpaces returns the number of leading space bytes of line.
func leadingSpaces(line string) int {
	n := 0
	for n < len(line) && line[n] == ' ' {
		n++
	}
	return n
}

// applyShift applies the heading level shift to all headings, converting
// setext headings to ATX when shifted. Lines inside code fences, HTML
// blocks, and processing instructions are kept as they are.
func applyShift(lines []string, shift int) []string {
	result := make([]string, 0, len(lines))
	var scan headingScan

	for i, line := range lines {
		level, text := scan.step(line)
		switch {
		case level == 0:
			result = append(result, line)
		case text > 0:
			// The scan reports a setext underline only after a paragraph,
			// whose text lines the previous iterations appended unchanged:
			// replace them with one ATX heading and drop the underline.
			heading := strings.Repeat("#", clampLevel(level+shift)) + " " + setextText(lines[i-text:i])
			result = append(result[:len(result)-text], heading)
		default:
			// The scan reported an ATX heading of level '#'s after indent
			// spaces; keep the indentation and the text after the run.
			indent := leadingSpaces(line)
			rest := line[indent+level:]
			if rest == "" {
				rest = " "
			}
			result = append(result, line[:indent]+strings.Repeat("#", clampLevel(level+shift))+rest)
		}
	}

	return result
}

// setextText returns the text of a setext heading whose paragraph
// lines are text, as one ATX heading line. A single line is kept as it
// is. Several lines, which an ATX heading cannot hold, are trimmed of
// surrounding spaces and tabs and joined by a space, as a renderer joins
// a heading's soft line breaks; a CRLF ending on the last line is kept.
func setextText(text []string) string {
	if len(text) == 1 {
		return text[0]
	}
	parts := make([]string, len(text))
	for i, l := range text {
		parts[i] = strings.Trim(l, " \t\r")
	}
	joined := strings.Join(parts, " ")
	if strings.HasSuffix(text[len(text)-1], "\r") {
		joined += "\r"
	}
	return joined
}

// clampLevel ensures a heading level is between 1 and 6.
func clampLevel(level int) int {
	if level < 1 {
		return 1
	}
	if level > 6 {
		return 6
	}
	return level
}
