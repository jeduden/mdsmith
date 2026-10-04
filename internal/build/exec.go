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
	// Setpgid, Job Object, or RFNOTEG), and a cancel or timeout signals
	// only its leader (sharedGroupKiller): signalling the group would hit
	// mdsmith. Hooks
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

// ErrForceKilled marks a cancelled or timed-out run whose group a
// second interrupt SIGKILLed before the SIGTERM grace ran out (Unix
// only).
var ErrForceKilled = errors.New("SIGKILL on a second interrupt")

// runRecipe executes argv with a hermetic environment, a fixed working
// directory, and process-group isolation. No shell is invoked: argv[0]
// is the program and argv[1:] its arguments. A hook opts out of the
// environment (inheritEnv) and the group (sharedGroup); the group and
// kill notes below then do not apply: a cancel or timeout signals only
// its leader through sharedGroupKiller's killer, which on Unix is
// SIGTERM, up to gracePeriod, then SIGKILL (a closed WithForceKill
// channel skips the grace), so a hook that traps TERM runs its cleanup;
// elsewhere it is an uncatchable leader kill at once.
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
// The command is bound to ctx with exec.CommandContext: when ctx ends
// while the leader runs, os/exec calls Cmd.Cancel, which runs the
// kill above. Cmd.WaitDelay (reapWait) then bounds the wait for the
// leader: if the kill left it running (a leader that ignored the group
// kill), os/exec kills it with Process.Kill, which it cannot catch on
// Unix (SIGKILL) or Windows (TerminateProcess); on plan9 that is a
// note, but kill already ended in the uncatchable ctl kill. runRecipe
// then waits for the leader to exit. A second interrupt (the
// WithForceKill channel closed), before or after the kill returned,
// kills the leader directly as soon as the kill has returned
// (killLeaderOnForce), without waiting out reapWait.
//
// A leader that exits before the deadline does not end the run while
// a child it left behind still holds a captured pipe: that child's
// output keeps reaching the caller until it closes the pipe or the
// deadline passes. os/exec stops watching ctx once the leader exits,
// so runRecipe watches it itself: at the deadline it runs the kill
// and reports a timeout with the leader's own exit code.
//
// After either kill, runRecipe waits at most reapWait for captured
// output to drain and closes its end of the pipes
// (recipeOutput.abandon), so a survivor that holds a captured pipe
// open cannot hang mdsmith (forcedReapWait once the WithForceKill
// channel is closed, at any point of that wait). The pipes are
// runRecipe's own, not os/exec's, because os/exec's WaitDelay drain
// waits for its copy goroutines after closing the pipes, and plan9
// cannot cancel a blocked read. On Unix and Windows the close also
// ends the copy goroutine and frees the fd; on plan9 they last until
// the survivor's next write or exit. Captured output written after
// runRecipe returns is dropped,
// never forwarded. A writer that is an *os.File (including the nil
// default, os.Stderr) is not captured: the child writes to it
// directly, so a survivor can still reach it after the return.
//
// A ctx already done at entry, or by the time Start runs, starts
// nothing: a cancel returns
// (-1, false, err) and a spent deadline returns (-1, true, err), err
// wrapping ErrNotStarted.
//
// It returns the process exit code, whether the run timed out, and any
// error. On success it returns (0, false, nil). On non-zero exit it
// returns the exit code and a non-nil error. On timeout it returns the
// leader's exit status, timedOut=true, and a non-nil error: a leader
// that exited before the deadline (a child held a captured pipe past
// it) keeps its own code, 0 included; one the kill ended reports the
// status the kill left it (-1 for a signal on Unix; Windows and plan9
// report a code), and one Wait read no status for reports -1.
func runRecipe(ctx context.Context, o runOpts) (int, bool, error) {
	// A context already done at entry (a CLI interrupt that landed while
	// the target was staged, or a spent deadline) starts no recipe and
	// opens no pipes. One that ends after this check is refused by
	// Start itself (startFailure). No kill path runs, so a cancel is not
	// timedOut.
	if err := ctx.Err(); err != nil {
		return -1, errors.Is(err, context.DeadlineExceeded), NotStartedError(err)
	}
	// exec.CommandContext binds the leader to ctx: once ctx is done
	// while the leader runs, os/exec calls Cancel (the group kill), and
	// if the leader is still running WaitDelay after Cancel returned,
	// kills it directly with Process.Kill and returns from Wait.
	cmd := exec.CommandContext(ctx, o.argv[0], o.argv[1:]...) //nolint:gosec // argv is explicit; user-declared recipe
	cmd.Dir = o.dir
	// The pipes are ours, not os/exec's: it gets *os.File ends and runs
	// no copy goroutines, so cmd.Wait returns when the leader exits and
	// the timeout path can close them on a survivor. os/exec's own
	// WaitDelay drain would wait for its copies after closing the
	// pipes, which on plan9 cannot end a pending read. The gate closes
	// on return, so no output reaches a caller after it.
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

	// WaitDelay is set once, here: os/exec reads it after Cancel
	// returns, and the os/exec docs promise nothing about a change made
	// inside Cancel. A second interrupt instead kills the leader
	// directly (killLeaderOnForce), whenever it comes.
	force := forceKillFrom(ctx)
	rk := newRecipeKill(force)
	cmd.Cancel = rk.cancel
	cmd.WaitDelay = reapWait

	err := cmd.Start()
	ro.closeChildEnds()
	if err != nil {
		ro.abandon()
		return startFailure(ctx, o.processLabel(), err)
	}
	killer := killerFor(cmd, o.sharedGroup)
	rk.arm(killer)
	defer killer.close()

	exited := make(chan struct{})
	go killLeaderOnForce(func() { _ = cmd.Process.Kill() }, rk.killed, force, exited)
	err = cmd.Wait()
	close(exited)
	if rk.cancelled {
		return timeoutResult(ctx, ro, cmd.ProcessState.ExitCode(), rk.forced)
	}
	// The leader exited on its own, so os/exec no longer watches ctx. A
	// child it left behind may still hold a captured pipe, and the
	// deadline still applies to the drain: at it, kill the group, unless
	// the killer found it already empty when told the leader exited.
	noteLeaderExited(killer)
	select {
	case <-ro.drained:
		if err == nil {
			err = ro.err()
		}
		return exitResult(err)
	case <-ctx.Done():
		return timeoutResult(ctx, ro, cmd.ProcessState.ExitCode(), killer.kill(force))
	}
}

// killerFor returns the groupKiller for a started cmd: the killer is
// the run's kill state, closed when runRecipe returns. A hook
// (sharedGroup) gets a leader-only one and skips afterStart, so it gets
// no Job Object whose kill-on-close would end a dev server it
// backgrounded.
func killerFor(cmd *exec.Cmd, sharedGroup bool) groupKiller {
	if sharedGroup {
		return sharedGroupKiller(cmd)
	}
	return afterStartFn(cmd)
}

// leaderExitNoter is a groupKiller that names its group by a number
// the leader's pid lent it (Unix's pgid). Once the leader is reaped and
// the group empty, that number may be reused by an unrelated group, so
// runRecipe tells such a killer, right after Wait reaped a leader that
// exited on its own, to check the group while the number is still its.
type leaderExitNoter interface{ leaderExited() }

// noteLeaderExited calls k's leaderExited when k is a leaderExitNoter.
// Killers that hold a handle (Windows' Job Object, plan9's notepg) or
// signal through the reaped cmd.Process need no such check.
func noteLeaderExited(k groupKiller) {
	if n, ok := k.(leaderExitNoter); ok {
		n.leaderExited()
	}
}

// recipeKill is the timeout kill os/exec runs through Cmd.Cancel.
// os/exec may call Cancel as soon as Start returns, so cancel first
// waits for arm to install the killer (ready). killed is closed once
// the kill returned. cancelled and forced are written by cancel and
// read only after cmd.Wait returned, which orders them.
type recipeKill struct {
	force             <-chan struct{}
	ready, killed     chan struct{}
	killer            groupKiller
	cancelled, forced bool
}

// newRecipeKill returns a recipeKill for a run whose WithForceKill
// channel is force (nil when there is none).
func newRecipeKill(force <-chan struct{}) *recipeKill {
	return &recipeKill{force: force, ready: make(chan struct{}), killed: make(chan struct{})}
}

// arm installs the started command's killer and lets cancel run.
func (k *recipeKill) arm(killer groupKiller) {
	k.killer = killer
	close(k.ready)
}

// cancel is Cmd.Cancel: it runs the group kill once, records that it
// ran and whether force cut the grace short, and closes killed.
func (k *recipeKill) cancel() error {
	<-k.ready
	k.cancelled = true
	k.forced = k.killer.kill(k.force)
	close(k.killed)
	return nil
}

// startFailure maps a failed Start to runRecipe's results, naming the
// process by label. Start refuses to fork once ctx is done and returns
// its Err: that run never started (NotStartedError, timedOut for a
// spent deadline). Any other error is a start failure.
func startFailure(ctx context.Context, label string, err error) (int, bool, error) {
	if ctxErr := ctx.Err(); ctxErr != nil && errors.Is(err, ctxErr) {
		return -1, errors.Is(ctxErr, context.DeadlineExceeded), NotStartedError(ctxErr)
	}
	return -1, false, fmt.Errorf("starting %s: %w", label, err)
}

// killLeaderOnForce runs kill, which kills the leader directly, once the timeout kill
// has returned (killed is closed) and a second interrupt has closed
// force, in either order. The group kill can leave the leader running
// (Windows with no Job Object sends only CTRL_BREAK; a Unix leader can
// leave its group), and without this a second interrupt would wait out
// WaitDelay's whole reapWait. It returns without a kill once the
// leader has exited (exited is closed); a nil force never fires.
func killLeaderOnForce(kill func(), killed, force, exited <-chan struct{}) {
	select {
	case <-killed:
	case <-exited:
		return
	}
	select {
	case <-force:
		kill()
	case <-exited:
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
// waits at most reapWait (forcedReapWait once a second interrupt closed
// the WithForceKill channel) for captured output to drain, abandons the
// pipes if a survivor still holds them, and reports the timeout or
// cancellation with exitCode, the leader's exit status (-1 when it died
// of a signal on Unix or Wait read no status). forced (a second
// interrupt escalated the kill) wraps ErrForceKilled into either one:
// a timed-out recipe still in its grace is cut short by it too.
func timeoutResult(ctx context.Context, ro *recipeOutput, exitCode int, forced bool) (int, bool, error) {
	if drained, _ := waitAtMost(ro.drained, reapWait, forceKillFrom(ctx)); !drained {
		ro.abandon()
	}
	what := "recipe cancelled"
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		what = "recipe timed out"
	}
	if forced {
		return exitCode, true, fmt.Errorf("%s (%w): %w", what, ErrForceKilled, ctx.Err())
	}
	return exitCode, true, fmt.Errorf("%s: %w", what, ctx.Err())
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

// groupKiller is one recipe's kill state, owned by runRecipe. afterStart
// builds it once the recipe has started; a hook (sharedGroup) gets
// sharedGroupKiller's leader-only one instead. A platform with no group
// to kill (exec_other.go) returns a killer that kills only the leader.
type groupKiller interface {
	// kill ends the recipe's whole group (or only its leader where the
	// platform has no group, plan9 captured none, or the run is a hook).
	// force is the run context's WithForceKill channel: on Unix, once it
	// is closed, kill skips what is left of the SIGTERM grace period and
	// sends SIGKILL at once; elsewhere kill has no grace and ignores it.
	// kill reports whether force cut the grace short while the target
	// was still alive. It can leave the leader running: Windows without
	// a Job Object sends only CTRL_BREAK, which the recipe can ignore,
	// and a Unix leader can leave its group; os/exec's WaitDelay then
	// kills the leader. A killer for a command that never started does
	// nothing and reports false.
	kill(force <-chan struct{}) bool
	// close releases what afterStart captured. runRecipe calls it once,
	// on return.
	close()
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

// afterStartFn indirects afterStart so a test can install a stub
// killer: one that records a call, or models a group kill that leaves
// the recipe running.
var afterStartFn = afterStart

// reapWait bounds each wait after a timeout kill: as Cmd.WaitDelay,
// for the leader before os/exec kills it directly (a second interrupt
// kills it at once, killLeaderOnForce), and for captured output to
// drain (forcedReapWait after a second interrupt). It is a var so a
// test can shorten it.
var reapWait = 5 * time.Second

// forcedReapWait bounds the output drain once a second interrupt
// closed the WithForceKill channel: the kill already escalated to
// SIGKILL, so one short poll drains what the group wrote, and a survivor
// outside the group (a setsid daemon holding a captured pipe) costs no
// more than that before runRecipe abandons it.
const forcedReapWait = 100 * time.Millisecond

// waitAtMost receives from ch for up to d. It reports true and the
// received value (the zero value once ch is closed), or false and the
// zero value when the wait runs out first. Once force is closed (a
// second interrupt, see WithForceKill) the wait ends at most
// forcedReapWait later; a nil force never fires. It still waits that
// long, so output a holder wrote before the SIGKILL reached it can
// still drain before runRecipe abandons the pipe.
func waitAtMost[T any](ch <-chan T, d time.Duration, force <-chan struct{}) (bool, T) {
	t := time.NewTimer(d)
	defer t.Stop()
	deadline := time.Now().Add(d)
	for {
		select {
		case v := <-ch:
			return true, v
		case <-t.C:
			var zero T
			return false, zero
		case <-force:
			force = nil // fires once; a nil channel blocks
			if short := time.Now().Add(forcedReapWait); short.Before(deadline) {
				t.Reset(forcedReapWait)
			}
		}
	}
}
