package punkt

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Token is a tokenized word annotated by the Punkt pipeline. The
// fork drops the six *regexp.Regexp pointers upstream Token carries —
// every per-token regex match is replaced by a byte scanner in
// word_tokenizer.go, so the struct shrinks to just its flag and
// metadata fields. Pooled allocation lives on the tokenizer's per-call
// state buffers, not here.
type Token struct {
	Tok       string
	Position  int
	SentBreak bool
	ParaStart bool
	LineStart bool
	Abbr      bool
}

// String is the fmt.Stringer impl, retained for log/debug inspection
// and parity with upstream.
func (t *Token) String() string {
	return fmt.Sprintf(
		"<Token Tok: %q, SentBreak: %t, Abbr: %t, Position: %d>",
		t.Tok, t.SentBreak, t.Abbr, t.Position,
	)
}

// HasPeriodFinal reports whether tok ends with a period.
// Upstream also accepts the CJK period (`。`), which the fork drops
// per the equivalence corpus restriction documented in doc.go.
func HasPeriodFinal(tok string) bool {
	return len(tok) > 0 && tok[len(tok)-1] == '.'
}

// isAlphaToken reports whether tok matches the upstream
// `^[A-Za-z]+$` regex (reAlpha). It is the byte form of that anchor:
// the whole token must be one or more ASCII letters.
func isAlphaToken(tok string) bool {
	if len(tok) == 0 {
		return false
	}
	for i := 0; i < len(tok); i++ {
		c := tok[i]
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
			return false
		}
	}
	return true
}

// isInitial reports whether tok matches upstream's `^[A-Za-z]\.$`
// regex (reInitial). Exactly two bytes: one ASCII letter then a
// period.
func isInitial(tok string) bool {
	if len(tok) != 2 {
		return false
	}
	c := tok[0]
	if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
		return false
	}
	return tok[1] == '.'
}

// isEllipsis reports whether tok matches upstream's `\.\.+$` regex
// (reEllipsis): at least two periods anchored at the end. The pattern
// has no `^` anchor in upstream, so any tok ending with `..` (or
// longer) matches — `a..` matches, `a.b` does not.
func isEllipsis(tok string) bool {
	n := len(tok)
	if n < 2 {
		return false
	}
	// Walk backward over the trailing period run.
	periods := 0
	for n > 0 && tok[n-1] == '.' {
		periods++
		n--
	}
	return periods >= 2
}

// isListNumber reports whether tok matches upstream's `^\d+.?\)?$`
// regex (reListNumber). The regex parses as:
//   - `\d+`   — one or more ASCII digits
//   - `.?`    — optional ANY character (the dot is unescaped)
//   - `\)?`   — optional ASCII close paren
//
// Tail variants: bare digits, `\d+x`, `\d+\)`, or `\d+x\)`.
// The optional-any matters because `1.` and `12.` are list numbers
// even though the dot is not anchored as a literal.
func isListNumber(tok string) bool {
	n := len(tok)
	if n == 0 {
		return false
	}
	// `\d+` — count leading digits.
	i := 0
	for i < n && tok[i] >= '0' && tok[i] <= '9' {
		i++
	}
	if i == 0 {
		return false
	}
	// `.?` — at most one ANY-character. Go's regexp `.` matches a
	// rune, not a byte, so decode one full rune to stay equivalent
	// on multi-byte tokens like `"1世"` (regex matches; a byte step
	// would leave the scanner mid-rune and diverge).
	if i < n {
		_, sz := utf8.DecodeRuneInString(tok[i:])
		i += sz
	}
	// `\)?` — optional `)`.
	if i < n && tok[i] == ')' {
		i++
	}
	return i == n
}

// isCoordinateSecondPart reports whether tok matches upstream's
// `^[0-9]*\.[0-9]*\.[0-9]*\.$` regex (reCoordinateSecondPart): three
// `\.` separators with optional digit runs between them, anchored
// start and end. Examples: `1.2.3.`, `1..3.`, `..1.`, `...`. The
// digit runs may all be empty, so the shortest match is `...`.
func isCoordinateSecondPart(tok string) bool {
	n := len(tok)
	// Three periods is the minimum (`...`).
	if n < 3 {
		return false
	}
	if tok[n-1] != '.' {
		return false
	}
	// Walk forward over <digits>.<digits>.<digits>.
	i := 0
	for seg := 0; seg < 3; seg++ {
		for i < n && tok[i] >= '0' && tok[i] <= '9' {
			i++
		}
		if i == n || tok[i] != '.' {
			return false
		}
		i++
	}
	return i == n
}

// isNumberPrefix reports whether tok starts with `##number##`, the
// sentinel `Type` substitutes for any numeric run. Mirrors upstream
// DefaultWordTokenizer.IsNumber.
func isNumberPrefix(tok string) bool {
	const prefix = "##number##"
	return len(tok) >= len(prefix) && tok[:len(prefix)] == prefix
}

// hasUnreliableEndChars reports whether tok ends with one of the
// ambiguous quote-paren end-of-sentence pairs upstream's English
// WordTokenizer flags (see english/main.go:HasUnreliableEndChars).
// The CJK variants upstream's DefaultWordTokenizer adds are not in
// this set on purpose — only the English overrides apply on the
// English pipeline.
var unreliableEnders = [...]string{
	`."`, `.'`, `.)`, `.’`, `.”`,
	`?"`, `?'`, `?)`, `?’`, `?”`,
	`!"`, `!'`, `!)`, `!’`, `!”`,
}

func hasUnreliableEndChars(tok string) bool {
	for _, e := range unreliableEnders {
		if strings.HasSuffix(tok, e) {
			return true
		}
	}
	return false
}

// sentEnders and sentEndParens are the English-pipeline sets from
// upstream WordTokenizer.HasSentEndChars (english/main.go). The
// DefaultWordTokenizer in upstream also tests CJK suffixes; the fork
// drops those per doc.go.
var sentEnders = [...]string{
	`."`, `.'`, `.)`, `.’`, `.”`,
	`?`, `?"`, `?'`, `?)`, `?’`, `?”`,
	`!`, `!"`, `!'`, `!)`, `!’`, `!”`,
}

var sentEndParens = [...]string{
	`.[`, `.(`, `."`, `.'`,
	`?[`, `?(`,
	`![`, `!(`,
}

func hasSentEndChars(tok string) bool {
	for _, e := range sentEnders {
		if strings.HasSuffix(tok, e) {
			return true
		}
	}
	for _, p := range sentEndParens {
		if strings.Contains(tok, p) {
			return true
		}
	}
	return false
}
