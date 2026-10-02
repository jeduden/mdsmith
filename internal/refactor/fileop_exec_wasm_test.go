//go:build wasm

package refactor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFileOpExecute_WasmUnsupported(t *testing.T) {
	err := FileOp{From: "a.md", To: "b.md"}.Execute(t.TempDir())
	assert.EqualError(t, err, "moving a.md: file operations are not supported under wasm")
}
