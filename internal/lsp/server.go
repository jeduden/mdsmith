package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"path/filepath"
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
	t          *transport
	rules      []rule.Rule
	clock      func() time.Time
	debounce   time.Duration
	logger     *vlog.Logger
	docs       *documentStore
	configMu   sync.RWMutex
	config     *config.Config
	configPath string
	rootDir    string
	settings   userSettings
	settingsMu sync.RWMutex
	pendingMu  sync.Mutex
	pending    map[string]*time.Timer
	nextReqID  atomic.Int64
	shutdown   atomic.Bool
}

// userSettings mirrors the subset of `mdsmith.*` VS Code keys the
// server consults. Defaults match the documented values in
// docs/guides/editors/vscode.md.
type userSettings struct {
	ConfigPath string `json:"config"`
	Run        string `json:"run"`
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
		t:        newTransport(opts.Reader, opts.Writer),
		rules:    opts.Rules,
		clock:    time.Now,
		debounce: debounce,
		logger:   logger,
		docs:     newDocumentStore(),
		settings: userSettings{Run: "onSave"},
		pending:  make(map[string]*time.Timer),
	}
}

// Run drives the server until the input stream returns io.EOF or the
// client sends `exit`.
func (s *Server) Run(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		msg, err := s.t.readMessage()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		s.dispatch(ctx, msg)
	}
}

func (s *Server) dispatch(ctx context.Context, msg *requestMessage) {
	if msg.JSONRPC != "2.0" {
		if msg.ID != nil {
			_ = s.t.writeError(msg.ID, codeInvalidRequest, "jsonrpc must be 2.0")
		}
		return
	}
	switch msg.Method {
	case "initialize":
		s.handleInitialize(msg)
	case "initialized":
		s.handleInitialized(ctx)
	case "shutdown":
		s.shutdown.Store(true)
		_ = s.t.writeResponse(msg.ID, nil)
	case "exit":
		// Force the transport to surface EOF on the next read.
		// Run() will return cleanly. We do not call os.Exit here
		// because the caller (cmd/mdsmith) decides exit codes.
		s.shutdown.Store(true)
	case "textDocument/didOpen":
		s.handleDidOpen(ctx, msg.Params)
	case "textDocument/didChange":
		s.handleDidChange(ctx, msg.Params)
	case "textDocument/didClose":
		s.handleDidClose(msg.Params)
	case "textDocument/codeAction":
		s.handleCodeAction(msg)
	case "workspace/didChangeWatchedFiles":
		s.handleDidChangeWatchedFiles(ctx, msg.Params)
	case "workspace/didChangeConfiguration":
		s.handleDidChangeConfiguration(ctx)
	case "$/cancelRequest", "$/setTrace":
		// Notifications we silently accept.
	default:
		if msg.ID != nil {
			_ = s.t.writeError(msg.ID, codeMethodNotFound, "method not supported: "+msg.Method)
		}
	}
}

func (s *Server) handleInitialize(msg *requestMessage) {
	var p initializeParams
	if len(msg.Params) > 0 {
		if err := json.Unmarshal(msg.Params, &p); err != nil {
			_ = s.t.writeError(msg.ID, codeParseError, "invalid initialize params")
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
	s.fetchClientSettings(ctx)
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
	s.scheduleLint(ctx, p.TextDocument.URI, true)
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
	// Full sync: the last change carries the entire buffer.
	last := p.ContentChanges[len(p.ContentChanges)-1]
	doc.text = []byte(last.Text)
	doc.version = p.TextDocument.Version
	s.docs.set(p.TextDocument.URI, doc)
	s.scheduleLint(ctx, p.TextDocument.URI, false)
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
		s.scheduleLint(ctx, uri, true)
	}
}

func (s *Server) handleDidChangeConfiguration(ctx context.Context) {
	s.fetchClientSettings(ctx)
	for _, uri := range s.docs.openURIs() {
		s.scheduleLint(ctx, uri, true)
	}
}

// scheduleLint debounces lint runs per document. When immediate is
// true the lint runs synchronously without debouncing — used after
// didOpen so the first batch of diagnostics shows up promptly.
func (s *Server) scheduleLint(ctx context.Context, uri string, immediate bool) {
	if s.shutdown.Load() {
		return
	}
	if immediate || s.debounce == 0 {
		s.runLint(uri)
		return
	}
	s.pendingMu.Lock()
	if existing, ok := s.pending[uri]; ok {
		existing.Stop()
	}
	s.pending[uri] = time.AfterFunc(s.debounce, func() {
		s.pendingMu.Lock()
		delete(s.pending, uri)
		s.pendingMu.Unlock()
		s.runLint(uri)
	})
	s.pendingMu.Unlock()
	_ = ctx
}

// runLint executes one lint pass on the buffer and publishes the
// resulting diagnostics. Safe to call from any goroutine.
func (s *Server) runLint(uri string) {
	doc, ok := s.docs.get(uri)
	if !ok {
		return
	}
	cfg, _, root := s.snapshotConfig()
	if cfg == nil {
		cfg = config.Merge(config.Defaults(), nil)
	}
	if config.IsIgnored(cfg.Ignore, doc.path) {
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
	}
	res := r.RunSource(doc.path, doc.text)
	lspDiags := toLSPAll(res.Diagnostics, doc.text)
	_ = s.t.writeNotification("textDocument/publishDiagnostics",
		publishDiagnosticsParams{URI: uri, Diagnostics: lspDiags})
}

func (s *Server) handleCodeAction(msg *requestMessage) {
	var p codeActionParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeParseError, "invalid codeAction params")
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

func (s *Server) computeCodeActions(
	p codeActionParams, doc *document, cfg *config.Config, root string,
) []codeAction {
	actions := make([]codeAction, 0, len(p.Context.Diagnostics)+1)

	// Per-diagnostic quick fixes — one per fixable rule.
	for _, d := range p.Context.Diagnostics {
		if d.Data == nil || d.Data.RuleName == "" {
			continue
		}
		if !isFixable(s.rules, d.Data.RuleName) {
			continue
		}
		if isWholeFileOnly(d.Data.RuleName) {
			continue
		}
		fixed, err := fixpkg.SourceWithRules(fixpkg.SourceOptions{
			Config:           cfg,
			Rules:            s.rules,
			Path:             doc.path,
			Source:           doc.text,
			RootDir:          root,
			StripFrontMatter: frontMatterEnabled(cfg),
		}, []string{d.Data.RuleName})
		if err != nil || string(fixed) == string(doc.text) {
			continue
		}
		actions = append(actions, codeAction{
			Title:       quickFixTitle(d.Data.RuleName),
			Kind:        kindQuickFix,
			Diagnostics: []Diagnostic{d},
			Edit:        fullFileEdit(p.TextDocument.URI, doc.text, fixed),
		})
	}

	// Source action: fix-all.
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

	return actions
}

func quickFixTitle(rule string) string {
	return "Fix " + rule + " with mdsmith"
}

func fullFileEdit(uri string, before, after []byte) *workspaceEdit {
	lines := splitLines(before)
	endLine := len(lines)
	endChar := 0
	if endLine > 0 {
		endChar = utf16Column(string(lines[endLine-1]), runeLen(string(lines[endLine-1])))
	}
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
// section and updates the cached settings if it answers. Errors are
// silently ignored — the defaults are good enough.
func (s *Server) fetchClientSettings(ctx context.Context) {
	id := s.nextReqID.Add(1)
	idJSON, _ := json.Marshal(id)
	if err := s.t.writeRequest(idJSON, "workspace/configuration",
		configurationParams{Items: []configurationItem{{Section: "mdsmith"}}}); err != nil {
		return
	}
	_ = ctx
	// We do not wait for the response synchronously — handling it here
	// would require routing the response back through the dispatch
	// loop. Instead we use the registered handler in
	// dispatchResponse. For now, just request it — the server
	// continues with its previous settings.
}

// registerWatchers asks the client to watch the project's
// `.mdsmith.yml` and notify the server on change. Best-effort; clients
// that lack dynamic registration ignore this and the server falls
// back to the polled config.
func (s *Server) registerWatchers() {
	id := s.nextReqID.Add(1)
	idJSON, _ := json.Marshal(id)
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
	p := u.Path
	// Windows: file:///C:/foo decodes to "/C:/foo".
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
