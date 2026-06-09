// Package cuelite is the public, versioned façade mdsmith uses for the
// small CUE subset it depends on — schema constraints, query filters,
// placeholder paths, and catalog templates. Like
// github.com/jeduden/mdsmith/pkg/markdown it is an exported public
// surface, imported under github.com/jeduden/mdsmith/cue/cuelite, and
// it sits at the bottom of the layering map: it imports no internal
// mdsmith package.
//
// # Strategy: CUE-backed façade, then in-house flip
//
// The package lands first as a thin wrapper over cuelang.org/go. Every
// method delegates to CUE, so mdsmith call sites can move onto the
// façade with behaviour unchanged. Only afterward is the implementation
// flipped, method by method, to a small in-house pure-Go engine behind
// this same stable API. Throughout that flip the CUE-backed path stays
// available as the differential oracle: the harness in difftest.go runs
// a value through both the in-house path and the CUE-backed path and
// asserts identical accept/reject outcomes and identical error field
// paths. Until a method is flipped, both paths are the same CUE-backed
// code, so the harness is a green scaffold the later phases extend.
//
// Phase 0 (plan 236) exposes only the minimal surface the delegation
// pattern, the differential harness, and the benchmark need: Compile,
// CompileJSON, and the Value methods Unify and Validate, plus the
// path-tagged PathError. The per-surface façade methods (ParsePath,
// LookupPath, String, Decode, Exists, Fields, …) arrive in the phases
// that migrate each call site.
package cuelite

import (
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
)

// sharedContext is the single *cue.Context every cuelite Value compiles
// against. A cue.Value is tied to its context and may not cross
// contexts, so unifying two values requires that they share one. The
// internal/schema call sites solve this today by threading a per-schema
// context through CompiledCUE; the façade hides that plumbing behind a
// process-wide context so any two cuelite Values unify directly. The
// context is internally synchronised, so concurrent compiles are safe.
//
// When the implementation is flipped to the in-house engine this
// variable disappears: a flipped Value is a context-free immutable
// struct shareable across goroutines, and the façade API is unchanged.
var sharedContext = cuecontext.New()

// Value is an immutable compiled CUE value behind the cuelite façade.
// In the phase-0 CUE-backed implementation it wraps the underlying
// cue.Value, compiled against the package-wide sharedContext so any two
// Values may be unified without re-binding. Once the implementation is
// flipped to the in-house engine a Value becomes a context-free struct;
// the façade API does not change.
type Value struct {
	val cue.Value
}

// Compile compiles a CUE source string into a Value. It is the façade
// over cue.Context.CompileString. A syntactically invalid source or a
// bottom value reports an error.
func Compile(src string) (*Value, error) {
	val := sharedContext.CompileString(src)
	if err := val.Err(); err != nil {
		return nil, err
	}
	return &Value{val: val}, nil
}

// CompileJSON compiles a JSON document into a Value. It is the façade
// over cue.Context.CompileBytes and is used to lift document data (for
// example marshalled front matter) into the value model so it can be
// unified against a schema. Invalid JSON reports an error.
func CompileJSON(data []byte) (*Value, error) {
	val := sharedContext.CompileBytes(data)
	if err := val.Err(); err != nil {
		return nil, err
	}
	return &Value{val: val}, nil
}

// Unify returns the meet of v and o — the value satisfying both. It is
// the façade over cue.Value.Unify. Both operands share the package-wide
// context, so the result is well-formed without any re-binding.
func (v *Value) Unify(o *Value) *Value {
	return &Value{val: v.val.Unify(o.val)}
}

// Validate reports whether the value is concrete and free of conflicts,
// mirroring cue.Value.Validate(cue.Concrete(true)). On failure it
// returns a *PathError tagged with the field path of the first
// offending leaf, so callers and the differential harness can compare
// error locations. A value that satisfies the schema returns nil.
func (v *Value) Validate() error {
	verr := v.val.Validate(cue.Concrete(true))
	if verr == nil {
		return nil
	}
	// errors.Errors returns a non-empty list for any non-nil CUE
	// validation error, so leaves[0] is always present here. The
	// internal/schema validator relies on the same invariant.
	first := errors.Errors(verr)[0]
	return NewPathError(first.Path(), first.Error())
}
