//go:build !wasm

package main

import "github.com/jeduden/mdsmith/internal/refactor"

// executeFileOp runs op's file relocation against rootDir. It is the
// CLI's only route to refactor.FileOp.Execute, which does not exist
// under wasm; fileop_exec_wasm.go supplies the wasm side, so a package
// the wasm bridge links can never compile a call to Execute.
func executeFileOp(op refactor.FileOp, rootDir string) error {
	return op.Execute(rootDir)
}
