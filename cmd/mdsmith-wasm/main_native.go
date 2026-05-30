//go:build !(js && wasm)

package main

// This package's real entry point is main.go, tagged `js && wasm`.
// On any other GOOS/GOARCH the command does nothing — it exists only
// so the WebAssembly artifact builds. This stub lets `go build ./...`
// and `go test ./...` resolve the package natively (e.g. for the
// method-set parity test in methods_test.go) without a WASM toolchain.
func main() {}
