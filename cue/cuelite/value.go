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
// this same stable API. Throughout that flip a differential oracle keeps
// the flip honest: the harness in internal/cuelitetest runs a value
// through both the in-house path (this façade) and an INDEPENDENT
// CUE-backed oracle it implements itself (internal/cuelitetest's
// OraclePath, talking to cuelang.org/go directly, not this façade), and
// asserts identical accept/reject outcomes and identical error field
// paths. This façade's own delegation to CUE is deleted at the flip
// (plan 240); the oracle is the harness's, not the façade's. Until a
// method is flipped, both paths reduce to CUE, so the harness is a green
// scaffold the later phases extend.
//
// Phase 0 (plan 236) exposes only the minimal surface the delegation
// pattern, the differential harness, and the benchmark need: Compile,
// CompileJSON, the Value methods Unify and Validate, the package-level
// Errors accessor, and the path-tagged PathError. The per-surface
// façade methods (ParsePath, LookupPath, String, Decode, Exists,
// Fields, …) arrive in the phases that migrate each call site.
//
// # Value isolation and the interim context cost
//
// A cue.Value is tied to the *cue.Context it was built in, and CUE
// v0.16.1 documents that values created from the same Context are not
// safe for concurrent use, and that long-lived contexts can grow
// unbounded. So each Compile/CompileJSON builds its own *cue.Context.
// Independently compiled roots are therefore isolated from one another.
//
// Unify needs both operands in one context, and it gets there by
// REBUILDING one side into the other's context. Whichever side still
// retains source is the side rebuilt; the result then lives in — and is
// NOT isolated from — the context of the side that was NOT rebuilt. Unify
// tries the operand first (rebuild the operand into the receiver's
// context, the common fresh-operand-against-a-root case) and otherwise
// rebuilds the receiver into the operand's context. When BOTH sides are
// derived results with no source, neither can be rebuilt and Unify
// returns errCrossContext as a bottom.
//
// Two consequences follow for concurrent use, and they are the teachable
// safe rule:
//
//   - A source-carrying Value used as the NON-rebuilt side has its context
//     MUTATED by each cross-context Unify (the rebuilt operand is compiled
//     INTO that context). So a compiled schema shared across goroutines —
//     used as the receiver in schemaVal.Unify(dataVal) — must have external
//     synchronization, or each goroutine must compile its own copy. Sharing
//     one compiled Value across goroutines without locking is a data race
//     under the same CUE v0.16.1 disclaimer.
//   - Repeated cross-context Unify against ONE long-lived Value accumulates
//     a compiled document in its context per call — CUE's documented
//     long-lived-context growth. A benchmark that validates many documents
//     against one cached schema therefore pays an N-dependent per-op cost.
//
// Both are interim costs of the CUE-backed phase. The in-house engine of
// plan 218 erases them — a flipped Value is a context-free immutable
// struct, shareable across goroutines and free of context growth —
// without changing this API: Value stays a value type whose Unify takes
// and returns a Value, so a bottom (⊥) absorbs cleanly in either
// implementation.
package cuelite

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"
	cuejson "cuelang.org/go/encoding/json"
)

// Value is an immutable compiled CUE value behind the cuelite façade.
// It is a value type: methods take and return Value by copy, so a
// zero/bottom Value composes without a nil receiver. A Value compiled
// by Compile or CompileJSON carries the compiled cue.Value and its
// original source, so Unify can rebuild it inside another Value's
// context when the two come from different roots. A Value carrying err
// is a bottom (⊥): Unify with it yields a bottom, and Validate returns
// the carried error. The zero Value (no compiled cue.Value, no source,
// no error) is also a bottom: Validate reports it and Unify absorbs it,
// so a caller that forgot to compile never triggers a nil-context
// panic. Once the implementation is flipped to the in-house engine a
// Value becomes a context-free struct; this API does not change.
type Value struct {
	val cue.Value
	// src is the retained source a cross-context Unify rebuilds the Value
	// from. Compile stores its string argument directly (strings are
	// immutable, so no copy); CompileJSON's one string(data) conversion
	// doubles as the defensive copy of the caller's byte slice. hasSrc
	// distinguishes a Value that retains source (any Compile/CompileJSON
	// result, even of an empty document) from a derived Unify result that
	// has none — so a literally empty source is not mistaken for derived.
	src    string
	hasSrc bool
	err    error
}

// errZeroValue is the bottom reason for a zero Value — one that was
// never compiled, so it has neither a usable cue.Value nor source to
// rebuild from. Unify absorbs it and Validate returns it.
var errZeroValue = stderrors.New("uninitialized cuelite.Value")

// bottom builds an error-carrying Value. Unify treats it as ⊥
// (absorbing), and Validate returns its error, so a compile failure or
// a bottom operand propagates through a Unify chain without panicking.
func bottom(err error) Value {
	return Value{err: err}
}

// isBottom reports whether v is a bottom: either it carries an explicit
// error, or it is the zero Value (never compiled, so its cue.Value does
// not exist). It returns the reason to surface so the bottom propagates
// with a message rather than a nil-context panic.
func (v Value) isBottom() (error, bool) {
	if v.err != nil {
		return v.err, true
	}
	if !v.val.Exists() {
		return errZeroValue, true
	}
	return nil, false
}

// Compile compiles a CUE source string into a Value. It is the façade
// over cue.Context.CompileString. A syntactically invalid source or a
// bottom value reports an error. The returned Value owns a fresh
// *cue.Context.
func Compile(src string) (Value, error) {
	ctx := cuecontext.New()
	val := ctx.CompileString(src)
	if err := val.Err(); err != nil {
		return bottom(err), err
	}
	return Value{val: val, src: src, hasSrc: true}, nil
}

// CompileJSON compiles a JSON document into a Value. It is the façade
// over the JSON-data lift mdsmith uses to validate marshalled front
// matter against a schema. The input must be strict JSON: arbitrary
// CUE source (an unquoted key, an expression) is rejected, unlike a
// raw CompileBytes.
//
// Duplicate object keys are rejected outright, naming the offending
// key. This is the strict-JSON no-duplicate-keys contract, and it is
// the rationale the scanner and buildJSON cross-reference: CUE's JSON
// lift instead UNIFIES same-named keys (mergeable or equal duplicates
// compile to a phantom merged object, only conflicting ones error),
// whereas real JSON consumers are last-wins, so cuelite enforces the
// contract before the CUE lift rather than validating a value no JSON
// reader would produce.
//
// Duplicate detection is best-effort on two edge inputs that defer to
// the CUE lift's own UTF-8 handling: a document that is not valid
// UTF-8, and a decoded key that contains U+FFFD (a lone-surrogate
// escape such as "\ud800", or a literal "�"). Such keys forgo
// scanner-level dup detection; a lone-surrogate key still errors at the
// build (Extract or BuildExpr reports "invalid string: unmatched
// surrogate pair"), and a literal-"�" duplicate is the only key shape
// that escapes both checks — accepted as a phantom merge, which no
// front-matter marshaller produces.
//
// The returned Value owns a fresh *cue.Context.
func CompileJSON(data []byte) (Value, error) {
	ctx := cuecontext.New()
	val, err := buildJSON(ctx, data)
	if err != nil {
		return bottom(err), err
	}
	// string(data) copies the caller's bytes, so the retained source is
	// immutable even if the caller later mutates data; strict JSON is a
	// syntactic subset of CUE, so rebuild can recompile it with
	// CompileString just like a CUE source.
	return Value{val: val, src: string(data), hasSrc: true}, nil
}

// buildJSON parses strict JSON into a cue.Value inside ctx. It rejects
// any input that is not valid JSON (so CUE source cannot slip through):
// Extract reports a malformed document before any value is built. It
// first rejects any duplicate object key (at any nesting depth,
// including inside array elements) — see CompileJSON for why strict JSON
// forbids them. The rejection sources — a duplicate key, a malformed
// document, and a build-time bottom — are the only errors CompileJSON
// surfaces.
func buildJSON(ctx *cue.Context, data []byte) (cue.Value, error) {
	if err := scanDuplicateJSONKeys(data); err != nil {
		return cue.Value{}, err
	}
	expr, err := cuejson.Extract("", data)
	if err != nil {
		return cue.Value{}, err
	}
	// BuildExpr can still bottom on a grammar-valid string that is not a
	// valid Unicode value — a lone-surrogate escape such as "\ud800"
	// passes the scanner and Extract but builds to ⊥ ("invalid string:
	// unmatched surrogate pair"). Surface it as a compile error so the
	// data arm classifies it at the data stage, not as a phantom
	// validation pass.
	val := ctx.BuildExpr(expr)
	if err := val.Err(); err != nil {
		return cue.Value{}, err
	}
	return val, nil
}

// scanDuplicateJSONKeys walks the JSON document with the streaming token
// decoder and reports the first object key that appears twice within the
// same object — at any nesting depth, including objects that are array
// elements. See CompileJSON for why strict JSON forbids duplicates.
//
// It keys off json.Decoder's structural guarantee: between a '{' and its
// matching '}', tokens alternate key, value, key, value, …, where each
// value is a single scalar token or a whole nested container (which the
// decoder emits as one '{'/'[' delimiter that we recurse on). So inside
// an object level, the next string token after an even number of values
// is a key; we track that parity per open object with seenKey, flipping
// it once per value. Array levels carry no key rule, so their elements
// are scanned only to recurse into nested objects.
//
// Four inputs make the scan defer rather than fabricate a duplicate:
//
//   - A malformed document yields no duplicate error: cuejson.Extract is
//     left to report the syntax error, so the two arms keep one place that
//     decides what "not JSON" means. dec.UseNumber() keeps a number
//     outside float64 range (1e999, valid JSON) from erroring mid-scan and
//     being misread as malformed, which would let a duplicate beside it
//     slip past.
//   - A second top-level value: the scan stops once the first top-level
//     value closes (the stack empties, or the first top-level scalar is
//     consumed). Any trailing data is "invalid JSON after top-level
//     value", which cuejson.Extract reports.
//   - Invalid UTF-8 input defers to Extract (a utf8.Valid pre-check):
//     json.Decoder replaces invalid bytes in a raw key with U+FFFD, so two
//     distinct invalid-byte keys would collapse to one fabricated
//     duplicate.
//   - A decoded key containing U+FFFD (a lone-surrogate escape such as
//     "\ud800", or a literal "�") is skipped for dup tracking: two
//     lone-surrogate keys decode to the same U+FFFD and are not duplicates
//     of each other. A lone-surrogate key still errors at the build (see
//     buildJSON); a literal-"�" key is the documented gap CompileJSON
//     describes.
//
// jsonLevel is one open container in scanDuplicateJSONKeys' stack. keys
// is non-nil for an object level and holds the keys already seen at it;
// nil marks an array level. seenKey is true when the next string token
// at an object level is a value (the key was already read), false when
// it is the next key.
type jsonLevel struct {
	keys    map[string]struct{}
	seenKey bool
}

// recordKey handles tok when it is the key half of a key/value pair at
// this object level, returning handled=true so the caller advances. It
// reports an error on the first duplicate key. tok is NOT a key — so
// handled is false and the caller treats it as a value — when l is nil
// (top level), an array level (l.keys == nil), already past the key
// (l.seenKey), or not a string. A key whose decode produced U+FFFD (a
// lone-surrogate escape or a literal "�") is consumed but skipped for
// dup tracking, since two such keys cannot be told apart.
func (l *jsonLevel) recordKey(tok any) (bool, error) {
	if l == nil || l.keys == nil || l.seenKey {
		return false, nil
	}
	s, ok := tok.(string)
	if !ok {
		return false, nil
	}
	l.seenKey = true
	if strings.ContainsRune(s, utf8.RuneError) {
		return true, nil
	}
	if _, dup := l.keys[s]; dup {
		return true, fmt.Errorf("duplicate JSON key %q", s)
	}
	l.keys[s] = struct{}{}
	return true, nil
}

func scanDuplicateJSONKeys(data []byte) error {
	// Invalid UTF-8 would make the decoder fold distinct raw keys onto one
	// U+FFFD; leave such input to cuejson.Extract.
	if !utf8.Valid(data) {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var stack []*jsonLevel
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			// Malformed JSON: leave the syntax error to cuejson.Extract.
			return nil
		}
		var cur *jsonLevel
		if len(stack) > 0 {
			cur = stack[len(stack)-1]
		}
		// At an object level, a string token is a key unless it is the value
		// half of a key/value pair. recordKey handles it (and the duplicate
		// check) when so; otherwise the token is a value, handled below.
		if handled, err := cur.recordKey(tok); err != nil {
			return err
		} else if handled {
			continue
		}
		switch tok {
		case json.Delim('{'):
			// A nested object is the value half of cur's pair; push it and
			// leave cur.seenKey set, so its matching '}' restores cur below.
			stack = append(stack, &jsonLevel{keys: map[string]struct{}{}})
			continue
		case json.Delim('['):
			stack = append(stack, &jsonLevel{})
			continue
		case json.Delim('}'), json.Delim(']'):
			// Close the container, then mark it consumed as a value in its
			// parent (now the top of the stack) so the parent expects its
			// next key.
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				// The first top-level value just closed. Stop scanning: any
				// trailing data is "invalid JSON after top-level value", which
				// cuejson.Extract reports — fabricating a duplicate from a
				// second top-level value would mask that.
				return nil
			}
			parent := stack[len(stack)-1]
			if parent.keys != nil {
				parent.seenKey = false
			}
			continue
		}
		// A scalar value was the value half of cur's pair: flip cur back to
		// expecting its next key. A top-level scalar (no open container) is
		// the whole first value; stop so any trailing data defers to Extract.
		if cur == nil {
			return nil
		}
		if cur.keys != nil {
			cur.seenKey = false
		}
	}
}

// rebuild returns o as a cue.Value living in ctx, so an operand can be
// unified inside another Value's context. It is general for every
// Value. When o's compiled value already belongs to ctx — a Unify
// result re-unified against its own root — it is returned directly,
// with no recompile. A cross-context operand re-compiles from its
// retained source; strict JSON is a syntactic subset of CUE, so one
// CompileString path rebuilds either a CUE or a JSON source. A Value
// with neither a value in ctx nor source to rebuild from (a derived
// Unify result) cannot be reconstructed, so ok is false; Unify then
// tries rebuilding the OTHER side and only absorbs as bottom when both
// sides are sourceless — never resolving to top.
func (o Value) rebuild(ctx *cue.Context) (cue.Value, bool) {
	if o.val.Context() == ctx {
		return o.val, true
	}
	if o.hasSrc {
		return ctx.CompileString(o.src), true
	}
	return cue.Value{}, false
}

// errCrossContext is the bottom reason when neither operand of a Unify
// retains source: both are derived results compiled in different
// contexts, so neither can be rebuilt into the other and CUE cannot
// unify values across contexts. The workaround is to keep at least one
// operand a Compile/CompileJSON result (which retains its source) so it
// can be rebuilt into the other's context.
var errCrossContext = stderrors.New(
	"cannot unify two derived values from different contexts; " +
		"unify a source-retaining Compile/CompileJSON value instead")

// Unify returns the meet of v and o — the value satisfying both. It is
// the façade over cue.Value.Unify. A bottom (⊥) operand absorbs: if
// either v or o is a bottom (an error-carrying or zero Value), the
// result carries that bottom, so a compile failure or an uninitialized
// operand flows through a Unify chain instead of panicking.
//
// Unification needs both values in one context. Unify rebuilds WHICHEVER
// side has retained source into the other's context: it first tries the
// operand in the receiver's context (the common case — a fresh operand
// against a root), and otherwise rebuilds the receiver into the operand's
// context (the operand is a derived result but the receiver still carries
// source). So order does not matter as long as at least one side retains
// source. Only when NEITHER side has source — both are derived results in
// different contexts — does Unify absorb as a bottom rather than vanishing
// into an empty struct that would silently drop every merged constraint.
func (v Value) Unify(o Value) Value {
	if err, ok := v.isBottom(); ok {
		return bottom(err)
	}
	if err, ok := o.isBottom(); ok {
		return bottom(err)
	}
	if rebuilt, ok := o.rebuild(v.val.Context()); ok {
		// The operand was rebuilt into v's context, so the merged value lives
		// in v's context — and is not isolated from v. The result retains no
		// source: a later cross-context Unify against it rebuilds the OTHER
		// side instead (the receiver-rebuild branch below handles that), and
		// only two such sourceless results in different contexts absorb as a
		// bottom. Re-Unify of a derived result IS exercised — by
		// TestValue_Unify_crossContext and the chained corpus cases — so this
		// is a real branch, not an unreachable convenience.
		return Value{val: v.val.Unify(rebuilt)}
	}
	// The operand is derived in a foreign context. If the receiver still
	// carries source, rebuild IT into the operand's context and unify there.
	if rebuilt, ok := v.rebuild(o.val.Context()); ok {
		return Value{val: rebuilt.Unify(o.val)}
	}
	return bottom(errCrossContext)
}

// Validate reports whether the value is concrete and free of conflicts,
// mirroring cue.Value.Validate(cue.Concrete(true)). A value that
// satisfies the schema returns nil. On any rejection it upholds one
// invariant the whole consumer chain depends on:
//
//	Validate() != nil  ⇒  len(Errors(Validate())) ≥ 1
//
// — so a loop over Errors (the internal/schema validator emitting one
// MDS020 diagnostic per failing field, the differential harness comparing
// every rejected path) always emits at least one diagnostic for a failing
// value and never silently accepts it. Two failure shapes feed that
// invariant:
//
//   - A schema/data conflict returns one *PathError per offending leaf,
//     each tagged with its field path, so callers see every rejection and
//     not only the first.
//   - A bottom Value — an error-carrying or zero Value, including a
//     replayed compile error or a cross-context derived bottom — returns a
//     single *PathError with an empty path carrying the bottom's message,
//     rather than a bare Go error that Errors would flatten to nil.
//
// The concrete shape of a multi-leaf result is unspecified: enumerate the
// per-field failures with the package-level Errors accessor (or
// errors.As), never by hand-traversing the join.
func (v Value) Validate() error {
	if err, ok := v.isBottom(); ok {
		// A bottom's message is path-free: tag it with an empty path so a
		// consumer loop over Errors still sees one diagnostic for the
		// failure. Wrap the bottom's cause so errors.Is/As still reach the
		// original CUE error or sentinel (errZeroValue, errCrossContext).
		return newPathError(nil, err.Error(), err)
	}
	verr := v.val.Validate(cue.Concrete(true))
	if verr == nil {
		return nil
	}
	return joinValidationErrors(errors.Errors(verr), verr)
}

// joinValidationErrors maps a CUE validation error's per-leaf decomposition
// into the *PathError shape Validate returns. Each leaf becomes a
// path-tagged *PathError; a single leaf is returned bare, several are
// joined with errors.Join. If the decomposition is empty — which a future
// CUE version or a malformed error could produce — it falls back to a
// path-free *PathError wrapping verr's own message, so Validate never
// returns a non-nil error that Errors flattens to nil (the fail-open
// hazard: stderrors.Join of nothing is nil, which a caller reads as
// acceptance). The fallback is covered by a package-internal unit test
// that hands this helper an empty leaf slice.
func joinValidationErrors(leaves []errors.Error, verr error) error {
	if len(leaves) == 0 {
		return newPathError(nil, verr.Error(), verr)
	}
	joined := make([]error, len(leaves))
	for i, leaf := range leaves {
		joined[i] = pathErrorOf(leaf)
	}
	if len(joined) == 1 {
		return joined[0]
	}
	return stderrors.Join(joined...)
}

// pathErrorOf converts a single CUE error into a *PathError. It uses
// the CUE error's path-free message (Msg, not Error, which prepends the
// dotted path) so PathError.Error() prints the path exactly once. It
// wraps the CUE error so errors.Is/As still reach it (the original
// cue/errors.Error) through the returned leaf.
func pathErrorOf(e errors.Error) *PathError {
	format, args := e.Msg()
	return newPathError(e.Path(), fmt.Sprintf(format, args...), e)
}
