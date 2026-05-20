package punkt

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// punctSentenceEnders is the set of single-character end punctuation
// PunctStrings.HasSentencePunct tests for. Upstream loops over a fixed
// rune string `.!?。！？`; the fork strips the CJK variants per
// doc.go, leaving the ASCII trio.
const punctSentenceEnders = ".!?"

// hasSentencePunct reports whether s contains any sentence-ending
// punctuation. Replaces the upstream double-loop over rune pairs.
func hasSentencePunct(s string) bool {
	return strings.ContainsAny(s, punctSentenceEnders)
}

// Tokenize splits text into Tokens annotated with line- and
// paragraph-start markers. Mirrors upstream
// DefaultWordTokenizer.Tokenize byte-for-byte except for the CJK
// punctuation branch (dropped per doc.go) and the per-token allocation
// of regex pointers (gone — Token has no such field).
//
// The tokens slice is allocated fresh on every call. Callers that
// want to pool the result pass in a preallocated slice via
// TokenizeInto.
func Tokenize(text string, onlyPeriodContext bool) []Token {
	if len(text) == 0 {
		return nil
	}
	tokens := make([]Token, 0, 50)
	return TokenizeInto(tokens, text, onlyPeriodContext)
}

// tokenizerCursor holds the per-rune scan state of TokenizeInto.
// Carved out so the main loop fits under funlen's ceiling and the
// per-rune branching reads as one step at a time.
type tokenizerCursor struct {
	lastSpace      int
	lineStart      bool
	paragraphStart bool
	getNextWord    bool
}

// updateLineFlags applies upstream's newline-handling rule: a `\n`
// flips lineStart on; a second `\n` (lineStart already set) elevates
// to paragraphStart.
func (c *tokenizerCursor) updateLineFlags(char rune) {
	if char != '\n' {
		return
	}
	if c.lineStart {
		c.paragraphStart = true
	}
	c.lineStart = true
}

// emitTok consumes the word ending at cursor, appends a Token to dst
// (subject to the onlyPeriodContext filter), and returns the extended
// slice. Mirrors the upstream emit branch byte-for-byte.
func (c *tokenizerCursor) emitTok(
	dst []Token, text string, cursor int, onlyPeriodContext bool,
) []Token {
	word := strings.TrimSpace(text[c.lastSpace:cursor])
	if word == "" {
		return dst
	}
	hasPunct := hasSentencePunct(word)
	if onlyPeriodContext && !hasPunct && !c.getNextWord {
		c.lastSpace = cursor
		return dst
	}
	dst = append(dst, Token{
		Tok:       word,
		Position:  cursor,
		ParaStart: c.paragraphStart,
		LineStart: c.lineStart,
	})
	c.lastSpace = cursor
	c.lineStart = false
	c.paragraphStart = false
	c.getNextWord = hasPunct
	return dst
}

// TokenizeInto appends tokens parsed from text to dst and returns the
// extended slice. Used by the pooled-state path on
// DefaultSentenceTokenizer so the same slice is reused across calls.
//
// Behaviour matches upstream DefaultWordTokenizer.Tokenize: split on
// unicode.IsSpace, mark paragraph starts after a blank line, mark
// line starts after a single newline. With onlyPeriodContext = false
// the tokenizer emits every word; with true it emits only words near
// a sentence-ending punctuation character.
func TokenizeInto(dst []Token, text string, onlyPeriodContext bool) []Token {
	textLength := len(text)
	if textLength == 0 {
		return dst
	}
	c := tokenizerCursor{}
	for i, char := range text {
		if !unicode.IsSpace(char) && i != textLength-1 {
			continue
		}
		c.updateLineFlags(char)
		cursor := i
		if i == textLength-1 {
			cursor = textLength
		}
		dst = c.emitTok(dst, text, cursor, onlyPeriodContext)
	}
	if len(dst) == 0 {
		dst = append(dst, Token{Tok: text, Position: textLength})
	}
	return dst
}

// typeOf returns the case-normalized representation of tok in the
// same shape upstream DefaultWordTokenizer.Type returns:
//   - all letters unicode-lowercased
//   - any numeric run replaced with `##number##`
//   - if the post-lower-post-numeric result is more than 1 byte long,
//     commas dropped (upstream's `if len(typ) == 1` short-circuits the
//     comma strip on a length-1 result so e.g. a lone comma survives)
//
// The work is a two-pass byte scan into buf (reset to length 0 by the
// caller, grown by append). buf is reused across calls via the
// tokenizer's pooled state so the function adds no per-call
// allocations of its own.
//
// Upstream regex `reNumeric` is `-?[\.,]?\d[\d,\.-]*\.?$` — anchored
// at end of string. numericTail returns the same [start, end) span
// the regex would match; if absent, the token passes through
// lowercase-only.
func typeOf(tok string, buf []byte) []byte {
	buf = buf[:0]
	// Step 1: lowercase + numeric replace, into buf.
	numStart, numEnd, ok := numericTail(tok)
	if !ok {
		buf = appendStringsToLower(buf, tok)
	} else {
		buf = appendStringsToLower(buf, tok[:numStart])
		buf = append(buf, "##number##"...)
		if numEnd < len(tok) {
			// Upstream `.?$` — at most one byte after the digit run.
			buf = append(buf, tok[numEnd:]...)
		}
	}
	// Step 2: upstream's "if len(typ) == 1 return typ" — single-byte
	// result is returned as-is, so a lone comma survives.
	if len(buf) == 1 {
		return buf
	}
	// Step 3: drop commas in place.
	j := 0
	for i := 0; i < len(buf); i++ {
		if buf[i] != ',' {
			buf[j] = buf[i]
			j++
		}
	}
	return buf[:j]
}

// numericTail returns the [start, end) span of the substring of tok
// that the upstream regex `-?[\.,]?\d[\d,\.-]*\.?$` matches, or
// ok=false if the regex does not match.
//
// The match must end at the final byte of tok (the regex is
// $-anchored). We walk backward and reproduce the regex's structure:
//
//	`-?[\.,]?\d[\d,\.-]*\.?$`
//	prefix:        `-?[\.,]?`         — optional `-` then optional `.` or `,`
//	core:          `\d[\d,\.-]*`      — at least one digit followed by any digit/punct run
//	suffix:        `\.?`              — optional trailing `.`
//	anchor:        `$`                — at end
//
// regexp/syntax's longest-match semantics let us extend the core
// rightward as far as possible. Working backward, we identify the
// trailing optional period, then the core (digits + punctuation), then
// require at least one digit, then attach the optional prefix.
func numericTail(tok string) (start, end int, ok bool) {
	n := len(tok)
	if n == 0 {
		return 0, 0, false
	}
	end = n

	// `.?$` — strip an optional final period from consideration. The
	// core that follows must end at this trimmed boundary.
	trim := end
	if trim > 0 && tok[trim-1] == '.' {
		trim--
	}

	// `[\d,\.-]*` — walk back over any run of digits, `,`, `.`, or `-`.
	// At least one of these bytes must be a digit (`\d`).
	hasDigit := false
	j := trim
	for j > 0 {
		c := tok[j-1]
		if c >= '0' && c <= '9' {
			hasDigit = true
			j--
			continue
		}
		if c == ',' || c == '.' || c == '-' {
			j--
			continue
		}
		break
	}
	if !hasDigit {
		return 0, 0, false
	}

	// `\d` core requirement: leftmost byte of [j, trim) must be a
	// digit. Regex semantics consume at least one digit, then the
	// `[\d,\.-]*` runs on the rest. If the leftmost byte of the run
	// we walked is not a digit, slide j rightward to the first digit.
	// Subsequent bytes between [j, firstDigit) become prefix candidates,
	// not core.
	core := j
	for core < trim && (tok[core] < '0' || tok[core] > '9') {
		core++
	}
	if core >= trim {
		return 0, 0, false
	}
	// Now [core, trim) starts with a digit. Drop one byte at a time
	// from the LEFT of the core — but only as long as the resulting
	// span still starts with a digit; otherwise we are shrinking past
	// the `\d` requirement.
	// To match the regex's left-extension behaviour: prefix may
	// supply `-?[\.,]?`, i.e. up to 2 bytes immediately before core,
	// chosen from the set `{-, ., ,}` with the first (leftmost) being
	// optional `-`.
	start = core
	// Optional `[\.,]?` — at most one byte from {.  ,}
	if start > 0 && (tok[start-1] == '.' || tok[start-1] == ',') {
		start--
	}
	// Optional `-?` — at most one byte equal to `-`
	if start > 0 && tok[start-1] == '-' {
		start--
	}

	return start, end, true
}

// appendStringsToLower appends to dst the unicode-lowercase form of
// src. Matches `strings.ToLower(src)` byte-for-byte without
// allocating an intermediate string.
//
// Equivalence note: strings.ToLower lowercases NON-ASCII letters too
// (`É` → `é`). DefaultWordTokenizer.Type calls strings.ToLower before
// the comma replace; this helper produces the same bytes, so the
// equivalence harness continues to gate any unforeseen divergence.
func appendStringsToLower(dst []byte, src string) []byte {
	for _, r := range src {
		l := unicode.ToLower(r)
		if l < utf8.RuneSelf {
			dst = append(dst, byte(l))
			continue
		}
		var buf [utf8.UTFMax]byte
		n := utf8.EncodeRune(buf[:], l)
		dst = append(dst, buf[:n]...)
	}
	return dst
}

// typeNoPeriod returns the result of typeOf with a single trailing
// period removed, when present. Mirrors upstream
// DefaultWordTokenizer.TypeNoPeriod.
func typeNoPeriod(tok string, buf []byte) []byte {
	buf = typeOf(tok, buf)
	if len(buf) > 1 && buf[len(buf)-1] == '.' {
		buf = buf[:len(buf)-1]
	}
	return buf
}

// typeNoSentPeriod calls typeNoPeriod when tok has been marked as a
// sentence break, otherwise typeOf. Mirrors upstream
// DefaultWordTokenizer.TypeNoSentPeriod.
func typeNoSentPeriod(t *Token, buf []byte) []byte {
	if t.SentBreak {
		return typeNoPeriod(t.Tok, buf)
	}
	return typeOf(t.Tok, buf)
}

// firstUpper reports whether tok's first rune is uppercase. Mirrors
// upstream DefaultWordTokenizer.FirstUpper, but decodes only the
// first rune instead of `[]rune(t.Tok)`.
func firstUpper(tok string) bool {
	if tok == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(tok)
	return unicode.IsUpper(r)
}

// firstLower reports whether tok's first rune is lowercase. Mirrors
// upstream DefaultWordTokenizer.FirstLower.
func firstLower(tok string) bool {
	if tok == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(tok)
	return unicode.IsLower(r)
}

// isCoordinatePartOne reports whether tok is the exact French
// coordinate prefix `N°.`. Mirrors upstream
// DefaultWordTokenizer.IsCoordinatePartOne.
func isCoordinatePartOne(tok string) bool { return tok == "N°." }
