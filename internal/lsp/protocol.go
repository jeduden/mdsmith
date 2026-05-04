// Package lsp implements a minimal Language Server Protocol surface for
// mdsmith. It speaks JSON-RPC 2.0 over stdio and handles only the methods
// the VS Code extension needs: lifecycle, document sync, diagnostics,
// code actions, and watched-file notifications.
package lsp

import "encoding/json"

// JSON-RPC 2.0 framing.

// requestMessage is an incoming JSON-RPC request or notification. The ID
// is absent on notifications.
type requestMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// responseMessage is an outgoing reply to a client request. The
// result field is always emitted (as `null` when no payload) on
// success; on error, only the error field appears. JSON-RPC 2.0
// forbids both fields appearing together, so the writer in
// transport.go takes the success-vs-error branch up front.
type responseMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *responseError  `json:"error,omitempty"`
}

// notificationMessage is an outgoing notification (no id, no reply expected).
type notificationMessage struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type responseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// JSON-RPC error codes.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
)

// LSP types — only the subset the server actually emits or consumes.

type initializeParams struct {
	ProcessID        int                `json:"processId,omitempty"`
	RootURI          string             `json:"rootUri,omitempty"`
	WorkspaceFolders []workspaceFolder  `json:"workspaceFolders,omitempty"`
	Capabilities     clientCapabilities `json:"capabilities"`
}

type workspaceFolder struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

type clientCapabilities struct {
	Workspace *workspaceClientCapabilities `json:"workspace,omitempty"`
}

type workspaceClientCapabilities struct {
	Configuration         bool `json:"configuration,omitempty"`
	DidChangeWatchedFiles *struct {
		DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
	} `json:"didChangeWatchedFiles,omitempty"`
}

type initializeResult struct {
	Capabilities serverCapabilities `json:"capabilities"`
	ServerInfo   serverInfo         `json:"serverInfo"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type serverCapabilities struct {
	TextDocumentSync   textDocumentSyncOptions `json:"textDocumentSync"`
	CodeActionProvider codeActionOptions       `json:"codeActionProvider"`
}

// textDocumentSyncKind is the LSP enum for change notification mode.
//
//nolint:unused // referenced via the typed numeric constants below
type textDocumentSyncKind int

const (
	syncFull textDocumentSyncKind = 1
)

type textDocumentSyncOptions struct {
	OpenClose bool                 `json:"openClose"`
	Change    textDocumentSyncKind `json:"change"`
	Save      *saveOptions         `json:"save,omitempty"`
}

type saveOptions struct {
	IncludeText bool `json:"includeText,omitempty"`
}

type codeActionOptions struct {
	CodeActionKinds []string `json:"codeActionKinds,omitempty"`
}

// Position is a 0-based location within a text document. Line and
// Character are zero-indexed; Character counts UTF-16 code units, per
// the LSP spec.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range covers a span between two Positions; End is exclusive.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// DiagnosticSeverity values mirror the LSP enum.
type DiagnosticSeverity int

const (
	severityError   DiagnosticSeverity = 1
	severityWarning DiagnosticSeverity = 2
)

// Diagnostic is the LSP wire shape produced by the server.
type Diagnostic struct {
	Range    Range              `json:"range"`
	Severity DiagnosticSeverity `json:"severity,omitempty"`
	Code     string             `json:"code,omitempty"`
	Source   string             `json:"source,omitempty"`
	Message  string             `json:"message"`
	Data     *diagnosticData    `json:"data,omitempty"`
}

// diagnosticData carries the rule name through to code-action handlers.
// LSP allows arbitrary `data` on diagnostics; clients echo it back on
// codeAction requests, which is exactly what we need to know which
// rule's fix to run for a given diagnostic.
type diagnosticData struct {
	RuleName string `json:"rule"`
}

type publishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

type didOpenTextDocumentParams struct {
	TextDocument textDocumentItem `json:"textDocument"`
}

type textDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type didChangeTextDocumentParams struct {
	TextDocument   versionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []textDocumentContentChangeEvent `json:"contentChanges"`
}

type versionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

type textDocumentContentChangeEvent struct {
	Text string `json:"text"`
}

type didCloseTextDocumentParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

type textDocumentIdentifier struct {
	URI string `json:"uri"`
}

type codeActionParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Range        Range                  `json:"range"`
	Context      codeActionContext      `json:"context"`
}

type codeActionContext struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
	Only        []string     `json:"only,omitempty"`
}

// Code action kinds — match the strings VS Code expects.
const (
	kindQuickFix         = "quickfix"
	kindSourceFixAll     = "source.fixAll.mdsmith"
	titleFixAllMdsmith   = "Fix all mdsmith issues"
	titleQuickFixPattern = "Fix %s with mdsmith"
)

// codeAction is what the server returns from textDocument/codeAction.
type codeAction struct {
	Title       string         `json:"title"`
	Kind        string         `json:"kind,omitempty"`
	Diagnostics []Diagnostic   `json:"diagnostics,omitempty"`
	Edit        *workspaceEdit `json:"edit,omitempty"`
}

type workspaceEdit struct {
	Changes map[string][]textEdit `json:"changes"`
}

type textEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

type didChangeWatchedFilesParams struct {
	Changes []fileEvent `json:"changes"`
}

type fileEvent struct {
	URI  string `json:"uri"`
	Type int    `json:"type"`
}

type configurationParams struct {
	Items []configurationItem `json:"items"`
}

type configurationItem struct {
	ScopeURI string `json:"scopeUri,omitempty"`
	Section  string `json:"section,omitempty"`
}

type registrationParams struct {
	Registrations []registration `json:"registrations"`
}

type registration struct {
	ID              string `json:"id"`
	Method          string `json:"method"`
	RegisterOptions any    `json:"registerOptions,omitempty"`
}

type didChangeWatchedFilesRegistrationOptions struct {
	Watchers []fileSystemWatcher `json:"watchers"`
}

type fileSystemWatcher struct {
	GlobPattern string `json:"globPattern"`
}
