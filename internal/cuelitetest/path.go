package cuelitetest

// path.go — differential harness for surface D: ParsePath.
//
// This file adds a path-comparing arm to the cuelitetest harness. Surface
// D (placeholder paths) uses only ParsePath; the schema/data arms in the
// main harness do not cover it because a path-only case has no schema or
// data document, and appending it to corpus() would agree vacuously —
// empty schema and data classify identically in both arms regardless of
// the path expression.
//
// So surface D has its own:
//   - PathCase — one path expression to parse.
//   - PathOutcome — accepted-with-segments or rejected.
//   - PathPath — a parse strategy (in-house or oracle).
//   - RunPath — the CI-visible runner that compares both arms.
//
// The oracle arm calls cuelang.org/go/cue.ParsePath directly and
// reconstructs the string-label-only contract cuelite.Path implements:
// every selector is classified by its label type, and only a string label
// yields a segment. An index, definition, hidden, or any other non-string
// selector maps to the documented contract rejection, so the two arms
// compare the SAME contract — "CUE paths restricted to string labels" —
// rather than the oracle silently accepting a selector the in-house parser
// cannot represent.

import (
	"fmt"
	"slices"
	"testing"

	"cuelang.org/go/cue"

	cuelitepkg "github.com/jeduden/mdsmith/cue/cuelite"
)

// PathCase is one differential-test input: a CUE path expression to
// parse. Name labels the case in failure messages.
type PathCase struct {
	Name string
	Expr string
}

// PathOutcome is the result of parsing a PathCase through one path arm.
// Accepted reports whether the expression parsed successfully. When
// Accepted is true, Segments holds the unquoted per-selector strings
// the parser produced; when false, Segments is nil. Comparing both
// Accepted and Segments ensures the two arms agree not only on
// accept/reject but on the exact decoded content they produce.
type PathOutcome struct {
	Accepted bool
	Segments []string
	// PanicNote records the recovered message when the oracle arm's
	// cue.ParsePath panics (it is empty on the in-house arm and on a clean
	// oracle parse). Equal ignores it — it is diagnostic only — but it keeps
	// the recover SCOPED: if a future CUE upgrade panics on a new input
	// class, the note surfaces it instead of silently degrading every panic
	// to a bare rejection. TestSafeParsePath_panicFamily pins that the only
	// currently-known panicking family is the double-dot one.
	PanicNote string
}

// Equal reports whether two PathOutcomes agree — the same accept/reject
// decision AND the same segment list. A nil Segments equals an empty
// Segments, consistent with how Path.Segments() behaves for a zero Path.
func (o PathOutcome) Equal(other PathOutcome) bool {
	if o.Accepted != other.Accepted {
		return false
	}
	return slices.Equal(o.Segments, other.Segments)
}

// PathPath is a path-parse strategy: it parses a PathCase and returns
// a PathOutcome. The in-house arm and the oracle arm are both PathPaths,
// so RunPath can call either uniformly.
type PathPath func(c PathCase) PathOutcome

// CueLitePathParsePath parses a PathCase through the cue/cuelite
// façade — the in-house path. ParsePath is now the pure-Go in-house
// implementation; this function is the stable binding that RunPath calls.
// A successful parse always yields at least one segment (ParsePath rejects
// zero-segment expressions), so Segments is always non-nil on acceptance.
func CueLitePathParsePath(c PathCase) PathOutcome {
	p, err := cuelitepkg.ParsePath(c.Expr)
	if err != nil {
		return PathOutcome{Accepted: false}
	}
	return PathOutcome{Accepted: true, Segments: p.Segments()}
}

// OraclePathParsePath parses a PathCase directly through
// cuelang.org/go/cue — the oracle the in-house path is measured against.
// It implements the SAME string-label-only contract cuelite.ParsePath
// documents, so the comparison is meaningful: both arms reject anything
// CUE would represent as an index, definition, or hidden selector.
//
// It mirrors the in-house path stage for stage:
//
//   - An empty expression is rejected (the in-house path's explicit check).
//   - cue.ParsePath itself nil-derefs on a few malformed inputs in
//     cuelang v0.16.1 (for example "a..."); safeParsePath wraps the call
//     with a documented recover that maps such a panic to a rejection,
//     matching the in-house parser, which rejects the same inputs.
//   - A cue.ParsePath error is a rejection.
//   - Each selector is classified by selectorSegment: a string label
//     yields its unquoted value; an index, definition, hidden, or any
//     other non-string selector is the documented contract rejection. An
//     empty unquoted string (a string label whose escapes decode to "",
//     such as the lone-surrogate "\ud800" or the Go-only "\x41") is also a
//     rejection, matching the in-house parser's empty-segment check.
//
// Note: cue.ParsePath returns zero selectors only for the empty string,
// which is caught by the empty-expression guard above. For any non-empty
// expression that passes the error check, at least one selector is
// present.
func OraclePathParsePath(c PathCase) PathOutcome {
	if c.Expr == "" {
		return PathOutcome{Accepted: false}
	}
	p, panicNote := safeParsePath(c.Expr)
	if panicNote != "" {
		return PathOutcome{Accepted: false, PanicNote: panicNote}
	}
	if p.Err() != nil {
		return PathOutcome{Accepted: false}
	}
	sels := p.Selectors()
	segs := make([]string, len(sels))
	for i, s := range sels {
		seg, ok := selectorSegment(s)
		if !ok {
			return PathOutcome{Accepted: false}
		}
		segs[i] = seg
	}
	return PathOutcome{Accepted: true, Segments: segs}
}

// selectorSegment classifies a CUE selector against the string-label-only
// contract. A string label (LabelType StringLabel) yields its unquoted
// value and ok=true, unless that value is empty (escapes that decode to
// "" are rejected, like the in-house parser's empty-segment check). Any
// other selector — an index label, a definition label, a hidden label, or
// the implicit-root/special kinds — is outside the contract and yields
// ok=false, so the oracle rejects exactly the selectors cuelite.ParsePath
// is documented to reject. No panic recovery is needed: Unquoted is called
// only after LabelType confirms a string label, so it never panics.
func selectorSegment(s cue.Selector) (string, bool) {
	if s.LabelType() != cue.StringLabel {
		return "", false
	}
	u := s.Unquoted()
	if u == "" {
		return "", false
	}
	return u, true
}

// safeParsePath calls cue.ParsePath and recovers from the panic
// cuelang.org/go v0.16.1 raises inside the parser on a few malformed
// inputs (a trailing "..." such as "a..." nil-derefs deep in the upstream
// expression parser). It returns the parsed path and, when a panic
// occurred, the recovered message (empty otherwise) — so OraclePathParsePath
// maps the panic to a rejection that CARRIES the panic note. Capturing the
// note keeps the recover scoped: a CUE upgrade that panics on a new,
// actually-valid input would otherwise silently degrade to a bare
// rejection and hide the divergence, whereas the note makes it inspectable
// (and TestSafeParsePath_panicFamily pins the known family).
func safeParsePath(expr string) (p cue.Path, panicNote string) {
	defer func() {
		if r := recover(); r != nil {
			panicNote = fmt.Sprintf("oracle panic: %v", r)
		}
	}()
	return cue.ParsePath(expr), ""
}

// ComparePathOutcomes runs one PathCase through both inHouse and oracle
// and reports a failure on t when the two PathOutcomes disagree. It
// returns true when they agree. The test name includes the expression so
// a failure message names the disagreeing input.
func ComparePathOutcomes(t testing.TB, inHouse, oracle PathPath, c PathCase) bool {
	t.Helper()
	got := inHouse(c)
	want := oracle(c)
	if got.Equal(want) {
		return true
	}
	t.Errorf("path case %q expr=%q: in-house %+v disagrees with oracle %+v",
		c.Name, c.Expr, got, want)
	return false
}

// RunPath compares every PathCase in cases through the in-house and
// oracle path arms, reporting each disagreement on t. It is the entry
// point a phase's differential path test calls over its corpus.
func RunPath(t testing.TB, cases []PathCase) {
	t.Helper()
	for _, c := range cases {
		ComparePathOutcomes(t, CueLitePathParsePath, OraclePathParsePath, c)
	}
}

// pathCorpus returns the representative set of path expressions for the
// differential path arm, one row per behaviour class of CUE's path
// grammar. These are the SAME inputs the in-house unit tests in
// cue/cuelite/path_test.go cover, exercised through two harnesses: the
// unit tests pin the in-house parser's output directly, while this corpus
// drives the differential arm (in-house vs cue.ParsePath), and every row
// also seeds FuzzParsePath so a regression in a known class fails before
// the fuzzer even mutates. Keeping the two lists aligned is deliberate —
// a divergence class only counts as covered when a row pins it in both
// places. It spans:
//
//   - accepted inputs: ASCII idents (with digits, underscores, mixed
//     case), unicode-letter idents, "$" idents, the non-literal keywords
//     if/for/let/in, dotted idents, keywords as non-leading selectors,
//     quoted keys (dots, escaped quotes, "\/", "\uXXXX", unicode,
//     numeric-looking), mixed ident+quoted, paired surrogate escapes that
//     combine into one astral rune, bracket string-index selectors
//     (a["b"]), multi-hash raw-string labels (#"b"#) — including an
//     escaped-quote-then-hash body the close scan must read past
//     (#"\#"#"#) — multiline string labels (head and bracket, plain and
//     raw, with indentation, escapes, surrogate pairs, and CRLF), and a
//     leading BOM;
//   - rejected inputs: the empty string, leading/trailing dots, an empty
//     quoted segment, a missing dot after a quoted segment, the literal
//     keywords true/false/null as the leading selector, hidden labels
//     (underscore-prefixed), definition labels (# prefix), index labels
//     (bare numeric, a[0]), Go-only escapes (\xNN, octal \NNN), a raw NUL
//     inside quotes, the upstream-parser-panic input "a...", whitespace
//     forms, unterminated quotes, invalid escape sequences, lone surrogate
//     halves, a raw-string label after a dot, a multiline string after a
//     dot, malformed multiline strings (bad opener/indent/close,
//     escaped final newline) whose CUE value is empty, and a non-offset-0
//     BOM.
//
//nolint:funlen // one table of corpus cases, one row per behaviour class.
func pathCorpus() []PathCase {
	return []PathCase{
		// Accepted: ASCII identifiers.
		{Name: "simple ident", Expr: "title"},
		{Name: "ident with digits", Expr: "abc123"},
		{Name: "ident with underscore", Expr: "a_b"},
		{Name: "ident trailing underscore", Expr: "a__b"},
		{Name: "upper-case ident", Expr: "Title"},
		{Name: "ident with dollar", Expr: "a$b"},

		// Accepted: unicode-letter and dollar idents.
		{Name: "unicode ident uber", Expr: "über"},
		{Name: "unicode ident cjk", Expr: "日本語"},
		{Name: "unicode digit continuation", Expr: "a٠"},
		{Name: "dollar-prefixed ident", Expr: "$foo"},
		{Name: "bare dollar", Expr: "$"},

		// Accepted: non-literal keywords as leading selectors.
		{Name: "keyword if", Expr: "if"},
		{Name: "keyword for", Expr: "for"},
		{Name: "keyword let", Expr: "let"},
		{Name: "keyword in", Expr: "in"},

		// Accepted: dotted identifiers.
		{Name: "dotted idents", Expr: "a.b.c"},
		{Name: "two-segment dotted", Expr: "params.subtitle"},
		{Name: "unicode dotted", Expr: "héllo.x"},

		// Accepted: literal keywords as non-leading selectors.
		{Name: "true as non-leading selector", Expr: "x.true"},
		{Name: "null as non-leading selector", Expr: "x.null"},
		{Name: "false as non-leading selector", Expr: "x.false"},

		// Accepted: quoted keys.
		{Name: "quoted key with hyphen", Expr: `"my-key"`},
		{Name: "quoted key with dot inside", Expr: `"a.b"`},
		{Name: "quoted key with escaped quote", Expr: `"a\"b"`},
		{Name: "quoted key with slash escape", Expr: `"a\/b"`},
		{Name: "quoted key with newline escape", Expr: `"a\nb"`},
		{Name: "quoted key with cr escape", Expr: `"a\rb"`},
		{Name: "quoted key with control escape", Expr: `"a\tb"`},
		{Name: "quoted key with big unicode escape", Expr: `"\U0001F600"`},
		{Name: "quoted key with unicode escape", Expr: `"\u0041"`},
		{Name: "quoted key with unicode", Expr: `"über"`},
		{Name: "numeric-looking quoted segment", Expr: `"123"`},
		{Name: "quoted key with space", Expr: `"a b"`},

		// Accepted: mixed ident and quoted.
		{Name: "quoted then ident", Expr: `"my-key".sub`},
		{Name: "ident then quoted", Expr: `params."a.b"`},

		// Accepted: whitespace trimmed around dots and segments.
		{Name: "leading space", Expr: " a"},
		{Name: "trailing space", Expr: "a "},
		{Name: "space before dot", Expr: "a .b"},
		{Name: "space after quoted before dot", Expr: `"a" .b`},
		{Name: "space after dot", Expr: `"a". "b"`},
		{Name: "spaces around dotted", Expr: " a.b "},
		{Name: "trailing newline", Expr: "a\n"},
		{Name: "leading newline", Expr: "\na"},
		{Name: "newline after dot", Expr: "a.\nb"},
		{Name: "trailing CRLF", Expr: "a\r\n"},
		{Name: "trailing line comment", Expr: "a//comment"},
		{Name: "line comment after dot", Expr: "a.//c\nb"},
		{Name: "line comment then trailing newline", Expr: "a//c\n"},

		// Rejected: empty expression.
		{Name: "empty string", Expr: ""},

		// Rejected: leading/trailing dot.
		{Name: "trailing dot", Expr: "a."},
		{Name: "leading dot", Expr: ".a"},
		{Name: "quoted trailing dot", Expr: `"a".`},

		// Rejected: empty quoted segment (both arms reject empty unquoted).
		{Name: "empty quoted segment", Expr: `""`},

		// Rejected: malformed — text after closing quote without dot.
		{Name: "missing dot after quoted", Expr: `"a"b`},

		// Rejected: literal keywords as the leading selector.
		{Name: "literal true", Expr: "true"},
		{Name: "literal false", Expr: "false"},
		{Name: "literal null", Expr: "null"},

		// Rejected: underscore-prefixed idents (CUE hidden labels).
		{Name: "underscore-prefixed ident", Expr: "_foo"},
		{Name: "double-underscore ident", Expr: "__"},
		{Name: "bare underscore", Expr: "_"},

		// Rejected: hash-prefixed idents (CUE definition labels).
		{Name: "hash-prefixed ident", Expr: "#foo"},

		// Rejected: bare numeric / index labels.
		{Name: "bare numeric segment", Expr: "123"},
		{Name: "bare zero", Expr: "0"},
		{Name: "index selector", Expr: "a[0]"},
		{Name: "digit-leading ident", Expr: "9a"},

		// Rejected: Go-only escapes inside quotes (CUE decodes to empty).
		{Name: "hex escape", Expr: `"\x41"`},
		{Name: "octal escape", Expr: `"\101"`},
		{Name: "unknown escape", Expr: `"\z"`},
		{Name: "invalid hex in unicode escape", Expr: `"\uZZZZ"`},
		{Name: "truncated unicode escape", Expr: `"\u12"`},
		{Name: "truncated big unicode escape", Expr: `"\U0001"`},
		{Name: "out-of-range big unicode escape", Expr: `"\U80000000"`},
		{Name: "lone surrogate escape", Expr: `"\ud800"`},

		// Rejected: raw NUL / control bytes inside quotes.
		{Name: "raw NUL in quotes", Expr: "\"a\x00b\""},
		{Name: "raw newline in quotes", Expr: "\"a\nb\""},
		{Name: "raw CR in quotes", Expr: "\"a\rb\""},

		// Rejected: invalid UTF-8 anywhere (quote, ident, comment).
		{Name: "invalid utf8 in quotes", Expr: "\"a\xfcb\""},
		{Name: "invalid utf8 in ident", Expr: "a\xfcb"},
		{Name: "invalid utf8 in comment", Expr: "a//\xe7"},

		// Rejected: upstream-parser-panic input.
		{Name: "triple-dot tail", Expr: "a..."},
		{Name: "double-dot", Expr: "a..b"},

		// Rejected: whitespace-only (both arms reject).
		{Name: "whitespace only", Expr: " "},

		// Rejected: whitespace mid-segment (both arms reject).
		{Name: "whitespace mid ident", Expr: "a b"},

		// Rejected: newline as a statement terminator before more content.
		{Name: "newline before dot", Expr: "a\n.b"},
		{Name: "newline between idents", Expr: "a\nb"},
		{Name: "vertical tab separator", Expr: "a\v.b"},
		{Name: "comment before dot", Expr: "a//c\n.b"},
		{Name: "block comment", Expr: "a/*c*/.b"},
		{Name: "single slash", Expr: "a/b"},
		{Name: "comment-only expression", Expr: "//c"},

		// Rejected: unterminated quoted segment.
		{Name: "unterminated quoted", Expr: `"unterminated`},
		// Rejected: a trailing lone backslash (no escape selector).
		{Name: "trailing backslash escape", Expr: `"a\`},

		// Accepted: paired surrogate escapes combine into one astral rune
		// (utf16.DecodeRune). CUE joins an adjacent high+low surrogate
		// escape pair regardless of \u vs \U form; a LONE half decodes to
		// "" and the empty-segment check rejects it (both arms agree).
		{Name: "raw astral rune in quotes", Expr: `"😀"`},
		{Name: "paired backslash-u surrogate escapes", Expr: `"\uD83D\uDE00"`},
		{Name: "paired backslash-U surrogate escapes", Expr: `"\U0000D800\U0000DC00"`},
		{Name: "mixed u-high U-low surrogate escapes", Expr: `"\uD83D\U0000DE00"`},
		{Name: "mixed U-high u-low surrogate escapes", Expr: `"\U0000D83D\uDE00"`},
		{Name: "lone high surrogate escape", Expr: `"\uD800"`},
		{Name: "lone low surrogate escape", Expr: `"\uDC00"`},
		{Name: "high surrogate then BMP escape", Expr: `"\uD83DA"`},
		{Name: "high surrogate then raw char", Expr: `"\uD83Dx"`},
		{Name: "low then high surrogate escapes", Expr: `"\uDC00\uD800"`},

		// Accepted: bracket string-index selectors. cue.ParsePath treats
		// ["<string>"] as a StringLabel — the same segment as a dotted
		// quoted selector — with space/tab/CR/newline tolerated before the
		// string but only space/tab/CR (no newline) before the ']'. A bare
		// numeric a[0] stays an IndexLabel rejection.
		{Name: "bracket string index", Expr: `a["b"]`},
		{Name: "chained bracket string index", Expr: `a["b"]["c"]`},
		{Name: "bracket index spaces inside", Expr: `a[ "b" ]`},
		{Name: "bracket index leading newline", Expr: "a[\n\"b\"]"},
		{Name: "bracket index tabs inside", Expr: "a[\t\"b\"\t]"},
		{Name: "bracket index then dot ident", Expr: `a["b"].c`},
		{Name: "dot ident then bracket index", Expr: `a.b["c"]`},
		{Name: "bracket numeric-looking string", Expr: `a["0"]`},
		{Name: "space before bracket index", Expr: `a ["b"]`},
		{Name: "quoted head then bracket index", Expr: `"b" ["c"]`},
		{Name: "bracket numeric is index label", Expr: `a[0]`},
		{Name: "bracket index trailing newline rejected", Expr: "a[\"b\"\n]"},
		{Name: "leading bracket index rejected", Expr: `["b"]`},
		{Name: "empty bracket rejected", Expr: `a[]`},
		{Name: "bracket two strings rejected", Expr: `a["b" "c"]`},
		{Name: "bracket bare ident rejected", Expr: `a[b]`},
		{Name: "bracket literal true rejected", Expr: `a[true]`},
		{Name: "bracket index with spaces is index label", Expr: `a[ 0 ]`},
		{Name: "unterminated bracket rejected", Expr: `a["b"`},

		// Accepted: multi-hash raw-string labels. A '#' before a '"' opens
		// a CUE raw string, NOT a definition label. cue.ParsePath accepts
		// #"..."# as a head selector and inside brackets — but NOT after a
		// dot (a.#"b"# rejects). Raw strings take #-counted delimiters and
		// \#-prefixed escapes; a bare #foo without a quote stays a
		// definition-selector rejection.
		{Name: "raw-string head label", Expr: `#"b"#`},
		{Name: "raw-string label in bracket", Expr: `a[#"b"#]`},
		{Name: "double-hash raw-string label", Expr: `##"b"##`},
		{Name: "many-hash raw-string label", Expr: `####"x"####`},
		{Name: "raw-string head then dot ident", Expr: `#"a"#.b`},
		{Name: "raw-string head then dot quoted", Expr: `#"a"#."b"`},
		{Name: "raw-string with dot inside", Expr: `#"a.b"#`},
		{Name: "raw-string literal backslash-n", Expr: `#"a\nb"#`},
		{Name: "raw-string escaped newline", Expr: "#\"a\\#nb\"#"},
		{Name: "raw-string escaped tab", Expr: "#\"tab\\#tend\"#"},
		{Name: "raw-string escaped quote", Expr: "#\"q\\#\"x\"#"},
		{Name: "raw-string embedded quotes", Expr: `#"say "hi""#`},
		{Name: "raw-string escaped unicode", Expr: "#\"\\#u0041\"#"},
		{Name: "raw-string astral rune", Expr: `#"😀"#`},
		{Name: "raw-string in bracket then dot", Expr: `a[#"b"#].c`},
		{Name: "empty raw-string label rejected", Expr: `#""#`},
		{Name: "raw-string after dot rejected", Expr: `a.#"b"#`},
		{Name: "raw-string unterminated one hash", Expr: `#"b"`},
		{Name: "raw-string unterminated more hashes", Expr: `##"a"#`},
		{Name: "raw-string unknown escape rejected", Expr: `#"a\##nb"#`},
		{Name: "raw-string truncated escape rejected", Expr: `#"a\#"#`},
		// Raw-string close scan is escape-aware: an escaped quote followed by a
		// hash run is NOT a terminator. CUE accepts these in head and bracket
		// positions and at deeper hash levels; a blind strings.Index close scan
		// rejected them all (the 240s fuzz minimized to `#"\#"#"#`).
		{Name: "raw-string escaped quote then hash", Expr: `#"\#"#"#`},
		{Name: "raw-string escaped quote mid body", Expr: `#"q\#"#x"#`},
		{Name: "double-hash raw escaped quote then hashes", Expr: `##"\##"##"##`},
		{Name: "raw escaped quote in bracket", Expr: `x[#"\#"#"#]`},
		// Multiline string labels ("""…/#"""…) are CUE string labels as the head
		// or a bracket operand, never after a dot. The opener must be followed by
		// a newline; the closing line's leading whitespace is the indentation
		// stripped from every content line; the final newline before the close is
		// excluded; escapes follow the same dialect as a single-line string. Any
		// malformed multiline whose CUE Unquoted() is "" is the empty-segment
		// rejection.
		{Name: "multiline basic", Expr: "\"\"\"\na\n\"\"\""},
		{Name: "multiline indented", Expr: "\"\"\"\n  a\n  \"\"\""},
		{Name: "multiline raw", Expr: "#\"\"\"\na\n\"\"\"#"},
		{Name: "multiline in bracket", Expr: "x[\"\"\"\na\n\"\"\"]"},
		{Name: "multiline then dot ident", Expr: "\"\"\"\na\n\"\"\".b"},
		{Name: "multiline escape decoded", Expr: "\"\"\"\na\\tb\n\"\"\""},
		{Name: "multiline surrogate pair", Expr: "\"\"\"\n\\uD83D\\uDE00\n\"\"\""},
		{Name: "multiline big-U escape", Expr: "\"\"\"\n\\U0001F600\n\"\"\""},
		{Name: "multiline mixed escapes", Expr: "\"\"\"\na\\nb\\\"c\\\\d\\/e\\tf\n\"\"\""},
		{Name: "multiline control escapes", Expr: "\"\"\"\na\\bb\\fc\\rd\\ve\\ag\n\"\"\""},
		{Name: "multiline raw escape decoded", Expr: "#\"\"\"\n  a\\#tb\n  \"\"\"#"},
		{Name: "multiline raw literal backslash-n", Expr: "#\"\"\"\n  a\\nb\n  \"\"\"#"},
		{Name: "multiline CRLF endings", Expr: "\"\"\"\r\n  a\r\n  \"\"\""},
		{Name: "multiline two content lines", Expr: "\"\"\"\n  a\n  b\n  \"\"\""},
		{Name: "multiline deeper-than-close indent", Expr: "\"\"\"\n    a\n  \"\"\""},
		{Name: "multiline blank line", Expr: "\"\"\"\n  a\n\n  b\n  \"\"\""},
		{Name: "multiline trailing blank before close", Expr: "\"\"\"\n  a\n\n  \"\"\""},
		{Name: "multiline tab indent", Expr: "\"\"\"\n\ta\n\t\"\"\""},
		{Name: "multiline opener not followed by newline rejected", Expr: "\"\"\"a\n\"\"\""},
		{Name: "multiline four-quote opener rejected", Expr: "\"\"\"\"\na\n\"\"\"\""},
		{Name: "multiline unterminated rejected", Expr: "\"\"\"\na\n"},
		{Name: "multiline indent mismatch rejected", Expr: "\"\"\"\n a\n  \"\"\""},
		{Name: "multiline content on closing line rejected", Expr: "\"\"\"\na\nb\"\"\""},
		{Name: "multiline empty body rejected", Expr: "\"\"\"\n\"\"\""},
		{Name: "multiline escaped last newline rejected", Expr: "\"\"\"\n  a\\\n  \"\"\""},
		{Name: "multiline lone surrogate rejected", Expr: "\"\"\"\n  \\uD800\n  \"\"\""},
		{Name: "multiline after dot rejected", Expr: "a.\"\"\"\nb\n\"\"\""},
		{Name: "multiline raw after dot rejected", Expr: "a.#\"\"\"\nb\n\"\"\"#"},
		{Name: "multiline nonws before close rejected", Expr: "\"\"\"\n  a\n  x\"\"\""},
		{Name: "multiline tab content space close rejected", Expr: "\"\"\"\n\ta\n  \"\"\""},
		{Name: "multiline later line bad indent rejected", Expr: "\"\"\"\n  a\n b\n  \"\"\""},
		// CR is stripped from a multiline token (CUE's scanner.stripCR): blank
		// CRLF lines and bare CRs decode as if absent.
		{Name: "multiline blank first line", Expr: "\"\"\"\n\n  b\n  \"\"\""},
		{Name: "multiline CRLF blank line", Expr: "\"\"\"\r\n\r\n  b\r\n  \"\"\""},
		{Name: "multiline bare CR in content", Expr: "\"\"\"\r\n  a\rb\r\n  \"\"\""},
		{Name: "multiline CRLF blank line mid", Expr: "\"\"\"\n  a\r\n\r\n  b\n  \"\"\""},
		// CR-family: CUE makes the opener-newline and escape decisions on the
		// RAW token (scanner.stripCR runs only AFTER scanning), so a CR run at
		// the opener, a CR between the backslash and the escape selector, and a
		// CR among \u hex digits are scan errors CUE rejects — NOT stripped-away
		// no-ops. The benign '\'+CR+'#' at level 1 stays accepted: the raw '\' is
		// a literal backslash (CR is not the '#' introducer), and after CR strips
		// the body is `\#n`, the level-1 newline escape (round-5 probe).
		{Name: "multiline CR run at opener rejected", Expr: "\"\"\"\r\r\n0\n\"\"\""},
		{Name: "multiline CR between backslash and selector rejected", Expr: "\"\"\"\n\\\rn\n\"\"\""},
		{Name: "multiline CR inside unicode hex digits rejected", Expr: "\"\"\"\n\\u00\r41\n\"\"\""},
		{Name: "multiline truncated unicode escape at close rejected", Expr: "\"\"\"\na\\u0\"\"\""},
		{Name: "multiline raw backslash-CR before hash introducer", Expr: "#\"\"\"\n  \\\r#n\n  \"\"\"#"},
		// Escaped-newline line continuation. A raw '\'+CR is a literal backslash
		// the scanner accepts; stripCR then fuses '\#'+newline into
		// literal.Unquote's escapedNewline, eliding the newline. A clean
		// '\#'+newline (no CR) is rejected by the scanner first, so this path is
		// reachable only via the CR fusion (round-5 fuzz find). An escaped FINAL
		// newline is errEscapedLastNewline → empty → rejected.
		{Name: "multiline escaped newline joins lines", Expr: "#\"\"\"\nx\\\r#\ny\\\r#\nz\n\"\"\"#"},
		{Name: "multiline escaped newline at start", Expr: "#\"\"\"\n\\\r#\n0\n\"\"\"#"},
		{Name: "multiline escaped newline keeps non-indent", Expr: "#\"\"\"\n\\\r#\n  \\\r#\n0\n\"\"\"#"},
		{Name: "multiline escaped last newline via CR rejected", Expr: "#\"\"\"\na\\\r#\n\"\"\"#"},
		{Name: "multiline clean hash-escaped newline rejected", Expr: "#\"\"\"\n\\#\n0\n\"\"\"#"},
		{Name: "multiline escaped newline bad next indent rejected", Expr: "#\"\"\"\n  a\\\r#\nx\n  \"\"\"#"},
		{Name: "multiline raw truncated escape rejected", Expr: "#\"\"\"\na\\#"},
		{Name: "raw-string trailing escape introducer rejected", Expr: "#\"a\\#"},
		// Raw-string surrogate pairing. At hash level N BOTH halves must carry
		// the '\#u…' introducer: a \#u high + \#u low pair combines into one
		// astral rune (accepted), while a \#u high followed by a PLAIN \u low
		// (literal text in a raw string) leaves the high lone — CUE decodes "",
		// rejected. A \#u high whose next byte is the closing delimiter's
		// backslash is also lone (the former out-of-bounds-panic input).
		{Name: "raw-string paired surrogate escapes", Expr: "#\"\\#uD800\\#uDC00\"#"},
		{Name: "double-hash raw-string paired surrogate escapes", Expr: "##\"\\##uD800\\##uDC00\"##"},
		{Name: "raw-string high then plain-u low rejected", Expr: "#\"\\#uD800\\uDC00\"#"},
		{Name: "double-hash raw-string high then plain-u low rejected", Expr: "##\"\\##uD800\\uDC00\"##"},
		{Name: "raw-string lone high before close rejected", Expr: "#\"\\#uD800\\\"#"},
		{Name: "multiline raw-string opener rejected", Expr: `##"""##`},
		{Name: "single-hash multiline raw opener rejected", Expr: `#"""#`},
		{Name: "raw newline in raw-string rejected", Expr: "#\"\n\"#"},
		{Name: "raw CR in raw-string rejected", Expr: "#\"\r\"#"},
		{Name: "bracket malformed quoted rejected", Expr: `a["\z"]`},
		{Name: "bracket malformed raw-string rejected", Expr: `a[#"a\##nb"#]`},
		{Name: "high surrogate then BMP escape rejected", Expr: `"\uD83DA"`},
		{Name: "high surrogate then non-unicode escape rejected", Expr: "\"\\uD83D\\n\""},
		{Name: "high surrogate then truncated escape rejected", Expr: `"\uD83D\u12"`},
		{Name: "bare hash ident is definition label", Expr: `#x`},

		// BOM handling: a UTF-8 BOM is skipped only at offset 0; anywhere
		// else (including inside quotes and comments) CUE rejects it with
		// 'illegal byte order mark'.
		{Name: "leading BOM skipped", Expr: "\ufeffa"},
		{Name: "interior BOM in ident rejected", Expr: "a\ufeffb"},
		{Name: "BOM inside quotes rejected", Expr: "\"a\ufeffb\""},
		{Name: "BOM-only quoted segment rejected", Expr: "\"\ufeff\""},
		{Name: "BOM in comment rejected", Expr: "a//\ufeff"},
		{Name: "trailing BOM rejected", Expr: "a\ufeff"},
	}
}
