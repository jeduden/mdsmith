package cuelitetest

import (
	"slices"
	"testing"
)

// TestRunExpr_corpus runs the surface-C differential arm over the row-expr
// corpus: every checked-in row expression class plus adversarial cases. Each
// case evaluates through both the in-house cuelite evaluator and the direct-CUE
// oracle and must agree on accept/reject and the exact string produced.
func TestRunExpr_corpus(t *testing.T) {
	RunExpr(t, exprCorpus())
}

// TestExprCorpus_uniqueNames guards the corpus: a duplicated case name would
// hide a divergence behind a confusing message. Every name must be distinct.
func TestExprCorpus_uniqueNames(t *testing.T) {
	names := sortedExprNames(exprCorpus())
	for i := 1; i < len(names); i++ {
		if names[i] == names[i-1] {
			t.Errorf("duplicate expr-corpus case name %q", names[i])
		}
	}
}

// bigCoverageRowExpr is the canonical coverage-matrix row expression from
// docs/research/markdownlint-coverage/README.md: the richest real row-expr,
// combining interpolation, `+` concatenation, the ternary idiom, nested
// for-comprehensions, strings.Join, empty-list guards, and fm["key"] access.
const bigCoverageRowExpr = `
  "| [\(id)](../../../internal/rules/" +
  "\(id)-\(name)/README.md) \(name)" +
  [if status != "ready" {" (not-ready)"},
   if status == "ready" {""}][0] +
  " | " +
  [if markdownlint == [] {"—"},
   if markdownlint != [] {
     strings.Join([for m in markdownlint {
       "\(m.id) " +
       [if m.default {"✅"}, if !m.default {"⚪"}][0] +
       [if m.id != m.name {" \(m.name)"},
        if m.id == m.name {""}][0] +
       [if m.partial {" (partial)"},
        if !m.partial {""}][0]
     }], ", ")
   }][0] +
  " | " +
  [if fm["obsidian-linter"] == [] {"—"},
   if fm["obsidian-linter"] != [] {
     strings.Join([for m in fm["obsidian-linter"] {
       "\(m.id) " +
       [if m.default {"✅"}, if !m.default {"⚪"}][0]
     }], ", ")
   }][0] +
  " |"`

// mappingRowExpr is the markdownlint-mapping.md row-expr: a leading
// strings.Join over a list-typed field, then a trailing interpolated cell.
const mappingRowExpr = `"| " +
  strings.Join([for m in markdownlint {
    "\(m.id)" +
    [if m.id != m.name {" \(m.name)"},
     if m.id == m.name {""}][0] +
    [if m.partial {" (partial)"},
     if !m.partial {""}][0]
  }], ", ") +
  " | [\(id)](../../internal/rules/\(id)-\(name)/README.md) \(name) |"`

// guideRowExpr is the generating-content.md guide example: a per-peer
// projection with an inline status mark interpolation.
const guideRowExpr = `"| \(id) | " +
  strings.Join(
    [for m in markdownlint {
      "\(m.id) \([if m.default {"✅"}, if !m.default {"⚪"}][0]) \(m.name)"
    }],
    ", "
  ) + " |"`

// realFileScopeJSON is a front-matter scope shaped like a real rule README:
// scalar id/name/status and list-of-struct peer mappings, including an
// obsidian-linter key that is not a valid CUE identifier.
const realFileScopeJSON = `{
  "id": "MDS064",
  "name": "atx-heading-whitespace",
  "status": "ready",
  "markdownlint": [
    {"id": "MD018", "name": "no-missing-space-atx", "default": true, "partial": false},
    {"id": "MD020", "name": "no-missing-space-closed-atx", "default": true, "partial": true}
  ],
  "rumdl": [],
  "obsidian-linter": [
    {"id": "headings-start-line", "name": "headings-start-line", "default": false, "partial": true}
  ]
}`

// emptyPeersScopeJSON has every peer list empty and a not-ready status, to
// exercise the em-dash and (not-ready) arms.
const emptyPeersScopeJSON = `{
  "id": "MDS029",
  "name": "conciseness-scoring",
  "status": "draft",
  "markdownlint": [],
  "rumdl": [],
  "obsidian-linter": []
}`

// exprCorpus returns the representative set of row expressions for the
// differential expression arm. It pairs the real checked-in row-exprs with
// realistic scopes and adds adversarial cases — nested interpolation,
// comprehension over empty and missing lists, ternary chains, selector on an
// absent field, and builtin arity errors — one row per behaviour class. Every
// row also seeds FuzzExpr so a regression in a known class fails before the
// mutator starts.
//
//nolint:funlen // one table of corpus cases, one row per behaviour class.
func exprCorpus() []ExprCase {
	return []ExprCase{
		// Real checked-in row expressions against realistic scopes.
		{Name: "big coverage matrix row", Expr: bigCoverageRowExpr, ScopeJSON: realFileScopeJSON},
		{Name: "big coverage matrix empty peers", Expr: bigCoverageRowExpr, ScopeJSON: emptyPeersScopeJSON},
		{Name: "markdownlint mapping row", Expr: mappingRowExpr, ScopeJSON: realFileScopeJSON},
		{Name: "generating-content guide row", Expr: guideRowExpr, ScopeJSON: realFileScopeJSON},

		// Interpolation: scalars, numbers, bools, nesting, escapes.
		{Name: "scalar interpolation", Expr: `"\(id) - \(name)"`, ScopeJSON: `{"id":"A","name":"b"}`},
		{Name: "interpolate int", Expr: `"n=\(count)"`, ScopeJSON: `{"count":42}`},
		{Name: "interpolate bool", Expr: `"on=\(flag)"`, ScopeJSON: `{"flag":true}`},
		{Name: "nested interpolation", Expr: `"\("\(id)")"`, ScopeJSON: `{"id":"X"}`},
		{Name: "interpolation with tab escape", Expr: `"a\tb \(id)"`, ScopeJSON: `{"id":"Z"}`},
		// Interpolation dialects (item 1): multiline, raw, raw-multiline, bytes.
		{Name: "multiline interpolation", Expr: "\"\"\"\n  a\\(id)b\n  \"\"\"", ScopeJSON: `{"id":"X"}`},
		{Name: "raw string interpolation", Expr: `#"a\#(id)b"#`, ScopeJSON: `{"id":"X"}`},
		{
			Name:      "raw multiline interpolation",
			Expr:      "#\"\"\"\n  a\\#(id)b\n  \"\"\"#",
			ScopeJSON: `{"id":"X"}`,
		},
		{Name: "bytes interpolation rejected", Expr: `'a\(id)b'`, ScopeJSON: `{"id":"X"}`},
		{Name: "multiline interpolation multibyte", Expr: "\"\"\"\n  héllo \\(id)\n  \"\"\"", ScopeJSON: `{"id":"Z"}`},

		{Name: "interpolate null is rejected", Expr: `"\(x)"`, ScopeJSON: `{"x":null}`},
		{Name: "interpolate list is rejected", Expr: `"\(x)"`, ScopeJSON: `{"x":[1,2]}`},
		{Name: "interpolate struct is rejected", Expr: `"\(x)"`, ScopeJSON: `{"x":{"a":1}}`},

		// Concatenation and arithmetic.
		{Name: "string concat", Expr: `id + " " + name`, ScopeJSON: `{"id":"A","name":"b"}`},
		{Name: "number add is not string", Expr: `1 + 1`, ScopeJSON: ``},
		{Name: "mixed add rejected", Expr: `"a" + 1`, ScopeJSON: ``},

		// Unary + numeric identity (CUE `+(1+2)` == 3).
		{Name: "unary plus int identity", Expr: `"\(+(1+2))"`, ScopeJSON: ``},
		{Name: "unary plus on string rejected", Expr: `+"a"`, ScopeJSON: ``},

		// String repetition (item 2): the FuzzExpr ""*0 find, both operand
		// orders, and the rejected pairings.
		{Name: "string repeat empty zero", Expr: `"" * 0`, ScopeJSON: ``},
		{Name: "string repeat three", Expr: `"ab" * 3`, ScopeJSON: ``},
		{Name: "int times string repeats", Expr: `3 * "ab"`, ScopeJSON: ``},
		{Name: "string repeat by float rejected", Expr: `"x" * 2.0`, ScopeJSON: ``},
		{Name: "int times int rejected", Expr: `2 * 3`, ScopeJSON: ``},

		// Ternary idiom and comparisons.
		{Name: "ternary true", Expr: `[if def {"on"}, if !def {"off"}][0]`, ScopeJSON: `{"def":true}`},
		{Name: "ternary false", Expr: `[if def {"on"}, if !def {"off"}][0]`, ScopeJSON: `{"def":false}`},
		{Name: "ternary chain string eq", Expr: `[if s == "a" {"A"}, if s == "b" {"B"}][0]`, ScopeJSON: `{"s":"b"}`},
		{Name: "empty list guard hit", Expr: `[if xs == [] {"—"}, if xs != [] {"full"}][0]`, ScopeJSON: `{"xs":[]}`},
		{
			Name:      "empty list guard miss",
			Expr:      `[if xs == [] {"—"}, if xs != [] {"full"}][0]`,
			ScopeJSON: `{"xs":["a"]}`,
		},

		// for-comprehension over empty, single, and multiple lists.
		{Name: "for over empty list", Expr: `strings.Join([for x in xs {"\(x)"}], ",")`, ScopeJSON: `{"xs":[]}`},
		{
			Name:      "for over scalar list",
			Expr:      `strings.Join([for x in xs {"\(x)"}], "-")`,
			ScopeJSON: `{"xs":["a","b","c"]}`,
		},
		{
			Name:      "for over struct list",
			Expr:      `strings.Join([for m in xs {"\(m.id)"}], ", ")`,
			ScopeJSON: `{"xs":[{"id":"A"},{"id":"B"}]}`,
		},

		// Field selection and indexing.
		{Name: "fm struct access", Expr: `"\(fm.id)"`, ScopeJSON: `{"id":"MDS001"}`},
		{Name: "fm quoted key", Expr: `fm["my-key"]`, ScopeJSON: `{"my-key":"value"}`},
		{Name: "fm quoted selector", Expr: `fm."my-key"`, ScopeJSON: `{"my-key":"value"}`},
		{Name: "fm quoted selector ident form", Expr: `fm."id"`, ScopeJSON: `{"id":"MDS001"}`},
		{Name: "fm list index", Expr: `"\(fm.xs[0].id)"`, ScopeJSON: `{"xs":[{"id":"MD013"}]}`},
		{Name: "selector on absent field", Expr: `"\(m.absent)"`, ScopeJSON: `{"m":{"id":"X"}}`},
		{Name: "list index out of range", Expr: `xs[5]`, ScopeJSON: `{"xs":["a"]}`},

		// Builtins: strings.Join, len, and arity errors.
		{Name: "strings.Join literals", Expr: `strings.Join(["a","b","c"], "-")`, ScopeJSON: ``},
		{Name: "len of list", Expr: `"\(len(xs))"`, ScopeJSON: `{"xs":[1,2,3]}`},
		{Name: "len of string", Expr: `"\(len(id))"`, ScopeJSON: `{"id":"abc"}`},
		{Name: "strings.Join arity error", Expr: `strings.Join(["a"])`, ScopeJSON: ``},
		{Name: "len arity error", Expr: `len("a","b")`, ScopeJSON: ``},

		// Builtin shadowing (item 3): a scope key or for-variable named `len`
		// shadows the builtin, so `len(...)` calls non-callable data and both arms
		// reject. `strings.Join` stays the builtin since `strings` has no alias.
		{Name: "scope key len shadows builtin", Expr: `len(xs)`, ScopeJSON: `{"len":"shadowed","xs":[1,2]}`},
		{
			Name:      "for variable len shadows builtin",
			Expr:      `[for len in xs {"\(len(len))"}][0]`,
			ScopeJSON: `{"xs":[[1,2]]}`,
		},
		{Name: "strings key does not shadow namespace", Expr: `strings.Join(["a","b"], ",")`, ScopeJSON: `{"strings":"sv"}`},

		// Equality semantics (item 3): struct field-wise, list type-strict,
		// scalar numeric-aware.
		{
			Name:      "struct equality field-wise",
			Expr:      `[if x == y {"T"}, if x != y {"F"}][0]`,
			ScopeJSON: `{"x":{"k":1},"y":{"k":1}}`,
		},
		{
			Name:      "struct inequality field differs",
			Expr:      `[if x == y {"T"}, if x != y {"F"}][0]`,
			ScopeJSON: `{"x":{"k":1},"y":{"k":2}}`,
		},
		{
			Name:      "list element equality type-strict",
			Expr:      `[if x == y {"T"}, if x != y {"F"}][0]`,
			ScopeJSON: `{"x":[2],"y":[2.0]}`,
		},
		{
			Name:      "scalar numeric equality crosses kinds",
			Expr:      `[if x == y {"T"}, if x != y {"F"}][0]`,
			ScopeJSON: `{"x":2,"y":2.0}`,
		},
		{
			Name:      "nested list equality type-strict",
			Expr:      `[if x == y {"T"}, if x != y {"F"}][0]`,
			ScopeJSON: `{"x":[[2]],"y":[[2.0]]}`,
		},

		// len byte count (item 4): multibyte string lengths.
		{Name: "len of multibyte string", Expr: `"\(len(s))"`, ScopeJSON: `{"s":"café"}`},
		{Name: "len of emoji string", Expr: `"\(len(s))"`, ScopeJSON: `{"s":"😀"}`},

		// Scope binding contract (item 6): hidden keys, reserved names, the fm
		// drop, and the selector-vs-index hidden-field rule.
		{Name: "hidden key not bare addressable", Expr: `_key`, ScopeJSON: `{"_key":"hidden"}`},
		{Name: "hidden key not via fm selector", Expr: `fm._key`, ScopeJSON: `{"_key":"hidden"}`},
		{Name: "hidden key via fm index", Expr: `fm["_key"]`, ScopeJSON: `{"_key":"hidden"}`},
		{Name: "hidden key via fm quoted selector", Expr: `fm."_key"`, ScopeJSON: `{"_key":"hidden"}`},
		{Name: "strings key only via fm", Expr: `fm.strings`, ScopeJSON: `{"strings":"sv"}`},
		{Name: "keyword key not bare", Expr: `for`, ScopeJSON: `{"for":"fv"}`},
		{Name: "literal fm key dropped", Expr: `fm["fm"]`, ScopeJSON: `{"fm":"lit"}`},
		{Name: "non-identifier key via fm", Expr: `fm["2x"]`, ScopeJSON: `{"2x":"v"}`},
		{
			Name:      "scaffolding key not bare",
			Expr:      `mdsmith_template_out`,
			ScopeJSON: `{"mdsmith_template_out":"x"}`,
		},
		{
			Name:      "scaffolding key dropped from fm",
			Expr:      `fm["mdsmith_template_out"]`,
			ScopeJSON: `{"mdsmith_template_out":"x"}`,
		},
		// The in-house parse-wrapper field name (mdsmith_row_out) is reserved and
		// dropped exactly like the oracle result field: a scope key colliding with
		// it gets no bare alias and is not reachable via fm. All three reference
		// forms must agree (item 1).
		{
			Name:      "row-out key not bare addressable",
			Expr:      `mdsmith_row_out`,
			ScopeJSON: `{"mdsmith_row_out":"x"}`,
		},
		{
			Name:      "row-out key not via fm selector",
			Expr:      `fm.mdsmith_row_out`,
			ScopeJSON: `{"mdsmith_row_out":"x"}`,
		},
		{
			Name:      "row-out key dropped from fm",
			Expr:      `fm["mdsmith_row_out"]`,
			ScopeJSON: `{"mdsmith_row_out":"x"}`,
		},

		// Result-shape and reference errors.
		{Name: "non-string result", Expr: `42`, ScopeJSON: ``},
		{Name: "missing reference", Expr: `"\(missing)"`, ScopeJSON: `{"id":"X"}`},
		{Name: "plain string literal", Expr: `"literal"`, ScopeJSON: ``},
		{Name: "empty scope literal", Expr: `"x"`, ScopeJSON: ``},
	}
}

// FuzzExpr is the differential fuzz target for surface C: it evaluates each
// (expression, scope JSON) pair through both arms — the in-house cuelite
// evaluator and the direct-CUE oracle — and fails when they disagree on
// accept/reject or on the string produced. It is the broad complement to the
// curated exprCorpus (which pins one case per known behaviour class; the
// fuzzer explores the rest of the expression × scope space around those
// seeds).
//
// It runs as an ordinary test in CI (the f.Add seeds execute with no -fuzz
// flag) and can be driven as a real fuzzer locally with:
//
//	go test -run=- -fuzz=FuzzExpr -fuzztime=30s ./internal/cuelitetest/
//
// Every corpus case seeds the fuzzer so a regression in a known class fails
// immediately and the mutator starts from grammar-relevant expression and
// scope bytes.
func FuzzExpr(f *testing.F) {
	for _, c := range exprCorpus() {
		f.Add(c.Expr, c.ScopeJSON)
	}
	// Extra seeds steering the mutator toward the row subset's boundaries: the
	// builtins, the comprehension forms, the ternary idiom, the add and compare
	// operators, and the interpolation grammar.
	for _, seed := range []struct{ expr, scope string }{
		{`strings.Join([for x in xs {x}], ",")`, `{"xs":["a","b"]}`},
		{`len(xs)`, `{"xs":[1,2,3]}`},
		{`[if c {"y"}, if !c {"n"}][0]`, `{"c":true}`},
		{`a + b`, `{"a":"x","b":"y"}`},
		{`a + b`, `{"a":1,"b":2}`},
		{`[if a == b {"e"}, if a != b {"d"}][0]`, `{"a":1,"b":1}`},
		{`"\(a)\(b)"`, `{"a":1,"b":true}`},
		{`fm["k"]`, `{"k":"v"}`},
		{`xs[0]`, `{"xs":["a"]}`},
		{`"\(x)"`, `{"x":1.5}`},
		{`[if x == y {"T"}, if x != y {"F"}][0]`, `{"x":{"k":1},"y":{"k":1}}`},
		{`[if x == y {"T"}, if x != y {"F"}][0]`, `{"x":[2],"y":[2.0]}`},
		{`"\(len(s))"`, `{"s":"café"}`},
		{`fm."my-key"`, `{"my-key":"value"}`},
		// The CI FuzzExpr find, minimized: string × int repetition. The empty
		// string × zero count is the corner that crashed before evalRowMul
		// landed; it now yields "".
		{`"" * 0`, ``},
		{`"ab" * 3`, ``},
		{`3 * "ab"`, ``},
		{"\"\"\"\n  a\\(id)b\n  \"\"\"", `{"id":"X"}`},
		{`#"a\#(id)b"#`, `{"id":"X"}`},
		{`'a\(id)b'`, `{"id":"X"}`},
		// Float-display divergence (hatch class a): interpolating a fractional
		// float value, the one display difference CUE and the float64 engine keep.
		{`"\(x)"`, `{"x":1.50}`},
		{`"\(x)"`, `{"x":2.0}`},
		// Loud out-of-subset rejections (hatch class b): CUE accepts each, the
		// in-house engine rejects with the "unsupported" wording. One seed per
		// construct pins the class.
		{`strings.Join([for x in xs if x != "b" {x}], ",")`, `{"xs":["a","b","c"]}`},      // for…if combined
		{`strings.Join([for i, x in xs {"\(i):\(x)"}], ",")`, `{"xs":["a","b"]}`},         // for i, x in
		{`strings.Join([for x in xs let y = x + x {y}], ",")`, `{"xs":["a","b"]}`},        // let multi-clause
		{`"\({a:1}.a)"`, ``},                          // struct literal in expr
		{`"\(0.1 + 0.2)"`, ``},                        // float arithmetic
		{`"\(x + 1)"`, `{"x":9223372036854775807}`},   // big-int overflow
		{`"\(len(m))"`, `{"m":{"k":"v"}}`},            // len(struct)
		{`'a\(id)b'`, `{"id":"X"}`},                   // bytes interpolation
		{`"ab" * 2000000`, ``},                        // repetition bound (d45b673)
		// Scope binding contract (item 6): hidden keys, reserved names, the fm
		// drop — both arms must agree.
		{`_key`, `{"_key":"hidden"}`},
		{`fm._key`, `{"_key":"hidden"}`},
		{`fm."_key"`, `{"_key":"hidden"}`},
		{`fm["_key"]`, `{"_key":"hidden"}`},
		{`for`, `{"for":"fv"}`},
		{`fm.strings`, `{"strings":"sv"}`},
		{`fm["fm"]`, `{"fm":"lit"}`},
		{`mdsmith_template_out`, `{"mdsmith_template_out":"x"}`},
		{`mdsmith_row_out`, `{"mdsmith_row_out":"x"}`},
		{`fm.mdsmith_row_out`, `{"mdsmith_row_out":"x"}`},
		{`fm["mdsmith_row_out"]`, `{"mdsmith_row_out":"x"}`},
		{`"\(_strings_used)"`, `{}`},
		// Builtin shadowing (item 3): scope/for binding named `len` shadows the
		// builtin; `strings` namespace stays reserved.
		{`len(xs)`, `{"len":"shadowed","xs":[1,2]}`},
		{`[for len in xs {"\(len(len))"}][0]`, `{"xs":[[1,2]]}`},
		{`strings.Join(["a","b"], ",")`, `{"strings":"sv"}`},
	} {
		f.Add(seed.expr, seed.scope)
	}
	f.Fuzz(func(t *testing.T, expr, scope string) {
		c := ExprCase{Expr: expr, ScopeJSON: scope}
		inHouse := CueLiteExprPath(c)
		oracle := OracleExprPath(c)
		if inHouse.Equal(oracle) {
			return
		}
		// Two documented, divergence-scoped tolerances replace the former
		// scope-scoped float hatch: float-display (both accept, only numeric
		// substrings differ and parse equal-ish) and unsupported-construct
		// (in-house rejects with the "unsupported" wording while CUE accepts). Each
		// is signature-matched to exactly its divergence, so neither masks an
		// unrelated accept/reject mismatch or an accept-vs-accept string diff.
		if HatchedDivergence(inHouse, oracle) {
			return
		}
		t.Fatalf("divergence on expr=%q scope=%q: in-house %+v vs oracle %+v",
			expr, scope, inHouse, oracle)
	})
}

// requireExprCorpusCoversReal is a sanity assertion the corpus test relies on:
// the big real expression must appear, so a future edit that drops it fails
// loudly rather than silently shrinking coverage.
func init() {
	if !slices.ContainsFunc(exprCorpus(), func(c ExprCase) bool {
		return c.Name == "big coverage matrix row"
	}) {
		panic("expr corpus missing the canonical big coverage-matrix row")
	}
}
