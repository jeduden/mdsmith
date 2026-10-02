//go:build wasm

package main

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/refactor"
	"github.com/stretchr/testify/assert"
)

func TestExecuteFileOp_WasmUnsupported(t *testing.T) {
	err := executeFileOp(refactor.FileOp{From: "a.md", To: "b.md"}, t.TempDir())
	assert.EqualError(t, err, "moving a.md: file operations are not supported under wasm")
}
