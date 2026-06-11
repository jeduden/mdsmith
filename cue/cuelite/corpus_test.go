package cuelite

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// corpus_test.go is the engine-only regression corpus ported from the deleted
// internal/cuelitetest differential harness (plan 240). The harness compared
// the in-house engine against a direct-cuelang oracle; once every surface was
// flipped, the oracle's whole purpose ended, so the corpus is preserved here as
// pinned engine-only assertions. The expected accept/reject verdicts and
// rendered strings below were validated against the cuelang oracle while it
// still existed (the differential run was green at the flip), so they are the
// now-canonical behaviour. A regression in the parser or evaluator fails one of
// these rows rather than a differential diff.

// schemaCase is one schema/data validation case: the CUE schema source, the
// JSON data document, and whether the engine accepts (data satisfies schema).
type schemaCase struct {
	name    string
	schema  string
	data    string
	accepts bool
}

// schemaCorpus is the schema/data corpus: representative accept/reject shapes
// (the former cuelitetest corpus() and realSchemaCases()), each pinned to its
// validated verdict.
func schemaCorpus() []schemaCase {
	return []schemaCase{
		// Core shapes (former corpus()).
		{"string ok", `{status: string}`, `{"status": "done"}`, true},
		{"int bound ok", `{n: >=0}`, `{"n": 3}`, true},
		{"int bound reject", `{n: >=0}`, `{"n": -1}`, false},
		{"literal reject", `{status: "✅"}`, `{"status": "🔲"}`, false},
		{"regex ok", `{slug: =~"^[a-z]+$"}`, `{"slug": "abc"}`, true},
		{"regex reject", `{slug: =~"^[a-z]+$"}`, `{"slug": "AB1"}`, false},
		// Raw-string dialect (`#"..."#`): the canonical regex idiom spares the
		// backslashes, and a plain raw literal matches verbatim. These pin the
		// item-1 raw-string round-trip fix.
		{"raw regex ok", `{n: =~#"^\d+$"#}`, `{"n": "123"}`, true},
		{"raw regex reject", `{n: =~#"^\d+$"#}`, `{"n": "12a"}`, false},
		{"raw literal ok", `{p: #"a\b"#}`, `{"p": "a\\b"}`, true},
		{"raw literal reject", `{p: #"a\b"#}`, `{"p": "axb"}`, false},
		{"nested reject", `{meta: {status: "✅"}}`, `{"meta": {"status": "x"}}`, false},
		{"multi-leaf reject", `{a: "x", b: "y"}`, `{"a": "p", "b": "q"}`, false},
		{"string-where-int reject", `{a: string, b: int}`, `{"a": "ok", "b": "x"}`, false},
		{"big-number no duplicate ok", `{x: number}`, `{"x":1e999}`, true},
		// Disjunction-default and bound semantics (former p0 rows).
		{"p0 multiple marks reject", `{a: *string | *""}`, `{}`, false},
		{"p0 equal disjuncts accept", `{a: "x" | "x"}`, `{}`, true},
		{"p0 all-bottom disjunction reject", `{x: 0&1 | 1&0}`, `{"x":0}`, false},
		{"p0 meet of defaults conflicts", `{x: (*1 | int) & (*2 | int)}`, `{}`, false},
		{"p0 nested default carries", `{x: (*1 | 2) | 3}`, `{}`, true},
		{"p0 empty bound reject", `{x: >=10 & <=5}`, `{"x":7}`, false},
		{"p0 numeric cross-kind compare", `{x: 2 == 2.0}`, `{"x":true}`, true},
		{"p0 float accepts float", `{x: float}`, `{"x": 2.0}`, true},
		{"p0 int rejects float", `{x: int}`, `{"x": 2.0}`, false},
		{"p0 list-element thunk", `{mech: string, xs: [mech != ""]}`, `{"mech": "p", "xs": [true]}`, true},
		{"p0 disjunction-branch thunk", `{m: string, x: (m == "a") | "z"}`, `{"m": "a", "x": true}`, true},

		// Real-schema constraints (former realSchemaCases()).
		{"date ok", `close({created: =~"^\\d{4}-\\d{2}-\\d{2}$"})`, `{"created": "2024-05-01"}`, true},
		{"date reject", `close({created: =~"^\\d{4}-\\d{2}-\\d{2}$"})`, `{"created": "2024-5-1"}`, false},
		{"email ok", `close({e: =~"^[^@\\s]+@[^@\\s]+\\.[^@\\s]+$"})`, `{"e": "user@example.com"}`, true},
		{"email reject", `close({e: =~"^[^@\\s]+@[^@\\s]+\\.[^@\\s]+$"})`, `{"e": "user@@example"}`, false},
		{"url ok", `close({u: =~"^https?://"})`, `{"u": "https://example.com"}`, true},
		{"url reject", `close({u: =~"^https?://"})`, `{"u": "ftp://example.com"}`, false},
		{"nonEmpty ok", `close({s: string & !=""})`, `{"s": "hello"}`, true},
		{"nonEmpty reject", `close({s: string & !=""})`, `{"s": ""}`, false},
		{"bounded int ok", `close({weight: int & >=1})`, `{"weight": 3}`, true},
		{"bounded int reject", `close({weight: int & >=1})`, `{"weight": 0}`, false},
		{"positive int ok", `close({periodDays: int & > 0})`, `{"periodDays": 30}`, true},
		{"sha pattern ok", `close({from: =~"^[0-9a-f]{7,40}$"})`, `{"from": "a1b2c3d"}`, true},
		{"sha pattern reject", `close({from: =~"^[0-9a-f]{7,40}$"})`, `{"from": "zzz"}`, false},
		{"mechanism enum ok", `close({m: "push" | "pull" | "toolchain"})`, `{"m": "push"}`, true},
		{"command-windows default", `close({cw: string | *""})`, `{"cw": "b.ps1"}`, true},
		{"platforms list ok", `close({platforms: [...string] | *[]})`, `{"platforms": ["linux"]}`, true},
		{"unlisted bool ok", `close({unlisted: bool | *false})`, `{"unlisted": true}`, true},
		// FuzzValidate crashers re-pinned (plan 240 round 1): a chained ordered
		// comparison whose inner result is bool (`0>0>A`, `0>A>0`) is rejected at
		// schema compile — CUE: "invalid operands ... to '>' (type bool and int)".
		{"chained compare bool-left reject", `{B:0>0>A,A:0}`, `0`, false},
		{"chained compare bool-inner reject", `{B:0>A>0,A:0}`, `0`, false},

		{"ternary push ok", ternarySchemaSrc, `{"mechanism": "push", "registry": "npm"}`, true},
		{"ternary push empty rejects", ternarySchemaSrc, `{"mechanism": "push", "registry": ""}`, false},
		{"ternary pull empty ok", ternarySchemaSrc, `{"mechanism": "pull", "registry": ""}`, true},
	}
}

// ternarySchemaSrc is the release-channels cross-field ternary schema.
const ternarySchemaSrc = `close({mechanism: "push" | "pull", ` +
	`registry: [if mechanism == "push" {string & != ""}, (string | *"")][0]})`

// TestSchemaCorpus runs every schema/data case through the engine and asserts
// the pinned accept/reject verdict, the regression that replaced the harness's
// schema/data differential arm.
func TestSchemaCorpus(t *testing.T) {
	for _, c := range schemaCorpus() {
		t.Run(c.name, func(t *testing.T) {
			schema, err := Compile(c.schema)
			if err != nil {
				// A schema that fails to compile (an empty bound interval, an
				// all-bottom disjunction) is a rejection at the compile stage.
				assert.False(t, c.accepts, "a schema that does not compile cannot accept")
				return
			}
			data, err := CompileJSON([]byte(c.data))
			require.NoError(t, err, "data must compile")
			got := schema.Unify(data).Validate() == nil
			assert.Equal(t, c.accepts, got, "accept/reject must match the pinned verdict")
		})
	}
}

// exprResult is one row-expression's pinned outcome: whether the engine renders
// a string (ok) and, when it does, that string (out).
type exprResult struct {
	ok  bool
	out string
}

// rowCase is one row-expression case: the expression, the front-matter scope as
// JSON, and the pinned engine result.
type rowCase struct {
	name  string
	expr  string
	scope string
	want  exprResult
}

// decodeScope parses a JSON object into a front-matter map, keeping integers as
// int (UseNumber) the way the real front-matter path does. An empty string is
// the empty scope.
func decodeScope(t *testing.T, scopeJSON string) map[string]any {
	t.Helper()
	if scopeJSON == "" {
		return map[string]any{}
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(scopeJSON)))
	dec.UseNumber()
	var m map[string]any
	require.NoError(t, dec.Decode(&m))
	return m
}

// renderCase evaluates one row case through CompileRow + Render and returns the
// engine result.
func renderCase(t *testing.T, expr, scopeJSON string) exprResult {
	t.Helper()
	tpl, err := CompileRow(expr)
	if err != nil {
		return exprResult{}
	}
	out, err := tpl.Render(decodeScope(t, scopeJSON))
	if err != nil {
		return exprResult{}
	}
	return exprResult{ok: true, out: out}
}

// TestRowCorpus runs every row-expression case through the engine and asserts
// the pinned accept/reject and rendered string — the regression that replaced
// the harness's surface-C differential arm.
func TestRowCorpus(t *testing.T) {
	for _, c := range rowCorpus() {
		t.Run(c.name, func(t *testing.T) {
			got := renderCase(t, c.expr, c.scope)
			assert.Equal(t, c.want.ok, got.ok, "accept/reject must match the pinned verdict")
			if c.want.ok {
				assert.Equal(t, c.want.out, got.out, "rendered string must match the pinned golden")
			}
		})
	}
}
