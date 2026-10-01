package include

import (
	"regexp"
	"strings"

	"github.com/jeduden/mdsmith/internal/mdfence"
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
// ignoring lines inside fenced code blocks. Returns 0 if no headings are found.
func findMinHeadingLevel(lines []string) int {
	minLevel := 0
	var fence mdfence.Tracker

	for i, line := range lines {
		if stepFence(&fence, line) {
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
	if i > 0 && setextContentLine(lines[i-1]) {
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
	var fence mdfence.Tracker

	for i, line := range lines {
		if stepFence(&fence, line) {
			result = append(result, line)
			continue
		}

		// Check setext heading (must check before appending the line,
		// because we may need to replace the previous line and skip this one).
		// The check reads the last result line, not lines[i-1], so a
		// setext heading already converted to ATX is not re-used as text.
		if len(result) > 0 && setextContentLine(result[len(result)-1]) {
			prevOriginal := lines[i-1]
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

// setextContentLine reports whether prev can carry the text of a setext
// heading whose underline is the next line. CommonMark reads an underline
// after a blank line, an ATX heading, a fence line (opener or closer), or
// another underline as a thematic break or paragraph text, never as a
// setext heading. This is a conservative check; it won't catch all edge
// cases (list items, block quotes).
func setextContentLine(prev string) bool {
	if strings.TrimSpace(prev) == "" || atxRe.MatchString(prev) {
		return false
	}
	if opensFence(prev) {
		return false
	}
	return !setextH1Re.MatchString(prev) && !setextH2Re.MatchString(prev)
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
