//go:build js && wasm

package main

import (
	"crypto/aes"
	"encoding/hex"
	"errors"
	"maps"
	"math"
	"reflect"
	"runtime"
	"runtime/debug"
	"slices"
	"sync"
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
// lifecycle (the shared method and Promise executor funcs on first use)
// registers through the funcOf seam; the engine releases none
// (TestEngineReleasesNoFunc). After a warm-up cycle, N more
// create/dispose cycles must register no func, so a restart loop does
// not grow syscall/js's handler table, and must leave the sessions
// registry the same size, so no disposed Session (workspace included)
// stays reachable from Go. Not parallel: it swaps the funcOf seam.
func TestRegisterSession_DisposeLeavesNoFuncs(t *testing.T) {
	cycle := func() {
		proxy := newTestProxy(t)
		awaitPromise(t, proxy.Call("check", "a.md", "# A\n"))
		proxy.Call("dispose")
		// A late call takes the disposed path, whose Promise
		// call must not stay pending either.
		awaitPromise(t, proxy.Call("check", "a.md", "# A\n"))
	}
	cycle() // warm-up: registers the shared method funcs if no earlier test did
	made := recordFuncs(t)
	baseSessions := len(sessions)
	for i := 0; i < 5; i++ {
		cycle()
	}
	assert.Empty(t, *made, "funcs registered by 5 more create/dispose cycles")
	assert.Len(t, sessions, baseSessions, "registered sessions after 5 more create/dispose cycles")
	assert.Empty(t, promiseCalls, "no Promise call is left pending")
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

// TestNewPromise_RejectsOnValueError checks that a *js.ValueError the
// executor raises (a Value method on the wrong type, such as Get on
// undefined) rejects the Promise with an Error. The executor callback
// swallows any JS-side failure that escapes rejectOnJSError, so one it
// did not turn into a rejection would leave the Promise pending forever.
func TestNewPromise_RejectsOnValueError(t *testing.T) {
	p := newPromise(func(_, _ func(any)) {
		js.Undefined().Get("x")
	})
	v, rejected := awaitPromise(t, p)
	require.True(t, rejected)
	assert.True(t, v.InstanceOf(js.Global().Get("Error")), "rejects with an Error")
	assert.Contains(t, v.Get("message").String(), "Value.Get")
}

// TestRejectOnJSError checks that a deferred rejectOnJSError rejects
// with the JS exception a js.Error panic carries, and with an Error for
// a *js.ValueError, does nothing without a panic, and re-raises any
// other panic.
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
	got = run(func() { js.Undefined().Get("x") })
	require.Len(t, got, 1)
	assert.True(t, jsValue(t, got[0]).InstanceOf(js.Global().Get("Error")), "a *js.ValueError rejects with an Error")
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
		bindMethods(proxy, newMethodKeys([]string{"check", "missing"}), sharedMethods(), -1, js.Undefined())
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
// syscall/js logs as "call to released function"). The engine releases
// no func (TestEngineReleasesNoFunc), so no reference a session object
// handed out can point at a released func; a stored dispose or method
// reference, a frozen session object, and a read-only method then all
// take the same disposed path as the writable object.
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
			proxy := newTestProxy(t)
			tt.setup(proxy)
			d := proxy.Get("dispose")
			check := proxy.Get("check")

			d.Invoke()
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

// recordUnregister replaces finalizer.unregister for the rest of t with
// a func that records each token it is called with. A caller must not
// run in parallel.
func recordUnregister(t *testing.T) *[]js.Value {
	t.Helper()
	sharedMethods()
	old := finalizer
	got := new([]js.Value)
	rec := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			*got = append(*got, args[0])
		}
		return nil
	})
	t.Cleanup(rec.Release)
	// Registered after rec.Release, so the seam is restored before the
	// recording func is released.
	t.Cleanup(func() { finalizer = old })
	finalizer.unregister = rec.Value
	return got
}

// TestReleaseSession checks releaseSession disposes the session and
// cancels its finalizer entry with the token, and with a token that is
// no object still disposes it but calls no unregister. Not parallel: it
// swaps finalizer.unregister.
func TestReleaseSession(t *testing.T) {
	_, id := newTestProxyWithID(t)
	unregistered := recordUnregister(t)
	tok := js.Global().Get("Object").New()
	releaseSession(id, tok)
	assert.NotContains(t, sessions, id, "the session is disposed")
	require.Len(t, *unregistered, 1)
	assert.True(t, (*unregistered)[0].Equal(tok), "unregister gets the token")

	_, id = newTestProxyWithID(t)
	releaseSession(id, js.Undefined())
	assert.NotContains(t, sessions, id, "disposed without a token too")
	assert.Len(t, *unregistered, 1, "no token, no unregister")
}

// TestCreateSession_ResolveThrowRegistersNoSession replaces Promise with
// a constructor whose resolve throws. The create is rejected, and the
// session registerSession registered is released with it: no proxy
// exists to dispose it. Not parallel: it swaps globalThis.Promise.
func TestCreateSession_ResolveThrowRegistersNoSession(t *testing.T) {
	sharedMethods()
	made := recordFuncs(t)
	unregistered := recordUnregister(t)
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
	assert.Empty(t, *made, "no func is registered per call")
	assert.Empty(t, promiseCalls, "no call is left pending")
	require.Len(t, *unregistered, 1, "the session's finalizer entry is cancelled")
	assert.Equal(t, js.TypeObject, (*unregistered)[0].Type(), "with its token")
}

// TestCreateSession_ReentrantResolveThrowFreesItsOwnSession replaces
// Promise with a constructor whose resolve, on its first call, runs the
// executor again, which registers and resolves a second session, and
// then throws. The first run's guard must free the first session, the
// one whose resolve threw, and leave the second one live. Not parallel:
// it swaps globalThis.Promise.
func TestCreateSession_ReentrantResolveThrowFreesItsOwnSession(t *testing.T) {
	sharedMethods()
	swapPromise(t, js.Global().Get("Function").New(`executor`,
		`var self = this, calls = 0;
		executor(function () {
			if (calls++ > 0) return;
			executor(function (inner) { self.inner = inner; }, function () {});
			throw new TypeError("outer resolve");
		}, function (e) { self.rejection = e; });`))
	before := maps.Clone(sessions)
	opts := js.ValueOf(map[string]any{})
	p := jsValue(t, createSession(js.Undefined(), []js.Value{opts}))
	require.True(t, p.Get("rejection").InstanceOf(js.Global().Get("TypeError")),
		"create rejects with the outer resolve's error")
	inner := p.Get("inner")
	require.Equal(t, js.TypeObject, inner.Type(), "the nested run resolved a session")
	defer inner.Call("dispose")
	assert.Positive(t, inner.Call("capabilities").Length(), "the nested run's session stays live")
	assert.Len(t, sessions, len(before)+1, "only the nested run's session stays registered")
}

// TestCreateSession_CtorThrowAfterReentrantExecutorFreesEverySession
// replaces Promise with a constructor whose resolve runs the executor
// again (the call stays pending until its outer run returns), so
// two sessions are registered, and then throws. Neither session object
// reaches the caller, so both must be freed, not only the last one.
// Not parallel: it swaps globalThis.Promise.
func TestCreateSession_CtorThrowAfterReentrantExecutorFreesEverySession(t *testing.T) {
	sharedMethods()
	unregistered := recordUnregister(t)
	swapPromise(t, js.Global().Get("Function").New(`executor`,
		`var calls = 0;
		executor(function () {
			if (calls++ > 0) return;
			executor(function () {}, function () {});
		}, function () {});
		throw new TypeError("after");`))
	before := maps.Clone(sessions)
	opts := js.ValueOf(map[string]any{})
	v := jsValue(t, createSession(js.Undefined(), []js.Value{opts}))
	assert.True(t, v.IsUndefined(), "a failed Promise construction yields undefined")
	assert.Equal(t, before, sessions, "neither session is left registered")
	assert.Len(t, *unregistered, 2, "both sessions' finalizer entries are cancelled")
}

// TestCreateSession_LateReleaseSurvivesUnregisterThrow is the
// re-entrant constructor above with a finalizer.unregister that throws,
// as a patched Reflect.apply could. The create, run through drainFirst
// as exposeAPI registers it, yields undefined, and the throw on the
// first session's token must not leave the second session registered.
// Not parallel: it swaps globalThis.Promise and finalizer.unregister.
func TestCreateSession_LateReleaseSurvivesUnregisterThrow(t *testing.T) {
	sharedMethods()
	old := finalizer.unregister
	t.Cleanup(func() { finalizer.unregister = old })
	finalizer.unregister = js.Global().Get("Function").New(`throw new TypeError("unregister");`)
	swapPromise(t, js.Global().Get("Function").New(`executor`,
		`var calls = 0;
		executor(function () {
			if (calls++ > 0) return;
			executor(function () {}, function () {});
		}, function () {});
		throw new TypeError("after");`))
	before := maps.Clone(sessions)
	opts := js.ValueOf(map[string]any{})
	v := jsValue(t, drainFirst(createSession)(js.Undefined(), []js.Value{opts}))
	assert.True(t, v.IsUndefined(), "a failed create yields undefined")
	assert.Equal(t, before, sessions, "neither session is left registered")
}

// TestCreateSession_ResolveThrowKeepsReasonWhenUnregisterThrows has a
// patched resolve throw while finalizer.unregister also throws (a
// patched Reflect.apply). The executor's cleanup of the session that
// resolve never delivered must not replace resolve's error: the create
// rejects with it, and the session is still disposed. Not parallel: it
// swaps globalThis.Promise and finalizer.unregister.
func TestCreateSession_ResolveThrowKeepsReasonWhenUnregisterThrows(t *testing.T) {
	sharedMethods()
	old := finalizer.unregister
	t.Cleanup(func() { finalizer.unregister = old })
	finalizer.unregister = js.Global().Get("Function").New(`throw new TypeError("unregister");`)
	swapPromise(t, js.Global().Get("Function").New(`executor`,
		`var self = this;
		executor(function () { throw new TypeError("resolve"); },
			function (e) { self.rejection = e; });`))
	before := maps.Clone(sessions)
	opts := js.ValueOf(map[string]any{})
	p := jsValue(t, createSession(js.Undefined(), []js.Value{opts}))
	require.Equal(t, js.TypeObject, p.Get("rejection").Type(), "the create rejects")
	assert.Equal(t, "resolve", p.Get("rejection").Get("message").String(),
		"with resolve's error, not unregister's")
	assert.Equal(t, before, sessions, "the undelivered session is disposed")
}

// TestReleaseSessionKeepingPanic checks the helper disposes and
// unregisters a session, and that a finalizer.unregister that throws
// (a patched Reflect.apply) is swallowed instead of panicking, with the
// session still disposed. Not parallel: it swaps finalizer.unregister.
func TestReleaseSessionKeepingPanic(t *testing.T) {
	unregistered := recordUnregister(t)
	newSession := func() (int64, js.Value) {
		sess, err := mdsmith.NewSession(mdsmith.SessionOptions{Workspace: mdsmith.NewMemWorkspace(nil)})
		require.NoError(t, err)
		_, id, tok := registerSession(sess)
		return id, tok
	}
	id, tok := newSession()
	releaseSessionKeepingPanic(id, tok)
	assert.NotContains(t, sessions, id, "the session is disposed")
	assert.Len(t, *unregistered, 1, "its finalizer entry is cancelled")

	finalizer.unregister = js.Global().Get("Function").New(`throw new TypeError("unregister");`)
	id, tok = newSession()
	assert.NotPanics(t, func() { releaseSessionKeepingPanic(id, tok) })
	assert.NotContains(t, sessions, id, "the session is disposed despite the throw")
}

// TestNewPromise_ReentrantExecutorRunsNested replaces Promise with a
// constructor whose resolve runs the executor a second time from inside
// the first run. The nested run executes the body too (the call is still
// pending while its outer run is on the stack), and both runs leave no
// call pending and register no func. Not parallel: it swaps Promise and
// the funcOf seam.
func TestNewPromise_ReentrantExecutorRunsNested(t *testing.T) {
	sharedMethods()
	swapPromise(t, js.Global().Get("Function").New(`executor`,
		`var calls = 0;
		executor(function () {
			if (calls++ > 0) return;
			executor(function () {}, function () {});
		}, function () {});`))
	made := recordFuncs(t)
	runs := 0
	newPromise(func(resolve, _ func(any)) { runs++; resolve(1) })
	assert.Equal(t, 2, runs, "the outer and the nested run both execute")
	assert.Empty(t, promiseCalls, "no call is left pending")
	assert.Empty(t, *made, "no func is registered")
}

// TestCreateSession_CtorThrowAfterExecutorRegistersNoSession replaces
// Promise with a constructor that runs the executor, so the session is
// registered and resolve returns, and then throws. createSession returns
// undefined, so the session object never reaches the caller and the
// session must not stay registered. Not parallel: it swaps
// globalThis.Promise.
func TestCreateSession_CtorThrowAfterExecutorRegistersNoSession(t *testing.T) {
	sharedMethods()
	unregistered := recordUnregister(t)
	swapPromise(t, js.Global().Get("Function").New(`executor`,
		`executor(function () {}, function () {}); throw new TypeError("after");`))
	before := maps.Clone(sessions)
	opts := js.ValueOf(map[string]any{})
	v := jsValue(t, createSession(js.Undefined(), []js.Value{opts}))
	assert.True(t, v.IsUndefined(), "a failed Promise construction yields undefined")
	assert.Equal(t, before, sessions, "no session is left registered")
	require.Len(t, *unregistered, 1, "the session's finalizer entry is cancelled")
	assert.Equal(t, js.TypeObject, (*unregistered)[0].Type(), "with its token")
}

// TestPromiseCtorThrow_KeepsProgramAndRegistersNoFunc replaces Promise with
// a constructor that fails: one that throws before it calls the
// executor, one that runs the executor and then throws, a Promise
// that is no function at all (syscall/js raises that as a
// *js.ValueError, not a js.Error), and one that returns without ever
// running the executor, which a spec Promise runs during construction.
// An async method returns undefined
// instead of ending the program, registers no func, and leaves no call
// pending. Not parallel: it swaps Promise and the funcOf seam.
func TestPromiseCtorThrow_KeepsProgramAndRegistersNoFunc(t *testing.T) {
	fn := js.Global().Get("Function")
	for _, tt := range []struct {
		name string
		ctor js.Value
	}{
		{"throws before the executor", fn.New(`throw new TypeError("ctor");`)},
		{"throws after the executor", fn.New(`executor`,
			`executor(function () {}, function () {}); throw new TypeError("after");`)},
		{"not a function", js.Undefined()},
		{"never runs the executor", fn.New(`executor`, `this.ignored = executor;`)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			proxy := newTestProxy(t)
			defer proxy.Call("dispose")
			made := recordFuncs(t)
			swapPromise(t, tt.ctor)

			v := proxy.Call("check", "a.md", "# A\n")
			assert.True(t, v.IsUndefined(), "a failed Promise construction yields undefined")
			assert.Empty(t, *made, "no func is registered per call")
			assert.Empty(t, promiseCalls, "no call is left pending")
		})
	}
}

// TestNewPromise_BadExecutorArgs replaces Promise with a constructor
// that calls the executor with no resolve or reject, and with a resolve
// and a reject that both throw. The executor callback must not let
// either failure out: a panic that leaves a js.FuncOf callback unwinds
// into the Go frames below the JS that called it. createSession returns,
// no session stays registered, no func is registered, and no call is
// left pending. Not parallel: it swaps Promise and the funcOf seam.
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
			made := recordFuncs(t)
			swapPromise(t, tt.ctor)
			before := maps.Clone(sessions)
			opts := js.ValueOf(map[string]any{})
			v := jsValue(t, createSession(js.Undefined(), []js.Value{opts}))
			assert.Equal(t, js.TypeObject, v.Type(), "createSession returns what the constructor built")
			assert.Equal(t, before, sessions, "no session is left registered")
			assert.Empty(t, *made, "no func is registered per call")
			assert.Empty(t, promiseCalls, "no call is left pending")
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

// TestSyncMethodJSException_ReturnsUndefined makes the Go-to-JS call a
// sync method performs throw. The method, as registered through
// drainFirst, returns undefined instead of ending the program, and not
// its disposed value: a failed capabilities() on a live session must
// not look like a disposed session's []. Not parallel: it registers a
// session in the shared sessions map.
func TestSyncMethodJSException_ReturnsUndefined(t *testing.T) {
	sharedMethods()
	throwing := func(*mdsmith.Session, []js.Value) js.Value {
		panic(js.Error{Value: js.Global().Get("TypeError").New("sync")})
	}
	_, id := newTestProxyWithID(t)
	defer disposeSession(id)
	list := drainFirst(sharedFunc(methodImpl{call: throwing, disposed: disposedEmptyList}))
	v := jsValue(t, list(js.Undefined(), []js.Value{js.ValueOf(id)}))
	assert.True(t, v.IsUndefined(), "a capabilities-shaped method yields undefined, not []")
	void := drainFirst(sharedFunc(methodImpl{call: throwing, disposed: disposedUndefined}))
	assert.True(t, jsValue(t, void(js.Undefined(), []js.Value{js.ValueOf(id)})).IsUndefined())
	assert.Contains(t, sessions, id, "the session stays live")
	assert.PanicsWithValue(t, "go bug", func() {
		drainFirst(sharedFunc(methodImpl{
			call:     func(*mdsmith.Session, []js.Value) js.Value { panic("go bug") },
			disposed: disposedUndefined,
		}))(js.Undefined(), []js.Value{js.ValueOf(id)})
	}, "a Go panic is re-raised")
}

// TestCreateSession_ThrowingThenGetterFreesSession defines a throwing
// `then` getter on Object.prototype. A native Promise resolve reads
// `then` on the session object and rejects the create with the thrown
// error without throwing to Go, so the session must never reach that
// lookup: the create resolves, and once the caller disposes the session
// sessions is as it was, and the create registered no func
// and left no call pending. Not parallel: it swaps package seams and patches
// Object.prototype.
func TestCreateSession_ThrowingThenGetterFreesSession(t *testing.T) {
	// Warm-up registers the shared method funcs before recording starts.
	newTestProxy(t).Call("dispose")
	made := recordFuncs(t)
	before := maps.Clone(sessions)

	object := js.Global().Get("Object")
	objectProto := object.Get("prototype")
	getter := js.Global().Get("Function").New("throw new Error('then getter')")
	desc := object.New()
	desc.Set("get", getter)
	desc.Set("configurable", true)
	object.Call("defineProperty", objectProto, "then", desc)
	defer objectProto.Delete("then")

	v, rejected := awaitPromise(t, jsValue(t, createSession(js.Undefined(), []js.Value{js.ValueOf(map[string]any{})})))
	require.False(t, rejected, "the session never reaches the getter, so the create resolves: %v", v)
	assert.True(t, object.Call("hasOwn", v, "then").Bool(), "session object carries its own then")
	v.Call("dispose")
	assert.Equal(t, before, sessions, "sessions after the create and dispose")
	assert.Empty(t, *made, "the create registered no func")
	assert.Empty(t, promiseCalls, "no call is left pending")
}

// TestExposeAPI_HidesThen defines a throwing `then` getter on
// Object.prototype and resolves a Promise with the mdsmith global, as a
// host's async engine load does when it returns the factory. The global
// carries its own non-enumerable `then`, so the resolve never reaches
// the getter, and Object.keys still lists only the API. Not parallel: it
// swaps apiFuncs and patches Object.prototype.
func TestExposeAPI_HidesThen(t *testing.T) {
	sharedMethods()
	old := apiFuncs
	t.Cleanup(func() { apiFuncs = old })
	apiFuncs = map[string]func(js.Value, []js.Value) any{}
	api := exposeAPI()
	object := js.Global().Get("Object")
	keys := object.Call("keys", api)
	require.Equal(t, 1, keys.Length(), "Object.keys lists only version")
	assert.Equal(t, "version", keys.Index(0).String())

	objectProto := object.Get("prototype")
	desc := object.New()
	desc.Set("get", js.Global().Get("Function").New("throw new Error('then getter')"))
	desc.Set("configurable", true)
	object.Call("defineProperty", objectProto, "then", desc)
	defer objectProto.Delete("then")

	v, rejected := awaitPromise(t, js.Global().Get("Promise").Call("resolve", api))
	require.False(t, rejected, "resolving with the API object never reads the getter: %v", v)
	assert.True(t, v.Equal(api), "the Promise resolves to the API object")
}

// TestExposeAPI_SurvivesFailedThenHider builds the mdsmith global with a
// then-hider whose load-time capture failed (captureGlobals left it
// zero) and with a captured defineProperty that throws, as one patched
// before load would be. main registers the global from exposeAPI, so a
// throw there would stop the engine from loading at all; the global
// comes back without its own `then` instead, and every create rejects at
// its own hideThen. Not parallel: it swaps apiFuncs, thenHider, and
// defineProperty.
func TestExposeAPI_SurvivesFailedThenHider(t *testing.T) {
	sharedMethods()
	saved := saveCaptures()
	oldFuncs := apiFuncs
	t.Cleanup(func() { apiFuncs = oldFuncs; saved.restore() })
	apiFuncs = map[string]func(js.Value, []js.Value) any{}
	throwing := js.Global().Get("Function").New("throw new TypeError('defineProperty')")
	for name, c := range map[string]struct {
		h      thenHiderValues
		define js.Value
	}{
		"capture failed": {define: saved.defineProperty},
		"define throws":  {h: saved.thenHider, define: throwing},
	} {
		t.Run(name, func(t *testing.T) {
			thenHider, defineProperty = c.h, c.define
			var api js.Value
			require.NotPanics(t, func() { api = exposeAPI() }, "a failed then-hider does not stop the load")
			assert.Equal(t, resolveVersion(), api.Get("version").String(), "the global is still built")
		})
	}
}

// TestHideThen checks that hideThen gives an object an own `then` of
// undefined that is non-enumerable, non-writable, and non-configurable,
// even while a page has put descriptor fields on Object.prototype: a
// callable `get`, which a descriptor with the usual prototype inherits
// and defineProperty rejects beside `value`, and `enumerable: true`,
// which would put `then` in Object.keys. Not parallel: it patches
// Object.prototype, only around the hideThen call.
func TestHideThen(t *testing.T) {
	sharedMethods()
	object := js.Global().Get("Object")
	objectProto := object.Get("prototype")
	o := object.New()
	require.NotPanics(t, func() {
		defer objectProto.Delete("get")
		defer objectProto.Delete("enumerable")
		objectProto.Set("get", js.Global().Get("Function").New())
		objectProto.Set("enumerable", true)
		hideThen(o)
	}, "a polluted Object.prototype does not make defineProperty throw")
	d := object.Call("getOwnPropertyDescriptor", o, "then")
	require.Equal(t, js.TypeObject, d.Type(), "then is an own property")
	assert.True(t, d.Get("value").IsUndefined(), "then is undefined")
	assert.False(t, d.Get("enumerable").Bool(), "then is not enumerable")
	assert.False(t, d.Get("writable").Bool(), "then is read-only")
	assert.False(t, d.Get("configurable").Bool(), "then is not configurable")
	assert.Zero(t, object.Call("keys", o).Length(), "then is not in Object.keys")
}

// TestHideThen_IgnoresDefinePropertyReplacedAfterLoad checks that
// hideThen uses the Object.defineProperty captured when the engine
// loads: a script that replaces it afterwards with a no-op neither turns
// the fix off nor receives the session object. Not parallel: it patches
// Object.defineProperty, only around the hideThen call.
func TestHideThen_IgnoresDefinePropertyReplacedAfterLoad(t *testing.T) {
	sharedMethods()
	object := js.Global().Get("Object")
	orig := object.Get("defineProperty")
	o := object.New()
	var seen []js.Value
	spy := js.FuncOf(func(_ js.Value, args []js.Value) any {
		seen = append(seen, args...)
		return nil
	})
	defer spy.Release()
	func() {
		defer object.Set("defineProperty", orig)
		object.Set("defineProperty", spy)
		hideThen(o)
	}()
	assert.Empty(t, seen, "the replacement defineProperty is never called")
	assert.True(t, object.Call("hasOwn", o, "then").Bool(), "then is still an own property")
}

// TestNewThenHider checks that newThenHider returns the JS string
// "then" (converted once, not on every create) and a frozen descriptor
// with a null prototype whose only own key is value, set to undefined.
// Frozen, a script that sees it (a patched Reflect.apply, on any create)
// cannot change the `then` of the sessions created after it.
func TestNewThenHider(t *testing.T) {
	object := js.Global().Get("Object")
	h := newThenHider(object)
	assert.Equal(t, js.TypeString, h.key.Type(), "key is a JS string")
	assert.Equal(t, "then", h.key.String(), "key is then")
	desc := h.desc
	assert.True(t, object.Call("getPrototypeOf", desc).IsNull(), "desc has a null prototype")
	assert.True(t, object.Call("isFrozen", desc).Bool(), "desc is frozen")
	keys := object.Call("getOwnPropertyNames", desc)
	require.Equal(t, 1, keys.Length(), "desc has one own key")
	assert.Equal(t, "value", keys.Index(0).String(), "desc's own key is value")
	assert.True(t, desc.Get("value").IsUndefined(), "desc.value is undefined")
}

// TestRegisterSession_DefineThrowRegistersNoSession swaps in a
// defineProperty that throws, as an Object.defineProperty patched before
// load would be captured. createSession must reject without leaving the
// Session in sessions, where no session object would ever reach
// dispose(). Not parallel: it swaps defineProperty.
func TestRegisterSession_DefineThrowRegistersNoSession(t *testing.T) {
	sharedMethods()
	old := defineProperty
	t.Cleanup(func() { defineProperty = old })
	defineProperty = js.Global().Get("Function").New("throw new TypeError('defineProperty')")
	before := maps.Clone(sessions)
	opts := js.ValueOf(map[string]any{})
	v, rejected := awaitPromise(t, jsValue(t, createSession(js.Undefined(), []js.Value{opts})))
	require.True(t, rejected, "createSession rejects when defineProperty throws")
	assert.True(t, v.InstanceOf(js.Global().Get("TypeError")), "rejects with the thrown TypeError")
	assert.Equal(t, before, sessions, "no session is left registered")
}

// TestRegisterSession_IgnoresObjectReplacedAfterLoad replaces
// globalThis.Object after the engine loaded with a constructor that
// counts its calls. registerSession builds the session object and its
// token from the Object captured at load, so the replacement never runs:
// it neither sees either object nor hands back one (a Proxy whose `then`
// trap throws) that would get past hideThen. Not parallel: it patches
// globalThis.Object, only around the create.
func TestRegisterSession_IgnoresObjectReplacedAfterLoad(t *testing.T) {
	// Warm-up captures the load-time globals and isRecord's toString.
	newTestProxy(t).Call("dispose")
	g := js.Global()
	orig := g.Get("Object")
	opts := js.ValueOf(map[string]any{})
	calls := 0
	spy := js.FuncOf(func(js.Value, []js.Value) any {
		calls++
		return nil
	})
	defer spy.Release()
	p := func() js.Value {
		defer g.Set("Object", orig)
		g.Set("Object", spy)
		return jsValue(t, createSession(js.Undefined(), []js.Value{opts}))
	}()
	v, rejected := awaitPromise(t, p)
	require.False(t, rejected, "the create resolves: %v", v)
	v.Call("dispose")
	assert.Zero(t, calls, "the replacement Object never runs")
}

// TestRegisterSession_MethodsIgnoreInheritedAccessors puts an accessor
// named dispose and a read-only value named check on Object.prototype.
// A plain Set would run the setter (handing it the session's bound
// dispose) or fail silently on the read-only one, leaving the session
// without either method. bindMethods defines each method as an own
// property instead, so the session keeps every method, the setter never
// runs, and Object.keys still lists them in sessionMethodNames order.
// Not parallel: it patches Object.prototype.
func TestRegisterSession_MethodsIgnoreInheritedAccessors(t *testing.T) {
	newTestProxy(t).Call("dispose")
	object := js.Global().Get("Object")
	objectProto := object.Get("prototype")
	calls := 0
	setter := js.FuncOf(func(js.Value, []js.Value) any {
		calls++
		return nil
	})
	defer setter.Release()
	acc := object.New()
	acc.Set("set", setter)
	acc.Set("configurable", true)
	object.Call("defineProperty", objectProto, "dispose", acc)
	defer objectProto.Delete("dispose")
	ro := object.New()
	ro.Set("value", 1)
	ro.Set("configurable", true)
	object.Call("defineProperty", objectProto, "check", ro)
	defer objectProto.Delete("check")

	v, rejected := awaitPromise(t, jsValue(t, createSession(js.Undefined(), []js.Value{js.ValueOf(map[string]any{})})))
	require.False(t, rejected, "the create resolves: %v", v)
	assert.Zero(t, calls, "the inherited dispose setter never runs")
	for _, name := range []string{"dispose", "check"} {
		require.True(t, object.Call("hasOwn", v, name).Bool(), "%s is an own property", name)
		assert.Equal(t, js.TypeFunction, v.Get(name).Type(), "%s is the bound method", name)
	}
	d := object.Call("getOwnPropertyDescriptor", v, "check")
	assert.True(t, d.Get("writable").Bool(), "a method stays writable")
	assert.True(t, d.Get("enumerable").Bool(), "a method stays enumerable")
	assert.True(t, d.Get("configurable").Bool(), "a method stays configurable")
	keys := object.Call("keys", v)
	names := sessionMethodNames()
	require.Equal(t, len(names), keys.Length())
	for i, name := range names {
		assert.Equal(t, name, keys.Index(i).String(), "Object.keys order")
	}
	before := len(sessions)
	v.Call("dispose")
	assert.Equal(t, before-1, len(sessions), "dispose frees the session")
}

// TestWorkspaceFromJS_IgnoresObjectPatchedAfterLoad replaces
// Object.keys and Object.prototype.toString, then globalThis.Object
// itself, after the engine loaded. workspaceFromJS and isRecord use the
// functions captured at load, so neither replacement runs or sees the
// workspace, and the workspace still converts. Not parallel: it patches
// globalThis.Object.
func TestWorkspaceFromJS_IgnoresObjectPatchedAfterLoad(t *testing.T) {
	sharedMethods()
	g := js.Global()
	object := g.Get("Object")
	objectProto := object.Get("prototype")
	origKeys, origToString := object.Get("keys"), objectProto.Get("toString")
	calls := 0
	spy := js.FuncOf(func(js.Value, []js.Value) any {
		calls++
		return nil
	})
	defer spy.Release()
	ws := js.ValueOf(map[string]any{"a.md": "# A\n"})
	want := map[string][]byte{"a.md": []byte("# A\n")}

	got := func() map[string][]byte {
		defer object.Set("keys", origKeys)
		defer objectProto.Set("toString", origToString)
		object.Set("keys", spy)
		objectProto.Set("toString", spy)
		return workspaceFromJS(ws)
	}()
	assert.Equal(t, want, got, "Object.keys and toString patched")

	got = func() map[string][]byte {
		defer g.Set("Object", object)
		g.Set("Object", spy)
		return workspaceFromJS(ws)
	}()
	assert.Equal(t, want, got, "globalThis.Object replaced")
	assert.Zero(t, calls, "no replacement runs")
}

// TestCaptureGlobals_SurvivesThrowingCaptures runs captureGlobals
// against globals patched before load to throw: an Object.freeze that
// throws (newThenHider) and a Function.prototype.call whose bind throws
// (bindTo, and isRecord's tag function bound through it). A throw at
// load would stop the engine from registering globalThis.mdsmith, so
// each failed capture is left undefined instead, and the create that
// needs it rejects without registering a session. Not parallel: it
// swaps the load-time captures.
func TestCaptureGlobals_SurvivesThrowingCaptures(t *testing.T) {
	sharedMethods()
	saved := saveCaptures()
	t.Cleanup(saved.restore)
	fake := js.Global().Get("Function").New(`
		const o = {
			create: Object.create, defineProperty: Object.defineProperty,
			keys: Object.keys, prototype: Object.prototype,
			freeze() { throw new TypeError('freeze'); },
		};
		const throwingBind = { bind() { throw new TypeError('bind'); } };
		return {
			Object: o,
			Function: { prototype: { call: throwingBind, bind: Function.prototype.bind } },
		};`).Invoke()
	require.NotPanics(t, func() { captureGlobals(fake) }, "a throwing capture does not stop the load")
	assert.True(t, thenHider.desc.IsUndefined(), "the then-hider capture failed")
	assert.True(t, defineProperty.Equal(js.Global().Get("Object").Get("defineProperty")),
		"defineProperty is captured on its own, apart from the then-hider")
	assert.True(t, bindTo.IsUndefined(), "the bind capture failed")
	assert.True(t, recordTag.IsUndefined(), "the tag capture failed")
	assert.True(t, objectKeys.Equal(js.Global().Get("Object").Get("keys")), "Object.keys is still captured")

	before := maps.Clone(sessions)
	opts := js.ValueOf(map[string]any{})
	_, rejected := awaitPromise(t, jsValue(t, createSession(js.Undefined(), []js.Value{opts})))
	assert.True(t, rejected, "a create that needs a failed capture rejects")
	assert.Equal(t, before, sessions, "no session is left registered")
}

// loadCaptures is a copy of every value captureGlobals sets, so a test
// that reruns it can put the load-time captures back.
type loadCaptures struct {
	bindTo, objectCtor, objectKeys, objectCreate, recordTag js.Value
	defineProperty, methodDesc                              js.Value
	thenHider                                               thenHiderValues
}

func saveCaptures() loadCaptures {
	return loadCaptures{
		bindTo, objectCtor, objectKeys, objectCreate, recordTag,
		defineProperty, methodDesc, thenHider,
	}
}

func (c loadCaptures) restore() {
	bindTo, objectCtor, objectKeys, objectCreate, recordTag =
		c.bindTo, c.objectCtor, c.objectKeys, c.objectCreate, c.recordTag
	defineProperty, methodDesc, thenHider = c.defineProperty, c.methodDesc, c.thenHider
}

// TestNewMethodKeys checks that newMethodKeys pairs each name, in order,
// with its JS string, so bindMethods passes a key converted once at load
// rather than converting every method name on every create.
func TestNewMethodKeys(t *testing.T) {
	names := sessionMethodNames()
	keys := newMethodKeys(names)
	require.Len(t, keys, len(names))
	for i, name := range names {
		assert.Equal(t, name, keys[i].name, "name order")
		assert.Equal(t, js.TypeString, keys[i].key.Type(), "%s key is a JS string", name)
		assert.Equal(t, name, keys[i].key.String(), "%s key text", name)
	}
}

// TestFrozenDesc checks that frozenDesc returns a frozen object with a
// null prototype whose own keys are exactly the ones its set func adds,
// the shape both newThenHider and newMethodDesc build their descriptors in.
func TestFrozenDesc(t *testing.T) {
	object := js.Global().Get("Object")
	d := frozenDesc(object, func(d js.Value) { d.Set("writable", true) })
	assert.True(t, object.Call("getPrototypeOf", d).IsNull(), "null prototype")
	assert.True(t, object.Call("isFrozen", d).Bool(), "frozen")
	keys := object.Call("getOwnPropertyNames", d)
	require.Equal(t, 1, keys.Length())
	assert.Equal(t, "writable", keys.Index(0).String())
	assert.True(t, d.Get("writable").Bool())
}

// patchApplyThrowOnWrapper replaces Reflect.apply with one that throws
// when called for syscall/js's _makeFuncWrapper (the call js.FuncOf
// makes after it stores the handler in its private func table) and
// otherwise delegates. It returns a JS object whose n property counts
// those throws, and restores Reflect.apply when t ends. A caller must
// not run in parallel.
func patchApplyThrowOnWrapper(t *testing.T) (state js.Value, restore func()) {
	t.Helper()
	reflectObj := js.Global().Get("Reflect")
	orig := reflectObj.Get("apply")
	state = js.Global().Get("Object").New()
	state.Set("n", 0)
	patched := js.Global().Get("Function").New("orig", "state", `return function (target, thisArg, args) {
		if (thisArg && thisArg._makeFuncWrapper === target) {
			state.n++;
			throw new TypeError("makeFuncWrapper");
		}
		return orig(target, thisArg, args);
	}`).Invoke(orig, state)
	restore = func() { reflectObj.Set("apply", orig) }
	t.Cleanup(restore)
	reflectObj.Set("apply", patched)
	return state, restore
}

// TestNewPromise_WrapperThrowRegistersNoFunc patches Reflect.apply to
// throw on js.FuncOf's _makeFuncWrapper call. FuncOf stores the handler
// in its func table before that call and drops the id when it panics, so
// a func registered per call could never be released. newPromise must
// register none: it constructs every Promise with the one executor func
// sharedMethods registered at load. An async method and createSession
// therefore still work under the patch, never reach _makeFuncWrapper,
// and leave no call pending. Not parallel: it swaps Reflect.apply and
// the funcOf seam.
func TestNewPromise_WrapperThrowRegistersNoFunc(t *testing.T) {
	proxy, id := newTestProxyWithID(t)
	defer disposeSession(id)
	made := recordFuncs(t)

	state, restore := patchApplyThrowOnWrapper(t)
	method := proxy.Call("check", "a.md", "# A\n")
	create := jsValue(t, createSession(js.Undefined(), []js.Value{js.ValueOf(map[string]any{})}))
	restore()

	assert.Empty(t, *made, "no func is registered per call")
	assert.Zero(t, state.Get("n").Int(), "_makeFuncWrapper is never called")
	assert.Empty(t, promiseCalls, "no call is left pending")
	_, rejected := awaitPromise(t, method)
	assert.False(t, rejected, "the async method resolves under the patch")
	s, rejected := awaitPromise(t, create)
	require.False(t, rejected, "createSession resolves under the patch")
	s.Call("dispose")
	proxy.Call("dispose")
	assert.NotContains(t, sessions, id, "dispose frees the session")
}

// TestNewPromise_DelegatingApplyKeepsWorking replaces Reflect.apply with
// a wrapper that only delegates, as a dev tool or instrumentation shim
// might after the engine loads. Async methods keep resolving: the engine
// does not refuse a replaced Reflect.apply. Not parallel: it swaps
// Reflect.apply.
func TestNewPromise_DelegatingApplyKeepsWorking(t *testing.T) {
	proxy, id := newTestProxyWithID(t)
	defer disposeSession(id)
	reflectObj := js.Global().Get("Reflect")
	orig := reflectObj.Get("apply")
	t.Cleanup(func() { reflectObj.Set("apply", orig) })
	reflectObj.Set("apply", js.Global().Get("Function").New("orig",
		"return function (f, t, a) { return orig(f, t, a); }").Invoke(orig))

	v, rejected := awaitPromise(t, proxy.Call("check", "a.md", "# A\n"))
	assert.False(t, rejected, "check resolves under a delegating Reflect.apply")
	assert.Equal(t, js.TypeObject, v.Type(), "with its diagnostics array")
}

// TestNewPromise_ConstructThrowLeavesNothingPending replaces
// Reflect.construct, which wasm_exec.js calls for every Go New, with one
// that throws, so the Promise construction fails before the executor
// runs. newPromise yields undefined, never runs the executor, registers
// no func, and leaves no call pending. Not parallel: it swaps
// Reflect.construct and the funcOf seam.
func TestNewPromise_ConstructThrowLeavesNothingPending(t *testing.T) {
	sharedMethods()
	made := recordFuncs(t)
	reflectObj := js.Global().Get("Reflect")
	orig := reflectObj.Get("construct")
	t.Cleanup(func() { reflectObj.Set("construct", orig) })
	throwing := js.Global().Get("Function").New(`throw new TypeError("construct");`)
	ran := false

	reflectObj.Set("construct", throwing)
	p := newPromise(func(_, _ func(any)) { ran = true })
	reflectObj.Set("construct", orig)

	assert.True(t, p.IsUndefined(), "a failed construction yields undefined")
	assert.False(t, ran, "the executor never ran")
	assert.Empty(t, *made, "no func is registered")
	assert.Empty(t, promiseCalls, "no call is left pending")
}

// TestNewPromise_LateExecutorCallIsIgnored replaces Promise with a
// constructor that stashes the executor without running it, then calls
// the stashed executor after newPromise returned. The call must not run
// the executor body: the call it belonged to is no longer pending, and
// running it would act on a session or result nobody receives. Not
// parallel: it swaps Promise and a global.
func TestNewPromise_LateExecutorCallIsIgnored(t *testing.T) {
	sharedMethods()
	g := js.Global()
	t.Cleanup(func() { g.Delete("__mdsmithStash") })
	swapPromise(t, g.Get("Function").New(`executor`, `globalThis.__mdsmithStash = executor;`))
	runs := 0
	p := newPromise(func(resolve, _ func(any)) { runs++; resolve(1) })
	assert.True(t, p.IsUndefined(), "a constructor that never ran the executor yields undefined")
	require.Empty(t, promiseCalls, "the call is no longer pending")

	noop := g.Get("Function").New()
	g.Get("__mdsmithStash").Invoke(noop, noop)
	assert.Zero(t, runs, "a late call of the stashed executor runs nothing")
}

// TestNewPromise_KeptExecutorCannotRunAnotherCall keeps the executor
// one call's Promise constructor received, then calls it with a
// script's own resolve while a later call's Promise is being built, as
// a Node async_hooks init hook or a Promise patched only for a moment
// could. The kept executor belongs to the first call, so it must run
// nothing: the later call's body runs once, with the resolve its own
// constructor passed, and the script's resolve never receives the
// result. Not parallel: it swaps Promise and a global.
func TestNewPromise_KeptExecutorCannotRunAnotherCall(t *testing.T) {
	sharedMethods()
	g := js.Global()
	t.Cleanup(func() {
		g.Delete("__mdsmithStash")
		g.Delete("__mdsmithStolen")
	})
	fn := g.Get("Function")
	swapPromise(t, fn.New(`executor`, `globalThis.__mdsmithStash = executor;`))
	newPromise(func(_, _ func(any)) {})
	require.Empty(t, promiseCalls, "the first call is no longer pending")

	swapPromise(t, fn.New(`executor`,
		`var self = this;
		globalThis.__mdsmithStash(function (v) { globalThis.__mdsmithStolen = v; }, function () {});
		executor(function (v) { self.value = v; }, function () {});`))
	runs := 0
	p := newPromise(func(resolve, _ func(any)) { runs++; resolve("result") })
	assert.Equal(t, 1, runs, "the later call's body runs once")
	assert.True(t, g.Get("__mdsmithStolen").IsUndefined(), "the kept executor's resolve never receives the result")
	require.Equal(t, js.TypeObject, p.Type(), "the later call yields its Promise")
	assert.Equal(t, "result", p.Get("value").String(), "its own constructor's resolve receives the result")
	assert.Empty(t, promiseCalls, "no call is left pending")
}

// TestNewPromise_OuterExecutorCannotRunNestedCall keeps the outer
// call's executor and calls it, with a script's own resolve, while a
// call nested in the outer body is being built. The outer call is still
// pending but is not the call on top, so the kept executor runs
// nothing: the nested body runs once and its result reaches its own
// constructor's resolve. Not parallel: it swaps Promise and a global.
func TestNewPromise_OuterExecutorCannotRunNestedCall(t *testing.T) {
	sharedMethods()
	g := js.Global()
	t.Cleanup(func() {
		g.Delete("__mdsmithStash")
		g.Delete("__mdsmithStolen")
	})
	swapPromise(t, g.Get("Function").New(`executor`,
		`var self = this;
		if (globalThis.__mdsmithStash === undefined) {
			globalThis.__mdsmithStash = executor;
		} else {
			globalThis.__mdsmithStash(function (v) { globalThis.__mdsmithStolen = v; }, function () {});
		}
		executor(function (v) { self.value = v; }, function () {});`))
	outerRuns, nestedRuns := 0, 0
	var nested js.Value
	newPromise(func(resolve, _ func(any)) {
		outerRuns++
		nested = newPromise(func(resolve, _ func(any)) { nestedRuns++; resolve("nested") })
		resolve("outer")
	})
	assert.Equal(t, 1, outerRuns, "the outer body runs once")
	assert.Equal(t, 1, nestedRuns, "the nested body runs once")
	assert.True(t, g.Get("__mdsmithStolen").IsUndefined(), "the kept outer executor's resolve receives nothing")
	require.Equal(t, js.TypeObject, nested.Type(), "the nested call yields its Promise")
	assert.Equal(t, "nested", nested.Get("value").String(), "its own constructor's resolve receives the result")
	assert.Empty(t, promiseCalls, "no call is left pending")
}

// TestNewPromise_RawExecutorCannotStepToNextCall hands a script the raw
// shared executor and the number of the call before, as a Reflect.apply
// patched for one call sees both in callExecutor's bind. While the next
// call's Promise is being built (as a Node async_hooks init hook could),
// the script runs the raw executor with the number one past the one it
// saw. Call numbers are keyed, not a counting sequence, so that runs
// nothing: the call's body runs once, with its own constructor's
// resolve. Not parallel: it swaps Promise and a global.
func TestNewPromise_RawExecutorCannotStepToNextCall(t *testing.T) {
	sharedMethods()
	g := js.Global()
	t.Cleanup(func() { g.Delete("__mdsmithStolen") })
	var seen int64
	newPromise(func(resolve, _ func(any)) {
		seen = promiseCalls[len(promiseCalls)-1].seq
		resolve(nil)
	})
	require.NotZero(t, seen, "the first call is bound to a number")

	swapPromise(t, g.Get("Function").New("shared", "next", `return function (executor) {
		var self = this;
		shared(next, function (v) { globalThis.__mdsmithStolen = v; }, function () {});
		executor(function (v) { self.value = v; }, function () {});
	};`).Invoke(promiseExecutor, float64(seen+1)))
	runs := 0
	p := newPromise(func(resolve, _ func(any)) { runs++; resolve("result") })
	assert.Equal(t, 1, runs, "the call's body runs once")
	assert.True(t, g.Get("__mdsmithStolen").IsUndefined(), "the stepped number runs nothing")
	require.Equal(t, js.TypeObject, p.Type(), "the call yields its Promise")
	assert.Equal(t, "result", p.Get("value").String(), "its own constructor's resolve receives the result")
}

// TestNewPromiseSeq checks the call numbers newPromise binds: each is in
// [1, maxSessionID], so splitSeq accepts it and a float64 holds it
// exactly, none repeats, and they are not a counting sequence. The
// block tag sits past every session-id round, so no number is drawn from
// a block a session id's permutation encrypts. Not parallel: it advances
// the shared counter.
func TestNewPromiseSeq(t *testing.T) {
	assert.GreaterOrEqual(t, promiseSeqTag, sessionIDRounds, "the tag is no session-id round")
	seen := make(map[int64]bool, 4096)
	prev := int64(0)
	for range 4096 {
		n := newPromiseSeq()
		require.GreaterOrEqual(t, n, int64(1))
		require.LessOrEqual(t, n, int64(maxSessionID))
		require.False(t, seen[n], "number %d repeats", n)
		assert.NotEqual(t, prev+1, n, "numbers count up")
		got, _ := splitSeq([]js.Value{js.ValueOf(float64(n))})
		require.Equal(t, n, got, "splitSeq accepts the number")
		seen[n] = true
		prev = n
	}
}

// TestSplitSeq checks how runPromiseCall splits the bound call number
// off the executor's arguments: a leading integer in [1, maxSessionID]
// is the number, any other leading number matches no call (-1), and a leading
// non-number (an unbound run's resolve) is no number at all (0).
func TestSplitSeq(t *testing.T) {
	fn := js.Global().Get("Function").New()
	bigInt := js.Global().Get("BigInt").Invoke(1)
	for _, tt := range []struct {
		name string
		args []js.Value
		seq  int64
		rest int
	}{
		{"no arguments", nil, 0, 0},
		{"unbound run", []js.Value{fn, fn}, 0, 2},
		{"bound run", []js.Value{js.ValueOf(3), fn, fn}, 3, 2},
		{"zero", []js.Value{js.ValueOf(0), fn}, -1, 1},
		{"negative", []js.Value{js.ValueOf(-2), fn}, -1, 1},
		{"fraction", []js.Value{js.ValueOf(1.5), fn}, -1, 1},
		{"NaN", []js.Value{js.ValueOf(math.NaN()), fn}, -1, 1},
		{"2^53", []js.Value{js.ValueOf(float64(maxSessionID)), fn}, maxSessionID, 1},
		{"past 2^53", []js.Value{js.ValueOf(float64(maxSessionID) + 2), fn}, -1, 1},
		{"Infinity", []js.Value{js.ValueOf(math.Inf(1)), fn}, -1, 1},
		{"BigInt", []js.Value{bigInt, fn}, 0, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			seq, rest := splitSeq(tt.args)
			assert.Equal(t, tt.seq, seq)
			assert.Len(t, rest, tt.rest)
		})
	}
}

// TestCallExecutor checks callExecutor binds the shared executor to
// the call's number, and, when bindTo was never captured, hands out
// the shared func unbound and marks the call unbound, so newPromise
// still settles its Promise (a create still rejects rather than
// returning undefined). A bindTo that throws gets the same fallback.
// Not parallel: it swaps bindTo.
func TestCallExecutor(t *testing.T) {
	shared := sharedExecutor()
	c := &promiseCall{seq: 5}
	bound := callExecutor(shared, c)
	assert.Equal(t, js.TypeFunction, bound.Type(), "a bound function")
	assert.False(t, bound.Equal(shared), "not the shared func itself")
	assert.Equal(t, int64(5), c.seq, "the call keeps its number")

	old := bindTo
	t.Cleanup(func() { bindTo = old })
	bindTo = js.Undefined()
	c = &promiseCall{seq: 6}
	assert.True(t, callExecutor(shared, c).Equal(shared), "the shared func, unbound")
	assert.Zero(t, c.seq, "the call is marked unbound")
	runs := 0
	p := newPromise(func(resolve, _ func(any)) { runs++; resolve(1) })
	bindTo = old
	assert.Equal(t, 1, runs, "an unbound call still runs")
	_, rejected := awaitPromise(t, p)
	assert.False(t, rejected, "and its Promise settles")

	bindTo = js.Global().Get("Function").New("throw new TypeError('bind')")
	c = &promiseCall{seq: 7}
	assert.True(t, callExecutor(shared, c).Equal(shared), "a throwing bind also yields the shared func")
	assert.Zero(t, c.seq, "and marks the call unbound")
	bindTo = old
}

// TestNewPromise_SecondSequentialRunIsIgnored replaces Promise with a
// constructor that runs the executor twice in a row. Only the first run
// executes the body; once it has returned, the call is finished, as a
// spec Promise runs its executor once. Not parallel: it swaps Promise.
func TestNewPromise_SecondSequentialRunIsIgnored(t *testing.T) {
	sharedMethods()
	swapPromise(t, js.Global().Get("Function").New(`executor`,
		`var f = function () {}; executor(f, f); executor(f, f);`))
	runs := 0
	newPromise(func(_, _ func(any)) { runs++ })
	assert.Equal(t, 1, runs, "the body runs once")
	assert.Empty(t, promiseCalls, "no call is left pending")
}

// TestRunPromiseCall_NothingPending calls the shared executor func
// directly while no newPromise call is pending, as a script that kept it
// could. It returns undefined and runs nothing.
func TestRunPromiseCall_NothingPending(t *testing.T) {
	sharedMethods()
	require.Empty(t, promiseCalls)
	noop := js.Global().Get("Function").New()
	assert.True(t, promiseExecutor.Invoke(noop, noop).IsUndefined())
	assert.True(t, promiseExecutor.Invoke().IsUndefined())
}

// TestSharedExecutor checks sharedExecutor returns the one executor
// func, a JS function, on every call and registers it only once. Not
// parallel: it swaps the funcOf seam.
func TestSharedExecutor(t *testing.T) {
	first := sharedExecutor()
	made := recordFuncs(t)
	assert.Equal(t, js.TypeFunction, first.Type(), "the executor is a JS function")
	assert.True(t, sharedExecutor().Equal(first), "a later call returns the same func")
	assert.True(t, promiseExecutor.Equal(first), "the one promiseExecutor")
	assert.Empty(t, *made, "a later call registers no func")
}

// TestNewPromise_ClearsPoppedSlot checks newPromise clears its call's
// slot in promiseCalls' backing array as it pops the call, so the array
// does not keep the Go executor, and the Session it closes over,
// reachable after the call returns. A nested call fills a second slot.
// Not parallel: it reads promiseCalls.
func TestNewPromise_ClearsPoppedSlot(t *testing.T) {
	sharedMethods()
	newPromise(func(_, _ func(any)) {
		newPromise(func(_, _ func(any)) {})
	})
	require.Empty(t, promiseCalls, "no call is left pending")
	require.GreaterOrEqual(t, cap(promiseCalls), 2, "the nested call grew the stack")
	for i, c := range promiseCalls[:cap(promiseCalls)] {
		assert.Nil(t, c, "slot %d is cleared", i)
	}
}

// TestNewPromise_PopsOwnCall checks newPromise pops its own call even
// when another call sits above it on promiseCalls by the time it
// returns, as an interleaved newPromise would leave it: it removes its
// call by identity rather than whatever is on top, so the other call's
// entry survives for its own newPromise to pop. Not parallel: it reads
// and writes promiseCalls.
func TestNewPromise_PopsOwnCall(t *testing.T) {
	sharedMethods()
	require.Empty(t, promiseCalls)
	other := &promiseCall{executor: func(_, _ func(any)) {}}
	t.Cleanup(func() { promiseCalls = nil })
	var own *promiseCall
	newPromise(func(_, _ func(any)) {
		own = promiseCalls[len(promiseCalls)-1]
		promiseCalls = append(promiseCalls, other)
	})
	require.NotNil(t, own)
	require.Len(t, promiseCalls, 1, "only the interleaved call is left")
	assert.Same(t, other, promiseCalls[0], "the interleaved call survives")
	require.GreaterOrEqual(t, cap(promiseCalls), 2)
	assert.Nil(t, promiseCalls[:2][1], "the freed slot is cleared")
}

// TestJSInt checks jsInt, the one integer check boundSession and
// splitSeq share: a number that is an integer with magnitude at most
// maxSessionID converts exactly; a non-number, a fraction, NaN,
// Infinity, or a magnitude past maxSessionID does not.
func TestJSInt(t *testing.T) {
	cases := []struct {
		name string
		v    js.Value
		want int64
		ok   bool
	}{
		{"zero", js.ValueOf(0), 0, true},
		{"one", js.ValueOf(1), 1, true},
		{"negative", js.ValueOf(-7), -7, true},
		{"2^53", js.ValueOf(float64(maxSessionID)), maxSessionID, true},
		{"-2^53", js.ValueOf(-float64(maxSessionID)), -maxSessionID, true},
		{"past 2^53", js.ValueOf(float64(maxSessionID) + 2), 0, false},
		{"fraction", js.ValueOf(1.5), 0, false},
		{"NaN", js.ValueOf(math.NaN()), 0, false},
		{"Infinity", js.ValueOf(math.Inf(1)), 0, false},
		{"string", js.ValueOf("1"), 0, false},
		{"undefined", js.Undefined(), 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, ok := jsInt(tc.v)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, n)
		})
	}
}

// TestSharedExecutor_FailedRegistrationNotRetried pins that a failed
// registration of the shared executor is not retried on a later call.
// js.FuncOf stores its handler before it builds the JS wrapper, so a
// retry while the wrapper keeps failing would strand one func-table
// entry per newPromise, the leak plan 2610031420 removes. Not parallel:
// it swaps the funcOf seam and resets the registration.
func TestSharedExecutor_FailedRegistrationNotRetried(t *testing.T) {
	sharedMethods()
	oldExec, oldOf := promiseExecutor, funcOf
	t.Cleanup(func() {
		promiseExecutor, funcOf = oldExec, oldOf
		executorOnce = sync.Once{}
		executorOnce.Do(func() {})
	})
	executorOnce = sync.Once{}
	promiseExecutor = js.Undefined()
	calls := 0
	funcOf = func(func(js.Value, []js.Value) any) js.Func {
		calls++
		panic(js.Error{Value: jsError("wrapper failed")})
	}
	assert.Panics(t, func() { sharedExecutor() })
	assert.NotPanics(t, func() { sharedExecutor() })
	assert.Equal(t, 1, calls, "registration is attempted once")
}
