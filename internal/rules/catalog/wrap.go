package catalog

import (
	"strings"

	"github.com/jeduden/mdsmith/internal/fieldinterp"
)

// columnConfig holds per-column width and wrapping configuration.
type columnConfig struct {
	maxWidth int    // maximum width for the column content
	wrap     string // "truncate" (default) or "br"
}

// truncateCell truncates text to maxWidth characters, appending "..." if
// the text is shortened. It preserves markdown links [text](url) and
// inline code `code` by not breaking inside these spans.
func truncateCell(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if len(text) <= maxWidth {
		return text
	}

	spans := parseMarkdownSpans(text)

	if maxWidth < 3 {
		// Not enough room for full ellipsis, return dots up to maxWidth
		return "..."[:maxWidth]
	}

	// Find a good truncation point that respects markdown spans.
	// We need room for "..." (3 chars).
	targetWidth := maxWidth - 3

	// Find the best break point that doesn't split a markdown span.
	breakPos := findBreakPoint(text, spans, targetWidth)

	if breakPos <= 0 {
		// Can't fit anything meaningful, hard truncate
		return text[:targetWidth] + "..."
	}

	return strings.TrimRight(text[:breakPos], " ") + "..."
}

// wrapCellBr wraps text at word boundaries using <br> to fit within
// maxWidth. Falls back to hard character breaks when a single word
// exceeds maxWidth. Preserves markdown links and inline code spans.
func wrapCellBr(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if len(text) <= maxWidth {
		return text
	}

	spans := parseMarkdownSpans(text)
	var lines []string
	remaining := text

	for len(remaining) > maxWidth {
		breakPos := findBreakPoint(remaining, spansInRange(spans, len(text)-len(remaining), len(remaining)), maxWidth)

		if breakPos <= 0 {
			// Hard break at maxWidth
			breakPos = maxWidth
		}

		line := strings.TrimRight(remaining[:breakPos], " ")
		lines = append(lines, line)
		remaining = strings.TrimLeft(remaining[breakPos:], " ")

		// Recalculate spans relative to new remaining position
	}

	if remaining != "" {
		lines = append(lines, remaining)
	}

	return strings.Join(lines, "<br>")
}

// markdownSpan represents a region that should not be broken.
type markdownSpan struct {
	start int // inclusive
	end   int // exclusive
}

// parseMarkdownSpans finds markdown links [text](url) and inline code `code`
// spans in the text and returns their positions.
func parseMarkdownSpans(text string) []markdownSpan {
	var spans []markdownSpan

	i := 0
	for i < len(text) {
		// Check for inline code: `...`
		if text[i] == '`' {
			end := strings.IndexByte(text[i+1:], '`')
			if end >= 0 {
				spanEnd := i + 1 + end + 1
				spans = append(spans, markdownSpan{start: i, end: spanEnd})
				i = spanEnd
				continue
			}
		}

		// Check for markdown link: [text](url)
		if text[i] == '[' {
			// Find closing ]
			closeBracket := findClosingBracket(text, i)
			if closeBracket > i && closeBracket+1 < len(text) && text[closeBracket+1] == '(' {
				// Find closing )
				closeParen := findClosingParen(text, closeBracket+1)
				if closeParen > closeBracket+1 {
					spans = append(spans, markdownSpan{start: i, end: closeParen + 1})
					i = closeParen + 1
					continue
				}
			}
		}

		i++
	}

	return spans
}

// findClosingBracket finds the closing ] for an opening [ at pos.
func findClosingBracket(text string, pos int) int {
	depth := 0
	for i := pos; i < len(text); i++ {
		switch text[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// findClosingParen finds the closing ) for an opening ( at pos.
func findClosingParen(text string, pos int) int {
	depth := 0
	for i := pos; i < len(text); i++ {
		switch text[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// findBreakPoint finds the best position to break text at or before targetWidth,
// respecting markdown spans (not breaking inside them). It prefers word boundaries
// (spaces) but will break before a markdown span if the span exceeds the width.
func findBreakPoint(text string, spans []markdownSpan, targetWidth int) int {
	if targetWidth >= len(text) {
		return len(text)
	}

	// Check if targetWidth falls inside a markdown span
	for _, s := range spans {
		if targetWidth > s.start && targetWidth < s.end {
			// We're inside a span. Try to break before the span.
			if s.start > 0 {
				// Find a word boundary before this span
				breakBefore := lastSpaceBefore(text, s.start)
				if breakBefore > 0 {
					return breakBefore
				}
				return s.start
			}
			// Span starts at 0 and is too long -- hard break at targetWidth
			return targetWidth
		}
	}

	// Not inside a span. Find the last word boundary at or before targetWidth.
	breakPos := lastSpaceBefore(text, targetWidth+1)
	if breakPos > 0 {
		return breakPos
	}

	// No word boundary found, use targetWidth as hard break
	return targetWidth
}

// lastSpaceBefore returns the position of the last space in text before position pos.
// Returns -1 if no space is found.
func lastSpaceBefore(text string, pos int) int {
	if pos > len(text) {
		pos = len(text)
	}
	for i := pos - 1; i >= 0; i-- {
		if text[i] == ' ' {
			return i
		}
	}
	return -1
}

// spansInRange returns spans adjusted to be relative to the start of a substring.
func spansInRange(spans []markdownSpan, offset, end int) []markdownSpan {
	var result []markdownSpan
	for _, s := range spans {
		if s.end <= offset || s.start >= end {
			continue
		}
		adjusted := markdownSpan{
			start: s.start - offset,
			end:   s.end - offset,
		}
		if adjusted.start < 0 {
			adjusted.start = 0
		}
		result = append(result, adjusted)
	}
	return result
}

// applyColumnConstraints applies column width constraints to a table row.
// colMap maps column index (0-based) to column name in the columns config.
// Returns the modified row string.
func applyColumnConstraints(row string, cols map[string]columnConfig, colMap map[int]string) string {
	if len(cols) == 0 || len(colMap) == 0 {
		return row
	}

	// Must be a table row (starts with |)
	if !strings.HasPrefix(strings.TrimSpace(row), "|") {
		return row
	}

	cells := splitTableRow(row)
	if len(cells) == 0 {
		return row
	}

	modified := false
	for idx, colName := range colMap {
		cc, ok := cols[colName]
		if !ok || cc.maxWidth <= 0 || idx >= len(cells) {
			continue
		}

		cellContent := strings.TrimSpace(cells[idx])
		if len(cellContent) <= cc.maxWidth {
			continue
		}

		var newContent string
		if cc.wrap == "br" {
			newContent = wrapCellBr(cellContent, cc.maxWidth)
		} else {
			newContent = truncateCell(cellContent, cc.maxWidth)
		}

		cells[idx] = " " + newContent + " "
		modified = true
	}

	if !modified {
		return row
	}

	return "|" + strings.Join(cells, "|") + "|"
}

// splitTableRow splits a markdown table row into cell contents.
// Input: "| cell1 | cell2 | cell3 |"
// Output: [" cell1 ", " cell2 ", " cell3 "]
func splitTableRow(row string) []string {
	trimmed := strings.TrimSpace(row)
	if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") {
		return nil
	}
	// Remove leading and trailing |
	inner := trimmed[1 : len(trimmed)-1]
	return strings.Split(inner, "|")
}

// buildColumnMap creates a mapping from column index to template field name
// by analyzing the row template. It looks for {fieldname} patterns in
// each column of the row template.
func buildColumnMap(rowTemplate string) map[int]string {
	// The row template should be a table row like:
	// "| {title} | {description} |"
	cells := splitTableRow(rowTemplate)
	if len(cells) == 0 {
		return nil
	}

	result := make(map[int]string)
	for i, cell := range cells {
		cell = strings.TrimSpace(cell)
		// Extract field names from {fieldname} patterns
		// We look for the primary field in the cell
		field := extractPrimaryField(cell)
		if field != "" {
			result[i] = field
		}
	}

	return result
}

// extractPrimaryField extracts the primary template field from a cell template.
// For example, from "{description}" it returns "description".
// For "[{title}]({filename})" it returns "title" (first field).
// For cells with no template fields, returns "".
func extractPrimaryField(cell string) string {
	fields := fieldinterp.Fields(cell)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
