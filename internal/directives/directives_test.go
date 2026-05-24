package directives_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/directives"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDirectivesSource verifies that the embed.FS carries each stub
// this package ships, with the YAML front matter the hover loader
// strips. The cross-package invariant — every value in the LSP hover
// provider's directiveToDocFile must resolve in this FS — is enforced
// by TestDirectiveToDocFileResolves in internal/lsp/hover_test.go,
// next to the map.
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
