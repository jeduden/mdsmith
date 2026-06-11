package syntax

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// coverage_review_test.go closes the statement-coverage gaps the round-1 review
// flagged on the in-house syntax package (plan 240). Each test drives a real
// parse/scan/decode path; the interface-marker test invokes the otherwise
// uncovered no-op marker methods directly.

// TestInterfaceMarkers invokes every aNode/aExpr/aDecl/aLabel/aClause marker
// method so the empty interface-satisfaction methods are covered. They carry no
// logic; calling them documents that the node set implements the interfaces.
func TestInterfaceMarkers(t *testing.T) {
	nodes := []Node{
		&File{}, &Ident{}, &BasicLit{}, &Interpolation{}, &UnaryExpr{},
		&BinaryExpr{}, &ParenExpr{}, &SelectorExpr{}, &IndexExpr{}, &CallExpr{},
		&StructLit{}, &ListLit{}, &Field{}, &EmbedDecl{}, &Ellipsis{},
		&Comprehension{}, &IfClause{}, &ForClause{}, &LetClause{},
	}
	for _, n := range nodes {
		n.aNode()
	}
	exprs := []Expr{
		&Ident{}, &BasicLit{}, &Interpolation{}, &UnaryExpr{}, &BinaryExpr{},
		&ParenExpr{}, &SelectorExpr{}, &IndexExpr{}, &CallExpr{}, &StructLit{},
		&ListLit{}, &Ellipsis{}, &Comprehension{},
	}
	for _, e := range exprs {
		e.aExpr()
	}
	decls := []Decl{&Field{}, &EmbedDecl{}, &Ellipsis{}, &Comprehension{}}
	for _, d := range decls {
		d.aDecl()
	}
	labels := []Label{&Ident{}, &BasicLit{}}
	for _, l := range labels {
		l.aLabel()
	}
	clauses := []Clause{&IfClause{}, &ForClause{}, &LetClause{}}
	for _, c := range clauses {
		c.aClause()
	}
}

// TestScanError_unexpectedChar covers the scanner's unexpected-character and
// unterminated paths that the parser surfaces as errors.
func TestScanError_paths(t *testing.T) {
	for _, src := range []string{
		`a: @x`,        // attribute: unexpected character
		`a: /* c */ 1`, // block comment: unexpected '*'
		"a: #x",        // a `#` not opening a raw string
		"a: \x00",      // NUL byte rejected up front
		`a: "unterm`,   // unterminated string
		`a: #"unterm`,  // unterminated raw string
		"a: 1 & & 2",   // operator with no right operand
		`a: [1`,        // unterminated list
		`a: {b: 1`,     // unterminated struct
		`a: (1`,        // unterminated paren
		`a: x[1`,       // unterminated index
		`a: f(1`,       // unterminated call
		`a: x.`,        // selector with no member
		`a: """ x`,     // multiline opener not followed by newline (scan ok, decode rejects later)
		"a: 1.5e",      // exponent with no digits → e is an ident → run-together
	} {
		_, err := parse(t, src)
		assert.Errorf(t, err, "%q must error", src)
	}
}

// TestScanExponent_noDigit covers scanExponent's reject branch: an `e` with no
// following digit is not an exponent (the `e` is scanned as an identifier).
func TestScanExponent_noDigit(t *testing.T) {
	// `1e+` has no exponent digit, so the number is `1` and `e+` is leftover →
	// the parser rejects the run-together.
	_, err := parse(t, `a: 1e+`)
	assert.Error(t, err)
}

// TestParseEllipsis_withType covers the ellipsis element-type branch in both
// struct and list position.
func TestParseEllipsis_withType(t *testing.T) {
	f, err := parse(t, `a: [...string]`)
	require.NoError(t, err)
	list := f.Decls[0].(*Field).Value.(*ListLit)
	require.Len(t, list.Elts, 1)
	ell := list.Elts[0].(*Ellipsis)
	require.NotNil(t, ell.Type)
	assert.Equal(t, "string", ell.Type.(*Ident).Name)

	// A struct ellipsis tail with a type.
	f, err = parse(t, `a: {...int}`)
	require.NoError(t, err)
	st := f.Decls[0].(*Field).Value.(*StructLit)
	require.Len(t, st.Elts, 1)
	assert.NotNil(t, st.Elts[0].(*Ellipsis).Type)
}

// TestMultilineInterpNestedStringParen covers skipNestedString and the
// skipInterpExpr nested-string branch: a multiline interpolation whose embedded
// expression contains a string literal carrying a `)` must not close the
// interpolation early when the scanner computes the whitespace prefix.
func TestMultilineInterpNestedStringParen(t *testing.T) {
	// The embedded expr strings.Join(xs, ")") carries a `)` inside a string; the
	// whitespace-prefix scan must skip that nested string.
	src := "\"\"\"\n  a\\(strings.Join(xs, \")\"))b\n  \"\"\""
	f, err := parse(t, src)
	require.NoError(t, err)
	require.Len(t, f.Decls, 1)
	_, ok := f.Decls[0].(*EmbedDecl).Expr.(*Interpolation)
	assert.True(t, ok, "parses as an interpolation")

	// A raw nested string inside the embedded expression.
	src = "\"\"\"\n  a\\(f(#\")\"#))b\n  \"\"\""
	_, err = parse(t, src)
	require.NoError(t, err)

	// A multiline nested string inside the embedded expression.
	src = "\"\"\"\n  a\\(f(\"\"\"\n)\n\"\"\"))b\n  \"\"\""
	_, err = parse(t, src)
	require.NoError(t, err)
}

// TestBinaryPrec_nonBinary covers binaryPrec's default (0) branch for a token
// that is not a binary operator (a prefix-only NOT).
func TestBinaryPrec_nonBinary(t *testing.T) {
	assert.Equal(t, 0, binaryPrec(NOT))
	assert.Equal(t, 0, binaryPrec(OPTION))
}

// TestRawUnquote_multiHashEscape covers rawUnquote's multi-hash introducer
// branch.
func TestRawUnquote_multiHash(t *testing.T) {
	got, err := Unquote(`##"a\##nb"##`)
	require.NoError(t, err)
	assert.Equal(t, "a\nb", got)

	// A backslash without the full hash run in a raw string is literal.
	got, err = Unquote(`##"a\nb"##`)
	require.NoError(t, err)
	assert.Equal(t, `a\nb`, got)
}

// TestStripIndent_blankAndEmptyWhitespace covers stripIndent's blank-line and
// empty-prefix branches via a decode.
func TestStripIndent_branches(t *testing.T) {
	// Empty whitespace prefix (closing quote at column 0): every line keeps its
	// own indentation, and stripIndent's ws=="" branch returns the line as-is.
	got, err := Unquote("\"\"\"\n  a\nb\n\"\"\"")
	require.NoError(t, err)
	assert.Equal(t, "  a\nb", got)
}

// TestParseFieldRest_branches covers parseFieldRest's optional, nested, and
// nested-stray-? branches.
func TestParseFieldRest_branches(t *testing.T) {
	// Optional nested-field shorthand: a?: b: c builds the OPTION + struct.
	f, err := parse(t, `a?: b: 1`)
	require.NoError(t, err)
	outer := f.Decls[0].(*Field)
	assert.Equal(t, OPTION, outer.Constraint)
	assert.IsType(t, &StructLit{}, outer.Value)

	// A stray ? in the nested value position propagates the error.
	_, err = parse(t, `a: b ?`)
	assert.Error(t, err)
}

// TestComprehension_errors covers parseComprehension / parseClause /
// parseForClause / parseLetClause error branches.
func TestComprehension_errors(t *testing.T) {
	for _, src := range []string{
		`a: [for x xs {x}]`,      // for clause missing 'in'
		`a: [for {x}]`,           // for clause missing variable
		`a: [for x, in xs {x}]`,  // two-var form missing value variable
		`a: [if c c]`,            // comprehension body not a struct
		`a: [for x in xs x]`,     // body not a struct
		`a: {let x 1}`,           // let clause missing '='
		`a: {let = 1}`,           // let clause missing variable
		`a: [for x in xs let y]`, // let in list comprehension missing '='
	} {
		_, err := parse(t, src)
		assert.Errorf(t, err, "%q must error", src)
	}

	// A two-clause comprehension (for + let) parses; the evaluator rejects the
	// multi-clause form later. This also covers parseLetClause's success path.
	f, err := parse(t, `a: {for x in xs let y = x {z: y}}`)
	require.NoError(t, err)
	comp := f.Decls[0].(*Field).Value.(*StructLit).Elts[0].(*Comprehension)
	assert.Len(t, comp.Clauses, 2)
}

// TestMultilineEscapes covers decodeMultiline's escape branch and
// isMultilineEscape's plain and raw-hash forms.
func TestMultilineEscapes(t *testing.T) {
	// Plain multiline with \t and \n escapes.
	got, err := Unquote("\"\"\"\n  a\\tb\\nc\n  \"\"\"")
	require.NoError(t, err)
	assert.Equal(t, "a\tb\nc", got)

	// Raw multiline: a \# introducer escape decodes; a bare \ is literal.
	got, err = Unquote("#\"\"\"\n  a\\#tb\n  \"\"\"#")
	require.NoError(t, err)
	assert.Equal(t, "a\tb", got)

	got, err = Unquote("#\"\"\"\n  a\\tb\n  \"\"\"#")
	require.NoError(t, err)
	assert.Equal(t, `a\tb`, got)

	// A bad escape in a multiline string errors.
	_, err = Unquote("\"\"\"\n  a\\xb\n  \"\"\"")
	assert.Error(t, err)
}

// TestRawUnquote_surrogateAndError covers rawUnquote's surrogate-pair and error
// branches via combineSurrogate.
func TestRawUnquote_surrogateAndError(t *testing.T) {
	// A raw-string surrogate pair (both halves via the \#u introducer) combines.
	got, err := Unquote(`#"\#uD83D\#uDE00"#`)
	require.NoError(t, err)
	assert.Equal(t, "😀", got)

	// A lone high surrogate errors.
	_, err = Unquote(`#"\#uD83D"#`)
	assert.Error(t, err)
}

// TestUnquoteErrors covers unquoteCUEString's malformed-literal branches.
func TestUnquoteErrors(t *testing.T) {
	for _, raw := range []string{
		"",        // empty
		"###",     // hashes with no quote
		"x",       // not a quote
		`"`,       // too short / no close
		`#"abc"`,  // raw close run mismatch
		`"""x"""`, // multiline opener not followed by newline
	} {
		_, err := Unquote(raw)
		assert.Errorf(t, err, "%q must error", raw)
	}
}

// TestTokenString covers Token.String / operatorString across the literal and
// operator names.
func TestTokenString_allNames(t *testing.T) {
	cases := map[Token]string{
		OPTION: "?", STRING: "string-literal", INT: "int-literal",
		FLOAT: "float-literal", TRUE: "true", FALSE: "false", NULL: "null",
		OR: "|", AND: "&", ADD: "+", SUB: "-", MUL: "*", NOT: "!",
		EQL: "==", NEQ: "!=", LSS: "<", GTR: ">", LEQ: "<=", GEQ: ">=",
		MAT: "=~", NMAT: "!~",
	}
	for tok, want := range cases {
		assert.Equal(t, want, tok.String())
	}
	// A token with no spelling renders as token(N).
	assert.Contains(t, NoToken.String(), "token(")
}

// TestPeekKind_pending covers peekKind's pending-token branch: after a failed
// field lookahead the label is put back, and the expression parser's peekKind
// reads the pending token.
func TestPeekKind_pending(t *testing.T) {
	// `status` alone is an embed (an ident used as a value), so tryFieldLabel
	// puts the label back and parsePrimary reads it via the pending path.
	f, err := parse(t, `status`)
	require.NoError(t, err)
	require.Len(t, f.Decls, 1)
	emb := f.Decls[0].(*EmbedDecl)
	assert.Equal(t, "status", emb.Expr.(*Ident).Name)
}
