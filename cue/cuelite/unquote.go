package cuelite

import (
	"fmt"
	"strings"
	"unicode/utf16"
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
// Surrogate escapes follow CUE's UTF-16 rule: a high-surrogate \u/\U
// escape immediately followed by a low-surrogate \u/\U escape combines via
// utf16.DecodeRune into the single astral rune the pair encodes
// (`😀` → 😀), regardless of whether each half is written \u or
// \U. A LONE surrogate half — a high not followed by a low, or a bare low —
// is rejected, matching cue.ParsePath, which decodes such a label to the
// empty string the empty-segment check then rejects (both arms agree on
// rejection).
//
// Rejected (each an error, so the segment is rejected): the Go-only \xNN
// hex byte escape and octal \NNN escape, any other unknown escape (\z), a
// lone surrogate half or an out-of-range \u/\U code point, a truncated
// \u/\U escape, a trailing lone backslash, and a raw newline byte inside
// the string. CUE
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
		switch c {
		case '\n':
			return "", fmt.Errorf("illegal newline in string")
		case '\r':
			return "", fmt.Errorf("illegal carriage return in string")
		case '\\':
			r, width, err := decodeEscapeAt(body, i)
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

// decodeEscapeAt decodes the backslash escape at body[i] (body[i]=='\\')
// and returns the decoded rune, the number of body bytes consumed
// (including the backslash), and any error. It handles CUE's UTF-16
// surrogate-pairing rule: when a \u/\U escape decodes to a HIGH surrogate
// and an immediately-following \u/\U escape decodes to a LOW surrogate, the
// two combine into the single astral rune (utf16.DecodeRune) and the
// consumed width spans both escapes. A lone surrogate half is rejected,
// matching cue.ParsePath. Non-unicode escapes delegate to decodeEscape.
//
// Precondition: i < len(body) and body[i]=='\\'.
func decodeEscapeAt(body string, i int) (rune, int, error) {
	c := body[i+escBackslash : i+escBackslash+1]
	if c != "u" && c != "U" {
		r, w, err := decodeEscape(body[i:])
		return r, w, err
	}
	hi, hiWidth, err := decodeUnicodeCodePoint(body[i:])
	if err != nil {
		return 0, 0, err
	}
	if !utf16.IsSurrogate(rune(hi)) {
		return rune(hi), hiWidth, nil
	}
	// A high surrogate may pair with an immediately-following low-surrogate
	// escape. Anything else (a lone high, a bare low, or a high followed by
	// a non-low) is rejected.
	r, w, ok := combineSurrogate(body, i, hi, hiWidth)
	if !ok {
		return 0, 0, fmt.Errorf("lone surrogate U+%04X in \\u/\\U escape", hi)
	}
	return r, w, nil
}

// escBackslash is the one-byte width of the leading "\\" of an escape, used
// to index the escape's selector byte (body[i+escBackslash]).
const escBackslash = 1

// combineSurrogate tries to join the high surrogate hi (already decoded
// from body[i:], hiWidth bytes wide) with an immediately-following
// low-surrogate \u/\U escape. It returns the combined astral rune, the
// total width of both escapes, and ok=true on success; ok=false when no
// low-surrogate escape follows. utf16.DecodeRune yields the real rune for a
// valid pair, so combineSurrogate is reached only with a genuine high half.
func combineSurrogate(body string, i int, hi uint32, hiWidth int) (rune, int, bool) {
	next := i + hiWidth
	if next >= len(body) || body[next] != '\\' {
		return 0, 0, false
	}
	c := body[next+escBackslash : next+escBackslash+1]
	if c != "u" && c != "U" {
		return 0, 0, false
	}
	lo, loWidth, err := decodeUnicodeCodePoint(body[next:])
	if err != nil || !utf16.IsSurrogate(rune(lo)) {
		return 0, 0, false
	}
	combined := utf16.DecodeRune(rune(hi), rune(lo))
	if combined == utf8.RuneError {
		// hi was high and lo was a surrogate, but not a valid high+low pair
		// (e.g. low-then-high or high-then-high); reject as a lone half.
		return 0, 0, false
	}
	return combined, hiWidth + loWidth, true
}

// decodeEscape decodes a single non-unicode backslash escape at the start
// of s (s[0] is '\\', s[1] is not 'u'/'U') and returns the decoded rune,
// the number of bytes consumed (including the backslash), and any error. It
// implements exactly CUE's escape set; an unsupported or malformed escape
// is an error so the quoted segment is rejected.
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
	default:
		return 0, 0, fmt.Errorf("unknown escape sequence \\%c", s[1])
	}
}

// decodeUnicodeCodePoint decodes a \u (4 hex) or \U (8 hex) escape at the
// start of s (s[0]=='\\', s[1]=='u' or 'U') into its raw code point. It
// requires exactly the right number of hex digits and rejects an
// out-of-range code point, but — unlike a final rune — it ALLOWS a
// surrogate-half value through, so decodeEscapeAt can pair two halves via
// utf16.DecodeRune before deciding accept/reject. A surrogate that never
// pairs is rejected by decodeEscapeAt.
func decodeUnicodeCodePoint(s string) (uint32, int, error) {
	n := 4
	if s[1] == 'U' {
		n = 8
	}
	if len(s) < 2+n {
		return 0, 0, fmt.Errorf("truncated \\%c escape", s[1])
	}
	// Accumulate into a uint32 so an 8-digit \U escape such as \U80000000
	// does not overflow rune (an int32) into a negative value that would
	// slip past the MaxRune check below; CUE rejects such an out-of-range
	// code point.
	var v uint32
	for j := 0; j < n; j++ {
		d, ok := hexVal(s[2+j])
		if !ok {
			return 0, 0, fmt.Errorf("invalid hex digit %q in \\%c escape", s[2+j], s[1])
		}
		v = v<<4 | uint32(d)
	}
	if v > utf8.MaxRune {
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

// rawUnquote decodes the body of a CUE raw-string label (the bytes between
// the opening N '#' + '"' and the closing '"' + N '#') given the hash count
// hashes. In a raw string the escape introducer is '\' followed by exactly
// `hashes` '#'; a backslash NOT followed by that many '#' is a literal
// backslash, and '\' + hashes-of-'#' + c decodes the escape c exactly as a
// regular quoted string does (\n \t \" \uXXXX surrogate pairs …). This
// mirrors cue.ParsePath's raw-string handling. An unknown escape after the
// '\#…' introducer is rejected. A raw newline or CR is rejected, matching
// cueUnquote: CUE errors on a raw newline in a single-line raw string and
// decodes a raw CR to the empty string the empty-segment check then rejects,
// so rejecting both here keeps the two arms in agreement. Other raw control
// bytes pass through verbatim. The body is already valid UTF-8 with no NUL
// (ParsePath validates the whole expression up front).
func rawUnquote(body string, hashes int) (string, error) {
	hashRun := strings.Repeat("#", hashes)
	var b strings.Builder
	b.Grow(len(body))
	for i := 0; i < len(body); {
		switch body[i] {
		case '\n':
			return "", fmt.Errorf("illegal newline in raw string")
		case '\r':
			return "", fmt.Errorf("illegal carriage return in raw string")
		}
		if body[i] == '\\' && strings.HasPrefix(body[i+1:], hashRun) {
			// '\' + hashes '#' introduces an escape; the escape char follows.
			// Rewrite into the regular "\c" form so decodeEscapeAt — which
			// also handles \u/\U surrogate pairing — can decode it.
			r, width, err := decodeRawEscape(body, i, hashes)
			if err != nil {
				return "", err
			}
			b.WriteRune(r)
			i += width
			continue
		}
		_, size := utf8.DecodeRuneInString(body[i:])
		b.WriteString(body[i : i+size])
		i += size
	}
	return b.String(), nil
}

// decodeRawEscape decodes a raw-string escape at body[i] — a '\' followed by
// `hashes` '#' and then the escape selector — and returns the decoded rune
// and the number of body bytes consumed (the '\', the '#' run, and the
// escape payload). It reuses the regular escape decoder by splicing the
// backslash directly onto the payload (dropping the '#' run), so \u/\U
// surrogate-pair handling and the accepted-escape set match a quoted label.
func decodeRawEscape(body string, i, hashes int) (rune, int, error) {
	payloadStart := i + 1 + hashes // past '\' and the '#' run
	if payloadStart >= len(body) {
		return 0, 0, fmt.Errorf("truncated raw-string escape")
	}
	// Form "\<rest>" so decodeEscapeAt sees a normal escape at offset 0.
	spliced := "\\" + body[payloadStart:]
	r, width, err := decodeEscapeAt(spliced, 0)
	if err != nil {
		return 0, 0, err
	}
	// width counts the spliced '\' (1 byte) plus the payload bytes consumed;
	// the real consumption is the '\', the '#' run, and those payload bytes.
	return r, 1 + hashes + (width - 1), nil
}
