package yamltime

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFormat(t *testing.T) {
	cases := []struct {
		name string
		in   time.Time
		want string
	}{
		{"midnight UTC renders the date",
			time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), "2026-01-02"},
		{"one second past midnight",
			time.Date(2026, 1, 2, 0, 0, 1, 0, time.UTC), "2026-01-02T00:00:01Z"},
		{"one nanosecond past midnight",
			time.Date(2026, 1, 2, 0, 0, 0, 1, time.UTC), "2026-01-02T00:00:00.000000001Z"},
		{"midnight with a non-zero offset",
			time.Date(2026, 1, 2, 0, 0, 0, 0, time.FixedZone("", 3600)),
			"2026-01-02T00:00:00+01:00"},
		{"zoned clock time",
			time.Date(2026, 1, 2, 15, 4, 5, 0, time.FixedZone("", -5*3600)),
			"2026-01-02T15:04:05-05:00"},
		{"fractional seconds drop trailing zeros",
			time.Date(2026, 1, 2, 15, 4, 5, 500_000_000, time.UTC),
			"2026-01-02T15:04:05.5Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Format(tc.in))
		})
	}
}
