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
	"errors"
	"math"
	"math/rand/v2"
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
	id := newSessionID()
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
// engine resolves. A random id or token hides nothing from that
// script; it is a documented limit (docs/background/concepts/engine-api.md).
var bindTo js.Value

// sessions maps a live session's id to its Session. js/wasm runs every
// goroutine on one thread with no preemption, and nothing between a
// read and a write here blocks, so it needs no lock.
var sessions = map[int]*mdsmith.Session{}

// newSessionID draws an unused id uniformly from [1, maxSessionID], so
// a script that holds a raw shared func cannot reach a session by
// counting up from 0. The range is 2^53 under standard Go and
// math.MaxInt under TinyGo; a collision with a live id redraws. The
// ids are not cryptographic: math/rand/v2's global source is seeded
// from the OS, which on js/wasm is crypto.getRandomValues. Plan
// 2610021439.
func newSessionID() int {
	for {
		id := 1 + rand.IntN(maxSessionID)
		if _, taken := sessions[id]; !taken {
			return id
		}
	}
}

// methodImpl pairs a forwarding session method's implementation with
// the result it returns once its session is disposed. Build one with
// asyncMethod, stringListMethod, or voidMethod, which fix both funcs
// from the method's result shape, so the two cannot disagree and
// neither is nil. Each constructor panics on a nil fn, and methodTable
// on an entry with a nil func, so a bad entry fails when the table is
// built at package init, not on its first call.
type methodImpl struct {
	// call runs the method for a live session; sess is never nil.
	call func(sess *mdsmith.Session, args []js.Value) js.Value
	// disposed builds the method's result after dispose(). It runs per
	// call because a Promise or array must be fresh each time.
	disposed func() js.Value
}

// asyncMethod builds the entry for a method that returns a Promise.
// fn runs inside the Promise executor; a non-nil error rejects with
// Error(err.Error()) — carrying mdsmith.ErrorCode(err) as its `code`
// when the error has one — otherwise the Promise resolves to toJS(value). A
// JS exception raised on the way (a js.Error panic) rejects with that
// exception, as newPromise does for every executor. After dispose the
// Promise rejects with Error("session disposed").
func asyncMethod(fn func(sess *mdsmith.Session, args []js.Value) (any, error)) methodImpl {
	if fn == nil {
		panic("asyncMethod: nil fn")
	}
	return methodImpl{
		call: func(sess *mdsmith.Session, args []js.Value) js.Value {
			return newPromise(func(resolve, reject func(any)) {
				v, err := fn(sess, args)
				if err != nil {
					reject(jsErrorFor(err))
					return
				}
				resolve(toJS(v))
			})
		},
		disposed: disposedReject,
	}
}

// stringListMethod builds the entry for a synchronous method that
// returns string[]. After dispose it returns an empty array.
func stringListMethod(fn func(sess *mdsmith.Session, args []js.Value) []string) methodImpl {
	if fn == nil {
		panic("stringListMethod: nil fn")
	}
	return methodImpl{
		call: func(sess *mdsmith.Session, args []js.Value) js.Value {
			list := fn(sess, args)
			arr := make([]any, len(list))
			for i, s := range list {
				arr[i] = s
			}
			return js.ValueOf(arr)
		},
		disposed: disposedEmptyList,
	}
}

// voidMethod builds the entry for a synchronous method that returns
// undefined. After dispose it returns undefined without running fn.
func voidMethod(fn func(sess *mdsmith.Session, args []js.Value)) methodImpl {
	if fn == nil {
		panic("voidMethod: nil fn")
	}
	return methodImpl{
		call: func(sess *mdsmith.Session, args []js.Value) js.Value {
			fn(sess, args)
			return js.Undefined()
		},
		disposed: disposedUndefined,
	}
}

// sharedMethodImpls maps each forwarding session method to its
// implementation and disposed result. The shared func calls impl.call
// only for a live session and impl.disposed otherwise; dispose is
// registered on its own (proxyDispose) because it alone needs the id,
// to drop it from sessions.
var sharedMethodImpls = methodTable(map[string]methodImpl{
	"check":        asyncMethod(proxyCheck),
	"fix":          asyncMethod(proxyFix),
	"kinds":        asyncMethod(proxyKinds),
	"rename":       asyncMethod(proxyRename),
	"move":         asyncMethod(proxyMove),
	"capabilities": stringListMethod(proxyCapabilities),
	"invalidate":   voidMethod(proxyInvalidate),
})

// methodTable returns m after checking that every entry carries both a
// call and a disposed func, panicking with the method's name if not.
// The constructors never build such an entry, but a hand-written
// methodImpl literal can, and its nil func would otherwise panic inside
// a js.FuncOf callback on first use; checked here, it fails at package
// init instead.
func methodTable(m map[string]methodImpl) map[string]methodImpl {
	for name, impl := range m {
		if impl.call == nil {
			panic("methodTable: " + name + " has no call func")
		}
		if impl.disposed == nil {
			panic("methodTable: " + name + " has no disposed func")
		}
	}
	return m
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
			sharedFuncs[name] = funcOf(sharedFunc(impl)).Value
		}
		sharedFuncs["dispose"] = funcOf(proxyDispose).Value
	})
	return sharedFuncs
}

// sharedFunc is the body of a forwarding method's shared func: it
// calls impl.call with the live session bound as args[0] and the
// remaining args, and returns impl.disposed() when that session is
// disposed or args[0] is no live id, so impl.call never runs without a
// live session.
func sharedFunc(impl methodImpl) func(js.Value, []js.Value) any {
	return func(_ js.Value, args []js.Value) any {
		if _, sess, rest := boundSession(args); sess != nil {
			return impl.call(sess, rest)
		}
		return impl.disposed()
	}
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

// The async proxies reject bad arguments with these errors, built once
// rather than on every bad call.
var (
	errCheckArgs  = errors.New("check(uri, src) requires two string arguments")
	errFixArgs    = errors.New("fix(uri, src) requires two string arguments")
	errKindsArgs  = errors.New("kinds(uri) requires a string argument")
	errRenameArgs = errors.New("rename(uri, source, as, old, new) requires five string arguments")
	errMoveArgs   = errors.New("move(src, dst) requires two string arguments")
)

// proxyCheck is session.check(uri, src) → Promise<Diagnostic[]>.
func proxyCheck(sess *mdsmith.Session, args []js.Value) (any, error) {
	uri, src, ok := uriAndSource(args)
	if !ok {
		return nil, errCheckArgs
	}
	diags, err := sess.Check(uri, src)
	if err != nil {
		return nil, err
	}
	// A nil Go slice marshals to JSON null, but the check() contract
	// is Diagnostic[]; normalise a clean file to [].
	if diags == nil {
		diags = []mdsmith.Diagnostic{}
	}
	return diags, nil
}

// proxyFix is session.fix(uri, src) → Promise<FixResult>.
func proxyFix(sess *mdsmith.Session, args []js.Value) (any, error) {
	uri, src, ok := uriAndSource(args)
	if !ok {
		return nil, errFixArgs
	}
	res, err := sess.Fix(uri, src)
	if err != nil {
		return nil, err
	}
	// Same nil-slice→null guard for the result's diagnostics.
	if res.Diagnostics == nil {
		res.Diagnostics = []mdsmith.Diagnostic{}
	}
	return res, nil
}

// proxyKinds is session.kinds(uri) → Promise<KindsResult>.
func proxyKinds(sess *mdsmith.Session, args []js.Value) (any, error) {
	if len(args) < 1 || jsType(args[0]) != js.TypeString {
		return nil, errKindsArgs
	}
	return sess.Kinds(args[0].String())
}

// proxyRename is session.rename(uri, source, as, old, new) →
// Promise<Plan>; as may be "".
func proxyRename(sess *mdsmith.Session, args []js.Value) (any, error) {
	if len(args) < 5 || !allStrings(args[:5]) {
		return nil, errRenameArgs
	}
	return sess.Rename(args[0].String(), []byte(args[1].String()),
		args[2].String(), args[3].String(), args[4].String())
}

// proxyMove is session.move(src, dst) → Promise<Plan>.
func proxyMove(sess *mdsmith.Session, args []js.Value) (any, error) {
	if len(args) < 2 || !allStrings(args[:2]) {
		return nil, errMoveArgs
	}
	return sess.Move(args[0].String(), args[1].String())
}

// proxyCapabilities is the synchronous session.capabilities() →
// string[].
func proxyCapabilities(sess *mdsmith.Session, _ []js.Value) []string {
	return sess.Capabilities()
}

// proxyInvalidate is the synchronous session.invalidate(uri, src?).
// A non-string uri is ignored; a string src replaces the cached source.
func proxyInvalidate(sess *mdsmith.Session, args []js.Value) {
	if len(args) < 1 || jsType(args[0]) != js.TypeString {
		return
	}
	uri := args[0].String()
	if len(args) >= 2 && jsType(args[1]) == js.TypeString {
		sess.Invalidate(uri, []byte(args[1].String()))
	} else {
		sess.Invalidate(uri)
	}
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

// disposedReject is the disposed result of an async method: a Promise
// that rejects with Error("session disposed"). A disposed dispose() is
// handled by proxyDispose.
func disposedReject() js.Value {
	return newPromise(func(_, reject func(any)) {
		reject(jsError(disposedAsyncReason))
	})
}

// disposedEmptyList is capabilities() after dispose: an empty array.
func disposedEmptyList() js.Value { return js.ValueOf([]any{}) }

// disposedUndefined is the disposed result of a method that does
// nothing, such as invalidate().
func disposedUndefined() js.Value { return js.Undefined() }

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
