// Package fieldinterp provides simple {field} placeholder interpolation.
//
// Placeholders use the syntax {fieldname}. A literal { is written as {{,
// and a literal } is written as }}. Missing keys resolve to empty string.
package fieldinterp

import (
	"fmt"
	"regexp"
	"strings"
)

// fieldPattern matches a single-brace placeholder like {fieldname}.
// Field names may contain word characters and hyphens.
var fieldPattern = regexp.MustCompile(`\{([\w-]+)\}`)

// Interpolate replaces {field} placeholders in text with values from data.
// Missing keys resolve to empty string. Escaped braces {{ and }} produce
// literal { and } respectively.
func Interpolate(text string, data map[string]string) string {
	// First pass: replace escaped braces with sentinel values.
	const openSentinel = "\x00OPEN\x00"
	const closeSentinel = "\x00CLOSE\x00"

	s := strings.ReplaceAll(text, "{{", openSentinel)
	s = strings.ReplaceAll(s, "}}", closeSentinel)

	// Replace {field} placeholders.
	s = fieldPattern.ReplaceAllStringFunc(s, func(match string) string {
		field := match[1 : len(match)-1]
		if data == nil {
			return ""
		}
		if v, ok := data[field]; ok {
			return v
		}
		return ""
	})

	// Restore escaped braces.
	s = strings.ReplaceAll(s, openSentinel, "{")
	s = strings.ReplaceAll(s, closeSentinel, "}")
	return s
}

// Fields returns the field names referenced by {field} placeholders in text.
// Escaped braces {{ are ignored.
func Fields(text string) []string {
	const openSentinel = "\x00OPEN\x00"
	s := strings.ReplaceAll(text, "{{", openSentinel)
	s = strings.ReplaceAll(s, "}}", "")

	matches := fieldPattern.FindAllStringSubmatch(s, -1)
	result := make([]string, 0, len(matches))
	for _, m := range matches {
		result = append(result, m[1])
	}
	return result
}

// ContainsField reports whether text contains at least one {field} placeholder
// (not counting escaped {{ braces).
func ContainsField(text string) bool {
	return len(Fields(text)) > 0
}

// SplitOnFields splits text on {field} placeholders and returns the literal
// parts between them. Escaped braces are treated as literals.
// For "{id}: {name}" it returns ["", ": ", ""].
func SplitOnFields(text string) []string {
	const openSentinel = "\x00OPEN\x00"
	const closeSentinel = "\x00CLOSE\x00"
	s := strings.ReplaceAll(text, "{{", openSentinel)
	s = strings.ReplaceAll(s, "}}", closeSentinel)

	parts := fieldPattern.Split(s, -1)
	for i, p := range parts {
		parts[i] = strings.ReplaceAll(p, openSentinel, "{")
		parts[i] = strings.ReplaceAll(parts[i], closeSentinel, "}")
	}
	return parts
}

// Validate checks that text has valid placeholder syntax. It returns an error
// if there are unclosed braces or empty placeholders like {}.
func Validate(text string) error {
	const openSentinel = "\x00OPEN\x00"
	const closeSentinel = "\x00CLOSE\x00"
	s := strings.ReplaceAll(text, "{{", openSentinel)
	s = strings.ReplaceAll(s, "}}", closeSentinel)

	for i := 0; i < len(s); i++ {
		if s[i] == '{' {
			// Find matching close
			end := strings.IndexByte(s[i+1:], '}')
			if end < 0 {
				return fmt.Errorf("unclosed placeholder at position %d", i)
			}
			field := s[i+1 : i+1+end]
			if field == "" {
				return fmt.Errorf("empty placeholder at position %d", i)
			}
			i = i + 1 + end // skip past }
		}
	}
	return nil
}
