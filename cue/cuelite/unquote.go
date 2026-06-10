package cuelite

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// cueUnquote decodes the body of a double-quoted CUE label (the bytes
// BETWEEN the quotes, quotes already stripped) into its rune value,
// applying the CUE string-escape set. It is the path parser's replacement
// for strconv.Unquote, whose escape set is Go's and so both accepts
// Go-only escapes CUE rejects (\xNN, octal \NNN) and rejects the \/ escape
// CUE accepts.
//
// Accepted escapes (matching cue.ParsePath):
//
//	\a \b \f \n \r \t \v   the C-style control escapes
//	\\ \" \/               backslash, double quote, forward slash
//	\uXXXX                 a 4-hex-digit BMP code point
//	\UXXXXXXXX             an 8-hex-digit code point
//
// Rejected (each an error, so the segment is rejected): the Go-only \xNN
// hex byte escape and octal \NNN escape, any other unknown escape (\z), a
// surrogate or out-of-range \u/\U code point, a truncated \u/\U escape, a
// trailing lone backslash, and a raw newline byte inside the string. CUE
// accepts most other raw control bytes verbatim, so cueUnquote passes them
// through unchanged; CR (U+000D) is the one CUE handles erratically (it
// decodes to an empty string), which the empty-segment check in ParsePath
// rejects regardless, so cueUnquote rejects a raw CR here too and both arms
// agree on rejection. A raw NUL never reaches cueUnquote — ParsePath
// rejects any expression containing one before scanning — and invalid
// UTF-8 is likewise pre-rejected, so the body here is valid UTF-8 with no
// NUL.
func cueUnquote(body string) (string, error) {
	var b strings.Builder
	b.Grow(len(body))
	for i := 0; i < len(body); {
		c := body[i]
		switch {
		case c == '\n':
			return "", fmt.Errorf("illegal newline in string")
		case c == '\r':
			return "", fmt.Errorf("illegal carriage return in string")
		case c == '\\':
			r, width, err := decodeEscape(body[i:])
			if err != nil {
				return "", err
			}
			b.WriteRune(r)
			i += width
		default:
			// Copy the next full UTF-8 rune verbatim. A raw control byte
			// other than NUL/newline/CR is accepted, matching CUE. The body
			// is already known to be valid UTF-8 (ParsePath validates the
			// whole expression up front), so no per-rune validity check is
			// needed here.
			_, size := utf8.DecodeRuneInString(body[i:])
			b.WriteString(body[i : i+size])
			i += size
		}
	}
	return b.String(), nil
}

// decodeEscape decodes a single backslash escape at the start of s (s[0]
// is '\\') and returns the decoded rune, the number of bytes consumed
// (including the backslash), and any error. It implements exactly CUE's
// escape set; an unsupported or malformed escape is an error so the
// quoted segment is rejected.
//
// Precondition: len(s) >= 2, i.e. the backslash is never the last byte of
// the body. parseQuotedSegment guarantees this — on a backslash it skips
// two bytes, so a trailing backslash would consume the closing quote and
// leave the string unterminated (rejected before cueUnquote runs), and the
// closing quote is therefore never preceded by an unescaped backslash.
func decodeEscape(s string) (rune, int, error) {
	switch s[1] {
	case 'a':
		return '\a', 2, nil
	case 'b':
		return '\b', 2, nil
	case 'f':
		return '\f', 2, nil
	case 'n':
		return '\n', 2, nil
	case 'r':
		return '\r', 2, nil
	case 't':
		return '\t', 2, nil
	case 'v':
		return '\v', 2, nil
	case '\\':
		return '\\', 2, nil
	case '"':
		return '"', 2, nil
	case '/':
		return '/', 2, nil
	case 'u':
		return decodeUnicodeEscape(s, 4)
	case 'U':
		return decodeUnicodeEscape(s, 8)
	default:
		return 0, 0, fmt.Errorf("unknown escape sequence \\%c", s[1])
	}
}

// decodeUnicodeEscape decodes a \u (n==4) or \U (n==8) escape at the start
// of s (s[0]=='\\', s[1]=='u' or 'U') into its code point. It requires
// exactly n following hex digits and rejects a surrogate-half or
// out-of-range code point, matching CUE — which represents such an escape
// as an invalid/empty value that the empty-segment check then rejects.
func decodeUnicodeEscape(s string, n int) (rune, int, error) {
	if len(s) < 2+n {
		return 0, 0, fmt.Errorf("truncated \\%c escape", s[1])
	}
	var v rune
	for j := 0; j < n; j++ {
		d, ok := hexVal(s[2+j])
		if !ok {
			return 0, 0, fmt.Errorf("invalid hex digit %q in \\%c escape", s[2+j], s[1])
		}
		v = v<<4 | rune(d)
	}
	if v > utf8.MaxRune || (0xD800 <= v && v < 0xE000) {
		return 0, 0, fmt.Errorf("invalid code point U+%04X in \\%c escape", v, s[1])
	}
	return v, 2 + n, nil
}

// hexVal returns the value of a single hex digit and whether c was one.
func hexVal(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10, true
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10, true
	default:
		return 0, false
	}
}
