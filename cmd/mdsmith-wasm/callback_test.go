//go:build js && wasm

package main

import (
	"syscall/js"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// callRaw runs body inside a JS-to-Go callback that trackCallback does
// not count, the stack shape a panic has when it comes from
// syscall/js's own handleEvent code rather than from a callback body.
func callRaw(t *testing.T, body func()) {
	t.Helper()
	f := js.FuncOf(func(js.Value, []js.Value) any {
		body()
		return nil
	})
	defer f.Release()
	f.Invoke()
}

// callTracked runs body inside a JS-to-Go callback that trackCallback
// counts, the stack shape of every func the engine registers.
func callTracked(t *testing.T, body func()) {
	t.Helper()
	f := js.FuncOf(trackCallback(func(js.Value, []js.Value) any {
		body()
		return nil
	}))
	defer f.Release()
	f.Invoke()
}

// TestHandleEventFrames checks handleEventFrames counts one
// syscall/js.handleEvent frame per JS-to-Go callback on the stack.
func TestHandleEventFrames(t *testing.T) {
	assert.Zero(t, handleEventFrames(), "a test goroutine runs in no callback")
	var one, two int
	callRaw(t, func() {
		one = handleEventFrames()
		callRaw(t, func() { two = handleEventFrames() })
	})
	assert.Equal(t, 1, one)
	assert.Equal(t, 2, two, "a nested callback adds a frame")
	var deep int
	callRaw(t, func() { deep = handleEventFramesAt(100) })
	assert.Equal(t, 1, deep, "a frame below more frames than the first Callers buffer holds")
}

// handleEventFramesAt is handleEventFrames called depth frames deeper,
// so the handleEvent frame sits below the first buffer Callers fills.
//
//go:noinline
func handleEventFramesAt(depth int) int {
	if depth == 0 {
		return handleEventFrames()
	}
	return handleEventFramesAt(depth - 1)
}

// TestRepanicUnlessJS checks repanicUnlessJS returns for a js.Error or
// *js.ValueError raised outside any callback or inside a tracked one,
// and re-raises one that unwound out of an untracked callback, and any
// other panic, unchanged.
func TestRepanicUnlessJS(t *testing.T) {
	jsErr := js.Error{Value: js.ValueOf("boom")}
	valueErr := &js.ValueError{Method: "Value.Get", Type: js.TypeUndefined}
	// raised runs repanicUnlessJS(v) and returns what it re-raised, nil
	// when it returned. testify's PanicsWithValue compares with !=, which
	// panics on a js.Error: the js.Value in it is not comparable.
	raised := func(v any) (r any) {
		defer func() { r = recover() }()
		repanicUnlessJS(v)
		return nil
	}
	assert.Nil(t, raised(jsErr), "a js.Error")
	assert.Nil(t, raised(valueErr), "a *js.ValueError")
	assert.Equal(t, "go bug", raised("go bug"), "any other panic")
	var trackedJS, trackedValue, rawJS, rawValue any
	callTracked(t, func() {
		trackedJS, trackedValue = raised(jsErr), raised(valueErr)
	})
	callRaw(t, func() {
		rawJS, rawValue = raised(jsErr), raised(valueErr)
	})
	assert.Nil(t, trackedJS, "a js.Error in a tracked callback")
	assert.Nil(t, trackedValue, "a *js.ValueError in a tracked callback")
	require.IsType(t, js.Error{}, rawJS, "an escaped js.Error is re-raised")
	assert.True(t, rawJS.(js.Error).Equal(jsErr.Value), "unchanged")
	assert.Same(t, valueErr, rawValue, "an escaped *js.ValueError is re-raised unchanged")
}

// TestTrackCallback checks trackCallback counts its callback as running
// for exactly the call, a panicking one included, and passes this,
// args, and the result through.
func TestTrackCallback(t *testing.T) {
	require.Zero(t, activeCallbacks)
	var during int
	this := js.ValueOf("this")
	got := trackCallback(func(th js.Value, args []js.Value) any {
		during = activeCallbacks
		assert.True(t, th.Equal(this))
		assert.Len(t, args, 1)
		return "result"
	})(this, []js.Value{js.ValueOf(1)})
	assert.Equal(t, "result", got)
	assert.Equal(t, 1, during)
	assert.Zero(t, activeCallbacks, "the count drops after the call")
	assert.Panics(t, func() {
		trackCallback(func(js.Value, []js.Value) any { panic("boom") })(js.Undefined(), nil)
	})
	assert.Zero(t, activeCallbacks, "and after a panicking call")
}

// TestEscapedCallback checks escapedCallback reports a stack with a
// handleEvent frame no tracked callback accounts for.
func TestEscapedCallback(t *testing.T) {
	assert.False(t, escapedCallback(), "no callback at all")
	var raw, tracked bool
	callRaw(t, func() { raw = escapedCallback() })
	callTracked(t, func() { tracked = escapedCallback() })
	assert.True(t, raw, "an untracked callback frame")
	assert.False(t, tracked, "a tracked callback frame")
}

// TestRecoverJS_EscapedCallbackRepanics checks recoverJS and
// rejectOnJSError re-raise a JS-side failure that unwound through a
// handleEvent frame no tracked callback accounts for. Go cannot resume
// below a JS-to-Go callback the panic left: the JS that called it is
// still on the wasm stack. Inside a tracked callback both still recover.
func TestRecoverJS_EscapedCallbackRepanics(t *testing.T) {
	for _, tt := range []struct {
		name string
		v    any
	}{
		{"js.Error", js.Error{Value: js.ValueOf("boom")}},
		{"*js.ValueError", &js.ValueError{Method: "Value.Get", Type: js.TypeUndefined}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recovers := func() (caught bool) {
				defer recoverJS(func() { caught = true })
				panic(tt.v)
			}
			rejects := func() (rejected bool) {
				defer rejectOnJSError(func(any) { rejected = true })
				panic(tt.v)
			}
			callRaw(t, func() {
				assert.Panics(t, func() { recovers() }, "recoverJS re-raises")
				assert.Panics(t, func() { rejects() }, "rejectOnJSError re-raises")
			})
			callTracked(t, func() {
				assert.True(t, recovers(), "recoverJS recovers in a tracked callback")
				assert.True(t, rejects(), "rejectOnJSError rejects in a tracked callback")
			})
		})
	}
}

// TestRegisteredFuncsAreTracked checks every func the engine hands JS,
// through the funcOf seam and through exposeAPI, counts as a running
// callback while JS calls it. The exposeAPI func is released at the
// end so the test leaves no handler in the func table.
func TestRegisteredFuncsAreTracked(t *testing.T) {
	var during int
	f := funcOf(func(js.Value, []js.Value) any { during = activeCallbacks; return nil })
	defer releaseFunc(f)
	f.Invoke()
	assert.Equal(t, 1, during, "a funcOf func is tracked")

	old := apiFuncs
	t.Cleanup(func() { apiFuncs = old })
	apiFuncs = map[string]func(js.Value, []js.Value) any{
		"probe": func(js.Value, []js.Value) any { during = activeCallbacks; return nil },
	}
	during = 0
	made := recordFuncs(t)
	exposeAPI().Call("probe")
	assert.Len(t, *made, 1, "exposeAPI registers through the funcOf seam")
	for _, f := range *made {
		releaseFunc(f)
	}
	assert.Equal(t, 1, during, "an exposeAPI func is tracked")
}
