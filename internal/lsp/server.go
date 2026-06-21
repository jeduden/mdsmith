package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/index"
	vlog "github.com/jeduden/mdsmith/internal/log"
	"github.com/jeduden/mdsmith/internal/rule"
	mdsmith "github.com/jeduden/mdsmith/pkg/mdsmith"
)

// Server runs the LSP loop over a transport pair. One Server instance
// serves one client.
type Server struct {
	t              *transport
	rules          []rule.Rule
	debounce       time.Duration
	fetchTimeout   time.Duration
	discoverConfig func(string) (string, error)
	onConfigReload func(cfgPath string)
	logger         *vlog.Logger
	docs           *documentStore

	configMu   sync.RWMutex
	config     *config.Config
	configPath string
	rootDir    string

	settingsMu sync.RWMutex
	settings   userSettings

	clientCapsMu sync.RWMutex
	clientCaps   clientCapabilities

	pendingMu     sync.Mutex
	pending       map[string]*pendingLint
	pendingRespMu sync.Mutex
	pendingResp   map[string]chan rpcResponse

	// idx is the lazy workspace symbol index. It is populated on
	// first symbol-navigation request and kept in sync via
	// document events and watcher notifications. nil until
	// ensureIndex builds it.
	idxMu sync.Mutex
	idx   *index.Index

	// diagsMu guards diags, the per-URI cache of the last published
	// LSP diagnostics. Hover uses this to answer diagnostic-first
	// requests without re-running lint.
	diagsMu sync.RWMutex
	diags   map[string][]Diagnostic

	nextReqID        atomic.Int64
	shutdown         atomic.Bool // we are tearing down (any cause)
	shutdownReceived atomic.Bool // client sent a `shutdown` request
	exitRequested    atomic.Bool // client sent an `exit` notification
	// previewFallbackLogged ensures the capability-fallback warning is
	// emitted via window/logMessage and s.logger at most once per session.
	previewFallbackLogged atomic.Bool

	// Parent-process watchdog (LSP §3.16 InitializeParams.processId).
	// runCtx is the Run() context; the watchdog stops with it on a
	// normal shutdown. parentAlive / onParentExit / parentInterval are
	// test seams — production uses processAlive, os.Exit(0), and
	// parentPollInterval. parentWatchOnce guards against a repeated
	// initialize starting a second watcher.
	runCtx          context.Context
	parentAlive     func(int) bool
	onParentExit    func()
	parentInterval  time.Duration
	parentWatchOnce sync.Once

	// Workspace singleton (newest-wins). When EnableWorkspaceSingleton
	// is set, handleInitialize claims the workspace root in a shared
	// registry under instanceID and starts a watcher that steps this
	// server aside — notifying the editor via mdsmith/superseded, then
	// exiting — once a newer server claims the same workspace. This
	// reaps an orphaned server kept alive by a leaked editor host: the
	// case the processId watchdog can't see, because that host stays
	// alive. instanceID is "" when the feature is off, which makes
	// startSingletonWatch a no-op. singletonClaim / singletonCurrent /
	// singletonInterval / onSupersededExit are test seams — production
	// uses a file registry, singletonPollInterval, and os.Exit(0).
	instanceID         string
	singletonClaim     func(key, id string) error
	singletonCurrent   func(key string) string
	singletonInterval  time.Duration
	onSupersededExit   func()
	singletonWatchOnce sync.Once

	// session is the per-workspace pkg/mdsmith.Session every lint and
	// fix path routes through (plan 219). It owns the cross-file read
	// cache and the version-keyed parse cache the latency gate depends
	// on, and an OverlayWorkspace whose open-buffer overlay lets the
	// editor's unsaved bytes reach cross-file rules. reloadConfig
	// rebuilds it when the compiled config changes (config is compiled
	// once per session); sessionMu guards the rebuild against concurrent
	// lint/fix readers. workspace is the session's overlay, held so
	// document events can Set/Delete buffers on it.
	//
	// didChange/didSave/didClose/didChangeWatchedFiles push buffer edits
	// and drop stale cache entries through session.Invalidate; a
	// watched create/delete drops the wikilink index through
	// session.InvalidateWikilinks. didOpen needs no drop — the version
	// starts fresh.
	sessionMu sync.RWMutex
	session   *mdsmith.Session
	workspace *mdsmith.OverlayWorkspace
	// newSession constructs the per-workspace Session. It is a test seam
	// — production uses mdsmith.NewSession. NewSession only fails when its
	// ConfigSource fails to load, and rebuildSession always passes a
	// compiled (already-loaded) source, so the error path is unreachable
	// in production; the seam lets a test drive rebuildSession's failure
	// branch (and the nil-session guards downstream of it) red/green.
	newSession func(mdsmith.SessionOptions) (*mdsmith.Session, error)
	// afterLintCheck, when non-nil, runs in runLint immediately after the
	// session Check returns and before the results are published. It is a
	// test seam (nil in production) that lets a test deterministically
	// simulate a didClose landing mid-lint — the race the "document was
	// closed while we were linting" guard protects against.
	afterLintCheck func()
	// lintPanicHook, when non-nil, is called inside runLint just before
	// the real sess.CheckVersion call. Tests set it to panic so the
	// deferred recover path can be driven red/green without a real rule
	// that panics. Nil in production.
	lintPanicHook func()
	// dispatchPanicHook, when non-nil, is called by dispatchRaw after
	// the frame is parsed and before dispatch, so a test can trigger a
	// handler-stage panic inside the dispatch path. One-shot: the hook
	// disables itself after firing so subsequent messages route
	// normally. Nil in production.
	dispatchPanicHook func()
}

// userSettings mirrors the subset of `mdsmith.*` VS Code keys the
// server consults. Defaults match the documented values in
// docs/guides/editors/vscode.md.
type userSettings struct {
	ConfigPath string `json:"config"`
	Run        string `json:"run"`
	PreviewFix bool   `json:"previewFix"`
}

// clientSettings is the JSON shape we accept from
// workspace/configuration. Pointer fields distinguish "client
// supplied an explicit value" (including empty string) from
// "client did not supply a value at all" (returns null per
// LSP §5.6, which Unmarshal turns into nil). Without this
// distinction we could never let the user clear a previously-set
// `mdsmith.config` back to the empty default; the cached
// non-empty value would stick across configuration changes.
type clientSettings struct {
	ConfigPath *string `json:"config"`
	Run        *string `json:"run"`
	PreviewFix *bool   `json:"previewFix"`
}

// runMode enumerates valid `mdsmith.run` values. Anything else is
// treated as the documented default.
const (
	runOnSave = "onSave"
	runOnType = "onType"
	runOff    = "off"
)

// rpcResponse is what dispatch hands to a waiting requester.
type rpcResponse struct {
	Result json.RawMessage
	Error  *responseError
}

// Options configures a new Server.
type Options struct {
	// Rules is the registered rule set. Pass rule.All() in production.
	Rules []rule.Rule
	// Reader is the LSP input stream (typically stdin).
	Reader io.Reader
	// Writer is the LSP output stream (typically stdout).
	Writer io.Writer
	// Debounce is the per-document quiet period before re-linting.
	// Zero defers to the default (200 ms). Negative disables debouncing.
	Debounce time.Duration
	// Logger receives server-side trace messages. May be nil.
	Logger *vlog.Logger
	// OnConfigReload, if non-nil, is invoked when the resolved config
	// path changes — the initial load that picks up a config, and any
	// later reload (didChangeConfiguration or watched-file event) whose
	// resolved path differs from the previously cached one. A no-op
	// reload (same path) does NOT fire the hook, so the host can install
	// a closure that captures the current cfgPath without paying for
	// reinstall on every settings refresh.
	//
	// cfgPath is the empty string when no config was successfully
	// loaded. Used by cmd/mdsmith to keep the include-extract projector
	// pointing at the active config so `<?include extract:?>` directives
	// produce the same diagnostics in the editor as `mdsmith check`
	// does on the CLI.
	OnConfigReload func(cfgPath string)
	// EnableWorkspaceSingleton turns on the newest-wins workspace
	// singleton. When two servers run for the same workspace root — a
	// leaked editor host left one orphaned and a reload spawned a fresh
	// one — the older steps aside so exactly one stays live. cmd/mdsmith
	// enables it; unit tests leave it off so they neither write to the
	// real cache dir nor leak a watcher goroutine (the dedicated
	// singleton tests drive the seams directly).
	EnableWorkspaceSingleton bool
}

// New constructs a Server. The Server does not run until Run() is
// called.
func New(opts Options) *Server {
	debounce := opts.Debounce
	if debounce == 0 {
		debounce = 200 * time.Millisecond
	}
	if debounce < 0 {
		debounce = 0
	}
	logger := opts.Logger
	if logger == nil {
		logger = &vlog.Logger{}
	}
	s := &Server{
		t:              newTransport(opts.Reader, opts.Writer),
		rules:          opts.Rules,
		debounce:       debounce,
		fetchTimeout:   2 * time.Second,
		discoverConfig: config.Discover,
		onConfigReload: opts.OnConfigReload,
		logger:         logger,
		docs:           newDocumentStore(),
		settings:       userSettings{Run: runOnType},
		pending:        make(map[string]*pendingLint),
		pendingResp:    make(map[string]chan rpcResponse),
		diags:          make(map[string][]Diagnostic),
		// Parent-process watchdog defaults; Run() overwrites runCtx
		// with its own context. Tests override these seams.
		runCtx:         context.Background(),
		parentAlive:    processAlive,
		onParentExit:   func() { osExit(0) },
		parentInterval: parentPollInterval,
		// Workspace-singleton defaults; the registry seams are wired
		// only when the feature is enabled so unit tests stay hermetic.
		// onParentExit and onSupersededExit share the osExit seam
		// (singleton.go) so both default closures are unit-testable.
		singletonInterval: singletonPollInterval,
		onSupersededExit:  func() { osExit(0) },
		// Production session constructor; tests override to exercise the
		// rebuild-failure branch.
		newSession: mdsmith.NewSession,
	}
	if opts.EnableWorkspaceSingleton {
		s.instanceID = newInstanceID()
		reg := defaultRegistry()
		s.singletonClaim = reg.claim
		s.singletonCurrent = reg.current
	}
	return s
}

// Run drives the server until the input stream returns io.EOF, the
// client sends `exit`, the supplied context is canceled, or a
// transport-level write fails (typically EPIPE when the client drops
// its stdout pipe).
//
// On any exit path Run sets the shutdown flag and cancels every
// pending debounce timer so a callback armed milliseconds before
// teardown does not race the parent goroutine and write
// publishDiagnostics into a half-closed pipe.
func (s *Server) Run(ctx context.Context) error {
	// Record the run context so the parent-process watchdog started in
	// handleInitialize stops when the server shuts down normally.
	s.runCtx = ctx
	defer func() {
		s.shutdown.Store(true)
		s.stopPendingLints()
	}()
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := s.t.WriteError(); err != nil {
			return err
		}
		raw, err := s.t.readRaw()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		s.dispatchRaw(ctx, raw)
		if err := s.t.WriteError(); err != nil {
			return err
		}
		if s.exitRequested.Load() {
			// LSP §3.16: receiving `exit` without a prior
			// successful `shutdown` request is an abnormal
			// termination — return an error so the CLI exits
			// non-zero. A clean shutdown→exit pair returns nil.
			if !s.shutdownReceived.Load() {
				return errExitWithoutShutdown
			}
			return nil
		}
	}
}

// errExitWithoutShutdown is returned from Run when the client
// sends an `exit` notification before a successful `shutdown`
// request, per the LSP lifecycle spec.
var errExitWithoutShutdown = errors.New("lsp: exit notification received before shutdown request")

// dispatchRaw routes one frame to either request/notification handling
// or response handling based on the message shape.
//
// JSON-RPC distinguishes the two by the presence of `method` (request
// or notification) versus `result`/`error` (response to a server-side
// request). Treating responses as unknown methods would break reply
// flow for `workspace/configuration`, `client/registerCapability`,
// and any future server-initiated request.
//
// A deferred recover wraps the entire body so a panic inside a message
// handler does not kill the dispatch loop. The panic is logged with
// its stack and the offending frame is dropped; when the frame was a
// request, an InternalError response settles the client's pending
// call. The next frame is processed normally.
func (s *Server) dispatchRaw(ctx context.Context, raw []byte) {
	var reqID json.RawMessage
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		s.logPanic("dispatch", r)
		// JSON-RPC 2.0 §5: every call gets a response. Without one
		// the client's promise for this ID pends forever — the
		// crash would become a hang.
		if len(reqID) > 0 {
			_ = s.t.writeError(reqID, codeInternalError,
				"internal error: panic while handling request")
		}
	}()
	var probe struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id,omitempty"`
		Method  string          `json:"method,omitempty"`
		Result  json.RawMessage `json:"result,omitempty"`
		Error   *responseError  `json:"error,omitempty"`
		Params  json.RawMessage `json:"params,omitempty"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		// JSON-RPC 2.0 §5.1: unparseable input gets a parse error
		// response with id: null. Without this, a client that sent
		// a request with a malformed body would hang waiting for a
		// reply we silently dropped.
		_ = s.t.writeError(json.RawMessage("null"), codeParseError, "parse error")
		return
	}
	if probe.JSONRPC != "2.0" {
		if probe.ID != nil {
			_ = s.t.writeError(probe.ID, codeInvalidRequest, "jsonrpc must be 2.0")
		}
		return
	}
	// Response: has id, no method, and exactly one of result/error
	// present. JSON-RPC 2.0 §5: a frame missing both result and
	// error is an invalid request, not a response — deliverResponse
	// would otherwise silently consume it (or worse, fire a stale
	// pending channel) instead of telling the client they sent
	// garbage.
	if probe.Method == "" && len(probe.ID) > 0 {
		if probe.Result != nil || probe.Error != nil {
			s.deliverResponse(string(probe.ID), rpcResponse{Result: probe.Result, Error: probe.Error})
			return
		}
		_ = s.t.writeError(probe.ID, codeInvalidRequest, "missing method, result, and error")
		return
	}
	msg := &requestMessage{
		JSONRPC: probe.JSONRPC, ID: probe.ID, Method: probe.Method, Params: probe.Params,
	}
	reqID = msg.ID
	// dispatchPanicHook is a test seam (nil in production). It fires
	// after the frame is parsed and before dispatch, so a test can
	// inject a handler-stage panic and assert the deferred recover
	// both survives it and answers the in-flight request.
	if s.dispatchPanicHook != nil {
		s.dispatchPanicHook()
	}
	s.dispatch(ctx, msg)
}

// recoverPanic is the deferred half of panic containment. It must be
// deferred directly (`defer s.recoverPanic("scope")`) so its
// recover() call observes the goroutine's panic.
func (s *Server) recoverPanic(scope string) {
	if r := recover(); r != nil {
		s.logPanic(scope, r)
	}
}

// logPanic records a recovered panic with its stack on the server log
// and forwards the same text as a window/logMessage error so the
// signal reaches the editor's output channel even when verbose
// logging is off — the production default: `mdsmith lsp` wires no
// Logger, so s.logger alone would swallow the only evidence of the
// contained crash.
func (s *Server) logPanic(scope string, r any) {
	stack := debug.Stack()
	s.logger.Printf("%s: recovered panic: %v\n%s", scope, r, stack)
	_ = s.t.writeNotification("window/logMessage", logMessageParams{
		Type: messageTypeError,
		Message: fmt.Sprintf("mdsmith: recovered panic in %s: %v\n%s",
			scope, r, stack),
	})
}

func (s *Server) dispatch(ctx context.Context, msg *requestMessage) {
	// LSP §3.16 (lifecycle): once `shutdown` has succeeded, the
	// server must reject any subsequent request other than `exit`
	// with InvalidRequest. Notifications are silently dropped.
	if s.shutdown.Load() && msg.Method != "exit" {
		if msg.ID != nil {
			_ = s.t.writeError(msg.ID, codeInvalidRequest, "server is shutting down")
		}
		return
	}
	if s.dispatchLifecycle(ctx, msg) {
		return
	}
	if s.dispatchDocument(ctx, msg) {
		return
	}
	if s.dispatchNavigation(msg) {
		return
	}
	if s.dispatchWorkspace(ctx, msg) {
		return
	}
	switch msg.Method {
	case "$/cancelRequest", "$/setTrace", "$/progress":
		// Notifications we silently accept.
	default:
		// Notifications (no ID) are silently ignored per the LSP
		// spec; only requests get a method-not-found error.
		if msg.ID != nil {
			_ = s.t.writeError(msg.ID, codeMethodNotFound, "method not supported: "+msg.Method)
		}
	}
}

// dispatchLifecycle handles the LSP lifecycle methods. Returns true
// when the message was handled.
func (s *Server) dispatchLifecycle(ctx context.Context, msg *requestMessage) bool {
	switch msg.Method {
	case "initialize":
		s.handleInitialize(msg)
	case "initialized":
		s.handleInitialized(ctx)
	case "shutdown":
		s.shutdown.Store(true)
		s.shutdownReceived.Store(true)
		s.stopPendingLints()
		_ = s.t.writeResponse(msg.ID, nil)
	case "exit":
		s.shutdown.Store(true)
		s.exitRequested.Store(true)
		s.stopPendingLints()
	default:
		return false
	}
	return true
}

// dispatchDocument handles textDocument/* sync and the codeAction
// surface that's tied to it.
func (s *Server) dispatchDocument(ctx context.Context, msg *requestMessage) bool {
	switch msg.Method {
	case "textDocument/didOpen":
		s.handleDidOpen(ctx, msg.Params)
	case "textDocument/didChange":
		s.handleDidChange(ctx, msg.Params)
	case "textDocument/didSave":
		s.handleDidSave(ctx, msg.Params)
	case "textDocument/didClose":
		s.handleDidClose(msg.Params)
	case "textDocument/codeAction":
		s.handleCodeAction(msg)
	case "textDocument/hover":
		s.handleHover(msg)
	default:
		return false
	}
	return true
}

// dispatchNavigation handles the symbol-navigation surface added in
// plan 131: documentSymbol, definition, implementation, references,
// workspace/symbol, and the call-hierarchy trio. Plan 134 adds completion.
func (s *Server) dispatchNavigation(msg *requestMessage) bool {
	switch msg.Method {
	case "textDocument/documentSymbol":
		s.handleDocumentSymbol(msg)
	case "textDocument/definition":
		s.handleDefinition(msg)
	case "textDocument/implementation":
		s.handleImplementation(msg)
	case "textDocument/references":
		s.handleReferences(msg)
	case "workspace/symbol":
		s.handleWorkspaceSymbol(msg)
	case "textDocument/prepareCallHierarchy":
		s.handlePrepareCallHierarchy(msg)
	case "callHierarchy/incomingCalls":
		s.handleIncomingCalls(msg)
	case "callHierarchy/outgoingCalls":
		s.handleOutgoingCalls(msg)
	case "textDocument/completion":
		s.handleCompletion(msg)
	case "textDocument/prepareRename":
		s.handlePrepareRename(msg)
	case "textDocument/rename":
		s.handleRename(msg)
	default:
		return false
	}
	return true
}

// dispatchWorkspace handles the workspace/* events that don't fit
// the navigation grouping.
func (s *Server) dispatchWorkspace(ctx context.Context, msg *requestMessage) bool {
	switch msg.Method {
	case "workspace/didChangeWatchedFiles":
		s.handleDidChangeWatchedFiles(ctx, msg.Params)
	case "workspace/didChangeConfiguration":
		s.handleDidChangeConfiguration(ctx)
	case "mdsmith/rulePatterns":
		s.handleRulePatterns(msg)
	default:
		return false
	}
	return true
}

