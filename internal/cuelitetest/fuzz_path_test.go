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
		// Raw-string escape-aware close: an escaped quote followed by a hash
		// run is not the terminator (the 240s-fuzz minimized regression).
		`#"\#"#"#`, `#"q\#"#x"#`, `##"\##"##"##`, `x[#"\#"#"#]`,
		// Multiline string labels: openers, indentation, escapes, CRLF, the
		// after-dot rejection, and malformed shapes that decode to "".
		"\"\"\"\na\n\"\"\"", "\"\"\"\n  a\n  \"\"\"", "#\"\"\"\na\n\"\"\"#",
		"x[\"\"\"\na\n\"\"\"]", "\"\"\"\na\n\"\"\".b", "\"\"\"\na\\tb\n\"\"\"",
		"\"\"\"\n\\uD83D\\uDE00\n\"\"\"", "\"\"\"\r\n  a\r\n  \"\"\"",
		"\"\"\"a\n\"\"\"", "\"\"\"\n a\n  \"\"\"", "a.\"\"\"\nb\n\"\"\"",
		"\"\"\"\n\"\"\"", "\"\"\"\n  a\\\n  \"\"\"",
		// CR stripping inside multiline tokens (scanner.stripCR): blank CRLF
		// lines, bare CRs, and blank first lines.
		"\"\"\"\n\n  b\n  \"\"\"", "\"\"\"\r\n\r\n  b\r\n  \"\"\"",
		"\"\"\"\r\n  a\rb\r\n  \"\"\"", "\"\"\"\n  a\r\n\r\n  b\n  \"\"\"",
		// CR-family (round 5): the opener-newline and escape decisions run on the
		// RAW token (stripCR runs only after scanning), so a CR run at the opener,
		// a CR between the backslash and the escape selector, and a CR among \u
		// hex digits are scan errors CUE rejects; the '\'+CR+'#' level-1 case
		// stays accepted (literal backslash, then `\#n` after CR strips).
		"\"\"\"\r\r\n0\n\"\"\"", "\"\"\"\n\\\rn\n\"\"\"", "\"\"\"\n\\u00\r41\n\"\"\"",
		"#\"\"\"\n  \\\r#n\n  \"\"\"#",
		"\"\"\"\n\\U0001F600\n\"\"\"", "\"\"\"\na\\u0\"\"\"",
		"\"\"\"\na\\nb\\\"c\\\\d\\/e\\tf\n\"\"\"", "\"\"\"\na\\bb\\fc\\rd\\ve\\ag\n\"\"\"",
		// Escaped-newline line continuation reachable via the '\'+CR+'#' fusion
		// (round-5 fuzz find): the scanner accepts the literal backslash, stripCR
		// fuses '\#'+newline, and literal.Unquote elides the newline.
		"#\"\"\"\n\\\r#\n0\n\"\"\"#", "#\"\"\"\nx\\\r#\ny\\\r#\nz\n\"\"\"#",
		"#\"\"\"\n\\\r#\n  \\\r#\n0\n\"\"\"#", "#\"\"\"\na\\\r#\n\"\"\"#",
		"#\"\"\"\n\\#\n0\n\"\"\"#", "#\"\"\"\n  a\\\r#\nx\n  \"\"\"#",
		// CR-near-close (round 6): the scanner finds the token end on RAW bytes,
		// but literal.Unquote re-finds the close on the CR-STRIPPED literal, so a CR
		// breaking a `"""`+'#' run can fuse an EARLIER close whose shorter content
		// decodes to "". Seed the level-1 bare-CR form (the fuzz-minimized input),
		// content-before and trailing-dot variants, the level-0 closing-quote-run
		// CR, and the benign CR-after-close shape.
		"#\"\"\"\n\"\"\"\r#\n\"\"\"#", "#\"\"\"\n\"\"\"\r#\n\"\"\"#.x",
		"#\"\"\"\nq\"\"\"\r#z\n\"\"\"#", "\"\"\"\n\"\"\r\"\n\"\"\"",
		"\"\"\"\na\n\"\"\"\r", "#\"\"\"\n\"\"\"\r##x\n\"\"\"#",
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
