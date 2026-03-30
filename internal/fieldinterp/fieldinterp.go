// Package fieldinterp provides {field} placeholder interpolation with
// CUE path resolution for nested front-matter access.
//
// Placeholders use the syntax {fieldname}. Nested access uses dot notation
// ({a.b.c}) and non-identifier keys use CUE quoting ({"my-key".sub}).
// A literal { is written as {{, and a literal } is written as }}.
// Missing keys resolve to empty string.
package fieldinterp

import (
	"fmt"
	"regexp"
	"strings"

	"cuelang.org/go/cue"
)

// fieldPattern matches a single-brace placeholder using CUE path
// syntax. Each segment is either a CUE identifier (\w+) or a quoted
// label ("..."). Quoted labels may not contain } (even escaped) since
// Validate uses a simple } scan to find the placeholder end.
// Non-identifier keys (hyphens, dots, spaces) must be quoted: {"my-key"}.
var fieldPattern = regexp.MustCompile(`\{((?:\w+|"(?:[^"\\}]|\\[^}])+")(?:\.(?:\w+|"(?:[^"\\}]|\\[^}])+"))*)\}`)

// Interpolate replaces {field} placeholders in text with values resolved
// from data using CUE path semantics. Supports nested access ({a.b}) and
// quoted keys ({"my-key"}). Missing keys resolve to empty string.
func Interpolate(text string, data map[string]any) string {
	const openSentinel = "\x00OPEN\x00"
	const closeSentinel = "\x00CLOSE\x00"

	s := strings.ReplaceAll(text, "{{", openSentinel)
	s = strings.ReplaceAll(s, "}}", closeSentinel)

	s = fieldPattern.ReplaceAllStringFunc(s, func(match string) string {
		expr := match[1 : len(match)-1]
		path := ParseCUEPath(expr)
		if len(path) == 0 || data == nil {
			return ""
		}
		val, err := ResolvePath(data, path)
		if err != nil {
			return ""
		}
		return val
	})

	s = strings.ReplaceAll(s, openSentinel, "{")
	s = strings.ReplaceAll(s, closeSentinel, "}")
	return s
}

// Fields returns the field names referenced by {field} placeholders in text.
// Escaped braces {{ are ignored. For nested paths like {a.b}, returns "a.b".
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
// if there are unclosed braces, stray closing braces, or invalid CUE paths.
// Non-identifier keys must be quoted: {"my-key"} not {my-key}.
func Validate(text string) error {
	for i := 0; i < len(text); {
		if text[i] == '{' {
			if i+1 < len(text) && text[i+1] == '{' {
				i += 2 // escaped {{
				continue
			}
			end := strings.IndexByte(text[i+1:], '}')
			if end < 0 {
				return fmt.Errorf("unclosed placeholder at position %d", i)
			}
			field := text[i+1 : i+1+end]
			if !fieldPattern.MatchString("{" + field + "}") {
				return fmt.Errorf("invalid placeholder %q at position %d", "{"+field+"}", i)
			}
			path := ParseCUEPath(field)
			if path == nil {
				return fmt.Errorf(
					"invalid CUE path %q at position %d; "+
						"non-identifier keys must be quoted, e.g. {\"my-key\"} not {my-key}",
					field, i)
			}
			i = i + 1 + end + 1 // skip past }
			continue
		}
		if text[i] == '}' {
			if i+1 < len(text) && text[i+1] == '}' {
				i += 2 // escaped }}
				continue
			}
			return fmt.Errorf("stray closing brace at position %d", i)
		}
		i++
	}
	return nil
}

// ParseCUEPath parses a CUE path expression into unquoted label
// segments using cue.ParsePath. Non-identifier keys (hyphens, dots,
// spaces) must be quoted: "my-key". Returns nil for malformed
// expressions.
func ParseCUEPath(expr string) []string {
	if expr == "" {
		return nil
	}
	p := cue.ParsePath(expr)
	if p.Err() != nil {
		return nil
	}
	sels := p.Selectors()
	if len(sels) == 0 {
		return nil
	}
	segments := make([]string, len(sels))
	for i, s := range sels {
		u := s.Unquoted()
		if u == "" {
			return nil // reject empty labels
		}
		segments[i] = u
	}
	return segments
}

// ResolvePath walks data using the given path segments and returns
// the string value at the resolved location.
func ResolvePath(data map[string]any, path []string) (string, error) {
	if len(path) == 0 {
		return "", fmt.Errorf("empty path")
	}
	if data == nil {
		return "", fmt.Errorf("front-matter key %q not found", strings.Join(path, "."))
	}

	current := any(data)
	for i, seg := range path {
		m, ok := current.(map[string]any)
		if !ok {
			return "", fmt.Errorf("front-matter key %q is not a map", strings.Join(path[:i], "."))
		}
		val, exists := m[seg]
		if !exists {
			return "", fmt.Errorf("front-matter key %q not found", strings.Join(path[:i+1], "."))
		}
		current = val
	}

	// Reject composite leaf values so callers can treat them as invalid paths.
	switch current.(type) {
	case map[string]any, []any:
		return "", fmt.Errorf("front-matter key %q is a composite value", strings.Join(path, "."))
	}

	return Stringify(current), nil
}

// Stringify converts a scalar value to a string representation.
// Maps and slices return empty string to avoid nondeterministic output.
func Stringify(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	case bool:
		return fmt.Sprintf("%v", x)
	case int:
		return fmt.Sprintf("%d", x)
	case int64:
		return fmt.Sprintf("%d", x)
	case float64:
		return fmt.Sprintf("%g", x)
	case map[string]any, []any:
		return "" // composite types produce nondeterministic output
	default:
		return fmt.Sprintf("%v", x)
	}
}

// DiagnoseYAMLQuoting checks whether a raw YAML value that was expected
// to be a string was instead parsed as a map because YAML interpreted
// {field} placeholder syntax as a flow mapping. Returns a diagnostic
// message if the conflict is detected, empty string otherwise.
func DiagnoseYAMLQuoting(paramName string, val any) string {
	if val == nil {
		return ""
	}
	m, ok := val.(map[string]any)
	if !ok {
		return ""
	}

	// Heuristic: only diagnose maps that look like a flow-mapping parse of
	// a placeholder, which commonly yields keys with nil values (e.g.
	// {title} → map["title":nil]). If any value is non-nil, this is likely
	// a genuine map-typed value — defer to generic non-string handling.
	var keys []string
	for k, v := range m {
		if v != nil {
			return ""
		}
		keys = append(keys, k)
	}

	var example string
	if len(keys) == 1 {
		example = "{" + keys[0] + "}"
	} else {
		example = "{...}"
	}

	return fmt.Sprintf(
		"%q value contains a YAML flow mapping where a {field} placeholder was likely intended; "+
			"YAML interprets %s as a mapping — quote the value, e.g. %s: '%s'",
		paramName, example, paramName, example)
}
