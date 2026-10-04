package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestWASMDepsExcludeTOML pins that the pyproject.toml config source
// (go-toml) is linked only into native builds: the WASM host passes
// config as inline YAML, and go-toml would cost artifact size. It lists
// the engine's dependency graph for GOOS=js GOARCH=wasm, with and
// without the tinygo build tag.
func TestWASMDepsExcludeTOML(t *testing.T) {
	for _, tags := range []string{"", "tinygo"} {
		args := []string{"list", "-deps"}
		if tags != "" {
			args = append(args, "-tags", tags)
		}
		cmd := exec.Command("go", append(args, ".")...)
		cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("go list (tags %q): %v", tags, err)
		}
		if strings.Contains(string(out), "github.com/pelletier/go-toml") {
			t.Errorf("go-toml is linked into the WASM engine (tags %q)", tags)
		}
	}
}
