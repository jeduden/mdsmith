package lsp

import (
	"context"
	"encoding/json"
)

// LSP lifecycle handlers: initialize, initialized, and the dynamic
// file-watcher registration that follows initialized. Split out of
// server.go so the lifecycle dispatch group owns its own file, matching
// the one-group-per-file layout the navigation surface already uses
// (rename.go, completion.go).

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

	// Honor the LSP processId watchdog: exit if the editor that launched
	// us goes away. Prevents an orphaned server from outliving an editor
	// update/reload/crash and racing the freshly-spawned one.
	s.startParentWatch(p.ProcessID)

	// Newest-wins workspace singleton: claim this workspace and step
	// aside if a newer server later claims it. Backstops the processId
	// watchdog for the case it can't see — a leaked editor host that
	// stays alive, holding our stdin open so no EOF arrives, while its
	// window is gone.
	s.startSingletonWatch(root)

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
