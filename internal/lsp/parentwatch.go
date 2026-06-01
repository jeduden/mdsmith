package lsp

import (
	"context"
	"time"
)

// parentPollInterval is how often the parent-process watchdog checks
// that the editor that launched the server is still alive. 10s keeps the
// overhead negligible while still reaping an orphaned server within
// seconds of the editor going away.
const parentPollInterval = 10 * time.Second

// watchParentProcess polls isAlive(pid) every interval and calls onDead
// the first time the parent process is gone, then returns. It returns
// without calling onDead if ctx is canceled first (a normal server
// shutdown). Splitting the poll loop from the platform-specific liveness
// probe keeps this unit-testable with a fake isAlive.
func watchParentProcess(
	ctx context.Context,
	pid int,
	interval time.Duration,
	isAlive func(int) bool,
	onDead func(),
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !isAlive(pid) {
				onDead()
				return
			}
		}
	}
}

// startParentWatch launches the LSP processId watchdog. The LSP spec
// (§3.16, InitializeParams.processId) states: "If the parent process is
// not alive then the server should exit." vscode-languageclient sends
// the editor host's PID as processId, so once that process is gone the
// server has lost its only client and must terminate — otherwise it
// lingers as an orphan and keeps racing a freshly-spawned server after
// an editor update, reload, or crash (the failure that motivated this).
//
// The server already exits on stdin EOF; this is the spec-mandated
// backstop for when EOF never arrives (an inherited or held-open pipe
// fd, a hard host swap). It is a no-op when the client sent no usable
// PID — those clients rely on stdin EOF alone.
func (s *Server) startParentWatch(processID *int) {
	if processID == nil || *processID <= 0 {
		return
	}
	pid := *processID
	s.parentWatchOnce.Do(func() {
		go watchParentProcess(s.runCtx, pid, s.parentInterval, s.parentAlive, func() {
			s.logger.Printf("lsp: parent process %d is gone; shutting down", pid)
			s.shutdown.Store(true)
			s.stopPendingLints()
			s.onParentExit()
		})
	})
}
