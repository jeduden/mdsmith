package build

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// HookEntry is a resolved hook ready to run: a tokenized command and
// an optional display name. Params have already been expanded into the
// tokens by the caller.
type HookEntry struct {
	// Tokens is the pre-tokenized argv (command split on whitespace,
	// {param} placeholders already substituted). Tokens[0] is the
	// executable; no shell is invoked.
	Tokens []string
	// Name is the display label used in output lines. If empty the
	// caller should use the first token.
	Name string
}

// HookResult carries the exit status of a single hook.
type HookResult struct {
	// ExitCode is the process exit code, or 1 when the hook could not
	// be started.
	ExitCode int
	// Err is the underlying error if the hook failed to run or returned
	// non-zero.
	Err error
}

// RunHooks runs the supplied hooks in order. Each hook is exec'd with
// root as the working directory and ctx bounding the run. On the first
// failure RunHooks returns immediately with the failing hook's result;
// subsequent hooks are not run. A nil or empty list returns nil.
func RunHooks(ctx context.Context, hooks []HookEntry, root string, w io.Writer) *HookResult {
	for _, h := range hooks {
		if len(h.Tokens) == 0 {
			continue
		}
		name := h.Name
		if name == "" {
			name = h.Tokens[0]
		}
		announceHook(ctx, name, w)
		if result := runHook(ctx, h.Tokens, root); result != nil {
			_, _ = fmt.Fprintf(w, "hook %s: FAIL (exit %d): %v\n",
				name, result.ExitCode, result.Err)
			return result
		}
		_, _ = fmt.Fprintf(w, "hook %s: OK\n", name)
	}
	return nil
}

// RunAfterHooks runs the supplied after-hooks in order. Unlike RunHooks,
// a failure does not stop subsequent hooks — all after-hooks run, and the
// first non-zero exit code is returned at the end. If all succeed nil is
// returned.
func RunAfterHooks(ctx context.Context, hooks []HookEntry, root string, w io.Writer) *HookResult {
	var first *HookResult
	for _, h := range hooks {
		if len(h.Tokens) == 0 {
			continue
		}
		name := h.Name
		if name == "" {
			name = h.Tokens[0]
		}
		announceHook(ctx, name, w)
		if result := runHook(ctx, h.Tokens, root); result != nil {
			_, _ = fmt.Fprintf(w, "hook %s: FAIL (exit %d): %v\n",
				name, result.ExitCode, result.Err)
			if first == nil {
				first = result
			}
			continue
		}
		_, _ = fmt.Fprintf(w, "hook %s: OK\n", name)
	}
	return first
}

// announceHook prints the "running" line for the hook named name,
// unless ctx is already done: runRecipe then refuses the hook before it
// starts, and only its FAIL line is printed.
func announceHook(ctx context.Context, name string, w io.Writer) {
	if ctx.Err() != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "hook %s: running\n", name)
}

// runHook executes a single hook and returns a HookResult on failure, nil
// on success. It runs through runRecipe, so a hook gets a recipe's
// process-group isolation and group kill: a cancel (CLI interrupt) or
// the hook timeout ends the hook's children too, not only its leader.
// Unlike a recipe it keeps mdsmith's environment and runs in root.
func runHook(ctx context.Context, tokens []string, root string) *HookResult {
	code, _, err := runRecipe(ctx, runOpts{
		argv:       tokens,
		dir:        root,
		stdout:     os.Stderr,
		stderr:     os.Stderr,
		inheritEnv: true,
		label:      "hook",
	})
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		// A deadline is the hook timeout; a cancel is a CLI interrupt.
		reason := "timed out"
		if errors.Is(ctxErr, context.Canceled) {
			reason = "interrupted"
		}
		return &HookResult{ExitCode: 1, Err: fmt.Errorf("%w (%s)", ctxErr, reason)}
	}
	if code < 0 {
		code = 1 // killed by a signal, or never started
	}
	return &HookResult{ExitCode: code, Err: err}
}

// TokenizeHook splits a hook command on whitespace and substitutes {param}
// tokens from the params map. It is the same expansion used for recipes.
func TokenizeHook(command string, params map[string]string) []string {
	tokens := strings.Fields(command)
	result := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		result = append(result, substituteParams(tok, params))
	}
	return result
}
