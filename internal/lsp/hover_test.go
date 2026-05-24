package lsp

import (
	"io/fs"
	"testing"

	"github.com/jeduden/mdsmith/internal/directives"
	"github.com/stretchr/testify/assert"
)

// TestDirectiveToDocFileResolves walks directiveToDocFile and asserts
// every mapped filename resolves to a non-empty file in directives.FS.
// Adding a directive→file mapping without shipping the stub would
// otherwise return an empty payload at hover time.
func TestDirectiveToDocFileResolves(t *testing.T) {
	for name, fname := range directiveToDocFile {
		t.Run(name, func(t *testing.T) {
			data, err := fs.ReadFile(directives.FS, fname)
			assert.NoError(t, err, "directive %q maps to %q but stub is missing", name, fname)
			assert.NotEmpty(t, data, "directive %q stub %q must have content", name, fname)
		})
	}
}
