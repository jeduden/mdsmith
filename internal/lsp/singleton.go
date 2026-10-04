package lsp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
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
// It is a no-op without a workspace root or instanceID (the feature is
// off, or the client sent no rootUri); New only sets instanceID when it
// also wires the registry seams, so the two travel together (the nil
// guard is belt-and-suspenders for a hand-built Server). A failed claim
// leaves the server running without singleton protection rather than
// risking it stepping itself aside on a transient registry error.
func (s *Server) startSingletonWatch(root string) {
	if root == "" || s.instanceID == "" || s.singletonClaim == nil {
		return
	}
	// Claim and watch exactly once. A spec-compliant client sends a
	// single initialize, but guarding the claim with the watcher's Once
	// means a stray second initialize cannot re-assert this (possibly
	// already-superseded) server's ownership and invert newest-wins.
	s.singletonWatchOnce.Do(func() {
		key := workspaceKey(root, "")
		// Claim the workspace under this instance's id, overwriting any
		// previous owner. Whichever server initialized most recently —
		// the window the user just opened or reloaded — wins; an older
		// server for the same workspace sees a different owner on its
		// next poll.
		if err := s.singletonClaim(key, s.instanceID); err != nil {
			s.logger.Printf("lsp: workspace singleton claim failed: %v", err)
			return
		}
		go watchSingleton(s.runCtx, key, s.instanceID, s.singletonInterval, s.singletonCurrent, func() {
			s.logger.Printf("lsp: superseded by a newer server for this workspace; exiting")
			s.shutdown.Store(true)
			s.stopPendingLints()
			// Tell the editor this exit is intentional so its client
			// does not treat the imminent close as a crash and restart
			// us — that respawn loop is what kept the orphan alive.
			_ = s.t.writeNotification("mdsmith/superseded", supersededParams{Reason: "superseded"})
			s.onSupersededExit()
		})
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
// A non-empty scope is framed as root + "\x00" + scope so no root/scope
// split can collide with another. An empty scope hashes the cleaned root
// alone — the legacy root-only key, byte for byte — so every caller
// shares this one derivation and an older root-only binary still agrees
// with a no-token client.
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

func (r fileRegistry) path(key string) string {
	return filepath.Join(r.dir, key+".owner")
}

// claim records id as the current owner of key. It writes to a
// per-instance temp path and renames it onto the owner record: the
// rename is atomic, so a concurrent reader sees either the old owner or
// the new one, never a half-written id, and the id-tagged temp name
// keeps two servers claiming the same workspace at once from clobbering
// each other's temp file.
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
