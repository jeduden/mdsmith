package cuelite

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Path is a parsed CUE field path — an ordered sequence of unquoted
// string segments such as ["a", "b", "c"] for "a.b.c" or ["my-key",
// "sub"] for `"my-key".sub`. It is a value type: methods take and
// return Path by copy. A zero Path (no segments) is valid and its
// Segments() is nil.
//
// The segment representation is the UNQUOTED label string: a quoted
// segment like `"my-key"` yields the segment "my-key", stripping the
// quotes and applying CUE string escapes. This is the same shape
// fieldinterp uses for map look-ups, so a Path from ParsePath feeds
// directly into ResolvePath without any extra unquoting step.
//
// MakePath and ParsePath are deliberately ASYMMETRIC, mirroring
// cue.MakePath/cue.ParsePath: MakePath accepts segments ParsePath could
// never parse back — an empty segment, a "true"/"false"/"null" head, a
// segment with embedded dots ("a.b"), or one needing quotes ("my-key") —
// because programmatic construction from data keys (e.g. Fields()
// iteration) must round-trip any map key, while ParsePath enforces the
// narrower string-label-only EXPRESSION grammar. There is deliberately no
// Path.String()/render method: PathError.Error()'s dot-join of segments is
// LOSSY (["a.b"] and ["a","b"] both join to "a.b") and is display-only —
// never parse it back. A future Path renderer must quote any segment that
// is not a bare identifier.
type Path struct {
	segments []string
}

// Segments returns the unquoted per-selector strings that make up the
// path, or nil for an empty path. The returned slice is a fresh copy so
// a caller that mutates it cannot corrupt the Path's internal state.
func (p Path) Segments() []string {
	return slices.Clone(p.segments)
}

// MakePath constructs a Path from the given unquoted segments — the
// in-house equivalent of cue.MakePath. It is the constructor consumers
// that build paths programmatically (query.collectPaths) need once they
// migrate off cuelang.org/go.
//
// MakePath does not validate the segment values; each segment is stored
// as-is. A zero-argument call returns a Path with nil segments. By design
// MakePath accepts segments ParsePath cannot parse — an empty segment, a
// "true"/"false"/"null" head, a dotted key ("a.b"), or a key that would
// need quoting ("my-key") — because it builds paths from arbitrary data
// keys, not from the string-label EXPRESSION grammar ParsePath enforces.
// Construct paths from map keys this way (e.g. over Fields() iteration);
// never round-trip a data key through ParsePath.
func MakePath(segments ...string) Path {
	if len(segments) == 0 {
		return Path{}
	}
	return Path{segments: slices.Clone(segments)}
}

// ParsePath parses a CUE field-path expression into a Path whose
// Segments() are the unquoted per-selector strings. It is the in-house
// pure-Go implementation of the string-label subset of the CUE path
// grammar mdsmith uses.
//
// # Grammar contract
//
// ParsePath accepts the STRING-LABEL SUBSET of CUE paths: a HEAD selector
// followed by zero or more dot- or bracket-selectors, with optional
// surrounding whitespace. It matches cuelang.org/go/cue.ParsePath for every
// input in that subset (verified by the differential harness in
// internal/cuelitetest) and rejects the CUE selectors that are NOT string
// labels. A leading UTF-8 BOM is skipped (offset 0 only); a BOM anywhere
// else is rejected, matching CUE's "illegal byte order mark".
//
// Identifier selector. A bare identifier starts with a Unicode letter or
// "$" and continues with Unicode letters, Unicode digits, "_", or "$" —
// matching CUE's identifier class (so "über", "$foo", and "a$b" parse,
// while a "_"-leading token does not, being a CUE hidden label). The
// three CUE literals "true", "false", and "null" are rejected as the
// LEADING selector (CUE parses the head of a path as an expression, where
// those are boolean/null literals, not field names) but accepted as a
// later selector; the non-literal keywords "if", "for", "let", and "in"
// are ordinary identifiers everywhere.
//
// Quoted selector. A double-quoted string whose value is its
// escape-decoded content. CUE string escapes are accepted —
// \a \b \f \n \r \t \v \\ \" \/ \uXXXX \UXXXXXXXX — while the Go-only
// \xNN and octal \NNN escapes, an unknown escape, a raw NUL or newline
// inside the quotes, and an unterminated string are rejected. A
// high-surrogate \u/\U escape immediately followed by a low-surrogate
// escape combines into one astral rune (CUE's UTF-16 rule); a lone
// surrogate half is rejected. A quoted segment whose decoded value is empty
// (`""`, or escapes that decode to "") is rejected: an empty label is a
// surprising map key.
//
// Raw-string selector. A multi-hash raw string (#"..."#, ##"..."##, …) is a
// CUE string label, accepted as the HEAD selector or as a bracket operand
// but NOT after a dot (a.#"b"# rejects, matching CUE). Its delimiter is N
// '#' + '"' … '"' + N '#'; inside it the escape introducer is '\' + N '#',
// so a backslash not followed by N '#' is literal. A bare "#foo" (a '#' not
// opening a raw string) is a definition selector, not a string label, and
// is rejected.
//
// Bracket selector. A bracket string-index selector a["b"] (and a[#"b"#])
// is a CUE string label, the same segment as the dotted form a."b". CUE
// tolerates whitespace and a newline before the bracketed string but only
// space/tab/CR — not a newline — before the closing "]". A numeric index
// a[0] is an index selector, not a string label, and is rejected.
//
// Rejected non-string selectors. An index selector (a bare number such
// as "123", or "a[0]"), a definition selector ("#foo"), and a hidden
// selector ("_foo") are valid CUE paths but are NOT string labels, so the
// string-label-only contract rejects them with an error naming the
// selector kind. The hidden-label rejection is PARITY with CUE, which
// itself rejects "_foo" ("hidden label _foo not allowed"); the index- and
// definition-label rejections are the deliberate narrowing to string labels
// only. cuelite.Path is []string-backed by design; the phase-2 consumers
// (fieldinterp, query) need only string labels.
//
// ParsePath returns a plain error (never a *PathError): a path-expression
// syntax error has no data-tree field path to tag, so a *PathError with a
// nil path would add nothing. The error message names the offending input
// and, where it helps, the rejected selector kind.
func ParsePath(expr string) (Path, error) {
	if !utf8.ValidString(expr) {
		// CUE's lexer rejects invalid UTF-8 anywhere in the source — inside a
		// quoted label, an identifier, or even a comment — so reject it once
		// up front rather than letting a replacement rune slip through.
		return Path{}, fmt.Errorf("path expression is not valid UTF-8")
	}
	// CUE's scanner skips a UTF-8 BOM only at offset 0 and rejects one
	// anywhere else (inside a quoted label, an identifier, or a comment) with
	// "illegal byte order mark". Strip a leading BOM, then reject any
	// remaining BOM up front rather than per-position.
	expr = strings.TrimPrefix(expr, "\ufeff")
	if strings.Contains(expr, "\ufeff") {
		return Path{}, fmt.Errorf("path expression contains an illegal byte order mark")
	}
	if strings.IndexByte(expr, 0x00) >= 0 {
		// A NUL byte is "illegal character NUL" to CUE wherever it appears —
		// a quoted label, an identifier, or a comment — so reject it up front
		// rather than per-position.
		return Path{}, fmt.Errorf("path expression contains a NUL byte")
	}
	if strings.TrimSpace(expr) == "" {
		return Path{}, fmt.Errorf("path expression must not be empty")
	}
	segments, err := parsePathSegments(expr)
	if err != nil {
		return Path{}, err
	}
	return Path{segments: segments}, nil
}

// isPathSpace reports whether r is whitespace ParsePath skips between
// tokens. CUE's scanner treats space, tab, and carriage return as
// insignificant around path selectors and dots, but treats newline,
// vertical tab, and form feed as illegal there — so only these three are
// skipped and any other "space-like" rune falls through to the
// unexpected-character branch, matching cue.ParsePath.
func isPathSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r'
}

// sepKind names what follows a completed segment: the path ends, a "."
// dot-selector follows, or a "[" bracket string-index selector follows.
type sepKind int

const (
	sepEnd     sepKind = iota // end of path (EOF or a trailing terminator)
	sepDot                    // a "." introducing a dot-selector
	sepBracket                // a "[" introducing a bracket string-index
)

// parsePathSegments splits expr into unquoted segment strings. It is the
// scanner driving ParsePath. A path is a HEAD selector followed by zero or
// more dot- or bracket-selectors:
//
//   - the head and a bracketed selector accept an identifier, a
//     double-quoted string, OR a multi-hash raw string (#"..."#);
//   - a dot-selector accepts only an identifier or a double-quoted string —
//     a raw string after a dot is rejected, matching cue.ParsePath;
//   - a "[" opens a bracket string-index selector ("a[\"b\"]"), whose
//     operand must be a quoted or raw string (a bare number "a[0]" stays an
//     index-label rejection), with newline tolerated before the operand but
//     not before the closing "]".
//
// A leading dot, a trailing dot, an empty decoded value, a malformed quoted
// string, an unexpected character, the literal true/false/null as the
// leading selector, or a non-string selector (index/definition/hidden) each
// return an error.
func parsePathSegments(expr string) ([]string, error) {
	segments := make([]string, 0, 4)
	// The head selector allows a raw string; a post-dot selector does not.
	// consumeSegment skips leading filler (incl. newlines) itself.
	seg, pos, err := consumeSegment(expr, 0, true)
	if err != nil {
		return nil, err
	}
	segments = append(segments, seg)
	for {
		kind, after, err := consumeSeparator(expr, pos)
		if err != nil {
			return nil, err
		}
		switch kind {
		case sepEnd:
			return segments, nil
		case sepDot:
			seg, after, err = consumeSegment(expr, after, false)
		case sepBracket:
			seg, after, err = consumeBracketSegment(expr, after)
		}
		if err != nil {
			return nil, err
		}
		segments = append(segments, seg)
		pos = after
	}
}

// consumeSegment reads one head- or dot-selector starting at pos, skipping
// any leading filler (space/tab/CR, newline, and line comments — all
// insignificant where a segment is expected). leading reports whether this
// is the head selector, which is where true/false/null reject as CUE
// literals and where a raw-string selector (#"...") is accepted; a post-dot
// selector (leading=false) rejects a raw string, matching cue.ParsePath. It
// returns the decoded segment and the position just past it.
func consumeSegment(expr string, pos int, leading bool) (string, int, error) {
	pos = skipExprSpace(expr, pos)
	if pos == len(expr) {
		// Only filler reached the end. At the head this is a non-blank
		// expression that carries no selector at all (e.g. "//c", a comment
		// with no field) — name that, not a non-existent dot. After a '.' it
		// is a genuine trailing dot.
		if leading {
			return "", 0, fmt.Errorf(
				"invalid path expression %q: expression contains no selector", expr)
		}
		return "", 0, fmt.Errorf("invalid path expression %q: trailing dot", expr)
	}
	if isMultilineStart(expr, pos) {
		// A multiline string ("""…/#"""…) is a string label only as the head
		// or a bracket operand — never after a dot, mirroring a raw string and
		// matching cue.ParsePath ("expected selector, found STRING"). The
		// leading-only multiline opener is consumed here; a post-dot one falls
		// through to the '"' case, where parseQuotedSegment's scan rejects the
		// stray triple quote.
		if leading {
			return consumeMultilineSegment(expr, pos)
		}
		return "", 0, fmt.Errorf(
			"invalid path expression %q: a multiline string is not a valid "+
				"selector after a dot", expr)
	}
	if leading && isRawStringStart(expr, pos) {
		return consumeRawStringSegment(expr, pos)
	}
	r, _ := utf8.DecodeRuneInString(expr[pos:])
	switch {
	case r == '"':
		return consumeQuotedSegment(expr, pos)
	case isIdentStart(r):
		seg, advance := parseIdentSegment(expr, pos)
		if leading && isCUELiteral(seg) {
			return "", 0, fmt.Errorf(
				"invalid path expression %q: %q is a CUE literal, not a field name",
				expr, seg)
		}
		return seg, pos + advance, nil
	default:
		return "", 0, unexpectedSegmentError(expr, r, pos)
	}
}

// consumeBracketSegment reads the operand of a bracket string-index
// selector starting at pos (just past the "["). CUE tolerates a newline
// before the operand, so leading filler (incl. newlines and comments) is
// skipped; the operand must be a quoted or raw string (a bare number, bool,
// or ident is an index/non-string selector and rejects); then only
// space/tab/CR — not a newline — may precede the closing "]". It returns
// the decoded segment and the position just past the "]".
func consumeBracketSegment(expr string, pos int) (string, int, error) {
	pos = skipExprSpace(expr, pos)
	var seg string
	var err error
	switch {
	case pos < len(expr) && isMultilineStart(expr, pos):
		seg, pos, err = consumeMultilineSegment(expr, pos)
	case pos < len(expr) && isRawStringStart(expr, pos):
		seg, pos, err = consumeRawStringSegment(expr, pos)
	case pos < len(expr) && expr[pos] == '"':
		seg, pos, err = consumeQuotedSegment(expr, pos)
	default:
		// A bare number ("a[0]"), bool, ident, or "]" is an index or
		// non-string bracket selector; name the kind so the rejection reads
		// as a contract error.
		return "", 0, fmt.Errorf(
			"invalid path expression %q: index selectors are not supported; "+
				"cuelite paths are the string-label subset of CUE paths",
			expr)
	}
	if err != nil {
		return "", 0, err
	}
	pos = skipPathSpace(expr, pos) // no newline allowed before ']'
	if pos == len(expr) || expr[pos] != ']' {
		return "", 0, fmt.Errorf(
			"invalid path expression %q: expected ']' to close bracket selector",
			expr)
	}
	return seg, pos + 1, nil
}

// consumeQuotedSegment reads a double-quoted string segment at pos and
// rejects an empty decoded value (an empty label is a surprising map key).
func consumeQuotedSegment(expr string, pos int) (string, int, error) {
	seg, advance, err := parseQuotedSegment(expr, pos)
	if err != nil {
		return "", 0, fmt.Errorf("invalid path expression %q: %s", expr, err)
	}
	if seg == "" {
		return "", 0, fmt.Errorf("invalid path expression %q: empty quoted segment", expr)
	}
	return seg, pos + advance, nil
}

// consumeRawStringSegment reads a multi-hash raw-string label at pos
// (expr[pos]=='#', a '"' follows the run of '#') and rejects an empty
// decoded value, mirroring consumeQuotedSegment.
func consumeRawStringSegment(expr string, pos int) (string, int, error) {
	seg, advance, err := parseRawStringSegment(expr, pos)
	if err != nil {
		return "", 0, fmt.Errorf("invalid path expression %q: %s", expr, err)
	}
	if seg == "" {
		return "", 0, fmt.Errorf("invalid path expression %q: empty raw-string segment", expr)
	}
	return seg, pos + advance, nil
}

// unexpectedSegmentError builds the rejection for a rune that cannot start a
// segment: a non-string CUE selector (index/definition/hidden) names its
// kind, anything else is a bare unexpected-character message.
func unexpectedSegmentError(expr string, r rune, pos int) error {
	if kind := nonStringSelectorKind(r); kind != "" {
		return fmt.Errorf(
			"invalid path expression %q: %s selectors are not supported; "+
				"cuelite paths are the string-label subset of CUE paths",
			expr, kind)
	}
	return fmt.Errorf(
		"invalid path expression %q: unexpected character %q at position %d",
		expr, r, pos)
}

// consumeSeparator scans what follows a completed segment at pos and
// reports whether the path ends or a dot- or bracket-selector follows.
// After a segment only space/tab/CR are insignificant — a newline or a
// "//" line comment terminates the path expression (CUE's statement-newline
// rule), so more content after one is an error while a terminator followed
// only by trailing filler ends the path. The returned position is just past
// the consumed "." or "[" (or unchanged at sepEnd).
func consumeSeparator(expr string, pos int) (sepKind, int, error) {
	pos = skipPathSpace(expr, pos)
	if pos == len(expr) {
		return sepEnd, pos, nil
	}
	if expr[pos] == '\n' || isLineComment(expr, pos) {
		if rest := skipExprSpace(expr, pos); rest == len(expr) {
			return sepEnd, rest, nil // terminator plus trailing filler
		}
		return sepEnd, pos, fmt.Errorf(
			"invalid path expression %q: unexpected content after newline at position %d",
			expr, pos)
	}
	switch expr[pos] {
	case '.':
		return sepDot, pos + 1, nil
	case '[':
		return sepBracket, pos + 1, nil
	default:
		return sepEnd, pos, fmt.Errorf(
			"invalid path expression %q: expected '.' or end, got %q at position %d",
			expr, rune(expr[pos]), pos)
	}
}

// skipPathSpace advances pos past any run of intra-expression whitespace
// (space/tab/CR) and returns the new position. A newline is NOT skipped:
// in the after-segment position it terminates the path expression.
func skipPathSpace(expr string, pos int) int {
	for pos < len(expr) {
		r, size := utf8.DecodeRuneInString(expr[pos:])
		if !isPathSpace(r) {
			break
		}
		pos += size
	}
	return pos
}

// skipExprSpace advances pos past filler that is insignificant where a
// segment is expected — space/tab/CR, newline, and "//" line comments (CUE
// accepts a trailing or post-dot comment). A leading newline/comment or
// one right after a '.' is insignificant (CUE's continuation rule), and a
// trailing run of such filler ends the path; only a newline or comment
// BETWEEN a completed segment and more content is significant, which the
// after-segment branch handles separately.
func skipExprSpace(expr string, pos int) int {
	for pos < len(expr) {
		if isLineComment(expr, pos) {
			pos = skipLineComment(expr, pos)
			continue
		}
		r, size := utf8.DecodeRuneInString(expr[pos:])
		if !isPathSpace(r) && r != '\n' {
			break
		}
		pos += size
	}
	return pos
}

// isLineComment reports whether expr[pos:] begins a "//" line comment.
func isLineComment(expr string, pos int) bool {
	return pos+1 < len(expr) && expr[pos] == '/' && expr[pos+1] == '/'
}

// skipLineComment advances pos past a "//" line comment (expr[pos]=='/',
// expr[pos+1]=='/') up to but not including the terminating newline, or to
// end of input when the comment runs to EOF.
func skipLineComment(expr string, pos int) int {
	pos += 2
	for pos < len(expr) && expr[pos] != '\n' {
		pos++
	}
	return pos
}

// isCUELiteral reports whether seg is one of CUE's boolean/null literals.
// CUE parses the leading selector of a path as an expression, so these
// three reject there (they are literals, not field names); later
// selectors are label tokens where they are ordinary identifiers.
func isCUELiteral(seg string) bool {
	return seg == "true" || seg == "false" || seg == "null"
}

// nonStringSelectorKind names the CUE selector kind a leading rune
// introduces when it is a valid CUE selector but NOT a string label, or
// "" when the rune does not introduce a known non-string selector. A
// digit starts an index label (a bare number such as "123"); "[" starts
// an index selector ("a[0]"); "#" starts a definition label; "_" starts a
// hidden label. ParsePath rejects each with a message naming the kind, so
// a caller sees a clear contract error instead of a bare
// unexpected-character message.
func nonStringSelectorKind(r rune) string {
	switch {
	case r >= '0' && r <= '9', r == '[':
		return "index"
	case r == '#':
		return "definition"
	case r == '_':
		return "hidden"
	default:
		return ""
	}
}

// isIdentStart reports whether r is a valid first character for a CUE
// string-label identifier: a Unicode letter or "$". "_" is excluded — a
// "_"-leading token is a CUE hidden label, rejected by string-label-only
// paths — and so are digits, which start index labels.
func isIdentStart(r rune) bool {
	return r == '$' || isLetter(r)
}

// isIdentCont reports whether r may appear after the first character of a
// CUE string-label identifier: a letter, a digit, "_", or "$".
func isIdentCont(r rune) bool {
	return isIdentStart(r) || r == '_' || isDigit(r)
}

// isLetter reports whether r is a Unicode letter CUE accepts in an
// identifier. CUE's identifier letter class is unicode.IsLetter; ASCII
// letters are the common case, checked first to keep the hot path
// branch-cheap, with the full Unicode test behind it for the rest.
func isLetter(r rune) bool {
	if r < utf8.RuneSelf {
		return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
	}
	return unicode.IsLetter(r)
}

// isDigit reports whether r is a Unicode decimal digit CUE accepts in an
// identifier continuation. ASCII digits are the common case; the full
// Unicode test follows for the rest.
func isDigit(r rune) bool {
	if r < utf8.RuneSelf {
		return r >= '0' && r <= '9'
	}
	return unicode.IsDigit(r)
}

// parseIdentSegment reads an identifier segment starting at expr[pos]
// and returns the segment text and the number of bytes consumed.
// It assumes pos < len(expr) and expr[pos] satisfies isIdentStart.
func parseIdentSegment(expr string, pos int) (string, int) {
	start := pos
	_, size := utf8.DecodeRuneInString(expr[pos:])
	pos += size
	for pos < len(expr) {
		r, size := utf8.DecodeRuneInString(expr[pos:])
		if !isIdentCont(r) {
			break
		}
		pos += size
	}
	return expr[start:pos], pos - start
}

// parseQuotedSegment reads a double-quoted string starting at expr[pos]
// and returns the unquoted string, the number of bytes consumed (including
// both quotes), and any decoding error. It assumes pos < len(expr) and
// expr[pos] == '"'. It scans to the closing quote, honoring backslash
// escapes, then decodes the body with the CUE-compatible unquoter so the
// accepted escape set matches cue.ParsePath (\a \b \f \n \r \t \v \\ \"
// \/ \uXXXX \UXXXXXXXX) and the Go-only \xNN / octal \NNN escapes,
// unknown escapes, and raw NUL/newline are rejected.
func parseQuotedSegment(expr string, pos int) (string, int, error) {
	end := pos + 1 // skip opening '"'
	for end < len(expr) {
		c := expr[end]
		if c == '\\' {
			end += 2 // skip the escape and the next byte
			continue
		}
		if c == '"' {
			end++ // include closing '"'
			s, err := cueUnquote(expr[pos+1 : end-1])
			if err != nil {
				return "", 0, fmt.Errorf(
					"malformed quoted segment %s: %w", expr[pos:end], err)
			}
			return s, end - pos, nil
		}
		end++
	}
	return "", 0, fmt.Errorf(
		"unterminated quoted segment starting at position %d", pos)
}

// isRawStringStart reports whether a CUE raw-string label begins at
// expr[pos]: a run of one or more '#' immediately followed by a '"'. A '#'
// NOT followed (after its hash run) by a '"' is a definition label, not a
// raw string. The caller has already established expr[pos]=='#'-or-other;
// this confirms the '#'…'"' shape.
func isRawStringStart(expr string, pos int) bool {
	if pos >= len(expr) || expr[pos] != '#' {
		return false
	}
	i := pos
	for i < len(expr) && expr[i] == '#' {
		i++
	}
	return i < len(expr) && expr[i] == '"'
}

// isMultilineStart reports whether a CUE multiline string opener begins at
// expr[pos]: an optional run of '#' immediately followed by three quotes
// (`"""` or '#'×N + `"""`). A multiline string is a string label as the head
// selector or a bracket operand but not after a dot, mirroring a raw string.
func isMultilineStart(expr string, pos int) bool {
	i := pos
	for i < len(expr) && expr[i] == '#' {
		i++
	}
	return strings.HasPrefix(expr[i:], `"""`)
}

// consumeMultilineSegment reads a multiline string label at pos (a '#' run
// then `"""`) and rejects an empty decoded value, mirroring
// consumeQuotedSegment. A malformed multiline literal whose CUE Unquoted() is
// "" decodes to "" here, so the empty-segment check rejects it — the same
// outcome as the oracle.
func consumeMultilineSegment(expr string, pos int) (string, int, error) {
	hashes := 0
	for pos+hashes < len(expr) && expr[pos+hashes] == '#' {
		hashes++
	}
	seg, advance, err := parseMultilineSegment(expr, pos, hashes)
	if err != nil {
		return "", 0, fmt.Errorf("invalid path expression %q: %s", expr, err)
	}
	if seg == "" {
		return "", 0, fmt.Errorf("invalid path expression %q: empty multiline segment", expr)
	}
	return seg, pos + advance, nil
}

// parseRawStringSegment reads a multi-hash raw-string label starting at
// expr[pos] (a run of N '#' then '"') and returns the decoded string, the
// bytes consumed, and any error. A raw string is delimited by N '#' + '"'
// … '"' + N '#'; inside it, the escape introducer is '\' + N '#', so at
// level N a backslash NOT followed by exactly N '#' is a literal backslash,
// and '\' + N '#' + c decodes the escape c with the same SELECTOR set a
// regular quoted string uses (\n \t \" \uXXXX …) — but a surrogate PAIR must
// carry the '\#…' introducer on BOTH halves (the low half is '\#u…', not a
// plain '\u…'). This matches cue.ParsePath's raw-string label handling. An
// unterminated string or an unknown escape is rejected.
func parseRawStringSegment(expr string, pos int) (string, int, error) {
	hashes := 0
	for pos+hashes < len(expr) && expr[pos+hashes] == '#' {
		hashes++
	}
	// expr[pos+hashes] is '"' (guaranteed by isRawStringStart). A multiline
	// raw opener (#"""…) is routed to parseMultilineSegment before reaching
	// here, so the byte after the run is a single '"' opening a single-line
	// raw string.
	bodyStart := pos + hashes + 1
	closing := `"` + strings.Repeat("#", hashes)
	rel := rawStringCloseIndex(expr[bodyStart:], hashes, closing)
	if rel < 0 {
		return "", 0, fmt.Errorf(
			"unterminated raw-string segment starting at position %d", pos)
	}
	body := expr[bodyStart : bodyStart+rel]
	s, err := rawUnquote(body, hashes)
	if err != nil {
		return "", 0, err
	}
	end := bodyStart + rel + len(closing)
	return s, end - pos, nil
}

// rawStringCloseIndex returns the byte offset within body of the closing
// delimiter of a hash-level-N raw string, or -1 when none is found. It scans
// left-to-right, skipping each escape sequence ('\' + N '#' + one selector
// byte) so an escaped quote followed by a hash run cannot be mistaken for the
// close — the divergence a blind strings.Index has (CUE accepts `#"\#"#"#`,
// whose body is `\#"#`, decoding to `"#`, but a blind scan stops at the first
// `"#`). A backslash NOT followed by N '#' is literal and advances one byte.
// closing is the precomputed `"`+N'#' delimiter the caller already built.
func rawStringCloseIndex(body string, hashes int, closing string) int {
	hashRun := strings.Repeat("#", hashes)
	for i := 0; i < len(body); {
		if body[i] == '\\' && strings.HasPrefix(body[i+1:], hashRun) {
			// '\' + N '#' introduces an escape; skip it and its selector byte so
			// an escaped '"' run is not read as the close. A '\#…' run with no
			// selector byte left is a truncated escape rawUnquote will reject —
			// stop advancing past the end and let the close search miss it.
			if i+1+hashes >= len(body) {
				return -1
			}
			i += 2 + hashes
			continue
		}
		if strings.HasPrefix(body[i:], closing) {
			return i
		}
		i++
	}
	return -1
}
