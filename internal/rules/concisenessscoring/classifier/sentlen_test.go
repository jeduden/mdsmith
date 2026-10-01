package classifier

import (
	"math"
	"math/rand"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var sentPattern = regexp.MustCompile(`[.!?]+`)

// refSentLenVariance is the regexp-based definition SentLenVariance
// must keep matching within float rounding.
func refSentLenVariance(text string) float64 {
	var lengths []float64
	for _, p := range sentPattern.Split(text, -1) {
		if n := len(wordPattern.FindAllString(strings.ToLower(p), -1)); n > 0 {
			lengths = append(lengths, float64(n))
		}
	}
	if len(lengths) < 2 {
		return 0
	}
	var sum float64
	for _, l := range lengths {
		sum += l
	}
	mean := sum / float64(len(lengths))
	var variance float64
	for _, l := range lengths {
		d := l - mean
		variance += d * d
	}
	variance /= float64(len(lengths))
	return math.Sqrt(variance) / mean
}

func TestSentLenVariance_MatchesReference(t *testing.T) {
	inputs := []string{
		"",
		"one sentence only",
		"Short. A much longer second sentence here!",
		"Wait... what?! Really? Yes.",
		"don't stop-believing. it's 42 o'clock",
		"İstanbul is big. KELVIN K sign. Ünïcode wörds.",
		"...!!!???",
		"a. b. c. d e f g h.",
		"bad \xff bytes. here \xfe too",
		"trailing no punct and then. x",
	}
	for _, in := range inputs {
		assert.InDelta(t, refSentLenVariance(in), SentLenVariance(in), 1e-12, "%q", in)
	}
}

func TestSentLenVariance_NoAlloc(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	text := "Short. A much longer second sentence here! Another one follows? Yes."
	allocs := testing.AllocsPerRun(100, func() { SentLenVariance(text) })
	assert.Zero(t, allocs)
}

func TestSentLenVariance_LargeInputFinite(t *testing.T) {
	text := strings.Repeat("a ", 3000) + "." + strings.Repeat("b. ", 3000)
	got := SentLenVariance(text)
	assert.False(t, math.IsNaN(got))
	assert.InDelta(t, refSentLenVariance(text), got, 1e-9)
}

func TestSentLenVariance_RandomMatchesReference(t *testing.T) {
	alphabet := []rune("abcXYZ019' .!?,-\n\u0130\u212A\u00e9\u4e16")
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 5000; i++ {
		rs := make([]rune, rng.Intn(60))
		for j := range rs {
			rs[j] = alphabet[rng.Intn(len(alphabet))]
		}
		in := string(rs)
		assert.InDelta(t, refSentLenVariance(in), SentLenVariance(in), 1e-9, "%q", in)
	}
}
