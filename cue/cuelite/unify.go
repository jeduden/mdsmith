package cuelite

import (
	"strconv"
	"unicode/utf8"
)

// unifyV is the lattice meet of two engine values at the given path — the
// value satisfying both, per plan 218's rules. ⊥ absorbs; ⊤ is the
// identity; concrete & concrete must be equal; concrete & bound/type must
// satisfy; bound & bound intersect on a shared kind; a disjunction
// distributes (dropping ⊥ branches, collapsing a singleton, preserving a
// default); structs unify field-wise honoring closedness; lists unify by
// element and length. The result is a fresh value; neither operand is
// mutated. path is the field route to v/o, carried into any ⊥ so Validate
// reports the failing leaf.
func unifyV(v, o *engineValue, path []string) *engineValue {
	// ⊥ absorbs. A bottom operand keeps its own reason and path so the
	// originating failure is the one surfaced.
	if v.isBottomV() {
		return v
	}
	if o.isBottomV() {
		return o
	}
	// ⊤ is the identity: _ & x == x.
	if v.kind == kTop {
		return o
	}
	if o.kind == kTop {
		return v
	}
	// A thunk reaching unifyV outside a struct's force pass has no sibling
	// scope to resolve against; force it with an empty scope (yielding a ⊥ for
	// an unresolved reference) so it never silently unifies to top.
	if v.kind == kThunk {
		return unifyV(v.thunkExpr(nil), o, path)
	}
	if o.kind == kThunk {
		return unifyV(v, o.thunkExpr(nil), path)
	}
	// A disjunction distributes over the other operand.
	if v.kind == kDisjoint {
		return unifyDisjunction(v, o, path)
	}
	if o.kind == kDisjoint {
		return unifyDisjunction(o, v, path)
	}
	switch v.kind {
	case kNull:
		return unifyNull(v, o, path)
	case kStruct:
		return unifyStruct(v, o, path)
	case kList:
		return unifyList(v, o, path)
	case kString, kInt, kFloat, kBool, kBytes:
		return unifyConcrete(v, o, path)
	case kAtom:
		return unifyAtomLeft(v, o, path)
	case kBound:
		return unifyBoundLeft(v, o, path)
	default:
		return mkBottom(path, "cannot unify %s and %s", v.describe(), o.describe())
	}
}

// unifyNull meets null with the other operand: null & null is null, null
// against anything else (including top, already handled) is a conflict.
func unifyNull(v, o *engineValue, path []string) *engineValue {
	if o.kind == kNull {
		return v
	}
	return conflict(path, v, o)
}

// unifyConcrete meets a concrete scalar v with o. Two concretes must be
// equal; a concrete against a type/bound must satisfy it (delegated to the
// symmetric handler so the concrete is always the value side).
func unifyConcrete(v, o *engineValue, path []string) *engineValue {
	switch o.kind {
	case kString, kInt, kFloat, kBool, kBytes, kNull:
		if concreteEqual(v, o) {
			return v
		}
		return conflict(path, v, o)
	case kAtom:
		return unifyConcreteAtom(v, o, path)
	case kBound:
		return unifyConcreteBound(v, o, path)
	default:
		return conflict(path, v, o)
	}
}

// concreteEqual reports whether two concrete scalars are equal. An int and
// a float with the same value unify (CUE treats 1 and 1.0 as the same
// number); otherwise kinds must match.
func concreteEqual(a, b *engineValue) bool {
	if a.kind == b.kind {
		switch a.kind {
		case kString:
			return a.str == b.str
		case kInt:
			return a.i == b.i
		case kFloat:
			return a.f == b.f
		case kBool:
			return a.b == b.b
		case kBytes:
			return string(a.bytes) == string(b.bytes)
		case kNull:
			return true
		}
		return false
	}
	// int vs float numeric equality (1 == 1.0).
	an, aok := a.numericValue()
	bn, bok := b.numericValue()
	return aok && bok && an == bn
}

// unifyConcreteAtom checks a concrete scalar against a typed atom: the
// concrete must belong to the atom's kind (with number admitting int and
// float). On match the concrete (the more specific value) is the result.
func unifyConcreteAtom(c, a *engineValue, path []string) *engineValue {
	if concreteSatisfiesAtom(c, a.atom) {
		return c
	}
	return conflict(path, c, a)
}

// concreteSatisfiesAtom reports whether a concrete scalar belongs to an
// atom kind. number admits int and float; int and float admit only their
// own concrete kind.
func concreteSatisfiesAtom(c *engineValue, ak atomKind) bool {
	ck, ok := c.atomKindOf()
	if !ok {
		return false
	}
	if ak == akNumber {
		return ck == akInt || ck == akFloat
	}
	return ck == ak
}

// unifyConcreteBound checks a concrete scalar against a bounded scalar: it
// must satisfy the base kind and every bound. On success the concrete is
// the result (a bound met by a concrete collapses to that concrete).
func unifyConcreteBound(c, b *engineValue, path []string) *engineValue {
	if !concreteSatisfiesAtom(c, b.atom) {
		return conflict(path, c, b)
	}
	for _, bd := range b.bounds {
		if !concreteSatisfiesBound(c, bd) {
			return mkBottom(path, "%s does not satisfy %s", c.describe(), bd.describe())
		}
	}
	return c
}

// concreteSatisfiesBound reports whether a concrete scalar satisfies one
// bound: a relational comparison for numeric/string operands, a regex
// match for =~, and a rune-count check for strings.MinRunes.
func concreteSatisfiesBound(c *engineValue, bd bound) bool {
	switch bd.op {
	case opMatch:
		return c.kind == kString && bd.re.MatchString(c.str)
	case opMinRunes:
		return c.kind == kString && utf8.RuneCountInString(c.str) >= int(bd.num)
	}
	if bd.isStr {
		if c.kind != kString {
			return false
		}
		return compareStr(c.str, bd.op, bd.str)
	}
	n, ok := c.numericValue()
	if !ok {
		return false
	}
	return compareNum(n, bd.op, bd.num)
}

// compareNum evaluates a numeric relational bound.
func compareNum(x float64, op boundOp, y float64) bool {
	switch op {
	case opGe:
		return x >= y
	case opLe:
		return x <= y
	case opGt:
		return x > y
	case opLt:
		return x < y
	case opNe:
		return x != y
	}
	return false
}

// compareStr evaluates a string relational bound (the only one front
// matter uses is != "").
func compareStr(x string, op boundOp, y string) bool {
	switch op {
	case opGe:
		return x >= y
	case opLe:
		return x <= y
	case opGt:
		return x > y
	case opLt:
		return x < y
	case opNe:
		return x != y
	}
	return false
}

// unifyAtomLeft meets a typed atom v with o. Against a concrete the
// concrete side wins (delegated); against another atom the kinds must be
// compatible (number ⊓ int = int); against a bound the bound's base kind
// must match and the bound is the result (the atom adds no constraint).
func unifyAtomLeft(v, o *engineValue, path []string) *engineValue {
	switch o.kind {
	case kString, kInt, kFloat, kBool, kBytes, kNull:
		return unifyConcrete(o, v, path)
	case kAtom:
		ak, ok := meetAtom(v.atom, o.atom)
		if !ok {
			return conflict(path, v, o)
		}
		return &engineValue{kind: kAtom, atom: ak}
	case kBound:
		ak, ok := meetAtom(v.atom, o.atom)
		if !ok {
			return conflict(path, v, o)
		}
		out := *o
		out.atom = ak
		return &out
	default:
		return conflict(path, v, o)
	}
}

// meetAtom intersects two atom kinds: equal kinds meet to themselves,
// number meets int/float to the narrower kind, and any other pair is
// incompatible.
func meetAtom(a, b atomKind) (atomKind, bool) {
	if a == b {
		return a, true
	}
	if a == akNumber && (b == akInt || b == akFloat) {
		return b, true
	}
	if b == akNumber && (a == akInt || a == akFloat) {
		return a, true
	}
	return 0, false
}

// unifyBoundLeft meets a bounded scalar v with o. Against a concrete the
// concrete is checked (delegated); against an atom the base kind narrows
// and v's bounds carry over; against another bound the base kinds meet and
// the bound sets union.
func unifyBoundLeft(v, o *engineValue, path []string) *engineValue {
	switch o.kind {
	case kString, kInt, kFloat, kBool, kBytes, kNull:
		return unifyConcrete(o, v, path)
	case kAtom:
		ak, ok := meetAtom(v.atom, o.atom)
		if !ok {
			return conflict(path, v, o)
		}
		out := *v
		out.atom = ak
		return &out
	case kBound:
		ak, ok := meetAtom(v.atom, o.atom)
		if !ok {
			return conflict(path, v, o)
		}
		merged := make([]bound, 0, len(v.bounds)+len(o.bounds))
		merged = append(merged, v.bounds...)
		merged = append(merged, o.bounds...)
		return &engineValue{kind: kBound, atom: ak, bounds: merged}
	default:
		return conflict(path, v, o)
	}
}

// unifyDisjunction distributes a meet over disjunction d's branches: each
// branch is met with o, ⊥ results are dropped, and the surviving branches
// form the result. A single survivor collapses to that value; an empty
// result is ⊥. A default branch that survives is preserved.
func unifyDisjunction(d, o *engineValue, path []string) *engineValue {
	var survivors []*engineValue
	var survivingDefault *engineValue
	for _, br := range d.branches {
		m := unifyV(br, o, path)
		if m.isBottomV() {
			continue
		}
		survivors = append(survivors, m)
		if d.def != nil && br == d.def {
			survivingDefault = m
		}
	}
	switch len(survivors) {
	case 0:
		return mkBottom(path, "%s does not satisfy %s", o.describe(), d.describe())
	case 1:
		return survivors[0]
	default:
		return &engineValue{kind: kDisjoint, branches: survivors, def: survivingDefault}
	}
}

// unifyStruct meets two structs field-wise. A field present in both is the
// meet of its constraints; a field present in one carries over (with its
// optionality). When either struct is closed, a field present only in the
// other and not declared in the closed struct is a ⊥ (a closed struct
// rejects an undeclared key). Field order follows v then o's new fields.
func unifyStruct(v, o *engineValue, path []string) *engineValue {
	if o.kind != kStruct {
		return conflict(path, v, o)
	}
	// Force any deferred (thunk) field now that the two structs' fields are
	// both in hand: a schema thunk like `[if mechanism == "push" {…}, …][0]`
	// resolves against the concrete sibling values data supplies. The scope
	// draws from BOTH structs' concrete fields, so a schema-side reference to
	// a data-side sibling resolves.
	scope := concreteScope(v, o)
	if hasThunkField(v) {
		v = forceThunks(v, scope)
	}
	if hasThunkField(o) {
		o = forceThunks(o, scope)
	}
	closed := v.closed || o.closed
	out := &engineValue{kind: kStruct, closed: closed}
	oIndex := indexFields(o.fields)
	seen := make(map[string]bool, len(v.fields))
	for _, fv := range v.fields {
		seen[fv.name] = true
		if oi, ok := oIndex[fv.name]; ok {
			of := o.fields[oi]
			child := unifyV(fv.val, of.val, appendPath(path, fv.name))
			out.fields = append(out.fields, field{
				name:     fv.name,
				val:      child,
				optional: fv.optional && of.optional,
			})
			continue
		}
		// Field only in v. If o is closed and does not declare it, reject.
		if o.closed {
			out.fields = append(out.fields, field{
				name: fv.name,
				val:  mkBottom(appendPath(path, fv.name), "field not allowed in closed struct"),
			})
			continue
		}
		out.fields = append(out.fields, fv)
	}
	for _, of := range o.fields {
		if seen[of.name] {
			continue
		}
		if v.closed {
			out.fields = append(out.fields, field{
				name: of.name,
				val:  mkBottom(appendPath(path, of.name), "field not allowed in closed struct"),
			})
			continue
		}
		out.fields = append(out.fields, of)
	}
	return out
}

// concreteScope builds a name→value scope from the concrete fields of two
// structs, used to force a thunk field that references a sibling. A field
// concrete in either struct is bound; a non-concrete or absent field is left
// out so a thunk that needs it stays deferred (and ultimately ⊥) rather than
// resolving against a still-unfixed reference.
func concreteScope(a, b *engineValue) map[string]*engineValue {
	scope := make(map[string]*engineValue)
	for _, src := range []*engineValue{a, b} {
		for _, f := range src.fields {
			if f.val.kind == kThunk {
				continue
			}
			if isConcrete(f.val) {
				scope[f.name] = f.val
			}
		}
	}
	return scope
}

// hasThunkField reports whether any direct field of v is an unforced thunk,
// so unifyStruct only pays for the force pass when one is present.
func hasThunkField(v *engineValue) bool {
	for _, f := range v.fields {
		if f.val.kind == kThunk {
			return true
		}
	}
	return false
}

// forceThunks returns a copy of struct v with each thunk field replaced by
// the value it evaluates to against scope. A thunk whose references are still
// unresolved evaluates to a ⊥ (deferToThunk's fallback), so Validate reports
// it rather than silently accepting an unforced schema field.
func forceThunks(v *engineValue, scope map[string]*engineValue) *engineValue {
	out := *v
	out.fields = make([]field, len(v.fields))
	for i, f := range v.fields {
		out.fields[i] = f
		if f.val.kind == kThunk {
			out.fields[i].val = f.val.thunkExpr(scope)
		}
	}
	return &out
}

// indexFields builds a name→index map over a field slice for O(1) lookup
// during struct unification.
func indexFields(fs []field) map[string]int {
	m := make(map[string]int, len(fs))
	for i, f := range fs {
		m[f.name] = i
	}
	return m
}

// unifyList meets two lists by prefix elements and tail/length. The
// resulting prefix is the element-wise meet up to the longer prefix, with
// each side's tail element filling where the other has no prefix element;
// the result stays open only when both inputs are open. A closed list
// against a longer prefix is a length conflict.
func unifyList(v, o *engineValue, path []string) *engineValue {
	if o.kind != kList {
		return conflict(path, v, o)
	}
	// Length compatibility: a closed list fixes its length to its prefix.
	if err := listLengthOK(v, o, path); err != nil {
		return err
	}
	n := len(v.prefix)
	if len(o.prefix) > n {
		n = len(o.prefix)
	}
	out := &engineValue{kind: kList, openTop: v.openTop && o.openTop}
	for i := 0; i < n; i++ {
		ev := listElemAt(v, i)
		oe := listElemAt(o, i)
		if ev == nil {
			ev = topValue()
		}
		if oe == nil {
			oe = topValue()
		}
		out.prefix = append(out.prefix, unifyV(ev, oe, appendPath(path, intLabel(i))))
	}
	if out.openTop {
		te := topValue()
		if v.elem != nil && o.elem != nil {
			te = unifyV(v.elem, o.elem, path)
		} else if v.elem != nil {
			te = v.elem
		} else if o.elem != nil {
			te = o.elem
		}
		out.elem = te
	}
	return out
}

// listLengthOK reports a ⊥ when the two lists' lengths cannot agree: a
// closed list (no tail) cannot match a longer required prefix on the other
// side.
func listLengthOK(v, o *engineValue, path []string) *engineValue {
	if !v.openTop && len(o.prefix) > len(v.prefix) {
		return mkBottom(path, "list length %d does not match closed list length %d",
			len(o.prefix), len(v.prefix))
	}
	if !o.openTop && len(v.prefix) > len(o.prefix) {
		return mkBottom(path, "list length %d does not match closed list length %d",
			len(v.prefix), len(o.prefix))
	}
	return nil
}

// listElemAt returns the constraint for the i-th element of a list: the
// explicit prefix element when present, else the tail element type for an
// open list, else nil (no constraint, treated as top).
func listElemAt(l *engineValue, i int) *engineValue {
	if i < len(l.prefix) {
		return l.prefix[i]
	}
	if l.openTop {
		return l.elem
	}
	return nil
}

// conflict builds the standard conflicting-values ⊥ at path, naming both
// operands. This is the in-house engine's own stable wording (plan 238):
// `conflicting values <a> and <b>`, lowercase, with the values shown. The
// two descriptions are ordered lexically so the message is deterministic
// regardless of which operand was the Unify receiver — operand order no
// longer matters for a context-free Value, and the message must not either.
func conflict(path []string, a, b *engineValue) *engineValue {
	da, db := a.describe(), b.describe()
	if da > db {
		da, db = db, da
	}
	return mkBottom(path, "conflicting values %s and %s", da, db)
}

// appendPath returns a fresh path with seg appended, never aliasing the
// caller's slice, so sibling unifications do not corrupt each other's path.
func appendPath(path []string, seg string) []string {
	out := make([]string, len(path)+1)
	copy(out, path)
	out[len(path)] = seg
	return out
}

// intLabel renders a list index as its decimal string for a path segment.
func intLabel(i int) string {
	return strconv.Itoa(i)
}
