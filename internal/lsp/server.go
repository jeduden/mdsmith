package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
func (s *Server) dispatchRaw(ctx context.Context, raw []byte) {
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
	s.dispatch(ctx, msg)
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

// workspaceRelative converts an absolute filesystem path to a path
// relative to the workspace root. Returns the input unchanged when
// root is empty, when path is already relative, or when path lies
// outside root (which would otherwise produce an unhelpful "../"
// prefix that does not match repo-style globs).
func workspaceRelative(root, path string) string {
	if root == "" || !filepath.IsAbs(path) {
		return path
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	// Only treat true parent traversals as outside root. A bare
	// HasPrefix(rel, "..") would also match in-root files whose
	// names happen to start with two dots (e.g. "..foo.md"),
	// breaking glob/ignore matching for those files.
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return rel
}

// dirFSForPath returns os.DirFS rooted at the directory containing
// path, or nil when path is not absolute (e.g. an in-memory test
// label). engine.Runner treats a nil SourceFS as "do not override
// the default" so this is safe in all cases.
func dirFSForPath(path string) fs.FS {
	if !filepath.IsAbs(path) {
		return nil
	}
	return os.DirFS(filepath.Dir(path))
}

// rebuildSession constructs a fresh per-workspace Session over an
// OverlayWorkspace rooted at the effective project root, disposes the
// previous one, and re-seeds the new overlay with every open buffer so
// cross-file rules keep reading unsaved bytes across the rebuild. cfg is
// already merged (and carries the include-extract projector / build
// injection the host applied), so it is handed over with ConfigCompiled
// and used as-is.
//
// A failure to build the session is non-fatal: NewSession only errors
// when its ConfigSource fails to load, and a compiled source cannot, so
// this never returns an error in practice. On the off chance it did, the
// previous session is left in place rather than dropped.
func (s *Server) rebuildSession(cfg *config.Config, cfgPath string) {
	root := cfgPath
	if root != "" {
		root = filepath.Dir(cfgPath)
	} else {
		s.configMu.RLock()
		root = s.rootDir
		s.configMu.RUnlock()
	}
	// Surface a malformed max-input-size to the editor (the session
	// silently falls back to the default; this keeps the user-facing
	// warning the LSP showed before).
	s.resolveMaxInputBytes(cfg)
	ws := mdsmith.NewOverlayWorkspace(root)
	sess, err := s.newSession(mdsmith.SessionOptions{
		Workspace: ws,
		Config:    mdsmith.ConfigCompiled(cfg, cfgPath),
	})
	if err != nil {
		s.logger.Printf("session: rebuild failed: %v", err)
		return
	}
	// Seed the overlay with every open buffer before publishing the new
	// session, so the first cross-file Check after a reload already sees
	// unsaved bytes.
	for _, uri := range s.docs.openURIs() {
		if doc, ok := s.docs.get(uri); ok {
			ws.Set(workspaceRelative(root, doc.path), doc.text)
		}
	}
	s.sessionMu.Lock()
	s.session = sess
	s.workspace = ws
	s.sessionMu.Unlock()
	// Do NOT Dispose the superseded session. A lint/fix goroutine may
	// still hold it (obtained from currentSession() before this swap),
	// and Dispose nils its checkCache under lock — so the held session's
	// next Check would lose its warm cache, and a concurrent reload while
	// linting is in flight is exactly when that happens. The superseded
	// session is unreferenced once every in-flight caller returns, so GC
	// reclaims it (its caches are plain maps, nothing OS-level to release);
	// the public Dispose() stays for external callers that own a session's
	// whole lifetime. Letting GC reap it keeps the invariant simple: a
	// session handed out by currentSession() is never disposed underfoot.
}

// currentSession returns the active session and its overlay workspace
// under the session lock, building one on demand if none exists yet.
// reloadConfig (from handleInitialized) builds the session eagerly for
// the normal path; this lazy fallback covers a client that lints after
// only `initialize` — there the session must still exist, with whatever
// config snapshotConfig holds (defaults when none was discovered),
// matching the pre-session behaviour where runLint linted against
// default config.
func (s *Server) currentSession() (*mdsmith.Session, *mdsmith.OverlayWorkspace) {
	s.sessionMu.RLock()
	sess, ws := s.session, s.workspace
	s.sessionMu.RUnlock()
	if sess != nil {
		return sess, ws
	}
	cfg, cfgPath, _ := s.snapshotConfig()
	if cfg == nil {
		cfg = config.Merge(config.Defaults(), nil)
	}
	s.rebuildSession(cfg, cfgPath)
	s.sessionMu.RLock()
	defer s.sessionMu.RUnlock()
	return s.session, s.workspace
}

// snapshotConfig returns the cached config, its source path, and the
// effective project root used for glob/ignore matching and as
// Runner.RootDir. The root mirrors the CLI's rootDirFromConfig:
// when a config file is loaded, the project root is the directory
// containing it (so ignore globs and overrides match the CLI even
// when the workspace folder is a subdirectory or the user pointed
// `mdsmith.config` at a config outside the workspace). When no
// config was discovered, the workspace folder root is used. Either
// value may be empty when neither is known yet.
func (s *Server) snapshotConfig() (*config.Config, string, string) {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	root := s.rootDir
	if s.configPath != "" {
		root = filepath.Dir(s.configPath)
	}
	return s.config, s.configPath, root
}

// reloadConfig walks from rootDir (or the user-supplied
// `mdsmith.config`) and refreshes the cached config. Any load /
// discover failure falls back to defaults and is surfaced via
// window/logMessage so the editor user can diagnose
// misconfiguration instead of silently seeing stale or default
// diagnostics.
func (s *Server) reloadConfig() {
	s.settingsMu.RLock()
	override := s.settings.ConfigPath
	s.settingsMu.RUnlock()

	cfg, cfgPath, loadErr := s.resolveConfig(override)

	s.configMu.Lock()
	pathChanged := s.configPath != cfgPath
	s.config = cfg
	s.configPath = cfgPath
	s.configMu.Unlock()

	// Rebuild the per-workspace Session against the freshly merged
	// config. The session compiles config once, so any reload (config or
	// settings change) needs a new one; this also gives it fresh caches,
	// which subsumes the old per-path parseCache.InvalidateAll on a moved
	// config path. The new overlay is re-seeded with every open buffer so
	// cross-file rules keep seeing unsaved bytes after the rebuild.
	s.rebuildSession(cfg, cfgPath)

	if pathChanged {
		// Notify the host only when the config path actually changes,
		// matching the OnConfigReload field doc ("resolves a new
		// config path"). A no-op reload (every didChangeConfiguration
		// where the file did not move) should not re-take the include
		// projector's write lock or re-build closures the host owns.
		if s.onConfigReload != nil {
			s.onConfigReload(cfgPath)
		}
	}

	if loadErr != "" {
		s.logger.Printf("config: %s", loadErr)
		_ = s.t.writeNotification("window/logMessage",
			logMessageParams{Type: messageTypeError, Message: "mdsmith: " + loadErr})
	}
}

// resolveConfig is the load/discover flow extracted from
// reloadConfig so the caller can release configMu before notifying
// the client. The returned cfg is always non-nil (defaults on
// failure); cfgPath is empty when no config was successfully
// loaded; loadErr is a human-readable message when load or
// discover surfaced an error worth logging.
func (s *Server) resolveConfig(override string) (cfg *config.Config, cfgPath, loadErr string) {
	defaults := config.Defaults()
	fallback := config.Merge(defaults, nil)

	if override != "" {
		path := override
		s.configMu.RLock()
		root := s.rootDir
		s.configMu.RUnlock()
		if !filepath.IsAbs(path) && root != "" {
			path = filepath.Join(root, path)
		}
		loaded, err := config.Load(path)
		if err != nil {
			return fallback, "", fmt.Sprintf("loading %q: %v", path, err)
		}
		return config.Merge(defaults, loaded), path, ""
	}

	s.configMu.RLock()
	root := s.rootDir
	s.configMu.RUnlock()
	if root == "" {
		return fallback, "", ""
	}
	discovered, err := s.discoverConfig(root)
	if err != nil {
		return fallback, "", fmt.Sprintf("discovering config under %q: %v", root, err)
	}
	if discovered == "" {
		return fallback, "", ""
	}
	loaded, err := config.Load(discovered)
	if err != nil {
		return fallback, "", fmt.Sprintf("loading %q: %v", discovered, err)
	}
	return config.Merge(defaults, loaded), discovered, ""
}

// fetchClientSettings asks the client for its `mdsmith` configuration
// section, waits for the response, applies it to s.settings, and
// reschedules a lint pass for every open document so the diagnostics
// reflect the new run mode and config. If the client does not
// respond within fetchTimeout the call returns without touching
// either the cached settings or the open buffers — the previous
// values stand.
//
// Must be called from a goroutine other than the dispatch loop, since
// the response arrives on the same loop.
func (s *Server) fetchClientSettings(ctx context.Context) {
	id := s.nextReqID.Add(1)
	// json.Marshal(int64) cannot fail; ignoring the error is safe.
	idJSON, _ := json.Marshal(id)
	ch := s.registerPendingResponse(string(idJSON))
	defer s.unregisterPendingResponse(string(idJSON))

	if err := s.t.writeRequest(idJSON, "workspace/configuration",
		configurationParams{Items: []configurationItem{{Section: "mdsmith"}}}); err != nil {
		return
	}

	// time.NewTimer + Stop instead of time.After: this function runs
	// on every workspace/didChangeConfiguration, so a fast-replying
	// client would otherwise leak one runtime timer per settings
	// change — not catastrophic, but avoidable. Stop releases the
	// timer eagerly when the response (or ctx) wins the select.
	timeout := time.NewTimer(s.fetchTimeout)
	defer timeout.Stop()

	select {
	case resp := <-ch:
		if resp.Error != nil || len(resp.Result) == 0 {
			return
		}
		// The result is an array (one entry per requested item). Our
		// single item ("mdsmith") yields a one-element array.
		var arr []clientSettings
		if err := json.Unmarshal(resp.Result, &arr); err != nil || len(arr) == 0 {
			return
		}
		s.settingsMu.Lock()
		// Only the fields the client actually supplied land in
		// s.settings. Pointer-nil means "absent" (e.g. JSON null
		// for an unset key), so the cached default stays. A
		// pointer to "" means the client explicitly cleared the
		// setting — propagate it so the user can revert
		// `mdsmith.config` back to the default.
		next := arr[0]
		if next.ConfigPath != nil {
			s.settings.ConfigPath = *next.ConfigPath
		}
		if next.Run != nil {
			s.settings.Run = *next.Run
		}
		if next.PreviewFix != nil {
			s.settings.PreviewFix = *next.PreviewFix
		}
		s.settingsMu.Unlock()
		// Reload config in case `mdsmith.config` changed, then
		// re-lint open buffers so diagnostics reflect the freshly
		// applied settings rather than whatever was in effect when
		// handleDidChangeConfiguration fired.
		s.reloadConfig()
		if s.runMode() == runOff {
			// off is a master switch: scheduleLint publishes nothing
			// in off mode, so squiggles shown before the switch would
			// linger until the buffer closes. Drop them and tell the
			// client to clear them.
			s.clearOpenDiagnostics()
		} else {
			for _, uri := range s.docs.openURIs() {
				s.scheduleLint(uri, lintTriggerConfig)
			}
		}
	case <-timeout.C:
		// Client never replied; defaults stand.
	case <-ctx.Done():
	}
}

// registerPendingResponse returns a channel that will receive the
// reply for the given request id.
func (s *Server) registerPendingResponse(id string) chan rpcResponse {
	ch := make(chan rpcResponse, 1)
	s.pendingRespMu.Lock()
	s.pendingResp[id] = ch
	s.pendingRespMu.Unlock()
	return ch
}

func (s *Server) unregisterPendingResponse(id string) {
	s.pendingRespMu.Lock()
	delete(s.pendingResp, id)
	s.pendingRespMu.Unlock()
}

// deliverResponse routes an incoming response to the channel the
// requester registered. Unknown ids are silently dropped — the client
// may legitimately reply to a request that has already timed out.
func (s *Server) deliverResponse(id string, resp rpcResponse) {
	s.pendingRespMu.Lock()
	ch, ok := s.pendingResp[id]
	s.pendingRespMu.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- resp:
	default:
	}
}

func frontMatterEnabled(cfg *config.Config) bool {
	if cfg == nil || cfg.FrontMatter == nil {
		return true
	}
	return *cfg.FrontMatter
}

func isFixable(rules []rule.Rule, name string) bool {
	for _, r := range rules {
		if r.Name() != name {
			continue
		}
		_, ok := r.(rule.FixableRule)
		return ok
	}
	return false
}

// uriToPath converts a `file://` URI to a filesystem path. Non-file
// URIs return "" so the caller can skip them.
//
// Host handling:
//
//   - Empty host (`file:///path`) is the common case.
//   - "localhost" is treated as empty per RFC 8089 §3.
//   - On Windows, a non-empty/non-localhost host produces a UNC path
//     (`\\server\share\…`); on other platforms we conservatively
//     return "" because we have no way to mount a remote share.
func uriToPath(uri string) string {
	return uriToPathOnOS(uri, runtime.GOOS)
}

// uriToPathOnOS is uriToPath split out so tests can exercise the
// Windows-only branches (UNC translation, drive-letter stripping)
// from any platform.
func uriToPathOnOS(uri, goos string) string {
	if !strings.HasPrefix(uri, "file://") {
		return ""
	}
	u, err := url.Parse(uri)
	// url.Parse only fails on inputs like "%". Anything that passed
	// the "file://" prefix check above is well-formed enough to
	// parse; the err-return is defensive and unreachable in
	// practice.
	if err != nil {
		return ""
	}
	host := u.Host
	if strings.EqualFold(host, "localhost") {
		host = ""
	}
	p := u.Path
	if host != "" {
		// UNC path on Windows: file://server/share/path → \\server\share\path
		if goos == "windows" {
			return filepath.Clean(`\\` + host + filepath.FromSlash(p))
		}
		// Non-Windows: we cannot resolve a remote share, so refuse.
		return ""
	}
	// Windows: file:///C:/foo decodes to "/C:/foo"; strip the
	// leading slash only when the path actually starts with a
	// drive-letter pattern, so a non-Windows absolute path whose
	// third byte happens to be ':' (e.g. "/a:/tmp/file.md") is left
	// alone. The check is also gated on Windows so the fix never
	// fires on platforms that don't have drive letters.
	if goos == "windows" && hasDriveLetterPrefix(p) {
		p = p[1:]
	}
	return filepath.Clean(p)
}

// hasDriveLetterPrefix reports whether p starts with "/X:/" or "/X:"
// where X is an ASCII letter — i.e. the canonical Windows
// drive-letter-after-leading-slash pattern produced by url.Parse on a
// `file:///C:/…` URI.
func hasDriveLetterPrefix(p string) bool {
	if len(p) < 3 || p[0] != '/' || p[2] != ':' {
		return false
	}
	c := p[1]
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

// pickRoot derives the workspace root from initialize params.
func pickRoot(p initializeParams) string {
	if len(p.WorkspaceFolders) > 0 {
		if path := uriToPath(p.WorkspaceFolders[0].URI); path != "" {
			return path
		}
	}
	// rootUri is `DocumentUri | null` per LSP §3.16. The pointer
	// dereference covers both the missing-key case (nil) and the
	// explicit JSON null case (also nil after Unmarshal).
	if p.RootURI != nil {
		if path := uriToPath(*p.RootURI); path != "" {
			return path
		}
	}
	return ""
}
