package cuelite

import (
	stderrors "errors"
	"fmt"

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

// evalExpr evaluates an AST expression against a scope of resolved
// sibling-field values, the in-house evaluator for the index/comprehension/
// reference constructs the release-channels schema uses. A reference to a
// name absent from scope (or present but non-concrete) returns errUnresolved
// so the caller defers the whole expression. The plain constructs delegate
// to the same builders compileExpr uses; the scoped ones live here.
func evalExpr(e ast.Expr, scope map[string]*engineValue) (*engineValue, error) {
	switch n := e.(type) {
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
		return evalUnary(n, scope)
	default:
		// Constructs with no sibling references compile directly.
		return compileExpr(e)
	}
}

// evalIdent resolves a bare identifier: a type keyword compiles as usual,
// otherwise the name is a sibling-field reference looked up in scope. A name
// absent from scope, or bound to a non-concrete value, defers the enclosing
// expression (errUnresolved) — the comprehension condition cannot be decided
// until the reference is fixed by data.
func evalIdent(n *ast.Ident, scope map[string]*engineValue) (*engineValue, error) {
	switch n.Name {
	case "_", "null", "true", "false", "string", "int", "float", "number", "bool", "bytes":
		return compileIdent(n)
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
		return nil, fmt.Errorf("cuelite: list index must be an integer, got %s", idxVal.describe())
	}
	list, ok := n.X.(*ast.ListLit)
	if !ok {
		return nil, fmt.Errorf("cuelite: index target must be a list literal, got %T", n.X)
	}
	elems, err := evalListElems(list, scope)
	if err != nil {
		return nil, err
	}
	i := int(idxVal.i)
	if i < 0 || i >= len(elems) {
		return mkBottom(nil, "list index %d out of range (len %d)", i, len(elems)), nil
	}
	return elems[i], nil
}

// evalListElems builds the concrete element list of a list literal, applying
// comprehension clauses: a plain element contributes itself, an `if`
// comprehension contributes its body only when the condition holds. The
// ellipsis tail is ignored for indexing (an index past the prefix is out of
// range).
func evalListElems(list *ast.ListLit, scope map[string]*engineValue) ([]*engineValue, error) {
	var out []*engineValue
	for _, el := range list.Elts {
		switch e := el.(type) {
		case *ast.Ellipsis:
			// The open tail adds no indexable element.
			continue
		case *ast.Comprehension:
			keep, body, err := evalComprehension(e, scope)
			if err != nil {
				return nil, err
			}
			if keep {
				out = append(out, body)
			}
		default:
			ev, err := evalExpr(el, scope)
			if err != nil {
				return nil, err
			}
			out = append(out, ev)
		}
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
	cond, err := evalExpr(ifc.Condition, scope)
	if err != nil {
		return false, nil, err
	}
	if cond.kind != kBool {
		return false, nil, errUnresolved
	}
	body, ok := c.Value.(*ast.StructLit)
	if !ok {
		return false, nil, fmt.Errorf("cuelite: comprehension body must be a struct, got %T", c.Value)
	}
	bv, err := evalStruct(body, scope)
	if err != nil {
		return false, nil, err
	}
	return cond.b, bv, nil
}

// evalBinary evaluates a binary expression in a scope. An == or != between a
// resolved reference and a literal yields a concrete bool (driving an `if`
// comprehension); the lattice operators & and | and the relational bounds
// delegate to the compile-time builders after their operands resolve.
func evalBinary(n *ast.BinaryExpr, scope map[string]*engineValue) (*engineValue, error) {
	switch n.Op {
	case token.EQL, token.NEQ:
		return evalEquality(n, scope)
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
	case token.GEQ, token.LEQ, token.GTR, token.LSS, token.MAT:
		// A relational bound's operand is a literal, not a reference, so the
		// compile-time builder handles it without scope.
		return compileRelational(n)
	default:
		return nil, fmt.Errorf("cuelite: unsupported binary operator %q", n.Op)
	}
}

// evalEquality evaluates an == or != comparison to a concrete bool. Both
// sides must resolve to concrete scalars; an unresolved reference defers.
// This is the comparison an `if mechanism == "push"` comprehension needs.
func evalEquality(n *ast.BinaryExpr, scope map[string]*engineValue) (*engineValue, error) {
	l, err := evalExpr(n.X, scope)
	if err != nil {
		return nil, err
	}
	r, err := evalExpr(n.Y, scope)
	if err != nil {
		return nil, err
	}
	if !isConcrete(l) || !isConcrete(r) {
		return nil, errUnresolved
	}
	eq := concreteEqual(l, r)
	if n.Op == token.NEQ {
		eq = !eq
	}
	return &engineValue{kind: kBool, b: eq}, nil
}

// evalDisjunction builds a disjunction value in a scope, flattening nested |
// and recording a *-marked default branch, the scoped counterpart of
// compileDisjunction. A branch that defers leaves the whole disjunction
// deferred.
func evalDisjunction(n *ast.BinaryExpr, scope map[string]*engineValue) (*engineValue, error) {
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
		br, err := evalExpr(e, scope)
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

// evalUnary evaluates a unary expression in a scope: a bound operator, the *
// default marker, and numeric sign, delegating to the compile-time builder
// once the operand resolves.
func evalUnary(n *ast.UnaryExpr, scope map[string]*engineValue) (*engineValue, error) {
	switch n.Op {
	case token.MUL:
		return evalExpr(n.X, scope)
	default:
		return compileUnary(n)
	}
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
				et, err := evalExpr(e.Type, scope)
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
			ev, err := evalExpr(el, scope)
			if err != nil {
				return nil, err
			}
			out.prefix = append(out.prefix, ev)
		}
	}
	return out, nil
}

// evalStruct builds a struct value in a scope, resolving each field's value
// against it. It mirrors compileStruct but threads the scope so a nested
// reference resolves; the embedded-value and ellipsis handling match.
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
			val, err := evalExpr(el.Value, scope)
			if err != nil {
				return nil, err
			}
			out.fields = appendOrUnifyField(out.fields, field{
				name:     name,
				val:      val,
				optional: el.Constraint == token.OPTION,
			})
		case *ast.EmbedDecl:
			ev, err := evalExpr(el.Expr, scope)
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
	if embedded != nil {
		if len(out.fields) == 0 {
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
	}
}
