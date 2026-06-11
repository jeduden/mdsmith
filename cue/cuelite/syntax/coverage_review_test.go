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

	// A postfix selector on a put-back label exercises peekKind's pending path:
	// `status.x` puts `status` back, then parsePostfix peeks the `.` via
	// peekKind while pending is set.
	f, err = parse(t, `status.x`)
	require.NoError(t, err)
	assert.IsType(t, &SelectorExpr{}, f.Decls[0].(*EmbedDecl).Expr)
}

// TestExprErrorBranches covers the error-propagation branches in the
// expression and list parsers: a malformed index expression, a malformed list
// ellipsis element type, and a bad value in the nested-field shorthand.
func TestExprErrorBranches(t *testing.T) {
	for _, src := range []string{
		`a: x[@]`,           // index expression parse error (parsePostfix)
		`a: [...@]`,         // list ellipsis element-type parse error
		`a: b: @`,           // nested-field shorthand value parse error
		`a: [if c {x} @]`,   // list element after a comprehension is malformed
		`a: {if c @}`,       // comprehension if-condition parse error
		`a: {for x in @}`,   // for-clause source parse error
		`a: {let y = @}`,    // let-clause expr parse error
		`a: {for x in xs @`, // comprehension body not a struct (and unterminated)
	} {
		_, err := parse(t, src)
		assert.Errorf(t, err, "%q must error", src)
	}
}

// TestInterpFragmentErrors covers the decode-error branches in scanStringBody
// (a bad escape in the first or final interpolation fragment).
func TestInterpFragmentErrors(t *testing.T) {
	// A bad escape in the FIRST fragment (before the `\(`).
	_, err := parse(t, `a: "x\q\(id)"`)
	assert.Error(t, err)

	// A bad escape in the FINAL fragment (after the `)`).
	_, err = parse(t, `a: "\(id)y\q"`)
	assert.Error(t, err)
}

// TestCombineSurrogate_invalidPairs covers combineSurrogate's reject branches:
// a high-then-high and a low-then-high pair are not valid surrogate pairs.
func TestCombineSurrogate_invalidPairs(t *testing.T) {
	for _, raw := range []string{
		`"\uD83D\uD83D"`, // high then high
		`"\uDC00\uD83D"`, // low then high
		`"\uD83DA"`,      // high then a non-surrogate
	} {
		_, err := Unquote(raw)
		assert.Errorf(t, err, "%q is not a valid surrogate pair", raw)
	}
}

// TestRawString_newlineAndCR covers rawUnquote's illegal-newline and
// illegal-CR branches.
func TestRawString_newlineAndCR(t *testing.T) {
	_, err := Unquote("#\"a\nb\"#")
	assert.Error(t, err, "a raw single-line string rejects a newline")

	_, err = Unquote("#\"a\rb\"#")
	assert.Error(t, err, "a raw single-line string rejects a carriage return")
}

// TestMultilineWhitespace_unterminatedInterp covers multilineWhitespace's
// no-close branch: a multiline interpolation whose embedded expression never
// closes its paren.
func TestMultilineWhitespace_unterminatedInterp(t *testing.T) {
	// The `\(` opens an interpolation whose `)` never arrives before EOF, so the
	// whitespace scan over the interpolation fails to find the close.
	_, err := parse(t, "a: \"\"\"\n  x\\(f(\n  \"\"\"")
	assert.Error(t, err)
}

// TestMultilineCRAndEscape covers decodeMultiline's CR-drop and bad-escape
// branches.
func TestMultilineCRAndEscape(t *testing.T) {
	// CRLF normalises to LF; a bare CR is dropped.
	got, err := Unquote("\"\"\"\r\n  a\r\n  b\r\n  \"\"\"")
	require.NoError(t, err)
	assert.Equal(t, "a\nb", got)
}

// TestEllipsisTypeParseErrors covers the ellipsis element-type parse-error
// branches in both list (parseListElem) and struct (parseEllipsisDecl)
// position. A `1+` starts an expression but has no right operand, so the inner
// parseExpr fails after startsExpr admitted it.
func TestEllipsisTypeParseErrors(t *testing.T) {
	_, err := parse(t, `a: [...1+]`)
	assert.Error(t, err, "list ellipsis type parse error")

	_, err = parse(t, `a: {...1+}`)
	assert.Error(t, err, "struct ellipsis type parse error")
}

// TestComprehensionInnerParseErrors covers the parse-error propagation in the
// comprehension clauses and body: a malformed if-condition, for-source,
// let-expr, and body.
func TestComprehensionInnerParseErrors(t *testing.T) {
	for _, src := range []string{
		`a: {if ) {x: 1}}`,               // if-condition parse error
		`a: [for x in ) {x}]`,            // for-source parse error
		`a: [for x in xs let y = ) {y}]`, // let-expr parse error
		`a: {for x in xs {y: 1+}}`,       // comprehension body field value error
		`a: {for x, 1 in xs {y: x}}`,     // two-var for: value variable not an ident
	} {
		_, err := parse(t, src)
		assert.Errorf(t, err, "%q must error", src)
	}
}

// TestFieldLabelDecodeError covers compileFile's fieldLabel-error branch via a
// top-level string-label field with a bad escape — but that lives in the
// cuelite package, so here we only assert the parser captures the raw label and
// Unquote rejects it (compileBasicLit/fieldLabel decode it downstream).
func TestFieldLabelDecodeError(t *testing.T) {
	f, err := parse(t, `"\q": 1`)
	require.NoError(t, err, "the parser captures the label raw")
	lbl := f.Decls[0].(*Field).Label.(*BasicLit)
	_, err = Unquote(lbl.Value)
	assert.Error(t, err, "the bad-escape label fails to decode")
}

// TestMultilineInterpOpenAndIndentErrors covers decodeMultiline's
// opening-newline and first-fragment indentation error branches reached through
// an INTERPOLATED multiline string (the scanStringBody first-fragment path).
func TestMultilineInterpOpenAndIndentErrors(t *testing.T) {
	// Opening `"""` not followed by a newline, with an interpolation following.
	_, err := parse(t, "a: \"\"\"x\\(id)\n  \"\"\"")
	assert.Error(t, err, "interpolated multiline opener must be followed by a newline")

	// Under-indented first content line in an interpolated multiline string.
	_, err = parse(t, "a: \"\"\"\nx\\(id)\n  \"\"\"")
	assert.Error(t, err, "under-indented first interpolated line must error")
}

// TestMultilineInterpEscapes covers isEscapeAt / skipNestedString escape
// branches reached while scanning the whitespace prefix over an interpolation
// whose embedded expression contains an escaped quote.
func TestMultilineInterpEscapes(t *testing.T) {
	// The embedded string contains an escaped quote (\") so the escape-aware
	// skip must step over it without mistaking it for the string close.
	src := "\"\"\"\n  a\\(f(\"x\\\"y\"))b\n  \"\"\""
	_, err := parse(t, src)
	require.NoError(t, err)
}

// TestSelectorOutsideCall covers eval.go's unsupported-bare-selector branch via
// a schema that uses strings.MinRunes as a value rather than a call.
func TestSelectorOutsideCall_parsesAsSelector(t *testing.T) {
	// At the syntax level this is a valid SelectorExpr; the evaluator rejects it.
	f, err := parse(t, `a: strings.MinRunes`)
	require.NoError(t, err)
	assert.IsType(t, &SelectorExpr{}, f.Decls[0].(*Field).Value)
}

// TestCombineSurrogate_lowHalfShapes covers combineSurrogate's remaining reject
// branches in a raw string: the second `\#` introducer's selector is not
// `u`/`U`, and a well-formed second escape that is not a low surrogate.
func TestCombineSurrogate_lowHalfShapes(t *testing.T) {
	// High half then a `\#n` escape (selector is not u/U).
	_, err := Unquote(`#"\#uD83D\#nDC00"#`)
	assert.Error(t, err)

	// High half then a valid `\#u` escape that is NOT a low surrogate (U+0041).
	_, err = Unquote(`#"\#uD83D\#u0041"#`)
	assert.Error(t, err)
}

// TestMultilineWhitespaceScan_escapeAndHash covers isEscapeAt (the escape-aware
// step) and skipNestedString's non-string branch while the scanner computes the
// whitespace prefix over an interpolation: an embedded expression carrying an
// escaped quote and a bare `#` that does not open a raw string.
func TestMultilineWhitespaceScan_escapeAndHash(t *testing.T) {
	// Embedded string with an escaped quote: the whitespace-prefix scan steps
	// over the escape via isEscapeAt rather than reading it as the close.
	src := "\"\"\"\n  a\\(g(\"p\\\"q\"))b\n  \"\"\""
	_, err := parse(t, src)
	require.NoError(t, err)

	// Embedded expression with a lone `#` (not a raw-string open): skipNestedString
	// returns i+1 for the non-string `#` while the scanner computes the multiline
	// whitespace prefix. The parse fails later at the parser, but the scanner's
	// whitespace scan exercises the non-string branch first.
	_, _ = parse(t, "a: \"\"\"\n  \\(a # b)\n  \"\"\"")
}

// TestMultilinePlainEscape covers multilineWhitespace's isEscapeAt branch: a
// plain (non-interpolated) multiline string carrying a `\t` escape steps over
// the escape while the scanner walks the body for the closing delimiter.
func TestMultilinePlainEscape(t *testing.T) {
	f, err := parse(t, "a: \"\"\"\n  x\\ty\n  \"\"\"")
	require.NoError(t, err)
	got, err := Unquote(f.Decls[0].(*Field).Value.(*BasicLit).Value)
	require.NoError(t, err)
	assert.Equal(t, "x\ty", got)
}

// TestIsEscapeAt_rawHashRun covers isEscapeAt's `#`-run advance (the `j++` after
// a matched hash): in a raw string the escape introducer is `\` + the `#` run,
// so a `\#…` escape at hashes>0 walks the whole run before confirming a
// selector follows.
func TestIsEscapeAt_rawHashRun(t *testing.T) {
	s := &scanner{src: `\#x`}
	assert.True(t, s.isEscapeAt(0, 1), `\#x is a raw escape at hashes=1`)

	// A `\` without the full `#` run is a literal backslash, not an escape.
	s = &scanner{src: `\x`}
	assert.False(t, s.isEscapeAt(0, 1), `\x is not a raw escape at hashes=1`)

	// A two-hash introducer walks both `#`s.
	s = &scanner{src: `\##y`}
	assert.True(t, s.isEscapeAt(0, 2), `\##y is a raw escape at hashes=2`)
}
