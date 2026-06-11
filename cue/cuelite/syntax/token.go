package syntax

import "fmt"

// token.go is the in-house token set the parser emits and the evaluators
// switch on. It replaces cuelang.org/go/cue/token (plan 240): only the tokens
// the cuelite subset reaches are defined, with the SAME names the consumers
// already use (token.MUL, token.OR, token.GEQ, …) so a consumer's switch
// changes only its import.

// Token is one lexical token or operator. The String method renders an
// operator as its source spelling for the error messages that quote it
// (`cuelite: unsupported binary operator "&&"`).
type Token int

const (
	// NoToken is the zero value: a Field with no `?` constraint carries it, so
	// `el.Constraint == OPTION` is false for a required field.
	NoToken Token = iota

	// STRING is a quoted string literal (BasicLit.Kind).
	STRING
	// INT is an integer literal (BasicLit.Kind).
	INT
	// FLOAT is a floating-point literal (BasicLit.Kind).
	FLOAT
	// TRUE is the bool literal true (BasicLit.Kind).
	TRUE
	// FALSE is the bool literal false (BasicLit.Kind).
	FALSE
	// NULL is the null literal (BasicLit.Kind).
	NULL
	kInterpFrag // an Interpolation fragment whose Value is already decoded

	// OPTION is the `?` optional-field marker (Field.Constraint).
	OPTION

	// OR is the `|` disjunction binary operator.
	OR
	// AND is the `&` meet binary operator.
	AND
	// ADD is the `+` addition / unary plus operator.
	ADD
	// SUB is the `-` subtraction / unary minus operator.
	SUB
	// MUL is the `*` multiplication / disjunction default mark.
	MUL
	// NOT is the `!` boolean negation operator.
	NOT
	// EQL is the `==` equal comparison operator.
	EQL
	// NEQ is the `!=` not-equal comparison operator.
	NEQ
	// LSS is the `<` less comparison operator.
	LSS
	// GTR is the `>` greater comparison operator.
	GTR
	// LEQ is the `<=` less-or-equal comparison operator.
	LEQ
	// GEQ is the `>=` greater-or-equal comparison operator.
	GEQ
	// MAT is the `=~` regex match operator.
	MAT
	// NMAT is the `!~` regex non-match operator.
	NMAT
)

// String renders a token as its source spelling, for error messages that quote
// the operator. A non-operator token renders as a name.
func (t Token) String() string {
	if s, ok := operatorString(t); ok {
		return s
	}
	switch t {
	case OPTION:
		return "?"
	case STRING:
		return "string-literal"
	case INT:
		return "int-literal"
	case FLOAT:
		return "float-literal"
	case TRUE:
		return "true"
	case FALSE:
		return "false"
	case NULL:
		return "null"
	default:
		return fmt.Sprintf("token(%d)", int(t))
	}
}

// operatorString returns the source spelling of a binary or unary operator
// token and ok=true. It returns "", false for tokens that are not operators
// (literals, OPTION, NoToken, kInterpFrag).
func operatorString(t Token) (string, bool) {
	switch t {
	case OR:
		return "|", true
	case AND:
		return "&", true
	case ADD:
		return "+", true
	case SUB:
		return "-", true
	case MUL:
		return "*", true
	case NOT:
		return "!", true
	case EQL:
		return "==", true
	case NEQ:
		return "!=", true
	case LSS:
		return "<", true
	case GTR:
		return ">", true
	case LEQ:
		return "<=", true
	case GEQ:
		return ">=", true
	case MAT:
		return "=~", true
	case NMAT:
		return "!~", true
	}
	return "", false
}
