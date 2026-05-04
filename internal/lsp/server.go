package lsp

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/jeduden/mdsmith/internal/rule"
)

// Server runs the LSP loop over a transport pair. One Server instance
// serves one client.
type Server struct {
	t        *transport
	rules    []rule.Rule
	clock    func() time.Time
	debounce time.Duration
	logger   *vlog.Logger
	docs     *documentStore

	configMu   sync.RWMutex
	config     *config.Config
	configPath string
	rootDir    string

	settingsMu sync.RWMutex
	settings   userSettings

	pendingMu     sync.Mutex
	pending       map[string]*time.Timer
	pendingRespMu sync.Mutex
	pendingResp   map[string]chan rpcResponse

	nextReqID     atomic.Int64
	shutdown      atomic.Bool
	exitRequested atomic.Bool
}

// userSettings mirrors the subset of `mdsmith.*` VS Code keys the
// server consults. Defaults match the documented values in
// docs/guides/editors/vscode.md.
type userSettings struct {
	ConfigPath string `json:"config"`
	Run        string `json:"run"`
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
		t:           newTransport(opts.Reader, opts.Writer),
		rules:       opts.Rules,
		clock:       time.Now,
		debounce:    debounce,
		logger:      logger,
		docs:        newDocumentStore(),
		settings:    userSettings{Run: runOnSave},
		pending:     make(map[string]*time.Timer),
		pendingResp: make(map[string]chan rpcResponse),
	}
}

// Run drives the server until the input stream returns io.EOF or the
// client sends `exit`.
func (s *Server) Run(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		raw, err := s.t.readRaw()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		s.dispatchRaw(ctx, raw)
		if s.shutdown.Load() && s.exitRequested.Load() {
			return nil
		}
	}
}

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
	// Response: has id, no method.
	if probe.Method == "" && len(probe.ID) > 0 {
		s.deliverResponse(string(probe.ID), rpcResponse{Result: probe.Result, Error: probe.Error})
		return
	}
	msg := &requestMessage{
		JSONRPC: probe.JSONRPC, ID: probe.ID, Method: probe.Method, Params: probe.Params,
	}
	s.dispatch(ctx, msg)
}

func (s *Server) dispatch(ctx context.Context, msg *requestMessage) {
	switch msg.Method {
	case "initialize":
		s.handleInitialize(msg)
	case "initialized":
		s.handleInitialized(ctx)
	case "shutdown":
		s.shutdown.Store(true)
		s.stopPendingLints()
		_ = s.t.writeResponse(msg.ID, nil)
	case "exit":
		s.shutdown.Store(true)
		s.exitRequested.Store(true)
		s.stopPendingLints()
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
	case "workspace/didChangeWatchedFiles":
		s.handleDidChangeWatchedFiles(ctx, msg.Params)
	case "workspace/didChangeConfiguration":
		s.handleDidChangeConfiguration(ctx)
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
		},
		ServerInfo: serverInfo{Name: "mdsmith", Version: "lsp"},
	}
	_ = s.t.writeResponse(msg.ID, res)
}

func (s *Server) handleInitialized(ctx context.Context) {
	// Load the workspace config eagerly so the first document event
	// already finds it cached.
	s.reloadConfig()
	// fetchClientSettings runs in a goroutine because dispatch must
	// remain available to deliver the response.
	go s.fetchClientSettings(ctx)
	s.registerWatchers()
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
	s.scheduleLint(p.TextDocument.URI, lintTriggerChange)
}

// handleDidSave re-lints when the user saves. This is the only event
// that triggers a lint when run=onSave.
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
	s.docs.delete(p.TextDocument.URI)
	// Clear diagnostics so VS Code stops showing stale squiggles.
	_ = s.t.writeNotification("textDocument/publishDiagnostics",
		publishDiagnosticsParams{URI: p.TextDocument.URI, Diagnostics: []Diagnostic{}})
}

func (s *Server) handleDidChangeWatchedFiles(ctx context.Context, raw json.RawMessage) {
	var p didChangeWatchedFilesParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return
	}
	relevant := false
	for _, c := range p.Changes {
		if strings.HasSuffix(uriToPath(c.URI), ".mdsmith.yml") {
			relevant = true
			break
		}
	}
	if !relevant {
		return
	}
	s.reloadConfig()
	for _, uri := range s.docs.openURIs() {
		s.scheduleLint(uri, lintTriggerConfig)
	}
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
	immediate := trigger != lintTriggerChange
	if immediate || s.debounce == 0 {
		s.runLint(uri)
		return
	}
	// Identity-checked replacement: the closure captures its own
	// timer handle and only removes it from the pending map if the
	// map still points at it. Without that check, a callback whose
	// firing raced with a fresh scheduleLint would delete the new
	// timer's map entry, breaking debouncing for the next change.
	s.pendingMu.Lock()
	if existing, ok := s.pending[uri]; ok {
		existing.Stop()
	}
	var timer *time.Timer
	timer = time.AfterFunc(s.debounce, func() {
		// Re-check shutdown so a timer that armed before
		// shutdown/exit but fires after them does not publish
		// stale diagnostics during teardown.
		if s.shutdown.Load() {
			return
		}
		s.pendingMu.Lock()
		if s.pending[uri] == timer {
			delete(s.pending, uri)
		}
		s.pendingMu.Unlock()
		s.runLint(uri)
	})
	s.pending[uri] = timer
	s.pendingMu.Unlock()
}

// stopPendingLints cancels every armed debounce timer. Called from
// the shutdown/exit handlers so we do not publish diagnostics after
// the client asked us to stop.
func (s *Server) stopPendingLints() {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	for uri, timer := range s.pending {
		timer.Stop()
		delete(s.pending, uri)
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
	cfg, _, root := s.snapshotConfig()
	if cfg == nil {
		cfg = config.Merge(config.Defaults(), nil)
	}
	relPath := workspaceRelative(root, doc.path)
	if config.IsIgnored(cfg.Ignore, relPath) {
		_ = s.t.writeNotification("textDocument/publishDiagnostics",
			publishDiagnosticsParams{URI: uri, Diagnostics: []Diagnostic{}})
		return
	}
	r := &engine.Runner{
		Config:           cfg,
		Rules:            s.rules,
		StripFrontMatter: frontMatterEnabled(cfg),
		RootDir:          root,
		MaxInputBytes:    lint.DefaultMaxInputBytes,
		SourceFS:         dirFSForPath(doc.path),
	}
	res := r.RunSource(relPath, doc.text)
	lspDiags := toLSPAll(res.Diagnostics, doc.text)
	_ = s.t.writeNotification("textDocument/publishDiagnostics",
		publishDiagnosticsParams{URI: uri, Diagnostics: lspDiags})
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
	if err != nil || strings.HasPrefix(rel, "..") {
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
		fixed, err := fixpkg.Source(fixpkg.SourceOptions{
			Config:           cfg,
			Rules:            s.rules,
			Path:             doc.path,
			Source:           doc.text,
			RootDir:          root,
			StripFrontMatter: frontMatterEnabled(cfg),
		})
		if err == nil && string(fixed) != string(doc.text) {
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
// `rule` over the buffer, or nil if the rule is not fixable, is
// whole-file-only, or its fix is a no-op against the current buffer.
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
	if isWholeFileOnly(rule) {
		return nil
	}
	fixed, err := fixpkg.SourceWithRules(fixpkg.SourceOptions{
		Config:           cfg,
		Rules:            s.rules,
		Path:             doc.path,
		Source:           doc.text,
		RootDir:          root,
		StripFrontMatter: frontMatterEnabled(cfg),
	}, []string{rule})
	if err != nil || string(fixed) == string(doc.text) {
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
// documentEndPosition(before), which is {lineCount, 0} for newline-
// terminated content and {lastLine, lastLineLen} otherwise. Sizing
// the range against `before` matches the LSP contract — clients
// apply a TextEdit by replacing the named range in the existing
// document.
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
// entire `source`. Trailing-newline-terminated files end at
// {Line: lineCount, Character: 0}; files without a trailing newline
// end at the last line's UTF-16 length. Empty input returns (0, 0).
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
	// No trailing newline: end at last line's UTF-16 length.
	lines := splitLines(source)
	if len(lines) == 0 {
		return 0, 0
	}
	last := string(lines[len(lines)-1])
	return len(lines) - 1, utf16Column(last, runeLen(last))
}

// snapshotConfig returns the cached config, its source path, and the
// project root. All return values may be empty when no config has
// been loaded yet.
func (s *Server) snapshotConfig() (*config.Config, string, string) {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	return s.config, s.configPath, s.rootDir
}

// reloadConfig walks from rootDir (or the user-supplied
// `mdsmith.config`) and refreshes the cached config.
func (s *Server) reloadConfig() {
	s.settingsMu.RLock()
	override := s.settings.ConfigPath
	s.settingsMu.RUnlock()

	s.configMu.Lock()
	defer s.configMu.Unlock()

	defaults := config.Defaults()
	if override != "" {
		path := override
		if !filepath.IsAbs(path) && s.rootDir != "" {
			path = filepath.Join(s.rootDir, path)
		}
		loaded, err := config.Load(path)
		if err != nil {
			s.config = config.Merge(defaults, nil)
			s.configPath = ""
			return
		}
		s.config = config.Merge(defaults, loaded)
		s.configPath = path
		return
	}
	if s.rootDir == "" {
		s.config = config.Merge(defaults, nil)
		s.configPath = ""
		return
	}
	discovered, err := config.Discover(s.rootDir)
	if err != nil || discovered == "" {
		s.config = config.Merge(defaults, nil)
		s.configPath = ""
		return
	}
	loaded, err := config.Load(discovered)
	if err != nil {
		s.config = config.Merge(defaults, nil)
		s.configPath = ""
		return
	}
	s.config = config.Merge(defaults, loaded)
	s.configPath = discovered
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
	const fetchTimeout = 2 * time.Second
	id := s.nextReqID.Add(1)
	idJSON, err := json.Marshal(id)
	if err != nil {
		return
	}
	ch := s.registerPendingResponse(string(idJSON))
	defer s.unregisterPendingResponse(string(idJSON))

	if err := s.t.writeRequest(idJSON, "workspace/configuration",
		configurationParams{Items: []configurationItem{{Section: "mdsmith"}}}); err != nil {
		return
	}

	select {
	case resp := <-ch:
		if resp.Error != nil || len(resp.Result) == 0 {
			return
		}
		// The result is an array (one entry per requested item). Our
		// single item ("mdsmith") yields a one-element array.
		var arr []userSettings
		if err := json.Unmarshal(resp.Result, &arr); err != nil || len(arr) == 0 {
			return
		}
		s.settingsMu.Lock()
		// Only overwrite values the client supplied — VS Code returns
		// `null` for unset entries, which Unmarshal turns into the
		// zero value, so we'd otherwise wipe defaults.
		next := arr[0]
		current := s.settings
		if next.ConfigPath != "" {
			current.ConfigPath = next.ConfigPath
		}
		if next.Run != "" {
			current.Run = next.Run
		}
		s.settings = current
		s.settingsMu.Unlock()
		// Reload config in case `mdsmith.config` changed, then
		// re-lint open buffers so diagnostics reflect the freshly
		// applied settings rather than whatever was in effect when
		// handleDidChangeConfiguration fired.
		s.reloadConfig()
		for _, uri := range s.docs.openURIs() {
			s.scheduleLint(uri, lintTriggerConfig)
		}
	case <-time.After(fetchTimeout):
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

// registerWatchers asks the client to watch the project's
// `.mdsmith.yml` and notify the server on change. Best-effort; clients
// that lack dynamic registration ignore this and the server falls
// back to the polled config.
func (s *Server) registerWatchers() {
	id := s.nextReqID.Add(1)
	idJSON, err := json.Marshal(id)
	if err != nil {
		return
	}
	_ = s.t.writeRequest(idJSON, "client/registerCapability",
		registrationParams{Registrations: []registration{{
			ID:     "mdsmith-watch",
			Method: "workspace/didChangeWatchedFiles",
			RegisterOptions: didChangeWatchedFilesRegistrationOptions{
				Watchers: []fileSystemWatcher{{GlobPattern: "**/.mdsmith.yml"}},
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

// isWholeFileOnly excludes rules whose fix touches multiple
// non-contiguous ranges from per-diagnostic quick fixes.
// catalog/include/toc rewrite a whole generated section; surfacing
// them as quick fixes invites partial regenerations.
func isWholeFileOnly(name string) bool {
	switch name {
	case "catalog", "toc", "toc-directive", "include":
		return true
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
	if !strings.HasPrefix(uri, "file://") {
		return ""
	}
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	if u.Scheme != "file" {
		return ""
	}
	host := u.Host
	if strings.EqualFold(host, "localhost") {
		host = ""
	}
	p := u.Path
	if host != "" {
		// UNC path on Windows: file://server/share/path → \\server\share\path
		if runtime.GOOS == "windows" {
			return filepath.Clean(`\\` + host + filepath.FromSlash(p))
		}
		// Non-Windows: we cannot resolve a remote share, so refuse.
		return ""
	}
	// Windows: file:///C:/foo decodes to "/C:/foo"; strip the leading slash.
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.Clean(p)
}

// pickRoot derives the workspace root from initialize params.
func pickRoot(p initializeParams) string {
	if len(p.WorkspaceFolders) > 0 {
		if path := uriToPath(p.WorkspaceFolders[0].URI); path != "" {
			return path
		}
	}
	if path := uriToPath(p.RootURI); path != "" {
		return path
	}
	return ""
}
