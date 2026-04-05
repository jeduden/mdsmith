package concisenessscoring

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewScorer(t *testing.T) {
	s, err := NewScorer()
	require.NoError(t, err)
	require.NotNil(t, s)
}

func TestScorer_VerboseText(t *testing.T) {
	s, err := NewScorer()
	require.NoError(t, err)
	text := "Basically, it seems that we are just trying to explain the same idea in order to make it very clear, and it appears that we are really saying very little new information overall."
	result := s.Score(text)
	assert.GreaterOrEqual(t, result.Conciseness, 0.0)
	assert.LessOrEqual(t, result.Conciseness, 1.0)
	assert.Less(t, result.Conciseness, 0.8)
}

func TestScorer_ConciseText(t *testing.T) {
	s, err := NewScorer()
	require.NoError(t, err)
	text := "Run go test ./... and publish checksums for release artifacts."
	result := s.Score(text)
	assert.GreaterOrEqual(t, result.Conciseness, 0.0)
	assert.LessOrEqual(t, result.Conciseness, 1.0)
	assert.Greater(t, result.Conciseness, 0.8)
}

func TestScorer_VerboseLowerThanConcise(t *testing.T) {
	s, err := NewScorer()
	require.NoError(t, err)
	verbose := "Basically, it seems that we are just trying to explain the same idea in order to make it very clear, and it appears that we are really saying very little new information overall."
	concise := "Run go test ./... and publish checksums for release artifacts."
	verboseResult := s.Score(verbose)
	conciseResult := s.Score(concise)
	assert.Less(t, verboseResult.Conciseness, conciseResult.Conciseness)
}

func TestScorer_EmptyText(t *testing.T) {
	s, err := NewScorer()
	require.NoError(t, err)
	result := s.Score("")
	assert.GreaterOrEqual(t, result.Conciseness, 0.0)
	assert.LessOrEqual(t, result.Conciseness, 1.0)
}

func TestScorer_WordCount(t *testing.T) {
	s, err := NewScorer()
	require.NoError(t, err)
	result := s.Score("hello world foo")
	assert.Equal(t, 3, result.WordCount)
}

func TestScorer_Deterministic(t *testing.T) {
	s, err := NewScorer()
	require.NoError(t, err)
	text := "Basically, it seems that we are just trying to explain the same idea."
	first := s.Score(text)
	for i := 0; i < 9; i++ {
		subsequent := s.Score(text)
		assert.True(
			t,
			math.Abs(first.Conciseness-subsequent.Conciseness) < 1e-12,
			"expected deterministic score, got %v vs %v on iteration %d",
			first.Conciseness, subsequent.Conciseness, i+1,
		)
	}
}
