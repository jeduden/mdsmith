package cuelitetest

import "testing"

// FuzzParsePath is the differential fuzz target for surface D: it parses
// each input through both arms — the in-house cuelite.ParsePath and the
// CUE-backed oracle — and fails when they disagree on accept/reject or on
// the produced segments. It is the broad complement to the curated
// pathCorpus: the corpus pins one case per known behaviour class, the
// fuzzer explores the rest of the input space around those seeds.
//
// It runs as an ordinary test in CI (the f.Add seeds execute with no
// -fuzz flag) and can be driven as a real fuzzer locally with:
//
//	go test -run=- -fuzz=FuzzParsePath -fuzztime=30s ./internal/cuelitetest/
//
// Every pathCorpus expression seeds the corpus so a regression in a known
// class fails immediately, and the mutator starts from grammar-relevant
// bytes.
func FuzzParsePath(f *testing.F) {
	for _, c := range pathCorpus() {
		f.Add(c.Expr)
	}
	// A few extra raw-byte and escape seeds the corpus does not name, to
	// steer the mutator toward the quoting, whitespace, surrogate, bracket,
	// raw-string, and BOM boundaries.
	for _, seed := range []string{
		"a\rb", "\"a\rb\"", "\"a\\u0041\"", "\"\\U0001F600\"",
		"a\t.\tb", "a\n.b", "a\v.b", "\"a\x01b\"", "\"a\x7fb\"",
		"$.$", "a.if.for", "x.true.false", "\"\\/\"", "\"\\\\\"",
		// Surrogate pairing and lone halves.
		"\"\\uD83D\\uDE00\"", "\"\\uD83D\"", "\"\\uDC00\"", "\"\\uD83D\\u0041\"",
		// Bracket string-index selectors.
		"a[\"b\"]", "a[\n\"b\"]", "a[\"b\"\n]", "a[0]", "a[ \"b\" ]", "[\"b\"]",
		// Multi-hash raw-string labels and the after-dot rejection.
		"#\"b\"#", "##\"b\"##", "a[#\"b\"#]", "a.#\"b\"#", "#\"a\\#nb\"#", "#x",
		// Raw-string surrogate pairing: both halves need the \#u introducer
		// (accept), a plain \u low half leaves the high lone (reject), and a
		// high half before the closing delimiter is lone (former panic input).
		"#\"\\#uD800\\#uDC00\"#", "##\"\\##uD800\\##uDC00\"##",
		"#\"\\#uD800\\uDC00\"#", "##\"\\##uD800\\uDC00\"##", "#\"\\#uD800\\\"#",
		// BOM at offset 0 (skipped) vs interior (rejected).
		"\ufeffa", "a\ufeffb", "\"a\ufeffb\"", "a//\ufeff",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, expr string) {
		inHouse := CueLitePathParsePath(PathCase{Expr: expr})
		oracle := OraclePathParsePath(PathCase{Expr: expr})
		if !inHouse.Equal(oracle) {
			t.Fatalf("divergence on expr=%q: in-house %+v vs oracle %+v",
				expr, inHouse, oracle)
		}
	})
}
