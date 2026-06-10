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

// concreteEqual reports whether two concrete scalars are equal. The kinds
// must match: CUE keeps a concrete int and a concrete float distinct (the
// literal 0 and the literal 0.0 do not unify), so an int never equals a
// float here. A YAML/JSON decoder hands a whole number back as an int (42)
// and a decimal as a float64 (42.0), and the lifter preserves that kind, so
// `weight: 42` unifies with `int` and `weight: 42.0` unifies with `float`,
// matching CUE — no cross-kind equality is needed or wanted here. (The
// relational == operator does compare numbers across kinds; see
// numericAwareEqual.)
func concreteEqual(a, b *engineValue) bool {
	if a.kind != b.kind {
		return false
	}
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

// concreteValueEqual reports whether two CONCRETE values are equal across every
// kind a disjunction default or dedup may carry: a scalar (via concreteEqual),
// a list (same length, element-wise equal), or a struct (same field set,
// value-wise equal). A non-concrete value is never equal to another here — the
// callers gate on concreteness first. This generalizes concreteEqual so a
// concrete LIST or STRUCT default (`*[]`, `*{x:0}`) deduplicates and matches a
// surviving branch the way a scalar default does.
func concreteValueEqual(a, b *engineValue) bool {
	if a.kind != b.kind {
		return false
	}
	switch a.kind {
	case kList:
		if a.openTop || b.openTop || len(a.prefix) != len(b.prefix) {
			return false
		}
		for i := range a.prefix {
			if !concreteValueEqual(a.prefix[i], b.prefix[i]) {
				return false
			}
		}
		return true
	case kStruct:
		if len(a.fields) != len(b.fields) {
			return false
		}
		bIdx := indexFields(b.fields)
		for _, fa := range a.fields {
			j, ok := bIdx[fa.name]
			if !ok || !concreteValueEqual(fa.val, b.fields[j].val) {
				return false
			}
		}
		return true
	default:
		return concreteEqual(a, b)
	}
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
	case opNotMatch:
		return c.kind == kString && !bd.re.MatchString(c.str)
	case opMinRunes:
		// Compare in float64 space: int(bd.num) would truncate a huge
		// MinRunes argument on 32-bit targets (wasm) and invert the check.
		return c.kind == kString && float64(utf8.RuneCountInString(c.str)) >= bd.num
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
		if b := incompatibleBounds(merged); b != nil {
			return mkBottom(path, "incompatible bounds %s and %s", b.lo.describe(), b.hi.describe())
		}
		return &engineValue{kind: kBound, atom: ak, bounds: merged}
	default:
		return conflict(path, v, o)
	}
}

// incompatiblePair names a lower/upper bound pair whose interval is empty.
type incompatiblePair struct {
	lo bound
	hi bound
}

// incompatibleBounds reports the first lower/upper relational bound pair whose
// interval is empty — the meet of `>=10 & <=5` or `>0 & <0` is ⊥. It mirrors
// CUE, which rejects such a pair at compile time ("incompatible number/string
// bounds") while leaving other shapes (an empty `>=5 & <=5 & !=5`, a regex
// pair) to resolve at validate time. So only the ordered numeric/string bounds
// participate: != , =~ , !~ , and strings.MinRunes are not folded in, matching
// CUE's compile-time bound check. A numeric and a string bound never share a
// kind here (meetAtom already rejected that mix), so the comparison is on like
// operands.
func incompatibleBounds(bounds []bound) *incompatiblePair {
	for i := range bounds {
		lo := bounds[i]
		if !lo.isLowerBound() {
			continue
		}
		for j := range bounds {
			hi := bounds[j]
			if !hi.isUpperBound() {
				continue
			}
			// Every bound in a merged set shares a kind: meetAtom rejects mixing a
			// numeric and a string bound before they reach here, so lo and hi are
			// both numeric or both string and emptyInterval compares like operands.
			if emptyInterval(lo, hi) {
				return &incompatiblePair{lo: lo, hi: hi}
			}
		}
	}
	return nil
}

// isLowerBound reports whether a relational bound is a lower bound (>= or >).
func (b bound) isLowerBound() bool { return b.op == opGe || b.op == opGt }

// isUpperBound reports whether a relational bound is an upper bound (<= or <).
func (b bound) isUpperBound() bool { return b.op == opLe || b.op == opLt }

// emptyInterval reports whether a lower bound and an upper bound describe an
// empty interval: the lower operand exceeds the upper, or the two operands are
// equal and at least one side is strict (>x & <x, >=x & <x, >x & <=x are all
// empty; >=x & <=x is the singleton {x} and non-empty). Operands are compared
// numerically for numeric bounds and lexically for string bounds, both via the
// same primitives the satisfaction checks use.
func emptyInterval(lo, hi bound) bool {
	strict := lo.op == opGt || hi.op == opLt
	if lo.isStr {
		if lo.str > hi.str {
			return true
		}
		return lo.str == hi.str && strict
	}
	if lo.num > hi.num {
		return true
	}
	return lo.num == hi.num && strict
}

// unifyDisjunction meets disjunction d with o, taking the cross product of d's
// branches with o's branches (each as a (value, mode) pair), per CUE's meet
// rule U2: ⟨v1,d1⟩ & ⟨v2,d2⟩ = ⟨v1&v2, d1&d2⟩, the mode of each result branch
// the combineMode (max) of its two parents'. A result branch whose meet
// produced ANY bottom — top-level OR nested (a closed-struct violation, a field
// conflict, a bound failure deep in a struct/list) — is pruned, so the
// surviving branch decides the disjunction. The survivors are deduped (the
// stronger mode kept). A single survivor collapses to that bare value; an empty
// cross product is ⊥.
//
// So `(*1|int) & (*2|int)` reduces to `1|2|int`: the only both-default pair is
// 1&2 = ⊥, dropped, so no default survives and the field stays non-concrete.
// `(*1|2|9) & (*2|3|9)` keeps default 2 (the pair 2&2* is mode max(not,is)=is).
func unifyDisjunction(d, o *engineValue, path []string) *engineValue {
	survivors := meetBranchModes(d, o, path)
	if len(survivors) == 0 {
		return mkBottom(path, "%s does not satisfy %s", o.describe(), d.describe())
	}
	survivors = dedupeBranchModes(survivors)
	if len(survivors) == 1 {
		return survivors[0].v
	}
	// Resolve the meet's default from the OPERAND defaults (U2), then re-mark
	// the surviving value branch that equals it as dfltIs. Computing the
	// default once from the operands avoids the per-branch combineMode
	// fabricating a spurious second default: `(*0|int)&(0|*int)` has the single
	// default 0&int = 0, not the ambiguous {0, int} the raw cross product
	// produces.
	def := meetDefault(d, o, path)
	out := &engineValue{kind: kDisjoint}
	out.branches = make([]*engineValue, len(survivors))
	out.modes = make([]defaultMode, len(survivors))
	for i, s := range survivors {
		out.branches[i] = s.v
		mode := dfltNot
		if def != nil && isConcrete(s.v) && concreteValueEqual(s.v, def) {
			mode = dfltIs
		}
		out.modes[i] = mode
	}
	return out
}

// meetDefault resolves the default of the meet d & o from the OPERAND defaults
// (CUE's U2 ⟨v1,d1⟩&⟨v2,d2⟩ = ⟨v1&v2, d1&d2⟩). Each operand's default is its
// marked value (operandDefault). A side's default SURVIVES the meet when it is
// compatible with the other operand's value (their meet is not ⊥). When BOTH
// survive, the default is their meet (`(*0|int)&(0|*int)` → 0&int = 0); when
// ONLY ONE survives, it is that surviving default met with the other operand
// (`(*1|2|9)&(*2|3|9)` → only 2 survives → 2); when neither survives, or the
// reconciled default is non-concrete, there is no usable default. A
// non-defaulted operand contributes no default of its own.
func meetDefault(d, o *engineValue, path []string) *engineValue {
	dd := surviving(operandDefault(d), o, path)
	od := surviving(operandDefault(o), d, path)
	switch {
	case dd != nil && od != nil:
		return concreteOrNil(unifyV(dd, od, path))
	case dd != nil:
		return concreteOrNil(unifyV(dd, o, path))
	case od != nil:
		return concreteOrNil(unifyV(od, d, path))
	default:
		return nil
	}
}

// surviving returns def when it is non-nil and compatible with other (their
// meet is not ⊥), else nil: a default that conflicts with the other operand's
// value is dropped from the meet's default (CUE's drops-default rule).
func surviving(def, other *engineValue, path []string) *engineValue {
	if def == nil {
		return nil
	}
	if unifyV(def, other, path).isBottomV() {
		return nil
	}
	return def
}

// concreteOrNil returns v when it is fully concrete (a scalar, or a concrete
// list/struct such as the `*[]` default), else nil — a meet default is usable
// only when it pins a single concrete value.
func concreteOrNil(v *engineValue) *engineValue {
	if isConcrete(v) {
		return v
	}
	return nil
}

// operandDefault returns a disjunction operand's own default value (the meet of
// its dfltIs branches when concrete), or nil when it has no usable default or
// is not a disjunction. It is the d in the ⟨v, d⟩ pair the meet rule consumes.
func operandDefault(v *engineValue) *engineValue {
	if v.kind != kDisjoint {
		return nil
	}
	def, _ := v.defaultValue()
	return def
}

// meetBranchModes takes the cross product of d's (value, mode) branches with
// o's, meeting each pair. A branch whose meet contains a bottom leaf (top-level
// OR nested — a deep closed-struct violation, field conflict, or bound failure)
// is pruned WHENEVER at least one branch met cleanly: the surviving clean
// branch decides the disjunction (CUE's branch-failure rule, P0c). But when
// EVERY branch's meet contains a bottom, the dirty branches are kept so their
// nested bottom leaves surface at validate (the disjunction fails, and the
// failing field — not just a root summary — is reported). The result branch's
// mode is combineMode of its two parents'. A non-disjunction operand
// contributes one branch (mode dfltMaybe).
func meetBranchModes(d, o *engineValue, path []string) []branchMode {
	dm := branchModesOf(d)
	om := branchModesOf(o)
	var clean, dirty []branchMode
	for _, db := range dm {
		for _, ob := range om {
			m := unifyV(db.v, ob.v, path)
			bm := branchMode{v: m, mode: combineMode(db.mode, ob.mode)}
			switch {
			case m.isBottomV():
				// A TOP-LEVEL bottom branch is dropped outright: it carries no
				// nested leaf to surface and a clean or nested-dirty branch is a
				// better failure witness.
				continue
			case hasBottomLeaf(m):
				dirty = append(dirty, bm)
			default:
				clean = append(clean, bm)
			}
		}
	}
	if len(clean) > 0 {
		return clean
	}
	// No branch met cleanly: keep the nested-dirty branches so their buried
	// bottom leaves (a deep field conflict) surface at validate, naming the
	// failing field rather than only a root summary.
	return dirty
}

// branchModesOf returns the (value, mode) branches of a value: a disjunction's
// own branches+modes, or a single dfltMaybe branch for any other value (a
// non-disjunction operand of a meet has no default mark of its own).
func branchModesOf(v *engineValue) []branchMode {
	if v.kind != kDisjoint {
		return []branchMode{{v: v, mode: dfltMaybe}}
	}
	out := make([]branchMode, len(v.branches))
	for i, br := range v.branches {
		m := dfltMaybe
		if i < len(v.modes) {
			m = v.modes[i]
		}
		out[i] = branchMode{v: br, mode: m}
	}
	return out
}

// hasBottomLeaf reports whether v is a bottom or contains a bottom anywhere a
// meet could have buried one: a struct field, a list element, or a disjunction
// branch. A disjunction branch whose meet produced a NESTED bottom (a deep
// closed-struct violation or field conflict) must be pruned exactly like a
// top-level bottom, so the surviving branch decides the disjunction (CUE's
// branch-failure rule). The walk descends structs and lists; it does not need
// to descend a kDisjoint operand differently — a surviving disjunction branch
// is itself a valid value, so only an actual bottom leaf prunes.
func hasBottomLeaf(v *engineValue) bool {
	switch v.kind {
	case kBottom:
		return true
	case kStruct:
		for _, f := range v.fields {
			if hasBottomLeaf(f.val) {
				return true
			}
		}
	case kList:
		for _, el := range v.prefix {
			if hasBottomLeaf(el) {
				return true
			}
		}
		if v.elem != nil && hasBottomLeaf(v.elem) {
			return true
		}
	case kDisjoint:
		// A disjunction survives as long as one branch is non-bottom; a buried
		// bottom branch was already pruned when that disjunction was built or met.
		// So a kDisjoint here is a valid value, never a failed branch.
		return false
	}
	return false
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
	// a data-side sibling resolves. A FIXPOINT loop re-forces until no thunk
	// newly resolves in a pass: an acyclic chain `n: [if m…][0], o: [if n…][0]`
	// resolves n in pass 1 (m is concrete), then o in pass 2 (n is now
	// concrete). The acyclic-schema guarantee bounds the loop by the field
	// count; a remaining thunk after the fixpoint surfaces as today.
	v, o = forceThunkFixpoint(v, o)
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

// forceThunkFixpoint repeatedly forces the thunk fields of v and o against the
// scope of their combined concrete fields until a pass resolves no new thunk —
// the fixpoint of CUE's lazy field resolution. Each pass rebuilds the scope
// from the (possibly newly-forced) fields, so a thunk that depends on another
// thunk's result resolves once the earlier thunk forced. The loop is bounded by
// the total field count: an acyclic schema resolves one more thunk per pass, so
// it terminates; the bound also caps a pathological input. A thunk still
// unresolved after the fixpoint is left as the ⊥ its force pass yields, so it
// surfaces at validate.
func forceThunkFixpoint(v, o *engineValue) (*engineValue, *engineValue) {
	bound := len(v.fields) + len(o.fields) + 1
	for pass := 0; pass < bound; pass++ {
		if !hasThunkField(v) && !hasThunkField(o) {
			return v, o
		}
		scope := concreteScope(v, o)
		before := len(scope)
		// SOFT force: a thunk whose references are not yet concrete is left for a
		// later pass rather than collapsed to ⊥, so a chain `n: …, o: [if n…]`
		// resolves n first, then o.
		if hasThunkField(v) {
			v = forceThunks(v, scope, true)
		}
		if hasThunkField(o) {
			o = forceThunks(o, scope, true)
		}
		// A pass that grew no new concrete field has reached the fixpoint: the
		// remaining thunks reference a still-unresolved (or absent) sibling.
		if len(concreteScope(v, o)) == before {
			break
		}
	}
	// A HARD final force collapses any thunk the fixpoint could not resolve to
	// the ⊥ its force pass yields, so an unresolvable thunk surfaces at validate.
	scope := concreteScope(v, o)
	if hasThunkField(v) {
		v = forceThunks(v, scope, false)
	}
	if hasThunkField(o) {
		o = forceThunks(o, scope, false)
	}
	return v, o
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

// hasThunkField reports whether v carries an unforced thunk anywhere a
// struct's force pass would reach it: a direct field, or a thunk nested in a
// field's list elements or disjunction branches. unifyStruct only pays for the
// force pass when one is present. The release-channels idiom puts a thunk in a
// direct field (`registry: [if …][0]`), but a constraint like
// `xs: [mech != ""]` nests the thunk in a list element and a `(m == "a") | "z"`
// nests it in a disjunction branch, so the scan must descend into both.
func hasThunkField(v *engineValue) bool {
	for _, f := range v.fields {
		if hasThunkValue(f.val) {
			return true
		}
	}
	return false
}

// hasThunkValue reports whether v IS an unforced thunk or carries one in a
// list element or disjunction branch. It deliberately does not descend into a
// nested struct: a nested struct forces its own thunks against its own sibling
// scope when it is itself unified, so the outer scope must not reach into it.
func hasThunkValue(v *engineValue) bool {
	switch v.kind {
	case kThunk:
		return true
	case kList:
		for _, el := range v.prefix {
			if hasThunkValue(el) {
				return true
			}
		}
		if v.elem != nil && hasThunkValue(v.elem) {
			return true
		}
	case kDisjoint:
		for _, br := range v.branches {
			if hasThunkValue(br) {
				return true
			}
		}
	}
	return false
}

// forceThunks returns a copy of struct v with every thunk reachable in a
// direct field — or nested in that field's list elements or disjunction
// branches — replaced by the value it evaluates to against scope. When soft is
// true a thunk whose references are not yet all concrete in scope is left as a
// thunk for a later fixpoint pass (so a chain `n: …, o: [if n…]` does not
// collapse o to ⊥ before n resolves); when soft is false a still-unresolved
// thunk evaluates to a ⊥ (deferToThunk's fallback), so Validate reports it
// rather than silently accepting an unforced schema field. A nested struct is
// left untouched: it forces its own thunks against its own scope when it is
// unified.
func forceThunks(v *engineValue, scope map[string]*engineValue, soft bool) *engineValue {
	out := *v
	out.fields = make([]field, len(v.fields))
	for i, f := range v.fields {
		out.fields[i] = f
		out.fields[i].val = forceThunkValue(f.val, scope, soft)
	}
	return &out
}

// thunkResolvable reports whether every sibling reference a thunk needs is
// present and concrete in scope, so a soft force may evaluate it. A thunk with
// an unresolved reference is deferred to a later fixpoint pass instead of
// collapsing to ⊥.
func thunkResolvable(v *engineValue, scope map[string]*engineValue) bool {
	for _, ref := range v.thunkRefs {
		s, ok := scope[ref]
		if !ok || !isConcrete(s) {
			return false
		}
	}
	return true
}

// forceThunkValue forces a thunk anywhere it sits in v — v itself, a list
// element, or a disjunction branch — against scope, returning the resolved
// value. A list or disjunction is rebuilt with each member forced; a
// disjunction is then re-reduced (its forced branches may collapse, dedupe, or
// drop to ⊥, just as at build time) so a forced branch participates in the
// disjunction's defaults and concreteness exactly as a compile-time branch
// would. A nested struct is returned unchanged: it forces its own thunks
// against its own scope when it is unified, so the outer scope must not leak
// into it. Any other value has no thunk and is returned as-is. When soft, a
// thunk whose references are not all concrete in scope is returned unchanged.
func forceThunkValue(v *engineValue, scope map[string]*engineValue, soft bool) *engineValue {
	switch v.kind {
	case kThunk:
		if soft && !thunkResolvable(v, scope) {
			return v
		}
		return v.thunkExpr(scope)
	case kList:
		if !hasThunkValue(v) {
			return v
		}
		out := *v
		out.prefix = make([]*engineValue, len(v.prefix))
		for i, el := range v.prefix {
			out.prefix[i] = forceThunkValue(el, scope, soft)
		}
		if v.elem != nil {
			out.elem = forceThunkValue(v.elem, scope, soft)
		}
		return &out
	case kDisjoint:
		if !hasThunkValue(v) {
			return v
		}
		// Force each branch, preserving its mode, then re-reduce so a forced
		// branch that dropped to ⊥ or collapsed is handled exactly like a
		// freshly built disjunction. The disjunction already carries an explicit
		// mode per branch (dfltIs/dfltNot survive a force), so buildDisjunction
		// must NOT re-raise maybe→not: pass hasMark=false and let the carried
		// modes stand.
		forced := make([]branchMode, len(v.branches))
		for i, br := range v.branches {
			m := dfltMaybe
			if i < len(v.modes) {
				m = v.modes[i]
			}
			forced[i] = branchMode{v: forceThunkValue(br, scope, soft), mode: m}
		}
		return buildDisjunction(forced, false)
	default:
		return v
	}
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
		// listElemAt never returns nil for an index reached here: a closed list
		// shorter than the other's prefix is already rejected by listLengthOK, and
		// an open list always carries an elem type (the builders set it), so a
		// past-prefix index of an open list yields that elem.
		ev := listElemAt(v, i)
		oe := listElemAt(o, i)
		out.prefix = append(out.prefix, unifyV(ev, oe, appendPath(path, intLabel(i))))
	}
	if out.openTop {
		// Both lists are open here (out.openTop = v.openTop && o.openTop), so each
		// carries a non-nil elem type; the tail is their meet.
		out.elem = unifyV(v.elem, o.elem, path)
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
