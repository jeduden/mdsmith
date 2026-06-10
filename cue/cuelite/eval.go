package cuelite

import (
	stderrors "errors"
	"fmt"
	"math"
	"regexp"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/token"
)

// errUnresolved signals that an expression could not be evaluated because it
// references a sibling field whose value is not yet known (a forward or
// data-dependent reference). compileExpr turns such an expression into a
// kThunk that re-evaluates once the enclosing struct's fields resolve, so a
// schema like
//
//	registry: [if mechanism == "push" {string & != ""}, (string | *"")][0]
//
// is deferred until data fixes `mechanism`, then forced.
var errUnresolved = stderrors.New("cuelite: unresolved reference")

// evalExpr is the single AST-to-value builder, threaded by a scope of
// resolved sibling-field values. It serves both callers: compileExpr passes
// scope == nil for the compile-time (unscoped) walk, and the struct force
// pass passes a populated scope when re-evaluating a deferred thunk. A
// reference to a name absent from scope (or present but non-concrete)
// returns errUnresolved so the caller defers the whole expression; with
// scope == nil every reference is unresolved, so compileExpr's IndexExpr/
// relational paths become thunks. The scope-free constructs (literals,
// type keywords, calls, bounds) carry no sibling reference and build the
// same value regardless of scope.
func evalExpr(e ast.Expr, scope map[string]*engineValue) (*engineValue, error) {
	switch n := e.(type) {
	case *ast.BasicLit:
		return compileBasicLit(n)
	case *ast.Ident:
		return evalIdent(n, scope)
	case *ast.IndexExpr:
		return evalIndex(n, scope)
	case *ast.BinaryExpr:
		return evalBinary(n, scope)
	case *ast.ParenExpr:
		return evalExpr(n.X, scope)
	case *ast.ListLit:
		return evalList(n, scope)
	case *ast.StructLit:
		return evalStruct(n, scope)
	case *ast.UnaryExpr:
		// A unary expression (a bound operator >=0, a numeric sign -1) carries no
		// reference, so it has nothing to resolve against scope and goes straight
		// to the compile-time builder. A * default marker is stripped by
		// evalDisjunction before a branch is built, so reaching compileUnary with
		// one yields the "* default is only valid in a disjunction" error.
		return compileUnary(n)
	case *ast.CallExpr:
		return compileCall(n)
	case *ast.SelectorExpr:
		// A bare selector like strings.MinRunes outside a call is not a value.
		return nil, fmt.Errorf("cuelite: unsupported selector expression %q", exprText(n))
	default:
		return nil, fmt.Errorf("cuelite: unsupported construct %T", e)
	}
}

// evalChild evaluates a sub-expression in a position that may hold a
// deferrable construct: a struct field value, a list element, or a
// disjunction branch. At compile time (scope == nil) it applies compileExpr's
// deferral — a deferrable index/relational expression over an unresolved
// reference becomes a kThunk, and any other unresolved reference is a
// "reference X not found" error. During a struct force pass (scope != nil) it
// evaluates directly: the scope already carries the resolved siblings, so an
// unresolved reference there is a genuine failure the caller surfaces.
func evalChild(e ast.Expr, scope map[string]*engineValue) (*engineValue, error) {
	if scope == nil {
		return compileExpr(e)
	}
	return evalExpr(e, scope)
}

// evalIdent resolves a bare identifier: a type keyword or bool/null literal
// builds its value directly, otherwise the name is a sibling-field reference
// looked up in scope. A name absent from scope, or bound to a non-concrete
// value, defers the enclosing expression (errUnresolved) — at compile time
// (a nil scope) every reference is unresolved, so compileExpr turns a
// deferrable construct into a thunk and reports any other reference as
// "reference X not found".
func evalIdent(n *ast.Ident, scope map[string]*engineValue) (*engineValue, error) {
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
	}
	v, ok := scope[n.Name]
	if !ok || v == nil {
		return nil, errUnresolved
	}
	if !isConcrete(v) {
		return nil, errUnresolved
	}
	return v, nil
}

// evalIndex evaluates a list index expression list[k]: it builds the list's
// elements (honoring any comprehension clauses that drop or keep elements),
// then selects the k-th. A constant integer index out of range is a ⊥. The
// ternary idiom `[if c {a}, b][0]` reduces here: when c holds, element 0 is
// a; when c does not, the comprehension contributes nothing and element 0 is
// b.
func evalIndex(n *ast.IndexExpr, scope map[string]*engineValue) (*engineValue, error) {
	idxVal, err := evalExpr(n.Index, scope)
	if err != nil {
		return nil, err
	}
	if idxVal.kind != kInt {
		return nil, fmt.Errorf(
			"cuelite: invalid operation: list index must be an integer, got %s", idxVal.describe())
	}
	list, ok := n.X.(*ast.ListLit)
	if !ok {
		// Indexing a non-list (`"0"[0]`) is an invalid operation CUE also rejects
		// — eagerly here, deferred-and-dropped in a disjunction by CUE.
		return nil, fmt.Errorf(
			"cuelite: invalid operation: index target must be a list literal, got %T", n.X)
	}
	elems, err := evalListElems(list, scope)
	if err != nil {
		return nil, err
	}
	// Bound in int64 space before narrowing: converting first would
	// truncate on 32-bit targets (wasm) and could index a wrong-but-valid
	// element. The math.MaxInt32 bound makes the narrowing provably safe
	// on every platform (a list literal cannot approach 2^31 elements).
	if idxVal.i < 0 || idxVal.i > math.MaxInt32 || int(idxVal.i) >= len(elems) {
		return mkBottom(nil, "list index %d out of range (len %d)", idxVal.i, len(elems)), nil
	}
	return elems[int(idxVal.i)], nil
}

// evalListElems builds the concrete element list of a list literal, applying
// comprehension clauses: a plain element contributes itself, an `if`
// comprehension contributes its body only when the condition holds. The
// ellipsis tail is ignored for indexing (an index past the prefix is out of
// range).
func evalListElems(list *ast.ListLit, scope map[string]*engineValue) ([]*engineValue, error) {
	var out []*engineValue
	// deferErr holds an errUnresolved seen on some element. A HARD error (an
	// unsupported construct, a bad call) is returned immediately, even when an
	// EARLIER element deferred: CUE rejects a `(string*"")` element at compile
	// regardless of whether `[…][0]` would reach it, so a hard error in any
	// element fails the whole list. Only when every element either evaluated or
	// merely deferred does the list itself defer.
	var deferErr error
	for _, el := range list.Elts {
		var err error
		switch e := el.(type) {
		case *ast.Ellipsis:
			// The open tail adds no indexable element.
			continue
		case *ast.Comprehension:
			var keep bool
			var body *engineValue
			keep, body, err = evalComprehension(e, scope)
			if err == nil && keep {
				out = append(out, body)
			}
		default:
			var ev *engineValue
			ev, err = evalExpr(el, scope)
			if err == nil {
				out = append(out, ev)
			}
		}
		if err != nil {
			if !stderrors.Is(err, errUnresolved) {
				return nil, err
			}
			deferErr = err
		}
	}
	if deferErr != nil {
		return nil, deferErr
	}
	return out, nil
}

// evalComprehension evaluates a single-clause `if` comprehension, returning
// whether its body is kept and the body value. Only the `if cond {body}`
// shape the release-channels schema uses is supported; a `for` clause or a
// multi-clause comprehension is rejected with a clear message. The condition
// must reduce to a concrete bool; a non-concrete one defers (errUnresolved).
func evalComprehension(c *ast.Comprehension, scope map[string]*engineValue) (bool, *engineValue, error) {
	if len(c.Clauses) != 1 {
		return false, nil, fmt.Errorf("cuelite: only a single-clause if-comprehension is supported")
	}
	ifc, ok := c.Clauses[0].(*ast.IfClause)
	if !ok {
		return false, nil, fmt.Errorf("cuelite: unsupported comprehension clause %T", c.Clauses[0])
	}
	body, ok := c.Value.(*ast.StructLit)
	if !ok {
		return false, nil, fmt.Errorf("cuelite: comprehension body must be a struct, got %T", c.Value)
	}
	cond, err := evalExpr(ifc.Condition, scope)
	if err != nil {
		if !stderrors.Is(err, errUnresolved) {
			return false, nil, err
		}
		// The condition is an unresolved reference: the comprehension defers.
		// Still compile the BODY so a hard error in it (`{string != ""}`) is
		// caught at compile, matching CUE — which rejects the body's invalid
		// operand regardless of whether the condition selects it.
		return false, nil, deferWithBodyCheck(body, scope)
	}
	if cond.kind != kBool {
		// A CONCRETE non-bool condition (`if ""`, `if 1`) is a type error CUE
		// rejects at compile ("cannot use \"\" (type string) as type bool"). A
		// NON-concrete condition (a type or top, `if string`) defers: the
		// enclosing list element becomes a thunk awaiting data. Distinguishing
		// the two avoids deferring an `if` whose condition can never become a
		// bool — which would surface as an empty-freeRefs panic in compileExpr.
		if isConcrete(cond) {
			return false, nil, fmt.Errorf(
				"cuelite: invalid operation: if condition must be a bool, got %s", cond.describe())
		}
		return false, nil, deferWithBodyCheck(body, scope)
	}
	bv, err := evalStruct(body, scope)
	if err != nil {
		return false, nil, err
	}
	return cond.b, bv, nil
}

// deferWithBodyCheck compiles a deferring comprehension's body solely to
// surface a HARD error (an unsupported construct, a type-mismatched
// comparison) the way CUE rejects the body eagerly. A body whose own
// references merely defer (errUnresolved) is fine — it resolves once the
// comprehension forces against data — so a body deferral returns errUnresolved
// (the comprehension defers); any other body error is returned as the hard
// rejection.
func deferWithBodyCheck(body *ast.StructLit, scope map[string]*engineValue) error {
	if _, err := evalStruct(body, scope); err != nil && !stderrors.Is(err, errUnresolved) {
		return err
	}
	return errUnresolved
}

// evalBinary evaluates a binary expression in a scope. An == or != between a
// resolved reference and a literal yields a concrete bool (driving an `if`
// comprehension); the lattice operators & and | and the relational bounds
// delegate to the compile-time builders after their operands resolve.
func evalBinary(n *ast.BinaryExpr, scope map[string]*engineValue) (*engineValue, error) {
	switch n.Op {
	case token.EQL, token.NEQ, token.GEQ, token.LEQ, token.GTR, token.LSS, token.MAT, token.NMAT:
		return evalComparison(n, scope)
	case token.AND:
		l, err := evalExpr(n.X, scope)
		if err != nil {
			return nil, err
		}
		r, err := evalExpr(n.Y, scope)
		if err != nil {
			return nil, err
		}
		return unifyV(l, r, nil), nil
	case token.OR:
		return evalDisjunction(n, scope)
	default:
		return nil, fmt.Errorf("cuelite: unsupported binary operator %q", n.Op)
	}
}

// evalComparison evaluates a binary comparison (==, !=, >=, <=, >, <, =~, !~)
// of two operands to a concrete bool, the comparison an `if mechanism ==
// "push"` comprehension or an `A != ""` constraint needs. Both sides must
// resolve to concrete scalars; an unresolved reference defers (errUnresolved)
// so the enclosing expression becomes a thunk. A regex comparison (=~ / !~)
// compiles its pattern and tests the left string.
func evalComparison(n *ast.BinaryExpr, scope map[string]*engineValue) (*engineValue, error) {
	l, lerr := evalExpr(n.X, scope)
	r, rerr := evalExpr(n.Y, scope)
	// A non-concrete TYPE operand (`_`, `string`, `int`) — one that EVALUATED
	// without an unresolved-reference error but is still non-concrete — makes
	// the comparison ill-typed: CUE rejects `a > _` and `a == string` at schema
	// compile ("'>' requires concrete value"). This holds even when the OTHER
	// operand is an unresolved reference (`A > _`), so it is checked before the
	// errUnresolved deferral: a type operand can never become concrete, so the
	// comparison can never resolve and must not become a thunk.
	// A HARD operand error (anything but errUnresolved — an unsupported
	// construct like `!0`, or a nested compile failure) can never resolve
	// against data, so the comparison is a compile error, not a deferral.
	// Propagate it before the errUnresolved deferral, even when the OTHER
	// operand is an unresolved reference (`A > !0`): the bad operand makes the
	// whole comparison non-resolvable, matching CUE's eager rejection.
	if lerr != nil && !stderrors.Is(lerr, errUnresolved) {
		return nil, lerr
	}
	if rerr != nil && !stderrors.Is(rerr, errUnresolved) {
		return nil, rerr
	}
	if lerr == nil && !isConcrete(l) {
		return nil, fmt.Errorf("cuelite: invalid operation: %s requires a concrete operand, got %s", n.Op, l.describe())
	}
	if rerr == nil && !isConcrete(r) {
		return nil, fmt.Errorf("cuelite: invalid operation: %s requires a concrete operand, got %s", n.Op, r.describe())
	}
	// Either operand is an unresolved reference: defer the whole comparison to a
	// thunk (the enclosing struct resolves it once data fixes the reference).
	if lerr != nil {
		return nil, lerr
	}
	if rerr != nil {
		return nil, rerr
	}
	res, err := compareConcrete(l, n.Op, r)
	if err != nil {
		return nil, err
	}
	return &engineValue{kind: kBool, b: res}, nil
}

// compareConcrete evaluates a comparison operator over two concrete scalar
// values, returning the boolean result. == / != compare for equality; the
// ordered relations and regex matches reuse the same primitives the bound
// checks use, so a comparison and a bound agree on the same operands.
//
// == / != compare NUMERICALLY across the int/float kinds, matching CUE: the
// expression `2 == 2.0` is true (CUE compares numbers by value, not by kind),
// so the relational `==`/`!=` operators agree with the engine's own ordered
// comparisons. String, bool, and null equality stays kind-strict (a string
// never equals an int, true never equals 1) — CUE rejects a cross-kind `==`
// of non-numbers as a type error, but here both operands are already concrete
// scalars, so an int-vs-string == reduces to "not equal" rather than failing.
func compareConcrete(l *engineValue, op token.Token, r *engineValue) (bool, error) {
	switch op {
	case token.EQL:
		return numericAwareEqual(l, r), nil
	case token.NEQ:
		return !numericAwareEqual(l, r), nil
	case token.MAT, token.NMAT:
		if l.kind != kString || r.kind != kString {
			return false, fmt.Errorf("cuelite: %s requires strings", op)
		}
		re, err := regexp.Compile(r.str)
		if err != nil {
			return false, err
		}
		m := re.MatchString(l.str)
		if op == token.NMAT {
			m = !m
		}
		return m, nil
	}
	// op is one of GEQ/LEQ/GTR/LSS here (EQL/NEQ/MAT/NMAT handled above), all in
	// boundOpOf's domain, so the lookup cannot fail.
	bop, _ := boundOpOf(op)
	if l.kind == kString && r.kind == kString {
		return compareStr(l.str, bop, r.str), nil
	}
	ln, lok := l.numericValue()
	rn, rok := r.numericValue()
	if !lok || !rok {
		return false, fmt.Errorf("cuelite: invalid operation: cannot compare %s and %s", l.describe(), r.describe())
	}
	return compareNum(ln, bop, rn), nil
}

// numericAwareEqual reports whether two concrete scalars are equal for the
// relational == / != operators. Two numbers compare by VALUE across int and
// float (2 == 2.0), matching CUE; every other pair (string, bool, null, or a
// number against a non-number) falls back to concreteEqual's kind-strict
// equality. This differs from concreteEqual — which keeps a concrete int and
// float DISTINCT for unification (the literals 0 and 0.0 do not unify) — so the
// relational operator and the lattice meet deliberately use different rules.
func numericAwareEqual(a, b *engineValue) bool {
	an, aok := a.numericValue()
	bn, bok := b.numericValue()
	if aok && bok {
		return an == bn
	}
	return concreteEqual(a, b)
}

// evalDisjunction builds a disjunction value in a scope, flattening nested |
// and recording every *-marked default disjunct. A branch that defers leaves
// the whole disjunction deferred.
//
// Construction is where CUE's build-time disjunction reductions happen:
//   - A ⊥ disjunct is dropped (CUE: `0&1 | 2` keeps only 2); if every disjunct
//     is ⊥ the disjunction is itself ⊥ — CUE reports "errors in empty
//     disjunction" at compile time.
//   - Equal concrete disjuncts collapse to one (`"x" | "x"` is the concrete
//     "x"), so the result is concrete rather than a stuck two-branch value.
//   - A parenthesized nested disjunction flattens, and its inner default marks
//     are carried up (`(*1|2)|3` keeps 1 as the default), so the nested
//     default is not lost.
//
// A defer (an unresolved sibling reference) skips these reductions and leaves
// the whole expression to become a thunk; the reductions then run when the
// thunk is forced against data.
func evalDisjunction(n *ast.BinaryExpr, scope map[string]*engineValue) (*engineValue, error) {
	var branches []*engineValue
	var defaults []*engineValue
	var walk func(e ast.Expr, defaulted bool) error
	walk = func(e ast.Expr, defaulted bool) error {
		if u, ok := e.(*ast.UnaryExpr); ok && u.Op == token.MUL {
			// A * mark applies to its whole operand, including a parenthesized
			// nested disjunction, so every disjunct underneath inherits it.
			return walk(u.X, true)
		}
		if p, ok := e.(*ast.ParenExpr); ok {
			return walk(p.X, defaulted)
		}
		if b, ok := e.(*ast.BinaryExpr); ok && b.Op == token.OR {
			if err := walk(b.X, defaulted); err != nil {
				return err
			}
			return walk(b.Y, defaulted)
		}
		br, err := evalChild(e, scope)
		if err != nil {
			return err
		}
		branches = append(branches, br)
		if defaulted {
			defaults = append(defaults, br)
		}
		return nil
	}
	if err := walk(n, false); err != nil {
		return nil, err
	}
	return buildDisjunction(branches, defaults), nil
}

// buildDisjunction reduces a freshly-walked set of disjuncts and defaults into
// a disjunction value, applying CUE's build-time reductions: drop ⊥ disjuncts,
// collapse equal concrete disjuncts, and surface an all-⊥ disjunction as a ⊥
// ("errors in empty disjunction"). A single surviving disjunct collapses to
// that value (with no disjunction wrapper). Defaults are filtered to the
// surviving branch set and likewise deduped.
func buildDisjunction(branches, defaults []*engineValue) *engineValue {
	live := dropBottomBranches(branches)
	if len(live) == 0 {
		// Every disjunct reduced to ⊥. CUE rejects this at compile time as an
		// empty disjunction rather than deferring to validate.
		return mkBottom(nil, "empty disjunction: every disjunct is bottom")
	}
	live = dedupeConcrete(live)
	keptDefaults := retainBranches(defaults, live)
	keptDefaults = dedupeConcrete(keptDefaults)
	if len(live) == 1 && len(keptDefaults) <= 1 {
		// A single surviving disjunct is just that value. (A surviving default
		// equal to it adds nothing.)
		return live[0]
	}
	return &engineValue{kind: kDisjoint, branches: live, defaults: keptDefaults}
}

// dropBottomBranches returns the non-⊥ entries of branches.
func dropBottomBranches(branches []*engineValue) []*engineValue {
	out := branches[:0:0]
	for _, br := range branches {
		if br.isBottomV() {
			continue
		}
		out = append(out, br)
	}
	return out
}

// retainBranches keeps only the entries of defaults that are still present
// (by pointer identity) in the surviving branch set, so a default whose branch
// was dropped as ⊥ does not haunt the result.
func retainBranches(defaults, live []*engineValue) []*engineValue {
	if len(defaults) == 0 {
		return nil
	}
	var out []*engineValue
	for _, d := range defaults {
		for _, b := range live {
			if d == b {
				out = append(out, d)
				break
			}
		}
	}
	return out
}

// evalList builds a list value in a scope, supporting comprehension elements
// the same way evalListElems does but preserving the open tail, so a scoped
// `[...string]` or `[if c {x}, ...]` resolves with its sibling references.
func evalList(n *ast.ListLit, scope map[string]*engineValue) (*engineValue, error) {
	out := &engineValue{kind: kList}
	for _, el := range n.Elts {
		switch e := el.(type) {
		case *ast.Ellipsis:
			out.openTop = true
			if e.Type != nil {
				et, err := evalChild(e.Type, scope)
				if err != nil {
					return nil, err
				}
				out.elem = et
			} else {
				out.elem = topValue()
			}
		case *ast.Comprehension:
			keep, body, err := evalComprehension(e, scope)
			if err != nil {
				return nil, err
			}
			if keep {
				out.prefix = append(out.prefix, body)
			}
		default:
			ev, err := evalChild(el, scope)
			if err != nil {
				return nil, err
			}
			out.prefix = append(out.prefix, ev)
		}
	}
	return out, nil
}

// evalStruct is the single struct-literal builder, threaded by scope. It
// resolves each field's value, unifying repeated keys, and folds an embedded
// value (a bound `{>=1 & <=10}`, a spread `{X, ...}`) into the struct. A `?`
// marks an optional key; a `...` ellipsis only documents openness (a struct
// is open by default unless close() wraps it).
//
// At compile time (scope == nil) the field values may defer to thunks, so
// the builder verifies every thunk references only a declared field — a
// reference to an undeclared name can never resolve, so it is a compile error
// here matching CUE's eager "reference X not found". During a struct force
// pass (scope != nil) the thunks are already resolved, so the check is a
// no-op the gate skips.
func evalStruct(n *ast.StructLit, scope map[string]*engineValue) (*engineValue, error) {
	out := &engineValue{kind: kStruct}
	var embedded *engineValue
	for _, d := range n.Elts {
		switch el := d.(type) {
		case *ast.Field:
			name, err := fieldLabel(el.Label)
			if err != nil {
				return nil, err
			}
			val, err := evalChild(el.Value, scope)
			if err != nil {
				return nil, err
			}
			out.fields = appendOrUnifyField(out.fields, field{
				name:     name,
				val:      val,
				optional: el.Constraint == token.OPTION,
			})
		case *ast.EmbedDecl:
			// An embedded value (a scalar bound or another struct spread in)
			// unifies with the struct. Defer the meet until the rest of the
			// struct is built so field order is preserved.
			ev, err := evalChild(el.Expr, scope)
			if err != nil {
				return nil, err
			}
			if embedded == nil {
				embedded = ev
			} else {
				embedded = unifyV(embedded, ev, nil)
			}
		case *ast.Ellipsis:
			continue
		default:
			return nil, fmt.Errorf("cuelite: unsupported struct element %T", d)
		}
	}
	if scope == nil {
		if err := checkThunkRefs(out); err != nil {
			return nil, err
		}
	}
	if embedded != nil {
		if scope == nil {
			if err := checkEmbeddedThunkRefs(out, embedded); err != nil {
				return nil, err
			}
		}
		if len(out.fields) == 0 {
			// A struct with only an embedded value IS that value: `{>=1 & <=10}`
			// is the bound, `{X}` is X. No struct wrapper survives.
			return embedded, nil
		}
		return unifyV(out, embedded, nil), nil
	}
	return out, nil
}

// deferToThunk wraps an expression whose evaluation hit an unresolved
// reference into a kThunk that re-evaluates against a later scope. Forcing
// the thunk (evalThunk) runs evalExpr again; if it still cannot resolve, the
// thunk yields a ⊥ naming the unresolved reference, so an unforced schema
// field never silently validates.
func deferToThunk(e ast.Expr) *engineValue {
	return &engineValue{
		kind: kThunk,
		thunkExpr: func(scope map[string]*engineValue) *engineValue {
			v, err := evalExpr(e, scope)
			if err != nil {
				return mkBottom(nil, "cuelite: unresolved expression %s", exprText(e))
			}
			return v
		},
		thunkRefs: freeRefs(e),
	}
}

// freeRefs collects the distinct non-keyword identifier names an expression
// references — the sibling fields a deferred thunk depends on. A type keyword
// (string, int, …) and the bool/null literals are not references. The
// compiler uses the result to reject a reference to a name that is not a
// declared field.
func freeRefs(e ast.Expr) []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		if isReferenceName(name) && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	var walk func(n ast.Node)
	walk = func(n ast.Node) {
		switch node := n.(type) {
		case nil:
			return
		case *ast.Ident:
			add(node.Name)
		case *ast.Field:
			// A field's LABEL is a key, not a reference; only its VALUE can carry
			// references. (A struct field title in a comprehension body is a key.)
			walk(node.Value)
			return
		case *ast.SelectorExpr:
			// Only the base of a selector is a reference; the selected name is a
			// member, not a sibling field.
			walk(node.X)
			return
		}
		ast.Walk(n, func(child ast.Node) bool {
			if child == n {
				return true
			}
			walk(child)
			return false
		}, nil)
	}
	walk(e)
	return out
}

// isReferenceName reports whether an identifier names a field reference, as
// opposed to a type keyword or a bool/null literal that compiles to a value.
func isReferenceName(name string) bool {
	switch name {
	case "_", "null", "true", "false", "string", "int", "float", "number", "bool", "bytes":
		return false
	}
	return true
}

// isTypeKeyword reports whether name is a CUE built-in TYPE keyword (one that
// builds a type atom, not a bool/null/top literal). Used to reject a bare
// type-keyword field LABEL: as a label it shadows the keyword for references
// in the field's value, a construct the in-house engine cannot model.
func isTypeKeyword(name string) bool {
	switch name {
	case "string", "int", "float", "number", "bool", "bytes":
		return true
	}
	return false
}
