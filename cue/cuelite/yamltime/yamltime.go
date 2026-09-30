// Package yamltime renders a decoded YAML timestamp as text. yaml.v3
// turns an unquoted front-matter date or timestamp (`date: 2026-01-02`)
// into a time.Time. Every mdsmith surface that needs that value as text
// renders it through [Format], so all of them show and check the same
// string:
//
//   - cue/cuelite lifts it into CUE as this string, so a front-matter
//     schema, a `list query` filter, a catalog `where:` filter, and a
//     catalog `row-expr:` all see it;
//   - internal/fieldinterp's Stringify renders it for catalog `{field}`
//     rows, `{field}` heading and body sync, and `\#(fmvar(...))`
//     substitution.
//
// It lives beside cue/cuelite, not in internal/, because cuelite imports
// no internal package. It depends only on the standard library.
package yamltime

import "time"

// Format renders t in one of two canonical forms. yaml.v3 turns an
// unquoted `date: 2026-01-02` into midnight UTC, so a value with no
// clock part and a zero UTC offset renders date-only (YYYY-MM-DD), as
// written. Any other value renders as RFC 3339 with fractional seconds
// only when the value has them. That is a normalised form, not the
// source text: a space-separated or zone-less `2026-01-02 10:00:00`
// renders `2026-01-02T10:00:00Z`. Go's `%v` form
// (`2026-01-02 00:00:00 +0000 UTC`) would otherwise leak into catalog
// rows, heading sync, `fmvar(...)` globs, and CUE checks, where it
// matches nothing.
//
// The symbol index (internal/index) formats a timestamp differently:
// always time.RFC3339, so a date renders `2026-01-02T00:00:00Z` there
// and fractional seconds are dropped. The two agree only on a time off
// midnight UTC with whole seconds.
//
// A timestamp written as exactly midnight UTC (`2026-01-02T00:00:00Z`)
// decodes to the same value as the bare date and renders date-only too.
func Format(t time.Time) string {
	if _, off := t.Zone(); off == 0 && t.Hour() == 0 &&
		t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0 {
		return t.Format(time.DateOnly)
	}
	return t.Format(time.RFC3339Nano)
}
