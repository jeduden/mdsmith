package syntax

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// p0review_test.go pins the round-1 parser fixes (plan 240 phase 4): the
// raw-string round-trip, leading-zero/underscore number rejection, the
// missing-declaration-separator rule, the stray-`?` report, and multiline
// strictness. Every expected verdict was confirmed against a cuelang.org/go
// v0.16.1 oracle before the fix landed.

// TestRawStringRoundTrip pins item 1: a non-interpolated raw string
// (`#"..."#`) must parse and decode to its body, including the canonical
// regex idiom `=~#"^\d+$"#`. The scanner previously dropped the leading `#`
// run from the raw token, so unquoteCUEString saw `"..."#` and reported
// "unterminated".
func TestRawStringRoundTrip(t *testing.T) {
	f, err := parse(t, `a: #"hello"#`)
	require.NoError(t, err)
	fld := f.Decls[0].(*Field)
	lit := fld.Value.(*BasicLit)
	require.Equal(t, STRING, lit.Kind)
	got, err := Unquote(lit.Value)
	require.NoError(t, err)
	assert.Equal(t, "hello", got)

	// The canonical regex idiom: a raw string spares the regex backslashes.
	f, err = parse(t, `a: =~#"^\d+$"#`)
	require.NoError(t, err)
	un := f.Decls[0].(*Field).Value.(*UnaryExpr)
	require.Equal(t, MAT, un.Op)
	rx := un.X.(*BasicLit)
	got, err = Unquote(rx.Value)
	require.NoError(t, err)
	assert.Equal(t, `^\d+$`, got)

	// A multi-hash raw string round-trips too.
	f, err = parse(t, `a: ##"x"##`)
	require.NoError(t, err)
	got, err = Unquote(f.Decls[0].(*Field).Value.(*BasicLit).Value)
	require.NoError(t, err)
	assert.Equal(t, "x", got)
}

// TestLeadingZeroAndUnderscoreNumbers pins item 2: a leading-zero decimal
// integer is illegal (CUE: "illegal integer number"), and a malformed
// underscore (trailing, doubled, or adjacent to a non-digit) is illegal
// ("illegal '_' in number"). A leading-zero FLOAT (`01.5`, `0e5`) is legal.
func TestLeadingZeroAndUnderscoreNumbers(t *testing.T) {
	reject := []string{
		`a: 010`,   // octal-looking decimal, was silently 8
		`a: 00`,    // leading zero
		`a: 0123`,  // leading zero
		`a: 08`,    // leading zero
		`a: 1_`,    // trailing underscore
		`a: 1__2`,  // doubled underscore
		`a: 0x12_`, // trailing underscore in a hex literal
		`a: 1_.5`,  // underscore before the fraction dot
		`a: 1e5_`,  // trailing underscore in exponent
	}
	for _, src := range reject {
		_, err := parse(t, src)
		assert.Error(t, err, "CUE rejects %q", src)
	}

	accept := map[string]int64{
		`a: 0`:     0,
		`a: 10`:    10,
		`a: 1_000`: 1000,
		`a: 1_2_3`: 123,
		`a: 0x10`:  16,
		`a: 0o17`:  15,
		`a: 0b101`: 5,
		`a: 0x1_0`: 16,
		`a: 0x_1`:  1, // a `_` right after the base prefix is legal in CUE
	}
	for src, want := range accept {
		f, err := parse(t, src)
		require.NoErrorf(t, err, "CUE accepts %q", src)
		lit := f.Decls[0].(*Field).Value.(*BasicLit)
		require.Equal(t, INT, lit.Kind, "%q is an int", src)
		assert.Equal(t, want, mustInt(t, lit.Value), "value of %q", src)
	}

	// A leading-zero float is legal in CUE.
	for _, src := range []string{`a: 01.5`, `a: 0e5`, `a: 00.5`} {
		f, err := parse(t, src)
		require.NoErrorf(t, err, "CUE accepts float %q", src)
		assert.Equal(t, FLOAT, f.Decls[0].(*Field).Value.(*BasicLit).Kind, "%q is a float", src)
	}
}

// mustInt decodes an int BasicLit value the way compileBasicLit does (base-10,
// underscores stripped) so the test asserts the numeric value, not just accept.
func mustInt(t *testing.T, raw string) int64 {
	t.Helper()
	v, err := ParseIntLiteral(raw)
	require.NoError(t, err)
	return v
}

// TestMissingDeclSeparator pins item 3: two declarations on one line with no
// comma or newline between them is a parse error (CUE: "missing ',' in struct
// literal"). The run-together field typo `a: 1 b: 2` must not silently parse
// as two fields.
func TestMissingDeclSeparator(t *testing.T) {
	reject := []string{
		`a: 1 b: 2`,      // missing comma between two fields, one line
		`{a: 1 b: 2}`,    // same inside a struct literal
		`a: 1 2`,         // field then embed, no separator
		"a: 1 b: 2\nc:3", // first line still run-together
	}
	for _, src := range reject {
		_, err := parse(t, src)
		assert.Errorf(t, err, "run-together decls must error: %q", src)
	}

	// Top-level field pairs: two file declarations.
	for _, src := range []string{
		"a: 1\nb: 2",  // newline separator
		`a: 1, b: 2`,  // comma separator
		"a: 1,\nb: 2", // both
	} {
		f, err := parse(t, src)
		require.NoErrorf(t, err, "separated decls must parse: %q", src)
		assert.Len(t, f.Decls, 2, "%q has two decls", strings.ReplaceAll(src, "\n", "\\n"))
	}

	// Brace-wrapped pairs: one embedded StructLit with two element decls.
	for _, src := range []string{`{a: 1, b: 2}`, "{a: 1\nb: 2}"} {
		f, err := parse(t, src)
		require.NoErrorf(t, err, "separated struct decls must parse: %q", src)
		st := f.Decls[0].(*EmbedDecl).Expr.(*StructLit)
		assert.Len(t, st.Elts, 2, "%q struct has two decls", strings.ReplaceAll(src, "\n", "\\n"))
	}
}

// TestStrayQuestionMark pins item 4: a label followed by `?` but not `?:` is a
// parse error, not a silently discarded token. CUE rejects `id ?`.
func TestStrayQuestionMark(t *testing.T) {
	_, err := parse(t, `id ?`)
	assert.Error(t, err, "a stray ? after a label must error")

	_, err = parse(t, `id ? x`)
	assert.Error(t, err, "a stray ? not forming ?: must error")
}

// TestMultilineStrictness pins item 5: an under-indented interior line is an
// error (not a silent no-op TrimPrefix), and the closing `"""` must follow a
// newline.
func TestMultilineStrictness(t *testing.T) {
	// Under-indented interior line: the closing line is indented two spaces but
	// the content line "x" has none. The literal parses (the scanner captures it
	// raw) but Unquote — the decode the compiler runs — must reject it.
	_, err := Unquote("\"\"\"\n  ok\nx\n  \"\"\"")
	assert.Error(t, err, "under-indented interior line must error on decode")

	// The closing `"""` must follow a newline; content on the closing line is an
	// error.
	_, err = Unquote("\"\"\"\n  ok\"\"\"")
	assert.Error(t, err, "closing quote must follow a newline")

	// A well-formed multiline string decodes to its dedented body.
	good := "a: \"\"\"\n  one\n  two\n  \"\"\""
	f, err := parse(t, good)
	require.NoError(t, err)
	got, err := Unquote(f.Decls[0].(*Field).Value.(*BasicLit).Value)
	require.NoError(t, err)
	assert.Equal(t, "one\ntwo", got)

	// A blank interior line needs no indent prefix.
	got, err = Unquote("\"\"\"\n  one\n\n  two\n  \"\"\"")
	require.NoError(t, err)
	assert.Equal(t, "one\n\ntwo", got)
}
