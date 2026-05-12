package config

import "strings"

// matchKindAssignmentEntry reports whether a kind-assignment entry
// matches a given file. An entry's selectors combine with AND:
// when both `glob:` and `fields-present:` are set, the file must
// satisfy both. An entry with no selectors never matches — declaring
// an unconditional kind assignment is not supported by design.
//
// fmFields, when nil, means the caller has no front-matter info; an
// entry with `fields-present:` set won't match in that case, since
// presence cannot be confirmed.
//
// The returned selector string describes which selectors fired and is
// surfaced through provenance output. For an unmatched entry it is
// always the empty string.
func matchKindAssignmentEntry(entry KindAssignmentEntry, filePath string, fmFields map[string]any) (bool, string) {
	hasGlob := len(entry.Patterns()) > 0
	hasFields := len(entry.FieldsPresent) > 0
	if !hasGlob && !hasFields {
		return false, ""
	}
	if hasGlob && !matchesAny(entry.Patterns(), filePath) {
		return false, ""
	}
	if hasFields && !allFieldsPresent(entry.FieldsPresent, fmFields) {
		return false, ""
	}
	return true, formatSelector(entry)
}

// allFieldsPresent reports whether every required key is set to a
// non-null value in fmFields. A nil map means no fields were observed
// — no key can satisfy the predicate.
func allFieldsPresent(required []string, fmFields map[string]any) bool {
	if fmFields == nil {
		return false
	}
	for _, key := range required {
		v, ok := fmFields[key]
		if !ok {
			return false
		}
		if v == nil {
			return false
		}
	}
	return true
}

// formatSelector renders the selectors of a kind-assignment entry
// in a stable form ("glob a,b AND fields-present x,y"). Used by
// provenance output to identify which selector(s) matched.
func formatSelector(entry KindAssignmentEntry) string {
	parts := make([]string, 0, 2)
	if pats := entry.Patterns(); len(pats) > 0 {
		parts = append(parts, "glob "+strings.Join(pats, ","))
	}
	if len(entry.FieldsPresent) > 0 {
		parts = append(parts, "fields-present "+strings.Join(entry.FieldsPresent, ","))
	}
	return strings.Join(parts, " AND ")
}
