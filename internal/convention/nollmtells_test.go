package convention

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLlmVocabulary(t *testing.T) {
	got := llmVocabulary()
	assert.NotEmpty(t, got)
	assert.Contains(t, got, "delve")
	assert.Contains(t, got, "leverage")
}

func TestLlmPhrases(t *testing.T) {
	got := llmPhrases()
	assert.NotEmpty(t, got)
	assert.Contains(t, got, "it's important to note that")
	assert.Contains(t, got, "in the realm of")
}

func TestLlmVocabularyAndPhrases(t *testing.T) {
	got := llmVocabularyAndPhrases()
	vocab := llmVocabulary()
	phrases := llmPhrases()
	require.Len(t, got, len(vocab)+len(phrases))
	assert.Equal(t, vocab[0], got[0], "vocabulary entries appear first")
	assert.Contains(t, got, "delve")
	assert.Contains(t, got, "it's important to note that")
}

func TestLlmParagraphOpeners(t *testing.T) {
	got := llmParagraphOpeners()
	assert.NotEmpty(t, got)
	assert.Contains(t, got, "Moreover,")
	assert.Contains(t, got, "Certainly,")
}
