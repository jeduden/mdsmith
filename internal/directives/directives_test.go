package directives_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/directives"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDirectivesSource verifies that the embed.FS carries every guide
// file the LSP hover provider references via directiveToDocFile. Adding
// a directive→file mapping without shipping the file would otherwise be
// caught only at hover time, returning an empty payload to the editor.
func TestDirectivesSource(t *testing.T) {
	expected := []string{
		"build.md",
		"enforcing-structure.md",
		"generating-content.md",
	}
	for _, name := range expected {
		t.Run(name, func(t *testing.T) {
			data, err := fs.ReadFile(directives.FS, name)
			require.NoError(t, err)
			assert.NotEmpty(t, data, "stub must have content for hover")
			assert.True(t, strings.HasPrefix(string(data), "---\n"),
				"stub must lead with YAML front matter so the hover loader can strip it")
		})
	}
}
