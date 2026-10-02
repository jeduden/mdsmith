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

	// Pin the build-info branch rather than mirror resolveVersion's own
	// fallback chain: a test binary always carries build info, so this
	// fails loudly if that ever stops holding instead of silently
	// exercising the "(devel)" literal under this subtest's name.
	t.Run("empty version falls back to build info", func(t *testing.T) {
		version = ""
		info, ok := debug.ReadBuildInfo()
		require.True(t, ok, "test binary must carry build info")
		require.NotEmpty(t, info.Main.Version)
		assert.Equal(t, info.Main.Version, resolveVersion())
	})
}

func TestWorkspaceFromJS(t *testing.T) {
	t.Run("non-object yields nil", func(t *testing.T) {
		assert.Nil(t, workspaceFromJS(js.ValueOf("x")))
		assert.Nil(t, workspaceFromJS(js.Undefined()))
		// JS typeof reports "object" for null; the guard must still
		// reject it.
		assert.Nil(t, workspaceFromJS(js.Null()))
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
		name    string
		args    []js.Value
		wantURI string
		wantSrc []byte
		wantOK  bool
	}{
		{"no args", nil, "", nil, false},
		{"one arg", []js.Value{js.ValueOf("a")}, "", nil, false},
		{"non-string uri", []js.Value{js.ValueOf(1), js.ValueOf("s")}, "", nil, false},
		{"non-string source", []js.Value{js.ValueOf("a"), js.ValueOf(1)}, "", nil, false},
		{"two strings", []js.Value{js.ValueOf("a.md"), js.ValueOf("# T\n")}, "a.md", []byte("# T\n"), true},
		{"extra args ignored", []js.Value{js.ValueOf("b.md"), js.ValueOf(""), js.ValueOf(1)}, "b.md", []byte(""), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uri, src, ok := uriAndSource(tt.args)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantURI, uri)
			assert.Equal(t, tt.wantSrc, src)
		})
	}
}

func TestAllStrings(t *testing.T) {
	tests := []struct {
		name string
		args []js.Value
		want bool
	}{
		{"no args", nil, true},
		{"all strings", []js.Value{js.ValueOf("a"), js.ValueOf("b")}, true},
		{"one number", []js.Value{js.ValueOf("a"), js.ValueOf(1)}, false},
		{"null", []js.Value{js.Null()}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, allStrings(tt.args))
		})
	}
}
