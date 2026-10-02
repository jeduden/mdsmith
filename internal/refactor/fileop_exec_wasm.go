//go:build wasm

package refactor

import "fmt"

// Execute is unavailable under wasm: relocating a file needs `git mv`
// or a host filesystem rename, neither of which the engine bundle
// carries. The stub keeps callers such as cmd/mdsmith compiling for
// GOOS=js GOARCH=wasm; it always fails so a move is never half-done.
// The error names op.From, matching the native Execute's "moving %s"
// prefix, so a caller's report says which file did not move.
func (op FileOp) Execute(string) error {
	return fmt.Errorf("moving %s: file operations are not supported under wasm", op.From)
}
