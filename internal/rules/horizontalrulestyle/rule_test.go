package horizontalrulestyle

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBasicDetection(t *testing.T) {
	r := &Rule{
		Style:             "dash",
		Length:            3,
		RequireBlankLines: true,
	}

	// Test 1: Valid horizontal rule
	src := []byte(`# Title

Text before.

---

Text after.
`)
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	diags := r.Check(f)
	assert.Empty(t, diags, "valid horizontal rule should not produce diagnostics")

	// Test 2: Wrong delimiter
	src2 := []byte(`# Title

Text before.

***

Text after.
`)
	f2, err := lint.NewFile("test2.md", src2)
	require.NoError(t, err)

	diags2 := r.Check(f2)
	require.Len(t, diags2, 1)
	assert.Contains(t, diags2[0].Message, "horizontal rule uses asterisk")
}

func TestParseHorizontalRule(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantDelim rune
		wantCount int
		wantSpace bool
	}{
		{"dash", "---", '-', 3, false},
		{"asterisk", "***", '*', 3, false},
		{"underscore", "___", '_', 3, false},
		{"with spaces", "- - -", '-', 3, true},
		{"longer", "-----", '-', 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delim, count, hasSpace := parseHorizontalRule(tt.line)
			assert.Equal(t, tt.wantDelim, delim, "delimiter mismatch")
			assert.Equal(t, tt.wantCount, count, "count mismatch")
			assert.Equal(t, tt.wantSpace, hasSpace, "space detection mismatch")
		})
	}
}
