//go:build js && wasm

// Command mdsmith-wasm is the WebAssembly entry point for the public
// mdsmith engine. It registers a JavaScript factory —
// globalThis.mdsmith.createSession — that mirrors pkg/mdsmith.NewSession
// one-to-one, plus globalThis.mdsmith.version. The session object it
// returns carries each Go Session method by the same name (check, fix,
// kinds, rename, move, capabilities, invalidate, dispose).
//
// Build with cmd/mdsmith-wasm/build.sh. The design — the open method
// namespace, the cache contract, and the WASM limits — lives in
// docs/background/concepts/engine-api.md.
package main

import (
	"runtime/debug"
	"sync"
	"syscall/js"

	"github.com/jeduden/mdsmith/internal/gctune"
	mdsmith "github.com/jeduden/mdsmith/pkg/mdsmith"
)

// funcOf and releaseFunc are js.FuncOf and js.Func.Release behind seams
// so a test can count the funcs a session's lifecycle registers (its
// proxy methods and every Promise executor), which syscall/js keeps
// private.
var (
	funcOf      = js.FuncOf
	releaseFunc = js.Func.Release
)

// version is set via ldflags at build time (-X main.version=v1.0.0),
// mirroring cmd/mdsmith. It falls back to the module build info.
var version string

// readBuildInfo is debug.ReadBuildInfo behind a seam so a test can
// drive each resolveVersion branch; a test binary always reports
// "(devel)", the same string as the final fallback.
var readBuildInfo = debug.ReadBuildInfo

func main() {
	// Apply the shared batch GC policy (internal/gctune): the WASM engine
	// runs the same check/fix work as the CLI, so it gets the same GOGC
	// default from the one source of truth. An explicit GOGC still wins.
	gctune.ApplyBatch()
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
	if info, ok := readBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

// createSession builds a Session from a JS options object and returns a
// Promise that resolves to a JS session proxy. The options object has
// the shape { workspace: Record<string,string>, configYAML: string }.
// An absent workspace means an empty one; a present workspace that is
// not a plain object (null, an array, a string) rejects, because an
// array's indices would otherwise become file paths "0", "1", ...
// Likewise an absent configYAML means the default config, and a present
// non-string rejects.
//
// It returns a Promise because WebAssembly.instantiate is async on the
// JS side; NewSession itself is synchronous, but a uniform Promise-
// returning factory keeps the JS API ergonomic.
func createSession(_ js.Value, args []js.Value) any {
	return newPromise(func(resolve, reject func(any)) {
		if len(args) < 1 || !isRecord(args[0]) {
			reject(jsError("createSession requires an options object"))
			return
		}
		opts := args[0]

		var files map[string][]byte
		if wv := opts.Get("workspace"); !wv.IsUndefined() {
			if files = workspaceFromJS(wv); files == nil {
				reject(jsError("createSession options.workspace must be an object of path to source strings"))
				return
			}
		}
		ws := mdsmith.NewMemWorkspace(files)
		// Same rule as workspace: absent means the default config, and a
		// present non-string (a Buffer, null) rejects rather than
		// silently linting with the default config.
		configYAML := ""
		if cy := opts.Get("configYAML"); !cy.IsUndefined() {
			if cy.Type() != js.TypeString {
				reject(jsError("createSession options.configYAML must be a string"))
				return
			}
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
		resolve(newSessionProxy(sess))
	})
}

// workspaceFromJS converts a JS Record<string,string> into the
// map[string][]byte a MemWorkspace expects. A value that is not a
// plain object (including null and an array) yields nil; a plain object
// always yields a non-nil map, so a caller can tell the two apart.
func workspaceFromJS(v js.Value) map[string][]byte {
	if !isRecord(v) {
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

// isRecord reports whether v is a plain JS object: its
// Object.prototype.toString tag is "[object Object]". JS typeof reports
// "object" for arrays, boxed strings, arguments, and Maps alike, which
// syscall/js mirrors as js.TypeObject (null is js.TypeNull, so the type
// check alone already rejects it). The tag rejects every one of those,
// so an array-like's indices never become file paths, and it still
// accepts an Object.create(null) record and an object from another
// realm.
func isRecord(v js.Value) bool {
	if v.Type() != js.TypeObject {
		return false
	}
	// Looked up on first use, not at package init, so loading the
	// module pays nothing for it; the zero js.Value is undefined.
	if objectToString.IsUndefined() {
		objectToString = js.Global().Get("Object").Get("prototype").Get("toString")
	}
	return objectToString.Call("call", v).String() == "[object Object]"
}

// objectToString caches Object.prototype.toString for isRecord.
var objectToString js.Value

// newSessionProxy builds the JS object whose methods forward to the Go
// Session. Method names match the Go method names exactly; the WASM
// smoke test and a native test assert the set equals
// pkg/mdsmith.Session's capability list, and a js/wasm test asserts
// the proxy's keys equal sessionMethodNames.
func newSessionProxy(sess *mdsmith.Session) js.Value {
	methods := map[string]js.Func{
		"check":        proxyCheck(sess),
		"fix":          proxyFix(sess),
		"kinds":        proxyKinds(sess),
		"rename":       proxyRename(sess),
		"move":         proxyMove(sess),
		"capabilities": proxyCapabilities(sess),
		"invalidate":   proxyInvalidate(sess),
	}
	fields := make(map[string]any, len(methods)+1)
	for name, f := range methods {
		fields[name] = f
	}
	proxy := js.ValueOf(fields)
	proxy.Set("dispose", proxyDispose(sess, proxy, methods))
	return proxy
}

// proxyCheck builds session.check(uri, src) → Promise<Diagnostic[]>.
func proxyCheck(sess *mdsmith.Session) js.Func {
	return funcOf(func(_ js.Value, args []js.Value) any {
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
			// A nil Go slice marshals to JSON null, but the check()
			// contract is Diagnostic[]; normalise a clean file to [].
			if diags == nil {
				diags = []mdsmith.Diagnostic{}
			}
			resolve(toJS(diags))
		})
	})
}

// proxyFix builds session.fix(uri, src) → Promise<FixResult>.
func proxyFix(sess *mdsmith.Session) js.Func {
	return funcOf(func(_ js.Value, args []js.Value) any {
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
			// Same nil-slice→null guard for the result's diagnostics.
			if res.Diagnostics == nil {
				res.Diagnostics = []mdsmith.Diagnostic{}
			}
			resolve(toJS(res))
		})
	})
}

// proxyKinds builds session.kinds(uri) → Promise<KindsResult>.
func proxyKinds(sess *mdsmith.Session) js.Func {
	return funcOf(func(_ js.Value, args []js.Value) any {
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
	})
}

// proxyRename builds session.rename(uri, source, as, old, new) →
// Promise<Plan>; as may be "".
func proxyRename(sess *mdsmith.Session) js.Func {
	return funcOf(func(_ js.Value, args []js.Value) any {
		return newPromise(func(resolve, reject func(any)) {
			if len(args) < 5 || !allStrings(args[:5]) {
				reject(jsError("rename(uri, source, as, old, new) requires five string arguments"))
				return
			}
			plan, err := sess.Rename(args[0].String(), []byte(args[1].String()),
				args[2].String(), args[3].String(), args[4].String())
			if err != nil {
				reject(jsError(err.Error()))
				return
			}
			resolve(toJS(plan))
		})
	})
}

// proxyMove builds session.move(src, dst) → Promise<Plan>.
func proxyMove(sess *mdsmith.Session) js.Func {
	return funcOf(func(_ js.Value, args []js.Value) any {
		return newPromise(func(resolve, reject func(any)) {
			if len(args) < 2 || !allStrings(args[:2]) {
				reject(jsError("move(src, dst) requires two string arguments"))
				return
			}
			plan, err := sess.Move(args[0].String(), args[1].String())
			if err != nil {
				reject(jsError(err.Error()))
				return
			}
			resolve(toJS(plan))
		})
	})
}

// proxyCapabilities builds the synchronous session.capabilities() →
// string[].
func proxyCapabilities(sess *mdsmith.Session) js.Func {
	return funcOf(func(_ js.Value, _ []js.Value) any {
		caps := sess.Capabilities()
		arr := make([]any, len(caps))
		for i, c := range caps {
			arr[i] = c
		}
		return js.ValueOf(arr)
	})
}

// proxyInvalidate builds the synchronous session.invalidate(uri, src?).
// A non-string uri is ignored; a string src replaces the cached source.
func proxyInvalidate(sess *mdsmith.Session) js.Func {
	return funcOf(func(_ js.Value, args []js.Value) any {
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
	})
}

// proxyDispose builds the synchronous session.dispose(). js.FuncOf
// keeps every closure in syscall/js's handler table until Release, so
// without releasing them each disposed session, workspace bytes
// included, would stay reachable for the life of the engine. dispose
// therefore releases the session's method funcs and points each
// method on proxy, dispose included, at a shared stand-in of the same
// shape (disposedFunc), so a late call through the session object
// never reaches a released func. It releases its own func on the first
// call (js.Func.Release is safe while the func runs), so a disposed
// session leaves nothing in the handler table. A dispose reference
// taken before the first call points at a released func, like the
// other methods.
func proxyDispose(sess *mdsmith.Session, proxy js.Value, methods map[string]js.Func) js.Func {
	var self js.Func
	self = funcOf(func(_ js.Value, _ []js.Value) any {
		if sess == nil {
			return js.Undefined()
		}
		sess.Dispose()
		for name, f := range methods {
			proxy.Set(name, disposedFunc(name))
			releaseFunc(f)
		}
		proxy.Set("dispose", disposedFunc("dispose"))
		sess, proxy, methods = nil, js.Undefined(), nil
		releaseFunc(self)
		return js.Undefined()
	})
	return self
}

// Stand-ins a disposed session's methods point at. They are shared by
// every session and never released, so disposing pins nothing new.
var (
	disposedOnce       sync.Once
	disposedAsync      js.Func // Promise rejecting with "session disposed"
	disposedEmptyArray js.Func // capabilities(): []
	disposedNoop       js.Func // invalidate() and dispose(): undefined
)

// disposedAsyncReason is the message of a disposed async method's
// rejection.
const disposedAsyncReason = "session disposed"

// disposedFunc returns the stand-in for method name: capabilities()
// returns an empty list, invalidate() does nothing, and every async
// method returns a Promise that rejects with Error("session disposed").
func disposedFunc(name string) js.Func {
	disposedOnce.Do(func() {
		disposedAsync = funcOf(func(js.Value, []js.Value) any {
			return newPromise(func(_, reject func(any)) {
				reject(jsError(disposedAsyncReason))
			})
		})
		disposedEmptyArray = funcOf(func(js.Value, []js.Value) any {
			return js.ValueOf([]any{})
		})
		disposedNoop = funcOf(func(js.Value, []js.Value) any {
			return js.Undefined()
		})
	})
	switch name {
	case "capabilities":
		return disposedEmptyArray
	case "invalidate", "dispose":
		return disposedNoop
	default:
		return disposedAsync
	}
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

// allStrings reports whether every arg is a JS string, so a method that
// takes a fixed set of string parameters can validate them in one call.
func allStrings(args []js.Value) bool {
	for _, a := range args {
		if a.Type() != js.TypeString {
			return false
		}
	}
	return true
}
