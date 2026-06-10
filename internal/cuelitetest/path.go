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
	p, panicked := safeParsePath(c.Expr)
	if panicked || p.Err() != nil {
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
// expression parser). It returns the parsed path and whether a panic
// occurred, so OraclePathParsePath can map the panic to a rejection — the
// in-house parser rejects the same inputs, so both arms still agree.
func safeParsePath(expr string) (p cue.Path, panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
		}
	}()
	return cue.ParsePath(expr), false
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
// differential path arm. The cases are derived from CUE's path grammar —
// one per behaviour class — rather than from the in-house parser's test
// list, so the corpus stays an independent description of the contract.
// It spans:
//
//   - accepted inputs: ASCII idents (with digits, underscores, mixed
//     case), unicode-letter idents, "$" idents, the non-literal keywords
//     if/for/let/in, dotted idents, keywords as non-leading selectors,
//     quoted keys (dots, escaped quotes, "\/", "\uXXXX", unicode,
//     numeric-looking), and mixed ident+quoted;
//   - rejected inputs: the empty string, leading/trailing dots, an empty
//     quoted segment, a missing dot after a quoted segment, the literal
//     keywords true/false/null as the leading selector, hidden labels
//     (underscore-prefixed), definition labels (# prefix), index labels
//     (bare numeric, a[0]), Go-only escapes (\xNN, octal \NNN), a raw NUL
//     inside quotes, the upstream-parser-panic input "a...", whitespace
//     forms, unterminated quotes, and invalid escape sequences.
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
		{Name: "quoted key with unicode escape", Expr: `"A"`},
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

		// Rejected: invalid escape sequence (lone surrogate).
		{Name: "lone-surrogate escape", Expr: `"\ud800"`},
	}
}
