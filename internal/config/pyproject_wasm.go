//go:build wasm

package config

import "fmt"

// loadPyproject is unavailable in the WebAssembly engine: the WASM host
// passes config as inline YAML text, and the go-toml parser is kept out
// of the artifact to hold its size budget.
func loadPyproject(path string) (*Config, error) {
	return nil, positionError(
		fmt.Errorf("%s: pyproject.toml config is not supported in the WebAssembly build", path),
		path, nil)
}
