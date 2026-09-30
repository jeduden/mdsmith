package fieldinterp

import (
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Stringify renders a timestamp in the form the author wrote, so
// `2026-01-02`, `...Z` and `...-05:00` values do not order
// chronologically as strings. ResolveSortKey keys a time.Time on the
// instant itself, normalised to UTC with a fixed-width clock part, so
// a plain string compare of the keys is a chronological compare.
func TestResolveSortKey_TimestampsOrderChronologically(t *testing.T) {
	// Listed in chronological order.
	docs := []string{
		"v: 2026-01-01",
		"v: 2026-01-02T01:00:00+05:00", // 2026-01-01T20:00Z
		"v: 2026-01-02",                // 2026-01-02T00:00Z
		"v: 2026-01-02T09:00:00.5Z",
		"v: 2026-01-02T09:00:01Z",
		"v: 2026-01-02T10:00:00-05:00", // 2026-01-02T15:00Z
		"v: 2026-01-03T00:00:00.000000001Z",
	}
	keys := make([]string, len(docs))
	for i, d := range docs {
		var m map[string]any
		require.NoError(t, yaml.Unmarshal([]byte(d), &m))
		require.IsType(t, time.Time{}, m["v"], d)
		k, err := ResolveSortKey(m, []string{"v"})
		require.NoError(t, err)
		keys[i] = k
	}
	assert.True(t, sort.StringsAreSorted(keys), "keys: %q", keys)
}

// Midnight UTC keys date-only, so an unquoted `date: 2026-01-02` ties
// with a quoted `"2026-01-02"` on the same text, as it did when both
// sorted on their rendered form.
func TestResolveSortKey_MidnightUTCKeysAsTheDate(t *testing.T) {
	for _, v := range []any{
		time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 2, 5, 0, 0, 0, time.FixedZone("", 5*3600)),
		"2026-01-02",
	} {
		k, err := ResolveSortKey(map[string]any{"v": v}, []string{"v"})
		require.NoError(t, err)
		assert.Equal(t, "2026-01-02", k, "%v", v)
	}
}

// Every other value keys exactly as ResolvePath renders it, and the
// errors are ResolvePath's.
func TestResolveSortKey_NonTimeMatchesResolvePath(t *testing.T) {
	data := map[string]any{
		"s": "Alpha", "n": 3, "f": 1.5, "b": true, "z": nil,
		"m": map[string]any{"k": "v"}, "l": []any{"a"},
	}
	for _, key := range []string{"s", "n", "f", "b", "z", "m", "l", "absent"} {
		want, wantErr := ResolvePath(data, []string{key})
		got, err := ResolveSortKey(data, []string{key})
		assert.Equal(t, want, got, key)
		assert.Equal(t, wantErr, err, key)
	}
	got, err := ResolveSortKey(data, []string{"m", "k"})
	require.NoError(t, err)
	assert.Equal(t, "v", got)
}
