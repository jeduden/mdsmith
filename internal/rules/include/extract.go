package include

import (
	"fmt"
	"strings"
)

// extractContentKeys lists the well-known projection keys that mark
// an object as a "leaf" content carrier. When a path lookup ends at
// an object holding exactly one of these keys (and no siblings),
// walkExtractPath returns the inner value so callers do not have to
// know which projection rule produced the wrapper. The set mirrors
// the default key names contentBaseKey returns in internal/extract.
var extractContentKeys = []string{"text", "code", "items", "rows"}

// walkExtractPath walks a dotted path through the extract JSON tree
// rooted at data. Empty path segments and missing keys are errors;
// the final value is returned as-is unless it is a single-content-key
// wrapper object, in which case the inner content is returned.
//
// Examples (with data = {"tagline": {"text": "Hello"}}):
//
//	walkExtractPath(data, "tagline.text") -> "Hello"
//	walkExtractPath(data, "tagline")      -> "Hello" (single content key)
//
// Multi-key objects with no single content key are ambiguous and
// surface as an error so callers can require a more specific path.
func walkExtractPath(data map[string]any, dotted string) (any, error) {
	dotted = strings.TrimSpace(dotted)
	if dotted == "" {
		return nil, fmt.Errorf("path is empty")
	}
	parts := strings.Split(dotted, ".")
	var cur any = data
	for i, p := range parts {
		if p == "" {
			return nil, fmt.Errorf("path has empty segment at position %d", i+1)
		}
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(
				"cannot descend into %q: parent is %T, not an object",
				p, cur)
		}
		next, found := obj[p]
		if !found {
			return nil, fmt.Errorf("missing key %q in extract projection", p)
		}
		cur = next
	}
	// If the final value is an object, try the single-content-key
	// fallback so callers do not have to spell out ".text" when the
	// only thing the section carries is its paragraph content.
	if obj, ok := cur.(map[string]any); ok {
		inner, err := pickContentKey(obj)
		if err != nil {
			return nil, err
		}
		return inner, nil
	}
	return cur, nil
}

// pickContentKey returns obj's content payload when obj is a single
// well-known content wrapper ({"text": ...} or {"code": ...} etc.),
// or surfaces an "ambiguous object" error otherwise. The error
// message lists the keys present so the user can pick one.
func pickContentKey(obj map[string]any) (any, error) {
	if len(obj) == 1 {
		for k, v := range obj {
			if isContentKey(k) {
				return v, nil
			}
			return nil, fmt.Errorf(
				"ambiguous object target: single key %q is not a "+
					"recognised content key; "+
					"append %q to the extract path", k, "."+k)
		}
	}
	// Multiple keys: see if exactly one is a content key.
	var hit string
	hits := 0
	for k := range obj {
		if isContentKey(k) {
			hits++
			hit = k
		}
	}
	if hits == 1 {
		return obj[hit], nil
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	return nil, fmt.Errorf(
		"ambiguous object target with keys %v; "+
			"pick a leaf with a more specific extract path", keys)
}

// isContentKey reports whether key is one of the well-known content
// projection names from internal/extract's contentBaseKey.
func isContentKey(key string) bool {
	for _, k := range extractContentKeys {
		if k == key {
			return true
		}
	}
	return false
}
