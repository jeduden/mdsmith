package cuelitetest

// expr.go — differential harness for surface C: the row-expression evaluator.
//
// This file adds an expression-comparing arm to the cuelitetest harness.
// Surface C (catalog row-expr) evaluates a CUE expression against a
// front-matter scope and produces a string. The schema/data and path arms in
// the main harness do not cover it: a row expression has no schema or data
// document, and it evaluates rather than validates.
//
// So surface C has its own:
//   - ExprCase — one expression plus a front-matter scope (as JSON).
//   - ExprOutcome — accepted-with-string or rejected.
//   - ExprPath — an evaluation strategy (in-house or oracle).
//   - RunExpr — the CI-visible runner that compares both arms.
//
// The in-house arm calls cue/cuelite.CompileRow + Render — the pure-Go
// evaluator plan 239 lands. The oracle arm reconstructs the CUE source the
// former cuetemplate.buildSource emitted (a `strings` import, the front-matter
// map under `fm`, identifier-safe top-level aliases, and the expression in a
// synthetic out field) and evaluates it through cuelang.org/go directly. So
// the two arms compare the SAME contract — "a CUE expression returning a
// string over a front-matter scope" — rather than the oracle silently
// diverging from the binding model the in-house engine implements.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"

	cuelitepkg "github.com/jeduden/mdsmith/cue/cuelite"
)

// ExprCase is one differential-test input: a row expression and a
// front-matter scope encoded as a JSON object. Name labels the case in
// failure messages. ScopeJSON is JSON (not a Go map) so a fuzzer can mutate
// it as bytes and so a seed corpus is plain text.
type ExprCase struct {
	Name      string
	Expr      string
	ScopeJSON string
}

// ExprOutcome is the result of evaluating an ExprCase through one arm.
// Accepted reports whether the expression produced a concrete string; when
// true, Result holds that string. A rejection (a syntax error, a missing
// reference, a non-string result, or an unsupported construct) sets Accepted
// false and leaves Result empty. Comparing both fields ensures the two arms
// agree on accept/reject AND on the exact string produced.
type ExprOutcome struct {
	Accepted bool
	Result   string
}

// Equal reports whether two ExprOutcomes agree — the same accept/reject
// decision and, when accepted, the same string result.
func (o ExprOutcome) Equal(other ExprOutcome) bool {
	if o.Accepted != other.Accepted {
		return false
	}
	return o.Result == other.Result
}

// ExprPath is an expression-evaluation strategy: it evaluates an ExprCase and
// returns an ExprOutcome. The in-house arm and the oracle arm are both
// ExprPaths, so RunExpr can call either uniformly.
type ExprPath func(c ExprCase) ExprOutcome

// CueLiteExprPath evaluates an ExprCase through cue/cuelite — the in-house
// arm. A scope that is not a JSON object, an expression that fails to compile,
// and a render error all map to a rejection, mirroring the oracle's
// stage-for-stage rejection.
func CueLiteExprPath(c ExprCase) ExprOutcome {
	scope, ok := decodeScope(c.ScopeJSON)
	if !ok {
		return ExprOutcome{}
	}
	tpl, err := cuelitepkg.CompileRow(c.Expr)
	if err != nil {
		return ExprOutcome{}
	}
	s, err := tpl.Render(scope)
	if err != nil {
		return ExprOutcome{}
	}
	return ExprOutcome{Accepted: true, Result: s}
}

// OracleExprPath evaluates an ExprCase directly through cuelang.org/go — the
// oracle the in-house arm is measured against. It reconstructs the CUE source
// the former cuetemplate emitted, so the comparison is against the exact
// binding model and standard-library surface the row expression was authored
// for: the front-matter map under `fm`, an identifier-safe alias per key, and
// the `strings` package preimported.
func OracleExprPath(c ExprCase) ExprOutcome {
	scope, ok := decodeScope(c.ScopeJSON)
	if !ok {
		return ExprOutcome{}
	}
	// scope came from decodeScope, so every value is a JSON-decoded type that
	// re-marshals cleanly; oracleSource therefore cannot fail here.
	src := oracleSource(scope, c.Expr)
	ctx := cuecontext.New()
	val := ctx.CompileString(src)
	if val.Err() != nil {
		return ExprOutcome{}
	}
	s, err := val.LookupPath(cue.ParsePath(oracleOutField)).String()
	if err != nil {
		return ExprOutcome{}
	}
	return ExprOutcome{Accepted: true, Result: s}
}

// decodeScope parses the ScopeJSON into a front-matter map. An empty string is
// the empty scope (the nil-map case). A non-object JSON document, or invalid
// JSON, is not a usable scope, so both arms reject it identically.
//
// It decodes with UseNumber so an integer literal stays a json.Number rather
// than collapsing to float64 — the in-house lifter then keeps it an int (the
// way a YAML/JSON front-matter decoder preserves `42` as an integer), so the
// interpolation of an integer field renders `42`, not `42.0`. Without this the
// harness would inject a float-vs-int divergence the real front-matter path
// never produces.
func decodeScope(scopeJSON string) (map[string]any, bool) {
	if scopeJSON == "" {
		return map[string]any{}, true
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(scopeJSON)))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, false
	}
	// Reject trailing content after the object, so a two-document scope is not
	// silently truncated to its first value.
	if dec.More() {
		return nil, false
	}
	return m, true
}

// oracleOutField is the synthetic field the oracle holds the expression's
// result in, matching the former cuetemplate scaffolding.
const oracleOutField = "mdsmith_template_out"

// oracleFMField is the field the oracle exposes the whole front-matter map
// under, so `fm["my-key"]` reaches a non-identifier key.
const oracleFMField = "fm"

// oracleIdentRE matches a front-matter key safe to emit as a bare CUE
// identifier alias — the same predicate the former cuetemplate used.
var oracleIdentRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

// oracleReserved lists names the oracle must not alias at top level: CUE
// keywords, the preimported `strings`, and the scaffolding fields. A
// front-matter key colliding with one stays reachable via `fm`.
var oracleReserved = map[string]bool{
	"package": true, "import": true, "for": true, "in": true,
	"if": true, "let": true, "true": true, "false": true,
	"null": true, "_": true, "strings": true,
	oracleOutField: true, oracleFMField: true,
}

// oracleSource reconstructs the CUE source the former cuetemplate.buildSource
// emitted: a `strings` import with a use sink, the front-matter map under
// `fm`, one top-level alias per identifier-safe non-reserved key, and the
// expression in the out field. Its scope always comes from decodeScope, so
// every value re-marshals cleanly; a marshal failure cannot occur and the
// error is dropped.
func oracleSource(scope map[string]any, expr string) string {
	emit := make(map[string]any, len(scope))
	aliases := make([]string, 0, len(scope))
	for k, v := range scope {
		if k == oracleFMField || k == oracleOutField {
			continue
		}
		emit[k] = v
		if oracleIdentRE.MatchString(k) && !oracleReserved[k] {
			aliases = append(aliases, k)
		}
	}
	sort.Strings(aliases)
	fmJSON, _ := json.Marshal(emit)
	src := "import \"strings\"\n\n_strings_used: strings.Join([], \"\")\n"
	src += oracleFMField + ": " + string(fmJSON) + "\n"
	for _, k := range aliases {
		src += fmt.Sprintf("%s: %s.%s\n", k, oracleFMField, k)
	}
	src += fmt.Sprintf("%s: %s\n", oracleOutField, expr)
	return src
}

// CompareExprOutcomes runs one ExprCase through both inHouse and oracle and
// reports a failure on t when the two ExprOutcomes disagree. It returns true
// when they agree.
func CompareExprOutcomes(t testing.TB, inHouse, oracle ExprPath, c ExprCase) bool {
	t.Helper()
	got := inHouse(c)
	want := oracle(c)
	if got.Equal(want) {
		return true
	}
	t.Errorf("expr case %q expr=%q scope=%q: in-house %+v disagrees with oracle %+v",
		c.Name, c.Expr, c.ScopeJSON, got, want)
	return false
}

// RunExpr compares every ExprCase in cases through the in-house and oracle
// arms, reporting each disagreement on t. It is the entry point surface C's
// differential test calls over its corpus.
func RunExpr(t testing.TB, cases []ExprCase) {
	t.Helper()
	for _, c := range cases {
		CompareExprOutcomes(t, CueLiteExprPath, OracleExprPath, c)
	}
}

// sortedExprNames is a small helper the corpus test uses to assert no two
// cases share a name (a duplicate name would mask a divergence behind a
// confusing failure message).
func sortedExprNames(cases []ExprCase) []string {
	names := make([]string, len(cases))
	for i, c := range cases {
		names[i] = c.Name
	}
	sort.Strings(names)
	return names
}
