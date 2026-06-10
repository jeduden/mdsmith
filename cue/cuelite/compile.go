package cuelite

import (
	stderrors "errors"
	"fmt"
	"regexp"
	"strconv"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/literal"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
)

// compileSource parses a CUE source string with cuelang's syntax frontend
// (cue/parser → cue/ast, the recorded plan-238 decision) and walks the
// resulting AST into the in-house value model. The evaluator — unify,
// validate, concreteness — is fully in-house; the parser only yields an
// AST. An unsupported construct returns a clear error naming it, so a
// future schema using syntax outside the subset fails loudly rather than
// silently mis-evaluating.
//
// After building the value, compileSource reduces it (unifying any
// remaining & operands), so a contradictory constraint like `int & string`
// surfaces as a ⊥ here — the behavior schema/extend.go's checkUnifiable
// depends on.
func compileSource(src string) (*engineValue, error) {
	file, err := parser.ParseFile("", src)
	if err != nil {
		return nil, fmt.Errorf("cuelite: parse: %w", err)
	}
	v, err := compileFile(file)
	if err != nil {
		return nil, err
	}
	// A top-level value that is itself an unforced thunk references a name with
	// no enclosing struct to resolve it (a free `0 > A` expression): the
	// reference can never bind, so it is an error, matching CUE's eager
	// "reference X not found".
	if v.kind == kThunk && len(v.thunkRefs) > 0 {
		return v, fmt.Errorf("reference %q not found", v.thunkRefs[0])
	}
	if v.isBottomV() {
		return v, fmt.Errorf("cuelite: %s", v.reason)
	}
	// A contradiction inside a field (int & string, conflicting bounds, a
	// closed-struct violation) reduces that field to ⊥ without collapsing the
	// whole value. Surface it as a compile error so schema/extend.go's
	// checkUnifiable sees the conflict at schema-compile time.
	if b := firstBottom(v); b != nil {
		return v, fmt.Errorf("cuelite: %s", b.reason)
	}
	return v, nil
}

// compileFile walks the declarations of a parsed CUE file into a value. A
// file whose single declaration is an embedded expression (the `{...}` or
// `close({...})` form query and the schema validator emit) compiles that
// expression directly. Otherwise the top-level declarations are field
// declarations forming an implicit struct (the bare `title: string` form),
// which compiles like a struct literal.
func compileFile(file *ast.File) (*engineValue, error) {
	if len(file.Decls) == 1 {
		if emb, ok := file.Decls[0].(*ast.EmbedDecl); ok {
			return compileExpr(emb.Expr)
		}
	}
	out := &engineValue{kind: kStruct}
	for _, d := range file.Decls {
		f, ok := d.(*ast.Field)
		if !ok {
			return nil, fmt.Errorf("cuelite: unsupported top-level declaration %T", d)
		}
		name, err := fieldLabel(f.Label)
		if err != nil {
			return nil, err
		}
		val, err := compileExpr(f.Value)
		if err != nil {
			return nil, err
		}
		out.fields = appendOrUnifyField(out.fields, field{
			name:     name,
			val:      val,
			optional: f.Constraint == token.OPTION,
		})
	}
	if err := checkThunkRefs(out); err != nil {
		return nil, err
	}
	return out, nil
}

// checkThunkRefs verifies that every deferred (thunk) field references only
// names declared as fields of the same struct. A reference to an undeclared
// name (a free comparison like `nature == "x"` in a malformed catalog
// where-expression) cannot ever resolve, so it is a compile error here rather
// than a thunk that silently ⊥s at validate time — matching CUE's eager
// "reference X not found".
func checkThunkRefs(s *engineValue) error {
	if !hasThunkField(s) {
		return nil
	}
	declared := make(map[string]bool, len(s.fields))
	for _, f := range s.fields {
		declared[f.name] = true
	}
	for _, f := range s.fields {
		if err := checkThunkRefsIn(f.val, declared); err != nil {
			return err
		}
	}
	return nil
}

// checkThunkRefsIn verifies every thunk reachable in v — v itself, or a thunk
// nested in a list element or disjunction branch — references only a declared
// name. It descends into the same positions the force pass reaches (lists,
// disjunctions) but NOT into a nested struct, whose own thunks reference its
// own fields and are checked when that struct is built.
func checkThunkRefsIn(v *engineValue, declared map[string]bool) error {
	switch v.kind {
	case kThunk:
		for _, ref := range v.thunkRefs {
			if !declared[ref] {
				return fmt.Errorf("reference %q not found", ref)
			}
		}
	case kList:
		for _, el := range v.prefix {
			if err := checkThunkRefsIn(el, declared); err != nil {
				return err
			}
		}
		if v.elem != nil {
			if err := checkThunkRefsIn(v.elem, declared); err != nil {
				return err
			}
		}
	case kDisjoint:
		for _, br := range v.branches {
			if err := checkThunkRefsIn(br, declared); err != nil {
				return err
			}
		}
	}
	return nil
}

// appendOrUnifyField adds f to fields, unifying with an existing field of
// the same name (CUE merges repeated fields by &) so a source that
// declares a key twice composes its constraints rather than shadowing.
func appendOrUnifyField(fields []field, f field) []field {
	for i, ex := range fields {
		if ex.name == f.name {
			fields[i].val = unifyV(ex.val, f.val, []string{f.name})
			fields[i].optional = ex.optional && f.optional
			return fields
		}
	}
	return append(fields, f)
}

// compileExpr walks one AST expression node into a value at compile time —
// the single scope-free entry point. It is the unscoped face of [evalExpr]:
// it evaluates e with a nil scope (so every sibling reference is unresolved).
// A deferrable construct (an index expression or a relational comparison over
// a sibling field, the release-channels ternary idiom) that cannot resolve
// becomes a kThunk to re-evaluate once data fixes the reference. Any other
// unresolved reference (a bare `undefinedRef`, a `0 > A`) is a hard
// "reference X not found" error — the subset has no scopes, so a name with no
// declared field can never bind. A fully resolvable expression compiles to
// its value.
func compileExpr(e ast.Expr) (*engineValue, error) {
	v, err := evalExpr(e, nil)
	if err == nil {
		return v, nil
	}
	if stderrors.Is(err, errUnresolved) {
		if isDeferrable(e) {
			return deferToThunk(e), nil
		}
		// A non-deferrable unresolved reference (a bare ident, or a comparison
		// whose result the subset cannot use lazily) names a field that cannot
		// exist. errUnresolved only originates from evalIdent on a reference
		// name, so freeRefs always finds at least that name; report CUE's eager
		// wording naming the first free reference.
		return nil, fmt.Errorf("reference %q not found", freeRefs(e)[0])
	}
	return nil, err
}

// isDeferrable reports whether an expression may be deferred to a kThunk when
// it references a still-unresolved sibling field: an index expression
// (`[if c {…}, …][k]`) or a relational comparison (`A != ""`) — the two
// constructs the release-channels ternary idiom uses. A bare reference or any
// other construct is not deferrable, so an unresolved reference in it is a
// compile error rather than a thunk that can never resolve.
func isDeferrable(e ast.Expr) bool {
	switch n := e.(type) {
	case *ast.ParenExpr:
		return isDeferrable(n.X)
	case *ast.IndexExpr:
		return true
	case *ast.BinaryExpr:
		switch n.Op {
		case token.GEQ, token.LEQ, token.GTR, token.LSS, token.NEQ, token.MAT, token.NMAT, token.EQL:
			return true
		}
	}
	return false
}

// compileBasicLit builds a concrete scalar from a literal token.
func compileBasicLit(n *ast.BasicLit) (*engineValue, error) {
	switch n.Kind {
	case token.STRING:
		s, err := literal.Unquote(n.Value)
		if err != nil {
			return nil, fmt.Errorf("cuelite: string literal %s: %w", n.Value, err)
		}
		return &engineValue{kind: kString, str: s}, nil
	case token.INT:
		i, err := strconv.ParseInt(stripUnderscores(n.Value), 0, 64)
		if err != nil {
			// The in-house engine represents integers as int64; CUE uses
			// arbitrary-precision big.Int. An int literal outside the int64 range
			// is outside the supported subset, not a malformed literal — report
			// it as unsupported so the cross-engine fuzzer's strict-subset hatch
			// recognizes the class.
			return nil, fmt.Errorf("cuelite: unsupported int literal %s (outside int64 range): %w", n.Value, err)
		}
		return &engineValue{kind: kInt, i: i}, nil
	case token.FLOAT:
		f, err := strconv.ParseFloat(stripUnderscores(n.Value), 64)
		if err != nil {
			// The in-house engine represents floats as float64; a literal that
			// overflows float64 is outside the supported subset (CUE keeps a
			// big.Float). Report it as unsupported for the same reason as INT.
			return nil, fmt.Errorf("cuelite: unsupported float literal %s (outside float64 range): %w", n.Value, err)
		}
		return &engineValue{kind: kFloat, f: f}, nil
	case token.TRUE:
		return &engineValue{kind: kBool, b: true}, nil
	case token.FALSE:
		return &engineValue{kind: kBool, b: false}, nil
	case token.NULL:
		return &engineValue{kind: kNull}, nil
	default:
		return nil, fmt.Errorf("cuelite: unsupported literal kind %s", n.Kind)
	}
}

// stripUnderscores removes the digit-group separators CUE allows in number
// literals (1_234_567) so strconv can parse them.
func stripUnderscores(s string) string {
	if !containsByte(s, '_') {
		return s
	}
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '_' {
			out = append(out, s[i])
		}
	}
	return string(out)
}

func containsByte(s string, b byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return true
		}
	}
	return false
}

// checkEmbeddedThunkRefs rejects an embedded thunk (a free comparison like
// `{nature == "x"}`) that references a name not declared as a field of the
// enclosing struct: such a reference can never resolve, so it is a compile
// error now rather than a thunk that ⊥s at validate time, matching CUE's
// eager "reference X not found". The thunk may be the embedded value itself or
// nested in a disjunction branch or list element (`{A > "" | ""}`), so the
// scan reuses checkThunkRefsIn to descend the same positions the force pass
// reaches.
func checkEmbeddedThunkRefs(s, embedded *engineValue) error {
	declared := make(map[string]bool, len(s.fields))
	for _, f := range s.fields {
		declared[f.name] = true
	}
	return checkThunkRefsIn(embedded, declared)
}

// fieldLabel extracts the string name of a struct field label, accepting a
// bare identifier or a quoted string label. ast.LabelName handles both and
// reports whether the label is valid; an index, definition, or hidden
// label is rejected as outside the subset.
func fieldLabel(l ast.Label) (string, error) {
	switch lab := l.(type) {
	case *ast.Ident:
		// A definition label (#foo), a hidden label (_foo), or the bare top
		// token `_` is not a data field: CUE excludes it from the data struct
		// (and rejects `_` as a label outright), so it is outside the
		// front-matter subset. Reject it so a schema using one fails loudly
		// rather than treating it as a required data key.
		if lab.Name == "_" || isDefinitionOrHidden(lab.Name) {
			return "", fmt.Errorf(
				"cuelite: unsupported field label %q (definitions and hidden fields are not in the subset)",
				lab.Name)
		}
		return lab.Name, nil
	case *ast.BasicLit:
		if lab.Kind != token.STRING {
			return "", fmt.Errorf("cuelite: unsupported field label %s", lab.Value)
		}
		s, err := literal.Unquote(lab.Value)
		if err != nil {
			return "", fmt.Errorf("cuelite: field label %s: %w", lab.Value, err)
		}
		return s, nil
	default:
		return "", fmt.Errorf("cuelite: unsupported field label %T", l)
	}
}

// isDefinitionOrHidden reports whether a label names a CUE definition (#foo,
// including the bare #) or a hidden field (_foo, but not the top type _),
// neither of which is a data field in the front-matter subset.
func isDefinitionOrHidden(name string) bool {
	if name == "_" {
		return false
	}
	return len(name) > 0 && (name[0] == '#' || name[0] == '_')
}

// compileUnary handles a unary bound operator (>=0 etc. parse as a unary
// expression whose operand is the bound value) and the * default marker.
func compileUnary(n *ast.UnaryExpr) (*engineValue, error) {
	switch n.Op {
	case token.GEQ, token.LEQ, token.GTR, token.LSS, token.NEQ, token.MAT, token.NMAT:
		// The case label is exactly boundOpOf's domain, so the lookup cannot
		// fail; the ok result is discarded.
		op, _ := boundOpOf(n.Op)
		operand, err := compileExpr(n.X)
		if err != nil {
			return nil, err
		}
		return boundFromOperand(op, operand)
	case token.MUL:
		// A * default marker is only valid as a disjunction branch (`*a | b`),
		// where compileDisjunction strips it before compiling the branch. A
		// standalone `*X` is invalid CUE, so reject it here rather than silently
		// treating it as X.
		return nil, fmt.Errorf("cuelite: * default is only valid in a disjunction")
	case token.SUB:
		// Negative numeric literal: -1, -1.5.
		inner, err := compileExpr(n.X)
		if err != nil {
			return nil, err
		}
		return negateNumeric(inner)
	case token.ADD:
		// Unary plus is valid only on a number (+1, +1.5). CUE itself rejects
		// `+x` on a non-number as an "invalid operation" — the in-house engine
		// reports the same class, just eagerly at schema compile rather than
		// deferred inside a disjunction (the cross-engine fuzzer's strict-subset
		// hatch keys on this wording).
		inner, err := compileExpr(n.X)
		if err != nil {
			return nil, err
		}
		if inner.kind != kInt && inner.kind != kFloat {
			return nil, fmt.Errorf("cuelite: invalid operation: unary + requires a number, got %s", inner.describe())
		}
		return inner, nil
	default:
		return nil, fmt.Errorf("cuelite: unsupported unary operator %q", n.Op)
	}
}

// negateNumeric flips the sign of a concrete numeric value, for a negative
// literal operand.
func negateNumeric(v *engineValue) (*engineValue, error) {
	switch v.kind {
	case kInt:
		return &engineValue{kind: kInt, i: -v.i}, nil
	case kFloat:
		return &engineValue{kind: kFloat, f: -v.f}, nil
	default:
		// CUE rejects `-x` on a non-number as an "invalid operation"; the
		// in-house engine reports the same class eagerly.
		return nil, fmt.Errorf("cuelite: invalid operation: cannot negate %s", v.describe())
	}
}

// boundOpOf maps an AST relational token to a boundOp, reporting ok=false for
// a token outside the relational set. Both callers pre-filter to the relational
// tokens, so ok is always true at those sites and discarded; the false return
// is the helper's own total-function guard.
func boundOpOf(t token.Token) (boundOp, bool) {
	switch t {
	case token.GEQ:
		return opGe, true
	case token.LEQ:
		return opLe, true
	case token.GTR:
		return opGt, true
	case token.LSS:
		return opLt, true
	case token.NEQ:
		return opNe, true
	case token.MAT:
		return opMatch, true
	case token.NMAT:
		return opNotMatch, true
	default:
		return 0, false
	}
}

// boundFromOperand builds a bounded scalar from a bound operator and its
// concrete operand value. A =~ requires a string pattern, compiled to a
// regexp once. A relational operand may be numeric (int/float) or, for !=,
// a string. The base atomKind is inferred so a later concrete value is
// type-checked against it.
func boundFromOperand(op boundOp, operand *engineValue) (*engineValue, error) {
	if op == opMatch || op == opNotMatch {
		if operand.kind != kString {
			return nil, fmt.Errorf("cuelite: %s requires a string pattern, got %s", op, operand.describe())
		}
		re, err := regexp.Compile(operand.str)
		if err != nil {
			return nil, fmt.Errorf("cuelite: %s pattern %q: %w", op, operand.str, err)
		}
		return &engineValue{
			kind:   kBound,
			atom:   akString,
			bounds: []bound{{op: op, re: re, src: operand.str}},
		}, nil
	}
	switch operand.kind {
	case kInt:
		return &engineValue{kind: kBound, atom: akNumber, bounds: []bound{{op: op, num: float64(operand.i)}}}, nil
	case kFloat:
		return &engineValue{kind: kBound, atom: akNumber, bounds: []bound{{op: op, num: operand.f}}}, nil
	case kString:
		return &engineValue{kind: kBound, atom: akString, bounds: []bound{{op: op, str: operand.str, isStr: true}}}, nil
	default:
		// A bound whose operand is a type (>string) or other non-scalar is
		// outside the supported subset — the in-house engine models bounds only
		// over concrete scalars. CUE accepts the construct and defers; report it
		// as unsupported so the cross-engine fuzzer's strict-subset hatch keys on
		// the class.
		return nil, fmt.Errorf(
			"cuelite: unsupported bound %s: requires a scalar operand, got %s", op, operand.describe())
	}
}

// compileCall handles the two builtin calls in the subset: close(struct),
// which marks a struct closed, and strings.MinRunes(n), which constrains a
// string's rune length. len() appears only inside a relational expression
// the subset does not evaluate as a standalone value, so a bare len() call
// is rejected with a clear message.
func compileCall(n *ast.CallExpr) (*engineValue, error) {
	switch fn := n.Fun.(type) {
	case *ast.Ident:
		if fn.Name == "close" {
			if len(n.Args) != 1 {
				return nil, fmt.Errorf("cuelite: close() takes one argument")
			}
			inner, err := compileExpr(n.Args[0])
			if err != nil {
				return nil, err
			}
			if inner.kind != kStruct {
				return nil, fmt.Errorf("cuelite: close() requires a struct, got %s", inner.describe())
			}
			closedCopy := *inner
			closedCopy.closed = true
			return &closedCopy, nil
		}
		return nil, fmt.Errorf("cuelite: unsupported function %q", fn.Name)
	case *ast.SelectorExpr:
		return compileSelectorCall(fn, n.Args)
	default:
		return nil, fmt.Errorf("cuelite: unsupported call target %T", n.Fun)
	}
}

// compileSelectorCall handles a package-qualified builtin call. Only
// strings.MinRunes(n) is in the subset; it becomes a string bound that
// requires at least n runes. Any other selector call names the construct
// in its error.
func compileSelectorCall(sel *ast.SelectorExpr, args []ast.Expr) (*engineValue, error) {
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return nil, fmt.Errorf("cuelite: unsupported call target %s", exprText(sel))
	}
	// The parser produces a plain identifier for a selector's member (`pkg.Sel`),
	// so sel.Sel is an *ast.Ident; selName reads it directly.
	selName := selectorName(sel.Sel)
	name := pkg.Name + "." + selName
	if name != "strings.MinRunes" {
		return nil, fmt.Errorf("cuelite: unsupported function %q", name)
	}
	if len(args) != 1 {
		return nil, fmt.Errorf("cuelite: strings.MinRunes takes one argument")
	}
	arg, err := compileExpr(args[0])
	if err != nil {
		return nil, err
	}
	if arg.kind != kInt {
		return nil, fmt.Errorf("cuelite: strings.MinRunes requires an integer, got %s", arg.describe())
	}
	return &engineValue{
		kind:   kBound,
		atom:   akString,
		bounds: []bound{{op: opMinRunes, num: float64(arg.i)}},
	}, nil
}

// selectorName returns the member name of a selector (`pkg.member`). The
// parser produces a plain identifier for a selector's member, so this reads
// the *ast.Ident directly; any other label node renders as "?" for an error
// message rather than failing.
func selectorName(l ast.Label) string {
	if id, ok := l.(*ast.Ident); ok {
		return id.Name
	}
	return "?"
}

// exprText renders an AST expression as its source-ish text for an error
// message, falling back to the Go type when the node is unprintable.
func exprText(e ast.Expr) string {
	switch n := e.(type) {
	case *ast.Ident:
		return n.Name
	case *ast.SelectorExpr:
		return exprText(n.X) + "." + selectorName(n.Sel)
	default:
		return fmt.Sprintf("%T", e)
	}
}
