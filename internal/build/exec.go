package build

import (
	"context"
	"errors"
	"fmt"
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
}

// runRecipe executes argv with a hermetic environment, a fixed working
// directory, and process-group isolation. No shell is invoked: argv[0]
// is the program and argv[1:] its arguments.
//
// The recipe runs in its own process group (Setpgid on Unix;
// CREATE_NEW_PROCESS_GROUP plus a Job Object on Windows). On timeout
// mdsmith signals the whole group (SIGTERM on Unix, CTRL_BREAK on
// Windows), waits up to gracePeriod, then force-kills the group, so a
// recipe that spawns daemons cannot leave orphans behind.
func runRecipe(ctx context.Context, o runOpts) error {
	// We manage the timeout and kill path ourselves (process group), so the
	// command itself is not bound to a context-cancel kill — that would
	// only kill the leader, not the group.
	cmd := exec.Command(o.argv[0], o.argv[1:]...) //nolint:gosec // argv is explicit; user-declared recipe
	cmd.Dir = o.dir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.Env = buildEnv(o.exec, o.defExec)
	configureProcessGroup(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting recipe: %w", err)
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
		return err
	case <-ctx.Done():
		killGroup(cmd)
		<-done
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("recipe timed out: %w", ctx.Err())
		}
		return fmt.Errorf("recipe cancelled: %w", ctx.Err())
	}
}

// gracePeriod is how long mdsmith waits after the first (polite)
// termination signal before force-killing the process group. It is a var,
// not a const, so a kill-path test can shorten it.
var gracePeriod = 5 * time.Second

// afterStartFn indirects afterStart so a test can install a non-nil job
// cleanup and exercise the deferred-cleanup branch on Unix.
var afterStartFn = afterStart
