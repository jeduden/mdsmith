package cuelite

import (
	stderrors "errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/jeduden/mdsmith/cue/cuelite/syntax"
)

// rowOutField is the synthetic field the parser wraps a row expression in, so
// `<expr>` parses as `mdsmith_row_out: <expr>` — a single field whose value
// is the expression. The name will never collide with a front-matter key
// because Render binds keys, not this field.
const rowOutField = "mdsmith_row_out"

// evalrow.go is the row-expression evaluator (surface C, plan 239): a
// tree-walker that evaluates a CUE expression returning a string against a
// scope of front-matter bindings. It is distinct from the schema evaluator
// (eval.go): a row expression resolves every reference against CONCRETE data
// (the front-matter map), never deferring, and it admits constructs the
// schema subset does not — string interpolation, `+` concatenation, `for`
// comprehensions, general field selection, and the `strings.Join` / `len`
// builtins. The two walkers share the value model and the literal builders
// (compileBasicLit) but keep separate walks so the schema path's deferral
// invariants are untouched.
//
// The entry points are [CompileRow] (parse the expression once, AST cached)
// and [RowTemplate.Render] (evaluate against a front-matter map, yielding a
// concrete string) — the Compile/Render split cuetemplate exposes, so the
// catalog hot path compiles a row-expr once and renders it per matched file.

// RowTemplate is a parsed row expression, ready to evaluate against
// successive front-matter maps. CompileRow validates syntax once and caches
// the AST; Render binds a scope and walks it. A RowTemplate is immutable and
// safe to reuse across Render calls (and goroutines): Render reads the cached
// AST and builds fresh scope and value nodes per call, mutating nothing.
type RowTemplate struct {
	src  string
	expr syntax.Expr
}

// CompileRow parses a row expression into a [RowTemplate]. It checks syntax
// only — an unresolved reference (a front-matter field absent from the map)
// and a non-string result surface from [RowTemplate.Render] against a
// specific scope, mirroring cuetemplate's parse-only Compile. An empty
// expression or a syntactically invalid one is an error.
func CompileRow(expr string) (*RowTemplate, error) {
	if expr == "" {
		return nil, stderrors.New("cuelite: empty row expression")
	}
	parsed, err := parseRowExpr(expr)
	if err != nil {
		return nil, err
	}
	return &RowTemplate{src: expr, expr: parsed}, nil
}

// parseRowExpr parses a single CUE expression by wrapping it in a synthetic
// field, the same parse-only frontend cuetemplate uses. It returns the
// expression node, so Render walks an AST rather than re-parsing per row.
func parseRowExpr(expr string) (syntax.Expr, error) {
	file, err := syntax.ParseFile(rowOutField + ": " + expr)
	if err != nil {
		return nil, fmt.Errorf("cuelite: invalid row expression: %w", err)
	}
	// The wrapped source is `mdsmith_row_out: <expr>`, so a single-expression
	// row parses to exactly one field declaration whose value is that
	// expression. A row source carrying a comma or newline (`1, 2`) parses to
	// several declarations — not a single expression — and is rejected. The
	// leading `mdsmith_row_out:` label guarantees the first declaration is a
	// field, so no non-field first declaration is reachable.
	if len(file.Decls) != 1 {
		return nil, fmt.Errorf("cuelite: row expression must be a single expression")
	}
	return file.Decls[0].(*syntax.Field).Value, nil
}

// Render evaluates the row expression against scope — a front-matter map
// whose keys bind as identifiers — and returns the concrete string result.
// A nil scope is treated as empty. Every map key binds by name (so a row can
// write `id` or `fm.id`); the whole map also binds under `fm` so a key that
// is not a valid identifier is reachable as `fm["my-key"]`. A reference to a
// key absent from scope, or a result that is not a concrete string, is an
// error so a row never silently renders a blank cell.
func (t *RowTemplate) Render(scope map[string]any) (string, error) {
	if scope == nil {
		scope = map[string]any{}
	}
	rs, err := newRowScope(scope)
	if err != nil {
		return "", err
	}
	v, err := evalRow(t.expr, rs)
	if err != nil {
		return "", err
	}
	if v.kind != kString {
		return "", fmt.Errorf("cuelite: row expression must yield a concrete string, got %s", v.describe())
	}
	return v.str, nil
}

// rowScope binds the names a row expression resolves: every front-matter key
// by name, plus the whole map under `fm`. Bindings are lifted into engine
// values once at Render and looked up by the walk.
type rowScope struct {
	vars map[string]*engineValue
}

// newRowScope lifts a front-matter map into a rowScope, binding the names a row
// expression resolves to MATCH the direct-CUE oracle exactly. The contract is:
//
//   - A key binds as a BARE identifier iff it is a CUE-safe identifier
//     (^[A-Za-z][A-Za-z0-9_]*$) AND is not reserved. The reserved names
//     (rowReserved) are the `fm` binding itself, the `strings` builtin
//     namespace, every CUE keyword, and the two scaffolding field names
//     (RowScaffoldFieldNames — the in-house parse wrapper `mdsmith_row_out`
//     and the oracle result field `mdsmith_template_out`): a key colliding with
//     one has no bare alias (bare `strings` is the builtin, bare `for` is the
//     keyword). The differential oracle derives its reserved set from the SAME
//     RowScaffoldFieldNames source, so the two arms agree on every reserved
//     name (round 2 fixed the oracle missing `mdsmith_row_out`). A `_`-prefixed
//     (hidden) key and a non-identifier key (`my-key`, `2x`) likewise get no
//     bare alias.
//   - The whole map binds under `fm` as a struct containing every key EXCEPT a
//     literal `fm` key and the two scaffolding keys (rowDropped): the `fm`
//     binding always wins, so `fm["fm"]` does not reach the data, and a
//     scaffolding key is reachable through neither a bare alias nor `fm[...]`.
//     The oracle drops the same keys.
//   - In the `fm` struct a `_`-prefixed key is reachable via a string INDEX
//     (`fm["_key"]`) but NOT via a bare SELECTOR (`fm._key`): CUE hides
//     `_`-prefixed fields from selection but not from indexing. evalRowSelector
//     enforces the selector half.
//
// A front-matter value type the lifter does not support yields an error rather
// than a silent skip.
func newRowScope(scope map[string]any) (*rowScope, error) {
	vars := make(map[string]*engineValue, len(scope)+1)
	fmStruct := &engineValue{kind: kStruct}
	for k, raw := range scope {
		ev, err := liftMapValue(raw)
		if err != nil {
			// A front-matter value the lifter cannot represent (an unsupported Go
			// type such as a chan) is an encoding failure, the class the earlier
			// cuelang-backed renderer surfaced when json.Marshal refused the value.
			return nil, fmt.Errorf("encoding frontmatter: field %q: %w", k, err)
		}
		// A non-finite float (±Inf, NaN — yaml.v3 decodes `.inf`/`.nan` to these)
		// has no stable string form a Markdown cell can carry, so reject it up
		// front, naming the field. The earlier renderer rejected the same front
		// matter when json.Marshal refused to encode it; this preserves that
		// contract on the in-house engine.
		if err := checkFinite(ev); err != nil {
			return nil, fmt.Errorf("encoding frontmatter: field %q: %w", k, err)
		}
		// A literal `fm` key and the scaffolding field names are dropped from the
		// data struct: the `fm` binding always wins and the oracle filters the
		// scaffolding out of its emitted data, so the data struct never carries
		// those members.
		if !rowDropped[k] {
			fmStruct.fields = append(fmStruct.fields, field{name: k, val: ev})
		}
		if bindsAsBareIdent(k) {
			vars[k] = ev
		}
	}
	vars[fmField] = fmStruct
	return &rowScope{vars: vars}, nil
}

// fmField is the name the whole front-matter map binds under, so a row can
// index any key — including one that is not a valid identifier — as
// `fm["my-key"]`.
const fmField = "fm"

// rowScaffoldOutField is the historical cuetemplate result-field name. It is
// scaffolding in both differential arms: the oracle holds the row result in a
// field of this name, so a front-matter key colliding with it must not shadow
// the result. The in-house engine reserves it too, so the two arms agree that a
// scope key named like the scaffolding is not addressable.
const rowScaffoldOutField = "mdsmith_template_out"

// RowScaffoldFieldNames returns the synthetic scaffolding field names neither
// differential arm exposes as data: the in-house parse-wrapper field
// (`mdsmith_row_out`) and the oracle result field (`mdsmith_template_out`). It
// is the SINGLE SOURCE the in-house reserved/dropped sets and the differential
// oracle both derive these names from, so the two arms cannot drift on which
// scaffolding keys are reserved and dropped (the round-2 review caught the
// oracle missing `mdsmith_row_out`). A scope key colliding with one of these
// gets no bare alias and is dropped from the `fm` struct, so it is reachable
// through neither a bare alias nor `fm[...]`.
func RowScaffoldFieldNames() []string {
	return []string{rowOutField, rowScaffoldOutField}
}

// rowReserved is the set of names a scope key must NOT bind to as a bare
// identifier, mirroring the oracle's reserved set: the `fm` binding, the
// `strings` builtin namespace, the CUE keywords, and the scaffolding field
// names (RowScaffoldFieldNames). A key colliding with one stays reachable
// through `fm` — except the scaffolding and `fm` keys, which are dropped from
// the `fm` struct entirely (see newRowScope).
var rowReserved = buildRowReserved()

func buildRowReserved() map[string]bool {
	m := map[string]bool{
		fmField: true, "strings": true,
		"package": true, "import": true, "for": true, "in": true,
		"if": true, "let": true, "true": true, "false": true, "null": true,
		"_": true,
	}
	for _, n := range RowScaffoldFieldNames() {
		m[n] = true
	}
	return m
}

// rowDropped is the set of scope keys dropped from the `fm` struct entirely:
// `fm` itself (the binding always wins) and the scaffolding field names
// (RowScaffoldFieldNames), which the oracle filters out of its emitted data so
// the in-house arm must too. A dropped key is reachable through neither a bare
// alias nor `fm[...]`.
var rowDropped = buildRowDropped()

func buildRowDropped() map[string]bool {
	m := map[string]bool{fmField: true}
	for _, n := range RowScaffoldFieldNames() {
		m[n] = true
	}
	return m
}

// bindsAsBareIdent reports whether a scope key binds as a bare identifier: it
// must be a CUE-safe identifier (a letter or underscore start is the CUE rule,
// but a `_`-prefixed name is a HIDDEN field with no bare binding, so the start
// must be a letter) and must not be reserved.
func bindsAsBareIdent(k string) bool {
	if rowReserved[k] || !isBareIdentifier(k) {
		return false
	}
	return true
}

// isBareIdentifier reports whether k matches ^[A-Za-z][A-Za-z0-9_]*$ — the
// identifier shape the oracle aliased at top level. A leading digit, an
// embedded hyphen, a leading underscore (hidden), or an empty string fails.
func isBareIdentifier(k string) bool {
	if k == "" {
		return false
	}
	for i := 0; i < len(k); i++ {
		c := k[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case (c >= '0' && c <= '9') || c == '_':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// checkFinite rejects a non-finite float (±Inf or NaN) anywhere in a lifted
// front-matter value, descending nested structs and lists. Such a value has
// no Markdown-renderable string form, so a row referencing it must fail
// loudly rather than emit a garbage cell.
func checkFinite(v *engineValue) error {
	switch v.kind {
	case kFloat:
		if math.IsInf(v.f, 0) || math.IsNaN(v.f) {
			return fmt.Errorf("non-finite number")
		}
	case kStruct:
		for _, f := range v.fields {
			if err := checkFinite(f.val); err != nil {
				return err
			}
		}
	case kList:
		for _, el := range v.prefix {
			if err := checkFinite(el); err != nil {
				return err
			}
		}
	}
	return nil
}

// evalRow walks one row-expression AST node into a concrete value against the
// scope. Unlike the schema walker it never defers: a reference resolves
// against concrete data or it is an error here.
func evalRow(e syntax.Expr, scope *rowScope) (*engineValue, error) {
	switch n := e.(type) {
	case *syntax.BasicLit:
		return compileBasicLit(n)
	case *syntax.Ident:
		return evalRowIdent(n, scope)
	case *syntax.Interpolation:
		return evalRowInterpolation(n, scope)
	case *syntax.BinaryExpr:
		return evalRowBinary(n, scope)
	case *syntax.ParenExpr:
		return evalRow(n.X, scope)
	case *syntax.SelectorExpr:
		return evalRowSelector(n, scope)
	case *syntax.IndexExpr:
		return evalRowIndex(n, scope)
	case *syntax.ListLit:
		return evalRowList(n, scope)
	case *syntax.CallExpr:
		return evalRowCall(n, scope)
	case *syntax.UnaryExpr:
		return evalRowUnary(n, scope)
	default:
		return nil, fmt.Errorf("cuelite: unsupported row construct %T", e)
	}
}

// evalRowUnary evaluates a unary expression. The row subset uses `!` for the
// boolean negation an `if !cond` ternary arm needs, `-` for a negative numeric
// literal, and `+` for the numeric-identity unary CUE admits (`+(1+2)` is 3).
// Any other unary operator is outside the subset.
func evalRowUnary(n *syntax.UnaryExpr, scope *rowScope) (*engineValue, error) {
	v, err := evalRow(n.X, scope)
	if err != nil {
		return nil, err
	}
	switch n.Op {
	case syntax.NOT:
		if v.kind != kBool {
			return nil, fmt.Errorf("cuelite: ! requires a bool, got %s", v.describe())
		}
		return &engineValue{kind: kBool, b: !v.b}, nil
	case syntax.SUB:
		// Negating int64 min has no int64 representation (CUE, arbitrary
		// precision, yields 9223372036854775808): reject it as out-of-subset
		// rather than silently wrapping, consistent with checkedAddInt64's
		// policy on `+`.
		if v.kind == kInt && v.i == math.MinInt64 {
			return nil, fmt.Errorf(
				"cuelite: unsupported integer overflow in -(%d) (big integers are not in the subset)", v.i)
		}
		return negateNumeric(v)
	case syntax.ADD:
		return identityNumeric(v)
	default:
		return nil, fmt.Errorf("cuelite: unsupported unary operator %q", n.Op)
	}
}

// identityNumeric returns a concrete numeric value unchanged, implementing CUE's
// unary `+` (a numeric identity: `+(1+2)` is 3, `+(-1.5)` is -1.5). A non-number
// operand is an invalid operation, matching CUE's `+"a"` rejection.
func identityNumeric(v *engineValue) (*engineValue, error) {
	switch v.kind {
	case kInt, kFloat:
		return v, nil
	default:
		return nil, fmt.Errorf("cuelite: invalid operation: cannot apply unary + to %s", v.describe())
	}
}

// evalRowIdent resolves a bare identifier to a front-matter field looked up in
// scope (or a comprehension's bound variable). An absent name is an error: the
// row references a field the matched file does not carry. The `true`, `false`,
// and `null` keywords are not identifiers — the parser emits them as basic
// literals (compileBasicLit handles them) — so they never reach here.
func evalRowIdent(n *syntax.Ident, scope *rowScope) (*engineValue, error) {
	v, ok := scope.vars[n.Name]
	if !ok {
		return nil, fmt.Errorf("cuelite: reference %q not found", n.Name)
	}
	return v, nil
}

// evalRowInterpolation evaluates a string interpolation (`"a\(x)b"`): the
// string fragments are already decoded by the in-house parser (the in-house
// syntax tree carries decoded fragments on the Interpolation, unlike CUE's
// raw-token tree), and the embedded expressions evaluate against scope, each
// rendered with CUE's interpolation rules (a string verbatim, a number/bool by
// its CUE textual form). A non-stringable embedded value (null, list, struct)
// is rejected, matching CUE's "invalid interpolation".
//
// The three string dialects (plain `"…"`, raw `#"…"#`, multiline `"""…"""`) all
// decode in the scanner, so this walker reads the decoded fragment text
// directly. A BYTES dialect (`'…\(x)…'`) is flagged on the node (IsBytes) and
// rejected as out-of-subset (the cross-engine fuzzer's strict-subset hatch keys
// on the "unsupported" wording).
func evalRowInterpolation(n *syntax.Interpolation, scope *rowScope) (*engineValue, error) {
	if n.IsBytes {
		return nil, fmt.Errorf("cuelite: unsupported bytes interpolation (bytes are not in the subset)")
	}
	// n.Elts always has an odd length ≥ 3 here: the parser only emits an
	// *syntax.Interpolation when at least one `\(…)` is present, interleaving
	// decoded string fragments (even indices) and embedded expressions (odd
	// indices), so the first and last elements are always string fragments.
	var b strings.Builder
	for i, elt := range n.Elts {
		if i%2 == 1 {
			s, exprErr := evalInterpExpr(elt, scope)
			if exprErr != nil {
				return nil, exprErr
			}
			b.WriteString(s)
			continue
		}
		// An even-index element is a decoded fragment BasicLit; its Value is the
		// fragment text the scanner already unquoted.
		b.WriteString(elt.(*syntax.BasicLit).Value)
	}
	return &engineValue{kind: kString, str: b.String()}, nil
}

// evalInterpExpr evaluates one embedded interpolation expression and renders
// it as CUE would: a string verbatim, an int/float/bool by its textual form.
// A null, list, or struct value has no interpolation rendering and is an
// error.
func evalInterpExpr(e syntax.Expr, scope *rowScope) (string, error) {
	v, err := evalRow(e, scope)
	if err != nil {
		return "", err
	}
	switch v.kind {
	case kString:
		return v.str, nil
	case kInt:
		return strconv.FormatInt(v.i, 10), nil
	case kFloat:
		return formatCUEFloat(v.f), nil
	case kBool:
		return strconv.FormatBool(v.b), nil
	default:
		return "", fmt.Errorf("cuelite: invalid interpolation: cannot use %s as a string", v.describe())
	}
}

// formatCUEFloat renders a float the way CUE renders a number in an
// interpolation. CUE keeps a float's original literal form, which the
// in-house engine — holding only a float64 — cannot reproduce exactly; it
// uses the shortest round-tripping decimal. Front matter rarely interpolates
// a float (the row corpus never does), so this divergence is documented
// rather than matched. A whole-number float still shows a trailing `.0` so it
// reads as a float, not an int.
func formatCUEFloat(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

// evalRowBinary evaluates a row binary expression. `+` concatenates strings
// or adds numbers; `==`/`!=` compare for equality (the comparison an `if`
// comprehension or a `markdownlint == []` guard needs). Other operators are
// outside the row subset.
func evalRowBinary(n *syntax.BinaryExpr, scope *rowScope) (*engineValue, error) {
	l, err := evalRow(n.X, scope)
	if err != nil {
		return nil, err
	}
	r, err := evalRow(n.Y, scope)
	if err != nil {
		return nil, err
	}
	switch n.Op {
	case syntax.ADD:
		return evalRowAdd(l, r)
	case syntax.MUL:
		return evalRowMul(l, r)
	case syntax.EQL:
		return &engineValue{kind: kBool, b: rowEqual(l, r)}, nil
	case syntax.NEQ:
		return &engineValue{kind: kBool, b: !rowEqual(l, r)}, nil
	default:
		return nil, fmt.Errorf("cuelite: unsupported row operator %q", n.Op)
	}
}

// evalRowMul applies `*` to two operands. CUE's `*` over a string and an int —
// in EITHER order (`"ab" * 3` or `3 * "ab"`) — repeats the string that many
// times: the FuzzExpr-minimized `"" * 0` is the empty-string, zero-count corner
// (it yields ""). A negative count is an error ("cannot convert negative number
// to uint64"), matching CUE. Every other `*` operand pairing — string × float,
// int × int (numeric multiplication, out of the string-producing subset), or
// list × int (CUE itself rejects list multiplication in favour of
// list.Repeat) — is rejected as out-of-subset, the cross-engine fuzzer's
// strict-subset hatch keying on the wording.
// maxRepeatCount and maxRepeatBytes bound string repetition: counts must
// stay int-safe on 32-bit targets, and the rendered output of a catalog row
// has no business approaching megabytes. Oversized repetitions reject with
// the out-of-subset wording so the differential hatch covers them.
const (
	maxRepeatCount = 1 << 20
	maxRepeatBytes = 1 << 20
)

func evalRowMul(l, r *engineValue) (*engineValue, error) {
	if s, count, ok := stringRepeatOperands(l, r); ok {
		if count < 0 {
			return nil, fmt.Errorf("cuelite: invalid operation: cannot repeat a string a negative number of times")
		}
		// Bound the count in int64 space before narrowing: int(count) would
		// truncate on 32-bit targets (wasm), and an unbounded count is a
		// memory-amplification hazard regardless of platform. maxRepeatCount
		// also caps the OUTPUT size so a small string cannot expand without
		// limit.
		if count > maxRepeatCount || int64(len(s))*count > maxRepeatBytes {
			return nil, fmt.Errorf("cuelite: unsupported operation: string repetition count too large")
		}
		return &engineValue{kind: kString, str: strings.Repeat(s, int(count))}, nil
	}
	return nil, fmt.Errorf("cuelite: unsupported operation: cannot multiply %s and %s", l.describe(), r.describe())
}

// stringRepeatOperands recognises the string×int repetition pattern in either
// operand order, returning the string, the repetition count, and ok=true when
// one operand is a concrete string and the other a concrete int. A string×float
// or int×int pairing returns ok=false so the caller rejects it as out-of-subset.
func stringRepeatOperands(l, r *engineValue) (string, int64, bool) {
	if l.kind == kString && r.kind == kInt {
		return l.str, r.i, true
	}
	if l.kind == kInt && r.kind == kString {
		return r.str, l.i, true
	}
	return "", 0, false
}

// evalRowAdd applies `+` to two evaluated operands. string+string is
// concatenation — the only `+` the real row corpus uses. int+int is a CHECKED
// addition: CUE is arbitrary-precision, so an int64 overflow is reported as
// out-of-subset rather than silently wrapping (consistent with the big-literal
// policy in compileBasicLit). FLOAT arithmetic — any `+` with a float operand —
// is rejected loudly: the engine holds only a float64, so `0.1 + 0.2` would
// render float64 noise where CUE keeps a decimal, and the real corpus never
// adds floats (the documented plan-239 divergence; display-interpolation of a
// float VALUE is unaffected, it is only float `+` that rejects). A mixed
// string/number is an invalid operation, matching CUE.
func evalRowAdd(l, r *engineValue) (*engineValue, error) {
	if l.kind == kString && r.kind == kString {
		return &engineValue{kind: kString, str: l.str + r.str}, nil
	}
	if l.kind == kInt && r.kind == kInt {
		sum, ok := checkedAddInt64(l.i, r.i)
		if !ok {
			return nil, fmt.Errorf(
				"cuelite: unsupported integer overflow in %d + %d (big integers are not in the subset)", l.i, r.i)
		}
		return &engineValue{kind: kInt, i: sum}, nil
	}
	if l.kind == kFloat || r.kind == kFloat {
		_, lok := l.numericValue()
		_, rok := r.numericValue()
		if lok && rok {
			return nil, fmt.Errorf("cuelite: unsupported float arithmetic (float + is not in the subset)")
		}
	}
	return nil, fmt.Errorf("cuelite: invalid operation: cannot add %s and %s", l.describe(), r.describe())
}

// checkedAddInt64 adds two int64 values, returning ok=false on overflow or
// underflow. CUE's integers are arbitrary-precision, so a sum the int64 engine
// cannot represent is out of the supported subset rather than a silent wrap.
func checkedAddInt64(a, b int64) (int64, bool) {
	sum := a + b
	// Overflow occurred iff the operands share a sign and the sum's sign differs
	// from theirs.
	if (a > 0 && b > 0 && sum < 0) || (a < 0 && b < 0 && sum >= 0) {
		return 0, false
	}
	return sum, true
}

// evalRowSelector evaluates a field selection (`fm.id`, `m.name`): the base
// resolves to a struct and the named member is read out. A selector on a
// non-struct, or a member the struct lacks, is an error.
func evalRowSelector(n *syntax.SelectorExpr, scope *rowScope) (*engineValue, error) {
	base, err := evalRow(n.X, scope)
	if err != nil {
		return nil, err
	}
	name, quoted, err := rowSelectorName(n.Sel)
	if err != nil {
		return nil, err
	}
	// A `_`-prefixed member is a CUE HIDDEN field: a BARE-identifier selector
	// (`fm._key`) cannot reach it, so reject it as not-found, matching the
	// oracle. A QUOTED selector (`fm."_key"`) DOES select the hidden field per
	// CUE, as does a string index (`fm["_key"]`); only the bare-ident form is
	// blocked. So apply the rule only when the label was a bare identifier.
	if !quoted && strings.HasPrefix(name, "_") {
		return nil, fmt.Errorf("cuelite: field %q not found", name)
	}
	return selectField(base, name)
}

// rowSelectorName resolves a selector's member label to the field name to read,
// also reporting whether the label was a QUOTED string. A bare identifier
// (`fm.id`) names the field directly (quoted=false). A quoted string label
// (`fm."my-key"`) is the same member selection CUE allows on a struct, so it
// decodes to its string value (quoted=true) — `fm."?"` selects the literal `?`
// key, not the "?" placeholder the schema-path selectorName falls back to. The
// quoted flag lets evalRowSelector apply the hidden-field (`_`-prefix) rejection
// only to the bare-ident form, since CUE selects a `_`-prefixed field through a
// quoted label but not a bare one.
//
// The CUE parser only ever produces an *syntax.Ident or a STRING *syntax.BasicLit for
// a selector member: a numeric or bytes member is a parse error
// ("expected selector"), so those AST shapes never reach here. A `\(…)` escape
// in the quoted label is impossible (a selector label is not an interpolation),
// so literal.Unquote on a parser-validated STRING token cannot fail; its error
// is returned for completeness but is unreachable from the parser.
func rowSelectorName(l syntax.Label) (name string, quoted bool, err error) {
	if id, ok := l.(*syntax.Ident); ok {
		return id.Name, false, nil
	}
	bl := l.(*syntax.BasicLit)
	s, uerr := syntax.Unquote(bl.Value)
	if uerr != nil {
		return "", false, fmt.Errorf("cuelite: selector label %s: %w", bl.Value, uerr)
	}
	return s, true, nil
}

// selectField reads a named member out of a struct value, erroring when the
// base is not a struct or the field is absent.
func selectField(base *engineValue, name string) (*engineValue, error) {
	if base.kind != kStruct {
		return nil, fmt.Errorf("cuelite: cannot select %q from %s", name, base.describe())
	}
	for _, f := range base.fields {
		if f.name == name {
			return f.val, nil
		}
	}
	return nil, fmt.Errorf("cuelite: field %q not found", name)
}

// evalRowIndex evaluates an index expression. A list indexed by an integer
// selects the element (the `[…][0]` ternary idiom and `fm.markdownlint[0]`); a
// struct indexed by a string selects the field (`fm["my-key"]`). An
// out-of-range or absent index is an error.
func evalRowIndex(n *syntax.IndexExpr, scope *rowScope) (*engineValue, error) {
	base, err := evalRow(n.X, scope)
	if err != nil {
		return nil, err
	}
	idx, err := evalRow(n.Index, scope)
	if err != nil {
		return nil, err
	}
	switch base.kind {
	case kList:
		if idx.kind != kInt {
			return nil, fmt.Errorf("cuelite: list index must be an integer, got %s", idx.describe())
		}
		if idx.i < 0 || idx.i >= int64(len(base.prefix)) {
			return nil, fmt.Errorf("cuelite: list index %d out of range (len %d)", idx.i, len(base.prefix))
		}
		return base.prefix[idx.i], nil
	case kStruct:
		if idx.kind != kString {
			return nil, fmt.Errorf("cuelite: struct index must be a string, got %s", idx.describe())
		}
		return selectField(base, idx.str)
	default:
		return nil, fmt.Errorf("cuelite: cannot index %s", base.describe())
	}
}

// evalRowList builds a list value from a row list literal, applying `if` and
// `for` comprehension clauses. A plain element contributes itself; an `if`
// comprehension contributes its body when the condition holds; a `for`
// comprehension contributes its body once per iterated element. The result is
// a concrete list the index expression or strings.Join consumes.
func evalRowList(n *syntax.ListLit, scope *rowScope) (*engineValue, error) {
	out := &engineValue{kind: kList}
	for _, el := range n.Elts {
		switch e := el.(type) {
		case *syntax.Ellipsis:
			// A row list literal has no use for an open tail; reject it so a
			// stray `...` is not silently dropped.
			return nil, fmt.Errorf("cuelite: open list tail is not valid in a row expression")
		case *syntax.Comprehension:
			elems, err := evalRowComprehension(e, scope)
			if err != nil {
				return nil, err
			}
			out.prefix = append(out.prefix, elems...)
		default:
			ev, err := evalRow(el, scope)
			if err != nil {
				return nil, err
			}
			out.prefix = append(out.prefix, ev)
		}
	}
	return out, nil
}

// evalRowComprehension evaluates a comprehension clause, returning the body
// values it contributes. An `if cond {body}` contributes the body when cond
// is a concrete true; a `for x in list {body}` contributes one body per
// element, binding x. The two clause forms cover the row corpus (the ternary
// idiom and the per-peer projection); any other clause is rejected.
func evalRowComprehension(c *syntax.Comprehension, scope *rowScope) ([]*engineValue, error) {
	if len(c.Clauses) != 1 {
		return nil, fmt.Errorf(
			"cuelite: unsupported multi-clause comprehension (only a single-clause comprehension is in the subset)")
	}
	// The CUE grammar requires a comprehension value to be a brace-delimited
	// struct (`[for x in xs {…}]`), so the parser always yields a *syntax.StructLit
	// here.
	body := c.Value.(*syntax.StructLit)
	// A single-clause comprehension is an `if` or a `for`: a `let` clause cannot
	// stand alone (it must be followed by another clause, which the len != 1
	// guard above already rejects), so those two cover every reachable
	// single-clause shape.
	if ifc, ok := c.Clauses[0].(*syntax.IfClause); ok {
		return evalRowIfClause(ifc, body, scope)
	}
	return evalRowForClause(c.Clauses[0].(*syntax.ForClause), body, scope)
}

// evalRowIfClause evaluates an `if cond {body}` comprehension: the condition
// must reduce to a concrete bool, and the body contributes one value when the
// condition is true. The body is a single-embed struct (`{expr}`), so it
// evaluates to that embedded expression's value.
func evalRowIfClause(clause *syntax.IfClause, body *syntax.StructLit, scope *rowScope) ([]*engineValue, error) {
	cond, err := evalRow(clause.Condition, scope)
	if err != nil {
		return nil, err
	}
	if cond.kind != kBool {
		return nil, fmt.Errorf("cuelite: if condition must be a bool, got %s", cond.describe())
	}
	if !cond.b {
		return nil, nil
	}
	bv, err := evalRowComprehensionBody(body, scope)
	if err != nil {
		return nil, err
	}
	return []*engineValue{bv}, nil
}

// evalRowForClause evaluates a `for x in list {body}` comprehension: it
// iterates the concrete list, binding the clause's value identifier to each
// element, and contributes the body once per element. A `for` over a
// non-list, or with a key variable (the two-variable form), is outside the
// row subset.
func evalRowForClause(clause *syntax.ForClause, body *syntax.StructLit, scope *rowScope) ([]*engineValue, error) {
	if clause.Key != nil {
		return nil, fmt.Errorf(
			"cuelite: unsupported for-comprehension key variable (the two-variable for form is not in the subset)")
	}
	src, err := evalRow(clause.Source, scope)
	if err != nil {
		return nil, err
	}
	if src.kind != kList {
		return nil, fmt.Errorf("cuelite: for-comprehension source must be a list, got %s", src.describe())
	}
	varName := clause.Value.Name
	out := make([]*engineValue, 0, len(src.prefix))
	for _, el := range src.prefix {
		child := scope.with(varName, el)
		bv, bodyErr := evalRowComprehensionBody(body, child)
		if bodyErr != nil {
			return nil, bodyErr
		}
		out = append(out, bv)
	}
	return out, nil
}

// with returns a scope that adds (or shadows) one binding, leaving the parent
// untouched so an outer iteration's binding is not clobbered by an inner one.
func (s *rowScope) with(name string, v *engineValue) *rowScope {
	vars := make(map[string]*engineValue, len(s.vars)+1)
	for k, val := range s.vars {
		vars[k] = val
	}
	vars[name] = v
	return &rowScope{vars: vars}
}

// evalRowComprehensionBody evaluates a comprehension body `{expr}` — a struct
// literal with a single embedded expression — to that expression's value. The
// body of a row comprehension is always a single embed (the projected string),
// so a multi-field or non-embed body is outside the subset.
func evalRowComprehensionBody(body *syntax.StructLit, scope *rowScope) (*engineValue, error) {
	if len(body.Elts) != 1 {
		return nil, fmt.Errorf("cuelite: comprehension body must be a single expression")
	}
	emb, ok := body.Elts[0].(*syntax.EmbedDecl)
	if !ok {
		return nil, fmt.Errorf("cuelite: comprehension body must embed an expression")
	}
	return evalRow(emb.Expr, scope)
}

// evalRowCall evaluates a row builtin call. The row subset has two builtins:
// `strings.Join(list, sep)` and `len(string|list)`. Any other call target is
// outside the subset.
//
// A bare-identifier call target resolves against SCOPE before the builtin
// registry, matching CUE's lexical scoping: a scope key or for-variable named
// `len` shadows the `len` builtin. Since no row value is callable, a shadowed
// target is a "cannot call non-function" error, exactly as CUE rejects
// `len(xs)` when `len` is bound to data. A package-qualified target
// (`strings.Join`) is never a scope name (the `strings` namespace has no bare
// alias and the row grammar binds no package), so it goes straight to the
// builtin registry.
func evalRowCall(n *syntax.CallExpr, scope *rowScope) (*engineValue, error) {
	if id, ok := n.Fun.(*syntax.Ident); ok {
		if shadow, bound := scope.vars[id.Name]; bound {
			return nil, fmt.Errorf("cuelite: cannot call non-function %s (a %s binding shadows it)",
				id.Name, shadow.describe())
		}
	}
	name, err := rowCallName(n.Fun)
	if err != nil {
		return nil, err
	}
	fn, ok := rowBuiltins[name]
	if !ok {
		return nil, fmt.Errorf("cuelite: unsupported function %q", name)
	}
	args := make([]*engineValue, len(n.Args))
	for i, a := range n.Args {
		av, argErr := evalRow(a, scope)
		if argErr != nil {
			return nil, argErr
		}
		args[i] = av
	}
	return fn(args)
}

// rowCallName renders the call target's name: a bare identifier (`len`) or a
// package-qualified selector (`strings.Join`). Any other target shape is
// rejected.
func rowCallName(fun syntax.Expr) (string, error) {
	switch f := fun.(type) {
	case *syntax.Ident:
		return f.Name, nil
	case *syntax.SelectorExpr:
		pkg, ok := f.X.(*syntax.Ident)
		if !ok {
			return "", fmt.Errorf("cuelite: unsupported call target %s", exprText(f))
		}
		return pkg.Name + "." + selectorName(f.Sel), nil
	default:
		return "", fmt.Errorf("cuelite: unsupported call target %T", fun)
	}
}

// rowBuiltin is one row-expression builtin: it validates arity and operand
// kinds and returns the result value.
type rowBuiltin func(args []*engineValue) (*engineValue, error)

// rowBuiltins is the row-expression builtin registry. It replaces an ad-hoc
// call switch with a lookup so adding a builtin is one map entry. Only the
// builtins the real row corpus uses are wired: strings.Join and len.
var rowBuiltins = map[string]rowBuiltin{
	"strings.Join": rowStringsJoin,
	"len":          rowLen,
}

// rowStringsJoin implements strings.Join(list, sep): it joins a list of
// concrete strings with a string separator. A non-list first argument, a
// non-string separator, or a non-string list element is an error, matching
// CUE's argument checks.
func rowStringsJoin(args []*engineValue) (*engineValue, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("cuelite: strings.Join takes two arguments")
	}
	list, sep := args[0], args[1]
	if list.kind != kList {
		return nil, fmt.Errorf("cuelite: strings.Join requires a list, got %s", list.describe())
	}
	if sep.kind != kString {
		return nil, fmt.Errorf("cuelite: strings.Join separator must be a string, got %s", sep.describe())
	}
	parts := make([]string, len(list.prefix))
	for i, el := range list.prefix {
		if el.kind != kString {
			return nil, fmt.Errorf("cuelite: strings.Join element %d must be a string, got %s", i, el.describe())
		}
		parts[i] = el.str
	}
	return &engineValue{kind: kString, str: strings.Join(parts, sep.str)}, nil
}

// rowLen implements len(x): the BYTE count of a string or the length of a
// list (the two operand kinds the row subset uses). CUE's len(string) is a
// byte count, not a rune count — `len("café")` is 5 (the é is two UTF-8
// bytes), `len("😀")` is 4 — so this counts bytes to match the oracle. A
// struct is rejected as out-of-subset: CUE's len(struct) is the field count,
// but the row corpus never takes len of a struct, so the construct stays a
// loud out-of-subset rejection (the cross-engine fuzzer's strict-subset hatch
// keys on the "unsupported" wording).
func rowLen(args []*engineValue) (*engineValue, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("cuelite: len takes one argument")
	}
	switch x := args[0]; x.kind {
	case kString:
		return &engineValue{kind: kInt, i: int64(len(x.str))}, nil
	case kList:
		return &engineValue{kind: kInt, i: int64(len(x.prefix))}, nil
	case kStruct:
		return nil, fmt.Errorf("cuelite: unsupported len of a struct (struct length is not in the subset)")
	default:
		return nil, fmt.Errorf("cuelite: len requires a string or list, got %s", x.describe())
	}
}

// rowEqual reports whether two row values are equal for `==`/`!=`, matching
// CUE's two distinct rules. A top-level scalar pair compares with
// numeric-aware equality, so `2 == 2.0` is true. A list or struct, by
// contrast, compares STRUCTURALLY with kind-strict element equality (CUE's
// concreteValueEqual): `{k:1} == {k:1}` is true (field-wise), but `[2] ==
// [2.0]` is false — inside a structure an int and a float are distinct values,
// even though the bare scalars compare equal. concreteValueEqual already
// implements exactly this — a kind-strict list/struct descent that bottoms out
// in concreteEqual for scalars — so the two arms agree on nested cases too.
func rowEqual(a, b *engineValue) bool {
	if a.kind == kList || a.kind == kStruct || b.kind == kList || b.kind == kStruct {
		return concreteValueEqual(a, b)
	}
	return numericAwareEqual(a, b)
}
