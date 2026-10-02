package build

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"
)

// defaultExecPath is the compiled-default PATH a recipe runs under when
// build.exec.path is empty. It is deliberately minimal: a recipe that
// needs a tool outside these dirs must opt the directory in.
const defaultExecPath = "/usr/bin:/bin"

// defaultPassThrough is the compiled-default env-pass-through list. These
// three names are safe, locale/home variables a typical recipe expects;
// anything else (tokens, credentials, CI injected secrets) is withheld.
func defaultPassThrough() []string {
	return []string{"HOME", "LANG", "LC_ALL"}
}

// ExecConfig is the resolved build.exec settings for a run: the
// allowlisted PATH and the names of environment variables passed through
// to every recipe. Both fields are optional; an empty field means the
// compiled default applies.
type ExecConfig struct {
	// Path is the PATH the recipe runs under. Empty means defaultExecPath.
	Path string
	// EnvPassThrough names the environment variables forwarded into the
	// recipe. Nil means the compiled default list; a non-nil list
	// *replaces* the default (it does not append).
	EnvPassThrough []string
}

// defaultExecConfig returns the compiled defaults as an ExecConfig.
func defaultExecConfig() ExecConfig {
	return ExecConfig{Path: defaultExecPath, EnvPassThrough: defaultPassThrough()}
}

// buildEnv constructs the minimal KEY=VALUE environment slice for a
// recipe. PATH comes from cfg.Path (or def.Path when empty). The
// pass-through list comes from cfg.EnvPassThrough (or def's when nil);
// each named variable that is actually set in the current process is
// forwarded with its current value. An unset name produces no entry.
// Entries are sorted for determinism.
func buildEnv(cfg, def ExecConfig) []string {
	path := cfg.Path
	if path == "" {
		path = def.Path
	}
	pass := cfg.EnvPassThrough
	if pass == nil {
		pass = def.EnvPassThrough
	}

	env := map[string]string{"PATH": path}
	for _, name := range pass {
		if name == "" || name == "PATH" {
			// PATH is set explicitly above; an empty name is meaningless.
			continue
		}
		if strings.ContainsAny(name, "=\x00\n\r") {
			// Defense in depth: config validation already rejects these, but
			// NewCustomBuilderExec is exported, so re-guard here. A name with
			// "=" or a control char could smuggle a value or a second entry
			// into the recipe environment.
			continue
		}
		if v, ok := os.LookupEnv(name); ok {
			env[name] = v
		}
	}

	out := make([]string, 0, len(env))
	for _, k := range slices.Sorted(maps.Keys(env)) {
		out = append(out, k+"="+env[k])
	}
	return out
}

// runOpts bundles the inputs to runRecipe. The per-recipe timeout is
// carried by the context's deadline, not a separate field.
type runOpts struct {
	argv    []string   // program plus arguments (no shell)
	dir     string     // working directory (the per-recipe staging dir)
	exec    ExecConfig // user-configured exec settings
	defExec ExecConfig // compiled defaults to fall back to
	stdout  io.Writer  // nil means os.Stderr
	stderr  io.Writer  // nil means os.Stderr
}

// runRecipe executes argv with a hermetic environment, a fixed working
// directory, and process-group isolation. No shell is invoked: argv[0]
// is the program and argv[1:] its arguments.
//
// The recipe runs in its own process group (Setpgid on Unix;
// CREATE_NEW_PROCESS_GROUP plus a Job Object on Windows). On timeout
// Unix sends SIGTERM to the group, waits up to gracePeriod, then sends
// SIGKILL; Windows sends CTRL_BREAK and terminates the Job Object at
// once, with no wait. Either way a recipe that spawns daemons cannot
// leave orphans behind. If the Job Object could not be created, Windows
// falls back to CTRL_BREAK alone, which reaches the leader's group but
// cannot guarantee that. Other targets
// (exec_other.go: js/wasm, wasip1, plan9) have no group primitive: the
// timeout kills only the leader, with no grace period, so the orphan
// guarantee holds on Unix and on Windows with a Job Object only.
//
// After the kill, runRecipe waits at most reapWait for the recipe to
// exit. If it has not, it kills the leader directly and waits at most
// reapWait again, then returns anyway: a leader that ignored the group
// kill, or a survivor that holds a captured output pipe open, cannot
// hang mdsmith. Output such a survivor writes after runRecipe returns
// is dropped (outputGate), never forwarded to o.stdout or o.stderr.
//
// It returns the process exit code, whether the run timed out, and any
// error. On success it returns (0, false, nil). On non-zero exit it
// returns the exit code and a non-nil error. On timeout it returns the
// exit code (or -1 if unavailable), timedOut=true, and a non-nil error.
func runRecipe(ctx context.Context, o runOpts) (int, bool, error) {
	// We manage the timeout and kill path ourselves (process group), so the
	// command itself is not bound to a context-cancel kill — that would
	// only kill the leader, not the group.
	cmd := exec.Command(o.argv[0], o.argv[1:]...) //nolint:gosec // argv is explicit; user-declared recipe
	cmd.Dir = o.dir
	// A caller's writer is reached through os/exec's copy goroutines,
	// which outlive runRecipe when the timeout path stops waiting on a
	// survivor. The gate closes on return, so no write lands after it.
	gate := &outputGate{}
	defer gate.close()
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if o.stdout != nil {
		cmd.Stdout = gate.wrap(o.stdout)
	}
	if o.stderr != nil {
		cmd.Stderr = gate.wrap(o.stderr)
	}
	cmd.Env = buildEnv(o.exec, o.defExec)
	configureProcessGroup(cmd)

	if err := cmd.Start(); err != nil {
		return -1, false, fmt.Errorf("starting recipe: %w", err)
	}

	jobCleanup := afterStartFn(cmd)
	if jobCleanup != nil {
		defer jobCleanup()
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	// The context carries the deadline (set by the caller via
	// context.WithTimeout). We do not bind the command to a context-cancel
	// kill — that would kill only the leader, not the process group — so on
	// ctx.Done() we kill the whole group ourselves, then drain the Wait.
	select {
	case err := <-done:
		if err == nil {
			return 0, false, nil
		}
		exitCode := -1
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exitCode = ee.ExitCode()
		}
		return exitCode, false, err
	case <-ctx.Done():
		killGroupFn(cmd)
		reaped, waitErr := waitAtMost(done, reapWait)
		if !reaped {
			// The group kill left the leader running (Windows when the
			// Job Object could not be set up and the recipe ignores
			// CTRL_BREAK), or a surviving child still holds a captured
			// output pipe, so Wait cannot return (targets with no group
			// kill). Kill the leader directly, then give up waiting so
			// mdsmith never hangs; done is buffered, so the Wait
			// goroutine still exits once the pipe closes.
			_ = cmd.Process.Kill()
			_, waitErr = waitAtMost(done, reapWait)
		}
		exitCode := -1
		var ee *exec.ExitError
		if errors.As(waitErr, &ee) {
			exitCode = ee.ExitCode()
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return exitCode, true, fmt.Errorf("recipe timed out: %w", ctx.Err())
		}
		return exitCode, true, fmt.Errorf("recipe cancelled: %w", ctx.Err())
	}
}

// afterStartFn indirects afterStart so a test can install a non-nil job
// cleanup and exercise the deferred-cleanup branch on Unix.
var afterStartFn = afterStart

// killGroupFn indirects killGroup so a test can model a group kill that
// leaves the recipe running.
var killGroupFn = killGroup

// reapWait bounds each wait for cmd.Wait after a timeout kill: once
// after killGroup, and once more after the leader-only fallback kill.
// It is a var so a test can shorten it.
var reapWait = 5 * time.Second

// outputGate forwards a recipe's stdout and stderr writes until close,
// then drops them. One mutex covers both streams and is held across
// each forwarded Write, so once close returns no write is in flight
// and none follows. Sharing it also keeps the os/exec guarantee that a
// writer passed as both stdout and stderr sees one Write at a time.
type outputGate struct {
	mu     sync.Mutex
	closed bool
}

// wrap returns a writer that forwards to w while the gate is open. The
// value is comparable, so os/exec still detects stdout == stderr.
func (g *outputGate) wrap(w io.Writer) io.Writer { return gatedWriter{g: g, w: w} }

// close stops forwarding, after any in-flight write completes.
func (g *outputGate) close() {
	g.mu.Lock()
	g.closed = true
	g.mu.Unlock()
}

// gatedWriter is one stream's side of an outputGate.
type gatedWriter struct {
	g *outputGate
	w io.Writer
}

// Write forwards p to the wrapped writer, or reports it written and
// drops it once the gate is closed, so the copy goroutine keeps
// draining the pipe and the survivor never blocks on a full one.
func (gw gatedWriter) Write(p []byte) (int, error) {
	gw.g.mu.Lock()
	defer gw.g.mu.Unlock()
	if gw.g.closed {
		return len(p), nil
	}
	return gw.w.Write(p)
}

// waitAtMost receives from done for up to d. It reports true and the
// received error, or false and nil when d elapses first.
func waitAtMost(done <-chan error, d time.Duration) (bool, error) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case err := <-done:
		return true, err
	case <-t.C:
		return false, nil
	}
}
