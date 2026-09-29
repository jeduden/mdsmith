package fieldinterp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// yaml.v3 decodes an unquoted YAML timestamp into a time.Time, so a
// front-matter `date: 2026-01-02` reaches Stringify as a time value,
// not as the text the author wrote. Stringify must render it back in
// that written form: a date-only value as YYYY-MM-DD, and a value
// with a clock part as RFC 3339. Go's default `%v` form
// (`2026-01-02 00:00:00 +0000 UTC`) matches no filename and carries
// spaces and colons into a path segment.
func TestStringify_YAMLTimestampRendersAsWritten(t *testing.T) {
	cases := []struct {
		name, yaml, want string
	}{
		{"date only", "v: 2026-01-02", "2026-01-02"},
		{"single-digit month and day", "v: 2026-1-2", "2026-01-02"},
		{"UTC timestamp", "v: 2026-01-02T15:04:05Z", "2026-01-02T15:04:05Z"},
		{"offset timestamp", "v: 2026-01-02T15:04:05+02:00",
			"2026-01-02T15:04:05+02:00"},
		{"fractional seconds", "v: 2026-01-02T15:04:05.25Z",
			"2026-01-02T15:04:05.25Z"},
		{"space-separated, no zone", "v: 2026-01-02 15:04:05",
			"2026-01-02T15:04:05Z"},
		// Once decoded, a timestamp at exactly midnight UTC is
		// indistinguishable from a date-only value; both render as
		// the date.
		{"midnight UTC", "v: 2026-01-02T00:00:00Z", "2026-01-02"},
		{"midnight with offset", "v: 2026-01-02T00:00:00+02:00",
			"2026-01-02T00:00:00+02:00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var m map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(tc.yaml), &m))
			assert.Equal(t, tc.want, Stringify(m["v"]))
		})
	}
}

// ResolvePath and Interpolate both render through Stringify, so a
// YAML date resolves and interpolates in its written form too.
func TestResolvePathAndInterpolate_YAMLDate(t *testing.T) {
	var m map[string]any
	require.NoError(t, yaml.Unmarshal([]byte("date: 2026-01-02"), &m))
	got, err := ResolvePath(m, []string{"date"})
	require.NoError(t, err)
	assert.Equal(t, "2026-01-02", got)
	assert.Equal(t, "posted 2026-01-02", Interpolate("posted {date}", m))
}
