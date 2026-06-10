package cuelite

import (
	"fmt"
	"regexp"
	"strings"
)

// kind classifies a node in the in-house value model. The model is the
// small CUE subset mdsmith uses (plan 218): a fixed set of shapes that
// the AST compiler builds, Unify meets, and Validate checks for
// concreteness. The lattice top is kTop, the bottom is kBottom.
type kind int

const (
	kBottom   kind = iota // ⊥, carries a reason and a path
	kTop                  // _, the lattice identity
	kNull                 // null
	kString               // concrete string
	kInt                  // concrete int64
	kFloat                // concrete float64
	kBool                 // concrete bool
	kBytes                // concrete bytes
	kAtom                 // a typed atom (a type with no concrete value): the atomKind field
	kBound                // a bounded scalar: a base atomKind plus bounds (>= <= > < != =~)
	kDisjoint             // a disjunction: branches plus an optional default
	kStruct               // an ordered struct: fields, open or closed
	kList                 // a list: [...T] or [_, ...T] with a required prefix length
	kThunk                // a deferred expression awaiting sibling-field resolution
)

// defaultMode is a disjunction branch's default status, mirroring CUE's
// per-disjunct mode (cue/internal/core/adt disjunct.go). A `*`-marked disjunct
// is dfltIs; a sibling of a marked disjunct is dfltNot (it is explicitly not a
// default); an unmarked disjunct in a disjunction with no marks is dfltMaybe.
// The ordering dfltMaybe < dfltNot < dfltIs lets a meet/dedup combine two modes
// by max with the spec's "default wins" precedence: a value that is a default
// via ANY surviving pairing is a default (so `(*1|2|9) & (*2|3|9)` keeps 2 as
// the default, the pair 2&2* being not&is = is), while two distinct sides with
// no default-pairing stay not.
type defaultMode uint8

const (
	dfltMaybe defaultMode = iota // unmarked, no mark in the disjunction
	dfltNot                      // unmarked sibling of a marked disjunct
	dfltIs                       // *-marked: a default
)

// combineMode combines two branch modes for a meet (U2) or a dedup, taking the
// stronger (max) with dfltIs winning: a value defaulted on either side stays a
// default, then a not-default, else maybe.
func combineMode(a, b defaultMode) defaultMode {
	if a > b {
		return a
	}
	return b
}

// atomKind names the base type of a typed atom or bounded scalar. number
// admits both int and float; _ is the top type (represented as kTop, not
// here). bytes is included for completeness though front matter never
// carries it.
type atomKind int

const (
	akString atomKind = iota
	akInt
	akFloat
	akNumber
	akBool
	akBytes
)

// String renders an atomKind as its CUE keyword, so a conflicting-values
// message names the type the user wrote.
func (a atomKind) String() string {
	switch a {
	case akString:
		return "string"
	case akInt:
		return "int"
	case akFloat:
		return "float"
	case akNumber:
		return "number"
	case akBool:
		return "bool"
	case akBytes:
		return "bytes"
	default:
		return fmt.Sprintf("atomKind(%d)", int(a))
	}
}

// boundOp names a comparison bound on a scalar: the five relational bounds
// and the =~ regex match.
type boundOp int

const (
	opGe       boundOp = iota // >=
	opLe                      // <=
	opGt                      // >
	opLt                      // <
	opNe                      // !=
	opMatch                   // =~
	opNotMatch                // !~
	opMinRunes                // strings.MinRunes(n): a string's rune count must be >= n
)

// String renders a boundOp as its CUE operator, used in bound-conflict and
// concreteness messages.
func (b boundOp) String() string {
	switch b {
	case opGe:
		return ">="
	case opLe:
		return "<="
	case opGt:
		return ">"
	case opLt:
		return "<"
	case opNe:
		return "!="
	case opMatch:
		return "=~"
	case opNotMatch:
		return "!~"
	case opMinRunes:
		return "strings.MinRunes"
	default:
		return fmt.Sprintf("boundOp(%d)", int(b))
	}
}

// bound is one constraint on a bounded scalar: an operator plus the value
// it compares against. For a relational bound num/str carries the operand
// (numeric for int/float kinds, string for a string !=); for opMatch re
// holds the compiled regex (compiled once at schema-compile time) and src
// its source, so the message can name the pattern.
type bound struct {
	op  boundOp
	num float64 // operand for a numeric relational bound
	str string  // operand for a string relational bound (!= "" etc.)
	re  *regexp.Regexp
	src string // regex source, for messages
	// isStr marks a relational bound whose operand is a string (str), so a
	// numeric kind never compares against a string operand.
	isStr bool
}

// field is one member of a struct value: its label, its constraint value,
// and whether the key is optional (a trailing ? in the schema).
type field struct {
	name     string
	val      *engineValue
	optional bool
}

// engineValue is one node of the in-house value model. Exactly the fields
// relevant to its kind are populated. It is built immutably by the AST
// compiler and by Unify; no method mutates a shared node.
type engineValue struct {
	kind kind

	// bottom
	reason string
	path   []string

	// concrete scalars
	str   string
	i     int64
	f     float64
	b     bool
	bytes []byte

	// kAtom / kBound base type
	atom atomKind
	// kBound constraints
	bounds []bound

	// kDisjoint: branches are the value disjuncts and modes is a parallel slice
	// carrying each branch's default mode — the ⟨value, default⟩ pair CUE
	// threads through evaluation as a per-disjunct mode (cue/internal/core/adt
	// disjunct.go). A `*` mark sets a disjunct's mode to dfltIs (M1: *v =
	// ⟨v,v⟩); building joins disjuncts keeping each mode; a meet takes the cross
	// product of branches with the mode combined by max (U2: ⟨v1,d1⟩&⟨v2,d2⟩ =
	// ⟨v1&v2, d1&d2⟩). The effective default is the branches whose mode is
	// dfltIs: exactly one distinct value is the usable default, none or several
	// is no usable default (the field stays non-concrete). When the value
	// collapses to a single branch the disjunction IS that value with no mode —
	// a single-branch disjunction is not a disjunction — so a nested default
	// whose value collapsed (`*0|0`) is discarded at the outer level, matching
	// CUE's nesting-sensitive default cancellation. modes is always the same
	// length as branches.
	branches []*engineValue
	modes    []defaultMode

	// kStruct
	fields []field
	closed bool

	// kList
	prefix  []*engineValue // required leading elements ([_, ...T])
	elem    *engineValue   // the [...T] tail element type (nil = no tail)
	openTop bool           // [...] / [...T] allows extra elements

	// kThunk: a schema expression (an index/comprehension over sibling
	// fields) that could not be evaluated at compile time because it
	// references another field. It is forced (evalThunk) once the enclosing
	// struct's referenced fields are concrete — typically after data unifies
	// in. thunkExpr is the AST to re-evaluate; thunkRefs names the sibling
	// fields it references, so the compiler can reject a reference to a name
	// that is not a declared field (an unresolvable free reference).
	thunkExpr thunkEval
	thunkRefs []string
}

// thunkEval re-evaluates a deferred schema expression against a scope of
// resolved sibling-field values, returning the value it produces. It is set
// by the compiler for an expression with free identifier references and is
// the only place the AST frontend leaks past compile time.
type thunkEval func(scope map[string]*engineValue) *engineValue

// mkBottom builds a ⊥ value carrying a reason and an optional path.
func mkBottom(path []string, format string, args ...any) *engineValue {
	return &engineValue{kind: kBottom, reason: fmt.Sprintf(format, args...), path: path}
}

// top is the lattice identity (the CUE _).
func topValue() *engineValue { return &engineValue{kind: kTop} }

// isBottomV reports whether v is ⊥.
func (v *engineValue) isBottomV() bool { return v != nil && v.kind == kBottom }

// concreteScalarV reports whether v is a concrete scalar leaf (string, int,
// float, bool, bytes, or null) — used to dedupe equal concrete disjuncts.
func (v *engineValue) concreteScalarV() bool {
	switch v.kind {
	case kString, kInt, kFloat, kBool, kBytes, kNull:
		return true
	}
	return false
}

// defaultValue resolves a disjunction's effective default from its per-branch
// modes: the branches marked dfltIs are the default disjuncts. A disjunction's
// branches are deduped at build/meet time (dedupeBranchModes), so the marked
// branches are distinct values. It returns (nil, false) when none is marked —
// the disjunction has no default and stays non-concrete — (the value, false)
// when exactly one default survives, and (nil, true) when several distinct
// defaults are marked — CUE treats multiple non-unifying defaults as ambiguous
// (`*1 | *2`), leaving the field non-concrete. (A meet's operand defaults are
// reconciled by unifyDisjunction before they reach a branch mode, so several
// dfltIs branches here are genuinely ambiguous build-time marks.)
func (v *engineValue) defaultValue() (*engineValue, bool) {
	var defs []*engineValue
	for i, br := range v.branches {
		if i < len(v.modes) && v.modes[i] == dfltIs {
			defs = append(defs, br)
		}
	}
	switch len(defs) {
	case 0:
		return nil, false
	case 1:
		return defs[0], false
	default:
		return nil, true
	}
}

// numericValue returns v's value as a float64 and true when v is a
// concrete int or float scalar, for relational-bound checks.
func (v *engineValue) numericValue() (float64, bool) {
	switch v.kind {
	case kInt:
		return float64(v.i), true
	case kFloat:
		return v.f, true
	}
	return 0, false
}

// atomKindOf returns the atomKind a concrete scalar belongs to, so a
// bound/type satisfaction check can compare base types.
func (v *engineValue) atomKindOf() (atomKind, bool) {
	switch v.kind {
	case kString:
		return akString, true
	case kInt:
		return akInt, true
	case kFloat:
		return akFloat, true
	case kBool:
		return akBool, true
	case kBytes:
		return akBytes, true
	}
	return 0, false
}

// describe renders v as a short CUE-like string for conflict messages. It
// is intentionally compact: a concrete scalar prints its literal, a type
// prints its keyword, a bound prints op+operand, a struct/list prints a
// shape token. The wording is the in-house engine's own stable contract
// (plan 238), not a reproduction of CUE's.
func (v *engineValue) describe() string {
	switch v.kind {
	case kBottom:
		return "_|_"
	case kTop:
		return "_"
	case kNull:
		return "null"
	case kString:
		return fmt.Sprintf("%q", v.str)
	case kInt:
		return fmt.Sprintf("%d", v.i)
	case kFloat:
		return fmt.Sprintf("%g", v.f)
	case kBool:
		return fmt.Sprintf("%t", v.b)
	case kBytes:
		return fmt.Sprintf("'%s'", string(v.bytes))
	case kAtom:
		return v.atom.String()
	case kBound:
		return v.describeBound()
	case kDisjoint:
		parts := make([]string, len(v.branches))
		for i, br := range v.branches {
			parts[i] = br.describe()
		}
		return strings.Join(parts, " | ")
	case kStruct:
		return "{...}"
	case kList:
		return "[...]"
	case kThunk:
		// An unforced deferred expression. It appears in a message only when a
		// thunk survives into a describe (a disjunction branch printed before its
		// force pass); name it as an unresolved expression rather than the opaque
		// "?".
		return "(unresolved expression)"
	default:
		return "?"
	}
}

// describeBound renders a bounded scalar's base type and each bound, e.g.
// `string & =~"^[a-z]+$"` or `int & >=0 & <=100`.
func (v *engineValue) describeBound() string {
	parts := make([]string, 0, len(v.bounds)+1)
	parts = append(parts, v.atom.String())
	for _, bd := range v.bounds {
		parts = append(parts, bd.describe())
	}
	return strings.Join(parts, " & ")
}

// describe renders a single bound as op+operand, e.g. `>=0`, `!=""`,
// `=~"^[a-z]+$"`.
func (b bound) describe() string {
	switch b.op {
	case opMatch:
		return fmt.Sprintf("=~%q", b.src)
	case opNotMatch:
		return fmt.Sprintf("!~%q", b.src)
	case opMinRunes:
		return fmt.Sprintf("strings.MinRunes(%d)", int64(b.num))
	}
	if b.isStr {
		return fmt.Sprintf("%s%q", b.op, b.str)
	}
	// Render an integral operand without a trailing .0.
	if b.num == float64(int64(b.num)) {
		return fmt.Sprintf("%s%d", b.op, int64(b.num))
	}
	return fmt.Sprintf("%s%g", b.op, b.num)
}
