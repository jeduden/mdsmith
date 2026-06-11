package syntax

import (
	"fmt"
	"strings"
)

// string_scan.go scans the three CUE string dialects with `\(…)`
// interpolation. A complete string (no interpolation) is returned as a
// tString whose text is the RAW quoted source (including delimiters), so
// compileBasicLit and fieldLabel decode it through unquoteCUEString — the same
// in-house unquote (unquote.go) the path parser uses. A string that opens an
// interpolation is returned as a tInterpStart whose text is the DECODED first
// fragment; the parser then parses the embedded expression and calls
// resumeInterp for each later fragment. Decoding fragments in the scanner
// (rather than carrying CUE's raw-token encoding) is the plan-240 divergence
// from cuelang's tree: the in-house Interpolation.Elts holds decoded fragments.

// scanString scans a string literal starting at s.pos. It dispatches on the
// opening delimiter run to determine the dialect (raw `#`-hashes, the quote
// char, and the 1-or-3 char count), then scans the body. A bytes dialect
// (`'…'`) is scanned as a string so the parser/compiler can reject it as
// out-of-subset with the raw text; only its delimiter shape is recognised here.
func (s *scanner) scanString() tok {
	litStart := s.pos
	hashes := 0
	for s.pos < len(s.src) && s.src[s.pos] == '#' {
		hashes++
		s.pos++
	}
	if s.pos >= len(s.src) {
		s.fail("unterminated string")
		return tok{kind: tEOF}
	}
	q := s.src[s.pos]
	if q != '"' && q != '\'' {
		// A `#` run not followed by a quote is not a raw string — it is a
		// definition or other construct outside the subset.
		s.fail("unexpected character %q", string(q))
		return tok{kind: tEOF}
	}
	numChar := 1
	// A triple-quote opens a multiline string. CUE requires the opening triple
	// quote to be followed by a newline, but the body decoders below handle the
	// raw bytes; the parser-level multiline rules are out of the subset's
	// schema/row use, so a `"""` is scanned as a single-char dialect unless a
	// full triple run is present.
	if s.pos+2 < len(s.src) && s.src[s.pos+1] == q && s.src[s.pos+2] == q {
		numChar = 3
	}
	d := quoteDialect{char: q, numChar: numChar, hashes: hashes}
	bodyStart := s.pos + numChar
	if numChar == 3 {
		// A multiline string's whitespace prefix is the indentation of its
		// closing-quote line; compute it up front so every fragment strips it.
		ws, ok := s.multilineWhitespace(d, bodyStart)
		if !ok {
			s.fail("multiline string not terminated")
			return tok{kind: tEOF}
		}
		d.whitespace = ws
	}
	return s.scanStringBody(d, hashes, numChar, litStart, bodyStart, true)
}

// scanStringBody scans from bodyStart to the next interpolation introducer or
// closing delimiter. opening reports whether this is the literal's first
// fragment (so the returned token carries the opening delimiter for a complete
// string, or is a tInterpStart for a fragment). It returns a tString (complete
// literal, raw quoted text) or a tInterpStart (decoded first fragment) and
// pushes the dialect onto interpStack on an interpolation. litStart is the
// index of the literal's first delimiter byte (the leading `#` run for a raw
// string), so the complete-literal branch returns the RAW source including the
// opening hashes; it is meaningful only when opening is true.
func (s *scanner) scanStringBody(d quoteDialect, hashes, numChar, litStart, bodyStart int, opening bool) tok {
	closeRun := s.closingRun(d)
	i := bodyStart
	for i < len(s.src) {
		// An interpolation introducer is `\` + hashes `#` + `(`.
		if s.isInterpAt(i, hashes) {
			frag := s.src[bodyStart:i]
			dec, err := decodeFragment(frag, d, fragPos{first: opening, last: false})
			if err != nil {
				s.fail("string literal: %v", err)
				return tok{kind: tEOF}
			}
			s.interpStack = append(s.interpStack, d)
			s.pos = i + 1 + hashes + 1 // past `\`, hashes, `(`
			return tok{kind: tInterpStart, text: dec, bytes: d.char == '\''}
		}
		// A closing delimiter run ends the literal.
		if strings.HasPrefix(s.src[i:], closeRun) {
			if opening {
				// Complete literal: return the RAW quoted source for unquoteCUEString,
				// including the opening `#` run (litStart precedes the quote) so the
				// raw-string dialect round-trips.
				raw := s.src[litStart : i+len(closeRun)]
				s.pos = i + len(closeRun)
				return tok{kind: tString, text: raw}
			}
			// Final fragment of an interpolation: decode and mark the literal done.
			frag := s.src[bodyStart:i]
			dec, err := decodeFragment(frag, d, fragPos{first: false, last: true})
			if err != nil {
				s.fail("string literal: %v", err)
				return tok{kind: tEOF}
			}
			s.pos = i + len(closeRun)
			return tok{kind: tString, text: encodeDecodedFragment(dec)}
		}
		// A bare backslash escape (not an interpolation) advances past the whole
		// escape so an escaped delimiter (`\"`) is not read as the close.
		if s.isEscapeAt(i, hashes) {
			i += s.escapeWidthAt(i, hashes)
			continue
		}
		// A newline inside a single-line string is illegal; let the body decoder
		// surface it (decodeFragment / unquoteCUEString reject a raw newline). A
		// multiline string admits newlines.
		i++
	}
	s.fail("string literal not terminated")
	return tok{kind: tEOF}
}

// multilineWhitespace finds the closing delimiter run for a multiline dialect
// starting at bodyStart and returns the whitespace prefix on the closing-quote
// line — the run of spaces/tabs between the last newline before the close and
// the close itself (CUE's QuoteInfo.whitespace). It scans past interpolations
// (matching nested parens) and escapes so a `"""` inside an embedded expression
// does not look like the close. It returns ok=false when no close is found.
func (s *scanner) multilineWhitespace(d quoteDialect, bodyStart int) (string, bool) {
	closeRun := s.closingRun(d)
	i := bodyStart
	for i < len(s.src) {
		if s.isInterpAt(i, d.hashes) {
			// Skip the embedded expression up to its matching `)`.
			j := s.skipInterpExpr(i + 1 + d.hashes + 1)
			if j < 0 {
				return "", false
			}
			i = j
			continue
		}
		if strings.HasPrefix(s.src[i:], closeRun) {
			// Walk back from the close to the preceding newline; the bytes between
			// are the closing-line whitespace prefix.
			start := i
			for start > bodyStart && (s.src[start-1] == ' ' || s.src[start-1] == '\t') {
				start--
			}
			return s.src[start:i], true
		}
		if s.isEscapeAt(i, d.hashes) {
			i += s.escapeWidthAt(i, d.hashes)
			continue
		}
		i++
	}
	return "", false
}

// skipInterpExpr advances past an embedded interpolation expression starting at
// index i (just after the `\(`), returning the index just past the matching
// `)`. It tracks nested parens and skips nested strings so a `)` inside a
// string literal does not close the interpolation prematurely. It returns -1
// when no matching `)` is found.
func (s *scanner) skipInterpExpr(i int) int {
	depth := 1
	for i < len(s.src) {
		switch s.src[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i + 1
			}
		case '"', '\'', '#':
			// Skip a nested string literal whole so its delimiters and any `)` it
			// contains are not counted.
			i = s.skipNestedString(i)
			continue
		}
		i++
	}
	return -1
}

// skipNestedString advances past a string literal beginning at i (used while
// scanning a multiline whitespace prefix over an interpolation), returning the
// index just past the closing delimiter. A non-string at i (a lone `#` not
// opening a raw string) advances one byte.
func (s *scanner) skipNestedString(i int) int {
	hashes := 0
	j := i
	for j < len(s.src) && s.src[j] == '#' {
		hashes++
		j++
	}
	if j >= len(s.src) || (s.src[j] != '"' && s.src[j] != '\'') {
		return i + 1
	}
	q := s.src[j]
	numChar := 1
	if j+2 < len(s.src) && s.src[j+1] == q && s.src[j+2] == q {
		numChar = 3
	}
	closeRun := strings.Repeat(string(q), numChar) + strings.Repeat("#", hashes)
	k := j + numChar
	for k < len(s.src) {
		if s.isEscapeAt(k, hashes) {
			k += s.escapeWidthAt(k, hashes)
			continue
		}
		if strings.HasPrefix(s.src[k:], closeRun) {
			return k + len(closeRun)
		}
		k++
	}
	return len(s.src)
}

// resumeInterp scans the string fragment that follows a closed interpolation
// expression: the `)` has just been consumed by the parser, and the scanner is
// positioned at the next body byte. It pops nothing yet (the dialect stays on
// the stack until the closing delimiter), scanning to the next `\(` or close.
func (s *scanner) resumeInterp() tok {
	d := s.interpStack[len(s.interpStack)-1]
	// A resumed fragment is never the literal's opening, so litStart (the raw
	// complete-literal start) is unused; pass s.pos as a harmless placeholder.
	t := s.scanStringBody(d, d.hashes, d.numChar, s.pos, s.pos, false)
	// A complete fragment (kind tString) ends the literal: pop the dialect.
	if t.kind == tString {
		s.interpStack = s.interpStack[:len(s.interpStack)-1]
	}
	return t
}

// closingRun returns the closing delimiter byte run for a dialect: the quote
// char repeated numChar times, then hashes `#`.
func (s *scanner) closingRun(d quoteDialect) string {
	return strings.Repeat(string(d.char), d.numChar) + strings.Repeat("#", d.hashes)
}

// isInterpAt reports whether an interpolation introducer (`\` + hashes `#` +
// `(`) begins at src[i].
func (s *scanner) isInterpAt(i, hashes int) bool {
	if s.src[i] != '\\' {
		return false
	}
	j := i + 1
	for k := 0; k < hashes; k++ {
		if j >= len(s.src) || s.src[j] != '#' {
			return false
		}
		j++
	}
	return j < len(s.src) && s.src[j] == '('
}

// isEscapeAt reports whether a backslash escape (`\` + hashes `#` + a selector)
// begins at src[i] — the introducer that protects the following bytes from
// being read as a delimiter. In a raw string the introducer needs the `#` run.
func (s *scanner) isEscapeAt(i, hashes int) bool {
	if s.src[i] != '\\' {
		return false
	}
	j := i + 1
	for k := 0; k < hashes; k++ {
		if j >= len(s.src) || s.src[j] != '#' {
			// A `\` without the full `#` run in a raw string is a literal
			// backslash, not an escape.
			return false
		}
		j++
	}
	return j < len(s.src)
}

// escapeWidthAt returns the byte width of the escape at src[i] so the scanner
// can skip it without decoding. It is a conservative width — the introducer
// plus the selector byte — sufficient to step past an escaped delimiter; the
// real decode happens in decodeFragment / unquoteCUEString.
func (s *scanner) escapeWidthAt(i, hashes int) int {
	return 1 + hashes + 1
}

// fragPos marks where a fragment sits in its string literal, so the multiline
// decoder applies CUE's leading-newline strip (first fragment) and
// trailing-newline strip (last fragment) at the right ends only. A complete
// (non-interpolation) string is both first and last.
type fragPos struct {
	first bool
	last  bool
}

// decodeFragment decodes one string fragment (the bytes between
// delimiters/introducers, exclusive) through the dialect's unquote rules. A
// single-line dialect uses cueUnquote (plain) or rawUnquote (raw); a multiline
// dialect (numChar==3) uses decodeMultiline, which admits raw newlines and
// applies CUE's whitespace/newline stripping. The single-line decode is
// position-independent; multiline needs pos.
func decodeFragment(frag string, d quoteDialect, pos fragPos) (string, error) {
	if d.numChar == 3 {
		return decodeMultiline(frag, d, pos)
	}
	if d.hashes > 0 {
		return rawUnquote(frag, d.hashes)
	}
	return cueUnquote(frag)
}

// decodeMultiline decodes a multiline string fragment, applying CUE's rules
// (cue/literal.QuoteInfo.Unquote): the opening `"""` must be followed by a
// newline (stripped on the FIRST fragment); each interior newline strips the
// dialect's whitespace prefix that follows it; the final newline before the
// closing `"""` is stripped on the LAST fragment. Backslash escapes decode
// with the raw `#`-introducer rule; a raw newline passes through.
func decodeMultiline(frag string, d quoteDialect, pos fragPos) (string, error) {
	body := frag
	if pos.first {
		// CUE requires a newline immediately after the opening `"""`; strip it,
		// then strip the whitespace prefix of the first content line.
		switch {
		case strings.HasPrefix(body, "\r\n"):
			body = body[2:]
		case strings.HasPrefix(body, "\n"):
			body = body[1:]
		default:
			return "", fmt.Errorf("opening quote of multiline string must be followed by newline")
		}
		stripped, err := stripIndent(body, d.whitespace)
		if err != nil {
			return "", err
		}
		body = stripped
	}
	var b strings.Builder
	b.Grow(len(body))
	for i := 0; i < len(body); {
		c := body[i]
		if c == '\r' {
			// CUE drops a bare CR (CRLF normalises to LF).
			i++
			continue
		}
		if c == '\n' {
			b.WriteByte('\n')
			// Each interior line must carry the closing-line whitespace prefix
			// (CUE: "invalid whitespace"); an under-indented line is an error, not
			// a silent no-op strip. A blank line (a bare newline) is exempt.
			rest, err := stripIndent(body[i+1:], d.whitespace)
			if err != nil {
				return "", err
			}
			i = len(body) - len(rest)
			continue
		}
		if isMultilineEscape(body, i, d.hashes) {
			r, w, err := decodeEscapeAt(body, i, d.hashes)
			if err != nil {
				return "", err
			}
			b.WriteRune(r)
			i += w
			continue
		}
		b.WriteByte(c)
		i++
	}
	out := b.String()
	if pos.last {
		// The newline before the closing `"""` is stripped: the decoder already
		// wrote it, and the closing line's whitespace was trimmed, so a trailing
		// newline is that pre-close newline.
		out = strings.TrimSuffix(out, "\n")
	}
	return out, nil
}

// stripIndent removes the multiline whitespace prefix ws from the start of a
// line (the bytes just after an interior newline). A blank line — empty, or one
// that begins with another newline — carries no content and needs no prefix, so
// it is returned unchanged. A non-blank line that does NOT begin with the full
// prefix is under-indented; CUE rejects it as "invalid whitespace", returned as
// an error rather than the silent no-op a bare TrimPrefix would do.
func stripIndent(line, ws string) (string, error) {
	if line == "" || line[0] == '\n' || line[0] == '\r' {
		return line, nil
	}
	if ws == "" {
		return line, nil
	}
	if !strings.HasPrefix(line, ws) {
		return "", fmt.Errorf("invalid whitespace in multiline string")
	}
	return line[len(ws):], nil
}

// isMultilineEscape reports whether a backslash escape (`\` + hashes `#` + a
// selector) begins at body[i] in a multiline string of the given raw hash
// level. A `\` without the full `#` run in a raw multiline string is a literal
// backslash.
func isMultilineEscape(body string, i, hashes int) bool {
	if body[i] != '\\' {
		return false
	}
	j := i + 1
	for k := 0; k < hashes; k++ {
		if j >= len(body) || body[j] != '#' {
			return false
		}
		j++
	}
	return j < len(body)
}

// encodeDecodedFragment tags an already-decoded fragment so compileBasicLit can
// tell it apart from a raw quoted literal. A decoded fragment is stored on a
// BasicLit with Kind kInterpFrag; this helper returns the text unchanged (the
// Kind, set by the parser, carries the distinction), kept as a named seam so
// the encoding choice lives in one place.
func encodeDecodedFragment(dec string) string { return dec }

// unquoteCUEString decodes a complete (non-interpolation) string literal's RAW
// quoted source — including delimiters — into its value, dispatching on the
// dialect the same way the scanner did. It is the in-house replacement for
// cuelang.org/go/cue/literal.Unquote that compileBasicLit and fieldLabel call.
func unquoteCUEString(raw string) (string, error) {
	if len(raw) == 0 {
		return "", fmt.Errorf("empty string literal")
	}
	hashes := 0
	for hashes < len(raw) && raw[hashes] == '#' {
		hashes++
	}
	if hashes >= len(raw) {
		return "", fmt.Errorf("invalid string literal %q", raw)
	}
	q := raw[hashes]
	if q != '"' && q != '\'' {
		return "", fmt.Errorf("invalid string literal %q", raw)
	}
	numChar := 1
	if strings.HasPrefix(raw[hashes:], strings.Repeat(string(q), 3)) {
		numChar = 3
	}
	open := hashes + numChar
	closeRun := strings.Repeat(string(q), numChar) + strings.Repeat("#", hashes)
	if len(raw) < open+len(closeRun) || !strings.HasSuffix(raw, closeRun) {
		return "", fmt.Errorf("unterminated string literal %q", raw)
	}
	body := raw[open : len(raw)-len(closeRun)]
	d := quoteDialect{char: q, numChar: numChar, hashes: hashes}
	if numChar == 3 {
		// A complete multiline literal: the whitespace prefix is the indentation
		// of the closing-quote line, found by walking back from the close over
		// spaces/tabs to the preceding newline.
		end := len(body)
		ws := end
		for ws > 0 && (body[ws-1] == ' ' || body[ws-1] == '\t') {
			ws--
		}
		// CUE requires the closing `"""` to sit on its own line: the byte before
		// the closing-line whitespace must be a newline. Content on the closing
		// line (`ok"""`) leaves a non-newline there and is rejected.
		if ws == 0 || body[ws-1] != '\n' {
			return "", fmt.Errorf("closing quote of multiline string must follow a newline")
		}
		d.whitespace = body[ws:end]
	}
	return decodeFragment(body, d, fragPos{first: true, last: true})
}

// isBytesLiteral reports whether a raw string literal uses the single-quote
// (bytes) dialect, so compileBasicLit can reject it as out-of-subset before
// decoding.
func isBytesLiteral(raw string) bool {
	i := 0
	for i < len(raw) && raw[i] == '#' {
		i++
	}
	return i < len(raw) && raw[i] == '\''
}
