package lsp

import (
	"fmt"
	"time"

	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
)

// Lint scheduling and the diagnostics-push pipeline: the debounce
// machinery (scheduleLint / pendingLint / runLintIfCurrent /
// stopPendingLints), the run-mode gate, the clear-on-off path, and
// runLint itself with its diagnostic-routing helpers. Split out of
// server.go so the diagnostics-push dispatch group owns its own file.
// (The LSP wire-shape conversion lives in diagnostics.go.)

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
		return runOnType
	}
}

// clearOpenDiagnostics drops every published diagnostic for open
// documents and asks the client to remove the squiggles. Used when
// mdsmith.run flips to off: scheduleLint publishes nothing in off
// mode, so diagnostics shown before the switch would otherwise linger
// until the buffer is closed. Any armed debounce timer is cancelled
// first so a lint scheduled just before the switch cannot re-publish
// after the clear.
func (s *Server) clearOpenDiagnostics() {
	for _, uri := range s.docs.openURIs() {
		// Collect the pending timer under pendingMu, then Stop outside
		// the lock — Stop hits the runtime timer heap and can block,
		// and holding pendingMu across it would serialize scheduleLint.
		s.pendingMu.Lock()
		pending, hadPending := s.pending[uri]
		if hadPending {
			delete(s.pending, uri)
		}
		s.pendingMu.Unlock()
		if hadPending && pending.timer != nil {
			pending.timer.Stop()
		}
		version := 0
		if doc, ok := s.docs.get(uri); ok {
			version = doc.version
		}
		// Delete and publish the empty set under diagsMu so this clear
		// serializes with runLint's mode-checked publish (which holds
		// the same lock across its check and write). Once mdsmith.run is
		// off, an in-flight lint either skips publishing or is ordered
		// before this clear — so the empty set is always the last word.
		s.diagsMu.Lock()
		delete(s.diags, uri)
		_ = s.t.writeNotification("textDocument/publishDiagnostics",
			publishDiagnosticsParams{URI: uri, Version: version, Diagnostics: []Diagnostic{}})
		s.diagsMu.Unlock()
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
		s.diagsMu.Lock()
		s.diags[uri] = nil
		s.diagsMu.Unlock()
		_ = s.t.writeNotification("textDocument/publishDiagnostics",
			publishDiagnosticsParams{URI: uri, Version: doc.version, Diagnostics: []Diagnostic{}})
		return
	}
	// Route the lint through the per-workspace Session. It owns the
	// cross-file read cache and the version-keyed parse cache, and reads
	// neighbouring files through its OverlayWorkspace — so the open
	// buffer this lint is about (pushed into the overlay by didOpen /
	// didChange) and every other open buffer reach cross-file rules.
	// CheckVersion serves the cached parse when (relPath, doc.version)
	// is already parsed, holding the latency gate.
	sess, _ := s.currentSession()
	if sess == nil {
		// No session yet (reloadConfig has not run): nothing to lint
		// against. handleInitialized builds the first session before any
		// document event, so this only guards a pre-init race.
		return
	}
	res := sess.CheckVersion(relPath, doc.text, doc.version)
	if s.afterLintCheck != nil {
		// Test seam: deterministically interpose a concurrent didClose /
		// shutdown between the Check and the publish below.
		s.afterLintCheck()
	}
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
	lspDiags := toLSPAll(docDiags, doc.text, root)
	// Cache and publish under diagsMu, re-checking the run mode inside
	// the lock. mdsmith.run can flip to off while the CPU-bound
	// RunSource above is in flight; clearOpenDiagnostics deletes and
	// publishes the empty set under the same lock when that happens.
	// Holding diagsMu across the check, the cache, and the wire write
	// serializes the two: an in-flight lint either sees off and
	// publishes nothing, or publishes under the lock before the clear —
	// whose empty publish is then ordered last — so off stays a true
	// master switch with no stale squiggles. Caching before the write
	// also keeps hover consistent with what the client just received.
	s.diagsMu.Lock()
	defer s.diagsMu.Unlock()
	if s.runMode() == runOff {
		return
	}
	s.diags[uri] = lspDiags
	_ = s.t.writeNotification("textDocument/publishDiagnostics",
		publishDiagnosticsParams{URI: uri, Version: doc.version, Diagnostics: lspDiags})
}

// resolveMaxInputBytes mirrors cmd/mdsmith's resolution of the
// project's `max-input-size`: unset (empty string) → default cap,
// "0" → unlimited, otherwise the parsed byte count. Parse errors
// fall back to the default and are surfaced via window/logMessage
// so the editor user can correct the config.
//
// The session resolves its own byte cap from the same config field, so
// rebuildSession calls this once per reload purely to surface the
// editor-facing warning on a malformed value — the returned cap is the
// session's job.
func (s *Server) resolveMaxInputBytes(cfg *config.Config) int64 {
	raw := ""
	if cfg != nil {
		raw = cfg.MaxInputSize
	}
	if raw == "" {
		return bytelimit.DefaultMaxInputBytes
	}
	n, err := config.ParseSize(raw)
	if err != nil {
		s.logger.Printf("config: invalid max-input-size %q: %v", raw, err)
		_ = s.t.writeNotification("window/logMessage", logMessageParams{
			Type:    messageTypeError,
			Message: fmt.Sprintf("mdsmith: invalid max-input-size %q: %v", raw, err),
		})
		return bytelimit.DefaultMaxInputBytes
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
