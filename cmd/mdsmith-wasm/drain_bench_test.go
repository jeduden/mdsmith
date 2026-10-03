//go:build js && wasm

package main

import (
	"syscall/js"
	"testing"
)

// BenchmarkDrainFinalizedEmpty measures the drain every engine entry
// point pays when no session was collected: one Go-to-JS read of the
// queue's length. Compare BenchmarkInvalidate, the cheapest real call;
// drainFinalized's doc records the ratio. Run it under Node with
// go_js_wasm_exec (see plan 2610030846).
func BenchmarkDrainFinalizedEmpty(b *testing.B) {
	sharedMethods()
	for i := 0; i < b.N; i++ {
		drainFinalized()
	}
}

// BenchmarkInvalidate measures session.invalidate(uri) through the
// session object, drain included: the cheapest engine call a host makes
// on every edit.
func BenchmarkInvalidate(b *testing.B) {
	sharedMethods()
	opts := js.Global().Get("Object").New()
	p := exposeAPI().Get("createSession").Invoke(opts)
	ch := make(chan js.Value, 1)
	f := js.FuncOf(func(_ js.Value, a []js.Value) any { ch <- a[0]; return nil })
	p.Call("then", f)
	s := <-ch
	uri := js.ValueOf("a.md")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Call("invalidate", uri)
	}
}
