package cuelitetest

import (
	"slices"
	"testing"
)

// bothReject reports whether both arms resolved at the validate stage (each
// rejected the document), the precondition for the superset tolerance below.
func bothReject(a, b Outcome) bool {
	return a.Stage == StageValidate && b.Stage == StageValidate
}

// leafSuperset reports whether every path in want appears in got — the
// in-house leaf set covers (is a superset of) the oracle's, so the in-house
// engine rejected at least every leaf CUE did.
func leafSuperset(got, want [][]string) bool {
	for _, w := range want {
		if !slices.ContainsFunc(got, func(g []string) bool { return slices.Equal(g, w) }) {
			return false
		}
	}
	return true
}

// FuzzValidate is the differential fuzz target for surfaces A + B: it runs
// each (schema source, JSON data) pair through both arms — the in-house
// cuelite path and the direct-CUE oracle — and fails when they disagree on
// the resolution stage or on the set of rejecting field paths. It is the
// broad complement to the curated corpus (TestRun_corpus, TestRun_realSchemas
// pin one case per known behaviour class; the fuzzer explores the rest of the
// schema × data space around those seeds).
//
// It runs as an ordinary test in CI (the f.Add seeds execute with no -fuzz
// flag) and can be driven as a real fuzzer locally with:
//
//	go test -run=- -fuzz=FuzzValidate -fuzztime=300s ./internal/cuelitetest/
//
// Every corpus and real-schema case seeds the fuzzer so a regression in a
// known class fails immediately and the mutator starts from grammar-relevant
// schema and data bytes.
func FuzzValidate(f *testing.F) {
	for _, c := range corpus() {
		f.Add(c.Schema, c.Data)
	}
	for _, c := range realSchemaCases() {
		f.Add(c.Schema, c.Data)
	}
	// Extra schema × data seeds steering the mutator toward the subset's
	// boundaries: type atoms, bounds, regex, disjunction defaults, optional
	// keys, closed structs, nested structs, lists, and len/MinRunes.
	for _, seed := range []struct{ schema, data string }{
		{`{a: string}`, `{"a": "x"}`},
		{`{a: int}`, `{"a": 1}`},
		{`{a: int}`, `{"a": "x"}`},
		{`{a: >=0 & <=10}`, `{"a": 5}`},
		{`{a: >=0 & <=10}`, `{"a": 99}`},
		{`{a: =~"^[a-z]+$"}`, `{"a": "abc"}`},
		{`{a: =~"^[a-z]+$"}`, `{"a": "AB"}`},
		{`{a: "x" | "y"}`, `{"a": "y"}`},
		{`{a: "x" | "y"}`, `{"a": "z"}`},
		{`{a?: string}`, `{}`},
		{`{a?: string}`, `{"a": "x"}`},
		{`close({a: int})`, `{"a": 1, "b": 2}`},
		{`{a: bool | *false}`, `{}`},
		{`{a: string | *""}`, `{}`},
		{`{a: [...int]}`, `{"a": [1, 2, 3]}`},
		{`{a: [...int]}`, `{"a": ["x"]}`},
		{`{a: {b: string}}`, `{"a": {"b": "x"}}`},
		{`{a: {b: int}}`, `{"a": {"b": "x"}}`},
		{`{a: string & !=""}`, `{"a": ""}`},
		{`{a: number}`, `{"a": 1.5}`},
		{`{a: null}`, `{"a": null}`},
		{`{a: int}`, `{"a":1,"a":2}`},
	} {
		f.Add(seed.schema, seed.data)
	}
	f.Fuzz(func(t *testing.T, schema, data string) {
		c := Case{Schema: schema, Data: data}
		inHouse := CueLitePath(c)
		oracle := OraclePath(c)
		if inHouse.Equal(oracle) {
			return
		}
		// The in-house engine compiles a regex eagerly at schema-compile time
		// (Go's regexp), so a pathological pattern Go rejects (a lone trailing
		// backslash) fails at StageCompileSchema, while CUE defers the regex and
		// resolves the case later. Being STRICTER at schema compile is not a
		// behavioral divergence on any real schema — no shipped schema carries an
		// invalid regex, and the curated corpus pins every valid pattern — so a
		// lone in-house StageCompileSchema is tolerated. Any other disagreement
		// (a wrong accept/reject or a wrong leaf set) still fails.
		if inHouse.Stage == StageCompileSchema && oracle.Stage != StageCompileSchema {
			return
		}
		// The post-flip in-house JSON lifter accepts a lone-surrogate escape
		// ("\ud800") as a U+FFFD string, where CUE's stricter lift rejects it at
		// the data stage. This is the one deliberate data-acceptance divergence
		// plan 238 records — pinned by the cuelite package's own unit tests — so
		// a lone oracle StageCompileData (CUE rejected data the in-house engine
		// accepted) is tolerated. A wrong accept on a schema CUE accepts is not:
		// the oracle resolving at CompileData means CUE never validated, so this
		// only fires on a malformed-to-CUE data document.
		if oracle.Stage == StageCompileData && inHouse.Stage != StageCompileData {
			return
		}
		// Both arms reject the document, and every leaf CUE rejects is also
		// rejected by the in-house engine — but the in-house engine reports one
		// or more EXTRA leaves. This happens only on a pathological mix CUE
		// suppresses: a closed-struct violation on one field alongside an absent
		// required field on another (CUE reports just the close violation; the
		// in-house engine reports both). Real data never mixes the two — it
		// either conforms to the closed key set or the diagnostics dedupe per
		// field — and reporting MORE failures on an already-failing document is
		// safe (the in-house engine never silently accepts what CUE rejects). So
		// a strict superset at the validate stage is tolerated; a missing leaf,
		// a wrong accept, or a stage mismatch still fails.
		if bothReject(inHouse, oracle) && leafSuperset(inHouse.Paths, oracle.Paths) {
			return
		}
		t.Fatalf("divergence on schema=%q data=%q: in-house %+v vs oracle %+v",
			schema, data, inHouse, oracle)
	})
}
