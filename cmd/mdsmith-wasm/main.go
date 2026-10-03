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
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
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
// returning factory keeps the JS API ergonomic. Like every engine entry
// point it first frees the sessions JS has collected (drainFinalized).
func createSession(_ js.Value, args []js.Value) any {
	drainFinalized()
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
// the first argument, so the binding lives in JS and a session
// registers no func of its own. A token object is registered with a
// FinalizationRegistry that queues the id once the token is collected,
// for the next engine call to dispose (drainFinalized). dispose is
// bound to the token too, and every other method is a key of a WeakMap
// whose value is the token (see bindMethods), so the token is collected
// only when the session object and every method taken off it are, and
// no call but dispose passes it. The session object itself is not
// registered: a method taken off it outlives it. A host that drops a
// session without dispose() therefore still frees the Go Session, on
// the first engine call after JS collects it; dispose() stays the
// prompt, deterministic path. Plan 2610021452.
// dispose drops the id from sessions, so a call through any reference
// (a stored `const d = session.dispose`, a frozen session object, a
// read-only method) finds no session and takes the disposed path: it
// never reaches a released func, so syscall/js logs nothing. See plan
// 2610021237. The id is put in sessions last, after every method is
// bound and the token registered: a bind or register that throws (one
// patched before load) rejects createSession without leaving a Session
// no session object can dispose. A resolve that throws after this
// returns still leaves one until JS collects the dropped session
// object; plan 2610021800 tracks that.
func newSessionProxy(sess *mdsmith.Session) js.Value {
	shared := sharedMethods()
	id := newSessionID()
	proxy := js.Global().Get("Object").New()
	token := js.Global().Get("Object").New()
	bindMethods(proxy, sessionMethodNames(), shared, id, token)
	if jsType(finalizer.register) == js.TypeFunction {
		finalizer.register.Invoke(token, id, token)
	}
	sessions[id] = sess
	return proxy
}

// bindMethods sets each named method on proxy to its shared func bound
// to id, in names order rather than Go map order, so
// Object.keys(session) is the same for every session. dispose is also
// bound to token, which it unregisters; every other method is passed
// with token to finalizer.keep instead, so the token outlives each of
// them while a call carries only the id. A name with no shared func
// (the names list and sharedMethodImpls drifted) is left off rather
// than passed to bind, which would throw on every createSession;
// TestNewSessionProxy_KeysMatchSessionMethodNames reports the drift.
func bindMethods(proxy js.Value, names []string, shared map[string]js.Value, id int64, token js.Value) {
	for _, name := range names {
		f, ok := shared[name]
		switch {
		case !ok:
		case name == "dispose":
			proxy.Set(name, bindTo.Invoke(f, js.Undefined(), id, token))
		default:
			m := bindTo.Invoke(f, js.Undefined(), id)
			if jsType(finalizer.keep) == js.TypeFunction {
				finalizer.keep.Invoke(m, token)
			}
			proxy.Set(name, m)
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
// engine resolves. Nor does it cover Reflect.get: wasm_exec.js reads
// each incoming call's arguments through it, so a Reflect.get replaced
// at any time sees the bound id of each session whose method is called.
// A random id or token hides nothing from that script; it is a
// documented limit (docs/background/concepts/engine-api.md).
var bindTo js.Value

// sessionFinalizer holds the JS values that free a session dropped
// without dispose(). Every field is undefined on a host with no usable
// FinalizationRegistry (see bindFinalizer).
type sessionFinalizer struct {
	// register(token, id, token) arranges for id to be pushed onto queue
	// once token is collected; unregister(token) cancels that, the token
	// being its own unregister token. Both are bound to the one
	// FinalizationRegistry sharedMethods creates.
	register, unregister js.Value
	// keep(m, token) is WeakMap.prototype.set bound to one private
	// WeakMap: while method m is reachable, so is token.
	keep js.Value
	// queue is the private array the registry's cleanup callback, a
	// bound Array.prototype.push, appends each collected session's id
	// to. drainFinalized disposes them on the next engine call.
	queue js.Value
}

// finalizer is the one sessionFinalizer, set by sharedMethods.
var finalizer sessionFinalizer

// sessions maps a live session's id to its Session. js/wasm runs every
// goroutine on one thread with no preemption, and nothing between a
// read and a write here blocks, so it needs no lock.
var sessions = map[int64]*mdsmith.Session{}

// newSessionID hands out the next id: a keyed Feistel permutation of a
// counter, shifted to start at 1. The permutation is a bijection and
// the counter never repeats, so no id is handed out twice, live or
// disposed, and a method kept from a disposed session can never reach
// a later one. The key is drawn at load, so the ids are not a counting
// sequence: a script that holds a raw shared func cannot find a
// session by counting up from 0 or stepping from an id it knows. The
// permutation spans 2^54 values; a counter value whose image is at or
// past maxSessionID is skipped, which happens about half the time, so
// every id is in [1, maxSessionID] and the loop ends after two tries
// on average. The id is an int64, not an int, so TinyGo, whose int is
// 32 bits on wasm, gets the same range. Plan 2610021439.
func newSessionID() int64 {
	for {
		x := permuteSessionID(sessionIDCounter, sessionIDCipher, sessionIDHalfBits)
		sessionIDCounter++
		if x < maxSessionID {
			return int64(x) + 1
		}
	}
}

// sessionIDHalfBits is the width of each Feistel half: two halves of
// 27 bits span 2^54 values, the smallest even width that covers [0,
// maxSessionID).
const sessionIDHalfBits = 27

// sessionIDRounds is the number of Feistel rounds: 10, as in NIST
// SP 800-38G's FF1, which is the same AES-keyed Feistel shape. With
// AES as the round function, a script that records the ids handed out
// after it patched Reflect.apply learns nothing that predicts the ids
// handed out before: recovering the key means breaking AES-128.
const sessionIDRounds = 10

// sessionIDCipher is the per-load AES-128 key of the id permutation.
// The key comes from crypto/rand, which on js/wasm reads
// crypto.getRandomValues: standard Go calls it directly, and TinyGo's
// arc4random_buf reaches it through the random_get its wasm_exec.js
// serves. The AES core is already linked into the standard build; the
// crypto/aes wrapper, the decrypt path a cipher.Block reaches, and
// crypto/rand add about 18 KiB raw to it.
var sessionIDCipher = newSessionIDCipher()

// newSessionIDCipher draws a fresh 128-bit key and returns its AES
// block. A failed read panics through mustCipher, like a bad key.
func newSessionIDCipher() cipher.Block {
	var key [16]byte
	_, rerr := rand.Read(key[:])
	blk, err := aes.NewCipher(key[:])
	return mustCipher(blk, errors.Join(rerr, err))
}

// mustCipher returns blk, or panics on err. crypto/rand.Read never
// fails on js/wasm, and aes.NewCipher fails only on a key that is not
// 16, 24, or 32 bytes, which newSessionIDCipher never passes, so the
// panic marks a programming error at load.
func mustCipher(blk cipher.Block, err error) cipher.Block {
	if err != nil {
		panic(err)
	}
	return blk
}

// sessionIDCounter is the next counter value newSessionID permutes.
// permuteSessionID reads only its low 54 bits, so ids start to repeat
// after 2^54 counter values, which yield 2^53 ids: at one id per
// microsecond, after over 280 years.
var sessionIDCounter uint64

// permuteSessionID maps x in [0, 2^(2*half)) to a distinct value in
// the same range with a balanced Feistel network of sessionIDRounds
// rounds keyed by blk. Each round XORs one half with the round
// function of the other, which is invertible whatever that function
// returns, so the whole is a bijection.
func permuteSessionID(x uint64, blk cipher.Block, half uint) uint64 {
	mask := uint64(1)<<half - 1
	l, r := x>>half&mask, x&mask
	// Encrypt through the cipher.Block interface moves its buffer to
	// the heap, so every round shares this one block.
	var buf [aes.BlockSize]byte
	for i := range sessionIDRounds {
		l, r = r, l^(roundSessionID(blk, &buf, byte(i), r)&mask)
	}
	return l<<half | r
}

// roundSessionID is the Feistel round function: AES under blk of the
// block holding round in byte 0 and r little-endian in bytes 1 to 8,
// zero after, read back as its low 64 bits little-endian. The round
// index in the block makes each round an independent function. buf is
// the caller's scratch block, encrypted in place and overwritten.
func roundSessionID(blk cipher.Block, buf *[aes.BlockSize]byte, round byte, r uint64) uint64 {
	*buf = [aes.BlockSize]byte{round}
	binary.LittleEndian.PutUint64(buf[1:], r)
	blk.Encrypt(buf[:], buf[:])
	return binary.LittleEndian.Uint64(buf[:])
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
// entries. Each takes the session id as args[0]; dispose also takes
// its token as args[1].
func sharedMethods() map[string]js.Value {
	sharedOnce.Do(func() {
		proto := js.Global().Get("Function").Get("prototype")
		bindTo = proto.Get("call").Call("bind", proto.Get("bind"))
		sharedFuncs = make(map[string]js.Value, len(sharedMethodImpls)+1)
		for name, impl := range sharedMethodImpls {
			sharedFuncs[name] = funcOf(sharedFunc(impl)).Value
		}
		sharedFuncs["dispose"] = funcOf(proxyDispose).Value
		finalizer = bindFinalizer(js.Global().Get("FinalizationRegistry"))
	})
	return sharedFuncs
}

// bindFinalizer creates the one FinalizationRegistry from ctor and
// returns its register and unregister methods bound to it, the set
// method of a new WeakMap bound to that map, and the queue the
// registry's cleanup callback pushes onto. They are captured once, with
// bindTo, so a later patch of FinalizationRegistry, WeakMap, or Array
// never sees a token or an id.
//
// The cleanup callback is Array.prototype.push bound to queue, a native
// function, not a Go func: the garbage collector never calls into Go,
// so a session collected after the Go program has exited (a panic in a
// js.FuncOf callback ends it) raises no "Go program has already exited"
// error in a GC task, outside any caller's try. Building that guard as
// a JS try/catch wrapper would need eval or Function, which a strict
// Content Security Policy blocks. A collected session is therefore
// freed on the next engine call rather than at collection time (see
// drainFinalized). Plan 2610030846.
//
// When ctor is not a function (a host with no FinalizationRegistry),
// or building the registry or the WeakMap or binding their methods
// throws (a ctor that is not a constructor, or a stub with no register
// or unregister), every field comes back undefined: main calls this
// before it exposes the API, so a throw here would stop the engine from
// loading. dispose() is then the only way to free a session. TinyGo
// does not implement recover() on WebAssembly, so in a TinyGo build
// such a throw still ends the program.
func bindFinalizer(ctor js.Value) (f sessionFinalizer) {
	if jsType(ctor) != js.TypeFunction {
		return sessionFinalizer{}
	}
	defer func() {
		if recover() != nil {
			f = sessionFinalizer{}
		}
	}()
	g := js.Global()
	queue := g.Get("Array").New()
	registry := ctor.New(bindTo.Invoke(g.Get("Array").Get("prototype").Get("push"), queue))
	weakMap := g.Get("WeakMap")
	return sessionFinalizer{
		register:   bindTo.Invoke(registry.Get("register"), registry),
		unregister: bindTo.Invoke(registry.Get("unregister"), registry),
		keep:       bindTo.Invoke(weakMap.Get("prototype").Get("set"), weakMap.New()),
		queue:      queue,
	}
}

// sharedFunc is the body of a forwarding method's shared func: it
// calls impl.call with the live session bound as args[0] and the args
// after the bound id, and returns impl.disposed() when that
// session is disposed or args[0] is no live id, so impl.call never
// runs without a live session. It first frees the sessions JS has
// collected since the last engine call (drainFinalized).
func sharedFunc(impl methodImpl) func(js.Value, []js.Value) any {
	return func(_ js.Value, args []js.Value) any {
		drainFinalized()
		if _, sess, rest := boundSession(args); sess != nil {
			return impl.call(sess, rest)
		}
		return impl.disposed()
	}
}

// drainFinalized disposes each session whose token JS has collected
// since the last drain: the FinalizationRegistry pushed its id onto
// finalizer.queue. Every engine entry point (createSession, each session
// method, dispose) calls it first, so a dropped session is freed on the
// next call after its collection. A session is collected only once its
// object and every method taken off it are, because each method keeps
// the token alive. An id already disposed is skipped, and each entry is
// read through boundSession, so a held value that is not an integer id
// (only a FinalizationRegistry patched before load can push one)
// disposes nothing rather than panicking in Value.Float. On a host with
// no usable registry there is no queue and nothing to drain.
func drainFinalized() {
	q := finalizer.queue
	if jsType(q) != js.TypeObject {
		return
	}
	n := q.Length()
	if n == 0 {
		return
	}
	for i := 0; i < n; i++ {
		if id, sess, _ := boundSession([]js.Value{q.Index(i)}); sess != nil {
			disposeSession(id)
		}
	}
	q.Set("length", 0)
}

// disposeSession drops the session with the given id from sessions and
// disposes it; an id with no live session is a no-op.
func disposeSession(id int64) {
	if sess := sessions[id]; sess != nil {
		delete(sessions, id)
		sess.Dispose()
	}
}

// maxSessionID bounds a bound id before its int64 conversion, so
// int64(f) is in range and exact: 2^53 is the largest float64 below
// which every integer is exact. It is also the top of the range
// newSessionID hands ids out from, so every id passes boundSession's
// check.
const maxSessionID = 1 << 53

// boundSession splits the session id a shared func is bound to off args
// and looks up its live Session. sess is nil once
// that session is disposed, and also when args[0] is not an integer
// number (a string, a fraction, NaN, Infinity, or past ±maxSessionID),
// which only a direct call to a shared func (never one through a
// session object) can pass; args then comes back whole, and no
// fraction is truncated onto a live id.
func boundSession(args []js.Value) (id int64, sess *mdsmith.Session, rest []js.Value) {
	if len(args) == 0 || jsType(args[0]) != js.TypeNumber {
		return 0, nil, args
	}
	f := args[0].Float()
	// NaN fails f == Trunc(f); the maxSessionID bound rejects Infinity
	// and any value whose int64 conversion is implementation-defined.
	if f != math.Trunc(f) || math.Abs(f) > maxSessionID {
		return 0, nil, args
	}
	id = int64(f)
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
// does nothing. Like every engine entry point it first frees the
// sessions JS has collected (drainFinalized).
func proxyDispose(_ js.Value, args []js.Value) any {
	drainFinalized()
	if id, sess, rest := boundSession(args); sess != nil {
		// Cancel the registered finalizer first, so the registry drops
		// its entry with the session. A direct call with no token
		// object, or a host with no FinalizationRegistry, has nothing
		// to cancel.
		if len(rest) > 0 && jsType(rest[0]) == js.TypeObject && jsType(finalizer.unregister) == js.TypeFunction {
			finalizer.unregister.Invoke(rest[0])
		}
		disposeSession(id)
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
