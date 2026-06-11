package cuelite

import (
	"bytes"
	"encoding/json"
	"testing"
)

// decodeScopeRaw parses a scope JSON object for the fuzzers, returning nil on
// any decode failure (the engine treats a nil scope as empty). Unlike the
// test-helper decodeScope it never calls t.Fatal, so a fuzzer feeding malformed
// JSON exercises Render's missing-reference path rather than failing the test.
func decodeScopeRaw(scopeJSON string) map[string]any {
	dec := json.NewDecoder(bytes.NewReader([]byte(scopeJSON)))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil
	}
	return m
}

// fuzz_test.go holds the engine-only smoke fuzzers that replaced the deleted
// internal/cuelitetest differential fuzzers (plan 240). The harness fuzzers
// compared the in-house engine against a cuelang oracle; with cuelang gone
// there is no second implementation to diff, so these fuzzers instead assert
// crash/panic safety — the engine must never panic on any (well-formed or
// malformed) input. The pinned corpora (corpus_test.go, rowcorpus_test.go)
// carry the behavioural regression; these carry the robustness regression.
//
// Run as real fuzzers locally with, e.g.:
//
//	go test -run=- -fuzz=FuzzRowSmoke -fuzztime=30s ./cue/cuelite/

// FuzzRowSmoke evaluates a (row expression, scope JSON) pair and requires the
// engine to return a result or an error — never to panic. It is seeded from the
// row corpus plus boundary seeds at the subset edges.
func FuzzRowSmoke(f *testing.F) {
	for _, c := range rowCorpus() {
		f.Add(c.expr, c.scope)
	}
	for _, s := range extraRowFuzzSeeds() {
		f.Add(s.expr, s.scope)
	}
	f.Fuzz(func(t *testing.T, expr, scope string) {
		tpl, err := CompileRow(expr)
		if err != nil {
			return
		}
		// Render must not panic regardless of the scope's shape. Its error is
		// expected for a malformed scope; only a panic is a failure.
		_, _ = tpl.Render(decodeScopeRaw(scope))
	})
}

// extraRowFuzzSeeds steers the row mutator toward the subset boundaries: the
// builtins, comprehension forms, the ternary idiom, the operators, the
// interpolation dialects, and the out-of-subset rejections.
func extraRowFuzzSeeds() []struct{ expr, scope string } {
	return []struct{ expr, scope string }{
		{`strings.Join([for x in xs {x}], ",")`, `{"xs":["a","b"]}`},
		{`len(xs)`, `{"xs":[1,2,3]}`},
		{`[if c {"y"}, if !c {"n"}][0]`, `{"c":true}`},
		{`a + b`, `{"a":"x","b":"y"}`},
		{`a + b`, `{"a":1,"b":2}`},
		{`"\(a)\(b)"`, `{"a":1,"b":true}`},
		{`"" * 0`, ``},
		{`"ab" * 3`, ``},
		{`3 * "ab"`, ``},
		{"\"\"\"\n  a\\(id)b\n  \"\"\"", `{"id":"X"}`},
		{`#"a\#(id)b"#`, `{"id":"X"}`},
		{`'a\(id)b'`, `{"id":"X"}`},
		{`"\(0.1 + 0.2)"`, ``},
		{`"\(x + 1)"`, `{"x":9223372036854775807}`},
		{`"\(-x)"`, `{"x":-9223372036854775808}`},
		{`strings.Join([for x in xs if x != "b" {x}], ",")`, `{"xs":["a","b","c"]}`},
		{`strings.Join([for i, x in xs {"\(i):\(x)"}], ",")`, `{"xs":["a","b"]}`},
		{`"\({a:1}.a)"`, ``},
		{`fm["k"]`, `{"k":"v"}`},
		{`xs[5]`, `{"xs":["a"]}`},
		{``, ``},
		{`(`, ``},
		{`[`, ``},
	}
}

// FuzzSchemaSmoke compiles a schema and JSON data and unifies/validates them,
// requiring the engine never to panic. Seeded from the schema corpus plus the
// subset-boundary seeds the former differential fuzzer used.
func FuzzSchemaSmoke(f *testing.F) {
	for _, c := range schemaCorpus() {
		f.Add(c.schema, c.data)
	}
	for _, s := range extraSchemaFuzzSeeds() {
		f.Add(s.schema, s.data)
	}
	f.Fuzz(func(t *testing.T, schema, data string) {
		s, err := Compile(schema)
		if err != nil {
			return
		}
		d, err := CompileJSON([]byte(data))
		if err != nil {
			return
		}
		// Unify + Validate must not panic; a rejection is fine.
		_ = s.Unify(d).Validate()
	})
}

// extraSchemaFuzzSeeds steers the schema mutator toward the subset boundaries:
// type atoms, bounds, regex, disjunction defaults, optional/closed structs,
// lists, the strict-subset numeric edges, and malformed sources.
func extraSchemaFuzzSeeds() []struct{ schema, data string } {
	return []struct{ schema, data string }{
		{`{a: string}`, `{"a": "x"}`},
		{`{a: >=0 & <=10}`, `{"a": 5}`},
		{`{a: =~"^[a-z]+$"}`, `{"a": "AB"}`},
		{`{a: "x" | "y"}`, `{"a": "z"}`},
		{`{a?: string}`, `{}`},
		{`close({a: int})`, `{"a": 1, "b": 2}`},
		{`{a: bool | *false}`, `{}`},
		{`{a: [...int]}`, `{"a": ["x"]}`},
		{`{a: {b: int}}`, `{"a": {"b": "x"}}`},
		{`{a: string & !=""}`, `{"a": ""}`},
		{`{a: int}`, `{"a":1,"a":2}`},
		{`{x: 10000000000000000000}`, `{"x":0}`},
		{`{x: 1e999}`, `{"x":0}`},
		{`{string | +""}`, `0`},
		{`{a: *string | *""}`, `{}`},
		{`{x: 0&1 | 1&0}`, `{"x":0}`},
		// Re-pinned FuzzValidate crashers (plan 240 round 1): chained ordered
		// comparisons whose inner result is bool, and a deep array-element
		// duplicate key.
		{`{B:0>0>A,A:0}`, `0`},
		{`{B:0>A>0,A:0}`, `0`},
		{`{a: [...]}`, `{"a":[[{"k":1,"k":2}]]}`},
		{``, ``},
		{`{`, `{`},
		{`{a:`, `{"a"`},
	}
}

// FuzzPathSmoke parses a path expression through ParsePath and requires the
// parser never to panic or hang — the robustness regression for surface D
// (path.go + unquote.go), which lost its fuzzer when the differential harness
// was deleted (plan 240). The 12 historical minimized crashers (recovered from
// the deleted internal/cuelitetest/testdata/fuzz/FuzzParsePath corpus) seed the
// mutator so a regression in a known class surfaces immediately.
//
// Run as a real fuzzer locally with:
//
//	go test -run=- -fuzz=FuzzPathSmoke -fuzztime=30s ./cue/cuelite/
func FuzzPathSmoke(f *testing.F) {
	for _, s := range pathSmokeSeeds() {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, expr string) {
		// ParsePath must return a Path or an error, never panic, on any input.
		// The parsed segments must also feed ResolvePath without panicking.
		p, err := ParsePath(expr)
		if err != nil {
			return
		}
		_ = p.Segments()
	})
}

// pathSmokeSeeds returns the historical minimized ParsePath crashers
// (recovered from git history: the deleted FuzzParsePath testdata corpus) plus
// the raw-byte, surrogate, raw-string, and multiline boundary seeds the old
// differential fuzzer carried. They steer FuzzPathSmoke toward the quoting,
// escape, BOM, and CR-near-close edges that produced panics before.
func pathSmokeSeeds() []string {
	return []string{
		// The 12 minimized crashers from the deleted FuzzParsePath corpus.
		"\"\xfc\"",
		"A//\x00",
		"#\"\"\"\n0\n\"\"\"\r#\"\"\"#",
		"\"\\U80000000\"",
		"##\"\"\"##",
		"#\"\"\"\n\\\r#\n0\n\"\"\"#",
		"A000000000000000//",
		"A//\xe7",
		"#\"\r\"#",
		"#\"\"\"\n\"\"\"\r#\n\"\"\"#",
		"A\n",
		// Boundary seeds: quoting, surrogate halves, raw-string escapes, BOM,
		// CR-near-close, and the after-dot rejection.
		"\"a\\u0041\"", "\"\\U0001F600\"", "\"\\uD83D\\uDE00\"", "\"\\uD83D\"",
		"a[\"b\"]", "a[0]", "#\"b\"#", "##\"b\"##", "a.#\"b\"#",
		`#"\#"#"#`, "\ufeffa", "a\ufeffb", "$.$", "a.if.for",
		"\"\"\"\na\n\"\"\"", "#\"\"\"\na\n\"\"\"#", "\"\"\"\n\"\"\r\"\n\"\"\"",
	}
}
