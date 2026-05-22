package punkt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOrthoHeuristic_NilToken(t *testing.T) {
	o := &OrthoContext{Storage: NewStorage()}
	assert.Equal(t, 0, o.heuristic(nil, nil),
		"nil token returns 0 (no evidence)")
}

func TestOrthoHeuristic_PunctuationToken(t *testing.T) {
	o := &OrthoContext{Storage: NewStorage()}
	for _, p := range []string{".", "!", "?", ":", ";", ","} {
		assert.Equalf(t, 0, o.heuristic(&Token{Tok: p}, nil),
			"single punctuation token %q returns 0", p)
	}
}

func TestOrthoHeuristic_FirstUpper_TriggersOne(t *testing.T) {
	// First-letter uppercase + orthotype has orthoLc-bits + no
	// orthoMidUc → return 1.
	s := NewStorage()
	s.OrthoContext["next"] = orthoBegLc
	o := &OrthoContext{Storage: s}
	got := o.heuristic(&Token{Tok: "Next"}, make([]byte, 0, 16))
	assert.Equal(t, 1, got,
		"capitalized token with trained lowercase-init orthotype "+
			"must classify as sentence-starter")
}

func TestOrthoHeuristic_FirstLower_TriggersZero(t *testing.T) {
	// Lowercase + orthotype has orthoUc-bits → return 0.
	s := NewStorage()
	s.OrthoContext["next"] = orthoBegUc
	o := &OrthoContext{Storage: s}
	got := o.heuristic(&Token{Tok: "next"}, make([]byte, 0, 16))
	assert.Equal(t, 0, got,
		"lowercase token whose trained type appears uppercase must "+
			"NOT be a sentence-starter")
}

func TestOrthoHeuristic_Unknown_ReturnsNegativeOne(t *testing.T) {
	// FirstUpper but orthotype has no lowercase-init bit (or has
	// orthoMidUc) → return -1.
	s := NewStorage()
	o := &OrthoContext{Storage: s}
	got := o.heuristic(&Token{Tok: "Next"}, make([]byte, 0, 16))
	assert.Equal(t, -1, got,
		"capitalized token with no trained evidence returns unknown")
}

func TestOrthoHeuristic_FirstLowerNoOrthoUcAndBegLcUnset(t *testing.T) {
	// Lowercase + no orthoUc bit, AND orthoBegLc bit unset → return 0
	// (second disjunct of the lowercase branch).
	s := NewStorage()
	// Set midLc but not orthoBegLc, so the begLc bit is clear.
	s.OrthoContext["foo"] = orthoMidLc
	o := &OrthoContext{Storage: s}
	got := o.heuristic(&Token{Tok: "foo"}, make([]byte, 0, 16))
	assert.Equal(t, 0, got,
		"lowercase token with no orthoUc but also no orthoBegLc must "+
			"return 0 (the 'never sentence-initial lowercase' rule)")
}

func TestOrthoHeuristic_StableAcrossManyCalls(t *testing.T) {
	// The pooled typeBuf is reused. Calling heuristic many times in a
	// row with the same input must produce the same answer — drives
	// the buffer-reset contract red/green.
	s := NewStorage()
	s.OrthoContext["next"] = orthoBegLc
	o := &OrthoContext{Storage: s}
	buf := make([]byte, 0, 16)
	for i := 0; i < 10; i++ {
		buf = buf[:0]
		got := o.heuristic(&Token{Tok: "Next"}, buf)
		assert.Equal(t, 1, got, "iter %d", i)
	}
}
