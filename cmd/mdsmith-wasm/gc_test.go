//go:build js && wasm

package main

import (
	"runtime"
	"syscall/js"
	"testing"
	"time"

	"github.com/jeduden/mdsmith/pkg/mdsmith"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// jsGC returns a function that forces a V8 collection, or nil when the
// host offers no way to. Node exposes gc only under --expose-gc, which
// the go_js_wasm_exec wrapper does not pass, so it turns the flag on at
// run time and reads gc out of a fresh context.
func jsGC(t *testing.T) func() {
	t.Helper()
	g := js.Global()
	if gc := g.Get("gc"); gc.Type() == js.TypeFunction {
		return func() { gc.Invoke() }
	}
	req := g.Get("require")
	if req.Type() != js.TypeFunction {
		return nil
	}
	req.Invoke("v8").Call("setFlagsFromString", "--expose-gc")
	gc := req.Invoke("vm").Call("runInNewContext", "gc")
	if gc.Type() != js.TypeFunction {
		return nil
	}
	return func() { gc.Invoke() }
}

// collectUntil forces garbage collection on both sides until done
// reports true, or fails t after a bounded number of rounds. syscall/js
// frees a Go-held reference from a Go finalizer, so Go collects first;
// each sleep hands control to the JS event loop, where V8 runs its
// FinalizationRegistry callbacks.
func collectUntil(t *testing.T, done func() bool) {
	t.Helper()
	gc := jsGC(t)
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
		// The object is gone after the first round, so a premature
		// finalizer would have fired by the last.
		for i := 0; i < 20; i++ {
			runtime.GC()
			jsGC(t)()
			time.Sleep(5 * time.Millisecond)
		}
		require.Contains(t, sessions, id, "session kept alive by its method")
		diags, rejected := awaitPromise(t, check.Invoke("a.md", "# A\n"))
		require.False(t, rejected, "check through the kept method: %v", diags)
		assert.Zero(t, diags.Length(), "clean file has no diagnostics")
		return id
	}()
	collectUntil(t, func() bool { _, ok := sessions[id]; return !ok })
}

// countCalls wraps fn in a JS function that adds one to *n per call.
func countCalls(fn js.Value, n *int) js.Value {
	counter := js.FuncOf(func(js.Value, []js.Value) any { *n++; return nil })
	return js.Global().Get("Function").New("f", "c",
		"return function(...a){ c(); return f.apply(this, a); }").Invoke(fn, counter)
}

func TestExplicitDisposeCancelsFinalizer(t *testing.T) {
	oldReg, oldUnreg := registerFinalizer, unregisterFinalizer
	t.Cleanup(func() { registerFinalizer, unregisterFinalizer = oldReg, oldUnreg })
	registers, unregisters := 0, 0
	registerFinalizer = countCalls(oldReg, &registers)
	unregisterFinalizer = countCalls(oldUnreg, &unregisters)

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
	t.Cleanup(func() { delete(sessions, id) })
	for i := 0; i < 20; i++ {
		runtime.GC()
		jsGC(t)()
		time.Sleep(5 * time.Millisecond)
	}
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
