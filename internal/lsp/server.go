package lsp

import (
	"bytes"
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
	"github.com/jeduden/mdsmith/internal/engine"
	fixpkg "github.com/jeduden/mdsmith/internal/fix"
	"github.com/jeduden/mdsmith/internal/lint"
	vlog "github.com/jeduden/mdsmith/internal/log"
	"github.com/jeduden/mdsmith/internal/lsp/index"
	"github.com/jeduden/mdsmith/internal/rule"
)

// Server runs the LSP loop over a transport pair. One Server instance
// serves one client.
type Server struct {
	t              *transport
	rules          []rule.Rule
	debounce       time.Duration
	fetchTimeout   time.Duration
	discoverConfig func(string) (string, error)
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
}

// userSettings mirrors the subset of `mdsmith.*` VS Code keys the
// server consults. Defaults match the documented values in
// docs/guides/editors/vscode.md.
type userSettings struct {
	ConfigPath string `json:"config"`
	Run        string `json:"run"`
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
	return &Server{
		t:              newTransport(opts.Reader, opts.Writer),
		rules:          opts.Rules,
		debounce:       debounce,
		fetchTimeout:   2 * time.Second,
		discoverConfig: config.Discover,
		logger:         logger,
		docs:           newDocumentStore(),
		settings:       userSettings{Run: runOnSave},
		pending:        make(map[string]*pendingLint),
		pendingResp:    make(map[string]chan rpcResponse),
		diags:          make(map[string][]Diagnostic),
	}
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

func (s *Server) handleInitialize(msg *requestMessage) {
	var p initializeParams
	if len(msg.Params) > 0 {
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid initialize params")
			return
		}
	}
	root := pickRoot(p)
	s.configMu.Lock()
	s.rootDir = root
	s.configMu.Unlock()
	// Record the client's advertised capabilities so handleInitialized
	// can gate optional follow-up requests (workspace/configuration,
	// dynamic file watchers) instead of sending them blind. Clients
	// without `workspace.configuration` would otherwise return an
	// error for fetchClientSettings; those without
	// `didChangeWatchedFiles.dynamicRegistration` cannot honor the
	// register-capability request and we should not bother sending it.
	s.clientCapsMu.Lock()
	s.clientCaps = p.Capabilities
	s.clientCapsMu.Unlock()

	res := initializeResult{
		Capabilities: serverCapabilities{
			TextDocumentSync: textDocumentSyncOptions{
				OpenClose: true,
				Change:    syncFull,
				Save:      &saveOptions{IncludeText: false},
			},
			CodeActionProvider: codeActionOptions{
				CodeActionKinds: []string{kindQuickFix, kindSourceFixAll},
			},
			HoverProvider:           true,
			DocumentSymbolProvider:  true,
			DefinitionProvider:      true,
			ImplementationProvider:  true,
			ReferencesProvider:      true,
			WorkspaceSymbolProvider: true,
			CallHierarchyProvider:   true,
			CompletionProvider: &completionOptions{
				TriggerCharacters: []string{"#", "[", ":", "/", "\""},
				ResolveProvider:   false,
			},
			RenameProvider: &renameOptions{PrepareProvider: true},
		},
		ServerInfo: serverInfo{Name: "mdsmith", Version: "lsp"},
	}
	_ = s.t.writeResponse(msg.ID, res)
}

func (s *Server) handleInitialized(ctx context.Context) {
	// Load the workspace config eagerly so the first document event
	// already finds it cached.
	s.reloadConfig()
	s.clientCapsMu.RLock()
	caps := s.clientCaps
	s.clientCapsMu.RUnlock()
	// Gate workspace/configuration on the client's advertised
	// capability. Per LSP §5.6 a client that doesn't list
	// `workspace.configuration` will reject the request; without this
	// guard we would log a window/logMessage error on every Helix /
	// JetBrains-LSP / Neovim launch.
	if caps.Workspace != nil && caps.Workspace.Configuration {
		// fetchClientSettings runs in a goroutine because dispatch must
		// remain available to deliver the response.
		go s.fetchClientSettings(ctx)
	}
	// Same gate for dynamic file watchers. Clients without
	// `workspace.didChangeWatchedFiles.dynamicRegistration` cannot
	// honor a client/registerCapability request, so don't bother.
	// Users on those clients still get config reloads on the next
	// document event (no-op fallback) — they just don't get
	// instant re-lint when they edit .mdsmith.yml in another window.
	if caps.Workspace != nil && caps.Workspace.DidChangeWatchedFiles != nil &&
		caps.Workspace.DidChangeWatchedFiles.DynamicRegistration {
		s.registerWatchers()
	}
}

func (s *Server) handleDidOpen(ctx context.Context, raw json.RawMessage) {
	var p didOpenTextDocumentParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return
	}
	path := uriToPath(p.TextDocument.URI)
	if path == "" {
		return
	}
	s.docs.set(p.TextDocument.URI, &document{
		uri:     p.TextDocument.URI,
		path:    path,
		text:    []byte(p.TextDocument.Text),
		version: p.TextDocument.Version,
	})
	s.indexUpdate(path, []byte(p.TextDocument.Text))
	// didOpen lints unless run=off — the user wants an initial
	// snapshot when linting is on at all. scheduleLint applies the
	// same off-skip as every other trigger.
	s.scheduleLint(p.TextDocument.URI, lintTriggerOpen)
}

func (s *Server) handleDidChange(ctx context.Context, raw json.RawMessage) {
	var p didChangeTextDocumentParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return
	}
	if len(p.ContentChanges) == 0 {
		return
	}
	doc, ok := s.docs.get(p.TextDocument.URI)
	if !ok {
		return
	}
	last := p.ContentChanges[len(p.ContentChanges)-1]
	doc.text = []byte(last.Text)
	doc.version = p.TextDocument.Version
	s.docs.set(p.TextDocument.URI, doc)
	s.indexUpdate(doc.path, doc.text)
	s.scheduleLint(p.TextDocument.URI, lintTriggerChange)
}

// handleDidSave re-lints when the user saves. The onSave run mode
// triggers a lint pass on save, on document open, and on
// config-change events; the only event it skips is didChange. See
// scheduleLint for the full per-trigger / per-mode table.
func (s *Server) handleDidSave(ctx context.Context, raw json.RawMessage) {
	var p struct {
		TextDocument textDocumentIdentifier `json:"textDocument"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return
	}
	s.scheduleLint(p.TextDocument.URI, lintTriggerSave)
}

func (s *Server) handleDidClose(raw json.RawMessage) {
	var p didCloseTextDocumentParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return
	}
	uri := p.TextDocument.URI
	doc, _ := s.docs.get(uri)
	s.docs.delete(uri)
	// Cancel any armed debounce timer so a pending runLint cannot fire
	// and re-publish diagnostics after we clear them below. Collect the
	// timer under the lock, delete the map entry, then call Stop OUTSIDE
	// pendingMu — Stop hits the runtime timer heap and can block under
	// load, and holding pendingMu across it would serialize concurrent
	// scheduleLint callers. The local is named `pending` (not `p`) to
	// avoid shadowing the function parameter holding the LSP params.
	s.pendingMu.Lock()
	var pendingTimer *time.Timer
	if pending, ok := s.pending[uri]; ok {
		pendingTimer = pending.timer
		delete(s.pending, uri)
	}
	s.pendingMu.Unlock()
	if pendingTimer != nil {
		pendingTimer.Stop()
	}
	// Refresh the index from on-disk content so the closed buffer's
	// last-saved state replaces the editor-only edits we accumulated.
	// When the file no longer exists on disk we silently skip — the
	// watcher path will catch the deletion if it lands separately.
	if doc != nil {
		s.indexReloadFromDisk(doc.path)
	}
	// Clear cached diagnostics and squiggles on close.
	s.diagsMu.Lock()
	delete(s.diags, uri)
	s.diagsMu.Unlock()
	_ = s.t.writeNotification("textDocument/publishDiagnostics",
		publishDiagnosticsParams{URI: uri, Diagnostics: []Diagnostic{}})
}

func (s *Server) handleDidChangeWatchedFiles(ctx context.Context, raw json.RawMessage) {
	var p didChangeWatchedFilesParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return
	}
	configChanged := false
	mdChanges := make([]string, 0, len(p.Changes))
	for _, c := range p.Changes {
		path := uriToPath(c.URI)
		if strings.HasSuffix(path, ".mdsmith.yml") {
			configChanged = true
			continue
		}
		// Use isMarkdownExt for case-insensitive extension match
		// — the rest of the navigation surface (docTextOrFile,
		// indexReloadFromDisk) treats `.MD` / `.Markdown` as
		// Markdown, and the watcher must agree or a rename to a
		// case-shifted extension would silently stop refreshing
		// the index.
		if isMarkdownExt(path) {
			mdChanges = append(mdChanges, path)
		}
	}
	if configChanged {
		s.reloadConfig()
		// kind / ignore globs may have shifted — drop the index so
		// the next symbol request rebuilds it from scratch.
		s.invalidateIndex()
		for _, uri := range s.docs.openURIs() {
			s.scheduleLint(uri, lintTriggerConfig)
		}
		return
	}
	openPaths := s.openDocPaths()
	for _, path := range mdChanges {
		// Skip files the editor currently has open as a buffer — the
		// watcher event would otherwise overwrite the live edits with
		// the stale on-disk content, and symbol navigation would
		// silently jump back to the last saved version. Open buffers
		// are kept in sync via didOpen/didChange instead.
		if openPaths[path] {
			continue
		}
		s.indexReloadFromDisk(path)
	}
}

// openDocPaths returns the set of filesystem paths currently held as
// open buffers. The map is keyed by the same absolute path the
// watcher emits so callers can do a direct lookup.
func (s *Server) openDocPaths() map[string]bool {
	out := make(map[string]bool)
	for _, uri := range s.docs.openURIs() {
		if doc, ok := s.docs.get(uri); ok {
			out[doc.path] = true
		}
	}
	return out
}

func (s *Server) handleDidChangeConfiguration(ctx context.Context) {
	// fetchClientSettings reschedules the per-document lint passes
	// after the new settings (and re-discovered config) land, so the
	// republished diagnostics reflect the updated state instead of
	// the stale settings the dispatch goroutine still has cached.
	go s.fetchClientSettings(ctx)
}

// lintTrigger names what caused a lint pass to be scheduled.
type lintTrigger int

const (
	lintTriggerOpen   lintTrigger = iota // textDocument/didOpen
	lintTriggerChange                    // textDocument/didChange
	lintTriggerSave                      // textDocument/didSave
	lintTriggerConfig                    // config or settings change
)

// scheduleLint debounces lint runs per document. The mdsmith.run
// setting filters which triggers actually result in a lint pass:
//
//   - off:    never lints (still allows fix-all code actions on
//     explicit user request).
//   - onSave: lints on open, save, and config-change triggers; skips
//     didChange.
//   - onType: lints on every trigger, debounced by `debounce`.
//
// open/save/config triggers always run synchronously so the user sees
// the result without waiting for the debounce timer.
func (s *Server) scheduleLint(uri string, trigger lintTrigger) {
	if s.shutdown.Load() {
		return
	}
	mode := s.runMode()
	if mode == runOff {
		return
	}
	if mode == runOnSave && trigger == lintTriggerChange {
		return
	}
	// Both the immediate (open/save/config) and the debounced
	// (didChange) trigger paths run runLint via time.AfterFunc so
	// the dispatch goroutine never blocks on a CPU-bound lint
	// pass. Synchronous runLint blocked the loop from processing
	// inbound responses to server-initiated requests
	// (workspace/configuration, client/registerCapability) and
	// could deadlock on slow files. Immediate triggers use a
	// duration of 0 so the goroutine runs as soon as the runtime
	// schedules it; debounced triggers use s.debounce.
	delay := s.debounce
	if trigger != lintTriggerChange {
		delay = 0
	}
	// Identity-token allocation: see runLintIfCurrent below. `p`
	// is allocated before AfterFunc starts the timer goroutine, so
	// the closure captures a stable, non-nil *pendingLint as its
	// identity. The callback never reads `p.timer` — it only
	// compares its captured `p` against `s.pending[uri]` — so the
	// subsequent `p.timer = AfterFunc(...)` assignment is invisible
	// to the callback path and cannot race the callback.
	//
	// The previous entry's timer.Stop() runs OUTSIDE pendingMu. Stop
	// can be slow under load (heap operation on the runtime timer
	// wheel) and holding the lock across it would serialize every
	// concurrent scheduleLint call. A previous callback whose
	// goroutine started before Stop wins this race only to find
	// `s.pending[uri] == p` (not `prev`), so live=false and it
	// returns silently.
	s.pendingMu.Lock()
	prev, hadPrev := s.pending[uri]
	p := &pendingLint{}
	p.timer = time.AfterFunc(delay, func() {
		s.runLintIfCurrent(uri, p)
	})
	s.pending[uri] = p
	s.pendingMu.Unlock()
	// Under pendingMu above, p is allocated, p.timer is assigned,
	// and s.pending[uri] = p all happen atomically — no concurrent
	// reader can observe a registered *pendingLint with a nil
	// timer. The nil guard here is pure defense against a future
	// caller that constructs a *pendingLint without going through
	// scheduleLint and forgets to assign timer; production paths
	// never trigger it.
	if hadPrev && prev.timer != nil {
		prev.timer.Stop()
	}
}

// pendingLint is the identity token a debounced lint registers in
// s.pending. The pointer itself is the identity key — each
// scheduleLint call allocates a fresh *pendingLint. A stale
// callback can identify itself by comparing its captured pointer
// against s.pending[uri]: equal means we are still the live entry,
// not equal means a newer scheduleLint has replaced us.
//
// The Stop handle is kept on the entry so handleDidClose and
// stopPendingLints can cancel a still-pending timer without
// reaching back into closure-captured locals.
type pendingLint struct {
	timer *time.Timer
}

// runLintIfCurrent is the body of the AfterFunc callback armed by
// scheduleLint. It is a method (not an inline closure) so the
// live-flag branch — which a real timer race only reaches
// nondeterministically — is unit-testable.
//
// `p` is the *pendingLint scheduleLint allocated and registered in
// s.pending. The pointer is captured by the closure, so a callback
// firing before scheduleLint releases pendingMu blocks at the Lock
// below; by the time it proceeds, the registration has completed
// and `s.pending[uri] == p` resolves cleanly. A racing scheduleLint
// that replaces s.pending[uri] makes `p` stale, and we bail out —
// the replacement is responsible for the next publish, and without
// this guard the editor would see back-to-back lints with the
// older one flashing stale diagnostics on every fast keystroke.
//
// The shutdown re-check at the top is a cheap early-return for
// timers that armed before Run's deferred cleanup ran; it covers
// the window where stopPendingLints has not yet emptied the map.
func (s *Server) runLintIfCurrent(uri string, p *pendingLint) {
	// Fast-path: avoid the lock when shutdown has already been
	// initiated. The atomic re-check below catches the case where
	// the flag flips between this check and acquiring pendingMu.
	if s.shutdown.Load() {
		return
	}
	s.pendingMu.Lock()
	// Fold the shutdown re-check into the live decision so the
	// callback never publishes during teardown — even if shutdown
	// flips after the fast-path above but before we get the lock.
	// The explicit `ok` makes the check robust against a caller
	// that ever passes a nil p: without `ok`, a missing map entry
	// (nil) would compare equal to a nil p, leading to a spurious
	// runLint on a deleted URI.
	cur, ok := s.pending[uri]
	live := ok && cur == p && !s.shutdown.Load()
	if live {
		delete(s.pending, uri)
	}
	s.pendingMu.Unlock()
	if !live {
		return
	}
	s.runLint(uri)
}

// stopPendingLints cancels every armed debounce timer. Called from
// the shutdown/exit handlers so we do not publish diagnostics after
// the client asked us to stop.
func (s *Server) stopPendingLints() {
	// Collect entries under the lock so we can drop the map state
	// quickly, then call Stop OUTSIDE the lock. Stop hits the
	// runtime timer heap and can block under load; holding
	// pendingMu across N Stop calls would serialize every
	// concurrent scheduleLint behind teardown.
	s.pendingMu.Lock()
	timers := make([]*time.Timer, 0, len(s.pending))
	for uri, p := range s.pending {
		if p.timer != nil {
			timers = append(timers, p.timer)
		}
		delete(s.pending, uri)
	}
	s.pendingMu.Unlock()
	for _, t := range timers {
		t.Stop()
	}
}

func (s *Server) runMode() string {
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	switch s.settings.Run {
	case runOff, runOnSave, runOnType:
		return s.settings.Run
	default:
		return runOnSave
	}
}

// runLint executes one lint pass on the buffer and publishes the
// resulting diagnostics. Safe to call from any goroutine.
//
// The path passed to engine.RunSource is normalized to be
// workspace-relative when possible, since config.IsIgnored,
// kind-assignment, and override matching all glob against repo-style
// paths ("docs/foo.md") rather than absolute file URIs. RunSource is
// then asked to wire FS=os.DirFS(absoluteDir) so rules that read
// neighbouring files (include, catalog) see the same view the CLI
// would.
func (s *Server) runLint(uri string) {
	doc, ok := s.docs.get(uri)
	if !ok {
		return
	}
	cfg, configPath, root := s.snapshotConfig()
	if cfg == nil {
		cfg = config.Merge(config.Defaults(), nil)
	}
	relPath := workspaceRelative(root, doc.path)
	if config.IsIgnored(cfg.Ignore, relPath) {
		s.diagsMu.Lock()
		s.diags[uri] = nil
		s.diagsMu.Unlock()
		_ = s.t.writeNotification("textDocument/publishDiagnostics",
			publishDiagnosticsParams{URI: uri, Version: doc.version, Diagnostics: []Diagnostic{}})
		return
	}
	maxBytes := s.resolveMaxInputBytes(cfg)
	r := &engine.Runner{
		Config:           cfg,
		Rules:            s.rules,
		StripFrontMatter: frontMatterEnabled(cfg),
		RootDir:          root,
		MaxInputBytes:    maxBytes,
		SourceFS:         dirFSForPath(doc.path),
		// ConfigPath gates whether config-target rules execute (see
		// engine.Runner.runConfigTargetRules); without it, LSP
		// linting silently skips those rules even when a config is
		// loaded.
		ConfigPath: configPath,
	}
	res := r.RunSource(relPath, doc.text)
	// engine.RunSource is CPU-bound and can run for hundreds of
	// milliseconds on large buffers. The client may have requested
	// shutdown/exit while we were busy; if so, drop everything we
	// would have published so the dispatch loop's teardown path is
	// not racing publishDiagnostics writes against a half-closed
	// pipe. The shutdown flag is set both by the explicit shutdown
	// handler and by Run's deferred cleanup, so checking it covers
	// every termination cause.
	if s.shutdown.Load() {
		return
	}
	// If the document was closed while we were linting, discard results
	// to avoid re-publishing stale diagnostics over didClose's empty notification.
	if _, ok := s.docs.get(uri); !ok {
		return
	}
	// Mirror `mdsmith check`: surface lint pipeline errors (parse
	// failures, oversized buffers, config-target rule errors) to
	// the editor instead of silently dropping them. Otherwise the
	// editor would show no diagnostics and look broken.
	for _, e := range res.Errors {
		s.logger.Printf("lint %s: %v", uri, e)
		_ = s.t.writeNotification("window/logMessage",
			logMessageParams{Type: messageTypeError, Message: "mdsmith: " + e.Error()})
	}
	// engine.RunSource also fires config-target rules whose
	// Diagnostic.File is the .mdsmith.yml path, not relPath. Showing
	// those as squiggles in the markdown buffer would put a finding
	// at the wrong file/line; route them to window/logMessage with
	// the file:line prefix the user needs to locate the issue, and
	// only publish diagnostics whose File matches the document we
	// just linted.
	docDiags, otherDiags := partitionDocDiagnostics(res.Diagnostics, relPath)
	s.surfaceForeignDiagnostics(uri, otherDiags)
	lspDiags := toLSPAll(docDiags, doc.text)
	// Cache before publishing so hover requests that arrive after the
	// client observes the notification always find current diagnostics.
	s.diagsMu.Lock()
	s.diags[uri] = lspDiags
	s.diagsMu.Unlock()
	_ = s.t.writeNotification("textDocument/publishDiagnostics",
		publishDiagnosticsParams{URI: uri, Version: doc.version, Diagnostics: lspDiags})
}

// resolveMaxInputBytes mirrors cmd/mdsmith's resolution of the
// project's `max-input-size`: unset (empty string) → default cap,
// "0" → unlimited, otherwise the parsed byte count. Parse errors
// fall back to the default and are surfaced via window/logMessage
// so the editor user can correct the config.
func (s *Server) resolveMaxInputBytes(cfg *config.Config) int64 {
	raw := ""
	if cfg != nil {
		raw = cfg.MaxInputSize
	}
	if raw == "" {
		return lint.DefaultMaxInputBytes
	}
	n, err := config.ParseSize(raw)
	if err != nil {
		s.logger.Printf("config: invalid max-input-size %q: %v", raw, err)
		_ = s.t.writeNotification("window/logMessage", logMessageParams{
			Type:    messageTypeError,
			Message: fmt.Sprintf("mdsmith: invalid max-input-size %q: %v", raw, err),
		})
		return lint.DefaultMaxInputBytes
	}
	return n
}

// surfaceForeignDiagnostics logs and notifies the client about
// diagnostics produced for a different file than the markdown
// buffer that triggered the lint pass — typically config-target
// rule findings against .mdsmith.yml. Pulled out of runLint so
// the routing has a unit-testable seam. Each diagnostic's
// severity is mapped to the matching window/logMessage type so
// warnings stay distinguishable from errors in the editor's
// output channel.
func (s *Server) surfaceForeignDiagnostics(uri string, diags []lint.Diagnostic) {
	for _, d := range diags {
		s.logger.Printf("lint %s: %s:%d %s [%s]", uri, d.File, d.Line, d.Message, d.RuleName)
		_ = s.t.writeNotification("window/logMessage", logMessageParams{
			Type:    messageTypeForLint(d.Severity),
			Message: fmt.Sprintf("mdsmith: %s:%d %s [%s]", d.File, d.Line, d.Message, d.RuleName),
		})
	}
}

// messageTypeForLint maps a lint severity to the
// window/logMessage MessageType the LSP spec defines (§3.18.1).
// Anything that isn't an explicit warning is reported as Error
// so the user notices — config-target findings tend to be
// actionable.
func messageTypeForLint(s lint.Severity) messageType {
	if s == lint.Warning {
		return messageTypeWarning
	}
	return messageTypeError
}

// partitionDocDiagnostics splits Runner-produced diagnostics into
// the ones that belong to the document we just linted and the ones
// that came from a different file (typically config-target rule
// findings against .mdsmith.yml). A diagnostic with an empty File
// is treated as belonging to the document — older rules left File
// blank when they only ever ran in single-file mode, and the LSP
// publishes against the document URI either way.
func partitionDocDiagnostics(diags []lint.Diagnostic, docPath string) (forDoc, other []lint.Diagnostic) {
	for _, d := range diags {
		if d.File == "" || d.File == docPath {
			forDoc = append(forDoc, d)
		} else {
			other = append(other, d)
		}
	}
	return forDoc, other
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

func (s *Server) handleCodeAction(msg *requestMessage) {
	var p codeActionParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid codeAction params")
		return
	}
	doc, ok := s.docs.get(p.TextDocument.URI)
	if !ok {
		_ = s.t.writeResponse(msg.ID, []codeAction{})
		return
	}
	cfg, _, root := s.snapshotConfig()
	if cfg == nil {
		cfg = config.Merge(config.Defaults(), nil)
	}
	// Mirror `mdsmith fix`'s on-disk behavior: skip every code
	// action when the document is in the project ignore list.
	// VS Code's editor.codeActionsOnSave can fire `source.fixAll`
	// even on files that never produced diagnostics, so without
	// this guard an ignored buffer would still be rewritten.
	if config.IsIgnored(cfg.Ignore, workspaceRelative(root, doc.path)) {
		_ = s.t.writeResponse(msg.ID, []codeAction{})
		return
	}
	actions := s.computeCodeActions(p, doc, cfg, root)
	_ = s.t.writeResponse(msg.ID, actions)
}

// computeCodeActions returns the set of code actions for one
// codeAction request. When `Only` is supplied we short-circuit kinds
// the client did not ask for so we don't run fix passes whose output
// the client will discard.
//
// Per-rule fix passes are deduped within a single request: a file
// with N MDS006 diagnostics issues only one fix.SourceWithRules call,
// not N. The resulting WorkspaceEdit is shared across the
// per-diagnostic actions, since each one would have produced the
// same whole-file edit anyway. This keeps the latency budget bounded
// even on files with many diagnostics from the same rule.
func (s *Server) computeCodeActions(
	p codeActionParams, doc *document, cfg *config.Config, root string,
) []codeAction {
	wantQuickFix := wantsKind(p.Context.Only, kindQuickFix)
	wantFixAll := wantsKind(p.Context.Only, kindSourceFixAll)

	actions := make([]codeAction, 0, len(p.Context.Diagnostics)+1)

	if wantQuickFix {
		// Cache fix results per rule so we run one fix.SourceWithRules
		// pass per distinct rule. nil entries mark rules whose fix is
		// either unavailable or a no-op against the current buffer.
		ruleEdits := make(map[string]*workspaceEdit)
		for _, d := range p.Context.Diagnostics {
			if d.Data == nil || d.Data.RuleName == "" {
				continue
			}
			rule := d.Data.RuleName
			edit, cached := ruleEdits[rule]
			if !cached {
				edit = s.quickFixEditFor(rule, doc, cfg, root, p.TextDocument.URI)
				ruleEdits[rule] = edit
			}
			if edit == nil {
				continue
			}
			actions = append(actions, codeAction{
				Title:       quickFixTitle(rule),
				Kind:        kindQuickFix,
				Diagnostics: []Diagnostic{d},
				Edit:        edit,
			})
		}
	}

	if wantFixAll {
		// fix.Source's Path is fed to config glob matching (ignore /
		// override / kind-assignment), which works against repo-style
		// relative paths. Pass the workspace-relative form so LSP
		// fixes match `mdsmith fix` on disk, and a SourceFS rooted
		// at the document's real directory so include/catalog rules
		// still resolve neighbour files independent of the process
		// CWD.
		relPath := workspaceRelative(root, doc.path)
		fixed, err := fixpkg.Source(fixpkg.SourceOptions{
			Config:           cfg,
			Rules:            s.rules,
			Path:             relPath,
			Source:           doc.text,
			RootDir:          root,
			SourceFS:         dirFSForPath(doc.path),
			StripFrontMatter: frontMatterEnabled(cfg),
			MaxInputBytes:    s.resolveMaxInputBytes(cfg),
		})
		if err == nil && !bytes.Equal(fixed, doc.text) {
			actions = append(actions, codeAction{
				Title: titleFixAllMdsmith,
				Kind:  kindSourceFixAll,
				Edit:  fullFileEdit(p.TextDocument.URI, doc.text, fixed),
			})
		}
	}

	return actions
}

// quickFixEditFor returns the WorkspaceEdit produced by running just
// `rule` over the buffer, or nil if the rule is not fixable or its
// fix is a no-op against the current buffer.
//
// The returned edit replaces the entire document because rules
// produce whole-file-fix output rather than per-range edits. The
// quick fix therefore covers every occurrence of the rule, not only
// the diagnostic the user clicked on; the action title reflects this
// ("Fix all <rule> with mdsmith").
func (s *Server) quickFixEditFor(
	rule string, doc *document, cfg *config.Config, root, uri string,
) *workspaceEdit {
	if !isFixable(s.rules, rule) {
		return nil
	}
	relPath := workspaceRelative(root, doc.path)
	fixed, err := fixpkg.SourceWithRules(fixpkg.SourceOptions{
		Config:           cfg,
		Rules:            s.rules,
		Path:             relPath,
		Source:           doc.text,
		RootDir:          root,
		SourceFS:         dirFSForPath(doc.path),
		StripFrontMatter: frontMatterEnabled(cfg),
		MaxInputBytes:    s.resolveMaxInputBytes(cfg),
	}, []string{rule})
	if err != nil || bytes.Equal(fixed, doc.text) {
		return nil
	}
	return fullFileEdit(uri, doc.text, fixed)
}

// wantsKind reports whether the client's `Only` filter accepts the
// given action kind. An empty/missing filter means "all kinds wanted",
// matching the LSP spec.
func wantsKind(only []string, kind string) bool {
	if len(only) == 0 {
		return true
	}
	for _, k := range only {
		// LSP allows kind prefixes (e.g. "source" matches
		// "source.fixAll.mdsmith"); follow that convention.
		if k == kind || strings.HasPrefix(kind, k+".") {
			return true
		}
	}
	return false
}

// quickFixTitle returns the lightbulb label. Phrased "Fix all" to
// signal that the action's WorkspaceEdit covers every occurrence of
// the rule, not only the diagnostic the user clicked on — see the
// comment on quickFixEditFor for why the edit is whole-file scoped.
func quickFixTitle(rule string) string {
	return "Fix all " + rule + " with mdsmith"
}

// fullFileEdit returns a WorkspaceEdit that replaces the entire
// document with `after`. The replacement range covers `before`
// (the buffer the client currently has): start at {0, 0} and end at
// documentEndPosition(before) — see that function's doc for the
// exact end coordinates. Sizing the range against `before` matches
// the LSP contract — clients apply a TextEdit by replacing the
// named range in the existing document.
func fullFileEdit(uri string, before, after []byte) *workspaceEdit {
	endLine, endChar := documentEndPosition(before)
	return &workspaceEdit{
		Changes: map[string][]textEdit{
			uri: {
				{
					Range: Range{
						Start: Position{Line: 0, Character: 0},
						End:   Position{Line: endLine, Character: endChar},
					},
					NewText: string(after),
				},
			},
		},
	}
}

// documentEndPosition returns the LSP end position covering the
// entire `source`. The end position is one-past-the-last-character
// in LSP coordinates:
//
//   - Empty input: (0, 0).
//   - Trailing-newline-terminated content (e.g. "abc\n"): the line
//     index equal to the number of newlines, character 0 — i.e. the
//     virtual empty line just past the final \n. For "abc\n" the
//     result is (1, 0); for "abc\ndef\n" it is (2, 0). This matches
//     LSP §3.18 (TextDocumentItem) where a final \n produces a
//     trailing empty line whose position is the file's end.
//   - No trailing newline: the last line's index plus its UTF-16
//     length, e.g. (0, 3) for "abc" or (1, 3) for "abc\ndef".
func documentEndPosition(source []byte) (int, int) {
	if len(source) == 0 {
		return 0, 0
	}
	if source[len(source)-1] == '\n' {
		// Count newlines; the position past the final \n is the
		// one-past-the-end line, character 0.
		nl := 0
		for _, b := range source {
			if b == '\n' {
				nl++
			}
		}
		return nl, 0
	}
	// No trailing newline: end at last line's UTF-16 length. source
	// is non-empty here (checked above), so splitLines always yields
	// at least one element.
	lines := splitLines(source)
	return len(lines) - 1, utf16Length(lines[len(lines)-1])
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
	s.config = cfg
	s.configPath = cfgPath
	s.configMu.Unlock()

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
		s.settingsMu.Unlock()
		// Reload config in case `mdsmith.config` changed, then
		// re-lint open buffers so diagnostics reflect the freshly
		// applied settings rather than whatever was in effect when
		// handleDidChangeConfiguration fired.
		s.reloadConfig()
		for _, uri := range s.docs.openURIs() {
			s.scheduleLint(uri, lintTriggerConfig)
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

// registerWatchers asks the client to watch project files we depend
// on:
//
//   - `**/.mdsmith.yml` invalidates cached config and the symbol
//     index (kind / ignore globs may shift scope).
//   - `**/*.md` keeps the symbol index in sync when files change
//     outside of any open buffer (sibling editor, VCS checkout).
//
// The request is best-effort: clients that don't support dynamic
// registration silently ignore it. There is no polling fallback;
// when the watcher is absent, the index still updates from open
// buffer events.
func (s *Server) registerWatchers() {
	id := s.nextReqID.Add(1)
	// json.Marshal(int64) cannot fail; ignoring the error is safe.
	idJSON, _ := json.Marshal(id)
	_ = s.t.writeRequest(idJSON, "client/registerCapability",
		registrationParams{Registrations: []registration{{
			ID:     "mdsmith-watch",
			Method: "workspace/didChangeWatchedFiles",
			RegisterOptions: didChangeWatchedFilesRegistrationOptions{
				Watchers: []fileSystemWatcher{
					{GlobPattern: "**/.mdsmith.yml"},
					{GlobPattern: "**/*.md"},
					{GlobPattern: "**/*.markdown"},
				},
			},
		}}})
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
