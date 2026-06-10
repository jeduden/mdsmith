// Package cuelite compiles and validates the small CUE subset mdsmith
// depends on: schema constraints, query filters, placeholder paths,
// and catalog templates. It imports no internal mdsmith package, so it
// is usable on its own.
//
// [Compile] turns CUE source into a [Value]. [CompileJSON] lifts a
// strict-JSON document (marshalled front matter, in mdsmith's case)
// into the same type. [Value.Unify] merges two values, typically a
// schema and its data. [Value.Validate] reports whether the merged
// value is concrete and free of conflicts. [Errors] decomposes a
// Validate error into one [PathError] per failing field, each tagged
// with the path of the field that failed.
//
// [ParsePath] parses the string-label subset of CUE paths — dotted,
// quoted, bracket, raw-string, and multiline-string labels — into a
// [Path]; index, definition, and hidden selectors are rejected with an
// error naming the kind. [MakePath] constructs a Path directly from segments, and
// deliberately accepts segments ParsePath cannot parse (data keys
// like "true" or "a.b"), mirroring cue.MakePath.
//
// [Value.LookupPath] reads the value at a [Path], [Value.Fields]
// enumerates a struct's members (whose selectors feed straight into
// [MakePath]), [Value.Exists] tells a resolved path from an absent one,
// and [Value.String] / [Value.Decode] read a concrete leaf out. A value
// derived by LookupPath or Fields keeps REBUILDABLE PROVENANCE — its
// root source plus the path that reached it — so a [Value.Unify] across
// contexts reconstructs it instead of pinning a context-bound value,
// which lets a section lookup against a cached schema cross contexts
// without mutating the shared value.
//
// # Error model
//
// Validate upholds one invariant:
//
//	Validate() != nil  ⇒  len(Errors(Validate())) ≥ 1
//
// Every non-nil error decomposes into at least one [PathError], so a
// loop over [Errors] always emits at least one diagnostic for a
// failing value. The concrete shape of a multi-field error is
// unspecified; enumerate it with Errors, not by type assertion.
//
// A [Value] can also be a bottom (⊥): a compile failure, the zero
// Value, or a Unify of two derived values from different contexts. A
// bottom absorbs through [Value.Unify] and surfaces from
// [Value.Validate] as a single path-free [PathError], so an error
// flows through a Unify chain instead of panicking.
//
// # Concurrency and memory
//
// Compile, Unify, and Validate delegate to cuelang.org/go; ParsePath
// is already in-house (a pure-Go parser checked against cue.ParsePath
// by a differential corpus and fuzzer). CUE v0.16.1
// documents that values from one *cue.Context are not safe for
// concurrent use and that a long-lived context grows without bound.
// Each [Compile] and [CompileJSON] result therefore owns a fresh
// context, and two costs show through the API:
//
//   - A cross-context [Value.Unify] compiles the rebuilt operand into
//     the context of the side that is not rebuilt, mutating it. A
//     compiled schema shared across goroutines needs external
//     synchronization, or one compiled copy per goroutine.
//   - Each cross-context Unify against one long-lived Value adds one
//     compiled document to that Value's context. Validating N documents
//     against one cached schema costs memory proportional to N.
//
// # Stability
//
// As a public package this is a cross-system contract, like
// pkg/markdown and pkg/mdsmith. The CUE delegation is an interim
// implementation: it is being replaced, method by method, by an
// in-house engine behind this same API. A differential harness pins
// identical accept/reject outcomes and identical error field paths
// across that swap, and the swap removes the two context costs above
// without changing the API. The strategy and the layering rules live
// in docs/development/architecture/index.md.
package cuelite
