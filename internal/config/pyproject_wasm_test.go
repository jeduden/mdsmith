//go:build wasm

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadPyproject_UnsupportedInWASM(t *testing.T) {
	_, err := loadPyproject("pyproject.toml")
	assert.ErrorContains(t, err, "not supported in the WebAssembly build")
}
