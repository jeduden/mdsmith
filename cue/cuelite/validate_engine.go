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

// isUnsatisfiedConstraint reports whether v is an absent optional field's
// still-unsatisfied constraint, as opposed to a value data provided. An
// absent optional field keeps exactly its declared constraint, which is some
// non-concrete shape with no ⊥; a provided optional field is concrete (or
// reduced to ⊥ on a conflict). So an optional field is "absent" when it
// carries no ⊥ and is not fully concrete — a bare type/bound/disjunction, an
// open or non-concrete list, or a struct with a non-concrete member. This
// distinguishes "the optional field was never provided" (skip it) from "the
// optional field was provided and conflicts" (a ⊥, still reported).
func isUnsatisfiedConstraint(v *engineValue) bool {
	if firstBottom(v) != nil {
		return false
	}
	return !isConcrete(v)
}

// isConcrete reports whether v is fully concrete data: a concrete scalar or
// null, a closed list of concrete elements, or a struct whose every field is
// concrete. A type, bound, disjunction, top, open list, or any ⊥ makes it
// non-concrete.
func isConcrete(v *engineValue) bool {
	switch v.kind {
	case kString, kInt, kFloat, kBool, kBytes, kNull:
		return true
	case kDisjoint:
		// A disjunction resolves to its effective default when one survives, so a
		// defaulted disjunction counts as concrete (the absent field takes its
		// default). Multiple distinct defaults are ambiguous — CUE leaves such a
		// disjunction non-concrete — so defaultValue reports no usable default and
		// the disjunction is not concrete.
		def, _ := v.defaultValue()
		return def != nil && isConcrete(def)
	case kStruct:
		for _, f := range v.fields {
			if f.optional && isUnsatisfiedConstraint(f.val) {
				continue
			}
			if !isConcrete(f.val) {
				return false
			}
		}
		return true
	case kList:
		// An open list's tail defaults to the empty list, so [...int] is
		// concrete (it can close to []). Only a non-concrete REQUIRED prefix
		// element ([_, ...int]) makes the list non-concrete.
		for _, el := range v.prefix {
			if !isConcrete(el) {
				return false
			}
		}
		return true
	default:
		return false
	}
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
		// A disjunction with a single effective default resolves to it when
		// nothing else pins it (e.g. an absent `unlisted: bool | *false` field
		// takes false). The default must itself be concrete; validate it. Multiple
		// distinct defaults are ambiguous (CUE: `*1 | *2` is non-concrete), so the
		// disjunction is reported incomplete rather than silently taking one.
		if def, _ := v.defaultValue(); def != nil {
			return collectLeaves(def, path, out)
		}
		return append(out, newPathError(path, fmt.Sprintf("incomplete value %s", v.describe()), nil))
	case kThunk:
		// An unforced thunk awaited data that never arrived (validating a schema
		// alone, or a reference left unresolved). It is incomplete.
		return append(out, newPathError(path, "incomplete value (unresolved expression)", nil))
	case kStruct:
		for _, f := range v.fields {
			// An optional field whose value is still a bare constraint was never
			// satisfied by data: it is absent, not failing. CUE drops an absent
			// optional field, so skip it rather than reporting it incomplete. An
			// optional field that DID receive concrete data (or reduced to ⊥ on a
			// conflict) is checked like any other.
			if f.optional && isUnsatisfiedConstraint(f.val) {
				continue
			}
			out = collectLeaves(f.val, appendPath(path, f.name), out)
		}
		return out
	case kList:
		// An open list's tail element type ([...int]) defaults to the empty
		// list, so it is concrete and adds no leaf (matching CUE). Only the
		// REQUIRED prefix elements are validated; a non-concrete prefix element
		// ([_, ...int]) surfaces as an incomplete leaf at its index.
		for i, el := range v.prefix {
			out = collectLeaves(el, appendPath(path, intLabel(i)), out)
		}
		return out
	default:
		return append(out, newPathError(path, "incomplete value", nil))
	}
}
