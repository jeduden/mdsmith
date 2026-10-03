//go:build js && wasm

package main

import (
	"crypto/aes"
	"encoding/hex"
	"errors"
	"maps"
	"reflect"
	"runtime"
	"runtime/debug"
	"slices"
	"syscall/js"
	"testing"

	"github.com/jeduden/mdsmith/pkg/mdsmith"
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

// jsValue returns v as a js.Value, failing t when v is anything else,
// so a js.FuncOf-shaped result (typed any) of the wrong type fails the
// test cleanly instead of panicking the js/wasm test binary. Call it
// only from the test goroutine, never inside a JS callback.
func jsValue(t helperT, v any) js.Value {
	t.Helper()
	jv, ok := v.(js.Value)
	require.True(t, ok, "got %T, want js.Value", v)
	return jv
}

// helperT is the part of *testing.T that jsValue uses.
type helperT interface {
	require.TestingT
	Helper()
}

// recordingT is a helperT that records a failure; FailNow ends the
// calling goroutine as *testing.T's does.
type recordingT struct{ failed bool }

func (r *recordingT) Errorf(string, ...any) { r.failed = true }
func (r *recordingT) FailNow()              { r.failed = true; runtime.Goexit() }
func (r *recordingT) Helper()               {}

func TestJSValue(t *testing.T) {
	want := js.ValueOf("x")
	assert.True(t, jsValue(t, want).Equal(want))
	inner := &recordingT{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		jsValue(inner, "not a js.Value")
	}()
	<-done
	assert.True(t, inner.failed, "a non-js.Value fails t")
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
			v, rejected := awaitPromise(t, jsValue(t, createSession(js.Undefined(), tt.args)))
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
			v, rejected := awaitPromise(t, jsValue(t, createSession(js.Undefined(), []js.Value{obj(tt.opts)})))
			require.False(t, rejected, "promise must resolve: %v", v)
			assert.Equal(t, js.TypeFunction, v.Get("check").Type())
			v.Call("dispose")
		})
	}
}

// TestCreateSession_RejectsThrowingObjects passes options and
// workspace objects whose inspection throws in JS (a revoked Proxy, a
// Proxy whose ownKeys trap throws). syscall/js raises that as a
// js.Error panic, which unrecovered would end the Go program and every
// session with it; createSession must reject with the thrown error.
func TestCreateSession_RejectsThrowingObjects(t *testing.T) {
	obj := func(m map[string]any) js.Value { return js.ValueOf(m) }
	proxyCtor := js.Global().Get("Proxy")
	revoked := proxyCtor.Call("revocable", obj(map[string]any{}), obj(map[string]any{}))
	revoked.Call("revoke")
	// An ownKeys trap must return an array-like object; Number returns
	// NaN, so Object.keys on this proxy throws a TypeError.
	throwingKeys := proxyCtor.New(obj(map[string]any{}),
		obj(map[string]any{"ownKeys": js.Global().Get("Number")}))
	for _, tt := range []struct {
		name string
		opts js.Value
	}{
		{"revoked proxy options", revoked.Get("proxy")},
		{"revoked proxy workspace", obj(map[string]any{"workspace": revoked.Get("proxy")})},
		{"throwing ownKeys workspace", obj(map[string]any{"workspace": throwingKeys})},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v, rejected := awaitPromise(t, jsValue(t, createSession(js.Undefined(), []js.Value{tt.opts})))
			require.True(t, rejected, "promise must reject")
			assert.True(t, v.InstanceOf(js.Global().Get("TypeError")), "rejects with the thrown TypeError")
		})
	}
}

// newTestProxy resolves createSession over an empty workspace and
// returns the session proxy.
func newTestProxy(t *testing.T) js.Value {
	t.Helper()
	v, _ := newTestProxyWithID(t)
	return v
}

// newTestProxyWithID is newTestProxy plus the id the new session was
// registered under, found as the one key sessions gained.
func newTestProxyWithID(t *testing.T) (js.Value, int64) {
	t.Helper()
	before := maps.Clone(sessions)
	opts := js.ValueOf(map[string]any{})
	v, rejected := awaitPromise(t, jsValue(t, createSession(js.Undefined(), []js.Value{opts})))
	require.False(t, rejected, "promise must resolve: %v", v)
	for id := range sessions {
		if _, had := before[id]; !had {
			return v, id
		}
	}
	require.Fail(t, "createSession registered no new session")
	return v, 0
}

// TestRegisterSession_KeysMatchSessionMethodNames ties the proxy's real
// keys to sessionMethodNames, the list the native parity test checks
// against the Go Session, so a key added to or dropped from
// registerSession alone cannot drift past that test. The order must
// match too, so Object.keys(session) is the same for every session.
func TestRegisterSession_KeysMatchSessionMethodNames(t *testing.T) {
	proxy := newTestProxy(t)
	defer proxy.Call("dispose")
	keys := js.Global().Get("Object").Call("keys", proxy)
	got := make([]string, keys.Length())
	for i := range got {
		got[i] = keys.Index(i).String()
	}
	assert.Equal(t, sessionMethodNames(), got)
}

// TestMethodTable_PanicsOnIncompleteEntry checks that methodTable
// rejects an entry with a nil call or disposed func, naming it, so a
// hand-written methodImpl literal fails at package init rather than
// panicking inside a js.FuncOf callback on its first call. A complete
// table comes back unchanged.
func TestMethodTable_PanicsOnIncompleteEntry(t *testing.T) {
	ok := voidMethod(func(*mdsmith.Session, []js.Value) {})
	assert.PanicsWithValue(t, "methodTable: x has no call func", func() {
		methodTable(map[string]methodImpl{"x": {disposed: ok.disposed}})
	})
	assert.PanicsWithValue(t, "methodTable: x has no disposed func", func() {
		methodTable(map[string]methodImpl{"x": {call: ok.call}})
	})
	got := methodTable(map[string]methodImpl{"x": ok})
	require.Contains(t, got, "x")
	assert.NotNil(t, got["x"].call)
}

// asyncMethodNames lists the session methods engine-api.md documents
// as returning a Promise. Tests that check the "session disposed"
// rejection range over it; TestAsyncMethodNames_MatchTable keeps it in
// step with sharedMethodImpls.
var asyncMethodNames = []string{"check", "fix", "kinds", "rename", "move"}

// TestAsyncMethodNames_MatchTable checks that a table entry's disposed
// result is a Promise exactly when its name is in asyncMethodNames, so
// a new async method cannot skip the tests that range over that list.
func TestAsyncMethodNames_MatchTable(t *testing.T) {
	for name, impl := range sharedMethodImpls {
		isPromise := settledShape(t, impl.disposed()) == "promise"
		assert.Equal(t, slices.Contains(asyncMethodNames, name), isPromise,
			"%s: disposed result is a Promise iff it is in asyncMethodNames", name)
	}
	for _, name := range asyncMethodNames {
		assert.Contains(t, sharedMethodImpls, name)
	}
}

// TestRegisterSession_DisposeKeepsMethodShapes checks that after
// dispose() every method keeps its return shape: async methods reject
// with "session disposed", capabilities() is empty, invalidate() does
// nothing, and a second dispose() is a no-op.
func TestRegisterSession_DisposeKeepsMethodShapes(t *testing.T) {
	proxy := newTestProxy(t)
	require.Equal(t, js.TypeObject, proxy.Call("check", "a.md", "# A\n").Type(),
		"a live check returns a Promise")
	require.Equal(t, js.TypeObject, proxy.Call("capabilities").Type(),
		"a live capabilities returns an array")

	proxy.Call("dispose")
	assertDisposedShapes(t, proxy)
}

// methodSampleArgs holds well-formed arguments for each forwarding
// session method, so a test can call every method in sharedMethodImpls
// on a live session and reach its real path, not only its argument
// check.
var methodSampleArgs = map[string][]any{
	"check":        {"a.md", "# A\n"},
	"fix":          {"a.md", "# A\n"},
	"kinds":        {"a.md"},
	"rename":       {"a.md", "# A\n", "", "A", "B"},
	"move":         {"a.md", "b.md"},
	"capabilities": {},
	"invalidate":   {"a.md"},
}

// assertDisposedShapes checks the return shape of every method of a
// disposed session object.
func assertDisposedShapes(t *testing.T, proxy js.Value) {
	t.Helper()
	for _, name := range asyncMethodNames {
		p := proxy.Call(name, methodSampleArgs[name]...)
		require.Equal(t, js.TypeObject, p.Type(), "%s after dispose returns a Promise", name)
		v, rejected := awaitPromise(t, p)
		assert.True(t, rejected, "%s after dispose rejects", name)
		assert.Equal(t, "session disposed", v.Get("message").String(), name)
	}
	caps := proxy.Call("capabilities")
	require.True(t, caps.InstanceOf(js.Global().Get("Array")), "capabilities after dispose")
	assert.Equal(t, 0, caps.Length())
	assert.True(t, proxy.Call("invalidate", "a.md").IsUndefined(), "invalidate after dispose")
	assert.True(t, proxy.Call("dispose").IsUndefined(), "second dispose")
}

// TestRegisterSession_DisposedShapeMatchesLive checks, for every method
// in sharedMethodImpls, that a disposed session returns a result of the
// same shape (Promise, array, or other JS type) as the live method. A
// synchronous method that falls back to the rejecting-Promise result
// after dispose fails here, and so does one whose disposed result is
// undefined where the live one is an array. Each method needs an entry
// in methodSampleArgs, so a new method cannot skip the check. A method
// left off the session object fails cleanly instead of panicking the
// test binary.
func TestRegisterSession_DisposedShapeMatchesLive(t *testing.T) {
	for name := range sharedMethodImpls {
		args, ok := methodSampleArgs[name]
		require.True(t, ok, "%s needs sample args in methodSampleArgs", name)
		proxy := newTestProxy(t)
		if !assert.Equal(t, js.TypeFunction, proxy.Get(name).Type(), "%s is not on the session object", name) {
			proxy.Call("dispose")
			continue
		}
		live := settledShape(t, proxy.Call(name, args...))
		proxy.Call("dispose")
		disposed := settledShape(t, proxy.Call(name, args...))
		assert.Equal(t, live, disposed, "%s: result shape after dispose", name)
	}
}

// settledShape names the shape of a method result: "promise" for a
// Promise, "array" for an array, and otherwise its JS type, such as
// "undefined". A Promise is awaited first, so its executor and
// callbacks finish before the next call.
func settledShape(t *testing.T, v js.Value) string {
	t.Helper()
	switch {
	case v.InstanceOf(js.Global().Get("Promise")):
		awaitPromise(t, v)
		return "promise"
	case js.Global().Get("Array").Call("isArray", v).Bool():
		return "array"
	default:
		return v.Type().String()
	}
}

func TestAsyncMethod(t *testing.T) {
	// fn runs inside a Promise executor, a JS callback, where t.Fatal
	// would block on the JS event loop and hang the suite; record the
	// call and assert after the Promise settles instead.
	t.Run("resolves the value as JS", func(t *testing.T) {
		var gotArgs []js.Value
		m := asyncMethod(func(_ *mdsmith.Session, args []js.Value) (any, error) {
			gotArgs = args
			return map[string]any{"n": 1}, nil
		})
		v, rejected := awaitPromise(t, m.call(nil, []js.Value{js.ValueOf("x")}))
		require.False(t, rejected)
		assert.Equal(t, 1, v.Get("n").Int())
		require.Len(t, gotArgs, 1)
		assert.Equal(t, "x", gotArgs[0].String())
	})
	t.Run("rejects with the error message", func(t *testing.T) {
		m := asyncMethod(func(*mdsmith.Session, []js.Value) (any, error) {
			return nil, errors.New("boom")
		})
		v, rejected := awaitPromise(t, m.call(nil, nil))
		require.True(t, rejected)
		assert.Equal(t, "boom", v.Get("message").String())
	})
	t.Run("an error without a code leaves code unset", func(t *testing.T) {
		m := asyncMethod(func(*mdsmith.Session, []js.Value) (any, error) {
			return nil, errors.New("boom")
		})
		v, rejected := awaitPromise(t, m.call(nil, nil))
		require.True(t, rejected)
		assert.True(t, v.Get("code").IsUndefined())
	})
	t.Run("a coded error sets the Error's code", func(t *testing.T) {
		m := asyncMethod(func(*mdsmith.Session, []js.Value) (any, error) {
			return nil, mdsmith.ErrNothingToRename
		})
		v, rejected := awaitPromise(t, m.call(nil, nil))
		require.True(t, rejected)
		assert.Equal(t, "nothing to rename", v.Get("message").String())
		assert.Equal(t, mdsmith.ErrorCodeNothingToRename, v.Get("code").String())
	})
	// That fn never runs after dispose is sharedFunc's job; see
	// TestSharedFunc.
	t.Run("disposed result rejects", func(t *testing.T) {
		m := asyncMethod(func(*mdsmith.Session, []js.Value) (any, error) {
			return nil, nil
		})
		v, rejected := awaitPromise(t, m.disposed())
		require.True(t, rejected)
		assert.Equal(t, disposedAsyncReason, v.Get("message").String())
	})
	// A JS exception fn raises (syscall/js panics with js.Error) must
	// reject the Promise, as in createSession, rather than end the Go
	// program and every session with it.
	t.Run("rejects with a thrown JS exception", func(t *testing.T) {
		m := asyncMethod(func(*mdsmith.Session, []js.Value) (any, error) {
			js.Global().Get("JSON").Call("parse", "{")
			return nil, nil
		})
		v, rejected := awaitPromise(t, m.call(nil, nil))
		require.True(t, rejected)
		assert.True(t, v.InstanceOf(js.Global().Get("SyntaxError")), "rejects with the thrown SyntaxError")
	})
}

// TestProxyArgErrors checks that each async proxy rejects bad arguments
// with its package-level sentinel, so a bad call allocates no new
// error, and that the sentinel names the method's signature. The
// argument check runs before the session is used, so sess is nil.
func TestProxyArgErrors(t *testing.T) {
	for _, tt := range []struct {
		name string
		fn   func(*mdsmith.Session, []js.Value) (any, error)
		want error
		msg  string
	}{
		{"check", proxyCheck, errCheckArgs, "check(uri, src) requires two string arguments"},
		{"fix", proxyFix, errFixArgs, "fix(uri, src) requires two string arguments"},
		{"kinds", proxyKinds, errKindsArgs, "kinds(uri) requires a string argument"},
		{"rename", proxyRename, errRenameArgs, "rename(uri, source, as, old, new) requires five string arguments"},
		{"move", proxyMove, errMoveArgs, "move(src, dst) requires two string arguments"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.fn(nil, []js.Value{js.ValueOf(1)})
			assert.Same(t, tt.want, err)
			assert.EqualError(t, err, tt.msg)
		})
	}
}

func TestStringListMethod(t *testing.T) {
	m := stringListMethod(func(*mdsmith.Session, []js.Value) []string {
		return []string{"a", "b"}
	})
	live := m.call(nil, nil)
	require.Equal(t, "array", settledShape(t, live))
	assert.Equal(t, 2, live.Length())
	assert.Equal(t, "b", live.Index(1).String())

	gone := m.disposed()
	require.Equal(t, "array", settledShape(t, gone))
	assert.Equal(t, 0, gone.Length())
}

// TestMethodConstructors_PanicOnNilFn checks that each constructor
// rejects a nil fn when the table is built (package init), so an entry
// such as asyncMethod(nil) fails at startup with a named cause instead
// of on the first live call, inside a js.FuncOf callback.
func TestMethodConstructors_PanicOnNilFn(t *testing.T) {
	assert.PanicsWithValue(t, "asyncMethod: nil fn", func() { asyncMethod(nil) })
	assert.PanicsWithValue(t, "stringListMethod: nil fn", func() { stringListMethod(nil) })
	assert.PanicsWithValue(t, "voidMethod: nil fn", func() { voidMethod(nil) })
}

func TestVoidMethod(t *testing.T) {
	calls := 0
	m := voidMethod(func(*mdsmith.Session, []js.Value) { calls++ })
	assert.True(t, m.call(nil, nil).IsUndefined())
	assert.Equal(t, 1, calls)
	assert.True(t, m.disposed().IsUndefined())
	assert.Equal(t, 1, calls, "disposed result must not run fn")
}

// TestDisposedReject checks that each call returns a new Promise that
// rejects with Error("session disposed").
func TestDisposedReject(t *testing.T) {
	p, q := disposedReject(), disposedReject()
	assert.False(t, p.Equal(q), "a fresh Promise per call")
	// Await both, so neither is left an unhandled rejection.
	for _, pr := range []js.Value{p, q} {
		v, rejected := awaitPromise(t, pr)
		require.True(t, rejected)
		assert.True(t, v.InstanceOf(js.Global().Get("Error")))
		assert.Equal(t, disposedAsyncReason, v.Get("message").String())
	}
}

// TestDisposedEmptyList checks that each call returns a new empty
// array, so a caller that pushes onto one cannot change the next.
func TestDisposedEmptyList(t *testing.T) {
	a := disposedEmptyList()
	require.Equal(t, "array", settledShape(t, a))
	assert.Equal(t, 0, a.Length())
	a.Call("push", "x")
	assert.Equal(t, 0, disposedEmptyList().Length(), "a fresh array per call")
}

func TestDisposedUndefined(t *testing.T) {
	assert.True(t, disposedUndefined().IsUndefined())
}

// TestRegisterSession_DisposeLeavesNoFuncs tracks the funcs a session's
// lifecycle (Promise executors, plus the shared method funcs on first
// use) registers and releases through the funcOf and releaseFunc seams.
// After a warm-up cycle, N more create/dispose cycles must leave the
// live set the same size, so a restart loop does not grow syscall/js's
// handler table, and must leave the sessions registry the same size, so
// no disposed Session (workspace included) stays reachable from Go.
// Each func is tracked by its JS wrapper rather than a bare counter, so
// releasing the wrong func (or one twice) cannot cancel out a leak.
// Not parallel: it swaps package seams.
func TestRegisterSession_DisposeLeavesNoFuncs(t *testing.T) {
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
		// A late call takes the disposed path, whose Promise
		// executor must be released too.
		awaitPromise(t, proxy.Call("check", "a.md", "# A\n"))
	}
	cycle() // warm-up: registers the shared method funcs if no earlier test did
	base, baseSessions := len(live), len(sessions)
	for i := 0; i < 5; i++ {
		cycle()
	}
	assert.Len(t, live, base, "live funcs after 5 more create/dispose cycles")
	assert.Len(t, sessions, baseSessions, "registered sessions after 5 more create/dispose cycles")
	assert.Zero(t, strays, "releases of a func that was not live (released twice, or registered outside funcOf)")
}

// TestBoundSession checks how a shared func splits its bound session id
// off args. A first arg that is not an integer number only reaches a
// shared func called directly, never through a session object; it must
// look up no session and keep args whole rather than panic in Value.Int
// or truncate a fraction, NaN, Infinity, or an id past 2^53 onto a
// live id.
func TestBoundSession(t *testing.T) {
	proxy, liveID := newTestProxyWithID(t)
	defer proxy.Call("dispose")
	require.NotNil(t, sessions[liveID], "newTestProxyWithID returned a live id")
	src := js.ValueOf("a.md")
	// Random ids can exceed 2^52, where float64 has no .5, so the
	// fraction targets a small id registered by hand.
	const smallID int64 = 7
	require.NotContains(t, sessions, smallID, "precondition: id 7 is free")
	sessions[smallID] = sessions[liveID]
	defer delete(sessions, smallID)
	frac := js.ValueOf(float64(smallID) + 0.5)
	nan := js.Global().Get("NaN")
	inf := js.Global().Get("Infinity")
	huge := js.ValueOf(0x1p64)

	tests := []struct {
		name     string
		args     []js.Value
		wantID   int64
		wantLive bool
		wantRest []js.Value
	}{
		{"no args", nil, 0, false, nil},
		{"string first arg", []js.Value{src}, 0, false, []js.Value{src}},
		{"unknown id", []js.Value{js.ValueOf(-1), src}, -1, false, []js.Value{src}},
		{"live id", []js.Value{js.ValueOf(liveID), src}, liveID, true, []js.Value{src}},
		{"live id alone", []js.Value{js.ValueOf(liveID)}, liveID, true, nil},
		{"fractional live id", []js.Value{frac, src}, 0, false, []js.Value{frac, src}},
		{"NaN", []js.Value{nan, src}, 0, false, []js.Value{nan, src}},
		{"beyond safe integer", []js.Value{huge, src}, 0, false, []js.Value{huge, src}},
		{"Infinity", []js.Value{inf, src}, 0, false, []js.Value{inf, src}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, sess, rest := boundSession(tt.args)
			assert.Equal(t, tt.wantID, id)
			assert.Equal(t, tt.wantLive, sess != nil, "live session found")
			require.Len(t, rest, len(tt.wantRest))
			for i := range rest {
				// Object.is, not ===, so a NaN arg equals itself.
				same := js.Global().Get("Object").Call("is", rest[i], tt.wantRest[i]).Bool()
				assert.True(t, same, "rest[%d]", i)
			}
		})
	}
}

// TestNewPromise_RejectsOnJSError checks that a JS exception raised in
// any executor (a js.Error panic) rejects that Promise with the
// exception rather than ending the Go program, so no caller of
// newPromise (createSession, an async method, or its disposed result)
// has to remember to defer rejectOnJSError itself.
func TestNewPromise_RejectsOnJSError(t *testing.T) {
	p := newPromise(func(_, _ func(any)) {
		js.Global().Get("JSON").Call("parse", "{")
	})
	v, rejected := awaitPromise(t, p)
	require.True(t, rejected)
	assert.True(t, v.InstanceOf(js.Global().Get("SyntaxError")), "rejects with the thrown SyntaxError")
}

// TestRejectOnJSError checks that a deferred rejectOnJSError rejects
// with the JS exception a js.Error panic carries, does nothing without
// a panic, and re-raises any other panic.
func TestRejectOnJSError(t *testing.T) {
	run := func(body func()) (rejected []any) {
		defer rejectOnJSError(func(v any) { rejected = append(rejected, v) })
		body()
		return rejected
	}
	jsErr := js.Global().Get("Error").New("boom")
	got := run(func() { panic(js.Error{Value: jsErr}) })
	require.Len(t, got, 1)
	assert.True(t, jsValue(t, got[0]).Equal(jsErr), "rejects with the thrown JS value")
	assert.Empty(t, run(func() {}), "no panic, no rejection")
	assert.PanicsWithValue(t, "go bug", func() { run(func() { panic("go bug") }) })
}

func TestJSType(t *testing.T) {
	assert.Equal(t, js.TypeString, jsType(js.ValueOf("a")))
	assert.Equal(t, js.TypeNumber, jsType(js.ValueOf(1)))
	assert.Equal(t, js.TypeNull, jsType(js.Null()))
	assert.Equal(t, typeUnknown, jsType(js.Global().Get("BigInt").Invoke(1)))
}

// TestBigIntArgs passes a BigInt, whose typeof syscall/js's Value.Type
// panics on ("bad type flag"), everywhere a caller's value meets a type
// check. A panic in a js.FuncOf callback ends the Go program and every
// session with it, so each check must treat a BigInt as a wrong type.
func TestBigIntArgs(t *testing.T) {
	big := js.Global().Get("BigInt").Invoke(1)
	obj := func(m map[string]any) js.Value { return js.ValueOf(m) }

	t.Run("helpers", func(t *testing.T) {
		assert.False(t, isRecord(big))
		assert.False(t, allStrings([]js.Value{big}))
		_, _, ok := uriAndSource([]js.Value{big, big})
		assert.False(t, ok)
		_, sess, rest := boundSession([]js.Value{big})
		assert.Nil(t, sess)
		assert.Len(t, rest, 1)
		assert.Empty(t, workspaceFromJS(obj(map[string]any{"a.md": big})))
	})

	t.Run("createSession", func(t *testing.T) {
		for _, opts := range []js.Value{
			big,
			obj(map[string]any{"configYAML": big}),
			obj(map[string]any{"workspace": big}),
		} {
			_, rejected := awaitPromise(t, jsValue(t, createSession(js.Undefined(), []js.Value{opts})))
			assert.True(t, rejected)
		}
	})

	t.Run("session methods", func(t *testing.T) {
		proxy := newTestProxy(t)
		defer proxy.Call("dispose")
		for _, m := range asyncMethodNames {
			args := []any{big, big, big, big, big}
			_, rejected := awaitPromise(t, proxy.Call(m, args...))
			assert.True(t, rejected, "%s(BigInt...) rejects", m)
		}
		assert.True(t, proxy.Call("invalidate", big).IsUndefined())
		assert.True(t, proxy.Call("invalidate", "a.md", big).IsUndefined())
		assert.Positive(t, proxy.Call("capabilities").Length(), "session still live")
	})

	t.Run("shared funcs called directly", func(t *testing.T) {
		shared := sharedMethods()
		_, rejected := awaitPromise(t, shared["check"].Invoke(big, "a.md", "# A\n"))
		assert.True(t, rejected)
		assert.True(t, shared["dispose"].Invoke(big).IsUndefined())
	})
}

// TestBindMethods_SkipsNameWithoutSharedFunc checks that a name in the
// method list with no shared func (the two lists drifted) is left off
// the session object rather than throwing in bind on every
// createSession. TestRegisterSession_KeysMatchSessionMethodNames then
// reports the drift.
func TestBindMethods_SkipsNameWithoutSharedFunc(t *testing.T) {
	proxy := js.Global().Get("Object").New()
	require.NotPanics(t, func() {
		bindMethods(proxy, []string{"check", "missing"}, sharedMethods(), -1, js.Undefined())
	})
	assert.Equal(t, js.TypeFunction, proxy.Get("check").Type())
	assert.False(t, proxy.Call("hasOwnProperty", "missing").Bool())
}

// TestRegisterSession_BindThrowRegistersNoSession swaps in a bind that
// throws, as a Function.prototype.bind patched before load would be
// captured. createSession must reject without leaving the Session in
// sessions, where no session object would ever reach dispose().
// Not parallel: it swaps bindTo.
func TestRegisterSession_BindThrowRegistersNoSession(t *testing.T) {
	sharedMethods()
	old := bindTo
	t.Cleanup(func() { bindTo = old })
	bindTo = js.Global().Get("Function").New("throw new TypeError('bind')")
	before := maps.Clone(sessions)
	opts := js.ValueOf(map[string]any{})
	v, rejected := awaitPromise(t, jsValue(t, createSession(js.Undefined(), []js.Value{opts})))
	require.True(t, rejected, "createSession rejects when bind throws")
	assert.True(t, v.InstanceOf(js.Global().Get("TypeError")), "rejects with the thrown TypeError")
	assert.Equal(t, before, sessions, "no session is left registered")
}

// TestSharedFunc checks the dispatch every shared method func runs: a
// live bound id calls impl.call with that session and the remaining
// args, and a disposed id, an unknown id, or no id returns
// impl.disposed() without ever calling impl.call.
func TestSharedFunc(t *testing.T) {
	proxy, liveID := newTestProxyWithID(t)
	// Disposed mid-test too; a second dispose is a no-op, and the defer
	// frees the session if a require stops the test before that.
	defer proxy.Call("dispose")
	live := sessions[liveID]
	require.NotNil(t, live)
	var calls []*mdsmith.Session
	var gotRest []js.Value
	disposedCalls := 0
	f := sharedFunc(methodImpl{
		call: func(sess *mdsmith.Session, args []js.Value) js.Value {
			calls = append(calls, sess)
			gotRest = args
			return js.ValueOf("live")
		},
		disposed: func() js.Value {
			disposedCalls++
			return js.ValueOf("disposed")
		},
	})

	got := jsValue(t, f(js.Undefined(), []js.Value{js.ValueOf(liveID), js.ValueOf("a.md")}))
	assert.Equal(t, "live", got.String())
	require.Len(t, calls, 1)
	assert.Same(t, live, calls[0])
	require.Len(t, gotRest, 1)
	assert.Equal(t, "a.md", gotRest[0].String())

	proxy.Call("dispose")
	for _, args := range [][]js.Value{
		{js.ValueOf(liveID), js.ValueOf("a.md")},
		{js.ValueOf(-1)},
		nil,
	} {
		got := jsValue(t, f(js.Undefined(), args))
		assert.Equal(t, "disposed", got.String())
	}
	assert.Len(t, calls, 1, "impl.call must not run without a live session")
	assert.Equal(t, 3, disposedCalls)
}

// TestSharedMethods_NoSessionID calls each shared func directly with no
// bound id: every method takes the disposed path instead of panicking,
// and dispose does nothing.
func TestSharedMethods_NoSessionID(t *testing.T) {
	shared := sharedMethods()
	require.ElementsMatch(t, sessionMethodNames(), slices.Collect(maps.Keys(shared)))
	before := len(sessions)
	for _, name := range asyncMethodNames {
		v, rejected := awaitPromise(t, shared[name].Invoke("a.md", "# A\n"))
		assert.True(t, rejected, "%s without an id rejects", name)
		assert.Equal(t, "session disposed", v.Get("message").String(), name)
	}
	assert.Equal(t, 0, shared["capabilities"].Invoke().Length())
	assert.True(t, shared["invalidate"].Invoke("a.md").IsUndefined())
	assert.True(t, shared["dispose"].Invoke().IsUndefined())
	assert.Len(t, sessions, before, "dispose without an id drops no session")
}

// TestRegisterSession_IgnoresLaterBindPatch checks that a
// Function.prototype.bind or .call replaced after the engine loaded (by
// another plugin sharing the realm) never receives an unbound shared
// func: the proxy binds through the pair captured at load. The patch is
// undone by a defer, so a failed require in newTestProxy cannot leave
// it in place for later tests.
func TestRegisterSession_IgnoresLaterBindPatch(t *testing.T) {
	warm := newTestProxy(t) // captures bind and registers the shared funcs
	warm.Call("dispose")

	for _, method := range []string{"bind", "call"} {
		t.Run(method, func(t *testing.T) {
			proto := js.Global().Get("Function").Get("prototype")
			orig := proto.Get(method)
			apply := js.Global().Get("Reflect").Get("apply")
			leaks := 0
			spy := js.FuncOf(func(this js.Value, args []js.Value) any {
				argv := make([]any, len(args))
				for i, a := range args {
					argv[i] = a
				}
				for _, v := range append([]js.Value{this}, args...) {
					if isSharedFunc(v) {
						leaks++
					}
				}
				return apply.Invoke(orig, this, js.ValueOf(argv))
			})
			defer spy.Release()
			proxy := func() js.Value {
				proto.Set(method, spy)
				defer proto.Set(method, orig)
				return newTestProxy(t)
			}()
			defer proxy.Call("dispose")

			assert.Zero(t, leaks, "patched %s received a raw shared func", method)
			_, rejected := awaitPromise(t, proxy.Call("check", "a.md", "# A\n"))
			assert.False(t, rejected, "the session built without the patched %s still works", method)
		})
	}
}

// isSharedFunc reports whether v is one of the unbound shared method
// funcs.
func isSharedFunc(v js.Value) bool {
	for _, f := range sharedMethods() {
		if v.Equal(f) {
			return true
		}
	}
	return false
}

// TestRegisterSession_StaleReferencesNeverReachReleasedFuncs checks
// that no call after dispose() reaches a released func (which
// syscall/js logs as "call to released function"). The release seam
// shows dispose releases nothing, so no reference a session object
// handed out can point at a released func; a stored dispose or method
// reference, a frozen session object, and a read-only method then all
// take the same disposed path as the writable object.
// Not parallel: it swaps the releaseFunc seam.
func TestRegisterSession_StaleReferencesNeverReachReleasedFuncs(t *testing.T) {
	object := js.Global().Get("Object")
	for _, tt := range []struct {
		name  string
		setup func(proxy js.Value)
	}{
		{"writable", func(js.Value) {}},
		{"frozen", func(p js.Value) { object.Call("freeze", p) }},
		{"read-only check", func(p js.Value) {
			object.Call("defineProperty", p, "check", map[string]any{"writable": false})
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			released := recordReleases(t)
			proxy := newTestProxy(t)
			tt.setup(proxy)
			d := proxy.Get("dispose")
			check := proxy.Get("check")
			*released = nil // drop releases made while creating the session

			d.Invoke()
			assert.Empty(t, *released, "dispose releases no func")
			assert.True(t, d.Invoke().IsUndefined(), "second call through a stored dispose")
			assert.True(t, proxy.Call("dispose").IsUndefined(), "dispose through the object")
			assertDisposedShapes(t, proxy)

			// A stale method reference rejects like the object's method.
			p := check.Invoke("a.md", "# A\n")
			require.Equal(t, js.TypeObject, p.Type(), "stored check returns a Promise")
			v, rejected := awaitPromise(t, p)
			assert.True(t, rejected, "stored check rejects")
			assert.Equal(t, "session disposed", v.Get("message").String())
		})
	}
}

// recordReleases swaps the releaseFunc seam for one that appends each
// released func's JS wrapper to the returned slice before releasing
// it, and restores the seam when t ends. A caller must not run in
// parallel.
func recordReleases(t *testing.T) *[]js.Value {
	t.Helper()
	oldRelease := releaseFunc
	t.Cleanup(func() { releaseFunc = oldRelease })
	released := new([]js.Value)
	releaseFunc = func(f js.Func) {
		*released = append(*released, f.Value)
		oldRelease(f)
	}
	return released
}

// A same-name rename rejects with code "nothing-to-rename", so a JS
// host can ignore the harmless no-op without matching message text.
func TestProxyRename_NothingToRenameHasCode(t *testing.T) {
	proxy := newTestProxy(t)
	defer proxy.Call("dispose")
	v, rejected := awaitPromise(t, proxy.Call("rename", "a.md", "# A\n", "", "A", "A"))
	require.True(t, rejected)
	assert.Equal(t, `nothing to rename for heading "A"`, v.Get("message").String())
	assert.Equal(t, "nothing-to-rename", v.Get("code").String())

	v, rejected = awaitPromise(t, proxy.Call("rename", "a.md", "# A\n", "", "Z", "B"))
	require.True(t, rejected)
	assert.True(t, v.Get("code").IsUndefined(), "a real failure has no code")
}

func TestJSErrorFor(t *testing.T) {
	e := jsErrorFor(errors.New("boom"))
	assert.True(t, e.InstanceOf(js.Global().Get("Error")))
	assert.Equal(t, "boom", e.Get("message").String())
	assert.True(t, e.Get("code").IsUndefined())

	e = jsErrorFor(mdsmith.ErrNothingToRename)
	assert.Equal(t, mdsmith.ErrorCodeNothingToRename, e.Get("code").String())
}

// TestSharedFunc_GuessedIDsReachNoSession calls a raw shared func as a
// script that captured one could: with every small integer id, and with
// every id within 4096 of one it learned (its own session's). None may
// reach the other live session: ids are a keyed permutation of a
// counter over a 53-bit range, so neither counting up from 0 nor
// stepping from a known id finds it. Plan 2610021439.
func TestSharedFunc_GuessedIDsReachNoSession(t *testing.T) {
	own, ownID := newTestProxyWithID(t)
	defer own.Call("dispose")
	other := newTestProxy(t)
	defer other.Call("dispose")
	raw := sharedMethods()["capabilities"]
	// Positive control: the raw func does reach a session by its id, so
	// an empty result below means a miss, not a broken call path.
	require.Positive(t, raw.Invoke(ownID).Length(), "raw func reaches its own session by id")
	guess := func(id int64) {
		// ownID is the one id this script holds; past maxSessionID a
		// float64 can round back onto it.
		if id == ownID || id > maxSessionID {
			return
		}
		got := raw.Invoke(id)
		assert.Equal(t, 0, got.Length(), "guessed id %d reached a live session", id)
	}
	for id := int64(-1); id <= 4096; id++ {
		guess(id)
	}
	for d := int64(1); d <= 4096; d++ {
		guess(ownID - d)
		guess(ownID + d)
	}
	// Each session's own method still works.
	assert.Positive(t, own.Call("capabilities").Length())
	assert.Positive(t, other.Call("capabilities").Length())
}

// fipsKey is the FIPS-197 appendix C.1 AES-128 key 00 01 … 0f.
var fipsKey = []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}

// TestPermuteSessionID_IsBijection runs the Feistel permutation over
// a 10-bit domain (5-bit halves) and checks every input maps to a
// distinct output inside the domain, so counter values never collide.
func TestPermuteSessionID_IsBijection(t *testing.T) {
	blk := mustCipher(aes.NewCipher(fipsKey))
	const half = 5
	seen := make(map[uint64]bool, 1<<(2*half))
	for x := uint64(0); x < 1<<(2*half); x++ {
		y := permuteSessionID(x, blk, half)
		require.Less(t, y, uint64(1)<<(2*half), "permute(%d) left the domain", x)
		require.False(t, seen[y], "permute(%d) = %d repeats", x, y)
		seen[y] = true
	}
}

// TestRoundSessionID pins the Feistel round function: the low 64 bits,
// little-endian, of AES-128 over the block holding the round index in
// byte 0 and the half little-endian in bytes 1 to 8. The key is
// FIPS-197's, whose C.1 vector checks the cipher itself. The scratch
// block starts dirty, so a byte left from an earlier round would show.
func TestRoundSessionID(t *testing.T) {
	blk := mustCipher(aes.NewCipher(fipsKey))
	var out [16]byte
	pt, err := hex.DecodeString("00112233445566778899aabbccddeeff")
	require.NoError(t, err)
	blk.Encrypt(out[:], pt)
	require.Equal(t, "69c4e0d86a7b0430d8cdb78070b4c55a", hex.EncodeToString(out[:]))
	buf := [aes.BlockSize]byte{0: 0xff, 9: 0xff, 15: 0xff}
	assert.Equal(t, uint64(0x825b8f87373ba1c6), roundSessionID(blk, &buf, 0, 0))
	assert.Equal(t, uint64(0x574272ad725f2164), roundSessionID(blk, &buf, 3, 0x123456))
	assert.Equal(t, uint64(0xb4a429fecc1ba37c), roundSessionID(blk, &buf, 9, 0x7ffffff))
}

// TestMustCipher checks mustCipher returns the block it is given and
// panics on the error aes.NewCipher returns for a bad key length.
func TestMustCipher(t *testing.T) {
	blk, err := aes.NewCipher(fipsKey)
	require.NoError(t, err)
	assert.Same(t, blk, mustCipher(blk, nil))
	assert.Panics(t, func() { mustCipher(aes.NewCipher(make([]byte, 3))) })
}

// TestSessionIDCipher_IsAES128 checks the load-time id cipher is an
// AES block, the PRF the Feistel rounds rely on, and that each
// newSessionIDCipher call draws a fresh key: a fixed key (all zero, or
// a constant) would encrypt the same block the same way every time and
// make every load's ids the same sequence.
func TestSessionIDCipher_IsAES128(t *testing.T) {
	require.NotNil(t, sessionIDCipher)
	assert.Equal(t, aes.BlockSize, sessionIDCipher.BlockSize())
	assert.Equal(t, 10, sessionIDRounds)
	var zero, a, b [aes.BlockSize]byte
	newSessionIDCipher().Encrypt(a[:], zero[:])
	newSessionIDCipher().Encrypt(b[:], zero[:])
	assert.NotEqual(t, a, b, "two keys encrypt the zero block alike")
	zeroKey := mustCipher(aes.NewCipher(make([]byte, 16)))
	var z [aes.BlockSize]byte
	zeroKey.Encrypt(z[:], zero[:])
	assert.NotEqual(t, z, a, "the key is all zero")
}

// TestNewSessionID_NeverRepeats draws 4096 ids in a row and checks none
// repeats, so a method kept from a disposed session never reaches a
// later one, each id is in [1, maxSessionID], and the ids are not a
// counting sequence a script could step through. Not parallel: it
// advances the shared counter.
func TestNewSessionID_NeverRepeats(t *testing.T) {
	seen := make(map[int64]bool, 4096)
	prev := int64(0)
	for range 4096 {
		id := newSessionID()
		require.GreaterOrEqual(t, id, int64(1))
		require.LessOrEqual(t, id, int64(maxSessionID))
		require.False(t, seen[id], "id %d repeats", id)
		assert.NotEqual(t, prev+1, id, "ids count up")
		seen[id] = true
		prev = id
	}
}

// TestNewSessionID_SkipsOutOfRange sets the counter to a value whose
// image is at or past maxSessionID and checks newSessionID moves on to
// the next counter value rather than hand out an id past the range.
// It only moves the counter forward and leaves it there: winding it
// back would hand the same ids out again. Not parallel: it moves the
// shared counter.
func TestNewSessionID_SkipsOutOfRange(t *testing.T) {
	image := func(c uint64) uint64 { return permuteSessionID(c, sessionIDCipher, sessionIDHalfBits) }
	c := sessionIDCounter
	for image(c) < maxSessionID {
		c++
	}
	next := c + 1
	for image(next) >= maxSessionID {
		next++
	}
	sessionIDCounter = c
	assert.Equal(t, int64(image(next))+1, newSessionID())
	assert.Equal(t, next+1, sessionIDCounter)
}

// TestSessionID_Int64On32BitInt pins the session id to int64, so the
// TinyGo build, whose int is 32 bits on wasm, hands ids out from the
// same 2^53 range as standard Go rather than a 2^31 − 1 one a script can sweep.
// Plan 2610021439.
func TestSessionID_Int64On32BitInt(t *testing.T) {
	// The func's result type, not a call, so no id is handed out.
	assert.Equal(t, reflect.Int64, reflect.TypeOf(newSessionID).Out(0).Kind())
	assert.Equal(t, reflect.Int64, reflect.TypeOf(sessions).Key().Kind())
	assert.Equal(t, int64(1)<<53, int64(maxSessionID))
}

// swapPromise replaces globalThis.Promise with ctor for the rest of t.
// A caller must not run in parallel.
func swapPromise(t *testing.T, ctor js.Value) {
	t.Helper()
	g := js.Global()
	old := g.Get("Promise")
	t.Cleanup(func() { g.Set("Promise", old) })
	g.Set("Promise", ctor)
}

// TestCreateSession_ResolveThrowRegistersNoSession replaces Promise with
// a constructor whose resolve throws. The create is rejected, and the
// session registerSession registered is released with it: no proxy
// exists to dispose it. Not parallel: it swaps globalThis.Promise.
func TestCreateSession_ResolveThrowRegistersNoSession(t *testing.T) {
	sharedMethods()
	swapPromise(t, js.Global().Get("Function").New(`executor`,
		`var self = this;
		executor(function () { throw new TypeError("resolve"); },
			function (e) { self.rejection = e; });`))
	before := maps.Clone(sessions)
	opts := js.ValueOf(map[string]any{})
	p := jsValue(t, createSession(js.Undefined(), []js.Value{opts}))
	rej := p.Get("rejection")
	require.True(t, rej.InstanceOf(js.Global().Get("TypeError")), "create rejects with the thrown TypeError")
	assert.Equal(t, before, sessions, "no session is left registered")
}

// TestPromiseCtorThrow_KeepsProgramAndReleasesFunc replaces Promise with
// a constructor that fails: one that throws before it calls the
// executor, one that runs the executor and then throws, and a Promise
// that is no function at all (syscall/js raises that as a
// *js.ValueError, not a js.Error). An async method returns undefined
// instead of ending the program, and the executor's func is released
// exactly once. Not parallel: it swaps Promise and the funcOf and
// releaseFunc seams.
func TestPromiseCtorThrow_KeepsProgramAndReleasesFunc(t *testing.T) {
	fn := js.Global().Get("Function")
	for _, tt := range []struct {
		name string
		ctor js.Value
	}{
		{"throws before the executor", fn.New(`throw new TypeError("ctor");`)},
		{"throws after the executor", fn.New(`executor`,
			`executor(function () {}, function () {}); throw new TypeError("after");`)},
		{"not a function", js.Undefined()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			proxy := newTestProxy(t)
			defer proxy.Call("dispose")
			released := recordReleases(t)
			made := recordFuncs(t)
			swapPromise(t, tt.ctor)

			v := proxy.Call("check", "a.md", "# A\n")
			assert.True(t, v.IsUndefined(), "a failed Promise construction yields undefined")
			require.Len(t, *made, 1, "the executor func was registered")
			require.Len(t, *released, 1, "and released once")
			assert.True(t, (*released)[0].Equal((*made)[0].Value), "the executor func is the one released")
		})
	}
}

// TestNewPromise_BadExecutorArgs replaces Promise with a constructor
// that calls the executor with no resolve or reject, and with a resolve
// and a reject that both throw. The executor callback must not let
// either failure out: a panic that leaves a js.FuncOf callback unwinds
// into the Go frames below the JS that called it. createSession returns,
// no session stays registered, and the executor func is released. Not
// parallel: it swaps Promise and the funcOf and releaseFunc seams.
func TestNewPromise_BadExecutorArgs(t *testing.T) {
	sharedMethods()
	fn := js.Global().Get("Function")
	for _, tt := range []struct {
		name string
		ctor js.Value
	}{
		{"no arguments", fn.New(`executor`, `executor();`)},
		{"resolve and reject throw", fn.New(`executor`,
			`executor(function () { throw new TypeError("resolve"); },
				function () { throw new TypeError("reject"); });`)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			released := recordReleases(t)
			made := recordFuncs(t)
			swapPromise(t, tt.ctor)
			before := maps.Clone(sessions)
			opts := js.ValueOf(map[string]any{})
			v := jsValue(t, createSession(js.Undefined(), []js.Value{opts}))
			assert.Equal(t, js.TypeObject, v.Type(), "createSession returns what the constructor built")
			assert.Equal(t, before, sessions, "no session is left registered")
			require.Len(t, *made, 1, "the executor func was registered")
			assert.Len(t, *released, 1, "and released once")
		})
	}
}

// TestProxyDispose_UnregisterThrows makes the FinalizationRegistry
// unregister call dispose() performs throw, as a patched Reflect.apply
// could. dispose() returns undefined instead of ending the program, and
// the session is disposed all the same. Not parallel: it swaps
// finalizer.unregister.
func TestProxyDispose_UnregisterThrows(t *testing.T) {
	proxy, id := newTestProxyWithID(t)
	defer disposeSession(id)
	old := finalizer.unregister
	t.Cleanup(func() { finalizer.unregister = old })
	finalizer.unregister = js.Global().Get("Function").New(`throw new TypeError("unregister");`)

	assert.True(t, proxy.Call("dispose").IsUndefined())
	assert.NotContains(t, sessions, id, "the session is disposed")
}

// TestCapabilities_ConstructThrows replaces Reflect.construct, which
// wasm_exec.js calls for every Go New, with one that throws. capabilities()
// then fails to build its live result and its disposed [] alike; it
// returns undefined instead of ending the program. Not parallel: it
// swaps Reflect.construct.
func TestCapabilities_ConstructThrows(t *testing.T) {
	proxy := newTestProxy(t)
	defer proxy.Call("dispose")
	reflectObj := js.Global().Get("Reflect")
	construct := reflectObj.Get("construct")
	t.Cleanup(func() { reflectObj.Set("construct", construct) })
	reflectObj.Set("construct", js.Global().Get("Function").New(`throw new TypeError("construct");`))
	v := proxy.Call("capabilities")
	reflectObj.Set("construct", construct) // nothing else runs under the patch

	assert.True(t, v.IsUndefined(), "capabilities() yields undefined")
	assert.Positive(t, proxy.Call("capabilities").Length(), "the session still works")
}

// TestSyncMethodJSException_ReturnsDisposedValue makes the Go-to-JS
// call a sync method performs throw. The method returns its disposed
// value instead of ending the program. Not parallel: it registers a
// session in the shared sessions map.
func TestSyncMethodJSException_ReturnsDisposedValue(t *testing.T) {
	sharedMethods()
	throwing := func(*mdsmith.Session, []js.Value) js.Value {
		panic(js.Error{Value: js.Global().Get("TypeError").New("sync")})
	}
	_, id := newTestProxyWithID(t)
	defer disposeSession(id)
	list := sharedFunc(methodImpl{call: throwing, disposed: disposedEmptyList})
	v := jsValue(t, list(js.Undefined(), []js.Value{js.ValueOf(id)}))
	assert.Equal(t, 0, v.Length(), "capabilities-shaped method returns an empty list")
	void := sharedFunc(methodImpl{call: throwing, disposed: disposedUndefined})
	assert.True(t, jsValue(t, void(js.Undefined(), []js.Value{js.ValueOf(id)})).IsUndefined())
	assert.PanicsWithValue(t, "go bug", func() {
		sharedFunc(methodImpl{
			call:     func(*mdsmith.Session, []js.Value) js.Value { panic("go bug") },
			disposed: disposedUndefined,
		})(js.Undefined(), []js.Value{js.ValueOf(id)})
	}, "a Go panic is re-raised")
}
