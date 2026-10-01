package include

import (
	"regexp"
	"strings"
)

// atxRe matches an ATX heading line: one or more '#' followed by a space or end of line.
var atxRe = regexp.MustCompile(`^(#{1,6})([ \t].*)?$`)

// setextH1Re matches a setext h1 underline: one or more '=' characters.
var setextH1Re = regexp.MustCompile(`^=+\s*$`)

// setextH2Re matches a setext h2 underline: one or more '-' characters.
var setextH2Re = regexp.MustCompile(`^-+\s*$`)

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
// 2 when it is a setext h2 underline (`-` run), and 0 otherwise.
func setextLevel(line string) int {
	if setextH1Re.MatchString(line) {
		return 1
	}
	if setextH2Re.MatchString(line) {
		return 2
	}
	return 0
}

// applyShift applies the heading level shift to all headings, converting
// setext headings to ATX when shifted. Lines inside code fences, HTML
// blocks, and processing instructions are kept as they are.
func applyShift(lines []string, shift int) []string {
	result := make([]string, 0, len(lines))
	var scan headingScan

	for i, line := range lines {
		level, setext := scan.step(line)
		switch {
		case level == 0:
			result = append(result, line)
		case setext:
			// The scan only reports a setext underline after a paragraph
			// line, which the previous iteration appended unchanged:
			// replace it with an ATX heading and drop the underline.
			result[len(result)-1] = strings.Repeat("#", clampLevel(level+shift)) + " " + lines[i-1]
		default:
			rest := atxRe.FindStringSubmatch(line)[2]
			if rest == "" {
				rest = " "
			}
			result = append(result, strings.Repeat("#", clampLevel(level+shift))+rest)
		}
	}

	return result
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
