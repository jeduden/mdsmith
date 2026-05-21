package punkt

// MatchAbbrPattern reports whether the upstream regex
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
// Plan 191 introduced this DFA in internal/mdtext/abbr.go; it is
// promoted here unchanged so the fork pipeline can reach it without
// importing back into mdtext. The byte-equivalence guarantee against
// the regex is gated by token_test.go's TestMatchAbbrPattern_*
// (mirrored from mdtext) and ultimately by the sentence-equivalence
// harness.
func MatchAbbrPattern(tok string) bool {
	n := len(tok)
	if n < 3 {
		return false
	}
	i := 0
	for i < n {
		if !isWordByte(tok[i]) {
			i++
			continue
		}
		if i+1 >= n || tok[i+1] != '.' {
			i++
			continue
		}
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
		i++
	}
	return false
}

// isWordByte reports whether b is a `\w` byte — ASCII digit, ASCII
// letter, or underscore — matching Go's regexp Perl character class.
// All non-ASCII bytes (≥ 0x80) return false.
func isWordByte(b byte) bool {
	return (b >= '0' && b <= '9') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= 'a' && b <= 'z') ||
		b == '_'
}
