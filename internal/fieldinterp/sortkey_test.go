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

// A quoted RFC 3339 string names an instant just as an unquoted YAML
// timestamp does, so one catalog sort column that holds both must
// interleave them chronologically. As plain text the quoted value
// would sort by its own offset and precision: `...10:00:00-05:00`
// (15:00 UTC) before an unquoted 14:00 UTC.
func TestResolveSortKey_QuotedTimestampsInterleaveWithUnquoted(t *testing.T) {
	// Listed in chronological order.
	docs := []string{
		"v: 2026-01-02T09:00:00Z",
		`v: "2026-01-02T09:00:00.5Z"`,
		"v: 2026-01-02T09:00:01Z",
		"v: 2026-01-02T14:00:00Z",
		`v: "2026-01-02T10:00:00-05:00"`,
		"v: 2026-01-02T16:00:00Z",
	}
	keys := make([]string, len(docs))
	for i, d := range docs {
		var m map[string]any
		require.NoError(t, yaml.Unmarshal([]byte(d), &m))
		k, err := ResolveSortKey(m, []string{"v"})
		require.NoError(t, err)
		keys[i] = k
	}
	assert.True(t, sort.StringsAreSorted(keys), "keys: %q", keys)

	quoted, err := ResolveSortKey(
		map[string]any{"v": "2026-01-02T15:00:00Z"}, []string{"v"})
	require.NoError(t, err)
	unquoted, err := ResolveSortKey(map[string]any{
		"v": time.Date(2026, 1, 2, 10, 0, 0, 0, time.FixedZone("", -5*3600)),
	}, []string{"v"})
	require.NoError(t, err)
	assert.Equal(t, unquoted, quoted, "the same instant keys the same either way")
}

// Only a string that parses as RFC 3339 with a clock part is keyed
// as a time. Every other string keys as the text ResolvePath returns,
// so a plain-text sort column orders exactly as before — including a
// quoted `YYYY-MM-DD`, whose text is already its chronological key.
func TestResolveSortKey_PlainTextKeysAsWritten(t *testing.T) {
	for _, s := range []string{
		"Alpha", "beta", "", "12:00", "2026", "2026 roadmap",
		"2026-01-02", "2026-01-02 notes", "2026-01-02T", "2026-01-02Tnope",
		"2026-01-02T10:00", "2026-01-02t10:00:00Z",
	} {
		data := map[string]any{"v": s}
		want, err := ResolvePath(data, []string{"v"})
		require.NoError(t, err)
		got, err := ResolveSortKey(data, []string{"v"})
		require.NoError(t, err)
		assert.Equal(t, want, got, "%q", s)
	}
}

func TestParseRFC3339(t *testing.T) {
	got, ok := parseRFC3339("2026-01-02T10:00:00-05:00")
	require.True(t, ok)
	assert.True(t, got.Equal(time.Date(2026, 1, 2, 15, 0, 0, 0, time.UTC)))
	for _, s := range []string{"2026-01-02", "2026-01-02 10:00:00", "2026-01-02Tx", "Alpha"} {
		_, ok := parseRFC3339(s)
		assert.False(t, ok, s)
	}
}
