//go:build js && wasm

package main

import (
	"runtime/debug"
	"syscall/js"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Not parallel: it writes the package-level version and readBuildInfo.
func TestResolveVersion(t *testing.T) {
	oldVersion, oldRead := version, readBuildInfo
	t.Cleanup(func() { version, readBuildInfo = oldVersion, oldRead })

	stub := func(info *debug.BuildInfo, ok bool) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) { return info, ok }
	}

	t.Run("set version wins over build info", func(t *testing.T) {
		version = "v9.9.9"
		readBuildInfo = stub(&debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}}, true)
		assert.Equal(t, "v9.9.9", resolveVersion())
	})

	t.Run("empty version uses build info main version", func(t *testing.T) {
		version = ""
		readBuildInfo = stub(&debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}}, true)
		assert.Equal(t, "v1.2.3", resolveVersion())
	})

	t.Run("empty build info version falls back to devel", func(t *testing.T) {
		version = ""
		readBuildInfo = stub(&debug.BuildInfo{}, true)
		assert.Equal(t, "(devel)", resolveVersion())
	})

	t.Run("missing build info falls back to devel", func(t *testing.T) {
		version = ""
		readBuildInfo = stub(nil, false)
		assert.Equal(t, "(devel)", resolveVersion())
	})
}

func TestWorkspaceFromJS(t *testing.T) {
	t.Run("non-object yields nil", func(t *testing.T) {
		assert.Nil(t, workspaceFromJS(js.ValueOf("x")))
		assert.Nil(t, workspaceFromJS(js.Undefined()))
		// JS typeof reports "object" for null (syscall/js reports
		// TypeNull); the guard must still reject it.
		assert.Nil(t, workspaceFromJS(js.Null()))
		assert.Nil(t, workspaceFromJS(js.ValueOf(3)))
	})

	// typeof reports "object" for an array too; Object.keys would
	// turn its indices into file paths "0", "1", ...
	t.Run("array yields nil", func(t *testing.T) {
		assert.Nil(t, workspaceFromJS(js.ValueOf([]any{"# A"})))
		assert.Nil(t, workspaceFromJS(js.ValueOf([]any{})))
	})

	// A boxed String is array-like too: Object.keys gives "0", "1", ...
	// with one-character string values.
	t.Run("boxed string yields nil", func(t *testing.T) {
		assert.Nil(t, workspaceFromJS(js.Global().Get("String").New("# A")))
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

func TestIsRecord(t *testing.T) {
	tests := []struct {
		name string
		v    js.Value
		want bool
	}{
		{"plain object", js.ValueOf(map[string]any{"a": 1}), true},
		{"empty object", js.ValueOf(map[string]any{}), true},
		{"null prototype", js.Global().Get("Object").Call("create", js.Null()), true},
		{"array", js.ValueOf([]any{"a"}), false},
		{"empty array", js.ValueOf([]any{}), false},
		{"boxed string", js.Global().Get("String").New("ab"), false},
		{"map", js.Global().Get("Map").New(), false},
		{"null", js.Null(), false},
		{"undefined", js.Undefined(), false},
		{"string", js.ValueOf("a"), false},
		{"number", js.ValueOf(1), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isRecord(tt.v))
		})
	}
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

// awaitPromise blocks until p settles and reports its value and whether
// it rejected. The Go js/wasm runtime yields to the JS event loop while
// the channel receive blocks, so the then-callbacks can run.
func awaitPromise(t *testing.T, p js.Value) (js.Value, bool) {
	t.Helper()
	type settled struct {
		v        js.Value
		rejected bool
	}
	ch := make(chan settled, 1)
	onResolve := js.FuncOf(func(_ js.Value, a []js.Value) any {
		ch <- settled{a[0], false}
		return nil
	})
	onReject := js.FuncOf(func(_ js.Value, a []js.Value) any {
		ch <- settled{a[0], true}
		return nil
	})
	defer onResolve.Release()
	defer onReject.Release()
	p.Call("then", onResolve, onReject)
	r := <-ch
	return r.v, r.rejected
}

func TestCreateSession(t *testing.T) {
	obj := func(m map[string]any) js.Value { return js.ValueOf(m) }

	rejects := []struct {
		name    string
		args    []js.Value
		wantMsg string
	}{
		{"no args", nil, "createSession requires an options object"},
		{"string options", []js.Value{js.ValueOf("x")}, "createSession requires an options object"},
		{"null options", []js.Value{js.Null()}, "createSession requires an options object"},
		{"array options", []js.Value{js.ValueOf([]any{})}, "createSession requires an options object"},
		{"empty array workspace", []js.Value{obj(map[string]any{"workspace": []any{}})}, "createSession options.workspace must be an object of path to source strings"},
		{"array workspace", []js.Value{obj(map[string]any{"workspace": []any{"# A"}})}, "createSession options.workspace must be an object of path to source strings"},
		{"null workspace", []js.Value{obj(map[string]any{"workspace": nil})}, "createSession options.workspace must be an object of path to source strings"},
		{"string workspace", []js.Value{obj(map[string]any{"workspace": "a.md"})}, "createSession options.workspace must be an object of path to source strings"},
		{"boxed string workspace", []js.Value{obj(map[string]any{"workspace": js.Global().Get("String").New("# A")})}, "createSession options.workspace must be an object of path to source strings"},
		{"null configYAML", []js.Value{obj(map[string]any{"configYAML": nil})}, "createSession options.configYAML must be a string"},
		{"byte configYAML", []js.Value{obj(map[string]any{"configYAML": js.Global().Get("Uint8Array").New(2)})}, "createSession options.configYAML must be a string"},
	}
	for _, tt := range rejects {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			v, rejected := awaitPromise(t, createSession(js.Undefined(), tt.args).(js.Value))
			require.True(t, rejected, "promise must reject")
			assert.Equal(t, tt.wantMsg, v.Get("message").String())
		})
	}

	resolves := []struct {
		name string
		opts map[string]any
	}{
		{"absent workspace", map[string]any{}},
		{"object workspace", map[string]any{"workspace": map[string]any{"a.md": "# A\n"}}},
		{"string configYAML", map[string]any{"configYAML": ""}},
	}
	for _, tt := range resolves {
		t.Run("resolves "+tt.name, func(t *testing.T) {
			v, rejected := awaitPromise(t, createSession(js.Undefined(), []js.Value{obj(tt.opts)}).(js.Value))
			require.False(t, rejected, "promise must resolve: %v", v)
			assert.Equal(t, js.TypeFunction, v.Get("check").Type())
			v.Call("dispose")
		})
	}
}
