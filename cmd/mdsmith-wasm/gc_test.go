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
	gc := jsGC()
	if gc == nil {
		t.Skip("host cannot force a JS garbage collection")
	}
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

// collectDropped drops a control session and collects until it has left
// sessions. Anything dropped before the call that a finalizer would
// free has then been freed too, so a test can assert that something it
// dropped earlier was kept without passing only because no collection
// ran.
func collectDropped(t *testing.T) {
	t.Helper()
	control := newDroppedSession(t)
	collectUntil(t, func() bool { _, ok := sessions[control]; return !ok })
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
	collectUntil(t, func() bool { _, ok := sessions[id]; return !ok })
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
	collectUntil(t, func() bool { _, ok := sessions[id]; return !ok })
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
	oldReg, oldUnreg := registerFinalizer, unregisterFinalizer
	registers, unregisters := 0, 0
	countReg := countCalls(t, oldReg, &registers)
	countUnreg := countCalls(t, oldUnreg, &unregisters)
	// Registered after countCalls, so the seams are restored before the
	// counting funcs are released.
	t.Cleanup(func() { registerFinalizer, unregisterFinalizer = oldReg, oldUnreg })
	registerFinalizer, unregisterFinalizer = countReg, countUnreg

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

func TestFinalizeSession_DisposesLiveIDOnce(t *testing.T) {
	proxy, id := newTestProxyWithID(t)
	defer proxy.Call("dispose")
	finalizeSession(js.Undefined(), []js.Value{js.ValueOf(id)})
	assert.NotContains(t, sessions, id)
	assert.NotPanics(t, func() { finalizeSession(js.Undefined(), []js.Value{js.ValueOf(id)}) })
	assert.NotPanics(t, func() { finalizeSession(js.Undefined(), nil) })
}

// TestFinalizeSession_IgnoresNonIDHeldValue passes held values that are
// not an integer id, as only a FinalizationRegistry patched before load
// could. A panic in a js.FuncOf callback ends the Go program and every
// session with it, so each must dispose nothing instead.
func TestFinalizeSession_IgnoresNonIDHeldValue(t *testing.T) {
	proxy, id := newTestProxyWithID(t)
	defer proxy.Call("dispose")
	for _, v := range []js.Value{
		js.ValueOf("x"),
		js.Undefined(),
		js.Global().Get("BigInt").Invoke(1),
		js.Global().Get("NaN"),
	} {
		assert.NotPanics(t, func() { finalizeSession(js.Undefined(), []js.Value{v}) })
	}
	assert.Contains(t, sessions, id, "a non-id held value disposes nothing")
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
