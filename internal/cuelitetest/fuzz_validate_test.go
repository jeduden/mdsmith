package cuelitetest

import (
	"slices"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/cue/cuelite"
)

// inHouseRejectsOutOfSubset reports whether the in-house engine rejects the
// schema at compile time for a documented strict-subset reason:
//
//   - an out-of-subset construct or literal ("cuelite: unsupported …", which
//     also covers an int/float literal outside the int64/float64 subset),
//   - a regex bound (=~ / !~) whose pattern Go's regexp rejects,
//   - a schema-to-schema reference the subset does not resolve ("reference X
//     not found"), and
//   - an "invalid operation" CUE itself also rejects but defers (unary +/-
//     on a non-number), which the in-house engine rejects eagerly.
//
// All are safe strictness: a schema-compile error CUE either does not share
// or only resolves later, never a silent wrong-accept of data. A rejection
// with any other message — or no rejection at all — is NOT covered, so the
// hatch never masks a wrong rejection of a valid subset schema.
func inHouseRejectsOutOfSubset(schema string) bool {
	_, err := cuelite.Compile(schema)
	if err == nil {
		return false
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "unsupported"):
		return true
	case strings.Contains(msg, "invalid operation"):
		return true
	case strings.Contains(msg, "reference") && strings.Contains(msg, "not found"):
		return true
	default:
		// A regex-pattern compile error always names the offending bound operator.
		return (strings.Contains(msg, "=~") || strings.Contains(msg, "!~")) &&
			strings.Contains(msg, "pattern")
	}
}

// dataHasLoneSurrogateEscape reports whether the data document contains a
// `\u`-escape that forms an unpaired surrogate — the residue the in-house
// lifter accepts as a U+FFFD string and CUE rejects (hatch 2). Scoping the
// hatch to this class keeps any other data-stage mismatch a hard failure. A
// high surrogate must be immediately followed by a low-surrogate escape to be
// paired; anything else is lone.
func dataHasLoneSurrogateEscape(data string) bool {
	b := []byte(data)
	for i := 0; i+5 < len(b); i++ {
		if b[i] != '\\' || b[i+1] != 'u' {
			continue
		}
		cu, ok := hex4(b[i+2 : i+6])
		if !ok {
			continue
		}
		if cu >= 0xDC00 && cu <= 0xDFFF {
			return true // a low surrogate reached standalone is lone
		}
		if cu >= 0xD800 && cu <= 0xDBFF {
			if i+11 >= len(b) || b[i+6] != '\\' || b[i+7] != 'u' {
				return true
			}
			lo, ok := hex4(b[i+8 : i+12])
			if !ok || lo < 0xDC00 || lo > 0xDFFF {
				return true
			}
			i += 11 // skip the paired low half
		}
	}
	return false
}

// hex4 parses exactly four hex digits, reporting ok=false on any non-hex byte.
func hex4(b []byte) (uint32, bool) {
	if len(b) != 4 {
		return 0, false
	}
	var v uint32
	for _, c := range b {
		var d uint32
		switch {
		case c >= '0' && c <= '9':
			d = uint32(c - '0')
		case c >= 'a' && c <= 'f':
			d = uint32(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = uint32(c-'A') + 10
		default:
			return 0, false
		}
		v = v<<4 | d
	}
	return v, true
}

// bothReject reports whether both arms resolved at the validate stage (each
// rejected the document), the precondition for the superset tolerance below.
func bothReject(a, b Outcome) bool {
	return a.Stage == StageValidate && b.Stage == StageValidate
}

// maxExtraLeaves bounds the leafSuperset tolerance: the in-house engine may
// report at most this many MORE rejecting leaves than CUE on an
// already-failing document. The only documented over-report class — a
// closed-struct violation reported alongside an absent-required-field
// violation CUE suppresses — adds a single extra leaf, so a tight bound of 1
// admits that class while turning any larger leaf-count blow-up (a sign the
// in-house engine fans a single failure into many phantom leaves) back into a
// hard fuzzer failure.
const maxExtraLeaves = 1

// leafSuperset reports whether every path in want appears in got — the
// in-house leaf set covers (is a superset of) the oracle's, so the in-house
// engine rejected at least every leaf CUE did — AND the in-house set carries
// at most maxExtraLeaves paths beyond the oracle's, bounding the tolerance.
func leafSuperset(got, want [][]string) bool {
	for _, w := range want {
		if !slices.ContainsFunc(got, func(g []string) bool { return slices.Equal(g, w) }) {
			return false
		}
	}
	return len(got)-len(want) <= maxExtraLeaves
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
	for _, seed := range extraFuzzSeeds() {
		f.Add(seed.schema, seed.data)
	}
	f.Fuzz(fuzzValidateBody())
}

// extraFuzzSeeds steers the mutator toward the subset's boundaries: type
// atoms, bounds, regex, disjunction defaults, optional keys, closed structs,
// nested structs, lists, len/MinRunes, the strict-subset literal and operator
// edges (each a documented hatch-1 class), and the lone-surrogate classes.
func extraFuzzSeeds() []struct{ schema, data string } {
	return []struct{ schema, data string }{
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
		// Strict-subset literal boundaries: an int outside int64 and a float
		// outside float64 are rejected at in-house schema compile (CUE keeps
		// big.Int/big.Float and accepts). These steer the mutator at the
		// numeric subset edge; hatch 1 covers the resulting divergence.
		{`{x: 10000000000000000000}`, `{"x":0}`},
		{`{10000000000000000000}`, `0`},
		{`{x: 1e999}`, `{"x":0}`},
		// Unary +/- on a non-number: CUE defers the invalid operation inside a
		// disjunction, the in-house engine rejects it eagerly at schema compile.
		{`{string | +""}`, `0`},
		{`{x: -"a"}`, `{"x":0}`},
		// A bound over a type rather than a concrete scalar (>string): out of
		// subset for the in-house engine, accepted-and-deferred by CUE.
		{`{A?: >string}`, `0`},
		// An ordered comparison of mismatched scalar kinds (0 > ""): CUE rejects
		// it as an invalid operation but defers in a disjunction; the in-house
		// engine rejects eagerly at schema compile.
		{`{0 > "" | ""}`, `0`},
		// An undeclared reference inside an embedded disjunction branch: CUE
		// rejects "reference A not found" at schema compile; the in-house engine
		// must descend the disjunction and reject the same way (regression seed
		// for the embedded-thunk recursive ref check).
		{`{A > "" | ""}`, `0`},
		// A cyclic structural reference with a multi-part selector (B.A): the
		// in-house engine rejects it out-of-subset, so the fuzzer's pre-oracle
		// guard skips it — WITHOUT the guard the cuelang.org/go oracle does not
		// terminate. Seeded so the guard stays exercised.
		{`A:A:B:B.A&A&A,A`, "\x7f"},
		// A comparison with a non-concrete TYPE operand (A > _): CUE rejects the
		// _ operand at schema compile ("'>' requires concrete value"); the
		// in-house engine must reject it eagerly too rather than defer a thunk
		// that can never resolve.
		{`A: A > _`, `0`},
		{`{a: int, b: a == string}`, `{"a":1}`},
		// An `if` comprehension whose condition is not a concrete bool (a string
		// literal, a type, or top): CUE rejects "cannot use ... as type bool" at
		// schema compile; the in-house engine must reject too, not panic on an
		// empty free-reference set. Regression seed for the freeRefs guard.
		{`({A: [if "" {}]})`, `0`},
		{`({A: [if string {}]})`, `0`},
		// A bare type-keyword field label (int:): CUE resolves a same-named
		// reference in the field value as a self-reference, not the type, so
		// `{int: {int}}` accepts `{}` where a quoted label would reject. The
		// in-house engine cannot model the shadowing and rejects the bare keyword
		// label out-of-subset; hatch 1 covers the divergence.
		{`{int: {int}}`, `{}`},
		{`{string: x}`, `{}`},
		// A lone-surrogate escape in a VALUE position (hatch 2) and in a KEY
		// position (now rejected in both arms — no hatch). Seeding both keeps
		// the surrogate classes exercised on every run.
		{`{a: _}`, `{"a": "\ud800"}`},
		{`{a: _}`, `{"\ud800": 1}`},
	}
}

// fuzzValidateBody is the per-input differential check FuzzValidate runs. It is
// a named closure so FuzzValidate itself stays short (the seed wiring and the
// body are separately readable).
func fuzzValidateBody() func(*testing.T, string, string) {
	return func(t *testing.T, schema, data string) {
		c := Case{Schema: schema, Data: data}
		inHouse := CueLitePath(c)
		// Hatch 1 — strict-subset schema compile, checked BEFORE the oracle. The
		// in-house engine compiles a strict CUE SUBSET and rejects documented
		// classes eagerly at schema-compile time, all of which CUE accepts (and
		// resolves later):
		//
		//   - a regex bound (=~ / !~) whose pattern Go's regexp rejects (CUE
		//     defers the regex),
		//   - a construct or literal outside the subset (arithmetic `*`/`+`, a
		//     multi-part selector `B.A`, an int/float literal outside the
		//     int64/float64 range, a bound over a type `>string`), reported as a
		//     clear "cuelite: unsupported …",
		//   - a schema-to-schema field reference (`A: B, B: 1`, a self-cycle
		//     `A: A`, or an undeclared name in an embedded thunk `{A > "" | ""}`),
		//     which the subset does not resolve — the in-house engine resolves
		//     references only against DATA (the thunk idiom), so a reference with
		//     no data binding is a "reference X not found", and
		//   - an "invalid operation" CUE also rejects but defers inside a
		//     disjunction (unary +/- on a non-number, an ordered compare of
		//     mismatched kinds `0 > ""`).
		//
		// All are SAFE strictness — a schema-compile error, never a silent
		// wrong-accept of data — so when the in-house compile error names one of
		// these classes the oracle is not consulted at all: CUE either also
		// rejects at schema compile (agreement) or accepts-and-defers (the
		// tolerated divergence), neither a failure. Skipping the oracle here is
		// also a soundness guard — some out-of-subset constructs CUE accepts (a
		// cyclic structural reference such as `A:A:B:B.A&A&A`) drive
		// cuelang.org/go's unifier into NON-TERMINATION, which would hang the
		// fuzzer on an input the in-house engine already classifies. A schema the
		// in-house engine rejects with any OTHER message is NOT skipped, so a
		// wrong rejection of a valid subset schema still reaches the oracle and
		// fails the fuzzer.
		if inHouse.Stage == StageCompileSchema && inHouseRejectsOutOfSubset(schema) {
			return
		}
		oracle := OraclePath(c)
		if inHouse.Equal(oracle) {
			return
		}
		// Hatch 2 — lone-surrogate VALUE lift. The post-flip in-house JSON lifter
		// accepts a lone-surrogate escape ("\ud800") in a VALUE position as a
		// U+FFFD string, where CUE's stricter lift rejects it at the data stage.
		// This is the one deliberate data-acceptance divergence plan 238 records,
		// pinned by the cuelite package's own unit tests. The hatch is scoped to
		// its class: it fires only when CUE rejected at the data stage AND the
		// data carries a lone-surrogate escape (the residue that diverges). A
		// lone-surrogate KEY now rejects in BOTH arms, so it never reaches here; a
		// data-stage mismatch with no surrogate escape still fails.
		if oracle.Stage == StageCompileData && inHouse.Stage != StageCompileData &&
			dataHasLoneSurrogateEscape(data) {
			return
		}
		// Both arms reject the document, and every leaf CUE rejects is also
		// rejected by the in-house engine — but the in-house engine reports at
		// most maxExtraLeaves (1) EXTRA leaves. This happens only on a
		// pathological mix CUE suppresses: a closed-struct violation on one field
		// alongside an absent required field on another (CUE reports just the
		// close violation; the in-house engine reports both — exactly one extra
		// leaf). Real data never mixes the two — it either conforms to the closed
		// key set or the diagnostics dedupe per field — and reporting one MORE
		// failure on an already-failing document is safe (the in-house engine
		// never silently accepts what CUE rejects). The bound keeps the tolerance
		// honest: a missing leaf, a wrong accept, a stage mismatch, OR a leaf-set
		// blow-up beyond one extra still fails.
		if bothReject(inHouse, oracle) && leafSuperset(inHouse.Paths, oracle.Paths) {
			return
		}
		t.Fatalf("divergence on schema=%q data=%q: in-house %+v vs oracle %+v",
			schema, data, inHouse, oracle)
	}
}
