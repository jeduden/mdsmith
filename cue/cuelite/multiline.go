package cuelite

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// parseMultilineSegment reads a CUE multiline string label starting at
// expr[pos]. The opener is a run of `hashes` '#' (already counted by the
// caller) followed by three quotes ("""), so expr[pos : pos+hashes] is the
// '#' run and expr[pos+hashes : pos+hashes+3] is `"""`. It returns the
// decoded label, the number of bytes consumed (opener through the closing
// `"""`+'#'-run), and any error.
//
// CUE multiline-string semantics (cue/literal):
//
//   - The opener `"""` (or '#'×N + `"""`) MUST be followed immediately by a
//     newline ('\n' or '\r\n'); any other byte after it — even a space — is
//     "opening quote of multiline string must be followed by newline".
//   - The closing line is the final line: the bytes between its last newline
//     and the closing `"""`+hashes must be only whitespace, which becomes the
//     INDENTATION prefix. That prefix must start every content line and is
//     stripped from each; a content line lacking it makes the literal decode
//     to the empty string (which the empty-segment check then rejects),
//     matching cue.ParsePath's Unquoted() — which maps the literal's
//     "invalid whitespace" error to "".
//   - The newline that precedes the closing line is excluded from the value.
//   - Escapes follow the same dialect as a single-line string at the same hash
//     level (\n \t \" \uXXXX … at level 0; \#n \#t … at level N), and surrogate
//     escape pairs combine per CUE's UTF-16 rule.
//
// Any malformed multiline literal whose CUE Unquoted() value is "" is reported
// here as an empty segment (decoded == ""), so the empty-segment check in
// consumeQuotedSegment/consumeRawStringSegment rejects it — the same outcome
// as the oracle.
func parseMultilineSegment(expr string, pos, hashes int) (string, int, error) {
	openerLen := hashes + 3 // '#'×N + `"""`
	bodyStart := pos + openerLen
	closeDelim := `"""` + strings.Repeat("#", hashes)
	rel := multilineCloseIndex(expr[bodyStart:], hashes, closeDelim)
	if rel < 0 {
		return "", 0, fmt.Errorf(
			"unterminated multiline string segment starting at position %d", pos)
	}
	// token is the full literal including the opener and closing delimiter, as
	// CUE's ParseQuotes expects (start == end == the whole token).
	tokenEnd := bodyStart + rel + len(closeDelim)
	token := expr[pos:tokenEnd]
	// A malformed multiline literal (bad opener/indent/escape) decodes to "",
	// which the empty-segment check in consumeMultilineSegment rejects — the
	// same outcome as cue.ParsePath's Unquoted(). Only the unterminated case
	// above is a structural error worth its own message.
	return unquoteMultiline(token, hashes), tokenEnd - pos, nil
}

// multilineCloseIndex returns the byte offset within body of the closing
// `"""`+N'#' delimiter of a hash-level-N multiline string, or -1 when none
// is found. It scans left-to-right, skipping each escape sequence
// ('\' + N '#' + one selector byte) so an escaped quote run is never read as
// the close. closeDelim is the precomputed `"""`+N'#' delimiter.
func multilineCloseIndex(body string, hashes int, closeDelim string) int {
	hashRun := strings.Repeat("#", hashes)
	for i := 0; i < len(body); {
		if body[i] == '\\' && strings.HasPrefix(body[i+1:], hashRun) {
			if i+1+hashes >= len(body) {
				return -1
			}
			i += 2 + hashes
			continue
		}
		if strings.HasPrefix(body[i:], closeDelim) {
			return i
		}
		i++
	}
	return -1
}

// unquoteMultiline decodes a full CUE multiline string TOKEN (opener through
// closing delimiter) at hash level `hashes` into its label value. It ports
// CUE's cue/literal multiline algorithm for the string-label subset: the
// quote char is always '"', interpolation ('\(') cannot appear in a path, and
// the \x/octal escapes are rejected (CUE rejects them for double-quoted
// strings). On any malformed-literal condition CUE's path Unquoted() yields
// "", so this returns "" and lets the caller's empty-segment check reject.
func unquoteMultiline(token string, hashes int) string {
	// CUE's scanner strips every '\r' from a MULTILINE string token (numChar
	// == 3) before lexing it (scanner.stripCR), so CRLF line endings and bare
	// CRs decode as if they were never there. Strip CR up front so the
	// whitespace/indent computation and the decode see a CR-free token,
	// matching cue.ParsePath exactly. (A single-line string keeps its CR, which
	// the single-line decoder rejects — that path never reaches here.)
	token = stripCR(token)
	ws, contentStart, ok := multilineWhitespace(token, hashes)
	if !ok {
		// Opener not followed by a newline, or the closing line carries
		// non-whitespace: CUE's Unquoted() is "".
		return ""
	}
	// The content region is everything from the first content byte up to (but
	// excluding) the closing delimiter; the multilineCloseIndex scan guarantees
	// the delimiter sits at the very end of the token, so trimming its length
	// leaves exactly the content lines plus the final newline and the closing
	// line's indentation.
	closeLen := hashes + 3
	content := token[contentStart : len(token)-closeLen]
	decoded, decodeOK := decodeMultilineBody(content, ws, hashes)
	if !decodeOK {
		return ""
	}
	return decoded
}

// multilineWhitespace ports the multiline arm of CUE's ParseQuotes for the
// string-label subset. It returns the indentation prefix `ws` (the whitespace
// the closing line carries before its `"""`+hashes), the byte offset
// contentStart in token where the body begins (just past the opener's newline
// and the leading copy of ws), and ok=false when the opener is not followed
// by a newline or the closing line is not whitespace-prefixed by a real
// newline. Precondition: token starts with '#'×hashes + `"""` and ends with
// `"""` + '#'×hashes.
func multilineWhitespace(token string, hashes int) (ws string, contentStart int, ok bool) {
	openerLen := hashes + 3
	// The opener must be followed immediately by a newline. CR is already
	// stripped (unquoteMultiline), so a CRLF opener is a bare '\n' here; any
	// other byte after the opener is "opening quote must be followed by
	// newline", which CUE maps to an empty Unquoted().
	if !strings.HasPrefix(token[openerLen:], "\n") {
		return "", 0, false
	}
	const nlLen = 1 // the opener's single '\n' (CR already stripped)
	// Walk back from just before the closing delimiter over trailing spaces to
	// the newline that ends the last content line; the spaces are the indent.
	closeLen := openerLen // `"""`+hashes has the same length as the opener
	end := len(token) - closeLen
	i := end
	hasNewline := false
	for i > 0 {
		r, size := utf8.DecodeLastRuneInString(token[:i])
		if r == '\n' || !unicode.IsSpace(r) {
			hasNewline = r == '\n'
			break
		}
		i -= size
	}
	if !hasNewline {
		return "", 0, false
	}
	ws = token[i:end]
	contentStart = openerLen + nlLen
	// The first content line must carry the indent prefix (unless it is itself
	// the closing newline, i.e. empty content).
	if contentStart < len(token) && token[contentStart] != '\n' {
		if !strings.HasPrefix(token[contentStart:], ws) {
			return "", 0, false
		}
		contentStart += len(ws)
	}
	return ws, contentStart, true
}

// decodeMultilineBody decodes the content region s of a multiline string (the
// content lines plus the final newline and the closing line's indentation, but
// NOT the closing delimiter itself) given the indentation prefix ws and hash
// level hashes. It ports CUE's literal.Unquote loop for the string-label subset
// and returns ok=false when the literal is malformed (an unknown escape, a bad
// indent on a continuation line, a lone surrogate, or an escaped final newline)
// — CUE's path Unquoted() maps all of these to "". The final newline before the
// closing line is dropped, matching CUE's stripNL rule.
func decodeMultilineBody(s, ws string, hashes int) (string, bool) {
	var b strings.Builder
	stripNL := false
	for len(s) > 0 {
		if s[0] == '\n' {
			rest, wsOK := skipIndentAfterNewline(s[1:], ws)
			if !wsOK {
				return "", false
			}
			s = rest
			stripNL = true
			b.WriteByte('\n')
			continue
		}
		r, width, ok := decodeMultilineChar(s, hashes)
		if !ok {
			return "", false
		}
		b.WriteRune(r)
		s = s[width:]
		stripNL = false
	}
	out := b.String()
	if stripNL && len(out) > 0 {
		// Drop the newline that preceded the closing line.
		out = out[:len(out)-1]
	}
	return out, true
}

// skipIndentAfterNewline consumes the indentation prefix ws at the start of s
// (the bytes just after a body newline). A line that carries the prefix has it
// stripped; a blank line (one that is itself just a newline) carries no prefix
// and is left as-is. Any other content that does not start with the prefix is
// an indentation error, returning ok=false — which CUE maps to an empty
// Unquoted(). The token is CR-free here (unquoteMultiline strips CR), so only
// a bare '\n' marks a blank line.
func skipIndentAfterNewline(s, ws string) (string, bool) {
	switch {
	case strings.HasPrefix(s, ws):
		return s[len(ws):], true
	case strings.HasPrefix(s, "\n"):
		return s, true
	default:
		return "", false
	}
}

// stripCR returns s with every '\r' byte removed, matching CUE's scanner.
// stripCR for multiline string tokens. When s has no CR it is returned
// unchanged with no allocation.
func stripCR(s string) string {
	if !strings.ContainsRune(s, '\r') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\r' {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// decodeMultilineChar decodes one character of a multiline body at hash level
// hashes: a backslash escape (reusing the single-line escape decoders, which
// already handle surrogate pairing) or a verbatim rune. It returns the decoded
// rune, the bytes consumed, and ok=false on a malformed escape. A backslash
// not followed by the '#'×hashes introducer is a literal backslash.
func decodeMultilineChar(s string, hashes int) (rune, int, bool) {
	if s[0] == '\\' && strings.HasPrefix(s[1:], strings.Repeat("#", hashes)) {
		// The introducer's selector byte is always present here: the closing
		// delimiter sits at the end of the content region, and
		// multilineCloseIndex already rejects (as unterminated) a '\'+'#' run
		// with no following byte, so decodeEscapeAt's selector index is in range.
		r, width, err := decodeEscapeAt(s, 0, hashes)
		if err != nil {
			return 0, 0, false
		}
		return r, width, true
	}
	r, size := utf8.DecodeRuneInString(s)
	return r, size, true
}
