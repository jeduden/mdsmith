package mdtext

import "testing"

// TestASCIISpaceTableMatchesIsSpace ties the asciiSpace byte table to
// IsSpace across the whole ASCII range. The byte-scan word counters
// (CountWordsBytes, countWordsString, wordCounter.writeBytes) classify
// ASCII through the table; the rune scans use IsSpace. If the two ever
// disagree those counters would silently diverge from the rune scans.
// This pins the two definitions to one another.
func TestASCIISpaceTableMatchesIsSpace(t *testing.T) {
	for c := 0; c < 0x80; c++ {
		if asciiSpace[c] != IsSpace(rune(c)) {
			t.Errorf("asciiSpace and IsSpace disagree on byte 0x%02x: table=%v IsSpace=%v",
				c, asciiSpace[c], IsSpace(rune(c)))
		}
	}
}
