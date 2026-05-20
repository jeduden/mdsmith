package mdtext

// matchAbbrPattern reports whether the upstream regex
// `((?:[\w]\.)+[\w]*\.)` would find at least one match anywhere in
// tok. It is the boolean form of
// `len(reAbbr.FindAllString(tok, 1)) > 0` from
// `github.com/neurosnap/sentences/english/main.go:15`, with the
// `regexp` engine's backtracking removed.
//
// The pattern in plain English: at least one `\w\.` pair, optionally
// followed by more word characters, ending with `\.`. Concretely, any
// matching substring has the form `\w \. (\w|\.)* \.` where the run
// of `\w`-or-`\.` between the first and final period may be empty.
//
// Go's `\w` is ASCII-only (`[0-9A-Za-z_]`), and `.` is ASCII. So a
// match cannot cross a non-ASCII rune — encountering any byte ≥ 0x80
// breaks the candidate just like any other separator. That lets the
// scanner operate byte-by-byte without any UTF-8 decoding: the first
// non-word non-`.` byte ends the current candidate, and we slide one
// byte forward to start a new attempt. Continuation bytes (0x80–0xBF)
// are also non-word non-`.`, so stepping into the middle of a
// multi-byte rune resolves itself in the same loop.
//
// MDS024's tokens are at most a few dozen runes (Punkt only feeds
// reAbbr period-ending words), so the linear scan is allocation-free
// and runs in nanoseconds. The corresponding Punkt frame is
// `english.MultiPunctWordAnnotation.tokenAnnotation`; replacing the
// regex there drops `regexp.(*Regexp).tryBacktrack` out of the
// segmenter's hot path. See plan 191 for the profile.
func matchAbbrPattern(tok string) bool {
	// Fast reject: the shortest possible match is "w.." (one word
	// byte, two periods) — three bytes, two of which are `.`. We
	// detect the two-period condition during the scan; the length
	// check is the cheap guard.
	n := len(tok)
	if n < 3 {
		return false
	}
	i := 0
	for i < n {
		// Try to anchor a match at position i. The candidate must
		// begin with `\w\.` — a word byte followed by a period.
		if !isWordByte(tok[i]) {
			i++
			continue
		}
		if i+1 >= n || tok[i+1] != '.' {
			i++
			continue
		}
		// One `\w\.` pair consumed. Scan forward over the body of
		// `(?:\w\.)*[\w]*\.` — which collapses to "any run of word
		// bytes or periods, with at least one final period". As
		// soon as we see another `.`, the candidate matches.
		j := i + 2
		for j < n {
			if tok[j] == '.' {
				return true
			}
			if !isWordByte(tok[j]) {
				break
			}
			j++
		}
		// Body ran out (end of input or non-word non-`.` byte) with
		// no closing period; slide one byte forward and try again.
		i++
	}
	return false
}

// isWordByte reports whether b is a `\w` byte — ASCII digit, ASCII
// letter, or underscore — matching Go's regexp Perl character class.
// All non-ASCII bytes (≥ 0x80) return false, which is what reAbbr's
// `\w` does and what makes a byte-by-byte scan correct.
func isWordByte(b byte) bool {
	return (b >= '0' && b <= '9') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= 'a' && b <= 'z') ||
		b == '_'
}
