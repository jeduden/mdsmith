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

	// kDisjoint
	branches []*engineValue
	def      *engineValue // the *-marked default branch, or nil

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
	// in. thunkExpr is the AST to re-evaluate.
	thunkExpr thunkEval
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
