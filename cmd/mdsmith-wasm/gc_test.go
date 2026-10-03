//go:build js && wasm

package main

import (
	"runtime"
	"sync"
	"syscall/js"
	"testing"
	"time"

	"github.com/jeduden/mdsmith/pkg/mdsmith"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	jsGCOnce sync.Once
	jsGCFunc func()
)

// jsGC returns a function that forces a V8 collection, or nil when the
// host offers no way to. Node exposes gc only under --expose-gc, which
// the go_js_wasm_exec wrapper does not pass, so it turns the flag on at
// run time and reads gc out of a fresh context. The lookup runs once:
// each runInNewContext builds a whole V8 context.
func jsGC() func() {
	jsGCOnce.Do(func() {
		g := js.Global()
		if gc := g.Get("gc"); gc.Type() == js.TypeFunction {
			jsGCFunc = func() { gc.Invoke() }
			return
		}
		req := g.Get("require")
		if req.Type() != js.TypeFunction {
			return
		}
		req.Invoke("v8").Call("setFlagsFromString", "--expose-gc")
		if gc := req.Invoke("vm").Call("runInNewContext", "gc"); gc.Type() == js.TypeFunction {
			jsGCFunc = func() { gc.Invoke() }
		}
	})
	return jsGCFunc
}

// collectUntil forces garbage collection on both sides until done
// reports true, or fails t after a bounded number of rounds; it skips t
// when the host cannot force a JS collection. syscall/js frees a
// Go-held reference from a Go finalizer, so Go collects first; each
// sleep hands control to the JS event loop, where V8 runs its
// FinalizationRegistry callbacks.
func collectUntil(t *testing.T, done func() bool) {
	t.Helper()
	gc := requireJSGC(t)
	for i := 0; i < 100; i++ {
		runtime.GC()
		gc()
		time.Sleep(10 * time.Millisecond)
		if done() {
			return
		}
	}
	require.True(t, done(), "not collected after forced GC")
}

// requireJSGC returns jsGC's collector, skipping t when the host offers
// none. A test that collects inside t.Run must call it on the parent
// first: a parent whose subtests all skip reports a pass, which
// test-js-wasm accepts, so only a skip of the top-level test fails that
// gate by name.
func requireJSGC(t *testing.T) func() {
	t.Helper()
	gc := jsGC()
	if gc == nil {
		t.Skip("host cannot force a JS garbage collection")
	}
	return gc
}

// collectGone collects until the session with the given id has left
// sessions, draining the finalizer queue each round as the next engine
// call would.
func collectGone(t *testing.T, id int64) {
	t.Helper()
	collectUntil(t, func() bool {
		drainFinalized()
		_, ok := sessions[id]
		return !ok
	})
}

// queued reports whether id is on the finalizer queue.
func queued(id int64) bool {
	return finalizer.queue.Call("includes", id).Bool()
}

// TestDroppedSessionFreedOnNextCall checks that collection alone only
// queues a dropped session's id, never calling into Go, and that each
// engine entry point (a session method, dispose, createSession) then
// frees it.
func TestDroppedSessionFreedOnNextCall(t *testing.T) {
	requireJSGC(t)
	for name, call := range map[string]func(t *testing.T, keeper js.Value){
		"session method": func(_ *testing.T, k js.Value) { k.Call("capabilities") },
		"dispose":        func(_ *testing.T, k js.Value) { k.Call("dispose") },
		"createSession": func(t *testing.T, _ js.Value) {
			opts := js.Global().Get("Object").New()
			sess, rejected := awaitPromise(t, jsValue(t, createSession(js.Undefined(), []js.Value{opts})))
			require.False(t, rejected, "createSession: %v", sess)
			sess.Call("dispose")
		},
	} {
		t.Run(name, func(t *testing.T) {
			keeper, _ := newTestProxyWithID(t)
			defer keeper.Call("dispose")
			id := newDroppedSession(t)
			collectUntil(t, func() bool { return queued(id) })
			require.Contains(t, sessions, id, "collection queues the id and frees nothing")
			call(t, keeper)
			assert.NotContains(t, sessions, id)
			assert.Zero(t, finalizer.queue.Length(), "the queue is drained")
		})
	}
}

// collectDropped drops a control session and collects until it has left
// sessions. Anything dropped before the call that a finalizer would
// free has then been freed too, so a test can assert that something it
// dropped earlier was kept without passing only because no collection
// ran.
func collectDropped(t *testing.T) {
	t.Helper()
	collectGone(t, newDroppedSession(t))
}

// newDroppedSession creates a session and returns only its id, so the
// session object is unreachable once this returns.
func newDroppedSession(t *testing.T) int64 {
	t.Helper()
	_, id := newTestProxyWithID(t)
	return id
}

func TestSessionDroppedWithoutDispose_LeavesSessions(t *testing.T) {
	id := newDroppedSession(t)
	require.Contains(t, sessions, id)
	collectGone(t, id)
	assert.NotContains(t, sessions, id)
}

// newDroppedMethod creates a session and returns one method taken off
// it plus its id; the session object itself is unreachable on return.
func newDroppedMethod(t *testing.T) (js.Value, int64) {
	t.Helper()
	proxy, id := newTestProxyWithID(t)
	return proxy.Get("check"), id
}

func TestMethodOutlivesSessionObject(t *testing.T) {
	// The method is held only inside this func, so it is unreachable
	// once it returns.
	id := func() int64 {
		check, id := newDroppedMethod(t)
		// The object was dropped before the control, so a finalizer
		// tied to the object alone would have fired by now.
		collectDropped(t)
		require.Contains(t, sessions, id, "session kept alive by its method")
		diags, rejected := awaitPromise(t, check.Invoke("a.md", "# A\n"))
		require.False(t, rejected, "check through the kept method: %v", diags)
		assert.Zero(t, diags.Length(), "clean file has no diagnostics")
		return id
	}()
	collectGone(t, id)
}

// countCalls wraps fn in a JS function that adds one to *n per call.
// The counting func is released when t ends.
func countCalls(t *testing.T, fn js.Value, n *int) js.Value {
	counter := js.FuncOf(func(js.Value, []js.Value) any { *n++; return nil })
	t.Cleanup(counter.Release)
	return js.Global().Get("Function").New("f", "c",
		"return function(...a){ c(); return f.apply(this, a); }").Invoke(fn, counter)
}

func TestExplicitDisposeCancelsFinalizer(t *testing.T) {
	sharedMethods() // creates the registry, so the seams hold its methods
	old := finalizer
	registers, unregisters := 0, 0
	countReg := countCalls(t, old.register, &registers)
	countUnreg := countCalls(t, old.unregister, &unregisters)
	// Registered after countCalls, so the seams are restored before the
	// counting funcs are released.
	t.Cleanup(func() { finalizer = old })
	finalizer.register, finalizer.unregister = countReg, countUnreg

	// cycle returns the id of a session created and then disposed.
	cycle := func() int64 {
		proxy, id := newTestProxyWithID(t)
		proxy.Call("dispose")
		proxy.Call("dispose")
		return id
	}
	for i := 0; i < 5; i++ {
		cycle()
	}
	assert.Equal(t, 5, registers, "one registry entry per session")
	assert.Equal(t, 5, unregisters, "every explicit dispose drops its entry, a second dispose none")

	// A finalizer that still fired after dispose would delete the
	// stand-in registered under the disposed id.
	id := cycle()
	stand, err := mdsmith.NewSession(mdsmith.SessionOptions{Workspace: mdsmith.NewMemWorkspace(nil)})
	require.NoError(t, err)
	sessions[id] = stand
	t.Cleanup(func() { disposeSession(id) })
	collectDropped(t)
	assert.Contains(t, sessions, id, "a disposed session's finalizer never fires")
}

// enqueue pushes v onto the finalizer queue, as the registry's cleanup
// callback does.
func enqueue(v js.Value) { finalizer.queue.Call("push", v) }

func TestDrainFinalized_DisposesLiveIDOnce(t *testing.T) {
	proxy, id := newTestProxyWithID(t)
	defer proxy.Call("dispose")
	enqueue(js.ValueOf(id))
	enqueue(js.ValueOf(id))
	assert.NotPanics(t, drainFinalized, "a repeated id is disposed once")
	assert.NotContains(t, sessions, id)
	assert.Zero(t, finalizer.queue.Length())
	assert.NotPanics(t, drainFinalized, "an empty queue is a no-op")
}

// TestDrainFinalized_IgnoresNonIDHeldValue queues held values that are
// not an integer id, as only a FinalizationRegistry patched before load
// could. A panic in a js.FuncOf callback ends the Go program and every
// session with it, so each must dispose nothing instead.
func TestDrainFinalized_IgnoresNonIDHeldValue(t *testing.T) {
	proxy, id := newTestProxyWithID(t)
	defer proxy.Call("dispose")
	for _, v := range []js.Value{
		js.ValueOf("x"),
		js.Undefined(),
		js.Global().Get("BigInt").Invoke(1),
		js.Global().Get("NaN"),
	} {
		enqueue(v)
	}
	assert.NotPanics(t, drainFinalized)
	assert.Contains(t, sessions, id, "a non-id held value disposes nothing")
	assert.Zero(t, finalizer.queue.Length())
}

// TestDrainFinalized_NoQueue checks a host with no usable registry,
// where there is no queue, drains nothing without a JS exception.
func TestDrainFinalized_NoQueue(t *testing.T) {
	sharedMethods()
	old := finalizer
	t.Cleanup(func() { finalizer = old })
	finalizer = sessionFinalizer{}
	assert.NotPanics(t, drainFinalized)
}

func TestDisposeSession(t *testing.T) {
	proxy, id := newTestProxyWithID(t)
	defer proxy.Call("dispose")
	disposeSession(id)
	assert.NotContains(t, sessions, id)
	assert.NotPanics(t, func() { disposeSession(id) }, "an id with no live session is a no-op")
	assert.Equal(t, 0, proxy.Call("capabilities").Length(), "the session object takes the disposed path")
}

// TestProxyDispose_DirectCallWithoutToken calls the raw shared dispose
// with a live id and no token object, as only a direct call can: it
// must dispose the session without passing the missing or non-object
// token to unregister, which would throw and end the Go program.
func TestProxyDispose_DirectCallWithoutToken(t *testing.T) {
	dispose := sharedMethods()["dispose"]
	for name, extra := range map[string][]any{
		"id alone":         nil,
		"non-object token": {"tok"},
	} {
		t.Run(name, func(t *testing.T) {
			proxy, id := newTestProxyWithID(t)
			defer proxy.Call("dispose")
			args := append([]any{id}, extra...)
			require.NotPanics(t, func() { dispose.Invoke(args...) })
			assert.NotContains(t, sessions, id)
		})
	}
}

// recordFuncs swaps the funcOf seam for one that appends each func it
// registers to the returned slice, and restores the seam when t ends.
// A caller must not run in parallel.
func recordFuncs(t *testing.T) *[]js.Func {
	t.Helper()
	oldOf := funcOf
	t.Cleanup(func() { funcOf = oldOf })
	made := new([]js.Func)
	funcOf = func(fn func(js.Value, []js.Value) any) js.Func {
		f := oldOf(fn)
		*made = append(*made, f)
		return f
	}
	return made
}

// TestBindFinalizer checks the real FinalizationRegistry yields bound
// register, unregister, and keep-alive funcs and an empty queue, and
// that its cleanup callback is no Go func, so collection never calls
// into Go. A host whose registry is missing or unusable (not a
// constructor, or a stub without register or unregister) gets undefined
// for every field instead of a JS exception, which in main would stop
// the engine from loading.
func TestBindFinalizer(t *testing.T) {
	sharedMethods() // captures bindTo, which bindFinalizer binds through
	made := recordFuncs(t)
	f := bindFinalizer(js.Global().Get("FinalizationRegistry"))
	assert.Equal(t, js.TypeFunction, f.register.Type())
	assert.Equal(t, js.TypeFunction, f.unregister.Type())
	assert.Equal(t, js.TypeFunction, f.keep.Type())
	require.True(t, js.Global().Get("Array").Call("isArray", f.queue).Bool(), "queue is an array")
	assert.Zero(t, f.queue.Length())
	assert.Empty(t, *made, "the cleanup callback is no Go func")
	// funcOf only sees Go funcs made through the seam; a callback built
	// with js.FuncOf directly would slip past it. A Go func reaches JS as
	// a wasm_exec.js wrapper whose source is plain JS, while a bound
	// native function prints as [native code].
	holder := js.Global().Get("Object").New()
	recording := js.Global().Get("Function").New("h",
		"return class extends FinalizationRegistry { constructor(cb) { super(cb); h.cb = cb; } }").Invoke(holder)
	bindFinalizer(recording)
	cb := holder.Get("cb")
	require.Equal(t, js.TypeFunction, cb.Type(), "the registry got a cleanup callback")
	src := js.Global().Get("Function").Get("prototype").Get("toString").Call("call", cb).String()
	assert.Contains(t, src, "[native code]", "the cleanup callback is a native function, not a Go func")

	fn := js.Global().Get("Function")
	for name, ctor := range map[string]js.Value{
		"undefined":         js.Undefined(),
		"non-function":      js.ValueOf(1),
		"not a constructor": fn.New("return () => {}").Invoke(),
		"no register":       fn.New(""),
		"no unregister":     fn.New("this.register = function () {}"),
	} {
		t.Run(name, func(t *testing.T) {
			var f sessionFinalizer
			require.NotPanics(t, func() { f = bindFinalizer(ctor) })
			assert.True(t, f.register.IsUndefined(), "no registry, no register")
			assert.True(t, f.unregister.IsUndefined(), "no registry, no unregister")
			assert.True(t, f.keep.IsUndefined(), "no registry, no keep-alive")
			assert.True(t, f.queue.IsUndefined(), "no registry, no queue")
		})
	}
}

// TestSessionWithoutFinalizationRegistry runs a session through create
// and dispose with the seams bindFinalizer leaves on a host that has no
// FinalizationRegistry: the engine still loads and works, dispose() being
// the only way to free a session.
func TestSessionWithoutFinalizationRegistry(t *testing.T) {
	sharedMethods()
	old := finalizer
	t.Cleanup(func() { finalizer = old })
	finalizer = sessionFinalizer{}

	var proxy js.Value
	var id int64
	require.NotPanics(t, func() { proxy, id = newTestProxyWithID(t) })
	require.Contains(t, sessions, id)
	require.NotPanics(t, func() { proxy.Call("dispose") })
	assert.NotContains(t, sessions, id)
}

// TestBindMethods_TokenOnDisposeAndKeepAlive checks the token's two
// holders: dispose is bound to it, for unregister, and every other
// method is a finalizer.keep key whose value is the token, so the token
// lives as long as any method without riding along on each call.
func TestBindMethods_TokenOnDisposeAndKeepAlive(t *testing.T) {
	sharedMethods()
	old := finalizer
	var keys, vals []js.Value
	rec := js.FuncOf(func(_ js.Value, args []js.Value) any {
		keys, vals = append(keys, args[0]), append(vals, args[1])
		return nil
	})
	t.Cleanup(rec.Release)
	// Registered after rec.Release, so the seam is restored before the
	// recording func is released.
	t.Cleanup(func() { finalizer = old })
	finalizer.keep = rec.Value

	proxy := js.Global().Get("Object").New()
	tok := js.Global().Get("Object").New()
	names := sessionMethodNames()
	bindMethods(proxy, names, sharedMethods(), -1, tok)

	require.Len(t, keys, len(names)-1, "one keep-alive entry per method but dispose")
	for _, v := range vals {
		assert.True(t, v.Equal(tok), "each entry's value is the token")
	}
	for _, k := range keys {
		assert.False(t, k.Equal(proxy.Get("dispose")), "dispose holds the token itself")
	}
}

// TestRecoverJS checks recoverJS swallows only the panics syscall/js
// raises for a JS-side failure, so a Go bug in bindFinalizer fails at
// load instead of silently turning off dropped-session collection.
func TestRecoverJS(t *testing.T) {
	run := func(p func()) (caught bool) {
		defer recoverJS(func() { caught = true })
		p()
		return false
	}
	assert.False(t, run(func() {}), "no panic, no onJS")
	assert.True(t, run(func() { panic(js.Error{Value: js.ValueOf("boom")}) }), "a JS exception")
	assert.True(t, run(func() { js.Undefined().Get("x") }), "a Value method on the wrong type")
	assert.PanicsWithValue(t, "go bug", func() { run(func() { panic("go bug") }) })
	assert.Panics(t, func() {
		run(func() { _ = indexEmpty(1) })
	}, "a Go runtime error is re-raised")
}

// TestBindFinalizer_NoWeakMap checks a host with a FinalizationRegistry
// but no WeakMap (a Value method on undefined, so a *js.ValueError
// rather than a JS exception) still loads, without the fallback.
func TestBindFinalizer_NoWeakMap(t *testing.T) {
	sharedMethods()
	g := js.Global()
	weakMap := g.Get("WeakMap")
	g.Delete("WeakMap")
	t.Cleanup(func() { g.Set("WeakMap", weakMap) })
	var f sessionFinalizer
	require.NotPanics(t, func() { f = bindFinalizer(g.Get("FinalizationRegistry")) })
	assert.True(t, f.register.IsUndefined(), "no WeakMap, no register")
	assert.True(t, f.queue.IsUndefined(), "no WeakMap, no queue")
}

// indexEmpty indexes an empty slice at i, a Go runtime error for any i.
func indexEmpty(i int) int { return []int{}[i] }
