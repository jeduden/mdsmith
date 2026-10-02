//go:build js && wasm

package main

import (
	"runtime/debug"
	"syscall/js"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Not parallel: it writes the package-level version.
func TestResolveVersion(t *testing.T) {
	old := version
	t.Cleanup(func() { version = old })

	t.Run("set version wins", func(t *testing.T) {
		version = "v9.9.9"
		assert.Equal(t, "v9.9.9", resolveVersion())
	})

	t.Run("empty version falls back to build info", func(t *testing.T) {
		version = ""
		want := "(devel)"
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
			want = info.Main.Version
		}
		assert.Equal(t, want, resolveVersion())
	})
}

func TestWorkspaceFromJS(t *testing.T) {
	t.Run("non-object yields nil", func(t *testing.T) {
		assert.Nil(t, workspaceFromJS(js.ValueOf("x")))
		assert.Nil(t, workspaceFromJS(js.Undefined()))
		assert.Nil(t, workspaceFromJS(js.ValueOf(3)))
	})

	t.Run("keeps string entries and drops others", func(t *testing.T) {
		got := workspaceFromJS(js.ValueOf(map[string]any{
			"a.md": "# A\n",
			"n":    1,
			"b.md": "",
		}))
		require.NotNil(t, got)
		assert.Equal(t, map[string][]byte{
			"a.md": []byte("# A\n"),
			"b.md": []byte(""),
		}, got)
	})

	t.Run("empty object yields empty map", func(t *testing.T) {
		got := workspaceFromJS(js.ValueOf(map[string]any{}))
		require.NotNil(t, got)
		assert.Empty(t, got)
	})
}

func TestURIAndSource(t *testing.T) {
	tests := []struct {
		name string
		args []js.Value
		ok   bool
	}{
		{"no args", nil, false},
		{"one arg", []js.Value{js.ValueOf("a")}, false},
		{"non-string uri", []js.Value{js.ValueOf(1), js.ValueOf("s")}, false},
		{"non-string source", []js.Value{js.ValueOf("a"), js.ValueOf(1)}, false},
		{"two strings", []js.Value{js.ValueOf("a.md"), js.ValueOf("# T\n")}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uri, src, ok := uriAndSource(tt.args)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, "a.md", uri)
				assert.Equal(t, []byte("# T\n"), src)
			} else {
				assert.Empty(t, uri)
				assert.Nil(t, src)
			}
		})
	}
}

func TestAllStrings(t *testing.T) {
	assert.True(t, allStrings(nil))
	assert.True(t, allStrings([]js.Value{js.ValueOf("a"), js.ValueOf("b")}))
	assert.False(t, allStrings([]js.Value{js.ValueOf("a"), js.ValueOf(1)}))
	assert.False(t, allStrings([]js.Value{js.Null()}))
}
