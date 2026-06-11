package syntax

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parse is a test helper: parse a source string into a File.
func parse(t *testing.T, src string) (*File, error) {
	t.Helper()
	return ParseFile(src)
}

// TestParseFile_basicShapes parses the schema and row shapes the consumers feed
// the frontend, asserting the tree shape so a regression in the parser surfaces
// here independent of the evaluators.
func TestParseFile_basicShapes(t *testing.T) {
	f, err := parse(t, `status: string`)
	require.NoError(t, err)
	require.Len(t, f.Decls, 1)
	fld := f.Decls[0].(*Field)
	assert.Equal(t, "status", fld.Label.(*Ident).Name)
	assert.Equal(t, NoToken, fld.Constraint)
	assert.IsType(t, &Ident{}, fld.Value)

	// An optional field carries the OPTION constraint.
	f, err = parse(t, `note?: string`)
	require.NoError(t, err)
	assert.Equal(t, OPTION, f.Decls[0].(*Field).Constraint)

	// A single embedded expression parses to one EmbedDecl.
	f, err = parse(t, `close({a: int})`)
	require.NoError(t, err)
	require.Len(t, f.Decls, 1)
	assert.IsType(t, &EmbedDecl{}, f.Decls[0])

	// A nested-field shorthand desugars to a single-field struct.
	f, err = parse(t, `meta: status: "x"`)
	require.NoError(t, err)
	outer := f.Decls[0].(*Field)
	st := outer.Value.(*StructLit)
	require.Len(t, st.Elts, 1)
	assert.Equal(t, "status", st.Elts[0].(*Field).Label.(*Ident).Name)
}

// TestParseFile_operatorsAndPrecedence checks the binary precedence: `a & b | c`
// parses as `a & b` bound tighter than `|`.
func TestParseFile_operatorsAndPrecedence(t *testing.T) {
	f, err := parse(t, `x: int & >=0 | string`)
	require.NoError(t, err)
	top := f.Decls[0].(*Field).Value.(*BinaryExpr)
	assert.Equal(t, OR, top.Op, "| is the lowest-precedence top operator")
	assert.Equal(t, AND, top.X.(*BinaryExpr).Op, "& binds tighter than |")
}

// TestParseFile_postfixChain parses a selector/index/call postfix chain.
func TestParseFile_postfixChain(t *testing.T) {
	f, err := parse(t, `x: strings.Join(a.b[0], ",")`)
	require.NoError(t, err)
	call := f.Decls[0].(*Field).Value.(*CallExpr)
	sel := call.Fun.(*SelectorExpr)
	assert.Equal(t, "Join", sel.Sel.(*Ident).Name)
	idx := call.Args[0].(*IndexExpr)
	assert.IsType(t, &SelectorExpr{}, idx.X)
}

// TestParseFile_comprehensions parses if/for and a multi-clause comprehension
// (the latter the evaluator rejects, but the parser must build it).
func TestParseFile_comprehensions(t *testing.T) {
	f, err := parse(t, `x: [if c {a}]`)
	require.NoError(t, err)
	list := f.Decls[0].(*Field).Value.(*ListLit)
	comp := list.Elts[0].(*Comprehension)
	require.Len(t, comp.Clauses, 1)
	assert.IsType(t, &IfClause{}, comp.Clauses[0])

	// for with a key variable (two-variable form).
	f, err = parse(t, `x: [for k, v in xs {v}]`)
	require.NoError(t, err)
	fc := f.Decls[0].(*Field).Value.(*ListLit).Elts[0].(*Comprehension).Clauses[0].(*ForClause)
	require.NotNil(t, fc.Key)
	assert.Equal(t, "k", fc.Key.Name)
	assert.Equal(t, "v", fc.Value.Name)

	// A multi-clause comprehension parses (the evaluator rejects it later).
	f, err = parse(t, `x: [for v in xs if v != "" {v}]`)
	require.NoError(t, err)
	comp = f.Decls[0].(*Field).Value.(*ListLit).Elts[0].(*Comprehension)
	assert.Len(t, comp.Clauses, 2)

	// A let clause parses into a multi-clause comprehension.
	f, err = parse(t, `x: [for v in xs let y = v {y}]`)
	require.NoError(t, err)
	comp = f.Decls[0].(*Field).Value.(*ListLit).Elts[0].(*Comprehension)
	require.Len(t, comp.Clauses, 2)
	assert.IsType(t, &LetClause{}, comp.Clauses[1])
}

// TestParseFile_errors drives the parser/scanner error branches.
func TestParseFile_errors(t *testing.T) {
	bad := []string{
		`x: int &`,              // dangling binary operand
		`x: (int`,               // unterminated paren
		`x: [1, 2`,              // unterminated list
		`{a: int`,               // unterminated struct
		`x: a.`,                 // selector with no member
		`x: a[1`,                // unterminated index
		`x: strings.Join(a`,     // unterminated call
		`x: @attr`,              // unexpected character (attribute)
		`x: /* c */ 1`,          // block comment is unexpected
		`x: "unterminated`,      // unterminated string
		`for`,                   // comprehension with no clause body
		`x: [for v xs {v}]`,     // for clause missing 'in'
		`x: [for {v}]`,          // for clause missing variable
		`x: [for v, in xs {v}]`, // for clause missing value variable
		`x: =`,                  // bare assign in value position
	}
	for _, src := range bad {
		_, err := parse(t, src)
		assert.Error(t, err, "source %q must fail to parse", src)
	}
}

// TestScanner_invalidInput rejects non-UTF-8 and NUL-bearing source.
func TestScanner_invalidInput(t *testing.T) {
	_, err := ParseFile("x: \"\xff\xfe\"")
	require.Error(t, err, "invalid UTF-8 is rejected")
	_, err = ParseFile("x: 1\x00")
	require.Error(t, err, "a NUL byte is rejected")
}

// TestScanner_numbers covers the int/float lexer: bases, underscores, floats
// with fraction and exponent.
func TestScanner_numbers(t *testing.T) {
	for _, src := range []string{
		`x: 0xFF`, `x: 0o17`, `x: 0b1010`, `x: 1_000`,
		`x: 1.5`, `x: 1e3`, `x: 1.5e-2`, `x: 1E+2`,
	} {
		_, err := parse(t, src)
		assert.NoError(t, err, "number source %q must parse", src)
	}
}

// TestScanner_comments skips a line comment.
func TestScanner_comments(t *testing.T) {
	f, err := parse(t, "x: 1 // a comment\ny: 2")
	require.NoError(t, err)
	assert.Len(t, f.Decls, 2)
}

// TestUnquote_dialects decodes the three string dialects and a bytes literal.
func TestUnquote_dialects(t *testing.T) {
	cases := map[string]string{
		`"abc"`:                      "abc",
		`"a\nb"`:                     "a\nb",
		`"aAb"`:                      "aAb",
		`#"raw\nstring"#`:            `raw\nstring`,
		"\"\"\"\n  a\n  b\n  \"\"\"": "a\nb",
	}
	for raw, want := range cases {
		got, err := Unquote(raw)
		require.NoError(t, err, "raw %q", raw)
		assert.Equal(t, want, got, "raw %q", raw)
	}
	// A bytes literal is detected.
	assert.True(t, IsBytesLiteral(`'bytes'`))
	assert.True(t, IsBytesLiteral(`#'raw bytes'#`))
	assert.False(t, IsBytesLiteral(`"string"`))
}

// TestUnquote_errors drives Unquote's error branches.
func TestUnquote_errors(t *testing.T) {
	for _, raw := range []string{
		``,         // empty
		`#`,        // hashes with no quote
		`xyz`,      // not a quote
		`"a`,       // unterminated
		`"\x41"`,   // Go-only hex escape CUE rejects
		`"\z"`,     // unknown escape
		"\"a\nb\"", // raw newline in a single-line string
	} {
		_, err := Unquote(raw)
		assert.Error(t, err, "raw %q must fail to unquote", raw)
	}
}

// TestInterpolation_dialects parses interpolations across the dialects so the
// scanner's fragment decode and the parser's interleave are exercised.
func TestInterpolation_dialects(t *testing.T) {
	cases := []struct {
		src   string
		frags []string // decoded even-index fragments
	}{
		{`x: "a\(v)b"`, []string{"a", "b"}},
		{`x: "\(v)"`, []string{"", ""}},
		{`x: "a\(v)b\(w)c"`, []string{"a", "b", "c"}},
		{`x: #"a\#(v)b"#`, []string{"a", "b"}},
	}
	for _, c := range cases {
		f, err := parse(t, c.src)
		require.NoError(t, err, "src %q", c.src)
		interp := f.Decls[0].(*Field).Value.(*Interpolation)
		var got []string
		for i, e := range interp.Elts {
			if i%2 == 0 {
				got = append(got, e.(*BasicLit).Value)
			}
		}
		assert.Equal(t, c.frags, got, "src %q", c.src)
	}
	// A bytes interpolation sets IsBytes.
	f, err := parse(t, `x: 'a\(v)b'`)
	require.NoError(t, err)
	assert.True(t, f.Decls[0].(*Field).Value.(*Interpolation).IsBytes)
}

// TestInterpolation_errors drives malformed interpolations.
func TestInterpolation_errors(t *testing.T) {
	for _, src := range []string{
		`x: "a\(v"`,   // unterminated interpolation expression
		`x: "a\(v)b`,  // unterminated string after interpolation
		`x: "a\(+)b"`, // bad embedded expression
	} {
		_, err := parse(t, src)
		assert.Error(t, err, "src %q must fail", src)
	}
}

// TestWalkChildren_allShapes calls WalkChildren over every node type so the
// descent's arms are covered, and exercises the marker methods by building one
// of each node.
func TestWalkChildren_allShapes(t *testing.T) {
	count := func(n Node) int {
		c := 0
		WalkChildren(n, func(Node) { c++ })
		return c
	}
	id := &Ident{Name: "a"}
	bl := &BasicLit{Kind: INT, Value: "1"}
	assert.Equal(t, 0, count(id), "an ident has no children")
	assert.Equal(t, 0, count(bl))
	assert.Equal(t, 2, count(&BinaryExpr{X: id, Op: AND, Y: id}))
	assert.Equal(t, 1, count(&UnaryExpr{Op: SUB, X: id}))
	assert.Equal(t, 1, count(&ParenExpr{X: id}))
	assert.Equal(t, 2, count(&SelectorExpr{X: id, Sel: id}))
	assert.Equal(t, 2, count(&IndexExpr{X: id, Index: id}))
	assert.Equal(t, 2, count(&CallExpr{Fun: id, Args: []Expr{id}}))
	assert.Equal(t, 1, count(&StructLit{Elts: []Decl{&EmbedDecl{Expr: id}}}))
	assert.Equal(t, 1, count(&ListLit{Elts: []Expr{id}}))
	assert.Equal(t, 2, count(&Field{Label: id, Value: id}))
	assert.Equal(t, 1, count(&EmbedDecl{Expr: id}))
	assert.Equal(t, 0, count(&Ellipsis{}), "an ellipsis with no type has no child")
	assert.Equal(t, 1, count(&Ellipsis{Type: id}))
	assert.Equal(t, 2, count(&Comprehension{Clauses: []Clause{&IfClause{Condition: id}}, Value: id}))
	assert.Equal(t, 1, count(&IfClause{Condition: id}))
	assert.Equal(t, 2, count(&LetClause{Ident: id, Expr: id}))
	assert.Equal(t, 2, count(&ForClause{Value: id, Source: id}))
	assert.Equal(t, 3, count(&ForClause{Key: id, Value: id, Source: id}))
	assert.Equal(t, 3, count(&Interpolation{Elts: []Expr{bl, id, bl}}))
	assert.Equal(t, 1, count(&File{Decls: []Decl{&EmbedDecl{Expr: id}}}))
	// A nil child (a UnaryExpr with no operand) is passed through.
	assert.Equal(t, 1, count(&UnaryExpr{Op: SUB}))
	// The marker methods are no-ops; call them so the coverage tool records them.
	exerciseMarkers()
}

// exerciseMarkers calls every interface-tag method so the coverage tool counts
// the otherwise-unreachable no-op bodies. The tags exist only to make the node
// types implement Node/Expr/Decl/Label/Clause.
func exerciseMarkers() {
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

// TestToken_String renders every operator and literal token to its spelling.
func TestToken_String(t *testing.T) {
	for tok, want := range map[Token]string{
		OR: "|", AND: "&", ADD: "+", SUB: "-", MUL: "*", NOT: "!",
		EQL: "==", NEQ: "!=", LSS: "<", GTR: ">", LEQ: "<=", GEQ: ">=",
		MAT: "=~", NMAT: "!~", OPTION: "?", STRING: "string-literal",
		INT: "int-literal", FLOAT: "float-literal", TRUE: "true",
		FALSE: "false", NULL: "null",
	} {
		assert.Equal(t, want, tok.String(), "token %d", int(tok))
	}
	assert.True(t, strings.HasPrefix(NoToken.String(), "token("), "an unspelled token renders as token(N)")
}
