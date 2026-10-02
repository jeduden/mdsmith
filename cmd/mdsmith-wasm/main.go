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
	"math"
	"runtime/debug"
	"sync"
	"syscall/js"

	"github.com/jeduden/mdsmith/internal/gctune"
	mdsmith "github.com/jeduden/mdsmith/pkg/mdsmith"
)

// funcOf and releaseFunc are js.FuncOf and js.Func.Release behind seams
// so a test can count the funcs a session's lifecycle registers (every
// Promise executor, plus the shared method funcs on first use), which
// syscall/js keeps private.
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
	// Capture Function.prototype.bind before the API is reachable, so a
	// later patch of bind or call never sees the unbound shared funcs.
	// A later patch of Reflect.apply still does (see bindTo).
	sharedMethods()
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
		defer rejectOnJSError(reject)
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
			if jsType(cy) != js.TypeString {
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
		if jsType(val) == js.TypeString {
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
	if jsType(v) != js.TypeObject {
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
//
// The method funcs are shared by every session and registered once.
// Each session gets a Function.prototype.bind of them with its id as
// the first argument, so the binding lives in JS and is collected with
// the session object, and a session registers no func of its own. The
// Go Session stays in sessions until dispose, even once the session
// object is collected.
// dispose drops the id from sessions, so a call through any reference
// (a stored `const d = session.dispose`, a frozen session object, a
// read-only method) finds no session and takes the disposed path: it
// never reaches a released func, so syscall/js logs nothing. See plan
// 2610021237.
func newSessionProxy(sess *mdsmith.Session) js.Value {
	shared := sharedMethods()
	id := nextSessionID
	nextSessionID++
	sessions[id] = sess
	proxy := js.Global().Get("Object").New()
	bindMethods(proxy, sessionMethodNames(), shared, id)
	return proxy
}

// bindMethods sets each named method on proxy to its shared func bound
// to id, in names order rather than Go map order, so Object.keys(session)
// is the same for every session. A name with no shared func (the names
// list and sharedMethodImpls drifted) is left off rather than passed to
// bind, which would throw on every createSession;
// TestNewSessionProxy_KeysMatchSessionMethodNames reports the drift.
func bindMethods(proxy js.Value, names []string, shared map[string]js.Value, id int) {
	for _, name := range names {
		if f, ok := shared[name]; ok {
			proxy.Set(name, bindTo.Invoke(f, js.Undefined(), id))
		}
	}
}

// bindTo is Function.prototype.call.bind(Function.prototype.bind), as
// captured by sharedMethods: bindTo(f, this, ...args) is f.bind(this,
// ...args) with no property lookup at call time. newSessionProxy binds
// through it, so a bind or call that another script installs after the
// engine loads never receives a raw shared func, which would accept any
// session id. It does not cover Reflect.apply: wasm_exec.js looks that
// up on every Go-to-JS call, so a Reflect.apply replaced at any time
// sees each raw shared func and id here, and every session object the
// engine resolves. Plan 2610021439 tracks that gap.
var bindTo js.Value

// sessions maps a live session's id to its Session. js/wasm runs every
// goroutine on one thread with no preemption, and nothing between a
// read and a write here blocks, so it needs no lock.
var (
	sessions      = map[int]*mdsmith.Session{}
	nextSessionID int
)

// sharedMethodImpls maps each forwarding session method to its
// implementation. The shared func only calls it for a live session, so
// sess is never nil; dispose is registered on its own (proxyDispose)
// because it alone needs the id, to drop it from sessions.
var sharedMethodImpls = map[string]func(sess *mdsmith.Session, args []js.Value) any{
	"check":        proxyCheck,
	"fix":          proxyFix,
	"kinds":        proxyKinds,
	"rename":       proxyRename,
	"move":         proxyMove,
	"capabilities": proxyCapabilities,
	"invalidate":   proxyInvalidate,
}

var (
	sharedOnce  sync.Once
	sharedFuncs map[string]js.Value
)

// sharedMethods captures bindTo and registers the shared method funcs on
// first use. main calls it before exposing the API; newSessionProxy
// calls it too, for the tests, which never run main. The funcs are
// never released, so every session reuses the same handler-table
// entries. Each takes the session id as args[0].
func sharedMethods() map[string]js.Value {
	sharedOnce.Do(func() {
		proto := js.Global().Get("Function").Get("prototype")
		bindTo = proto.Get("call").Call("bind", proto.Get("bind"))
		sharedFuncs = make(map[string]js.Value, len(sharedMethodImpls)+1)
		for name, impl := range sharedMethodImpls {
			sharedFuncs[name] = funcOf(func(_ js.Value, args []js.Value) any {
				if _, sess, rest := boundSession(args); sess != nil {
					return impl(sess, rest)
				}
				return disposedResult(name)
			}).Value
		}
		sharedFuncs["dispose"] = funcOf(proxyDispose).Value
	})
	return sharedFuncs
}

// maxSessionID bounds a bound id before its int conversion: 2^53 under
// standard Go, whose int is 64 bits, and math.MaxInt under TinyGo,
// whose int is 32 bits on wasm. Either way int(f) is in range.
const maxSessionID = min(1<<53, math.MaxInt)

// boundSession splits the session id a shared func is bound to off
// args and looks up its live Session. sess is nil once that session is
// disposed, and also when args[0] is not an integer number (a string,
// a fraction, NaN, Infinity, or past ±maxSessionID), which only a
// direct call to a shared func (never one through a session object)
// can pass; args then comes back whole, and no fraction is truncated
// onto a live id.
func boundSession(args []js.Value) (id int, sess *mdsmith.Session, rest []js.Value) {
	if len(args) == 0 || jsType(args[0]) != js.TypeNumber {
		return 0, nil, args
	}
	f := args[0].Float()
	// NaN fails f == Trunc(f); the maxSessionID bound rejects Infinity
	// and any value whose int conversion is implementation-defined.
	if f != math.Trunc(f) || math.Abs(f) > maxSessionID {
		return 0, nil, args
	}
	id = int(f)
	return id, sessions[id], args[1:]
}

// proxyCheck is session.check(uri, src) → Promise<Diagnostic[]>.
func proxyCheck(sess *mdsmith.Session, args []js.Value) any {
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
}

// proxyFix is session.fix(uri, src) → Promise<FixResult>.
func proxyFix(sess *mdsmith.Session, args []js.Value) any {
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
}

// proxyKinds is session.kinds(uri) → Promise<KindsResult>.
func proxyKinds(sess *mdsmith.Session, args []js.Value) any {
	return newPromise(func(resolve, reject func(any)) {
		if len(args) < 1 || jsType(args[0]) != js.TypeString {
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
}

// proxyRename is session.rename(uri, source, as, old, new) →
// Promise<Plan>; as may be "".
func proxyRename(sess *mdsmith.Session, args []js.Value) any {
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
}

// proxyMove is session.move(src, dst) → Promise<Plan>.
func proxyMove(sess *mdsmith.Session, args []js.Value) any {
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
}

// proxyCapabilities is the synchronous session.capabilities() →
// string[].
func proxyCapabilities(sess *mdsmith.Session, _ []js.Value) any {
	caps := sess.Capabilities()
	arr := make([]any, len(caps))
	for i, c := range caps {
		arr[i] = c
	}
	return js.ValueOf(arr)
}

// proxyInvalidate is the synchronous session.invalidate(uri, src?).
// A non-string uri is ignored; a string src replaces the cached source.
func proxyInvalidate(sess *mdsmith.Session, args []js.Value) any {
	if len(args) < 1 || jsType(args[0]) != js.TypeString {
		return js.Undefined()
	}
	uri := args[0].String()
	if len(args) >= 2 && jsType(args[1]) == js.TypeString {
		sess.Invalidate(uri, []byte(args[1].String()))
	} else {
		sess.Invalidate(uri)
	}
	return js.Undefined()
}

// proxyDispose is the shared func behind the synchronous
// session.dispose(). For a live session it drops the id from sessions
// and disposes the Go session, so the Session and its workspace are
// unreachable from Go. The session registered no func of its own, so
// there is nothing to release. A second call finds no session and
// does nothing.
func proxyDispose(_ js.Value, args []js.Value) any {
	if id, sess, _ := boundSession(args); sess != nil {
		delete(sessions, id)
		sess.Dispose()
	}
	return js.Undefined()
}

// disposedAsyncReason is the message of a disposed async method's
// rejection.
const disposedAsyncReason = "session disposed"

// disposedResult is what forwarding method name returns once its
// session is disposed: capabilities() returns an empty list,
// invalidate() does nothing, and every async method returns a Promise
// that rejects with Error("session disposed"). A disposed dispose()
// is handled by proxyDispose.
func disposedResult(name string) any {
	switch name {
	case "capabilities":
		return js.ValueOf([]any{})
	case "invalidate":
		return js.Undefined()
	default:
		return newPromise(func(_, reject func(any)) {
			reject(jsError(disposedAsyncReason))
		})
	}
}

// uriAndSource pulls a (uri string, source []byte) pair from JS args.
// A JS string source crosses as Go []byte while the URI stays a
// string, matching the design contract.
func uriAndSource(args []js.Value) (string, []byte, bool) {
	if len(args) < 2 || jsType(args[0]) != js.TypeString || jsType(args[1]) != js.TypeString {
		return "", nil, false
	}
	return args[0].String(), []byte(args[1].String()), true
}

// allStrings reports whether every arg is a JS string, so a method that
// takes a fixed set of string parameters can validate them in one call.
func allStrings(args []js.Value) bool {
	for _, a := range args {
		if jsType(a) != js.TypeString {
			return false
		}
	}
	return true
}
