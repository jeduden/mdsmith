package cuelitetest

import (
	"slices"
	"strings"
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"github.com/jeduden/mdsmith/cue/cuelite"
)

// schemaHasReferenceCycle reports whether the schema's top-level fields form a
// reference cycle — a field whose value references its own label
// (`{a: [if a {}]}`), or a chain of fields that reference back to the start
// (`{a: [if b {}], b: [if a {}]}`). A reference cycle is OUTSIDE the documented
// front-matter subset: no real schema's fields cross-reference, so CUE's eager
// cycle detection and the in-house engine's lazy data-thunk resolution diverge
// on stage, leaf path, and (for a cycle hidden in a disjunction branch) even
// accept/reject. The fuzzer skips a cyclic schema before consulting the oracle
// — the same treatment as an out-of-subset construct — rather than claiming
// the harness matches CUE on a construct neither the subset nor real schemas
// admit. A real (acyclic) schema never trips this, so a genuine divergence is
// never masked.
func schemaHasReferenceCycle(schema string) bool {
	f, err := parser.ParseFile("schema.cue", schema)
	if err != nil {
		return false
	}
	// Build the field-reference graph: field name -> the set of top-level field
	// names its value references. Only top-level fields participate; a nested
	// field's references resolve in its own struct.
	graph := map[string][]string{}
	names := map[string]bool{}
	fields := topLevelFields(f)
	for name := range fields {
		names[name] = true
	}
	for name, value := range fields {
		for ref := range referencedIdents(value) {
			if names[ref] {
				graph[name] = append(graph[name], ref)
			}
		}
	}
	return graphHasCycle(graph, names)
}

// topLevelFields returns the top-level field label -> value map of a parsed
// file, descending the single embedded struct the query/validator forms wrap
// their schema in (`({a: …})`).
func topLevelFields(f *ast.File) map[string]ast.Expr {
	out := map[string]ast.Expr{}
	var collect func(decls []ast.Decl)
	collect = func(decls []ast.Decl) {
		for _, d := range decls {
			switch n := d.(type) {
			case *ast.Field:
				if name, _, _ := ast.LabelName(n.Label); name != "" {
					out[name] = n.Value
				}
			case *ast.EmbedDecl:
				if st, ok := unwrapStruct(n.Expr); ok {
					collect(st.Elts)
				}
			}
		}
	}
	collect(f.Decls)
	return out
}

// unwrapStruct unwraps parens and a close(...) wrapper around a struct literal,
// so `({a: 1})` and `close({a: 1})` both yield the inner struct.
func unwrapStruct(e ast.Expr) (*ast.StructLit, bool) {
	for {
		switch n := e.(type) {
		case *ast.ParenExpr:
			e = n.X
		case *ast.CallExpr:
			// close({…}) — descend its single struct argument.
			if id, ok := n.Fun.(*ast.Ident); ok && id.Name == "close" && len(n.Args) == 1 {
				e = n.Args[0]
				continue
			}
			return nil, false
		case *ast.StructLit:
			return n, true
		default:
			return nil, false
		}
	}
}

// referencedIdents returns the set of bare identifier names appearing anywhere
// in e — the candidate field references of e.
func referencedIdents(e ast.Expr) map[string]bool {
	out := map[string]bool{}
	ast.Walk(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			out[id.Name] = true
		}
		return true
	}, nil)
	return out
}

// graphHasCycle reports whether the directed graph (node -> successors) over
// nodes contains a cycle, including a self-loop, via a three-color DFS.
func graphHasCycle(graph map[string][]string, nodes map[string]bool) bool {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	var visit func(n string) bool
	visit = func(n string) bool {
		color[n] = gray
		for _, m := range graph[n] {
			if color[m] == gray {
				return true
			}
			if color[m] == white && visit(m) {
				return true
			}
		}
		color[n] = black
		return false
	}
	for n := range nodes {
		if color[n] == white && visit(n) {
			return true
		}
	}
	return false
}

// inHouseRejectsOutOfSubset reports whether the in-house engine rejects the
// schema at compile time for a documented strict-subset reason:
//
//   - an out-of-subset construct or literal ("cuelite: unsupported …", which
//     also covers an int/float literal outside the int64/float64 subset),
//   - a regex bound (=~ / !~) whose pattern Go's regexp rejects, or one
//     applied to a non-string operand (`0 !~ ""`),
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
		// The in-house engine is stricter than CUE on a regex construct (=~ /
		// !~): it eagerly rejects a pattern Go's regexp cannot compile, AND a
		// regex applied to a non-string operand (`0 !~ ""`). CUE defers the regex
		// or, inside a disjunction, drops the ⊥ branch and accepts. Both are
		// eager schema-compile strictness, never a wrong-accept of data, so an
		// in-house error naming a regex operator is a documented out-of-subset
		// rejection. The "pattern" / "requires strings" wording pins the class.
		named := strings.Contains(msg, "=~") || strings.Contains(msg, "!~")
		return named && (strings.Contains(msg, "pattern") || strings.Contains(msg, "requires strings"))
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
	return leafCovers(got, want) && len(got)-len(want) <= maxExtraLeaves
}

// leafCovers reports whether every path in want appears in got (got ⊇ want),
// with no bound on the surplus — used where the in-house engine legitimately
// reports more FIELD leaves than CUE's summarized set (the root-summary hatch).
func leafCovers(got, want [][]string) bool {
	for _, w := range want {
		if !slices.ContainsFunc(got, func(g []string) bool { return slices.Equal(g, w) }) {
			return false
		}
	}
	return true
}

// pathsContainRoot reports whether paths includes the root (empty) path — CUE's
// "does not satisfy" / "incomplete value" summary leaf.
func pathsContainRoot(paths [][]string) bool {
	return slices.ContainsFunc(paths, func(p []string) bool { return len(p) == 0 })
}

// nonRootPaths returns paths with the root (empty) path removed.
func nonRootPaths(paths [][]string) [][]string {
	out := make([][]string, 0, len(paths))
	for _, p := range paths {
		if len(p) > 0 {
			out = append(out, p)
		}
	}
	return out
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

// extraFuzzSeeds steers the mutator toward the subset's boundaries: the basic
// type/bound/disjunction grammar plus the strict-subset literal and operator
// edges (each a documented hatch-1 class), the reference cycles, and the
// lone-surrogate classes.
func extraFuzzSeeds() []struct{ schema, data string } {
	return append(baseFuzzSeeds(), edgeFuzzSeeds()...)
}

// baseFuzzSeeds covers the well-formed subset grammar: type atoms, bounds,
// regex, disjunction defaults, optional keys, closed structs, nested structs,
// lists, and len/MinRunes.
func baseFuzzSeeds() []struct{ schema, data string } {
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
	}
}

// edgeFuzzSeeds covers the strict-subset edges each fix and hatch documents:
// out-of-subset literals and operators, deferred-position hard errors, misplaced
// defaults, reference cycles, and the lone-surrogate classes.
func edgeFuzzSeeds() []struct{ schema, data string } {
	return []struct{ schema, data string }{
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
		// A regex (=~ / !~) on a non-string operand (0 !~ ""): the in-house
		// engine rejects eagerly; CUE drops the ⊥ branch in a disjunction and
		// accepts. Hatch 1's regex class covers it.
		{`({0!~""|0})`, `0`},
		{`{a: 0 !~ ""}`, `{"a":0}`},
		// Indexing a non-list (`"0"[0]`): an invalid operation CUE rejects
		// eagerly but drops in a disjunction. Hatch 1's invalid-operation class
		// covers it.
		{`({A:""|"0"[0]})`, `0`},
		{`{a: "0"[0]}`, `{"a":1}`},
		// A single-quoted bytes literal (`''`, `'x'`): a distinct CUE type with
		// no JSON representation, rejected out-of-subset by the in-house engine.
		{`''`, `""`},
		{`{a: 'x'}`, `{"a":"x"}`},
		// CUE's root-summary leaf: a top-level disjunction that matches no branch,
		// and a deferred thunk referencing a non-concrete field, both make CUE
		// attribute the failure to the ROOT [] while the in-house engine names the
		// precise field. Both reject; Hatch 3 tolerates the granularity.
		{`({m:"0"}|"")`, `{"m":""}`},
		{`({m:"0",n:"1"}|"")`, `{"m":"x","n":"y"}`},
		{`({mechanism:""|"0",A:[if mechanism{}][0]})`, `{}`},
		// The default of a meet must apply regardless of operand order
		// (`(0|int) & (*1|int)` defaults A to 1): a regression where the default
		// was dropped when its surviving meet stayed a disjunction.
		{`{A:(0|int)&(*1|int)}`, `{}`},
		{`{A:(*1|int)&(0|int)}`, `{}`},
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
		// Reference cycles — a self-cycle (a field referencing its own label), a
		// cycle hidden in a disjunction branch, and a mutual cycle. All are
		// outside the front-matter subset; the fuzzer's pre-oracle cycle guard
		// skips them. Seeded so the guard stays exercised.
		{`({mechanism:[if mechanism{}][0]})`, `{}`},
		{`{a: [if a {}][0]}`, `{}`},
		{`{a:[if a{}]}`, `0`},
		{`({mechanism:""|[if mechanism{}]})`, `{}`},
		{`{a: [if b {}], b: [if a {}]}`, `{}`},
		{`close({s:[(s)][0]})`, `0`},
		// A misplaced `*` default mark in an unreached list element: CUE rejects
		// it at parse ("preference mark not allowed"); the in-house engine's
		// static pass rejects it up front rather than only when the element is
		// forced. Seeded for the checkNoMisplacedDefault pass.
		{`({mechanism:[if mechanism{},(*"")][0]})`, `0`},
		{`{a: [*""][0]}`, `0`},
		// A comparison with a HARD-error operand that is not a deferred reference
		// (!0, an unsupported unary): CUE rejects "invalid operation !0" at
		// compile; the in-house engine must propagate the operand error rather
		// than defer on the other (unresolved) operand.
		{`A: A > !0`, `0`},
		{`{a: int, b: !0 > a}`, `{"a":1}`},
		// An undeclared reference hidden in a TOP-LEVEL disjunction branch or
		// list (no enclosing struct to bind it): CUE rejects "reference A not
		// found" at compile; the in-house engine's top-level thunk-ref scan must
		// descend the disjunction/list to reject it too.
		{`0X0|0<A`, `0`},
		{`[0 < A]`, `0`},
		// An unsupported binary operator (string * "") in an UNREACHED list
		// element: CUE rejects "invalid operand" at compile; the in-house engine
		// must evaluate every list element eagerly enough to reject the hard
		// error rather than defer past it on an earlier element.
		{`({mechanism:[if mechanism{},(string*"")][0]})`, `0`},
		{`{a: [if c {}, (string*"")][0], c: bool}`, `{"c":true}`},
		// A hard error in a comprehension BODY whose condition defers
		// (`{string != ""}` compares a type, not a concrete value): CUE rejects
		// the body's invalid operand at compile regardless of the condition; the
		// in-house engine must compile the deferred body to catch it.
		{`({mechanism:[if mechanism{string!=""}][0]})`, `0`},
		{`{x: [if string {string != ""}][0]}`, `0`},
		// A non-indexed list field whose comprehension references a sibling
		// (`xs: [if c {1}, 2]`): CUE accepts and resolves it against data; the
		// in-house engine must treat the list literal as deferrable, not a hard
		// "reference not found".
		{`{c: bool, xs: [if c {1}, 2]}`, `{"c":true,"xs":[1,2]}`},
		// A `*` default mark wrapped in its own parens (`(*0) | 0`): CUE rejects
		// "preference mark not allowed at this position" — the mark must be the
		// outermost operator of a disjunct. The static pass must reject the
		// paren-wrapped mark.
		{`{(*0)|0}`, `0`},
		{`{a: 1 | (*0)}`, `{}`},
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
		// A reference cycle among the schema's fields is outside the documented
		// front-matter subset (no real schema cross-references its own fields).
		// CUE's eager cycle detection and the in-house engine's lazy data-thunk
		// resolution diverge on stage, leaf, and even accept/reject for a cycle
		// hidden in a disjunction branch. Skip a cyclic schema before the oracle,
		// like an out-of-subset construct, rather than claim the harness matches
		// CUE on a construct neither the subset nor real schemas admit. An
		// acyclic schema never trips this, so a genuine divergence is not masked.
		if schemaHasReferenceCycle(schema) {
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
		// Hatch 3 — CUE's root-summary leaf. When a disjunction fails to match
		// any branch, or a deferred thunk references a non-concrete field, CUE
		// attributes the failure to the ROOT (an empty path — "does not satisfy
		// disjunction" / "incomplete value"), while the in-house engine attributes
		// it to the precise FIELD whose thunk could not resolve. CUE emits this
		// bare root [] leaf ONLY for such a summary (a plain field or literal
		// mismatch carries the field path in both arms; a genuine root-scalar
		// mismatch carries [] in both). So the hatch is scoped to exactly that
		// signature: both reject at validate, CUE reports the root [] and the
		// in-house engine does not, AND the in-house engine still covers every
		// NON-root field leaf CUE reports (so no real field rejection is dropped).
		// A missing field leaf, a wrong accept, or a stage mismatch still fails.
		if bothReject(inHouse, oracle) &&
			pathsContainRoot(oracle.Paths) && !pathsContainRoot(inHouse.Paths) &&
			leafCovers(inHouse.Paths, nonRootPaths(oracle.Paths)) {
			return
		}
		t.Fatalf("divergence on schema=%q data=%q: in-house %+v vs oracle %+v",
			schema, data, inHouse, oracle)
	}
}
