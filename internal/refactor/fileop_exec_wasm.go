//go:build wasm

package refactor

import "errors"

// Execute is unavailable under wasm: relocating a file needs `git mv`
// or a host filesystem rename, neither of which the engine bundle
// carries. The stub keeps callers such as cmd/mdsmith compiling for
// GOOS=js GOARCH=wasm; it always fails so a move is never half-done.
func (op FileOp) Execute(rootDir string) error {
	return errors.New("file operations are not supported under wasm")
}
