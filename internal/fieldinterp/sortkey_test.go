package fieldinterp

import (
	"sort"
	"strings"
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

// Every form of one instant keys the same: an unquoted date, a quoted
// one, midnight written with an offset, and the quoted clock forms.
func TestResolveSortKey_SameInstantKeysTheSame(t *testing.T) {
	for _, v := range []any{
		time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 2, 5, 0, 0, 0, time.FixedZone("", 5*3600)),
		"2026-01-02",
		"2026-01-02T00:00",
		"2026-01-02T00:00:00",
		"2026-01-02 00:00:00",
		"2026-01-02T00:00:00Z",
		"2026-01-02T05:00:00+05:00",
	} {
		k, err := ResolveSortKey(map[string]any{"v": v}, []string{"v"})
		require.NoError(t, err)
		assert.Equal(t, "2026-01-02 00:00:00.000000000", k, "%v", v)
	}
}

// One sort column may mix every date form: unquoted YAML timestamps,
// quoted RFC 3339, a quoted `YYYY-MM-DDTHH:MM`, a quoted YAML-style
// `YYYY-MM-DD HH:MM:SS` and bare dates. Each keys on its UTC instant,
// a zone-less value counting as UTC as YAML and time.Parse read it.
// The catalog lowercases every key before it compares, so the keys
// must still order after strings.ToLower.
func TestResolveSortKey_MixedDateFormsOrderChronologically(t *testing.T) {
	// Listed in chronological order.
	docs := []string{
		`v: "2026-01-01"`,
		"v: 2026-01-02T01:00:00+05:00", // 2026-01-01T20:00Z
		"v: 2026-01-02",
		`v: "2026-01-02 09:00:00"`,
		"v: 2026-01-02T09:30:00Z",
		`v: "2026-01-02T10:00"`,
		"v: 2026-01-02 10:30:00",
		`v: "2026-01-02T11:00:00+00:00"`,
		"v: 2026-01-02T11:15",            // not a YAML timestamp: a string
		`v: "2026-01-02T10:00:00-05:00"`, // 15:00Z
		`v: "2026-01-02T15:00:00.5"`,
		"v: 2026-01-03T00:00:00.000000001Z",
	}
	keys := make([]string, len(docs))
	for i, d := range docs {
		var m map[string]any
		require.NoError(t, yaml.Unmarshal([]byte(d), &m))
		k, err := ResolveSortKey(m, []string{"v"})
		require.NoError(t, err)
		assert.Equal(t, k, strings.ToLower(k), "ToLower must not change a time key")
		keys[i] = strings.ToLower(k)
	}
	assert.True(t, sort.StringsAreSorted(keys), "keys: %q", keys)
}

// A time key is the UTC instant in one fixed-width form with no
// letter, so lowercasing leaves it as is and a byte compare is a
// chronological one.
func TestTimeSortKey(t *testing.T) {
	assert.Equal(t, "2026-01-02 15:00:00.000000000", timeSortKey(
		time.Date(2026, 1, 2, 10, 0, 0, 0, time.FixedZone("", -5*3600))))
	assert.Equal(t, "2026-01-02 00:00:00.000000000",
		timeSortKey(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)))
	assert.Equal(t, "0999-12-31 23:59:59.500000000",
		timeSortKey(time.Date(999, 12, 31, 23, 59, 59, 5e8, time.UTC)))
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

// Only a string that parses as a date or timestamp is keyed as a
// time. Every other string keys as the text ResolvePath returns, so a
// plain-text sort column orders exactly as before.
func TestResolveSortKey_PlainTextKeysAsWritten(t *testing.T) {
	for _, s := range []string{
		"Alpha", "beta", "", "12:00", "2026", "2026 roadmap",
		"2026-01-02 notes", "2026-01-02T", "2026-01-02Tnope",
		"2026-01-02t10:00:00Z", "2026-01-02 10:00", "2026-13-45",
		"2026/01/02", "20260102", "2026-01-02x",
	} {
		data := map[string]any{"v": s}
		want, err := ResolvePath(data, []string{"v"})
		require.NoError(t, err)
		got, err := ResolveSortKey(data, []string{"v"})
		require.NoError(t, err)
		assert.Equal(t, want, got, "%q", s)
	}
}

func TestParseSortTime(t *testing.T) {
	utc := func(h, m, sec, ns int) time.Time {
		return time.Date(2026, 1, 2, h, m, sec, ns, time.UTC)
	}
	for in, want := range map[string]time.Time{
		"2026-01-02":                utc(0, 0, 0, 0),
		"2026-01-02T10:00:00-05:00": utc(15, 0, 0, 0),
		"2026-01-02T10:00:00.5Z":    utc(10, 0, 0, 5e8),
		"2026-01-02T10:00:00":       utc(10, 0, 0, 0),
		"2026-01-02T10:00":          utc(10, 0, 0, 0),
		"2026-01-02T10:00+01:00":    utc(9, 0, 0, 0),
		"2026-01-02 10:00:00":       utc(10, 0, 0, 0),
		"2026-01-02 10:00:00.25":    utc(10, 0, 0, 25e7),
		"2026-01-02 10:00:00-05:00": utc(15, 0, 0, 0),
		"2026-01-02 10:00:00.5Z":    utc(10, 0, 0, 5e8),
	} {
		got, ok := parseSortTime(in)
		require.True(t, ok, in)
		assert.True(t, got.Equal(want), "%s: got %v", in, got)
	}
	for _, s := range []string{
		"", "Alpha", "2026", "2026-01-0", "2026/01/02", "2026-13-02",
		"2026-01-02Tx", "2026-01-02 10:00", "2026-01-02_10:00:00",
		"2026-01-02t10:00:00Z", "2026-01-02T10:00+",
	} {
		_, ok := parseSortTime(s)
		assert.False(t, ok, s)
	}
}

// resolveScalar returns the leaf value as decoded, not its string
// form, and rejects every path ResolvePath rejects.
func TestResolveScalar(t *testing.T) {
	ts := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	data := map[string]any{
		"n": 3, "t": ts, "s": "x",
		"m": map[string]any{"k": true}, "l": []any{"a"},
	}
	for path, want := range map[string]any{"n": 3, "t": ts, "s": "x"} {
		got, err := resolveScalar(data, []string{path})
		require.NoError(t, err, path)
		assert.Equal(t, want, got, path)
	}
	got, err := resolveScalar(data, []string{"m", "k"})
	require.NoError(t, err)
	assert.Equal(t, true, got)

	_, err = resolveScalar(data, nil)
	assert.EqualError(t, err, "empty path")
	_, err = resolveScalar(nil, []string{"n"})
	assert.EqualError(t, err, `front-matter key "n" not found`)
	_, err = resolveScalar(data, []string{"absent"})
	assert.EqualError(t, err, `front-matter key "absent" not found`)
	_, err = resolveScalar(data, []string{"s", "k"})
	assert.EqualError(t, err, `front-matter key "s" is not a map`)
	assert.ErrorIs(t, err, ErrNotMap)
	_, err = resolveScalar(data, []string{"l"})
	assert.ErrorIs(t, err, ErrCompositeValue)
}
