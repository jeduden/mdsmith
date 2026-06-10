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
// and [Value.String] / [Value.Decode] read a concrete leaf out.
// [Value.CompileMap] validates a map[string]any directly against a
// compiled schema, with no JSON marshal/parse round-trip — the
// front-matter hot path.
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
// Value, or a conflicting Unify. A bottom absorbs through [Value.Unify]
// and surfaces from [Value.Validate] as a single path-free [PathError],
// so an error flows through a Unify chain instead of panicking.
//
// # Concurrency and memory
//
// A [Value] is a context-free immutable struct: it owns no *cue.Context
// and no mutable state. A compiled schema is therefore safe for
// concurrent use and shareable across goroutines with no
// synchronization — the engine creates fresh value nodes on every
// Unify rather than mutating a shared one. Validating N documents
// against one cached schema costs no per-document recompile and no
// memory that grows with N: each [Value.CompileMap] or [Value.Unify]
// produces a new immutable result and the schema is read, never
// written. (The earlier CUE-backed implementation owned a per-Value
// *cue.Context and paid two interim costs — a context-mutating
// cross-context Unify and a context that grew per validated document —
// both of which the in-house engine erases.)
//
// # Stability
//
// As a public package this is a cross-system contract, like
// pkg/markdown and pkg/mdsmith. The evaluator — unify, validate,
// concreteness — is in-house; the AST frontend reuses cuelang's
// cue/parser to walk the supported CUE subset into the value model
// (the interim recorded in plan 238, removed in plan 240's phase 4). A
// differential harness pins identical accept/reject outcomes and
// identical sets of rejecting leaf paths (deduplicated) against a
// direct-CUE oracle across the whole corpus, plus a schema×data
// fuzzer. The strategy and the layering rules live in
// docs/development/architecture/index.md.
package cuelite
