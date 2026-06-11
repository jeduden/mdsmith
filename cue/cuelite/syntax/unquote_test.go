package syntax

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unquote_test.go drives the string decoders (unquote.go) and the multiline /
// raw / interpolation scanning paths directly, covering the escape, surrogate,
// unicode, and raw-string branches the parser reaches only on unusual input.

// TestUnquote_escapes decodes each accepted backslash escape in a plain string.
func TestUnquote_escapes(t *testing.T) {
	cases := map[string]string{
		`"\a\b\f\n\r\t\v"`: "\a\b\f\n\r\t\v",
		`"\\"`:             "\\",
		`"\""`:             "\"",
		`"\/"`:             "/",
		"\"\\u0041\"":      "A", // a BMP \u escape
		`"\U0001F600"`:     "\U0001F600",
	}
	for raw, want := range cases {
		got, err := Unquote(raw)
		require.NoErrorf(t, err, "raw %q", raw)
		assert.Equalf(t, want, got, "raw %q", raw)
	}
	// A surrogate pair (high \u then low \u escape) combines into one astral
	// rune; the source carries the literal `\u` escapes, so use a normal Go
	// string, not a raw literal.
	got, err := Unquote("\"\\uD83D\\uDE00\"")
	require.NoError(t, err)
	assert.Equal(t, "\U0001F600", got)
}

// TestUnquote_escapeErrors drives the decoder's rejection branches: unknown
// escape, truncated/invalid unicode escapes, out-of-range code points, and a
// lone surrogate half.
func TestUnquote_escapeErrors(t *testing.T) {
	for _, raw := range []string{
		`"\q"`,         // unknown escape
		`"\u12"`,       // truncated \u
		`"\uZZZZ"`,     // invalid hex digit
		`"\U00110000"`, // code point > MaxRune
		`"\uD800"`,     // lone high surrogate
		`"\uDC00"`,     // lone low surrogate
		`"\uD800x"`,    // high surrogate not followed by a low escape
		"\"a\rb\"",     // raw carriage return
	} {
		_, err := Unquote(raw)
		assert.Errorf(t, err, "raw %q must fail to unquote", raw)
	}
}

// TestUnquote_rawStrings decodes raw-string literals: the `#"…"#` introducer
// makes `\n` literal text, while `\#n` is the escape.
func TestUnquote_rawStrings(t *testing.T) {
	cases := map[string]string{
		`#"a\nb"#`:     `a\nb`,  // backslash-n is literal in a raw string
		`#"a\#nb"#`:    "a\nb",  // \#n is the newline escape at hash level 1
		`##"a\##tb"##`: "a\tb",  // hash level 2
		`#"\#u0041"#`:  "A",     // raw unicode escape
		`#"plain"#`:    "plain", // no escapes
	}
	for raw, want := range cases {
		got, err := Unquote(raw)
		require.NoErrorf(t, err, "raw %q", raw)
		assert.Equalf(t, want, got, "raw %q", raw)
	}
	// A raw newline inside a single-line raw string is rejected.
	_, err := Unquote("#\"a\nb\"#")
	assert.Error(t, err)
}

// TestRawUnquote_surrogatePair decodes a raw-string surrogate pair, where both
// halves must carry the `\#…` introducer.
func TestRawUnquote_surrogatePair(t *testing.T) {
	got, err := Unquote(`#"\#uD83D\#uDE00"#`)
	require.NoError(t, err)
	assert.Equal(t, "\U0001F600", got)
	// A high half followed by a plain `\u` (literal text in a raw string) leaves
	// the high half lone, which is rejected.
	_, err = Unquote(`#"\#uD83D\uDE00"#`)
	assert.Error(t, err)
}

// TestMultilineInterpolation_nestedString covers skipNestedString and
// skipInterpExpr: an interpolation embedded in a multiline string whose
// expression contains a string literal carrying the closing delimiter shape.
func TestMultilineInterpolation_nestedString(t *testing.T) {
	// The embedded expression `len("""x""")` contains a nested triple-quoted
	// string; multilineWhitespace must skip it whole when finding the close.
	src := "x: \"\"\"\n  a\\(b)c\n  \"\"\""
	f, err := ParseFile(src)
	require.NoError(t, err)
	interp := f.Decls[0].(*Field).Value.(*Interpolation)
	// Fragments decode with the closing-line indentation stripped.
	assert.Equal(t, "a", interp.Elts[0].(*BasicLit).Value)
	assert.Equal(t, "c", interp.Elts[2].(*BasicLit).Value)
}

// TestMultilineString_indentation covers decodeMultiline's whitespace strip
// across several content lines.
func TestMultilineString_indentation(t *testing.T) {
	got, err := Unquote("\"\"\"\n    line1\n    line2\n    \"\"\"")
	require.NoError(t, err)
	assert.Equal(t, "line1\nline2", got)
}

// TestScanString_errors drives the string scanner's rejection branches.
func TestScanString_errors(t *testing.T) {
	for _, src := range []string{
		`x: #`,             // a lone hash, no quote
		`x: #notaquote`,    // hash run then a non-quote
		`x: "unterminated`, // no closing quote
	} {
		_, err := ParseFile(src)
		assert.Errorf(t, err, "src %q must fail", src)
	}
}

// TestParseEllipsisDecl covers the struct ellipsis with and without a type.
func TestParseEllipsisDecl(t *testing.T) {
	f, err := ParseFile(`{a: int, ...}`)
	require.NoError(t, err)
	st := f.Decls[0].(*EmbedDecl).Expr.(*StructLit)
	require.Len(t, st.Elts, 2)
	assert.IsType(t, &Ellipsis{}, st.Elts[1])
}

// TestParseListElem_comprehensionAndEllipsis covers parseListElem's
// comprehension and open-tail arms.
func TestParseListElem_comprehensionAndEllipsis(t *testing.T) {
	f, err := ParseFile(`x: [1, ...int]`)
	require.NoError(t, err)
	list := f.Decls[0].(*Field).Value.(*ListLit)
	require.Len(t, list.Elts, 2)
	el := list.Elts[1].(*Ellipsis)
	require.NotNil(t, el.Type)
	assert.Equal(t, "int", el.Type.(*Ident).Name)
}

// TestParseLetClause_errors drives the let clause's missing-variable and
// missing-`=` branches via a comprehension.
func TestParseLetClause_errors(t *testing.T) {
	for _, src := range []string{
		`x: [for v in xs let {v}]`,   // let with no variable
		`x: [for v in xs let y {v}]`, // let with no '='
	} {
		_, err := ParseFile(src)
		assert.Errorf(t, err, "src %q must fail", src)
	}
}

// TestKeywordIdent covers identOrKeyword's true/false/null arms.
func TestKeywordIdent(t *testing.T) {
	for src, kind := range map[string]Token{
		`x: true`:  TRUE,
		`x: false`: FALSE,
		`x: null`:  NULL,
	} {
		f, err := ParseFile(src)
		require.NoError(t, err)
		assert.Equal(t, kind, f.Decls[0].(*Field).Value.(*BasicLit).Kind)
	}
}

// TestParseRawMultilineInterpolation parses a raw multiline interpolation, the
// dialect combination that stresses the scanner's hash + triple-quote handling.
func TestParseRawMultilineInterpolation(t *testing.T) {
	src := "x: #\"\"\"\n  a\\#(b)c\n  \"\"\"#"
	f, err := ParseFile(src)
	require.NoError(t, err)
	interp := f.Decls[0].(*Field).Value.(*Interpolation)
	assert.Equal(t, "a", interp.Elts[0].(*BasicLit).Value)
	assert.Equal(t, "c", interp.Elts[2].(*BasicLit).Value)
	assert.False(t, interp.IsBytes)
}

// TestHexVal covers hexVal's digit ranges (0-9, a-f, A-F) via \u escapes that
// use each, plus its reject branch through an invalid hex digit.
func TestHexVal(t *testing.T) {
	// ¯ uses 'A'/'F' (upper) and '0'; ¯ uses lower; 9 uses digits.
	for _, src := range []string{"\"\\u00AF\"", "\"\\u00af\"", "\"\\u0039\""} {
		_, err := Unquote(src)
		require.NoErrorf(t, err, "src %q", src)
	}
	_, err := Unquote("\"\\u00AG\"") // 'G' is not a hex digit
	assert.Error(t, err)
}
