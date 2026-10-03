//go:build js && wasm

package main

import (
	"encoding/json"
	"sync"
	"syscall/js"

	"github.com/jeduden/mdsmith/pkg/mdsmith"
)

// newPromise wraps a Go executor in a JavaScript Promise. The executor
// receives resolve and reject callbacks; any Go method returning
// (T, error) maps to a Promise<T> that rejects with new Error(msg).
//
// It registers no func per call. Every Promise is constructed with the
// one executor func sharedMethods registers at load (sharedExecutor),
// and the call's Go executor waits on promiseCalls, a Go-side stack, for
// the constructor to run it: a spec Promise runs its executor
// synchronously during construction, so the shared func runs the call
// on top of the stack. newPromise pops its call before it returns, so
// nothing outlives the call. js.FuncOf stores a handler in syscall/js's
// private func table before it builds the handler's JS wrapper through
// Reflect.apply, and a patched one that throws there would strand the
// entry, with its closure over the Session, for good; building no
// wrapper per call leaves nothing to strand, whatever Reflect.apply,
// Reflect.get, or _makeFuncWrapper a page installs. Plan 2610031420.
//
// A JS exception the executor raises (a js.Error panic) rejects the
// Promise with that exception, and a *js.ValueError rejects it with an
// Error (see rejectOnJSError), so no executor needs its own guard.
//
// A patched Promise can fail around the executor too. A constructor
// that throws, or a Promise that is no constructor, would end the
// program: newPromise returns undefined instead, since Go cannot throw
// to its caller. A constructor that returns without running the
// executor, which a spec Promise runs during construction, gets the
// same treatment, and a later call of the executor it kept runs nothing
// (runPromiseCall). A constructor that passes the executor too few
// arguments, or a reject that itself throws or is no function, would end
// the program from inside the executor callback: a panic that leaves a
// js.FuncOf callback unwinds into the Go frames below the JS that called
// it. The callback swallows that failure, and the Promise stays as the
// constructor left it, never settling. Any other panic is re-raised
// (recoverJS).
func newPromise(executor func(resolve, reject func(any))) (p js.Value) {
	exec := sharedExecutor()
	c := &promiseCall{executor: executor}
	promiseCalls = append(promiseCalls, c)
	// Pop c whatever happens, and clear its slot so the stack's backing
	// array does not keep the executor, and the Session it closes over,
	// reachable. A call the constructor never ran yields undefined.
	defer func() {
		n := len(promiseCalls) - 1
		promiseCalls[n] = nil
		promiseCalls = promiseCalls[:n]
		if !c.ran {
			p = js.Undefined()
		}
	}()
	defer recoverJS(func() { p = js.Undefined() })
	return js.Global().Get("Promise").New(exec)
}

// promiseCall is one pending newPromise call: its Go executor, whether
// the Promise constructor ran it, how many of its runs are on the stack,
// and whether its outermost run has returned.
type promiseCall struct {
	executor func(resolve, reject func(any))
	ran      bool
	depth    int
	done     bool
}

// promiseCalls is the stack of pending newPromise calls, innermost last.
// js/wasm runs every goroutine on one thread, and a call is pushed and
// popped within one newPromise frame, so it needs no lock.
var promiseCalls []*promiseCall

// promiseExecutor is the one Promise executor func, runPromiseCall
// registered through the funcOf seam by sharedExecutor. It is never
// released.
var (
	promiseExecutor js.Value
	executorOnce    sync.Once
)

// sharedExecutor registers promiseExecutor on first use and returns it.
// sharedMethods calls it at load, before the API is reachable, so the
// one FuncOf runs before a page can have patched anything after load.
// newPromise calls it too rather than sharedMethods, whose method table
// reaches newPromise, so a test that builds a Promise first still works.
func sharedExecutor() js.Value {
	executorOnce.Do(func() { promiseExecutor = funcOf(runPromiseCall).Value })
	return promiseExecutor
}

// runPromiseCall is the shared Promise executor. It runs the Go executor
// of the newPromise call on top of promiseCalls with the resolve and
// reject the constructor passed. With no call pending (a constructor or
// script that kept the executor and calls it after newPromise returned)
// it runs nothing. A constructor can run the executor again from inside
// resolve; that nested run executes the body too, as it did when each
// call had its own func. Once the outermost run has returned the call is
// done, and a further run (a constructor that runs the executor twice in
// a row) executes nothing, as a released func would not.
func runPromiseCall(_ js.Value, pArgs []js.Value) any {
	if len(promiseCalls) == 0 {
		return js.Undefined()
	}
	c := promiseCalls[len(promiseCalls)-1]
	if c.done {
		return js.Undefined()
	}
	c.ran = true
	c.depth++
	defer func() {
		c.depth--
		if c.depth == 0 {
			c.done = true
		}
	}()
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
	c.executor(resolve, reject)
	return js.Undefined()
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
