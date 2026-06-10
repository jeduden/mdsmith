package cuelitetest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRunAccess_corpus is the CI-visible differential accessor run: every
// AccessCase must classify identically through the cuelite façade and the
// CUE oracle. It mirrors RunPath/Run for the LookupPath/Fields/Exists/
// String reads that surfaces A and B depend on.
func TestRunAccess_corpus(t *testing.T) {
	RunAccess(t, accessCorpus())
}

// TestAccessOutcome_Equal pins the comparison the differential arm relies
// on: agreement requires the same existence, the same looked-up string
// (and string-ness), and the same field set; a nil field set equals an
// empty one.
func TestAccessOutcome_Equal(t *testing.T) {
	base := AccessOutcome{Exists: true, Str: "x", HasStr: true, Fields: []string{"a"}}
	assert.True(t, base.Equal(base))
	assert.False(t, base.Equal(AccessOutcome{Exists: false, Str: "x", HasStr: true, Fields: []string{"a"}}))
	assert.False(t, base.Equal(AccessOutcome{Exists: true, Str: "y", HasStr: true, Fields: []string{"a"}}))
	assert.False(t, base.Equal(AccessOutcome{Exists: true, Str: "x", HasStr: false, Fields: []string{"a"}}))
	assert.False(t, base.Equal(AccessOutcome{Exists: true, Str: "x", HasStr: true, Fields: []string{"b"}}))
	assert.True(t,
		AccessOutcome{Fields: nil}.Equal(AccessOutcome{Fields: []string{}}),
		"nil fields must equal empty fields")
}

// TestCueLiteAccess_compileGuard pins that a non-JSON data document yields
// the zero AccessOutcome through the cuelite arm, the same guard the oracle
// arm applies — so a compile failure agrees rather than diverging.
func TestCueLiteAccess_compileGuard(t *testing.T) {
	c := AccessCase{Name: "bad", Data: `{not json`, Segments: []string{"a"}}
	assert.Equal(t, AccessOutcome{}, CueLiteAccess(c))
	assert.Equal(t, AccessOutcome{}, OracleAccess(c))
}

// TestCompareAccess_disagreement pins CompareAccess's failure path: two
// arms that disagree record one Errorf and return false. A recorder
// captures the failure instead of failing the test.
func TestCompareAccess_disagreement(t *testing.T) {
	agree := func(AccessCase) AccessOutcome { return AccessOutcome{Exists: true} }
	disagree := func(AccessCase) AccessOutcome { return AccessOutcome{Exists: false} }
	rec := &recorder{}
	ok := CompareAccess(rec, agree, disagree, AccessCase{Name: "x"})
	assert.False(t, ok)
	assert.Len(t, rec.failures, 1)
}
