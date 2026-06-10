package cuelite

import "fmt"

// validateEngine reports every non-concrete or ⊥ leaf in v, each tagged
// with its field path — the in-house equivalent of CUE's
// Validate(Concrete(true)). It returns the leaves in encounter order; the
// caller (Value.Validate) joins them into the *PathError shape consumers
// read through Errors. A fully concrete value yields no leaves.
//
// "Concrete" means a scalar with a definite value: a string/int/float/
// bool/bytes/null leaf, or a struct/list whose members are all concrete.
// A bare type, a bound awaiting data, or a disjunction with more than one
// branch is non-concrete and reported. An optional struct field that is
// absent is fine; an optional field present as a constraint is checked
// like any other.
func validateEngine(v *engineValue) []*PathError {
	var out []*PathError
	return collectLeaves(v, nil, out)
}

// collectLeaves walks v depth-first, appending a *PathError for every
// non-concrete or ⊥ leaf at its path. A struct or list recurses into its
// members; a scalar/atom/bound/disjunction is a leaf decided here.
func collectLeaves(v *engineValue, path []string, out []*PathError) []*PathError {
	switch v.kind {
	case kBottom:
		// A ⊥ carries its own path from the failing unify; prefer it over the
		// walk path so a nested conflict reports the leaf it occurred at.
		p := v.path
		if p == nil {
			p = path
		}
		return append(out, newPathError(p, v.reason, nil))
	case kString, kInt, kFloat, kBool, kBytes, kNull:
		return out // concrete leaf
	case kTop:
		return append(out, newPathError(path, "incomplete value _", nil))
	case kAtom:
		return append(out, newPathError(path, fmt.Sprintf("incomplete value %s", v.atom), nil))
	case kBound:
		return append(out, newPathError(path, fmt.Sprintf("incomplete value %s", v.describeBound()), nil))
	case kDisjoint:
		return append(out, newPathError(path, fmt.Sprintf("incomplete value %s", v.describe()), nil))
	case kStruct:
		for _, f := range v.fields {
			out = collectLeaves(f.val, appendPath(path, f.name), out)
		}
		return out
	case kList:
		for i, el := range v.prefix {
			out = collectLeaves(el, appendPath(path, intLabel(i)), out)
		}
		// An open list's tail element type is a constraint, not data: a list
		// value reduced to data has its elements in prefix, so the tail is only
		// present on an unsatisfied schema. Report it as incomplete.
		if v.openTop && v.elem != nil && v.elem.kind != kTop {
			out = append(out, newPathError(path, fmt.Sprintf("incomplete list element %s", v.elem.describe()), nil))
		}
		return out
	default:
		return append(out, newPathError(path, "incomplete value", nil))
	}
}
