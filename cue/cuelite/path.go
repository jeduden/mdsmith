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
// as-is. A zero-argument call returns a Path with nil segments.
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
// ParsePath accepts the STRING-LABEL SUBSET of CUE paths: a
// dot-separated sequence of selectors, each either a bare identifier or a
// double-quoted string, with optional surrounding whitespace. It matches
// cuelang.org/go/cue.ParsePath for every input in that subset (verified
// by the differential harness in internal/cuelitetest) and rejects the
// CUE selectors that are NOT string labels.
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
// inside the quotes, and an unterminated string are rejected. A quoted
// segment whose decoded value is empty (`""`, or escapes that decode to
// "") is rejected: an empty label is a surprising map key.
//
// Rejected non-string selectors. An index selector (a bare number such
// as "123", or "a[0]"), a definition selector ("#foo"), and a hidden
// selector ("_foo") are valid CUE paths but are NOT string labels, so the
// string-label-only contract rejects them with an error naming the
// selector kind. cuelite.Path is []string-backed by design; the phase-2
// consumers (fieldinterp, query) need only string labels.
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

// parsePathSegments splits expr into unquoted segment strings.
// It is the scanner driving ParsePath. The two segment kinds are
// idents and quoted strings; they alternate with '.' separators, with
// optional space/tab/CR around each token and dot. A leading dot, a
// trailing dot, an empty quoted value, a malformed quoted string, an
// unexpected character, the literal true/false/null as the leading
// selector, or a non-string selector (index/definition/hidden) each
// return an error.
func parsePathSegments(expr string) ([]string, error) {
	segments := make([]string, 0, 4)
	pos := 0
	for {
		// Expecting a segment (the start, or just after a '.'): leading and
		// post-dot newlines are insignificant here, so skip them along with
		// space/tab/CR.
		pos = skipExprSpace(expr, pos)
		if pos == len(expr) {
			// Only whitespace followed a '.': a trailing dot.
			return nil, fmt.Errorf("invalid path expression %q: trailing dot", expr)
		}
		r, _ := utf8.DecodeRuneInString(expr[pos:])
		leading := len(segments) == 0
		switch {
		case r == '"':
			seg, advance, err := parseQuotedSegment(expr, pos)
			if err != nil {
				return nil, fmt.Errorf("invalid path expression %q: %s", expr, err)
			}
			if seg == "" {
				return nil, fmt.Errorf("invalid path expression %q: empty quoted segment", expr)
			}
			segments = append(segments, seg)
			pos += advance
		case isIdentStart(r):
			seg, advance := parseIdentSegment(expr, pos)
			if leading && isCUELiteral(seg) {
				return nil, fmt.Errorf(
					"invalid path expression %q: %q is a CUE literal, not a field name",
					expr, seg)
			}
			segments = append(segments, seg)
			pos += advance
		default:
			if kind := nonStringSelectorKind(r); kind != "" {
				return nil, fmt.Errorf(
					"invalid path expression %q: %s selectors are not supported; "+
						"cuelite paths are the string-label subset of CUE paths",
					expr, kind)
			}
			return nil, fmt.Errorf(
				"invalid path expression %q: unexpected character %q at position %d",
				expr, r, pos)
		}

		// After a segment, only space/tab/CR are insignificant — a newline
		// or a "//" line comment here terminates the path expression (CUE's
		// statement-newline rule), so a '.' after one is an error, while a
		// terminator followed only by trailing whitespace/comments ends the
		// path.
		pos = skipPathSpace(expr, pos)
		if pos == len(expr) {
			break
		}
		if expr[pos] == '\n' || isLineComment(expr, pos) {
			if rest := skipExprSpace(expr, pos); rest == len(expr) {
				break // a terminator plus trailing filler: the path ends here.
			}
			return nil, fmt.Errorf(
				"invalid path expression %q: unexpected content after newline at position %d",
				expr, pos)
		}
		if expr[pos] == '[' {
			// An index selector ("a[0]") follows a string label; name the
			// kind so the rejection reads as a contract error, not a bare
			// unexpected-character message.
			return nil, fmt.Errorf(
				"invalid path expression %q: index selectors are not supported; "+
					"cuelite paths are the string-label subset of CUE paths",
				expr)
		}
		if expr[pos] != '.' {
			return nil, fmt.Errorf(
				"invalid path expression %q: expected '.' or end, got %q at position %d",
				expr, rune(expr[pos]), pos)
		}
		pos++ // consume '.'
	}
	return segments, nil
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
