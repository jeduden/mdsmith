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

// codeFenceRe matches the opening of a fenced code block after leading
// whitespace has been stripped. Unlike the CommonMark spec (which
// limits indent to 3 spaces), we strip all leading whitespace so that
// fenced blocks inside list items are also detected and skipped.
var codeFenceRe = regexp.MustCompile("^(`{3,}|~{3,})")

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

// findMinHeadingLevel scans lines and returns the minimum heading level found,
// ignoring lines inside fenced code blocks. Returns 0 if no headings are found.
func findMinHeadingLevel(lines []string) int {
	minLevel := 0
	inFence := false
	fenceMarker := ""

	for i, line := range lines {
		if inFence {
			if isClosingFence(line, fenceMarker) {
				inFence = false
				fenceMarker = ""
			}
			continue
		}

		if m := codeFenceRe.FindStringSubmatch(strings.TrimLeft(line, " \t")); m != nil {
			inFence = true
			fenceMarker = m[1]
			continue
		}

		level := headingLevel(lines, i, line)
		if level > 0 && (minLevel == 0 || level < minLevel) {
			minLevel = level
		}
	}

	return minLevel
}

// headingLevel returns the heading level of line at index i, or 0 if not a heading.
func headingLevel(lines []string, i int, line string) int {
	if m := atxRe.FindStringSubmatch(line); m != nil {
		return len(m[1])
	}
	if i > 0 && lines[i-1] != "" {
		if setextH1Re.MatchString(line) {
			return 1
		}
		if setextH2Re.MatchString(line) {
			return 2
		}
	}
	return 0
}

// applyShift applies the heading level shift to all headings, converting
// setext headings to ATX when shifted. Lines inside code fences are skipped.
func applyShift(lines []string, shift int) []string {
	result := make([]string, 0, len(lines))
	inFence := false
	fenceMarker := ""

	for i, line := range lines {
		if inFence {
			if isClosingFence(line, fenceMarker) {
				inFence = false
				fenceMarker = ""
			}
			result = append(result, line)
			continue
		}

		if m := codeFenceRe.FindStringSubmatch(strings.TrimLeft(line, " \t")); m != nil {
			inFence = true
			fenceMarker = m[1]
			result = append(result, line)
			continue
		}

		// Check setext heading (must check before appending the line,
		// because we may need to replace the previous line and skip this one).
		if i > 0 && !isResultPrevLineFence(result) {
			prevOriginal := lines[i-1]
			if prevOriginal != "" {
				if setextH1Re.MatchString(line) {
					newLevel := clampLevel(1 + shift)
					// Replace previous line (the heading text) with ATX heading.
					result[len(result)-1] = strings.Repeat("#", newLevel) + " " + prevOriginal
					// Skip the underline.
					continue
				}
				if setextH2Re.MatchString(line) {
					newLevel := clampLevel(2 + shift)
					result[len(result)-1] = strings.Repeat("#", newLevel) + " " + prevOriginal
					continue
				}
			}
		}

		// Check ATX heading.
		if m := atxRe.FindStringSubmatch(line); m != nil {
			oldLevel := len(m[1])
			newLevel := clampLevel(oldLevel + shift)
			rest := m[2]
			if rest == "" {
				rest = " "
			}
			result = append(result, strings.Repeat("#", newLevel)+rest)
			continue
		}

		result = append(result, line)
	}

	return result
}

// isClosingFence checks if a line closes a code fence opened with the given marker.
// Leading whitespace is stripped (any amount) to handle fences inside list items.
func isClosingFence(line, marker string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	trimmed = strings.TrimRight(trimmed, " \t")
	if len(trimmed) < len(marker) {
		return false
	}
	ch := marker[0]
	for _, c := range []byte(trimmed) {
		if c != ch {
			return false
		}
	}
	return true
}

// isResultPrevLineFence checks if the last line appended to result was a code
// fence opening. This prevents treating lines after a fence marker as setext.
// This is a conservative check; it won't catch all edge cases.
func isResultPrevLineFence(result []string) bool {
	if len(result) == 0 {
		return false
	}
	return codeFenceRe.MatchString(strings.TrimLeft(result[len(result)-1], " \t"))
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
