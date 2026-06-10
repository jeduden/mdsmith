package cuelite

import (
	stderrors "errors"
)

// Value is an immutable compiled value in the in-house CUE-subset engine.
// It is a value type: methods take and return Value by copy, so a zero or
// bottom Value composes without a nil receiver. A Value is context-free —
// it owns no *cue.Context — so it is safe for concurrent use and shareable
// across goroutines without synchronization, and a cached schema validates
// many documents with no per-document recompile.
//
// A Value carrying an error is a bottom (⊥): Unify with it yields a
// bottom, and [Value.Validate] returns the carried error. The zero Value
// (no compiled value, no error) is also a bottom: Validate reports it and
// Unify absorbs it, so a caller that forgot to compile gets an error
// instead of a nil-pointer panic.
type Value struct {
	v   *engineValue
	err error
}

// errZeroValue is the bottom reason for a zero Value — one that was never
// compiled. Unify absorbs it and Validate returns it.
var errZeroValue = stderrors.New("uninitialized cuelite.Value")

// bottom builds an error-carrying Value. Unify treats it as ⊥ (absorbing),
// and Validate returns its error, so a compile failure or a bottom operand
// propagates through a Unify chain without panicking.
func bottom(err error) Value {
	return Value{err: err}
}

// isBottom reports whether v is a bottom: it carries an explicit error, has
// no engine value, or wraps a ⊥ engine value. It returns the reason to
// surface so the bottom propagates with a message rather than a nil
// dereference.
func (v Value) isBottom() (error, bool) {
	if v.err != nil {
		return v.err, true
	}
	if v.v == nil {
		return errZeroValue, true
	}
	if v.v.isBottomV() {
		return newPathError(v.v.path, v.v.reason, nil), true
	}
	return nil, false
}

// Compile compiles a CUE-subset source string into a [Value], using
// cuelang's parser as a syntax frontend and the in-house engine as the
// evaluator (plan 238). A syntactically invalid source, an unsupported
// construct, or a value that reduces to ⊥ (a contradiction such as
// `int & string`) reports an error.
func Compile(src string) (Value, error) {
	ev, err := compileSource(src)
	if err != nil {
		return bottom(err), err
	}
	return Value{v: ev}, nil
}

// CompileJSON compiles a strict-JSON document into a [Value] using the
// in-house JSON lifter. The input must be strict JSON: an unquoted key or a
// CUE expression is rejected.
//
// A duplicate object key is rejected outright, naming the offending key.
// This is the durable strict-JSON contract (plan 238): a JSON reader is
// last-wins, so a same-named key pair has no single meaning; cuelite
// rejects it before lifting rather than silently merging. Duplicate
// detection is best-effort on two edge inputs that defer to the lifter's
// UTF-8 handling: a document that is not valid UTF-8, and a decoded key
// containing U+FFFD (a lone-surrogate escape or a literal "�").
func CompileJSON(data []byte) (Value, error) {
	ev, err := liftJSON(data)
	if err != nil {
		return bottom(err), err
	}
	return Value{v: ev}, nil
}

// CompileMap validates a map[string]any directly against this Value (a
// compiled schema), with no JSON marshal/parse round-trip — the hot path
// plan 218 mandates. It lifts the map into a concrete value and unifies it
// with the receiver, returning the merged Value whose Validate reports
// every failing leaf. A lift failure (an unsupported front-matter value
// type) yields a bottom Value.
func (v Value) CompileMap(m map[string]any) Value {
	if err, ok := v.isBottom(); ok {
		return bottom(err)
	}
	ev, err := liftMapValue(m)
	if err != nil {
		return bottom(err)
	}
	return Value{v: unifyV(ev, v.v, nil)}
}

// LiftMap lifts a map[string]any directly into a concrete data [Value],
// with no JSON marshal/parse round-trip. It is the standalone lift
// CompileMap unifies with a schema, exposed so a caller (query) can read
// the data back — for example to confirm a required field is PRESENT in the
// data before unifying, since an open schema would otherwise fill a missing
// field. An unsupported front-matter value type yields a bottom Value.
func LiftMap(m map[string]any) Value {
	ev, err := liftMapValue(m)
	if err != nil {
		return bottom(err)
	}
	return Value{v: ev}
}

// Unify returns the meet of v and o: the value satisfying both, the
// lattice meet over the in-house value model. A bottom (⊥) operand
// absorbs: if either v or o is a bottom (an error-carrying or zero Value),
// the result carries that bottom, so a compile failure or an uninitialized
// operand flows through a Unify chain instead of panicking.
//
// Because a Value is context-free, Unify needs no rebuild and no operand
// order: v.Unify(o) and o.Unify(v) produce the same value, and a shared
// schema can be the receiver or the operand without synchronization.
func (v Value) Unify(o Value) Value {
	if err, ok := v.isBottom(); ok {
		return bottom(err)
	}
	if err, ok := o.isBottom(); ok {
		return bottom(err)
	}
	return Value{v: unifyV(v.v, o.v, nil)}
}

// Validate reports whether the value is concrete and free of conflicts,
// mirroring cue.Value.Validate(cue.Concrete(true)). A value that satisfies
// the schema returns nil. On any rejection it upholds the invariant every
// consumer depends on:
//
//	Validate() != nil  ⇒  len(Errors(Validate())) ≥ 1
//
// A loop over [Errors] therefore always emits at least one diagnostic for a
// failing value and never silently accepts it. Two failure shapes feed that
// invariant:
//
//   - A schema/data conflict or an incomplete value returns one [*PathError]
//     per offending leaf, each tagged with its field path, so callers see
//     every rejection and not only the first.
//   - A bottom Value — an error-carrying or zero Value, including a replayed
//     compile error — returns a single [*PathError] with an empty path
//     carrying the bottom's message, rather than a bare Go error that Errors
//     would flatten to nil.
//
// The concrete shape of a multi-leaf result is unspecified: enumerate the
// per-field failures with the package-level [Errors] accessor (or
// errors.As), never by hand-traversing the join.
func (v Value) Validate() error {
	if err, ok := v.isBottom(); ok {
		// A zero or error-carrying Value: tag the reason with an empty path so
		// a consumer loop over Errors still sees one diagnostic. Wrap the cause
		// so errors.Is/As reaches the sentinel (errZeroValue) or compile error.
		var pe *PathError
		if stderrors.As(err, &pe) {
			return pe
		}
		return newPathError(nil, err.Error(), err)
	}
	leaves := validateEngine(v.v)
	return joinLeaves(leaves)
}

// joinLeaves maps the engine's per-leaf decomposition into the *PathError
// shape Validate returns: a single leaf is returned bare, several are
// joined with errors.Join, and an empty decomposition is nil (the value is
// concrete). This is the in-house equivalent of the former
// joinValidationErrors; the fail-open guard moved to Validate's bottom
// branch, which always produces one leaf for a non-nil Go error.
func joinLeaves(leaves []*PathError) error {
	switch len(leaves) {
	case 0:
		return nil
	case 1:
		return leaves[0]
	default:
		joined := make([]error, len(leaves))
		for i, leaf := range leaves {
			joined[i] = leaf
		}
		return stderrors.Join(joined...)
	}
}
