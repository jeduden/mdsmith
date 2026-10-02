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
	const (
		wsMsg  = "createSession options.workspace must be an object of path to source strings"
		cfgMsg = "createSession options.configYAML must be a string"
	)

	rejects := []struct {
		name    string
		args    []js.Value
		wantMsg string
	}{
		{"no args", nil, "createSession requires an options object"},
		{"string options", []js.Value{js.ValueOf("x")}, "createSession requires an options object"},
		{"null options", []js.Value{js.Null()}, "createSession requires an options object"},
		{"array options", []js.Value{js.ValueOf([]any{})}, "createSession requires an options object"},
		{"empty array workspace", []js.Value{obj(map[string]any{"workspace": []any{}})}, wsMsg},
		{"array workspace", []js.Value{obj(map[string]any{"workspace": []any{"# A"}})}, wsMsg},
		{"null workspace", []js.Value{obj(map[string]any{"workspace": nil})}, wsMsg},
		{"string workspace", []js.Value{obj(map[string]any{"workspace": "a.md"})}, wsMsg},
		{"boxed string workspace",
			[]js.Value{obj(map[string]any{"workspace": js.Global().Get("String").New("# A")})}, wsMsg},
		{"null configYAML", []js.Value{obj(map[string]any{"configYAML": nil})}, cfgMsg},
		{"byte configYAML",
			[]js.Value{obj(map[string]any{"configYAML": js.Global().Get("Uint8Array").New(2)})}, cfgMsg},
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

// newTestProxy resolves createSession over an empty workspace and
// returns the session proxy.
func newTestProxy(t *testing.T) js.Value {
	t.Helper()
	opts := js.ValueOf(map[string]any{})
	v, rejected := awaitPromise(t, createSession(js.Undefined(), []js.Value{opts}).(js.Value))
	require.False(t, rejected, "promise must resolve: %v", v)
	return v
}

// TestNewSessionProxy_KeysMatchSessionMethodNames ties the proxy's real
// keys to sessionMethodNames, the list the native parity test checks
// against the Go Session, so a key added to or dropped from
// newSessionProxy alone cannot drift past that test.
func TestNewSessionProxy_KeysMatchSessionMethodNames(t *testing.T) {
	proxy := newTestProxy(t)
	defer proxy.Call("dispose")
	keys := js.Global().Get("Object").Call("keys", proxy)
	got := make([]string, keys.Length())
	for i := range got {
		got[i] = keys.Index(i).String()
	}
	assert.ElementsMatch(t, sessionMethodNames(), got)
}

// TestNewSessionProxy_DisposeReleasesMethods checks that dispose()
// releases the session's own method funcs, so syscall/js's handler
// table stops pinning the Session, while every method keeps its return
// shape: async methods reject with "session disposed", capabilities()
// is empty, invalidate() does nothing, and a second dispose() is a
// no-op. No call reaches a released func.
func TestNewSessionProxy_DisposeReleasesMethods(t *testing.T) {
	proxy := newTestProxy(t)
	require.Equal(t, js.TypeObject, proxy.Call("check", "a.md", "# A\n").Type(),
		"a live check returns a Promise")
	require.Equal(t, js.TypeObject, proxy.Call("capabilities").Type(),
		"a live capabilities returns an array")
	liveCheck := proxy.Get("check")

	proxy.Call("dispose")

	// dispose now points at the shared no-op stand-in, so a second
	// session.dispose() never reaches its own released func (which would
	// return undefined too, but log "call to released function").
	assert.True(t, proxy.Get("dispose").Equal(disposedFunc("dispose").Value),
		"dispose points at the shared no-op stand-in")

	// The session's own func is released: invoking it returns undefined
	// (syscall/js logs "call to released function" for this one call).
	assert.True(t, liveCheck.Invoke("a.md", "# A\n").IsUndefined(), "released check func")

	for _, m := range [][]any{
		{"check", "a.md", "# A\n"},
		{"fix", "a.md", "# A\n"},
		{"kinds", "a.md"},
		{"rename", "a.md", "1", "B", ""},
		{"move", "a.md", "b.md"},
	} {
		p := proxy.Call(m[0].(string), m[1:]...)
		require.Equal(t, js.TypeObject, p.Type(), "%s after dispose returns a Promise", m[0])
		v, rejected := awaitPromise(t, p)
		assert.True(t, rejected, "%s after dispose rejects", m[0])
		assert.Equal(t, "session disposed", v.Get("message").String(), m[0])
	}
	caps := proxy.Call("capabilities")
	require.True(t, caps.InstanceOf(js.Global().Get("Array")), "capabilities after dispose")
	assert.Equal(t, 0, caps.Length())
	assert.True(t, proxy.Call("invalidate", "a.md").IsUndefined(), "invalidate after dispose")
	assert.NotPanics(t, func() { proxy.Call("dispose") }, "second dispose")
}

// TestNewSessionProxy_DisposeLeavesNoFuncs tracks the funcs a session's
// lifecycle (proxy methods and Promise executors) registers and
// releases through the funcOf and releaseFunc seams. After a warm-up
// cycle, N more create/dispose cycles must leave the live set the same
// size, so a restart loop does not grow syscall/js's handler table.
// Each func is tracked by its JS wrapper rather than a bare counter, so
// releasing the wrong func (or one twice) cannot cancel out a leak.
// Not parallel: it swaps package seams.
func TestNewSessionProxy_DisposeLeavesNoFuncs(t *testing.T) {
	oldOf, oldRelease := funcOf, releaseFunc
	t.Cleanup(func() { funcOf, releaseFunc = oldOf, oldRelease })
	var live []js.Value
	// strays counts releases of a func not in live. It is asserted after
	// the cycles, not reported with t.Errorf in releaseFunc: test output
	// waits on the JS event loop, which a running callback blocks.
	strays := 0
	funcOf = func(fn func(js.Value, []js.Value) any) js.Func {
		f := oldOf(fn)
		live = append(live, f.Value)
		return f
	}
	releaseFunc = func(f js.Func) {
		oldRelease(f)
		for i, v := range live {
			if v.Equal(f.Value) {
				live = append(live[:i], live[i+1:]...)
				return
			}
		}
		strays++
	}

	cycle := func() {
		proxy := newTestProxy(t)
		awaitPromise(t, proxy.Call("check", "a.md", "# A\n"))
		proxy.Call("dispose")
	}
	cycle() // warm-up: creates the shared disposed stand-ins if no earlier test did
	base := len(live)
	for i := 0; i < 5; i++ {
		cycle()
	}
	assert.Len(t, live, base, "live funcs after 5 more create/dispose cycles")
	assert.Zero(t, strays, "releases of a func that was not live (released twice, or registered outside funcOf)")
}
