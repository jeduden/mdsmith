package gensection

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsRawStartMarker_Exact(t *testing.T) {
	assert.True(t, IsRawStartMarker([]byte("<?catalog?>"), "catalog"),
		"expected match for <?catalog?>")
}

func TestIsRawStartMarker_WithBody(t *testing.T) {
	assert.True(t, IsRawStartMarker([]byte("<?catalog glob: plan/*.md"), "catalog"),
		"expected match for <?catalog with body")
}

func TestIsRawStartMarker_WhitespacePrefix(t *testing.T) {
	assert.True(t, IsRawStartMarker([]byte("  <?catalog?>"), "catalog"),
		"expected match with leading whitespace")
}

func TestIsRawStartMarker_NameBoundary(t *testing.T) {
	assert.False(t, IsRawStartMarker([]byte("<?catalogue?>"), "catalog"),
		"should not match <?catalogue?> for name 'catalog'")
}

func TestIsRawStartMarker_NoMatch(t *testing.T) {
	assert.False(t, IsRawStartMarker([]byte("some text"), "catalog"),
		"should not match plain text")
}

func TestIsRawStartMarker_NameOnly(t *testing.T) {
	assert.True(t, IsRawStartMarker([]byte("<?catalog"), "catalog"),
		"expected match for bare <?catalog")
}

func TestIsRawStartMarker_TabAfterName(t *testing.T) {
	assert.True(t, IsRawStartMarker([]byte("<?catalog\tglob: x"), "catalog"),
		"expected match with tab after name")
}

func TestIsRawEndMarker_Exact(t *testing.T) {
	assert.True(t, IsRawEndMarker([]byte("<?/catalog?>"), "catalog"),
		"expected match for <?/catalog?>")
}

func TestIsRawEndMarker_WhitespacePrefix(t *testing.T) {
	assert.True(t, IsRawEndMarker([]byte("  <?/catalog?>"), "catalog"),
		"expected match with leading whitespace")
}

func TestIsRawEndMarker_TrailingContent(t *testing.T) {
	assert.False(t, IsRawEndMarker([]byte("<?/catalog?> extra"), "catalog"),
		"should not match end marker with trailing content")
}

func TestIsRawEndMarker_NoMatch(t *testing.T) {
	assert.False(t, IsRawEndMarker([]byte("<?/include?>"), "catalog"),
		"should not match wrong name")
}
