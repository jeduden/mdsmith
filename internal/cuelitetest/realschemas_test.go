package cuelitetest

import "testing"

// TestRun_realSchemas sweeps the constraint expressions the repo's real
// schemas use — the cue/types shortcut canonicals (date, url, email,
// nonEmpty, …) and the proto.md / inline-kind frontmatter values
// (.mdsmith.yml) — through the differential harness. Each schema is wrapped
// as the validator does (close({key: expr})) and run with accepting and
// rejecting front-matter data, so the in-house engine and the direct-CUE
// oracle must agree on every real constraint shape the flip now evaluates.
//
// This is the plan-238 acceptance check that "every frontmatter: constraint"
// agrees with CUE, expanded to a representative slice of the actual repo
// schemas (the named-shortcut regexes, the disjunction-with-default fields,
// the bounded ints, the list types, and the cross-field ternary the
// release-channels proto.md declares).
func TestRun_realSchemas(t *testing.T) {
	Run(t, realSchemaCases())
}

// Shared schema sources for the cases whose CUE exceeds one line, lifted to
// constants so each Case row stays under the line-length limit.
const (
	dateSchema     = `close({created: =~"^\\d{4}-\\d{2}-\\d{2}$"})`
	datetimeSchema = `close({ts: =~"^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(Z|[+-]\\d{2}:\\d{2})?$"})`
	emailSchema    = `close({e: =~"^[^@\\s]+@[^@\\s]+\\.[^@\\s]+$"})`
	ternarySchema  = `close({mechanism: "push" | "pull", ` +
		`registry: [if mechanism == "push" {string & != ""}, (string | *"")][0]})`
)

func realSchemaCases() []Case {
	return []Case{
		// cue/types shortcut canonicals.
		{Name: "date ok", Schema: dateSchema, Data: `{"created": "2024-05-01"}`},
		{Name: "date reject", Schema: dateSchema, Data: `{"created": "2024-5-1"}`},
		{Name: "datetime ok", Schema: datetimeSchema, Data: `{"ts": "2024-05-01T12:30:00Z"}`},
		{Name: "datetime reject", Schema: datetimeSchema, Data: `{"ts": "2024-05-01 12:30:00"}`},
		{Name: "time ok", Schema: `close({t: =~"^\\d{2}:\\d{2}(:\\d{2})?$"})`, Data: `{"t": "12:30"}`},
		{Name: "email ok", Schema: emailSchema, Data: `{"e": "user@example.com"}`},
		{Name: "email reject", Schema: emailSchema, Data: `{"e": "user@@example"}`},
		{Name: "url ok", Schema: `close({u: =~"^https?://"})`, Data: `{"u": "https://example.com"}`},
		{Name: "url reject", Schema: `close({u: =~"^https?://"})`, Data: `{"u": "ftp://example.com"}`},
		{Name: "filename ok", Schema: `close({fn: =~"^[A-Za-z0-9._-]+\\.md$"})`, Data: `{"fn": "notes.md"}`},
		{Name: "nonEmpty ok", Schema: `close({s: string & !=""})`, Data: `{"s": "hello"}`},
		{Name: "nonEmpty reject", Schema: `close({s: string & !=""})`, Data: `{"s": ""}`},

		// Inline-kind frontmatter values from .mdsmith.yml.
		{Name: "bounded int ok", Schema: `close({weight: int & >=1})`, Data: `{"weight": 3}`},
		{Name: "bounded int reject", Schema: `close({weight: int & >=1})`, Data: `{"weight": 0}`},
		{Name: "positive int ok", Schema: `close({periodDays: int & > 0})`, Data: `{"periodDays": 30}`},
		{Name: "sha pattern ok", Schema: `close({from: =~"^[0-9a-f]{7,40}$"})`, Data: `{"from": "a1b2c3d"}`},
		{Name: "sha pattern reject", Schema: `close({from: =~"^[0-9a-f]{7,40}$"})`, Data: `{"from": "zzz"}`},

		// release-channels proto.md: disjunctions with defaults, list types,
		// and the cross-field ternary that resolves once mechanism is concrete.
		{Name: "mechanism enum ok", Schema: `close({m: "push" | "pull" | "toolchain"})`, Data: `{"m": "push"}`},
		{Name: "command-windows default", Schema: `close({cw: string | *""})`, Data: `{"cw": "b.ps1"}`},
		{Name: "platforms list ok", Schema: `close({platforms: [...string] | *[]})`, Data: `{"platforms": ["linux"]}`},
		{Name: "unlisted bool ok", Schema: `close({unlisted: bool | *false})`, Data: `{"unlisted": true}`},
		{Name: "ternary push ok", Schema: ternarySchema, Data: `{"mechanism": "push", "registry": "npm"}`},
		{Name: "ternary push empty rejects", Schema: ternarySchema, Data: `{"mechanism": "push", "registry": ""}`},
		{Name: "ternary pull empty ok", Schema: ternarySchema, Data: `{"mechanism": "pull", "registry": ""}`},
	}
}
