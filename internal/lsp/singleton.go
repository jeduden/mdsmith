package lsp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// randRead and userCacheDir indirect the crypto/rand and os entry
// points the registry depends on, so their otherwise-unreachable
// failure branches can be driven red/green in tests. Production points
// them at the real calls.
var (
	randRead     = rand.Read
	userCacheDir = os.UserCacheDir
	// osExit is the process-exit seam shared by the parent-process and
	// workspace-singleton watchdogs (see server.go). Overriding it lets
	// tests exercise the default exit closures without terminating the
	// test binary.
	osExit = os.Exit
)

// singletonPollInterval is how often the workspace-singleton watcher
// re-reads the registry to see whether a newer server has claimed the
// same workspace. 5s converges within a few seconds of a reload while
// keeping the per-tick cost (one small file read) negligible.
const singletonPollInterval = 5 * time.Second

// watchSingleton polls current(key) every interval and calls
// onSuperseded the first time the workspace's recorded owner is a
// different, non-empty instance — meaning a newer server claimed this
// workspace and this one should step aside. It returns without calling
// onSuperseded if ctx is canceled first (a normal shutdown). An empty
// owner (registry unreadable or never written) is treated as "still
// ours": we never step the last server aside on a transient read miss.
// Splitting the loop from the registry probe keeps it unit-testable
// with a fake current, mirroring watchParentProcess.
func watchSingleton(
	ctx context.Context,
	key, instanceID string,
	interval time.Duration,
	current func(string) string,
	onSuperseded func(),
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if owner := current(key); owner != "" && owner != instanceID {
				onSuperseded()
				return
			}
		}
	}
}

// startSingletonWatch claims the workspace for this server (newest wins)
// and launches the watcher that steps it aside once a newer server
// claims the same workspace. It is the spec-silent companion to the
// processId watchdog: the watchdog reaps a server whose editor host has
// *died*, while this reaps one whose host is still *alive but orphaned*
// — a leaked extension host that survives its own window, keeps the
// server's stdin pipe open (so no EOF) and registers as alive (so the
// watchdog stays quiet), then races the freshly-spawned server.
//
// The singleton is opt-in per client. scope is the client's
// initializationOptions.mdsmith.singletonScope token; the owner record
// is keyed on root plus scope, so only servers sharing a scope (the VS
// Code orphan and its respawn, which both send the workspace's
// storageUri) contend. An empty scope means the client did not opt in: the
// server never claims the registry or starts the watcher, so it neither
// supersedes nor is superseded, and many such servers coexist on one
// workspace.
//
// It is also a no-op without a workspace root or instanceID (the
// feature is off, or the client sent no rootUri); New only sets
// instanceID when it also wires the registry seams, so the two travel
// together (the nil guard is belt-and-suspenders for a hand-built
// Server). A failed claim leaves the server running without singleton
// protection rather than risking it stepping itself aside on a
// transient registry error.
func (s *Server) startSingletonWatch(root, scope string) {
	// Decide exactly once. A spec-compliant client sends a single
	// initialize, but guarding the whole decision with the watcher's
	// Once means a stray second initialize can neither re-assert this
	// (possibly already-superseded) server's ownership and invert
	// newest-wins, nor opt a server in mid-session after the first
	// initialize opted out (no scope or no root).
	s.singletonWatchOnce.Do(func() {
		if scope == "" || root == "" || s.instanceID == "" || s.singletonClaim == nil {
			return
		}
		// A NUL in the root (rootUri "%00" decodes to one) breaks the
		// workspaceKey framing: its legacy key would equal another
		// root's scoped key. No real path holds a NUL, so opt out.
		if strings.IndexByte(root, 0) >= 0 {
			return
		}
		key := workspaceKey(root, scope)
		// Claim the workspace under this instance's id, overwriting any
		// previous owner. Whichever server initialized most recently —
		// the window the user just opened or reloaded — wins; an older
		// server for the same workspace sees a different owner on its
		// next poll.
		if err := s.singletonClaim(key, s.instanceID); err != nil {
			s.logger.Printf("lsp: workspace singleton claim failed: %v", err)
			return
		}
		// Also take the legacy root-only record once, never watching it.
		// An older root-only binary (the leaked host on the first upgrade
		// to a scoped build) polls that record and steps aside when it
		// sees a new owner, exactly as it did before scopes. A no-token
		// server never writes it, so new clients without a scope still
		// coexist with everything.
		if err := s.singletonClaim(workspaceKey(root, ""), s.instanceID); err != nil {
			s.logger.Printf("lsp: legacy workspace singleton claim failed: %v", err)
		}
		onSuperseded := func() {
			s.logger.Printf("lsp: superseded by a newer server for this workspace; exiting")
			s.shutdown.Store(true)
			s.stopPendingLints()
			// Tell the editor this exit is intentional so its client
			// does not treat the imminent close as a crash and restart
			// us — that respawn loop is what kept the orphan alive.
			_ = s.t.writeNotification("mdsmith/superseded", supersededParams{Reason: "superseded"})
			s.onSupersededExit()
		}
		// Prune once, after both claims, so a start scans the registry
		// directory a single time. The scan runs on the watcher
		// goroutine, before its first poll, so the initialize response
		// never waits on a directory read. runCtx is read here, on the
		// dispatch goroutine, as startParentWatch does, not inside the
		// spawned goroutine.
		ctx := s.runCtx
		go func() {
			if s.singletonPrune != nil {
				s.singletonPrune(s.instanceID)
			}
			watchSingleton(ctx, key, s.instanceID, s.singletonInterval, s.singletonCurrent, onSuperseded)
		}()
	})
}

// supersededParams is the payload of the mdsmith/superseded
// server-to-client notification. The reason is informational; the
// extension keys off the method itself to suppress its restart.
type supersededParams struct {
	Reason string `json:"reason"`
}

// workspaceKey maps a workspace root path plus a client-supplied
// singleton scope to a stable, filesystem-safe registry key. Cleaning
// first makes "/w", "/w/" and "/w/." share one key, so two servers on
// the same workspace and scope contend for the same owner record, while
// different scopes on one workspace get different records and coexist.
//
// A non-empty scope is framed as root + "\x00" + scope, so a split is
// unambiguous for any root and scope free of NUL bytes; singletonScope
// turns a NUL-bearing scope into the opt-out and startSingletonWatch
// does the same for a NUL-bearing root, so neither reaches here and a
// NUL-free root cannot be split two ways. An empty scope hashes the
// cleaned root alone — the legacy root-only key, byte for byte — so
// there is one derivation, not two. startSingletonWatch uses
// that legacy key only for a scoped server's one-shot write that steps
// an older root-only binary aside; it never watches it, and a no-token
// server neither reads nor writes it.
func workspaceKey(root, scope string) string {
	h := sha256.New()
	_, _ = io.WriteString(h, filepath.Clean(root))
	if scope != "" {
		_, _ = io.WriteString(h, "\x00")
		_, _ = io.WriteString(h, scope)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// newInstanceID returns a random per-process identifier used to tell
// servers apart in the registry. Returns "" only if the OS RNG fails,
// which makes the singleton a no-op (the server still runs); we never
// fall back to a guessable id that could collide and cause a server to
// step aside for itself.
func newInstanceID() string {
	var b [16]byte
	if _, err := randRead(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}

// fileRegistry records the current owner of each workspace as a small
// file under a shared directory, so sibling server processes (launched
// by different editor hosts, with no parent/child relationship) can see
// each other's claims. One file per workspace key; its contents are the
// owning instance id.
type fileRegistry struct{ dir string }

// defaultRegistry locates the per-user registry directory. It prefers
// the OS cache dir and falls back to the temp dir so a claim never fails
// purely because the cache dir is unavailable.
func defaultRegistry() fileRegistry {
	base, err := userCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return fileRegistry{dir: filepath.Join(base, "mdsmith", "lsp-singleton")}
}

// singletonRecordMaxAge is how long an owner record may go unwritten
// before a scoped start prunes it (claim itself never prunes; see
// startSingletonWatch). Each record is written only at claim time,
// so the age is time since its owner started. A pruned record of a
// still-running server reads as "no owner", which watchSingleton treats
// as "still ours", so pruning never reaps a live server.
const singletonRecordMaxAge = 30 * 24 * time.Hour

func (r fileRegistry) path(key string) string {
	return filepath.Join(r.dir, key+".owner")
}

// claim records id as the current owner of key. It writes to a
// per-instance temp path and renames it onto the owner record: the
// rename is atomic, so a concurrent reader sees either the old owner or
// the new one, never a half-written id, and the id-tagged temp name
// keeps two servers claiming the same workspace at once from clobbering
// each other's temp file. It does not prune; startSingletonWatch calls
// prune once per start, after both of its claims.
func (r fileRegistry) claim(key, id string) error {
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return err
	}
	tmp := r.path(key) + "." + id + ".tmp"
	if err := os.WriteFile(tmp, []byte(id), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, r.path(key)); err != nil {
		// Best-effort: drop the temp file so a failed claim does not
		// leave an orphan in the shared registry dir.
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// prune removes records untouched for singletonRecordMaxAge on behalf
// of instance id. Records this server just claimed carry a fresh
// mtime, so they always stay.
func (r fileRegistry) prune(id string) {
	pruneStale(r.dir, id, time.Now().Add(-singletonRecordMaxAge))
}

// pruneStale removes owner records, leftover claim temp files, and
// leftover prune quarantine files in dir last modified before cutoff.
// Keys are per root and scope, and a deleted or moved workspace leaves
// its records behind, so without this the directory would grow without
// bound. It is best effort: any error just leaves the entry in place.
// Production passes no hooks; tests pass hooks[0], run after an entry
// is judged stale, and hooks[1], run after it is quarantined, to land a
// concurrent claim in each window.
//
// A plain stat-then-remove would delete a fresh record that a
// concurrent claim renamed onto the path in between. So a stale entry
// is first renamed (atomically) to a quarantine path tagged with the
// pruning instance's id, unique because one instance prunes one entry
// at a time, and its age re-checked there. Still stale: it is removed.
// Fresh: a claim landed in the window, so it is hard-linked back, which
// fails rather than overwrite a still newer claim that reached the path
// meanwhile. A filesystem without hard links falls back to a rename,
// which can only overwrite a claim that landed within that last
// instant.
func pruneStale(dir, id string, cutoff time.Time, hooks ...func(string)) {
	hook := func(i int, p string) {
		if i < len(hooks) && hooks[i] != nil {
			hooks[i](p)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !isRegistryRecord(name) {
			continue
		}
		// Stat now rather than via e.Info(): on Windows and plan9 the
		// DirEntry caches what ReadDir saw, so an entry removed or
		// re-claimed since would still read as present and stale.
		p := filepath.Join(dir, name)
		info, err := os.Lstat(p)
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		hook(0, p)
		if strings.HasSuffix(name, ".prune") {
			// Already a quarantine file: nothing renames onto it.
			_ = os.Remove(p)
			continue
		}
		// A failed move means the entry is gone or not ours to move.
		q := p + "." + id + ".prune"
		if err := os.Rename(p, q); err != nil {
			continue
		}
		hook(1, p)
		if qi, err := os.Lstat(q); err == nil && qi.ModTime().Before(cutoff) {
			_ = os.Remove(q)
			continue
		}
		if err := os.Link(q, p); err != nil && !errors.Is(err, fs.ErrExist) {
			_ = os.Rename(q, p)
		}
		_ = os.Remove(q)
	}
}

// isRegistryRecord reports whether name is a file the registry writes:
// an owner record, a claim temp file, or a prune quarantine file.
func isRegistryRecord(name string) bool {
	return strings.HasSuffix(name, ".owner") ||
		strings.HasSuffix(name, ".tmp") ||
		strings.HasSuffix(name, ".prune")
}

// current returns the instance id currently recorded for key, or "" if
// none is recorded or the file cannot be read. The empty case is
// deliberately conflated with "no owner": watchSingleton treats it as
// "still ours" so a transient read error never reaps the last server.
//
// This is a deliberate direct infra read, NOT routed through the
// pkg/mdsmith Workspace seam that internal/lsp uses for linted content:
// the owner record lives in the OS cache dir, outside any workspace, and
// is cross-process coordination state. os.Open + io.ReadAll keeps that
// distinction explicit rather than borrowing the workspace-sandbox
// reader for a file that is not workspace content.
func (r fileRegistry) current(key string) string {
	f, err := os.Open(r.path(key))
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
