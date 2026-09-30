// Package fieldinterp provides {field} placeholder interpolation with
// CUE path resolution for nested front-matter access.
//
// Placeholders use the syntax {fieldname}. Nested access uses dot notation
// ({a.b.c}) and non-identifier keys use CUE quoting ({"my-key".sub}).
// A literal { is written as {{, and a literal } is written as }}.
// Missing keys resolve to empty string.
package fieldinterp

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jeduden/mdsmith/cue/cuelite"
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
// segments using cuelite.ParsePath. Non-identifier keys (hyphens, dots,
// spaces) must be quoted: "my-key". Returns nil for malformed
// expressions.
//
// cuelite.ParsePath never succeeds with zero segments (it rejects the
// empty and whitespace-only expression), so a nil error guarantees a
// non-empty Segments(); the result is returned directly with no
// zero-length guard and no extra copy beyond Segments()' own clone.
func ParseCUEPath(expr string) []string {
	p, err := cuelite.ParsePath(expr)
	if err != nil {
		return nil
	}
	return p.Segments()
}

// ErrCompositeValue is wrapped by ResolvePath's error when the path
// resolves to a list or map rather than a scalar.
var ErrCompositeValue = errors.New("composite value")

// ErrNotMap is wrapped by ResolvePath's error when the path walks
// into a key whose value is not a map, so it cannot hold the rest.
var ErrNotMap = errors.New("not a map")

// ResolvePath walks data using the given path segments and returns
// the string value at the resolved location.
func ResolvePath(data map[string]any, path []string) (string, error) {
	v, err := resolveScalar(data, path)
	if err != nil {
		return "", err
	}
	return Stringify(v), nil
}

// ResolveSortKey is ResolvePath for ordering. It returns the same
// string and error for every value except a date or timestamp, which
// it keys on the instant rather than on its rendered form (see
// timeSortKey). Stringify keeps each timestamp's own offset and
// precision — `2026-01-02`, `...T10:00:00-05:00`, `...T12:00:00Z` —
// and those strings do not compare chronologically, so a caller that
// sorts must use this.
//
// A date or timestamp is a time.Time (an unquoted YAML timestamp) or
// a string that parseSortTime reads, so one sort column may mix
// quoted and unquoted values in any of those forms. Any other string
// keys as its text.
func ResolveSortKey(data map[string]any, path []string) (string, error) {
	v, err := resolveScalar(data, path)
	if err != nil {
		return "", err
	}
	switch x := v.(type) {
	case time.Time:
		return timeSortKey(x), nil
	case string:
		if t, ok := parseSortTime(x); ok {
			return timeSortKey(t), nil
		}
	}
	return Stringify(v), nil
}

// Layouts parseSortTime tries, grouped by the byte after the date. A
// layout with no zone parses as UTC, as YAML reads a zone-less
// timestamp. time.Parse accepts a fractional second after the seconds
// field whether or not the layout shows one.
var (
	sortTimeLayoutsT = []string{
		time.RFC3339,             // 2026-01-02T10:00:00-05:00
		"2006-01-02T15:04:05",    // 2026-01-02T10:00:00
		"2006-01-02T15:04Z07:00", // 2026-01-02T10:00Z
		"2006-01-02T15:04",       // 2026-01-02T10:00
	}
	sortTimeLayoutsSpace = []string{
		time.DateTime, // 2026-01-02 10:00:00, YAML's spaced form
	}
)

// parseSortTime reads s as a date or timestamp: `YYYY-MM-DD`, RFC 3339
// (`2026-01-02T10:00:00-05:00`), RFC 3339 without seconds or without
// a zone (`2026-01-02T10:00`), or YAML's `2026-01-02 10:00:00`. The
// byte checks before the parse keep plain text, which is nearly every
// sort value, off time.Parse.
func parseSortTime(s string) (time.Time, bool) {
	n := len(time.DateOnly)
	if len(s) < n || s[4] != '-' || s[7] != '-' {
		return time.Time{}, false
	}
	var layouts []string
	switch {
	case len(s) == n:
		layouts = []string{time.DateOnly}
	case s[n] == 'T':
		layouts = sortTimeLayoutsT
	case s[n] == ' ':
		layouts = sortTimeLayoutsSpace
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// resolveScalar walks data along path and returns the scalar leaf,
// rejecting an empty path, an absent key, a non-map intermediate and
// a composite leaf.
func resolveScalar(data map[string]any, path []string) (any, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("empty path")
	}
	if data == nil {
		return nil, fmt.Errorf("front-matter key %q not found", strings.Join(path, "."))
	}

	current := any(data)
	for i, seg := range path {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("front-matter key %q is %w", strings.Join(path[:i], "."), ErrNotMap)
		}
		val, exists := m[seg]
		if !exists {
			return nil, fmt.Errorf("front-matter key %q not found", strings.Join(path[:i+1], "."))
		}
		current = val
	}

	// Reject composite leaf values so callers can treat them as invalid
	// paths. The error wraps ErrCompositeValue so a caller can tell a
	// present list or map apart from an absent key.
	switch current.(type) {
	case map[string]any, []any:
		return nil, fmt.Errorf("front-matter key %q is a %w",
			strings.Join(path, "."), ErrCompositeValue)
	}

	return current, nil
}

// Stringify converts a scalar value to a string representation.
// Maps and slices return empty string to avoid nondeterministic output.
// A time.Time — what yaml.v3 decodes an unquoted YAML timestamp into —
// renders through formatTime, as a date or as RFC 3339.
func Stringify(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	case bool:
		return strconv.FormatBool(x)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case time.Time:
		return formatTime(x)
	case map[string]any, []any:
		return "" // composite types produce nondeterministic output
	default:
		return fmt.Sprintf("%v", x)
	}
}

// formatTime renders a decoded YAML timestamp in one of two canonical
// forms. yaml.v3 turns an unquoted `date: 2026-01-02` into midnight
// UTC, so a value with no clock part and a zero UTC offset renders
// date-only (YYYY-MM-DD), as written. Any other value renders as
// RFC 3339 with fractional seconds only when the value has them. That
// is a normalised form, not the source text: a space-separated or
// zone-less `2026-01-02 10:00:00` renders `2026-01-02T10:00:00Z`.
//
// The symbol index (internal/index) formats a timestamp differently:
// always time.RFC3339, so a date renders `2026-01-02T00:00:00Z` there
// and fractional seconds are dropped. The two agree only on a time
// off midnight UTC with whole seconds.
// Go's `%v` form (`2026-01-02 00:00:00 +0000 UTC`) would otherwise
// leak into catalog rows, heading sync, and `fmvar(...)` globs, where
// it matches nothing.
//
// A timestamp written as exactly midnight UTC (`2026-01-02T00:00:00Z`)
// decodes to the same value as the bare date and renders date-only too.
func formatTime(t time.Time) string {
	if _, off := t.Zone(); off == 0 && t.Hour() == 0 &&
		t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0 {
		return t.Format(time.DateOnly)
	}
	return t.Format(time.RFC3339Nano)
}

// timeSortKeyLayout is the UTC instant with a fixed-width, nine-digit
// fraction and no zone suffix. Every key has the same length and
// layout, so a byte compare is a chronological one, and it holds no
// letter, so the catalog's case-folding leaves it unchanged.
const timeSortKeyLayout = "2006-01-02 15:04:05.000000000"

// timeSortKey keys t for a string sort: its UTC instant in
// timeSortKeyLayout, whatever offset or precision the author wrote.
func timeSortKey(t time.Time) string {
	return t.UTC().Format(timeSortKeyLayout)
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
