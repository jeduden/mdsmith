package concepts

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripFrontMatter_NoFrontMatter(t *testing.T) {
	result := stripFrontMatter("# Title\n\nBody text.\n")
	assert.Equal(t, "# Title\n\nBody text.\n", result)
}

func TestStripFrontMatter_UnclosedFrontMatter(t *testing.T) {
	result := stripFrontMatter("---\nkey: value\n")
	assert.Equal(t, "---\nkey: value\n", result)
}
