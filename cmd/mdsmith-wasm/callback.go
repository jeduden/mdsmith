//go:build js && wasm

package main

import (
	"runtime"
	"syscall/js"
)

// activeCallbacks counts the JS-to-Go callbacks trackCallback wraps
// that are running now. js/wasm runs every goroutine on one thread
// with no preemption, so it needs no lock.
var activeCallbacks int

// trackCallback wraps fn, a func the engine hands JS through
// js.FuncOf, so activeCallbacks counts it while it runs. Every such
// func goes through it: the funcOf seam and exposeAPI both wrap. A
// count that falls short of the handleEvent frames on the stack tells
// escapedCallback that a panic came from syscall/js around a callback,
// not from one.
func trackCallback(fn func(js.Value, []js.Value) any) func(js.Value, []js.Value) any {
	return func(this js.Value, args []js.Value) any {
		activeCallbacks++
		defer func() { activeCallbacks-- }()
		return fn(this, args)
	}
}

// handleEventFrame is the syscall/js function that runs each JS-to-Go
// callback. A nested callback runs on the goroutine of the Go code that
// called JS, above that code's frames, so its frame sits between them.
const handleEventFrame = "syscall/js.handleEvent"

// handleEventFrames counts the handleEventFrame frames on the calling
// goroutine's stack: one per JS-to-Go callback running on it.
func handleEventFrames() int {
	pcs := make([]uintptr, 64)
	n := runtime.Callers(2, pcs)
	for n == len(pcs) {
		pcs = make([]uintptr, 2*len(pcs))
		n = runtime.Callers(2, pcs)
	}
	count := 0
	frames := runtime.CallersFrames(pcs[:n])
	for {
		f, more := frames.Next()
		if f.Function == handleEventFrame {
			count++
		}
		if !more {
			return count
		}
	}
}

// escapedCallback reports, from a deferred func recovering a panic,
// whether the stack holds a handleEvent frame that no tracked callback
// accounts for. The panic then came from syscall/js's own code around
// a callback (reading the event's arguments, or writing its result
// back), and recovering it in a frame below would resume Go while the
// JS that called the callback is still on the wasm stack. A tracked
// callback a panic escaped has already dropped its count, so that is
// reported too. The count is global, so a tracked callback blocked on
// another goroutine can hide one; the engine's callbacks never block.
func escapedCallback() bool {
	return handleEventFrames() > activeCallbacks
}

// repanicUnlessJS re-raises r, a value a deferred func recovered,
// unless it is a JS-side failure Go may resume after: a js.Error (a JS
// exception from Call, Invoke, or New) or a *js.ValueError (a Value
// method on the wrong type, such as Get on undefined) that did not
// unwind out of a JS-to-Go callback (escapedCallback). recoverJS and
// rejectOnJSError both classify through it, so the two guards always
// recover the same panics.
func repanicUnlessJS(r any) {
	switch r.(type) {
	case js.Error, *js.ValueError:
		if !escapedCallback() {
			return
		}
	}
	panic(r)
}
