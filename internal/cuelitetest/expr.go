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
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
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
// false and leaves Result empty, and Reason carries the rejection's error text
// so the divergence-scoped hatch can signature-match the unsupported-construct
// class. Comparing Accepted and Result ensures the two arms agree on
// accept/reject AND on the exact string produced; Reason is diagnostic only and
// is NOT part of Equal.
type ExprOutcome struct {
	Accepted bool
	Result   string
	Reason   string
}

// Equal reports whether two ExprOutcomes agree — the same accept/reject
// decision and, when accepted, the same string result. Reason is excluded: the
// two arms produce different rejection wording (one is CUE's, one is the
// in-house engine's), so comparing it would spuriously fail.
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
		return ExprOutcome{Reason: "scope is not a JSON object"}
	}
	tpl, err := cuelitepkg.CompileRow(c.Expr)
	if err != nil {
		return ExprOutcome{Reason: err.Error()}
	}
	s, err := tpl.Render(scope)
	if err != nil {
		return ExprOutcome{Reason: err.Error()}
	}
	return ExprOutcome{Accepted: true, Result: s}
}

// OracleExprPath evaluates an ExprCase directly through cuelang.org/go — the
// oracle the in-house arm is measured against. It reconstructs the same binding
// model the in-house engine documents: the front-matter map under `fm` (minus
// the `fm` and scaffolding keys), an identifier-safe alias per non-reserved
// key, and the `strings` package available as a builtin namespace.
//
// To avoid leaking an unreachable scaffolding name (the former `_strings_used`
// sink was referenceable and so diverged from the in-house engine, which has no
// such name), it makes the `strings` import LEAK-FREE by a two-pass compile: it
// first compiles the body with NO import, and only when that fails — because the
// expression references `strings`, which is then unresolved — recompiles WITH
// the import (now used, so no unused-import error and no sink). An expression
// that needs no `strings` never sees the import, so no extra name exists for a
// row expression to reach.
func OracleExprPath(c ExprCase) ExprOutcome {
	scope, ok := decodeScope(c.ScopeJSON)
	if !ok {
		return ExprOutcome{Reason: "scope is not a JSON object"}
	}
	// scope came from decodeScope, so every value is a JSON-decoded type that
	// re-marshals cleanly; oracleBody therefore cannot fail here.
	body := oracleBody(scope, c.Expr)
	ctx := cuecontext.New()
	// Pass 1: no import. If the expression does not use strings this succeeds
	// with the correct value and no leaky import name exists.
	if s, accepted := oracleLookup(ctx, body); accepted {
		return ExprOutcome{Accepted: true, Result: s}
	}
	// Pass 2: with the import. Adding the binding can only resolve a `strings`
	// reference, never mask an unrelated error, so a pass-1 failure for any other
	// reason still fails here.
	if s, accepted := oracleLookup(ctx, oracleStringsImport+body); accepted {
		return ExprOutcome{Accepted: true, Result: s}
	}
	return ExprOutcome{Reason: "oracle rejected"}
}

// oracleStringsImport is the import line prepended in the two-pass oracle's
// second pass when the expression references the strings builtin.
const oracleStringsImport = "import \"strings\"\n\n"

// oracleLookup compiles src and reads the row result out of the out field,
// reporting accepted=false on a compile error or a non-string result.
func oracleLookup(ctx *cue.Context, src string) (string, bool) {
	val := ctx.CompileString(src)
	if val.Err() != nil {
		return "", false
	}
	s, err := val.LookupPath(cue.ParsePath(oracleOutField)).String()
	if err != nil {
		return "", false
	}
	return s, true
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
// keywords, the preimported `strings`, the `fm` binding, and EVERY scaffolding
// field name. The scaffolding names come from cuelite.RowScaffoldFieldNames —
// the SAME single source the in-house rowReserved set uses — so the two arms
// cannot drift on which scaffolding keys are reserved (round 2 caught the
// oracle missing `mdsmith_row_out`, which the in-house engine reserves as its
// parse-wrapper field). A front-matter key colliding with a non-scaffolding
// reserved name stays reachable via `fm`; a scaffolding key is dropped from
// `fm` too (oracleBody).
var oracleReserved = buildOracleReserved()

func buildOracleReserved() map[string]bool {
	m := map[string]bool{
		"package": true, "import": true, "for": true, "in": true,
		"if": true, "let": true, "true": true, "false": true,
		"null": true, "_": true, "strings": true,
		oracleFMField: true,
	}
	for _, n := range cuelitepkg.RowScaffoldFieldNames() {
		m[n] = true
	}
	return m
}

// oracleDropped is the set of scope keys the oracle drops from the emitted `fm`
// struct entirely: the `fm` binding (which always wins) and every scaffolding
// field name (RowScaffoldFieldNames), matching the in-house rowDropped set so a
// scaffolding key is reachable through neither a bare alias nor `fm[...]` in
// either arm.
var oracleDropped = buildOracleDropped()

func buildOracleDropped() map[string]bool {
	m := map[string]bool{oracleFMField: true}
	for _, n := range cuelitepkg.RowScaffoldFieldNames() {
		m[n] = true
	}
	return m
}

// oracleBody reconstructs the CUE body the row binding model documents — the
// front-matter map under `fm`, one top-level alias per identifier-safe
// non-reserved key, and the expression in the out field — WITHOUT the leaky
// `_strings_used` sink the former source carried (OracleExprPath adds the
// `strings` import in a separate, sink-free pass only when the expression needs
// it). The `fm` key and BOTH scaffolding field names (oracleDropped, derived
// from the same cuelite.RowScaffoldFieldNames source as the in-house rowDropped
// set) are dropped from the emitted data, so neither arm exposes a scaffolding
// name. Its scope always comes from decodeScope, so every value re-marshals
// cleanly; a marshal failure cannot occur and the error is dropped.
func oracleBody(scope map[string]any, expr string) string {
	emit := make(map[string]any, len(scope))
	aliases := make([]string, 0, len(scope))
	for k, v := range scope {
		if oracleDropped[k] {
			continue
		}
		emit[k] = v
		if oracleIdentRE.MatchString(k) && !oracleReserved[k] {
			aliases = append(aliases, k)
		}
	}
	sort.Strings(aliases)
	fmJSON, _ := json.Marshal(emit)
	src := oracleFMField + ": " + string(fmJSON) + "\n"
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

// HatchedDivergence reports whether a disagreement between the in-house arm and
// the oracle falls in one of the two documented, divergence-scoped tolerance
// classes — and is therefore not a bug. It is the replacement for the former
// scope-scoped float hatch, which masked any string diff whenever the scope
// carried a fractional number (and so could hide an unrelated regression). Each
// class is signature-matched to exactly its divergence:
//
//   - float-display: BOTH arms accept and the only differences between the two
//     result strings are numeric substrings whose parsed values are equal-ish.
//     CUE preserves a float's literal form (`2.0`, `1.50`) while the float64
//     engine renders the shortest round-tripping decimal, so a float VALUE
//     interpolated into a cell differs in display only. A diff in any
//     non-numeric byte still fails.
//   - unsupported-construct: the in-house arm REJECTS with the engine's
//     "unsupported" wording while the oracle ACCEPTS. This covers the
//     constructs CUE admits but the row subset deliberately does not. Its real
//     members, each carrying the "unsupported" token and each seeded below so
//     the class stays pinned, are: a for…if combined comprehension clause; the
//     two-variable `for i, x in` form; a multi-clause comprehension (`let`
//     followed by another clause); `len(struct)`; a struct literal used as an
//     expression value; an int64-overflowing big-int `+`; a bytes
//     interpolation (`'…'`); `float` arithmetic (a `+` with a float operand);
//     and a string repetition (`s * n`) whose count or output size exceeds the
//     maxRepeat bound (added with the d45b673 CodeQL hardening — CUE would
//     repeat, the engine rejects). It is keyed on the error-text class, so it
//     NEVER masks an accept-vs-accept string diff or an
//     in-house-accepts/oracle-rejects mismatch.
func HatchedDivergence(inHouse, oracle ExprOutcome) bool {
	return floatDisplayDivergence(inHouse, oracle) ||
		unsupportedConstructDivergence(inHouse, oracle)
}

// floatDisplayDivergence reports the float-display tolerance: both arms accept
// and the two result strings differ only in numeric substrings whose parsed
// values are equal-ish. A non-numeric difference, or either arm rejecting,
// fails the predicate.
func floatDisplayDivergence(inHouse, oracle ExprOutcome) bool {
	if !inHouse.Accepted || !oracle.Accepted {
		return false
	}
	return numericallyEquivalent(inHouse.Result, oracle.Result)
}

// unsupportedConstructDivergence reports the unsupported-construct tolerance:
// the in-house arm rejected with the engine's "unsupported" wording while the
// oracle accepted. An in-house rejection whose reason is NOT an unsupported
// class (a genuine parse or reference error), or any case where the oracle also
// rejected, fails the predicate — so an accept-vs-accept string diff is never
// masked here.
func unsupportedConstructDivergence(inHouse, oracle ExprOutcome) bool {
	if inHouse.Accepted || !oracle.Accepted {
		return false
	}
	return strings.Contains(inHouse.Reason, "unsupported")
}

// numericallyEquivalent reports whether a and b are identical once every
// maximal numeric run (a CUE-shaped decimal: optional sign, digits, optional
// fraction, optional exponent) is compared by parsed value rather than by
// text. It walks both strings in lockstep: a non-numeric byte must match
// exactly; a numeric run in BOTH positions must parse to equal-ish floats. Any
// other shape mismatch returns false, so the float hatch tolerates only the
// digit-rendering divergence and nothing else.
func numericallyEquivalent(a, b string) bool {
	for len(a) > 0 && len(b) > 0 {
		na, ra := leadingNumber(a)
		nb, rb := leadingNumber(b)
		if na != "" && nb != "" {
			fa, ea := strconv.ParseFloat(na, 64)
			fb, eb := strconv.ParseFloat(nb, 64)
			if ea != nil || eb != nil || !floatEqualish(fa, fb) {
				return false
			}
			a, b = ra, rb
			continue
		}
		if a[0] != b[0] {
			return false
		}
		a, b = a[1:], b[1:]
	}
	return a == b
}

// leadingNumber splits off a maximal CUE-shaped numeric run at the start of s,
// returning the run and the remainder. A leading byte that does not start a
// number returns ("", s). The run is the lexical shape the float-display
// divergence touches — a signless integer or decimal possibly carrying a
// fraction or exponent — so a hyphen that is punctuation (`a-b`) is not eaten as
// a sign.
func leadingNumber(s string) (num, rest string) {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') && i+1 < len(s) && isDigit(s[i+1]) {
		i++
	}
	start := i
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	if i < len(s) && s[i] == '.' && i+1 < len(s) && isDigit(s[i+1]) {
		i++
		for i < len(s) && isDigit(s[i]) {
			i++
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		j := i + 1
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		if j < len(s) && isDigit(s[j]) {
			i = j
			for i < len(s) && isDigit(s[i]) {
				i++
			}
		}
	}
	if i == start {
		return "", s
	}
	return s[:i], s[i:]
}

// isDigit reports whether c is an ASCII digit.
func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// floatEqualish reports whether two float64 values are equal within a tight
// relative tolerance, so the round-trip rendering difference (`1.50` vs `1.5`)
// counts as equal while a genuine value difference does not.
func floatEqualish(a, b float64) bool {
	if a == b {
		return true
	}
	diff := math.Abs(a - b)
	scale := math.Max(math.Abs(a), math.Abs(b))
	return diff <= 1e-9*scale
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
