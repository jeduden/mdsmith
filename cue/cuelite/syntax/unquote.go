package syntax

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
			r, width, err := decodeEscapeAt(body, i, 0)
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

// decodeEscapeAt decodes the backslash escape at body[i] and returns the
// decoded rune, the number of body bytes consumed (including the backslash
// and any hash-introducer run), and any error. hashes is the raw-string
// hash level: 0 for a normal quoted string, where the escape introducer is a
// bare '\'; N>0 for a raw string at hash level N, where the introducer is
// '\' followed by exactly N '#' (so the escape selector sits at
// body[i+1+hashes]). It handles CUE's UTF-16 surrogate-pairing rule: when a
// \u/\U (or \#u/\#U at level N) escape decodes to a HIGH surrogate and an
// immediately-following low-surrogate escape WRITTEN WITH THE SAME
// INTRODUCER decodes to a LOW surrogate, the two combine into the single
// astral rune (utf16.DecodeRune) and the consumed width spans both escapes.
// A lone surrogate half is rejected, matching cue.ParsePath. Non-unicode
// escapes delegate to decodeEscape.
//
// Precondition: body[i]=='\\' and body[i+1 : i+1+hashes] is the '#' run, so
// the selector byte body[i+1+hashes] is in range.
func decodeEscapeAt(body string, i, hashes int) (rune, int, error) {
	sel := i + escBackslash + hashes
	c := body[sel : sel+1]
	if c != "u" && c != "U" {
		r, w, err := decodeEscape(body[i:], hashes)
		return r, w, err
	}
	hi, hiWidth, err := decodeUnicodeCodePoint(body[i:], hashes)
	if err != nil {
		return 0, 0, err
	}
	if !utf16.IsSurrogate(rune(hi)) {
		return rune(hi), hiWidth, nil
	}
	// A high surrogate may pair with an immediately-following low-surrogate
	// escape that uses the SAME introducer. Anything else (a lone high, a
	// bare low, a high followed by a non-low, or — in a raw string — a low
	// half written with a different introducer) is rejected.
	r, w, ok := combineSurrogate(body, i, hashes, hi, hiWidth)
	if !ok {
		return 0, 0, fmt.Errorf("lone surrogate U+%04X in \\u/\\U escape", hi)
	}
	return r, w, nil
}

// escBackslash is the one-byte width of the leading "\\" of an escape, used
// to index past the backslash to the (hash run and) selector byte.
const escBackslash = 1

// combineSurrogate tries to join the high surrogate hi (already decoded from
// body[i:], hiWidth bytes wide, at raw hash level hashes) with an
// immediately-following low-surrogate escape that uses the SAME introducer —
// '\' + hashes '#' + 'u'/'U'. It returns the combined astral rune, the total
// width of both escapes, and ok=true on success; ok=false when no matching
// low-surrogate escape follows. In a raw string the low half MUST also carry
// the '\#…' introducer: a plain '\u' there is literal text, so the high half
// stays lone and is rejected, matching cue.ParsePath. utf16.DecodeRune
// yields the real rune for a valid pair, so combineSurrogate is reached only
// with a genuine high half.
func combineSurrogate(body string, i, hashes int, hi uint32, hiWidth int) (rune, int, bool) {
	next := i + hiWidth
	// The low half must be '\' + hashes '#' + a 'u'/'U' selector. Bounds-check
	// the whole introducer (backslash, hash run, selector byte) before reading
	// it: a high half at the very end of the body — e.g. a raw '\#uD800' whose
	// next byte is the backslash that opens the closing delimiter run — leaves
	// too few bytes for a second escape, so there is no pairing.
	if next+escBackslash+hashes >= len(body) || body[next] != '\\' {
		return 0, 0, false
	}
	// The low half must repeat the SAME '\' + N '#' introducer. Compare the
	// '#' run byte-by-byte rather than allocating a strings.Repeat introducer
	// per pair attempt: this is the decoder's only would-be allocation and a
	// second source of truth for the introducer shape.
	for j := 0; j < hashes; j++ {
		if body[next+escBackslash+j] != '#' {
			return 0, 0, false
		}
	}
	sel := next + escBackslash + hashes
	c := body[sel : sel+1]
	if c != "u" && c != "U" {
		return 0, 0, false
	}
	lo, loWidth, err := decodeUnicodeCodePoint(body[next:], hashes)
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
// of s (s[0] is '\\', then a hashes-long '#' run, then a selector that is
// not 'u'/'U') and returns the decoded rune, the number of bytes consumed
// (the backslash, the '#' run, and the selector), and any error. It
// implements exactly CUE's escape set; an unsupported or malformed escape
// is an error so the segment is rejected. hashes is the raw-string hash
// level (0 for a normal quoted string).
//
// Precondition: len(s) >= 2+hashes, i.e. the selector byte is in range. For
// a quoted string parseQuotedSegment guarantees the backslash is never the
// last body byte; for a raw string rawUnquote only enters here after
// confirming the '\' + hashes '#' introducer plus a following selector.
func decodeEscape(s string, hashes int) (rune, int, error) {
	sel := 1 + hashes
	w := 2 + hashes
	switch s[sel] {
	case 'a':
		return '\a', w, nil
	case 'b':
		return '\b', w, nil
	case 'f':
		return '\f', w, nil
	case 'n':
		return '\n', w, nil
	case 'r':
		return '\r', w, nil
	case 't':
		return '\t', w, nil
	case 'v':
		return '\v', w, nil
	case '\\':
		return '\\', w, nil
	case '"':
		return '"', w, nil
	case '/':
		return '/', w, nil
	default:
		return 0, 0, fmt.Errorf("unknown escape sequence \\%c", s[sel])
	}
}

// decodeUnicodeCodePoint decodes a \u (4 hex) or \U (8 hex) escape at the
// start of s (s[0]=='\\', then a hashes-long '#' run, then 'u'/'U') into its
// raw code point. It requires exactly the right number of hex digits and
// rejects an out-of-range code point, but — unlike a final rune — it ALLOWS
// a surrogate-half value through, so decodeEscapeAt can pair two halves via
// utf16.DecodeRune before deciding accept/reject. A surrogate that never
// pairs is rejected by decodeEscapeAt. hashes is the raw-string hash level
// (0 for a normal quoted string).
func decodeUnicodeCodePoint(s string, hashes int) (uint32, int, error) {
	sel := 1 + hashes // index of the 'u'/'U' selector
	n := 4
	if s[sel] == 'U' {
		n = 8
	}
	digits := sel + 1 // first hex digit
	if len(s) < digits+n {
		return 0, 0, fmt.Errorf("truncated \\%c escape", s[sel])
	}
	// Accumulate into a uint32 so an 8-digit \U escape such as \U80000000
	// does not overflow rune (an int32) into a negative value that would
	// slip past the MaxRune check below; CUE rejects such an out-of-range
	// code point.
	var v uint32
	for j := 0; j < n; j++ {
		d, ok := hexVal(s[digits+j])
		if !ok {
			return 0, 0, fmt.Errorf("invalid hex digit %q in \\%c escape", s[digits+j], s[sel])
		}
		v = v<<4 | uint32(d)
	}
	if v > utf8.MaxRune {
		return 0, 0, fmt.Errorf("invalid code point U+%04X in \\%c escape", v, s[sel])
	}
	return v, digits + n, nil
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
// backslash. The escape SELECTOR set after the '\#…' introducer matches a
// regular quoted string (\#n \#t \#" \#uXXXX …), but a \u/\U surrogate PAIR
// must be written with BOTH halves carrying the same '\#…' introducer: the
// low half of a pair is itself a '\#u…' escape, not a plain '\u…'. A high
// half followed by a plain '\u…' (which is literal text in a raw string), or
// by nothing (the closing delimiter's backslash), stays lone and is
// rejected — matching cue.ParsePath, which decodes such a label to the empty
// string the empty-segment check then rejects. An unknown escape after the
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
			// '\' + hashes '#' introduces an escape; the selector follows. Its
			// byte is always present here: rawStringCloseIndex already rejects (as
			// unterminated) a '\#…' run with no following byte before the close,
			// so decodeEscapeAt's selector index is in range.
			r, width, err := decodeEscapeAt(body, i, hashes)
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
