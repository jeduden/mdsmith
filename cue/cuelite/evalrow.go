package cuelite

import (
	stderrors "errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/literal"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
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
	expr ast.Expr
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
func parseRowExpr(expr string) (ast.Expr, error) {
	file, err := parser.ParseFile("row", rowOutField+": "+expr)
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
	return file.Decls[0].(*ast.Field).Value, nil
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

// newRowScope lifts a front-matter map into a rowScope. Each top-level key
// binds both by its own name and (via the `fm` struct) under `fm.<key>` /
// `fm["<key>"]`. A front-matter value type the lifter does not support yields
// an error rather than a silent skip.
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
		vars[k] = ev
		fmStruct.fields = append(fmStruct.fields, field{name: k, val: ev})
	}
	// The `fm` binding always wins for the bare `fm` identifier, even when a
	// front-matter key is literally named `fm` — `fm["fm"]` reaches the data.
	vars[fmField] = fmStruct
	return &rowScope{vars: vars}, nil
}

// fmField is the name the whole front-matter map binds under, so a row can
// index any key — including one that is not a valid identifier — as
// `fm["my-key"]`.
const fmField = "fm"

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
func evalRow(e ast.Expr, scope *rowScope) (*engineValue, error) {
	switch n := e.(type) {
	case *ast.BasicLit:
		return compileBasicLit(n)
	case *ast.Ident:
		return evalRowIdent(n, scope)
	case *ast.Interpolation:
		return evalRowInterpolation(n, scope)
	case *ast.BinaryExpr:
		return evalRowBinary(n, scope)
	case *ast.ParenExpr:
		return evalRow(n.X, scope)
	case *ast.SelectorExpr:
		return evalRowSelector(n, scope)
	case *ast.IndexExpr:
		return evalRowIndex(n, scope)
	case *ast.ListLit:
		return evalRowList(n, scope)
	case *ast.CallExpr:
		return evalRowCall(n, scope)
	case *ast.UnaryExpr:
		return evalRowUnary(n, scope)
	default:
		return nil, fmt.Errorf("cuelite: unsupported row construct %T", e)
	}
}

// evalRowUnary evaluates a unary expression. The row subset uses `!` for the
// boolean negation an `if !cond` ternary arm needs, and `-` for a negative
// numeric literal. Any other unary operator is outside the subset.
func evalRowUnary(n *ast.UnaryExpr, scope *rowScope) (*engineValue, error) {
	v, err := evalRow(n.X, scope)
	if err != nil {
		return nil, err
	}
	switch n.Op {
	case token.NOT:
		if v.kind != kBool {
			return nil, fmt.Errorf("cuelite: ! requires a bool, got %s", v.describe())
		}
		return &engineValue{kind: kBool, b: !v.b}, nil
	case token.SUB:
		return negateNumeric(v)
	default:
		return nil, fmt.Errorf("cuelite: unsupported unary operator %q", n.Op)
	}
}

// evalRowIdent resolves a bare identifier to a front-matter field looked up in
// scope (or a comprehension's bound variable). An absent name is an error: the
// row references a field the matched file does not carry. The `true`, `false`,
// and `null` keywords are not identifiers — the parser emits them as basic
// literals (compileBasicLit handles them) — so they never reach here.
func evalRowIdent(n *ast.Ident, scope *rowScope) (*engineValue, error) {
	v, ok := scope.vars[n.Name]
	if !ok {
		return nil, fmt.Errorf("cuelite: reference %q not found", n.Name)
	}
	return v, nil
}

// evalRowInterpolation evaluates a string interpolation (`"a\(x)b"`): the
// string fragments decode as literals and the embedded expressions evaluate
// against scope, each rendered with CUE's interpolation rules (a string
// verbatim, a number/bool by its CUE textual form). A non-stringable embedded
// value (null, list, struct) is rejected, matching CUE's
// "invalid interpolation".
func evalRowInterpolation(n *ast.Interpolation, scope *rowScope) (*engineValue, error) {
	var b strings.Builder
	for i, elt := range n.Elts {
		// The parser interleaves string fragments and embedded expressions, so an
		// EVEN index is always a partial-quote string literal and an ODD index is
		// always an embedded expression (the Elts shape ast.Interpolation
		// documents).
		if i%2 == 0 {
			b.WriteString(interpFragment(elt.(*ast.BasicLit), i, len(n.Elts)))
			continue
		}
		s, exprErr := evalInterpExpr(elt, scope)
		if exprErr != nil {
			return nil, exprErr
		}
		b.WriteString(s)
	}
	return &engineValue{kind: kString, str: b.String()}, nil
}

// interpFragment decodes one string fragment of an interpolation. The first
// fragment carries the opening `"` and a trailing `\(`; the last carries a
// leading `)` and the closing `"`; a middle fragment carries `)` … `\(`. The
// inner bytes are re-wrapped in double quotes and unquoted, so escapes decode
// exactly as in a plain string literal. The parser already validated every
// escape of the whole interpolation, so re-wrapping a fragment in `"…"` and
// unquoting it never fails — the only string dialect the row subset's
// interpolation grammar admits is the double-quote one.
func interpFragment(bl *ast.BasicLit, i, total int) string {
	raw := bl.Value
	// Strip the leading delimiter — the opening `"` on the first fragment, else
	// the `)` that closes the preceding interpolation — and the trailing
	// delimiter — the closing `"` on the last fragment, else the `\(` that opens
	// the next interpolation.
	end := len(raw) - 2
	if i == total-1 {
		end = len(raw) - 1
	}
	dec, _ := literal.Unquote(`"` + raw[1:end] + `"`)
	return dec
}

// evalInterpExpr evaluates one embedded interpolation expression and renders
// it as CUE would: a string verbatim, an int/float/bool by its textual form.
// A null, list, or struct value has no interpolation rendering and is an
// error.
func evalInterpExpr(e ast.Expr, scope *rowScope) (string, error) {
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
func evalRowBinary(n *ast.BinaryExpr, scope *rowScope) (*engineValue, error) {
	l, err := evalRow(n.X, scope)
	if err != nil {
		return nil, err
	}
	r, err := evalRow(n.Y, scope)
	if err != nil {
		return nil, err
	}
	switch n.Op {
	case token.ADD:
		return evalRowAdd(l, r)
	case token.EQL:
		return &engineValue{kind: kBool, b: rowEqual(l, r)}, nil
	case token.NEQ:
		return &engineValue{kind: kBool, b: !rowEqual(l, r)}, nil
	default:
		return nil, fmt.Errorf("cuelite: unsupported row operator %q", n.Op)
	}
}

// evalRowAdd applies `+` to two evaluated operands: string+string is
// concatenation, number+number is addition (kept int when both are int, else
// float). A mixed string/number is an invalid operation, matching CUE.
func evalRowAdd(l, r *engineValue) (*engineValue, error) {
	if l.kind == kString && r.kind == kString {
		return &engineValue{kind: kString, str: l.str + r.str}, nil
	}
	ln, lok := l.numericValue()
	rn, rok := r.numericValue()
	if lok && rok {
		if l.kind == kInt && r.kind == kInt {
			return &engineValue{kind: kInt, i: l.i + r.i}, nil
		}
		return &engineValue{kind: kFloat, f: ln + rn}, nil
	}
	return nil, fmt.Errorf("cuelite: invalid operation: cannot add %s and %s", l.describe(), r.describe())
}

// evalRowSelector evaluates a field selection (`fm.id`, `m.name`): the base
// resolves to a struct and the named member is read out. A selector on a
// non-struct, or a member the struct lacks, is an error.
func evalRowSelector(n *ast.SelectorExpr, scope *rowScope) (*engineValue, error) {
	base, err := evalRow(n.X, scope)
	if err != nil {
		return nil, err
	}
	name := selectorName(n.Sel)
	return selectField(base, name)
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
func evalRowIndex(n *ast.IndexExpr, scope *rowScope) (*engineValue, error) {
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
func evalRowList(n *ast.ListLit, scope *rowScope) (*engineValue, error) {
	out := &engineValue{kind: kList}
	for _, el := range n.Elts {
		switch e := el.(type) {
		case *ast.Ellipsis:
			// A row list literal has no use for an open tail; reject it so a
			// stray `...` is not silently dropped.
			return nil, fmt.Errorf("cuelite: open list tail is not valid in a row expression")
		case *ast.Comprehension:
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
func evalRowComprehension(c *ast.Comprehension, scope *rowScope) ([]*engineValue, error) {
	if len(c.Clauses) != 1 {
		return nil, fmt.Errorf("cuelite: only a single-clause comprehension is supported")
	}
	// The CUE grammar requires a comprehension value to be a brace-delimited
	// struct (`[for x in xs {…}]`), so the parser always yields a *ast.StructLit
	// here.
	body := c.Value.(*ast.StructLit)
	// A single-clause comprehension is an `if` or a `for`: a `let` clause cannot
	// stand alone (it must be followed by another clause, which the len != 1
	// guard above already rejects), so those two cover every reachable
	// single-clause shape.
	if ifc, ok := c.Clauses[0].(*ast.IfClause); ok {
		return evalRowIfClause(ifc, body, scope)
	}
	return evalRowForClause(c.Clauses[0].(*ast.ForClause), body, scope)
}

// evalRowIfClause evaluates an `if cond {body}` comprehension: the condition
// must reduce to a concrete bool, and the body contributes one value when the
// condition is true. The body is a single-embed struct (`{expr}`), so it
// evaluates to that embedded expression's value.
func evalRowIfClause(clause *ast.IfClause, body *ast.StructLit, scope *rowScope) ([]*engineValue, error) {
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
func evalRowForClause(clause *ast.ForClause, body *ast.StructLit, scope *rowScope) ([]*engineValue, error) {
	if clause.Key != nil {
		return nil, fmt.Errorf("cuelite: for-comprehension key variable is not supported")
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
func evalRowComprehensionBody(body *ast.StructLit, scope *rowScope) (*engineValue, error) {
	if len(body.Elts) != 1 {
		return nil, fmt.Errorf("cuelite: comprehension body must be a single expression")
	}
	emb, ok := body.Elts[0].(*ast.EmbedDecl)
	if !ok {
		return nil, fmt.Errorf("cuelite: comprehension body must embed an expression")
	}
	return evalRow(emb.Expr, scope)
}

// evalRowCall evaluates a row builtin call. The row subset has two builtins:
// `strings.Join(list, sep)` and `len(string|list)`. Any other call target is
// outside the subset.
func evalRowCall(n *ast.CallExpr, scope *rowScope) (*engineValue, error) {
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
func rowCallName(fun ast.Expr) (string, error) {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name, nil
	case *ast.SelectorExpr:
		pkg, ok := f.X.(*ast.Ident)
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
