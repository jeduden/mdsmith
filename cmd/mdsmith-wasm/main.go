//go:build js && wasm

// Command mdsmith-wasm is the WebAssembly entry point for the public
// mdsmith engine. It registers a JavaScript factory —
// globalThis.mdsmith.createSession — that mirrors pkg/mdsmith.NewSession
// one-to-one, plus globalThis.mdsmith.version. The session object it
// returns carries each Go Session method by the same name (check, fix,
// kinds, capabilities, invalidate, dispose).
//
// Build with cmd/mdsmith-wasm/build.sh. The design — the open method
// namespace, the cache contract, and the WASM limits — lives in
// docs/background/concepts/engine-api.md.
package main

import (
	"runtime/debug"
	"syscall/js"

	mdsmith "github.com/jeduden/mdsmith/pkg/mdsmith"
)

// version is set via ldflags at build time (-X main.version=v1.0.0),
// mirroring cmd/mdsmith. It falls back to the module build info.
var version string

func main() {
	js.Global().Set("mdsmith", js.ValueOf(map[string]any{
		"createSession": js.FuncOf(createSession),
		"version":       resolveVersion(),
	}))
	// Block forever so the registered callbacks stay alive; a WASM
	// main that returns tears down the Go runtime and the exported
	// functions with it.
	select {}
}

// resolveVersion mirrors cmd/mdsmith.printVersion's resolution so the
// CLI and the WASM build report the same string for the same build.
func resolveVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

// createSession builds a Session from a JS options object and returns a
// Promise that resolves to a JS session proxy. The options object has
// the shape { workspace: Record<string,string>, configYAML: string }.
//
// It returns a Promise because WebAssembly.instantiate is async on the
// JS side; NewSession itself is synchronous, but a uniform Promise-
// returning factory keeps the JS API ergonomic.
func createSession(_ js.Value, args []js.Value) any {
	return newPromise(func(resolve, reject func(any)) {
		if len(args) < 1 || args[0].Type() != js.TypeObject {
			reject(jsError("createSession requires an options object"))
			return
		}
		opts := args[0]

		ws := mdsmith.NewMemWorkspace(workspaceFromJS(opts.Get("workspace")))
		configYAML := ""
		if cy := opts.Get("configYAML"); cy.Type() == js.TypeString {
			configYAML = cy.String()
		}

		sess, err := mdsmith.NewSession(mdsmith.SessionOptions{
			Workspace: ws,
			Config:    mdsmith.ConfigYAML(configYAML),
		})
		if err != nil {
			reject(jsError(err.Error()))
			return
		}
		resolve(newSessionProxy(sess, ws))
	})
}

// workspaceFromJS converts a JS Record<string,string> into the
// map[string][]byte a MemWorkspace expects. A non-object value yields
// an empty workspace.
func workspaceFromJS(v js.Value) map[string][]byte {
	if v.Type() != js.TypeObject {
		return nil
	}
	keys := js.Global().Get("Object").Call("keys", v)
	n := keys.Length()
	out := make(map[string][]byte, n)
	for i := 0; i < n; i++ {
		key := keys.Index(i).String()
		val := v.Get(key)
		if val.Type() == js.TypeString {
			out[key] = []byte(val.String())
		}
	}
	return out
}

// newSessionProxy builds the JS object whose methods forward to the Go
// Session. Method names match the Go method names exactly; the WASM
// smoke test and a native test assert the set equals
// pkg/mdsmith.Session's capability list.
func newSessionProxy(sess *mdsmith.Session, ws *mdsmith.MemWorkspace) js.Value {
	return js.ValueOf(map[string]any{
		"check": js.FuncOf(func(_ js.Value, args []js.Value) any {
			return newPromise(func(resolve, reject func(any)) {
				uri, src, ok := uriAndSource(args)
				if !ok {
					reject(jsError("check(uri, src) requires two string arguments"))
					return
				}
				diags, err := sess.Check(uri, src)
				if err != nil {
					reject(jsError(err.Error()))
					return
				}
				resolve(toJS(diags))
			})
		}),
		"fix": js.FuncOf(func(_ js.Value, args []js.Value) any {
			return newPromise(func(resolve, reject func(any)) {
				uri, src, ok := uriAndSource(args)
				if !ok {
					reject(jsError("fix(uri, src) requires two string arguments"))
					return
				}
				res, err := sess.Fix(uri, src)
				if err != nil {
					reject(jsError(err.Error()))
					return
				}
				resolve(toJS(res))
			})
		}),
		"kinds": js.FuncOf(func(_ js.Value, args []js.Value) any {
			return newPromise(func(resolve, reject func(any)) {
				if len(args) < 1 || args[0].Type() != js.TypeString {
					reject(jsError("kinds(uri) requires a string argument"))
					return
				}
				res, err := sess.Kinds(args[0].String())
				if err != nil {
					reject(jsError(err.Error()))
					return
				}
				resolve(toJS(res))
			})
		}),
		"capabilities": js.FuncOf(func(_ js.Value, _ []js.Value) any {
			caps := sess.Capabilities()
			arr := make([]any, len(caps))
			for i, c := range caps {
				arr[i] = c
			}
			return js.ValueOf(arr)
		}),
		"invalidate": js.FuncOf(func(_ js.Value, args []js.Value) any {
			if len(args) < 1 || args[0].Type() != js.TypeString {
				return js.Undefined()
			}
			uri := args[0].String()
			if len(args) >= 2 && args[1].Type() == js.TypeString {
				sess.Invalidate(uri, []byte(args[1].String()))
			} else {
				sess.Invalidate(uri)
			}
			return js.Undefined()
		}),
		"dispose": js.FuncOf(func(_ js.Value, _ []js.Value) any {
			sess.Dispose()
			return js.Undefined()
		}),
	})
}

// uriAndSource pulls a (uri string, source []byte) pair from JS args.
// A JS string source crosses as Go []byte while the URI stays a
// string, matching the design contract.
func uriAndSource(args []js.Value) (string, []byte, bool) {
	if len(args) < 2 || args[0].Type() != js.TypeString || args[1].Type() != js.TypeString {
		return "", nil, false
	}
	return args[0].String(), []byte(args[1].String()), true
}
