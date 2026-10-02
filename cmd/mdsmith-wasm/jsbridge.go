//go:build js && wasm

package main

import (
	"encoding/json"
	"syscall/js"
)

// newPromise wraps a Go executor in a JavaScript Promise. The executor
// receives resolve and reject callbacks; any Go method returning
// (T, error) maps to a Promise<T> that rejects with new Error(msg).
//
// The js.Func backing the executor is released inside the executor so
// it is freed once Promise construction calls it (Promise executors run
// synchronously during construction).
func newPromise(executor func(resolve, reject func(any))) js.Value {
	var handler js.Func
	handler = funcOf(func(_ js.Value, pArgs []js.Value) any {
		resolveFn := pArgs[0]
		rejectFn := pArgs[1]
		resolve := func(v any) { resolveFn.Invoke(v) }
		reject := func(v any) { rejectFn.Invoke(v) }
		// Free this handler now that the executor body has captured
		// the resolve/reject functions; the executor runs to
		// completion synchronously within Promise construction for
		// our synchronous engine calls.
		defer releaseFunc(handler)
		executor(resolve, reject)
		return js.Undefined()
	})
	return js.Global().Get("Promise").New(handler)
}

// jsError constructs a JavaScript Error with the given message, the
// rejection value the design contract specifies for failed methods.
func jsError(msg string) js.Value {
	return js.Global().Get("Error").New(msg)
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

// typeUnknown is what jsType reports for a value syscall/js has no
// js.Type for.
const typeUnknown js.Type = -1

// jsType is v.Type() for a value a caller passed in. syscall/js panics
// with "bad type flag" on a typeof it does not model (a BigInt), and a
// panic in a js.FuncOf callback ends the Go program and every session
// with it, so jsType reports typeUnknown instead, which every check
// treats as a wrong type.
func jsType(v js.Value) (t js.Type) {
	defer func() {
		if recover() != nil {
			t = typeUnknown
		}
	}()
	return v.Type()
}
