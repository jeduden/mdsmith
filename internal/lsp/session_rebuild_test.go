package lsp

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/config"
	vlog "github.com/jeduden/mdsmith/internal/log"
	"github.com/jeduden/mdsmith/internal/rule"
	mdsmith "github.com/jeduden/mdsmith/pkg/mdsmith"

	_ "github.com/jeduden/mdsmith/internal/rules/all"
)

// failingSessionServer returns a Server whose session constructor always
// fails, so rebuildSession takes its error branch and currentSession
// returns a nil session — the precondition the nil-session guards across
// the LSP defend against. Production cannot reach this (a compiled config
// source never fails to load), so the seam is the only red/green driver.
func failingSessionServer(t *testing.T, w io.Writer) *Server {
	t.Helper()
	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All(),
		Logger: &vlog.Logger{Enabled: true, W: w}})
	s.newSession = func(mdsmith.SessionOptions) (*mdsmith.Session, error) {
		return nil, errors.New("injected session-build failure")
	}
	return s
}

// TestRebuildSessionLogsAndKeepsNilSessionOnError covers rebuildSession's
// error branch: when the session constructor fails, the server logs the
// failure and leaves s.session nil rather than swapping in a half-built
// one. currentSession then returns nil, which every lint/fix path guards.
func TestRebuildSessionLogsAndKeepsNilSessionOnError(t *testing.T) {
	t.Parallel()
	var buf strings.Builder
	s := failingSessionServer(t, &buf)

	s.rebuildSession(config.Merge(config.Defaults(), nil), "")

	s.sessionMu.RLock()
	sess := s.session
	s.sessionMu.RUnlock()
	if sess != nil {
		t.Fatal("rebuildSession: session must stay nil after a build failure")
	}
	if !strings.Contains(buf.String(), "rebuild failed") {
		t.Fatalf("rebuildSession: expected a 'rebuild failed' log, got %q", buf.String())
	}

	// currentSession re-attempts the (still-failing) build and returns nil.
	if got, _ := s.currentSession(); got != nil {
		t.Fatal("currentSession: expected nil when every rebuild fails")
	}
}

// TestSyncBufferNoSessionIsNoOp covers syncBuffer's guard: with no
// session it returns without touching an overlay, so a buffer edit that
// arrives before the session exists cannot panic.
func TestSyncBufferNoSessionIsNoOp(t *testing.T) {
	t.Parallel()
	s := failingSessionServer(t, io.Discard)
	// Must not panic and must not block: the guard returns immediately.
	s.syncBuffer("/abs/x.md", []byte("# buffer\n"))
	// The empty-absPath arm of the same guard.
	s.syncBuffer("", []byte("# buffer\n"))
}

// TestDropPathNoSessionIsNoOp covers dropPath's guard: with no session it
// returns without dropping caches, mirroring syncBuffer.
func TestDropPathNoSessionIsNoOp(t *testing.T) {
	t.Parallel()
	s := failingSessionServer(t, io.Discard)
	s.dropPath("/abs/x.md")
	s.dropPath("") // empty-absPath arm
}

// TestRunLintNoSessionPublishesNothing covers runLint's nil-session
// guard: a lint scheduled before any session exists publishes no
// diagnostics instead of dereferencing a nil session.
func TestRunLintNoSessionPublishesNothing(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	s.newSession = func(mdsmith.SessionOptions) (*mdsmith.Session, error) {
		return nil, errors.New("injected session-build failure")
	}
	s.docs.set("file:///x.md", &document{
		uri: "file:///x.md", path: "x.md", text: []byte("# Hi\n\ndirty   \n"),
	})

	s.runLint("file:///x.md")
	assert.NotContains(t, buf.String(), "publishDiagnostics",
		"a lint with no session must not publish diagnostics")
}

// TestRunLintDiscardsResultsWhenDocClosedMidLint covers runLint's
// "document was closed while we were linting" guard: didClose can land
// after the Check has started but before the publish. The afterLintCheck
// seam deletes the document at exactly that point; runLint must then
// discard the results so it does not re-publish stale squiggles over
// didClose's empty notification.
func TestRunLintDiscardsResultsWhenDocClosedMidLint(t *testing.T) {
	t.Parallel()
	var buf safeBuffer
	s := New(Options{Reader: nil, Writer: &buf, Rules: rule.All()})
	const uri = "file:///x.md"
	s.docs.set(uri, &document{
		uri: uri, path: "x.md", text: []byte("# Hi\n\ndirty   \n"),
	})
	// Simulate didClose racing in after the Check returns: drop the doc
	// before runLint re-checks docs.get below the Check.
	s.afterLintCheck = func() { s.docs.delete(uri) }

	s.runLint(uri)
	assert.NotContains(t, buf.String(), "publishDiagnostics",
		"results for a document closed mid-lint must be discarded")
}

// TestAppendFixAllActionNoSessionReturnsActions covers appendFixAllAction's
// nil-session guard: a source.fixAll request with no session yet returns
// the actions list unchanged (no fix-all entry) instead of panicking.
func TestAppendFixAllActionNoSessionReturnsActions(t *testing.T) {
	t.Parallel()
	s := failingSessionServer(t, io.Discard)
	cfg := config.Merge(config.Defaults(), nil)
	doc := &document{path: "x.md", text: []byte("# Hi\n\ndirty   \n")}
	p := codeActionParams{
		TextDocument: textDocumentIdentifier{URI: "file:///x.md"},
		Context:      codeActionContext{Only: []string{kindSourceFixAll}},
	}

	actions := s.computeCodeActions(p, doc, cfg, "")
	assert.Empty(t, actions,
		"fix-all with no session must surface no action (guarded), even on a fixable buffer")
}

// TestQuickFixBytesForNoSessionReturnsNil covers quickFixBytesFor's
// nil-session guard: a per-rule quick-fix with no session returns nil
// (no action) rather than dereferencing the nil session, even though the
// buffer has a fixable violation the rule owns.
func TestQuickFixBytesForNoSessionReturnsNil(t *testing.T) {
	t.Parallel()
	s := failingSessionServer(t, io.Discard)
	cfg := config.Merge(config.Defaults(), nil)
	// A real trailing-spaces violation: with a session this would fix.
	doc := &document{path: "x.md", text: []byte("# Hi\n\ndirty   \n")}

	got := s.quickFixBytesFor("no-trailing-spaces", doc, cfg, "")
	require.Nil(t, got,
		"quick-fix with no session must return nil so no broken edit is offered")
}

// serverWithSession builds a Server whose per-workspace session is
// already constructed against an on-disk config under dir, mirroring the
// post-handleInitialized state. It returns the server so a test can hold
// the live session via currentSession() and drive concurrent reloads.
func serverWithSession(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"), []byte("{}\n"), 0o600))

	s := New(Options{Reader: nil, Writer: io.Discard, Rules: rule.All()})
	s.configMu.Lock()
	s.rootDir = dir
	s.configMu.Unlock()
	s.reloadConfig() // builds the first session
	return s
}

// TestRebuildSessionDoesNotDisposeHeldSession is the use-after-dispose
// regression guard. A lint/fix goroutine obtains a session from
// currentSession(); a concurrent reloadConfig (config or
// didChangeWatchedFiles change) then swaps in a new session. The swap
// must NOT Dispose the superseded session, because the held goroutine is
// still using it: Dispose nils its checkCache under lock, so the held
// session's Check would lose its warm cache (and, without the engine's
// nil-map guard, would panic on the next write). Run under -race.
//
// The held session is exercised through Check (which writes checkCache),
// Fix (which routes through Check), and CheckVersion (the per-keystroke
// path) while reloadConfig hammers rebuildSession in parallel. The
// assertions: no panic/race, and the held session keeps returning the
// correct diagnostic for a known-dirty buffer the whole time.
func TestRebuildSessionDoesNotDisposeHeldSession(t *testing.T) {
	t.Parallel()
	s := serverWithSession(t)

	held, _ := s.currentSession()
	require.NotNil(t, held, "reloadConfig must have built a session")

	// A buffer with a trailing-space violation (MDS006) and a long line
	// (MDS001): every Check on it must keep reporting the same findings,
	// proving the held session stays a working linter across the swap.
	dirty := []byte("# Title\n\ntrailing line here   \n")

	// Warm the held session's checkCache so a concurrent Dispose (the bug)
	// would visibly nil a populated cache, not an empty one.
	if _, err := held.Check("doc.md", dirty); err != nil {
		t.Fatalf("warm Check: %v", err)
	}

	const iters = 200
	var wg sync.WaitGroup

	// Reloader: repeatedly rebuild the session under sessionMu. Before the
	// fix this called old.Dispose() on the held session.
	wg.Add(1)
	go func() {
		defer wg.Done()
		cfg, cfgPath, _ := s.snapshotConfig()
		for i := 0; i < iters; i++ {
			s.rebuildSession(cfg, cfgPath)
		}
	}()

	// Users of the HELD session: Check / Fix / CheckVersion must keep
	// working and keep finding MDS006 while the swap races underneath.
	check := func(do func() bool) {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			if !do() {
				t.Errorf("held session lost its MDS006 finding mid-reload")
				return
			}
		}
	}
	wg.Add(3)
	go check(func() bool {
		diags, err := held.Check("doc.md", dirty)
		return err == nil && hasPublicRule(diags, "MDS006")
	})
	go check(func() bool {
		res, err := held.Fix("doc.md", dirty)
		// MDS006 (trailing spaces) is fixable, so a working Fix removes it:
		// the source must change and the trailing run must be gone. This
		// proves Fix re-linted through the held session without panic.
		return err == nil && res.Changed && !strings.Contains(res.Source, "here   ")
	})
	go check(func() bool {
		res := held.CheckVersion("doc.md", dirty, 1)
		for _, d := range res.Diagnostics {
			if d.RuleID == "MDS006" {
				return true
			}
		}
		return false
	})

	wg.Wait()

	// After the storm, the held session must still cache and lint
	// correctly — a Disposed session would have a dead checkCache.
	diags, err := held.Check("doc.md", dirty)
	require.NoError(t, err, "held session unusable after concurrent reloads")
	assert.True(t, hasPublicRule(diags, "MDS006"),
		"held session must still report MDS006 after the reload storm")
}

// hasPublicRule reports whether any public diagnostic carries ruleID.
func hasPublicRule(diags []mdsmith.Diagnostic, ruleID string) bool {
	for _, d := range diags {
		if d.Rule == ruleID {
			return true
		}
	}
	return false
}
