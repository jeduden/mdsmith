package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFrontMatterKinds_NoFrontMatter(t *testing.T) {
	got, err := ParseFrontMatterKinds(nil)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestParseFrontMatterKinds_NoKindsKey(t *testing.T) {
	fm := []byte("---\ntitle: hello\n---\n")
	got, err := ParseFrontMatterKinds(fm)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestParseFrontMatterKinds_ListOfStrings(t *testing.T) {
	fm := []byte("---\nkinds: [plan, proto]\n---\n")
	got, err := ParseFrontMatterKinds(fm)
	require.NoError(t, err)
	assert.Equal(t, []string{"plan", "proto"}, got)
}

func TestParseFrontMatterKinds_BlockSequence(t *testing.T) {
	fm := []byte("---\nkinds:\n  - plan\n  - proto\n---\n")
	got, err := ParseFrontMatterKinds(fm)
	require.NoError(t, err)
	assert.Equal(t, []string{"plan", "proto"}, got)
}

func TestParseFrontMatterKinds_NotAList(t *testing.T) {
	fm := []byte("---\nkinds: plan\n---\n")
	_, err := ParseFrontMatterKinds(fm)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a list")
}

func TestParseFrontMatterKinds_NonStringItem(t *testing.T) {
	fm := []byte("---\nkinds: [plan, 42]\n---\n")
	_, err := ParseFrontMatterKinds(fm)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a string")
}

func TestParseFrontMatterKinds_NullKinds(t *testing.T) {
	fm := []byte("---\nkinds:\n---\n")
	got, err := ParseFrontMatterKinds(fm)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestParseFrontMatterKinds_RejectsAliases(t *testing.T) {
	fm := []byte("---\nbase: &b [a]\nkinds: *b\n---\n")
	_, err := ParseFrontMatterKinds(fm)
	require.Error(t, err)
}
