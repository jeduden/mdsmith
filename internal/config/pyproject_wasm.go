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

// probePyproject never reports a config source or a hint in the
// WebAssembly build, so discovery never selects a pyproject.toml there.
func probePyproject(string) (source bool, hint string) { return false, "" }
