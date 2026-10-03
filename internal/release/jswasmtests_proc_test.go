//go:build unix || windows

package release

// These tests run the real go toolchain, which a js/wasm test binary
// cannot spawn.

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOSGoRunner(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, osGoRunner(t.TempDir())(&out, nil, "env", "GOROOT"))
	assert.NotEmpty(t, strings.TrimSpace(out.String()))

	out.Reset()
	require.NoError(t, osGoRunner(t.TempDir())(&out, []string{"GOOS=js", "GOARCH=wasm"}, "env", "GOOS"))
	assert.Equal(t, "js", strings.TrimSpace(out.String()))

	assert.Error(t, osGoRunner(t.TempDir())(io.Discard, nil, "no-such-go-subcommand"))
}

// TestRunJSWasmTests drives the production wiring against a
// package with no js/wasm-only test files. It runs the real go env
// and go list but stops before go test, so it needs no Node.
func TestRunJSWasmTests(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	err = RunJSWasmTests(root, "./internal/release", &bytes.Buffer{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no js/wasm-only test files")
}

// TestListJSOnlyFilesHostIndependent runs the real go list on a module
// with cgo, !cgo and !linux test files beside a js-tagged one. Whatever
// the host's CGO_ENABLED or OS, both lists run with cgo off, so the cgo
// file is in neither and the !cgo file is shared, and the native list
// targets linux, so the !linux file is js/wasm-only.
func TestListJSOnlyFilesHostIndependent(t *testing.T) {
	dir, _ := newJSWasmFixture(t, map[string]string{
		"go.mod":         "module example.com/p\ngo 1.25\n",
		"p.go":           "package p\n",
		"cgo_test.go":    "//go:build cgo\npackage p\n",
		"nocgo_test.go":  "//go:build !cgo\npackage p\n",
		"notlnx_test.go": "//go:build !linux\npackage p\n",
		"js_test.go":     "//go:build js && wasm\npackage p\n",
	})
	// Base names: go list reports its own view of the directory, which
	// can differ from t.TempDir() (macOS's /var is a symlink to
	// /private/var; Windows joins with a "/" and may shorten names).
	want := []string{"js_test.go", "notlnx_test.go"}
	// "" leaves cgo at the host default: on with a C compiler.
	for _, cgo := range []string{"", "0", "1"} {
		t.Run("CGO_ENABLED="+cgo, func(t *testing.T) {
			t.Setenv("CGO_ENABLED", cgo)
			got, err := listJSOnlyFiles(osJSWasmDeps(dir, io.Discard), "m", ".")
			require.NoError(t, err)
			names := make([]string, len(got))
			for i, f := range got {
				names[i] = filepath.Base(f)
			}
			assert.Equal(t, want, names)
		})
	}
}

// TestRunJSWasmPackage drives the production wiring against a
// package that does not exist: the js/wasm go list fails, before go
// test or Node runs.
func TestRunJSWasmPackage(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	// go list names the load failure in both sub-modes, before go test
	// (or, under requireJSOnly, a misleading "no js/wasm-only Test").
	for _, requireJSOnly := range []bool{false, true} {
		err = RunJSWasmPackage(root, "./internal/does-not-exist", requireJSOnly, &bytes.Buffer{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "go list (js/wasm) ./internal/does-not-exist")
	}
}
