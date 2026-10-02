//go:build wasm

package main

import (
	"fmt"

	"github.com/jeduden/mdsmith/internal/refactor"
)

// executeFileOp is unavailable under wasm: refactor.FileOp.Execute is
// not compiled there, since relocating a file needs `git mv` or a host
// filesystem rename. The CLI is not a wasm product; this stub only keeps
// `GOOS=js GOARCH=wasm go build ./...` green. It always fails, naming
// op.From like the native "moving %s" errors, so a move is never
// half-done.
func executeFileOp(op refactor.FileOp, _ string) error {
	return fmt.Errorf("moving %s: file operations are not supported under wasm", op.From)
}
