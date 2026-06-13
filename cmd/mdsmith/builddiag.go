package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	buildexec "github.com/jeduden/mdsmith/internal/build"
	"github.com/jeduden/mdsmith/internal/config"
)

// explainTarget prints the ActionID inputs and the cache verdict for the
// target whose first declared output equals name. No recipe runs. It
// returns 0 on success or 2 when no target matches.
func explainTarget(
	targets []buildTarget, name string, cfg *config.Config,
	cache *buildexec.Cache, w io.Writer,
) int {
	want := normalizeTargetName(name)
	for _, bt := range targets {
		if len(bt.target.Outputs) == 0 {
			continue
		}
		if normalizeTargetName(bt.target.Outputs[0]) != want {
			continue
		}
		return printExplanation(bt, cfg, cache, w)
	}
	_, _ = fmt.Fprintf(w, "mdsmith: no target named %q\n", name)
	return 2
}

// normalizeTargetName slash-normalizes and cleans a target name for the
// --build-explain match.
func normalizeTargetName(p string) string {
	return filepath.ToSlash(filepath.Clean(p))
}

// printExplanation writes the full ActionID breakdown for one target.
func printExplanation(
	bt buildTarget, cfg *config.Config, cache *buildexec.Cache, w io.Writer,
) int {
	stin := stalenessFor(bt, cfg)
	ex, err := buildexec.Explain(stin)
	if err != nil {
		_, _ = fmt.Fprintf(w, "mdsmith: %v\n", err)
		return 2
	}
	_, _ = fmt.Fprintf(w, "explain %s\n", targetName(bt))
	_, _ = fmt.Fprintf(w, "  recipe.command: %s\n", ex.Command)
	_, _ = fmt.Fprintf(w, "  params:\n")
	for _, k := range sortedKeys(ex.Params) {
		_, _ = fmt.Fprintf(w, "    %s: %s\n", k, ex.Params[k])
	}
	_, _ = fmt.Fprintf(w, "  inputs:\n")
	for _, in := range ex.Inputs {
		_, _ = fmt.Fprintf(w, "    %s  %s\n", in.Path, in.Hash)
	}
	_, _ = fmt.Fprintf(w, "  outputs:\n")
	for _, o := range ex.Outputs {
		_, _ = fmt.Fprintf(w, "    %s\n", o)
	}
	_, _ = fmt.Fprintf(w, "  cache.version: %d\n", ex.CacheVersion)
	_, _ = fmt.Fprintf(w, "  action-id: %s\n", ex.ActionID)
	printVerdict(stin, cache, w)
	return 0
}

// printVerdict appends the cache verdict line and an unstable note.
func printVerdict(stin buildexec.StalenessInput, cache *buildexec.Cache, w io.Writer) {
	res, err := buildexec.CheckStaleness(stin, cache)
	if err != nil {
		_, _ = fmt.Fprintf(w, "  verdict: ERROR: %v\n", err)
		return
	}
	_, _ = fmt.Fprintf(w, "  verdict: %s\n", res.Verdict)
	if entry, ok := cache.Lookup(stin.Target.Outputs); ok && entry.Unstable {
		_, _ = fmt.Fprintf(w, "  unstable: true\n")
	}
}

// sortedKeys returns the sorted keys of a string map.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// diagTailLines is the number of trailing stream lines printed in the
// failure and timeout diagnostics.
const diagTailLines = 20

// printStreamTail writes the last diagTailLines header and body for one stream.
func printStreamTail(label string, lines []string, w io.Writer) {
	_, _ = fmt.Fprintf(w, "  --- last %d lines of %s ---\n", diagTailLines, label)
	for _, line := range lastLines(lines, diagTailLines) {
		_, _ = fmt.Fprintf(w, "  %s\n", line)
	}
}

// reportBuildFailure prints the rich diagnostic for a failed recipe. A
// timeout prints the hung-recipe block (last lines of both streams before
// SIGTERM); any other failure prints the six-field block plus the last 20
// lines of stderr. When the recipe never ran (Argv is empty, e.g. ActionID
// computation failed before dispatch), only the error is printed.
func reportBuildFailure(bt buildTarget, res targetRunResult, w io.Writer) {
	name := targetName(bt)
	if res.TimedOut {
		reportTimeout(name, res, w)
		return
	}
	_, _ = fmt.Fprintf(w, "FAIL %s (recipe: %s)\n", name, bt.target.Recipe)
	_, _ = fmt.Fprintf(w, "  source:   %s:%d <?build?>\n", bt.file, bt.line)
	if len(res.Argv) == 0 {
		if res.Err != nil {
			_, _ = fmt.Fprintf(w, "  error:    %v\n", res.Err)
		}
		return
	}
	_, _ = fmt.Fprintf(w, "  argv:     %s\n", strings.Join(res.Argv, " "))
	_, _ = fmt.Fprintf(w, "  cwd:      %s\n", res.Cwd)
	_, _ = fmt.Fprintf(w, "  exit:     %d\n", res.ExitCode)
	_, _ = fmt.Fprintf(w, "  duration: %s\n", res.Duration.Round(time.Millisecond))
	_, _ = fmt.Fprintf(w, "  log:      %s\n", relLogPath(bt.target.Root, res.LogPath))
	if res.Err != nil {
		_, _ = fmt.Fprintf(w, "  error:    %v\n", res.Err)
	}
	if len(res.StderrTail) == 0 {
		return
	}
	printStreamTail("stderr", res.StderrTail, w)
}

// reportTimeout prints the hung-recipe diagnostic before the SIGTERM that
// the context cancellation already sent.
func reportTimeout(name string, res targetRunResult, w io.Writer) {
	_, _ = fmt.Fprintf(w, "TIMEOUT %s after %s\n", name, res.Duration.Round(time.Millisecond))
	printStreamTail("stdout", res.StdoutTail, w)
	printStreamTail("stderr", res.StderrTail, w)
	_, _ = fmt.Fprintf(w, "  sent SIGTERM to process group\n")
}

// lastLines returns the last n elements of lines (or all if fewer).
func lastLines(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}

// relLogPath returns the log path relative to root for readable output,
// or the original path when it is not under root or is empty.
func relLogPath(root, logPath string) string {
	if logPath == "" {
		return "(no log)"
	}
	if rel, err := filepath.Rel(root, logPath); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return logPath
}

// verifyTarget re-runs the recipe a second time in an independent staging
// dir, diffs the declared output bytes against the first run, and sets
// res.Unstable with a warning when they differ. A mismatch is a warning,
// not a failure: some recipes embed timestamps or random seeds.
func verifyTarget(
	b buildexec.Builder, bt buildTarget, id string,
	opts buildPassOpts, timeout time.Duration, res *targetRunResult, w io.Writer,
) {
	first := snapshotOutputs(bt)

	vctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	verifyOpts := buildexec.Options{TargetName: targetName(bt)}
	if id != "" {
		verifyOpts.LogRoot = bt.target.Root
		verifyOpts.ActionID = "verify-" + id
	}
	if opts.stream {
		verifyOpts.LiveSink = w
	}
	second := b.BuildWithResult(vctx, bt.target, verifyOpts)
	if second.Err != nil {
		_, _ = fmt.Fprintf(w, "WARN %s: verify re-run failed: %v\n", targetName(bt), second.Err)
		res.Unstable = true
		return
	}
	if !outputsEqual(first, snapshotOutputs(bt)) {
		_, _ = fmt.Fprintf(w,
			"WARN %s: non-deterministic output (two runs differ); marking unstable\n",
			targetName(bt))
		res.Unstable = true
	}
}

// snapshotOutputs reads every declared output of bt from disk into a
// path→bytes map. A missing output maps to nil.
func snapshotOutputs(bt buildTarget) map[string][]byte {
	out := make(map[string][]byte, len(bt.target.Outputs))
	for _, rel := range bt.target.Outputs {
		abs := filepath.Join(bt.target.Root, filepath.FromSlash(rel))
		data, err := os.ReadFile(abs) //nolint:gosec // abs is an in-root declared output
		if err != nil {
			out[rel] = nil
			continue
		}
		out[rel] = data
	}
	return out
}

// outputsEqual reports whether two output snapshots hold identical bytes
// for every key.
func outputsEqual(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || !bytes.Equal(av, bv) {
			return false
		}
	}
	return true
}
