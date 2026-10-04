package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/jeduden/mdsmith/internal/config"
	mdsmith "github.com/jeduden/mdsmith/pkg/mdsmith"
)

// rebuildSession constructs a fresh per-workspace Session over an
// OverlayWorkspace rooted at the effective project root and re-seeds the
// new overlay with every open buffer so cross-file rules keep reading
// unsaved bytes across the rebuild. The superseded session is retired,
// not disposed: it is disposed once no caller holds it (see
// sessionLease and the note at the end of the function body). cfg is
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
	old := s.sessionLease
	s.session = sess
	s.workspace = ws
	s.sessionRoot = root
	s.sessionLease = &sessionLease{sess: sess, ws: ws}
	s.sessionMu.Unlock()
	// Retire the superseded session rather than Dispose it here: a
	// lint/fix goroutine may still hold it (from currentSession() before
	// this swap), and closing its lent root and overlay disk root under
	// that lint would turn its cross-file reads into false findings. The
	// lease disposes it once the last holder releases it.
	if old != nil {
		old.retire()
	}
}

// currentSession returns the active session and a release the caller
// calls once it is done with it, building a session on demand if none
// exists yet. Until released, a rebuild does not close the session's
// disk handles (see sessionLease).
// reloadConfig (from handleInitialized) builds the session eagerly for
// the normal path; this lazy fallback covers a client that lints after
// only `initialize` -- there the session must still exist, with whatever
// config snapshotConfig holds (defaults when none was discovered),
// matching the pre-session behaviour where runLint linted against
// default config.
func (s *Server) currentSession() (*mdsmith.Session, func()) {
	if sess, release := s.leaseSession("", true); sess != nil {
		return sess, release
	}
	cfg, cfgPath, _ := s.snapshotConfig()
	if cfg == nil {
		cfg = config.Merge(config.Defaults(), nil)
	}
	s.rebuildSession(cfg, cfgPath)
	return s.leaseSession("", true)
}

// leaseSession returns the current session and its release under the
// session lock, or nil and a no-op release when there is none or,
// unless anyRoot, when the session was built at a root other than root.
func (s *Server) leaseSession(root string, anyRoot bool) (*mdsmith.Session, func()) {
	s.sessionMu.RLock()
	defer s.sessionMu.RUnlock()
	if s.session == nil || (!anyRoot && s.sessionRoot != root) {
		return nil, noRelease
	}
	return s.session, s.sessionLease.acquire()
}

// invalidateWikilinks drops the current session's cached wikilink
// index, building a session on demand as every reader does.
func (s *Server) invalidateWikilinks() {
	sess, release := s.currentSession()
	defer release()
	if sess != nil {
		sess.InvalidateWikilinks()
	}
}

// sessionAt returns the current session and its release when it was
// built at root, or nil and a no-op release when there is none or a
// reload rebuilt it at another root, so a reader of the session's
// root-keyed caches never gets another directory's answer.
func (s *Server) sessionAt(root string) (*mdsmith.Session, func()) {
	_, release := s.currentSession() // build one on demand, as every reader does
	release()
	return s.leaseSession(root, false)
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
// either the cached settings or the open buffers -- the previous
// values stand.
//
// Must be called from a goroutine other than the dispatch loop, since
// the response arrives on the same loop.
func (s *Server) fetchClientSettings(ctx context.Context) {
	// This runs on its own goroutine (never the dispatch loop), so a
	// panic in the response path — config load, schema compile,
	// session rebuild, the host's OnConfigReload callback — would
	// kill the whole server without this recover, the same crash
	// class the lint and dispatch recovers contain.
	defer s.recoverPanic("fetch client settings")
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
	// change -- not catastrophic, but avoidable. Stop releases the
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
		// setting -- propagate it so the user can revert
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
// requester registered. Unknown ids are silently dropped -- the client
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
