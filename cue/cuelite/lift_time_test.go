package cuelite

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// yaml.v3 decodes an unquoted front-matter timestamp (`date: 2026-01-02`)
// into a time.Time. The lifter turns it into a concrete CUE string in the
// same text the rest of mdsmith renders for it: the date alone at midnight
// UTC, RFC 3339 otherwise, with fractional seconds only when present.
func TestLiftMap_TimeLiftsAsRenderedString(t *testing.T) {
	cases := []struct {
		name string
		in   time.Time
		want string
	}{
		{"midnight UTC", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), "2026-01-02"},
		{"with zone", time.Date(2026, 1, 2, 15, 4, 5, 0, time.FixedZone("", 2*3600)),
			"2026-01-02T15:04:05+02:00"},
		{"midnight with zone", time.Date(2026, 1, 2, 0, 0, 0, 0, time.FixedZone("", -5*3600)),
			"2026-01-02T00:00:00-05:00"},
		{"fractional seconds", time.Date(2026, 1, 2, 15, 4, 5, 250_000_000, time.UTC),
			"2026-01-02T15:04:05.25Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := LiftMap(map[string]any{"date": tc.in})
			require.NoError(t, v.Err())
			leaf, ok := v.LookupPath(MakePath("date"))
			require.True(t, ok)
			got, err := leaf.String()
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// A time nested in a list or struct lifts the same way, since the lifter
// recurses through both.
func TestLiftMap_NestedTimeLifts(t *testing.T) {
	d := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	v := LiftMap(map[string]any{
		"meta":  map[string]any{"date": d},
		"dates": []any{d},
	})
	require.NoError(t, v.Err())
	schema, err := Compile(`{meta: {date: "2026-01-02"}, dates: ["2026-01-02"]}`)
	require.NoError(t, err)
	assert.NoError(t, v.Unify(schema).Validate())
}

// A schema sees the lifted text: `string` accepts the time, a regex
// constraint matches the rendered date, and `int` rejects it as a string.
func TestCompileMap_TimeAgainstConstraints(t *testing.T) {
	fm := map[string]any{"date": time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)}
	for _, src := range []string{
		`{date: string}`,
		`{date: =~"^2026-01-02$"}`,
	} {
		schema, err := Compile(src)
		require.NoError(t, err, src)
		assert.NoError(t, schema.CompileMap(fm).Validate(), src)
	}

	schema, err := Compile(`{date: int}`)
	require.NoError(t, err)
	verr := schema.CompileMap(fm).Validate()
	require.Error(t, verr)
	assert.NotContains(t, verr.Error(), "unsupported front-matter value")
	errs := Errors(verr)
	require.NotEmpty(t, errs)
	assert.Equal(t, []string{"date"}, errs[0].Path())
}
