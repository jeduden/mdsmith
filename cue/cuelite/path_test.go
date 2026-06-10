package cuelite_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/cue/cuelite"
)

// TestParsePath_accepted covers inputs ParsePath accepts and the
// segments it produces.
//
//nolint:funlen // table-driven accept cases, one row per grammar class.
func TestParsePath_accepted(t *testing.T) {
	cases := []struct {
		name string
		expr string
		want []string
	}{
		{"simple ident", "title", []string{"title"}},
		{"dotted idents", "a.b.c", []string{"a", "b", "c"}},
		{"ident with digits", "abc123", []string{"abc123"}},
		{"ident with underscore continuation", "a_b", []string{"a_b"}},
		{"trailing underscore continuation", "a__b", []string{"a__b"}},
		{"dollar-prefixed ident", "$foo", []string{"$foo"}},
		{"dollar in continuation", "a$b", []string{"a$b"}},
		{"bare dollar", "$", []string{"$"}},
		{"unicode letter ident", "über", []string{"über"}},
		{"cjk ident", "日本語", []string{"日本語"}},
		{"unicode dotted", "héllo.x", []string{"héllo", "x"}},
		{"non-literal keyword if", "if", []string{"if"}},
		{"non-literal keyword for", "for", []string{"for"}},
		{"non-literal keyword let", "let", []string{"let"}},
		{"non-literal keyword in", "in", []string{"in"}},
		{"keyword as later selector", "x.if", []string{"x", "if"}},
		{"true as later selector", "x.true", []string{"x", "true"}},
		{"null as later selector", "x.null", []string{"x", "null"}},
		{"false as later selector", "x.false", []string{"x", "false"}},
		{"single quoted key", `"my-key"`, []string{"my-key"}},
		{"quoted key then ident", `"my-key".sub`, []string{"my-key", "sub"}},
		{"ident then quoted key", `params."a.b"`, []string{"params", "a.b"}},
		{"quoted key with dot inside", `"a.b"`, []string{"a.b"}},
		{"quoted key with escaped quote", `"a\"b"`, []string{`a"b`}},
		{"quoted key with slash escape", `"a\/b"`, []string{"a/b"}},
		{"quoted key with control escapes", `"a\tb"`, []string{"a\tb"}},
		{"quoted key with bell escape", `"a\ab"`, []string{"a\ab"}},
		{"quoted key with backspace escape", `"a\bb"`, []string{"a\bb"}},
		{"quoted key with formfeed escape", `"a\fb"`, []string{"a\fb"}},
		{"quoted key with vtab escape", `"a\vb"`, []string{"a\vb"}},
		{"quoted key with newline escape", `"a\nb"`, []string{"a\nb"}},
		{"quoted key with cr escape", `"a\rb"`, []string{"a\rb"}},
		{"quoted key with backslash escape", `"a\\b"`, []string{`a\b`}},
		{"quoted key with lower-hex unicode escape", `"\u00ff"`, []string{"ÿ"}},
		{"quoted key with upper-hex unicode escape", `"\u00FF"`, []string{"ÿ"}},
		{"quoted key with unicode escape", `"\u0041"`, []string{"A"}},
		{"quoted key with big unicode escape", `"\U0001F600"`, []string{"😀"}},
		{"numeric-looking quoted segment", `"123"`, []string{"123"}},
		{"quoted key with unicode", `"über"`, []string{"über"}},
		{"quoted key with space", `"a b"`, []string{"a b"}},
		{"leading whitespace", " a", []string{"a"}},
		{"trailing whitespace", "a ", []string{"a"}},
		{"space before dot", "a .b", []string{"a", "b"}},
		{"space after quoted before dot", `"a" .b`, []string{"a", "b"}},
		{"space after dot", `"a". "b"`, []string{"a", "b"}},
		{"spaces around dotted", " a.b ", []string{"a", "b"}},
		{"tab whitespace", "\ta", []string{"a"}},
		{"unicode-digit continuation", "a٠", []string{"a٠"}},
		{"trailing newline", "a\n", []string{"a"}},
		{"leading newline", "\na", []string{"a"}},
		{"newline after dot", "a.\nb", []string{"a", "b"}},
		{"trailing CRLF", "a\r\n", []string{"a"}},
		{"trailing line comment", "a//comment", []string{"a"}},
		{"line comment after dot", "a.//c\nb", []string{"a", "b"}},
		{"line comment then trailing newline", "a//c\n", []string{"a"}},
		// Paired surrogate escapes combine into one astral rune.
		{"paired backslash-u surrogate escapes", `"\uD83D\uDE00"`, []string{"😀"}},
		{"paired backslash-U surrogate escapes", `"\U0000D800\U0000DC00"`, []string{"𐀀"}},
		{"mixed surrogate escape forms", `"\uD83D\U0000DE00"`, []string{"😀"}},
		// Bracket string-index selectors yield string labels.
		{"bracket string index", `a["b"]`, []string{"a", "b"}},
		{"chained bracket string index", `a["b"]["c"]`, []string{"a", "b", "c"}},
		{"bracket index with inner spaces", `a[ "b" ]`, []string{"a", "b"}},
		{"bracket index leading newline", "a[\n\"b\"]", []string{"a", "b"}},
		{"bracket index then dot", `a["b"].c`, []string{"a", "b", "c"}},
		{"dot then bracket index", `a.b["c"]`, []string{"a", "b", "c"}},
		{"bracket numeric-looking string", `a["0"]`, []string{"a", "0"}},
		{"space before bracket index", `a ["b"]`, []string{"a", "b"}},
		// Multi-hash raw-string labels (head and bracket positions).
		{"raw-string head label", `#"b"#`, []string{"b"}},
		{"double-hash raw-string label", `##"b"##`, []string{"b"}},
		{"raw-string label in bracket", `a[#"b"#]`, []string{"a", "b"}},
		{"raw-string head then dot ident", `#"a"#.b`, []string{"a", "b"}},
		{"raw-string with dot inside", `#"a.b"#`, []string{"a.b"}},
		{"raw-string literal backslash-n", `#"a\nb"#`, []string{`a\nb`}},
		{"raw-string escaped newline", "#\"a\\#nb\"#", []string{"a\nb"}},
		{"raw-string embedded quotes", `#"say "hi""#`, []string{`say "hi"`}},
		// Raw-string surrogate pairing: at hash level N, BOTH halves must use
		// the hash-level escape introducer (\#u…). A valid \#u high + \#u low
		// pair combines into one astral rune, matching cue.ParsePath.
		{"raw-string paired surrogate escapes", "#\"\\#uD800\\#uDC00\"#", []string{"𐀀"}},
		{"double-hash raw-string paired surrogate escapes", "##\"\\##uD800\\##uDC00\"##", []string{"𐀀"}},
		// A leading BOM is skipped (offset 0 only).
		{"leading BOM skipped", "\ufeffa", []string{"a"}},
		// Raw-string close scan is escape-aware: an escaped quote followed by a
		// hash run is NOT the close. CUE decodes `#"\#"#"#` to `"#` (body `\#"#`,
		// the `\#"` escapes a literal '"', then a literal '#').
		{"raw-string escaped quote then hash", `#"\#"#"#`, []string{`"#`}},
		{"raw-string escaped quote mid body", `#"q\#"#x"#`, []string{`q"#x`}},
		{"double-hash raw escaped quote then hashes", `##"\##"##"##`, []string{`"##`}},
		{"raw escaped quote in bracket", `x[#"\#"#"#]`, []string{"x", `"#`}},
		// Multiline string labels. The opener `"""` (or a '#' run + `"""`) must be
		// followed by a newline; the closing line's leading whitespace is the
		// indentation stripped from every content line; the final newline before
		// the close is excluded.
		{"multiline basic", "\"\"\"\na\n\"\"\"", []string{"a"}},
		{"multiline indented", "\"\"\"\n  a\n  \"\"\"", []string{"a"}},
		{"multiline raw", "#\"\"\"\na\n\"\"\"#", []string{"a"}},
		{"multiline in bracket", "x[\"\"\"\na\n\"\"\"]", []string{"x", "a"}},
		{"multiline then dot ident", "\"\"\"\na\n\"\"\".b", []string{"a", "b"}},
		{"multiline escape decoded", "\"\"\"\na\\tb\n\"\"\"", []string{"a\tb"}},
		{"multiline surrogate pair", "\"\"\"\n\\uD83D\\uDE00\n\"\"\"", []string{"\U0001F600"}},
		{"multiline big-U escape", "\"\"\"\n\\U0001F600\n\"\"\"", []string{"\U0001F600"}},
		// Cover every non-unicode escape selector the raw escape scan accepts
		// (\n \" \\ \/ \t and \b \f \r \v \a), each agreeing with cue.ParsePath.
		{"multiline mixed escapes", "\"\"\"\na\\nb\\\"c\\\\d\\/e\\tf\n\"\"\"", []string{"a\nb\"c\\d/e\tf"}},
		{"multiline control escapes", "\"\"\"\na\\bb\\fc\\rd\\ve\\ag\n\"\"\"", []string{"a\bb\fc\rd\ve\ag"}},
		{"multiline raw escape decoded", "#\"\"\"\n  a\\#tb\n  \"\"\"#", []string{"a\tb"}},
		{"multiline raw literal backslash-n", "#\"\"\"\n  a\\nb\n  \"\"\"#", []string{`a\nb`}},
		{"multiline CRLF endings", "\"\"\"\r\n  a\r\n  \"\"\"", []string{"a"}},
		{"multiline two content lines", "\"\"\"\n  a\n  b\n  \"\"\"", []string{"a\nb"}},
		{"multiline deeper-than-close indent", "\"\"\"\n    a\n  \"\"\"", []string{"  a"}},
		{"multiline blank line", "\"\"\"\n  a\n\n  b\n  \"\"\"", []string{"a\n\nb"}},
		{"multiline trailing blank before close", "\"\"\"\n  a\n\n  \"\"\"", []string{"a\n"}},
		{"multiline tab indent", "\"\"\"\n\ta\n\t\"\"\"", []string{"a"}},
		// CR is stripped from a multiline token (CUE's scanner.stripCR): a blank
		// CRLF line, a bare CR mid-content, and a CRLF blank line in the middle
		// all decode as if the CR were never there.
		{"multiline blank first line", "\"\"\"\n\n  b\n  \"\"\"", []string{"\nb"}},
		{"multiline CRLF blank line", "\"\"\"\r\n\r\n  b\r\n  \"\"\"", []string{"\nb"}},
		{"multiline bare CR in content", "\"\"\"\r\n  a\rb\r\n  \"\"\"", []string{"ab"}},
		{"multiline CRLF blank line mid", "\"\"\"\n  a\r\n\r\n  b\n  \"\"\"", []string{"a\n\nb"}},
		// The opener-newline and escape decisions run on the RAW (CR-bearing)
		// token, but value assembly is CR-stripped: at hash level 1 a '\'+CR is a
		// literal backslash (CR not the '#' introducer), so the raw scan accepts;
		// after CR is stripped the body is `\#n`, the level-1 escape for a real
		// newline. Both arms agree (round-5 CR-family probe).
		{"multiline raw backslash-CR before hash introducer", "#\"\"\"\n  \\\r#n\n  \"\"\"#", []string{"\n"}},
		// Escaped-newline line continuation. CUE's scanner accepts a raw '\'+CR
		// as a literal backslash, and stripCR then fuses '\#' with the following
		// newline into literal.Unquote's escapedNewline — eliding the newline and
		// joining lines. (A clean '\#'+newline is rejected by the scanner first,
		// so this path is reachable only via the CR fusion — round-5 fuzz find.)
		{"multiline escaped newline joins lines", "#\"\"\"\nx\\\r#\ny\\\r#\nz\n\"\"\"#", []string{"xyz"}},
		{"multiline escaped newline at start", "#\"\"\"\n\\\r#\n0\n\"\"\"#", []string{"0"}},
		{"multiline escaped newline keeps non-indent", "#\"\"\"\n\\\r#\n  \\\r#\n0\n\"\"\"#", []string{"  0"}},
		// Mirror rows that the corpus pins but the unit table had not (item 5).
		{"many-hash raw-string label", `####"x"####`, []string{"x"}},
		{"raw-string escaped quote", "#\"q\\#\"x\"#", []string{`q"x`}},
		{"raw-string escaped unicode", "#\"\\#u0041\"#", []string{"A"}},
		{"mixed U-high u-low surrogate escapes", `"\U0000D83D\uDE00"`, []string{"\U0001F600"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := cuelite.ParsePath(tc.expr)
			require.NoError(t, err)
			assert.Equal(t, tc.want, p.Segments())
		})
	}
}

// TestParsePath_rejected covers inputs ParsePath rejects. ParsePath
// returns a plain error (not a *PathError): a syntax error in a path
// EXPRESSION has no data-tree field path to tag.
//
//nolint:funlen // table-driven reject cases, one row per grammar class.
func TestParsePath_rejected(t *testing.T) {
	cases := []struct {
		name string
		expr string
	}{
		{"empty string", ""},
		{"whitespace only", " "},
		{"trailing dot", "a."},
		{"leading dot", ".a"},
		{"quoted trailing dot", `"a".`},
		{"empty quoted segment", `""`},
		{"malformed quoted segment", `"a"b`},
		{"unterminated quoted segment", `"unterminated`},
		{"lone-surrogate escape", `"\ud800"`},
		{"go-only hex escape", `"\x41"`},
		{"go-only octal escape", `"\101"`},
		{"unknown escape", `"\z"`},
		{"trailing backslash escape", `"a\`},
		{"truncated unicode escape", `"\u12"`},
		{"invalid hex digit in unicode escape", `"\uZZZZ"`},
		{"raw NUL in quotes", "\"a\x00b\""},
		{"literal true as head", "true"},
		{"literal false as head", "false"},
		{"literal null as head", "null"},
		{"underscore-prefixed ident", "_foo"},
		{"bare underscore", "_"},
		{"hash-prefixed ident", "#foo"},
		{"bare numeric ident", "123"},
		{"bare zero", "0"},
		{"index selector", "a[0]"},
		{"digit-leading ident", "9a"},
		{"whitespace mid ident", "a b"},
		{"triple-dot tail", "a..."},
		{"double-dot", "a..b"},
		{"invalid utf8 in ident", "a\xfcb"},
		{"invalid utf8 in quotes", "\"a\xfcb\""},
		{"newline between idents", "a\nb"},
		{"newline before dot", "a\n.b"},
		{"comment before content", "a//c\n.b"},
		{"comment-only expression", "//c"},
		{"truncated big unicode escape", `"\U0001"`},
		{"out-of-range big unicode escape", `"\U80000000"`},
		{"max-overflow big unicode escape", `"\UFFFFFFFF"`},
		{"raw newline in quotes", "\"a\nb\""},
		{"raw CR in quotes", "\"a\rb\""},
		{"lone high surrogate escape", `"\uD800"`},
		{"lone low surrogate escape", `"\uDC00"`},
		{"high surrogate then non-low", `"\uD83DA"`},
		{"low then high surrogate", `"\uDC00\uD800"`},
		{"raw-string after dot", `a.#"b"#`},
		{"empty raw-string label", `#""#`},
		{"unterminated raw-string", `#"b"`},
		{"raw-string unknown escape", `#"a\##nb"#`},
		{"bracket bare numeric index", `a[0]`},
		{"bracket bare ident", `a[b]`},
		{"bracket trailing newline", "a[\"b\"\n]"},
		{"leading bracket", `["b"]`},
		{"empty bracket", `a[]`},
		{"unterminated bracket", `a["b"`},
		{"interior BOM in ident", "a\ufeffb"},
		{"BOM inside quotes", "\"a\ufeffb\""},
		{"BOM in comment", "a//\ufeff"},
		{"bracket malformed quoted", `a["\z"]`},
		{"bracket malformed raw-string", `a[#"a\##nb"#]`},
		{"raw-string truncated escape", `#"a\#"#`},
		// Raw-string surrogate mis-pairing: a \#u high half followed by a
		// PLAIN \u low half leaves the high lone (the \u is literal text in a
		// raw string), so CUE decodes an empty string the empty-segment check
		// rejects. Both arms reject.
		{"raw-string high then plain-u low", "#\"\\#uD800\\uDC00\"#"},
		{"double-hash raw-string high then plain-u low", "##\"\\##uD800\\uDC00\"##"},
		// A \#u high half that is the last escape before the closing delimiter
		// (the byte after it is a backslash that opens the closing run) leaves
		// the high lone; CUE decodes empty and rejects. This is the former
		// out-of-bounds-panic input.
		{"raw-string lone high before close", "#\"\\#uD800\\\"#"},
		{"multiline raw-string opener", `##"""##`},
		{"single-hash multiline raw opener", `#"""#`},
		{"raw newline in raw-string", "#\"\n\"#"},
		{"raw CR in raw-string", "#\"\r\"#"},
		{"high surrogate then BMP escape", `"\uD83DA"`},
		{"high surrogate then non-unicode escape", "\"\\uD83D\\n\""},
		{"high surrogate then truncated escape", `"\uD83D\u12"`},
		// Multiline error / empty-decode cases — CUE's Unquoted() yields ""
		// for each, which the empty-segment check rejects.
		{"multiline opener not followed by newline", "\"\"\"a\n\"\"\""},
		{"multiline four-quote opener", "\"\"\"\"\na\n\"\"\"\""},
		{"multiline unterminated", "\"\"\"\na\n"},
		{"multiline indent mismatch", "\"\"\"\n a\n  \"\"\""},
		{"multiline content on closing line", "\"\"\"\na\nb\"\"\""},
		{"multiline empty body", "\"\"\"\n\"\"\""},
		{"multiline escaped last newline", "\"\"\"\n  a\\\n  \"\"\""},
		{"multiline lone surrogate", "\"\"\"\n  \\uD800\n  \"\"\""},
		{"multiline after dot rejected", "a.\"\"\"\nb\n\"\"\""},
		{"multiline raw after dot rejected", "a.#\"\"\"\nb\n\"\"\"#"},
		{"multiline nonws before close", "\"\"\"\n  a\n  x\"\"\""},
		{"multiline tab content space close", "\"\"\"\n\ta\n  \"\"\""},
		{"multiline later line bad indent", "\"\"\"\n  a\n b\n  \"\"\""},
		{"multiline raw truncated escape", "#\"\"\"\na\\#"},
		{"raw-string trailing escape introducer", "#\"a\\#"},
		// CR-family divergences (round 5): CUE makes the opener-newline and
		// escape decisions on the RAW token (scanner.stripCR runs only AFTER
		// scanning), so a CR run at the opener, a CR between the backslash and
		// the escape selector, or a CR among \u hex digits is a scan error CUE
		// rejects — not a stripped-away no-op. The in-house parser had accepted
		// all three because it stripped CR before lexing.
		{"multiline CR run at opener", "\"\"\"\r\r\n0\n\"\"\""},
		{"multiline CR between backslash and selector", "\"\"\"\n\\\rn\n\"\"\""},
		{"multiline CR inside unicode hex digits", "\"\"\"\n\\u00\r41\n\"\"\""},
		// A \u/\U whose fixed hex run reaches the closing delimiter mid-run is a
		// truncated escape CUE rejects (it reads the close '"' as an illegal hex
		// digit); the in-house raw escape scan rejects it as a short hex run.
		{"multiline truncated unicode escape at close", "\"\"\"\na\\u0\"\"\""},
		// An escaped FINAL newline (the line continuation lands right before the
		// close) is CUE's errEscapedLastNewline → empty Unquoted() → rejected.
		{"multiline escaped last newline via CR", "#\"\"\"\na\\\r#\n\"\"\"#"},
		// A clean '\#'+newline (no CR fusion) is rejected by CUE's SCANNER as an
		// unknown escape before literal.Unquote runs; the raw escape scan rejects
		// it the same way.
		{"multiline clean hash-escaped newline", "#\"\"\"\n\\#\n0\n\"\"\"#"},
		// An escaped newline followed by a line that does NOT carry the indent
		// prefix is CUE's skipWhitespaceAfterNewline error → empty → rejected.
		{"multiline escaped newline bad next indent", "#\"\"\"\n  a\\\r#\nx\n  \"\"\"#"},
		// Mirror rows that the corpus pins but the unit table had not (item 5).
		{"block comment separator", "a/*c*/.b"},
		{"vertical tab separator", "a\v.b"},
		{"bracket literal true", `a[true]`},
		{"bracket two strings", `a["b" "c"]`},
		{"trailing BOM", "a\ufeff"},
		{"BOM-only quoted segment", "\"\ufeff\""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := cuelite.ParsePath(tc.expr)
			require.Error(t, err)
			// ParsePath returns a plain error, never a *PathError.
			var pe *cuelite.PathError
			assert.NotErrorAs(t, err, &pe)
		})
	}
}

// TestParsePath_rejectedMessages pins that a rejected non-string selector
// names its kind, so a caller sees a clear contract error rather than a
// bare unexpected-character message.
func TestParsePath_rejectedMessages(t *testing.T) {
	t.Run("index selector names index", func(t *testing.T) {
		_, err := cuelite.ParsePath("123")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "index")
	})
	t.Run("definition selector names definition", func(t *testing.T) {
		_, err := cuelite.ParsePath("#foo")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "definition")
	})
	t.Run("hidden selector names hidden", func(t *testing.T) {
		_, err := cuelite.ParsePath("_foo")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "hidden")
	})
	t.Run("bracket index selector names index", func(t *testing.T) {
		_, err := cuelite.ParsePath("a[0]")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "index")
	})
	t.Run("literal head names CUE literal", func(t *testing.T) {
		_, err := cuelite.ParsePath("true")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "literal")
	})
	t.Run("comment-only expression names the missing selector", func(t *testing.T) {
		// "//c" is non-blank but carries no selector; the message must name the
		// real condition, not a non-existent "trailing dot".
		_, err := cuelite.ParsePath("//c")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no selector")
		assert.NotContains(t, err.Error(), "trailing dot")
	})
	t.Run("trailing dot still names trailing dot", func(t *testing.T) {
		_, err := cuelite.ParsePath("a.")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "trailing dot")
	})
}

// TestMakePath covers MakePath and round-trip through Segments.
func TestMakePath(t *testing.T) {
	t.Run("single segment", func(t *testing.T) {
		p := cuelite.MakePath("title")
		assert.Equal(t, []string{"title"}, p.Segments())
	})
	t.Run("multiple segments", func(t *testing.T) {
		p := cuelite.MakePath("a", "b", "c")
		assert.Equal(t, []string{"a", "b", "c"}, p.Segments())
	})
	t.Run("zero segments", func(t *testing.T) {
		p := cuelite.MakePath()
		assert.Nil(t, p.Segments())
	})
	t.Run("segments with hyphens", func(t *testing.T) {
		p := cuelite.MakePath("my-key", "sub")
		assert.Equal(t, []string{"my-key", "sub"}, p.Segments())
	})
}

// TestPath_Segments_returnsCopy ensures Segments returns a fresh copy so
// callers cannot corrupt the Path's internal state.
func TestPath_Segments_returnsCopy(t *testing.T) {
	p := cuelite.MakePath("a", "b", "c")
	got := p.Segments()
	require.Len(t, got, 3)
	got[0] = "MUTATED"
	// A second call must still see the original.
	assert.Equal(t, []string{"a", "b", "c"}, p.Segments())
}
