//go:build js && wasm

package main

import (
	"encoding/json"
	"syscall/js"

	"github.com/jeduden/mdsmith/pkg/mdsmith"
)

// newPromise wraps a Go executor in a JavaScript Promise. The executor
// receives resolve and reject callbacks; any Go method returning
// (T, error) maps to a Promise<T> that rejects with new Error(msg).
//
// The js.Func backing the executor is released inside the executor so
// it is freed once Promise construction calls it (Promise executors run
// synchronously during construction).
//
// A JS exception the executor raises (a js.Error panic) rejects the
// Promise with that exception (see rejectOnJSError), so no executor
// needs its own guard.
func newPromise(executor func(resolve, reject func(any))) js.Value {
	var handler js.Func
	released := false
	handler = funcOf(func(_ js.Value, pArgs []js.Value) any {
		resolveFn := pArgs[0]
		rejectFn := pArgs[1]
		resolve := func(v any) { resolveFn.Invoke(v) }
		reject := func(v any) { rejectFn.Invoke(v) }
		// Free this handler now that the executor body has captured
		// the resolve/reject functions; the executor runs to
		// completion synchronously within Promise construction for
		// our synchronous engine calls.
		defer func() {
			released = true
			releaseFunc(handler)
		}()
		defer rejectOnJSError(reject)
		executor(resolve, reject)
		return js.Undefined()
	})
	return constructPromise(handler, func() {
		if !released {
			releaseFunc(handler)
		}
	})
}

// constructPromise is new Promise(handler). A JS exception the
// constructor raises before it calls the executor (a patched Promise)
// would end the program, and handler, never run, would stay registered:
// it calls release and returns undefined instead, since Go cannot throw
// to the caller. If the executor already ran, release is a second call
// on a freed func, so newPromise's release callback skips it.
func constructPromise(handler js.Func, release func()) (p js.Value) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if _, ok := r.(js.Error); !ok {
			panic(r)
		}
		release()
		p = js.Undefined()
	}()
	return js.Global().Get("Promise").New(handler)
}

// jsError constructs a JavaScript Error with the given message, the
// rejection value the design contract specifies for failed methods.
func jsError(msg string) js.Value {
	return js.Global().Get("Error").New(msg)
}

// jsErrorFor builds a JS Error from a Go engine error: its message is
// err.Error(), and a non-empty mdsmith.ErrorCode(err) is set as the
// Error's `code` property (as Node does for system errors), so a host
// can branch on a harmless outcome such as "nothing-to-rename" without
// matching message text.
func jsErrorFor(err error) js.Value {
	e := jsError(err.Error())
	if code := mdsmith.ErrorCode(err); code != "" {
		e.Set("code", code)
	}
	return e
}

// toJS marshals a Go value to JSON and parses it back into a native JS
// value via JSON.parse, so the object the caller receives has exactly
// the wire shape the CLI and LSP emit (snake_case keys, omitempty
// fields). Marshalling cannot realistically fail for the engine's
// result types; on the off chance it does, null is returned.
func toJS(v any) js.Value {
	data, err := json.Marshal(v)
	if err != nil {
		return js.Null()
	}
	return js.Global().Get("JSON").Call("parse", string(data))
}

// rejectOnJSError, which newPromise defers around every executor, turns
// a JS exception that a syscall/js Call, Invoke, or New raised as a
// js.Error panic into a rejection with that exception. Inspecting a
// caller's object can throw (a revoked Proxy, a Proxy trap that throws),
// and an unrecovered panic in a js.FuncOf callback ends the Go program
// and every session with it. wasm_exec.js caught the exception before Go
// panicked, so the runtime is intact. Any other panic is re-raised
// unchanged. TinyGo does not implement recover() on WebAssembly, so in a
// TinyGo build the exception still ends the program.
func rejectOnJSError(reject func(any)) {
	r := recover()
	if r == nil {
		return
	}
	if e, ok := r.(js.Error); ok {
		reject(e.Value)
		return
	}
	panic(r)
}

// typeUnknown is what jsType reports for a value syscall/js has no
// js.Type for.
const typeUnknown js.Type = -1

// jsType is v.Type() for a value a caller passed in. syscall/js panics
// with "bad type flag" on a typeof it does not model (a BigInt), and a
// panic in a js.FuncOf callback ends the Go program and every session
// with it, so jsType reports typeUnknown instead, which every check
// treats as a wrong type. TinyGo does not implement recover() on
// WebAssembly, so in a TinyGo build the panic still ends the program.
func jsType(v js.Value) (t js.Type) {
	defer func() {
		if recover() != nil {
			t = typeUnknown
		}
	}()
	return v.Type()
}
