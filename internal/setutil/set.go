// Package setutil provides helpers for map[string]struct{} sets.
package setutil

// Contains reports whether key is present in m.
func Contains(m map[string]struct{}, key string) bool {
	_, ok := m[key]
	return ok
}

// FromStrings builds a set from values with no transformation applied.
func FromStrings(values []string) map[string]struct{} {
	m := make(map[string]struct{}, len(values))
	for _, v := range values {
		m[v] = struct{}{}
	}
	return m
}
