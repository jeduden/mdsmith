package cuelite

import (
	"fmt"
)

// Exists reports whether the Value names something concrete enough to read.
// A bottom (a zero or error-carrying Value) does not exist; a compiled
// value, or a successful [Value.LookupPath] result, does. A consumer uses
// it to tell "the path resolved" from "the path was absent" without
// inspecting the value further.
func (v Value) Exists() bool {
	if _, ok := v.isBottom(); ok {
		return false
	}
	return true
}

// Err reports whether the Value is a bottom — a compile failure, a zero
// Value, or a value reduced to bottom by a conflicting Unify — or nil when
// it names a value. Unlike [Value.Validate] it does NOT check
// concreteness: a successfully compiled but non-concrete schema (a
// constraint awaiting data) has no Err. A caller uses it to tell a broken
// schema from one merely waiting on its document.
func (v Value) Err() error {
	if err, ok := v.isBottom(); ok {
		return err
	}
	// A struct or list reduced by a conflicting Unify carries a ⊥ at the
	// offending leaf while the container itself is not ⊥. Err reports that
	// reduced-to-bottom value (matching cue.Value.Err), so checkUnifiable and
	// the cached-schema status check see a conflict.
	if b := firstBottom(v.v); b != nil {
		return newPathError(b.path, b.reason, nil)
	}
	return nil
}

// firstBottom returns the first ⊥ engine value reachable in v (depth-first),
// or nil when v carries none. It lets Err and Compile detect a conflict that
// reduced a nested field to ⊥ without collapsing the whole container, so the
// per-leaf paths survive for Validate.
func firstBottom(v *engineValue) *engineValue {
	switch v.kind {
	case kBottom:
		return v
	case kStruct:
		for _, f := range v.fields {
			if b := firstBottom(f.val); b != nil {
				return b
			}
		}
	case kList:
		for _, el := range v.prefix {
			if b := firstBottom(el); b != nil {
				return b
			}
		}
	}
	return nil
}

// LookupPath returns the Value at p within v, and whether it exists. A
// missing leaf, or a lookup against a bottom, returns ok=false; an empty
// path returns the receiver. Because a Value is context-free, the result is
// immediately usable and shareable with no rebuild.
func (v Value) LookupPath(p Path) (Value, bool) {
	if _, ok := v.isBottom(); ok {
		return bottom(errZeroValue), false
	}
	cur := v.v
	for _, seg := range p.segments {
		next, ok := lookupField(cur, seg)
		if !ok {
			return bottom(errZeroValue), false
		}
		cur = next
	}
	return Value{v: cur}, true
}

// lookupField resolves one path segment within a value: a struct field by
// name. A list, scalar, or any non-struct value has no string-labelled
// members, so a lookup into one returns ok=false — mirroring cue.Value's
// string-selector lookup, which does not index a list by a string segment.
// Query and schema only ever look up struct keys.
func lookupField(v *engineValue, seg string) (*engineValue, bool) {
	if v.kind != kStruct {
		return nil, false
	}
	for _, f := range v.fields {
		if f.name == seg {
			return f.val, true
		}
	}
	return nil, false
}

// Field is one member of a struct Value: its label (the unquoted selector
// string, usable directly with [MakePath]) and its Value.
type Field struct {
	Selector string
	Value    Value
}

// Fields returns the members of a struct Value in definition order, or nil
// for a non-struct or bottom Value. Each [Field.Selector] is the raw label
// string, so a consumer building a path from it must use [MakePath] (not
// [ParsePath]), which stores a dotted or hyphenated key verbatim.
func (v Value) Fields() []Field {
	if _, ok := v.isBottom(); ok {
		return nil
	}
	if v.v.kind != kStruct {
		return nil
	}
	out := make([]Field, 0, len(v.v.fields))
	for _, f := range v.v.fields {
		out = append(out, Field{Selector: f.name, Value: Value{v: f.val}})
	}
	return out
}

// String returns the Value's concrete string, or an error when it is not a
// concrete string. A bottom returns its reason (wrapped, so errors.Is
// reaches the sentinel).
func (v Value) String() (string, error) {
	if err, ok := v.isBottom(); ok {
		return "", err
	}
	if v.v.kind != kString {
		return "", fmt.Errorf("cuelite: value is %s, not a concrete string", v.v.describe())
	}
	return v.v.str, nil
}

// Decode unmarshals the Value into the Go value x points at. A bottom
// returns its reason; a non-concrete value (an unresolved schema, not data)
// errors rather than filling x with a zero value. Decode supports the
// concrete shapes mdsmith's callers read out: a string into *string, and a
// struct/list/scalar into *any.
func (v Value) Decode(x any) error {
	if err, ok := v.isBottom(); ok {
		return err
	}
	goVal, err := decodeValue(v.v)
	if err != nil {
		return err
	}
	switch dst := x.(type) {
	case *any:
		*dst = goVal
		return nil
	case *string:
		s, ok := goVal.(string)
		if !ok {
			return fmt.Errorf("cuelite: cannot decode %s into *string", v.v.describe())
		}
		*dst = s
		return nil
	case *map[string]any:
		m, ok := goVal.(map[string]any)
		if !ok {
			return fmt.Errorf("cuelite: cannot decode %s into *map[string]any", v.v.describe())
		}
		*dst = m
		return nil
	default:
		return fmt.Errorf("cuelite: Decode does not support target type %T", x)
	}
}

// decodeValue converts a concrete engine value into a plain Go value
// (string, int64, float64, bool, []byte, nil, map[string]any, []any). A
// non-concrete leaf (a type, bound, or multi-branch disjunction) errors,
// matching cue.Value.Decode's refusal to fill from an incomplete value.
func decodeValue(v *engineValue) (any, error) {
	switch v.kind {
	case kNull:
		return nil, nil
	case kString:
		return v.str, nil
	case kInt:
		return v.i, nil
	case kFloat:
		return v.f, nil
	case kBool:
		return v.b, nil
	case kBytes:
		return v.bytes, nil
	case kStruct:
		out := make(map[string]any, len(v.fields))
		for _, f := range v.fields {
			child, err := decodeValue(f.val)
			if err != nil {
				return nil, err
			}
			out[f.name] = child
		}
		return out, nil
	case kList:
		out := make([]any, 0, len(v.prefix))
		for _, el := range v.prefix {
			child, err := decodeValue(el)
			if err != nil {
				return nil, err
			}
			out = append(out, child)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("cuelite: cannot decode incomplete value %s", v.describe())
	}
}
