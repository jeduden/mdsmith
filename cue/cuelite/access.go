package cuelite

import (
	"cuelang.org/go/cue"
)

// Exists reports whether the Value names something concrete enough to
// read — it is the façade over cue.Value.Exists. A bottom (a zero or
// error-carrying Value) does not exist; a compiled value, or a
// successful [Value.LookupPath] result, does. A consumer uses it to
// tell "the path resolved" from "the path was absent" without inspecting
// the value further.
func (v Value) Exists() bool {
	if _, ok := v.isBottom(); ok {
		return false
	}
	return v.val.Exists()
}

// cuePath builds a cue.Path from a [Path]'s raw segments. Each segment
// is wrapped with cue.Str so a data key needing quotes (a dotted or
// hyphenated map key) is looked up verbatim, never reparsed — mirroring
// cue.MakePath, the constructor MakePath itself is modelled on. An empty
// Path produces the empty cue.Path, which selects the value unchanged.
func cuePath(p Path) cue.Path {
	sels := make([]cue.Selector, len(p.segments))
	for i, seg := range p.segments {
		sels[i] = cue.Str(seg)
	}
	return cue.MakePath(sels...)
}

// Err reports whether the Value is a bottom — a compile failure, a zero
// Value, or a value reduced to bottom by a conflicting Unify — or nil
// when it names a value. It is the façade over cue.Value.Err, and unlike
// [Value.Validate] it does NOT check concreteness: a successfully
// compiled but non-concrete schema (a constraint awaiting data) has no
// Err. A caller uses it to tell a broken schema from one merely waiting
// on its document.
func (v Value) Err() error {
	if err, ok := v.isBottom(); ok {
		return err
	}
	return v.val.Err()
}

// LookupPath returns the Value at p within v, and whether it exists. It
// is the façade over cue.Value.LookupPath. A missing leaf, or a lookup
// against a bottom, returns ok=false; an empty path returns the receiver.
//
// The result keeps REBUILDABLE PROVENANCE: rather than pin the derived
// cue.Value to v's context (which would race a shared cached schema, or
// forfeit caching by recompiling per lookup), the result retains v's
// root source plus p. A later cross-context [Value.Unify] then rebuilds
// the result by recompiling the root and re-applying the lookup in the
// target context — so a section-level lookup against a cached schema
// crosses contexts without mutating the shared value. A lookup against a
// value with no retained source (a derived Unify result) keeps no
// provenance and stays usable only in its own context.
func (v Value) LookupPath(p Path) (Value, bool) {
	if _, ok := v.isBottom(); ok {
		return bottom(errZeroValue), false
	}
	leaf := v.val.LookupPath(cuePath(p))
	if !leaf.Exists() {
		return Value{val: leaf}, false
	}
	out := Value{val: leaf}
	if v.hasSrc {
		// Retain rebuildable provenance: the root source plus this path, so
		// rebuild can recompile the root in another context and re-apply the
		// lookup. The derived leaf itself cannot cross contexts. hasSrc lets
		// Unify's rebuild path fire; hasLookup routes it through the
		// path-replay branch rather than a plain source recompile.
		out.hasSrc = true
		out.lookupRoot = v.rootSrc()
		out.lookupPath = p.segments
		out.hasLookup = true
	}
	return out, true
}

// rootSrc returns the source that, recompiled, reproduces this Value's
// own root. A Value compiled directly carries its source in src; a
// lookup result carries the source of the root it was looked up in.
// Only a Value that retains source (hasSrc) reaches here.
func (v Value) rootSrc() string {
	if v.hasLookup {
		return v.lookupRoot
	}
	return v.src
}

// Field is one member of a struct Value: its label (the unquoted
// selector string, usable directly with [MakePath]) and its Value.
type Field struct {
	Selector string
	Value    Value
}

// Fields returns the members of a struct Value in definition order, or
// nil for a non-struct or bottom Value. It is the façade over
// cue.Value.Fields. Each [Field.Selector] is the raw label string, so a
// consumer building a path from it must use [MakePath] (not [ParsePath]),
// which stores a dotted or hyphenated key verbatim. Each [Field.Value]
// retains rebuildable provenance the same way [Value.LookupPath] does,
// so a field can be re-looked-up or unified across contexts.
func (v Value) Fields() []Field {
	if _, ok := v.isBottom(); ok {
		return nil
	}
	iter, err := v.val.Fields()
	if err != nil {
		return nil
	}
	var out []Field
	for iter.Next() {
		sel := iter.Selector().Unquoted()
		child := Value{val: iter.Value()}
		if v.hasSrc {
			child.hasSrc = true
			child.lookupRoot = v.rootSrc()
			child.lookupPath = append(append([]string{}, v.lookupPathPrefix()...), sel)
			child.hasLookup = true
		}
		out = append(out, Field{Selector: sel, Value: child})
	}
	return out
}

// lookupPathPrefix returns the path segments that lead to this Value
// from its root, so Fields can extend it by one selector per child. A
// directly-compiled Value has no prefix (its root IS itself); a lookup
// result carries the path it was looked up at.
func (v Value) lookupPathPrefix() []string {
	if v.hasLookup {
		return v.lookupPath
	}
	return nil
}

// String returns the Value's concrete string, or an error when it is not
// a concrete string. It is the façade over cue.Value.String. A bottom
// returns its reason (wrapped, so errors.Is reaches the sentinel).
func (v Value) String() (string, error) {
	if err, ok := v.isBottom(); ok {
		return "", err
	}
	return v.val.String()
}

// Decode unmarshals the Value into the Go value x points at, the façade
// over cue.Value.Decode. A bottom returns its reason; a non-concrete
// value (an unresolved schema, not data) errors rather than filling x
// with a zero value.
func (v Value) Decode(x any) error {
	if err, ok := v.isBottom(); ok {
		return err
	}
	return v.val.Decode(x)
}
