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
// Promise with that exception, and a *js.ValueError rejects it with an
// Error (see rejectOnJSError), so no executor needs its own guard.
//
// A patched Promise can fail around the executor too. A constructor
// that throws, or a Promise that is no constructor, would end the
// program, and a handler it never ran would stay registered: newPromise
// releases that handler and returns undefined instead, since Go cannot
// throw to its caller. A constructor that returns without running the
// executor, which a spec Promise runs during construction, gets the
// same treatment: its handler would otherwise stay registered for good,
// and a later call to the released handler only logs an error. A
// constructor that passes the executor too few arguments, or a reject
// that itself throws or is no function, would end the program from
// inside the handler callback: a panic that leaves a js.FuncOf callback
// unwinds into the Go frames below the JS that called it. The handler
// swallows that failure, and the Promise stays as the constructor left
// it, never settling. Any other panic is re-raised (recoverJS).
func newPromise(executor func(resolve, reject func(any))) (p js.Value) {
	// One heap object for the handler and its ran flag: the escaping
	// callback captures both.
	st := new(promiseHandler)
	st.f = funcOf(func(_ js.Value, pArgs []js.Value) any {
		st.ran = true
		// Free this handler once the executor body returns; the executor
		// runs to completion synchronously within Promise construction
		// for our synchronous engine calls.
		defer releaseFunc(st.f)
		defer recoverJS(func() {})
		var resolveFn, rejectFn js.Value // undefined unless passed
		if len(pArgs) > 0 {
			resolveFn = pArgs[0]
		}
		if len(pArgs) > 1 {
			rejectFn = pArgs[1]
		}
		resolve := func(v any) { resolveFn.Invoke(v) }
		reject := func(v any) { rejectFn.Invoke(v) }
		defer rejectOnJSError(reject)
		executor(resolve, reject)
		return js.Undefined()
	})
	// A handler the constructor already ran released itself.
	defer recoverJS(func() {
		if !st.ran {
			releaseFunc(st.f)
		}
		p = js.Undefined()
	})
	p = js.Global().Get("Promise").New(st.f)
	if !st.ran {
		releaseFunc(st.f)
		return js.Undefined()
	}
	return p
}

// promiseHandler is newPromise's executor func and whether the Promise
// constructor ran it.
type promiseHandler struct {
	f   js.Func
	ran bool
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
// a JS-side failure (the panics recoverJS recovers) into a rejection: a
// JS exception that a syscall/js Call, Invoke, or New raised as a
// js.Error panic rejects with that exception, and a *js.ValueError (a
// Value method on the wrong type, such as Get on undefined) rejects with
// an Error carrying its message. Inspecting a caller's object can throw
// (a revoked Proxy, a Proxy trap that throws), and an unrecovered panic
// in a js.FuncOf callback ends the Go program and every session with it.
// newPromise's executor callback swallows a failure this does not turn
// into a rejection, which would leave the Promise pending forever.
// wasm_exec.js caught the exception before Go panicked, so the runtime
// is intact. Any other panic is re-raised unchanged, and so is a
// JS-side failure that unwound out of a JS-to-Go callback, as in
// recoverJS. TinyGo does not implement recover() on WebAssembly, so in
// a TinyGo build the exception still ends the program.
func rejectOnJSError(reject func(any)) {
	r := recover()
	if r == nil {
		return
	}
	repanicUnlessJS(r)
	switch e := r.(type) {
	case js.Error:
		reject(e.Value)
	case *js.ValueError:
		reject(jsError(e.Error()))
	}
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
