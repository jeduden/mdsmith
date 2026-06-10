package cuelite

import (
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
	return out, nil
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

// compileExpr walks one AST expression node into a value. It dispatches on
// the node type, covering the subset plan 218 names; an unhandled node is
// an error naming the construct.
func compileExpr(e ast.Expr) (*engineValue, error) {
	switch n := e.(type) {
	case *ast.BasicLit:
		return compileBasicLit(n)
	case *ast.Ident:
		return compileIdent(n)
	case *ast.StructLit:
		return compileStruct(n)
	case *ast.ListLit:
		return compileList(n)
	case *ast.BinaryExpr:
		return compileBinary(n)
	case *ast.UnaryExpr:
		return compileUnary(n)
	case *ast.CallExpr:
		return compileCall(n)
	case *ast.ParenExpr:
		return compileExpr(n.X)
	case *ast.SelectorExpr:
		// A bare selector like strings.MinRunes outside a call is not a value.
		return nil, fmt.Errorf("cuelite: unsupported selector expression %q", exprText(n))
	default:
		return nil, fmt.Errorf("cuelite: unsupported construct %T", e)
	}
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
			return nil, fmt.Errorf("cuelite: int literal %s: %w", n.Value, err)
		}
		return &engineValue{kind: kInt, i: i}, nil
	case token.FLOAT:
		f, err := strconv.ParseFloat(stripUnderscores(n.Value), 64)
		if err != nil {
			return nil, fmt.Errorf("cuelite: float literal %s: %w", n.Value, err)
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

// compileIdent builds a typed atom or the top type from a type keyword.
// number, string, int, float, bool, bytes, null, and _ are the recognized
// idents; any other bare identifier (an unresolved reference) is an error,
// since the subset has no scopes or definitions.
func compileIdent(n *ast.Ident) (*engineValue, error) {
	switch n.Name {
	case "_":
		return topValue(), nil
	case "null":
		return &engineValue{kind: kNull}, nil
	case "true":
		return &engineValue{kind: kBool, b: true}, nil
	case "false":
		return &engineValue{kind: kBool, b: false}, nil
	case "string":
		return &engineValue{kind: kAtom, atom: akString}, nil
	case "int":
		return &engineValue{kind: kAtom, atom: akInt}, nil
	case "float":
		return &engineValue{kind: kAtom, atom: akFloat}, nil
	case "number":
		return &engineValue{kind: kAtom, atom: akNumber}, nil
	case "bool":
		return &engineValue{kind: kAtom, atom: akBool}, nil
	case "bytes":
		return &engineValue{kind: kAtom, atom: akBytes}, nil
	default:
		return nil, fmt.Errorf("cuelite: unsupported reference %q", n.Name)
	}
}

// compileStruct builds a struct value from a struct literal, preserving
// field order. A field label may be a bare identifier or a quoted string;
// the trailing ? marks an optional key. An embedded close() is not handled
// here — close() wraps a struct expression (compileCall).
func compileStruct(n *ast.StructLit) (*engineValue, error) {
	out := &engineValue{kind: kStruct}
	for _, d := range n.Elts {
		f, ok := d.(*ast.Field)
		if !ok {
			return nil, fmt.Errorf("cuelite: unsupported struct element %T", d)
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
	return out, nil
}

// fieldLabel extracts the string name of a struct field label, accepting a
// bare identifier or a quoted string label. ast.LabelName handles both and
// reports whether the label is valid; an index, definition, or hidden
// label is rejected as outside the subset.
func fieldLabel(l ast.Label) (string, error) {
	switch lab := l.(type) {
	case *ast.Ident:
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

// compileList builds a list value. A trailing ...T ellipsis marks an open
// list with the given tail element type (the [...T] form); the leading
// expressions are required prefix elements ([_, ...T] or [a, b]). A closed
// list (no ellipsis) keeps openTop false, so a length mismatch is a ⊥ at
// unify.
func compileList(n *ast.ListLit) (*engineValue, error) {
	out := &engineValue{kind: kList}
	for _, el := range n.Elts {
		if ell, ok := el.(*ast.Ellipsis); ok {
			out.openTop = true
			if ell.Type != nil {
				et, err := compileExpr(ell.Type)
				if err != nil {
					return nil, err
				}
				out.elem = et
			} else {
				out.elem = topValue()
			}
			continue
		}
		ev, err := compileExpr(el)
		if err != nil {
			return nil, err
		}
		out.prefix = append(out.prefix, ev)
	}
	return out, nil
}

// compileBinary handles the binary operators in the subset: & (meet), |
// (disjunction), and the relational/regex bounds (>= <= > < != =~) when
// they appear as a standalone constraint expression (e.g. `>=0 & <=100`
// parses each bound as a UnaryExpr; a binary relational like `len(x) >= 3`
// is handled here). For & the two operands are compiled and met; for | a
// disjunction is built.
func compileBinary(n *ast.BinaryExpr) (*engineValue, error) {
	switch n.Op {
	case token.AND:
		l, err := compileExpr(n.X)
		if err != nil {
			return nil, err
		}
		r, err := compileExpr(n.Y)
		if err != nil {
			return nil, err
		}
		return unifyV(l, r, nil), nil
	case token.OR:
		return compileDisjunction(n)
	case token.GEQ, token.LEQ, token.GTR, token.LSS, token.NEQ, token.MAT:
		return compileRelational(n)
	default:
		return nil, fmt.Errorf("cuelite: unsupported binary operator %q", n.Op)
	}
}

// compileDisjunction flattens a | tree into a disjunction value, compiling
// each branch and recording a *-marked default branch. Nested
// disjunctions flatten so `a | b | c` yields three branches.
func compileDisjunction(n *ast.BinaryExpr) (*engineValue, error) {
	out := &engineValue{kind: kDisjoint}
	var walk func(e ast.Expr) error
	walk = func(e ast.Expr) error {
		if b, ok := e.(*ast.BinaryExpr); ok && b.Op == token.OR {
			if err := walk(b.X); err != nil {
				return err
			}
			return walk(b.Y)
		}
		isDefault := false
		if u, ok := e.(*ast.UnaryExpr); ok && u.Op == token.MUL {
			isDefault = true
			e = u.X
		}
		br, err := compileExpr(e)
		if err != nil {
			return err
		}
		out.branches = append(out.branches, br)
		if isDefault {
			out.def = br
		}
		return nil
	}
	if err := walk(n); err != nil {
		return nil, err
	}
	return out, nil
}

// compileRelational builds a bounded scalar from a binary relational
// expression. The right operand carries the bound value; the base kind is
// inferred from that operand (a string operand for != "" makes a string
// bound, a number operand makes a numeric bound). A =~ takes a string
// pattern compiled once here.
func compileRelational(n *ast.BinaryExpr) (*engineValue, error) {
	op, err := boundOpOf(n.Op)
	if err != nil {
		return nil, err
	}
	operand, err := compileExpr(n.Y)
	if err != nil {
		return nil, err
	}
	return boundFromOperand(op, operand)
}

// compileUnary handles a unary bound operator (>=0 etc. parse as a unary
// expression whose operand is the bound value) and the * default marker.
func compileUnary(n *ast.UnaryExpr) (*engineValue, error) {
	switch n.Op {
	case token.GEQ, token.LEQ, token.GTR, token.LSS, token.NEQ, token.MAT:
		op, err := boundOpOf(n.Op)
		if err != nil {
			return nil, err
		}
		operand, err := compileExpr(n.X)
		if err != nil {
			return nil, err
		}
		return boundFromOperand(op, operand)
	case token.MUL:
		// A bare *X default outside a disjunction collapses to X (a one-branch
		// disjunction with a default is just X).
		return compileExpr(n.X)
	case token.SUB:
		// Negative numeric literal: -1, -1.5.
		inner, err := compileExpr(n.X)
		if err != nil {
			return nil, err
		}
		return negateNumeric(inner)
	case token.ADD:
		return compileExpr(n.X)
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
		return nil, fmt.Errorf("cuelite: cannot negate %s", v.describe())
	}
}

// boundOpOf maps an AST relational token to a boundOp.
func boundOpOf(t token.Token) (boundOp, error) {
	switch t {
	case token.GEQ:
		return opGe, nil
	case token.LEQ:
		return opLe, nil
	case token.GTR:
		return opGt, nil
	case token.LSS:
		return opLt, nil
	case token.NEQ:
		return opNe, nil
	case token.MAT:
		return opMatch, nil
	default:
		return 0, fmt.Errorf("cuelite: unsupported bound operator %q", t)
	}
}

// boundFromOperand builds a bounded scalar from a bound operator and its
// concrete operand value. A =~ requires a string pattern, compiled to a
// regexp once. A relational operand may be numeric (int/float) or, for !=,
// a string. The base atomKind is inferred so a later concrete value is
// type-checked against it.
func boundFromOperand(op boundOp, operand *engineValue) (*engineValue, error) {
	if op == opMatch {
		if operand.kind != kString {
			return nil, fmt.Errorf("cuelite: =~ requires a string pattern, got %s", operand.describe())
		}
		re, err := regexp.Compile(operand.str)
		if err != nil {
			return nil, fmt.Errorf("cuelite: =~ pattern %q: %w", operand.str, err)
		}
		return &engineValue{
			kind:   kBound,
			atom:   akString,
			bounds: []bound{{op: opMatch, re: re, src: operand.str}},
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
		return nil, fmt.Errorf("cuelite: bound %s requires a scalar operand, got %s", op, operand.describe())
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
	selName, _, err := ast.LabelName(sel.Sel)
	if err != nil {
		return nil, fmt.Errorf("cuelite: unsupported selector: %w", err)
	}
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

// exprText renders an AST expression as its source-ish text for an error
// message, falling back to the Go type when the node is unprintable.
func exprText(e ast.Expr) string {
	switch n := e.(type) {
	case *ast.Ident:
		return n.Name
	case *ast.SelectorExpr:
		sel, _, err := ast.LabelName(n.Sel)
		if err != nil {
			return exprText(n.X) + ".?"
		}
		return exprText(n.X) + "." + sel
	default:
		return fmt.Sprintf("%T", e)
	}
}
