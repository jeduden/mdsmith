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
	// inheritEnv keeps mdsmith's own environment instead of the
	// hermetic buildEnv one. Hooks set it: they are developer tooling
	// (dev servers, deploy steps), not reproducible builds.
	inheritEnv bool
	// label names the process in start errors; empty means "recipe".
	label string
	// sharedGroup keeps the process in mdsmith's own process group (no
	// Setpgid, Job Object, or RFNOTEG), and a cancel or timeout kills
	// only its leader: signalling the group would hit mdsmith. Hooks
	// set it. A before-hook may background a dev server that must
	// outlive the hook, which a Job Object's kill-on-close would end;
	// in the terminal's foreground group the Ctrl-C reaches that server
	// and a hook can prompt on /dev/tty without SIGTTIN.
	sharedGroup bool
}

// ErrNotStarted marks a run runRecipe refused because its context was
// already done at entry: no process started and no kill ran, so a
// report must not name one. It is wrapped with the context's error.
var ErrNotStarted = errors.New("before start")

// NotStartedError is the error for a run refused before start because
// its context was already done; ctxErr is that context's Err. It wraps
// ErrNotStarted and ctxErr, and says "timed out" for a spent deadline
// and "cancelled" otherwise. runRecipe returns it, and a caller that
// refuses a run itself can return the same error.
func NotStartedError(ctxErr error) error {
	what := "recipe cancelled"
	if errors.Is(ctxErr, context.DeadlineExceeded) {
		what = "recipe timed out"
	}
	return fmt.Errorf("%s %w: %w", what, ErrNotStarted, ctxErr)
}

// ErrForceKilled marks a cancelled run whose group a second interrupt
// SIGKILLed before the SIGTERM grace ran out (Unix only).
var ErrForceKilled = errors.New("SIGKILL on a second interrupt")

// runRecipe executes argv with a hermetic environment, a fixed working
// directory, and process-group isolation. No shell is invoked: argv[0]
// is the program and argv[1:] its arguments. A hook opts out of the
// environment (inheritEnv) and the group (sharedGroup); the group and
// kill notes below then do not apply, and a cancel or timeout kills
// only its leader with forceKillLeader.
//
// The recipe runs in its own process group (Setpgid on Unix;
// CREATE_NEW_PROCESS_GROUP plus a Job Object on Windows). On timeout
// Unix sends SIGTERM to the group, waits up to gracePeriod, then sends
// SIGKILL; Windows sends CTRL_BREAK and terminates the Job Object at
// once, with no wait. Either way a recipe that spawns daemons cannot
// leave orphans behind. If the Job Object could not be created, Windows
// falls back to CTRL_BREAK alone, which reaches the leader's group but
// cannot guarantee that. On plan9 the recipe leads its own note group
// (RFNOTEG). afterStart reads the noteid and opens /proc/<pid>/notepg
// while the leader is alive, and keeps nothing if the noteid is
// unreadable or is mdsmith's own. The timeout writes "kill" to that
// file, which reaches every process still in the group at once, with
// no grace period, even after the leader exited; it then writes a
// forced "kill" to the ctl file of every process whose noteid still
// matches, so a member that catches the note dies too, and repeats
// that sweep until a pass finds no new member. Last it writes a forced
// "kill" to the leader's own ctl file, falling back to a note only when
// that file cannot be opened or written. rc's `&` starts a new note
// group, so a job backgrounded that way escapes, as a setsid daemon
// does on Unix.
// A leader that exits before afterStart runs leaves nothing to find
// its group by, so its children survive. On js/wasm and wasip1
// (exec_other.go) no subprocess can start. So the orphan guarantee
// holds on Unix, on plan9 for processes that stay in the note group,
// and on Windows with a Job Object.
//
// After the kill, runRecipe waits at most reapWait for the leader to
// exit. If it has not (a leader that ignored the group kill), it kills
// the leader directly with forceKillLeader, a kill it cannot catch,
// and waits at most reapWait again. It then waits at most reapWait
// for captured output to drain and closes its end of
// the pipes (recipeOutput.abandon), so a survivor that holds a captured
// pipe open cannot hang mdsmith. On Unix and Windows the close also
// ends the copy goroutine and frees the fd; plan9 cannot cancel a
// blocked read, so there they last until the survivor's next write or
// exit. Captured output written after runRecipe returns is dropped,
// never forwarded. A writer that is an *os.File (including the nil
// default, os.Stderr) is not captured: the child writes to it
// directly, so a survivor can still reach it after the return.
//
// A ctx already done at entry starts nothing: a cancel returns
// (-1, false, err) and a spent deadline returns (-1, true, err), err
// wrapping ErrNotStarted.
//
// It returns the process exit code, whether the run timed out, and any
// error. On success it returns (0, false, nil). On non-zero exit it
// returns the exit code and a non-nil error. On timeout it returns the
// exit code (or -1 if unavailable), timedOut=true, and a non-nil error.
func runRecipe(ctx context.Context, o runOpts) (int, bool, error) {
	// A context already done at entry (a CLI interrupt that landed while
	// the target was staged, or a spent deadline) starts no recipe:
	// exec.Command's Start ignores ctx, so it would fork one only to
	// kill it at once. No kill path runs, so a cancel is not timedOut.
	if err := ctx.Err(); err != nil {
		return -1, errors.Is(err, context.DeadlineExceeded), NotStartedError(err)
	}
	// We manage the timeout and kill path ourselves (process group), so the
	// command itself is not bound to a context-cancel kill — that would
	// only kill the leader, not the group.
	cmd := exec.Command(o.argv[0], o.argv[1:]...) //nolint:gosec // argv is explicit; user-declared recipe
	cmd.Dir = o.dir
	// The pipes are ours, not os/exec's, so cmd.Wait returns when the
	// leader exits and the timeout path can close them on a survivor.
	// The gate closes on return, so no output reaches a caller after it.
	ro := &recipeOutput{}
	defer ro.gate.close()
	if err := ro.attach(cmd, o.stdout, o.stderr); err != nil {
		return -1, false, fmt.Errorf("capturing %s output: %w", o.processLabel(), err)
	}
	if !o.inheritEnv {
		cmd.Env = buildEnv(o.exec, o.defExec)
	}
	if !o.sharedGroup {
		configureProcessGroup(cmd)
	}

	err := cmd.Start()
	ro.closeChildEnds()
	if err != nil {
		ro.abandon()
		return -1, false, fmt.Errorf("starting %s: %w", o.processLabel(), err)
	}

	forced := false
	kill := func() { forced = killGroupFn(cmd, forceKillFrom(ctx)) }
	if o.sharedGroup {
		kill = func() { forceKillLeaderFn(cmd) }
	} else if jobCleanup := afterStartFn(cmd); jobCleanup != nil {
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
		// The leader exited; a child it left behind may still hold a
		// captured pipe, so the deadline still applies to the drain.
		select {
		case <-ro.drained:
			if err == nil {
				err = ro.err()
			}
			return exitResult(err)
		case <-ctx.Done():
			kill()
			return timeoutResult(ctx, ro, err, forced)
		}
	case <-ctx.Done():
		kill()
		reaped, waitErr := waitAtMost(done, reapWait)
		if !reaped {
			// The group kill left the leader running (Windows when the
			// Job Object could not be set up and the recipe ignores
			// CTRL_BREAK). Kill it directly with a kill it cannot
			// catch; done is buffered, so the Wait goroutine exits
			// whenever the leader does.
			forceKillLeaderFn(cmd)
			_, waitErr = waitAtMost(done, reapWait)
		}
		return timeoutResult(ctx, ro, waitErr, forced)
	}
}

// processLabel returns o.label, or "recipe" when it is empty.
func (o runOpts) processLabel() string {
	if o.label == "" {
		return "recipe"
	}
	return o.label
}

// exitResult maps a finished recipe's Wait (or output copy) error to
// runRecipe's results: (0, false, nil) on success, else the exit code,
// or -1 when err is not an *exec.ExitError.
func exitResult(err error) (int, bool, error) {
	if err == nil {
		return 0, false, nil
	}
	return exitCodeOf(err), false, err
}

// timeoutResult finishes a run whose context ended after the kill: it
// waits at most reapWait for captured output to drain, abandons the
// pipes if a survivor still holds them, and reports the timeout or
// cancellation with the exit code waitErr carries. forced (a second
// interrupt escalated the kill) wraps ErrForceKilled into a cancel.
func timeoutResult(ctx context.Context, ro *recipeOutput, waitErr error, forced bool) (int, bool, error) {
	if drained, _ := waitAtMost(ro.drained, reapWait); !drained {
		ro.abandon()
	}
	exitCode := exitCodeOf(waitErr)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return exitCode, true, fmt.Errorf("recipe timed out: %w", ctx.Err())
	}
	if forced {
		return exitCode, true, fmt.Errorf("recipe cancelled (%w): %w", ErrForceKilled, ctx.Err())
	}
	return exitCode, true, fmt.Errorf("recipe cancelled: %w", ctx.Err())
}

// exitCodeOf returns the exit code an *exec.ExitError in err carries,
// or -1 when there is none.
func exitCodeOf(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// forceKillKey is the context key WithForceKill stores its channel under.
type forceKillKey struct{}

// WithForceKill returns a copy of ctx that carries force. Once force is
// closed, a recipe kill skips what is left of the Unix SIGTERM grace
// period and sends SIGKILL to the group at once. The CLI closes it on a
// second interrupt so an impatient user is not made to wait. Other
// platforms kill with no grace period and ignore it.
func WithForceKill(ctx context.Context, force <-chan struct{}) context.Context {
	return context.WithValue(ctx, forceKillKey{}, force)
}

// forceKillFrom returns the channel WithForceKill stored in ctx, or nil
// (which never fires) when there is none.
func forceKillFrom(ctx context.Context) <-chan struct{} {
	force, _ := ctx.Value(forceKillKey{}).(<-chan struct{})
	return force
}

// afterStartFn indirects afterStart so a test can install a non-nil job
// cleanup and exercise the deferred-cleanup branch on Unix.
var afterStartFn = afterStart

// killGroupFn indirects killGroupUntil so a test can model a group
// kill that leaves the recipe running. Its second argument is the
// WithForceKill channel of the run's context; it reports whether that
// channel escalated the kill.
var killGroupFn = killGroupUntil

// forceKillLeaderFn indirects forceKillLeader so a test can check that
// runRecipe's leader-only fallback uses it.
var forceKillLeaderFn = forceKillLeader

// reapWait bounds each wait after a timeout kill: for the leader after
// killGroup, for it again after the leader-only fallback kill, and for
// captured output to drain. It is a var so a test can shorten it.
var reapWait = 5 * time.Second

// waitAtMost receives from ch for up to d. It reports true and the
// received value (the zero value once ch is closed), or false and the
// zero value when d elapses first.
func waitAtMost[T any](ch <-chan T, d time.Duration) (bool, T) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case v := <-ch:
		return true, v
	case <-t.C:
		var zero T
		return false, zero
	}
}
