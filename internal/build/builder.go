// Package build runs <?build?> directives during `mdsmith fix`. Each
// directive names a user-declared recipe in build.recipes; the
// CustomBuilder dispatches that recipe via os/exec with an explicit
// argv (no shell) and writes every declared output atomically through a
// per-target staging directory.
//
// The build pass is CLI-only. It is not part of the public pkg/mdsmith
// Session API and is excluded from the WASM bindings and the LSP/merge
// in-memory fix paths, all of which must never exec a process.
package build

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	buildrule "github.com/jeduden/mdsmith/internal/rules/build"
)

// mkdirTempFn is the os.MkdirTemp implementation; tests may replace it.
var mkdirTempFn = os.MkdirTemp

// builderGlobCapFn is the CheckGlobMatchCap implementation; tests may replace it.
var builderGlobCapFn = buildrule.CheckGlobMatchCap

// snapshotDirsFn is the snapshotDirs implementation; tests may replace it to
// inject snapshot failures in the before/after post-condition scan.
var snapshotDirsFn = snapshotDirs

// lstatFn is the os.Lstat implementation; tests may replace it to inject a
// stat failure when inspecting an output destination.
var lstatFn = os.Lstat

// renameFn is the os.Rename implementation; tests may replace it to force the
// cross-device copy fallback in commitOutputs.
var renameFn = os.Rename

// copyFileImplFn is the copyFile implementation used by the rename fallback;
// tests may replace it to inject a copy failure.
var copyFileImplFn = func(src, dst string) error {
	return copyFile(src, dst)
}

// RecipeSpec is the resolved command and tokenized argv for one
// user-declared recipe. Tokenization happens once at construction so a
// param value containing whitespace can never re-split.
type RecipeSpec struct {
	// Command is the raw recipe command string from build.recipes.
	Command string
}

// Target is one <?build?> directive to execute. Paths are stored
// project-root-relative and slash-normalized so they are stable across
// clone locations; absolute paths are recomposed against Root at exec
// time.
type Target struct {
	Recipe  string            // recipe name declared in build.recipes
	Params  map[string]string // named params from the directive
	Root    string            // absolute project root
	Inputs  []string          // project-root-relative, slash-normalized
	Outputs []string          // project-root-relative, slash-normalized
}

// Options controls stream capture and live forwarding for one
// BuildWithResult call. A zero value disables log capture (LogRoot empty)
// and live forwarding (LiveSink nil).
type Options struct {
	ActionID   string    // ActionID; names the log file <action-id>.log
	LogRoot    string    // project root holding .mdsmith/build-logs/
	LiveSink   io.Writer // when non-nil, each line is forwarded here, prefixed
	TargetName string    // prefix used on LiveSink lines (the target name)
	AllFinals  []string  // all declared finals across concurrent targets; for concurrent-safe post-condition check
}

// Result is the rich outcome of one recipe run: the argv it ran, the
// cwd, the process exit code, wall-clock duration, the log file path, and
// the captured stdout/stderr tails. Err is non-nil on any failure
// (non-zero exit, timeout, or a staging/commit error).
type Result struct {
	Argv       []string
	Cwd        string
	ExitCode   int
	Duration   time.Duration
	LogPath    string
	StdoutTail []string
	StderrTail []string
	TimedOut   bool
	Err        error
}

// Builder dispatches a single build Target.
type Builder interface {
	Build(ctx context.Context, target Target) error
	BuildWithResult(ctx context.Context, target Target, opts Options) Result
}

// CustomBuilder is the sole Builder implementation. It dispatches a
// directive's recipe command via os/exec under a hermetic environment,
// staging every output in a random-suffixed per-recipe dir, and
// committing it with a symlink-safe atomic rename.
type CustomBuilder struct {
	recipes map[string]RecipeSpec
	exec    ExecConfig
}

// NewCustomBuilder returns a CustomBuilder over the given recipe map. The
// build executor runs every recipe under the compiled-default hermetic
// environment (PATH /usr/bin:/bin, pass-through [HOME, LANG, LC_ALL]).
// Use NewCustomBuilderExec to override those defaults from config.
func NewCustomBuilder(recipes map[string]RecipeSpec) *CustomBuilder {
	return &CustomBuilder{recipes: recipes}
}

// NewCustomBuilderExec returns a CustomBuilder whose recipes run under
// the given exec configuration. An empty ExecConfig field falls back to
// the compiled default at exec time.
func NewCustomBuilderExec(recipes map[string]RecipeSpec, ec ExecConfig) *CustomBuilder {
	return &CustomBuilder{recipes: recipes, exec: ec}
}

var _ Builder = (*CustomBuilder)(nil)

// Build resolves the target's inputs and outputs against the project
// root, stages every output in a random-suffixed per-recipe dir under
// .mdsmith/build-staging/, runs the recipe under a hermetic environment
// with that dir as its working directory, verifies the output
// post-conditions, and commits the staged files with symlink-safe atomic
// renames. On any failure before the commit phase the staging dir is
// removed and no declared output is touched.
func (b *CustomBuilder) Build(ctx context.Context, target Target) error {
	return b.BuildWithResult(ctx, target, Options{}).Err
}

// BuildWithResult runs the target's recipe like Build but captures the
// recipe's stdout/stderr (to a log file under opts.LogRoot when set, and
// to in-memory tails) and returns a rich Result for diagnostics. The
// staging and atomic-commit contract is identical to Build.
func (b *CustomBuilder) BuildWithResult(
	ctx context.Context, target Target, opts Options,
) Result {
	res := Result{}
	tokens, err := b.buildArgv(target)
	if err != nil {
		res.Err = err
		return res
	}

	plan, cleanup, err := b.stage(target)
	if err != nil {
		res.Err = err
		return res
	}
	defer cleanup()

	argv := expandArgv(tokens, target.Params, plan.absInputs, plan.stagePaths)
	res.Argv = argv
	res.Cwd = plan.stageDir

	// Open the log file before the before-snapshot so that creating
	// .mdsmith/build-logs/ does not appear as an undeclared write.
	sc, logPath, err := openCapture(opts)
	if err != nil {
		res.Err = err
		return res
	}
	if sc != nil {
		res.LogPath = logPath
		defer sc.Close() //nolint:errcheck // log-file close is best-effort
	}

	before, err := snapshotDirsFn(plan.parents, snapshotCap, nil)
	if err != nil {
		res.Err = err
		return res
	}

	if runErr := b.execWithCapture(ctx, plan, argv, sc, &res); runErr != nil {
		res.Err = fmt.Errorf("recipe %q failed: %w", target.Recipe, runErr)
		return res
	}
	if err := verifyOutputsExist(plan.outputs, plan.stagePaths); err != nil {
		res.Err = err
		return res
	}
	if err := verifyNoUndeclaredWrites(before, plan.parents, plan.finals, opts.AllFinals); err != nil {
		res.Err = err
		return res
	}
	if err := commitOutputs(plan.finals, plan.outputs, plan.stagePaths); err != nil {
		res.Err = err
		return res
	}
	return res
}

// buildArgv looks up the recipe command for target and tokenizes it into argv.
func (b *CustomBuilder) buildArgv(target Target) ([]string, error) {
	spec, ok := b.recipes[target.Recipe]
	if !ok {
		return nil, fmt.Errorf("unknown recipe %q", target.Recipe)
	}
	tokens := strings.Fields(spec.Command)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("recipe %q has an empty command", target.Recipe)
	}
	return tokens, nil
}

// openCapture creates a streamCapture for the given options. Returns nil,"",nil
// when opts has no log root or action ID (capture disabled).
func openCapture(opts Options) (*streamCapture, string, error) {
	if opts.LogRoot == "" || opts.ActionID == "" {
		return nil, "", nil
	}
	p := logPathFor(opts.LogRoot, opts.ActionID)
	sc, err := newStreamCapture(p, opts.TargetName, opts.LiveSink)
	return sc, p, err
}

// execWithCapture runs the recipe argv and populates timing and stream-tail
// fields on res. It returns the raw execution error without wrapping.
func (b *CustomBuilder) execWithCapture(
	ctx context.Context, plan buildPlan, argv []string, sc *streamCapture, res *Result,
) error {
	var stdout, stderr io.Writer
	if sc != nil {
		stdout = sc.stdout()
		stderr = sc.stderr()
	}
	start := time.Now()
	exitCode, timedOut, err := runRecipe(ctx, runOpts{
		argv:    argv,
		dir:     plan.stageDir,
		exec:    b.exec,
		defExec: defaultExecConfig(),
		stdout:  stdout,
		stderr:  stderr,
	})
	res.Duration = time.Since(start)
	res.ExitCode = exitCode
	res.TimedOut = timedOut
	if sc != nil {
		res.StdoutTail = sc.stdoutTail()
		res.StderrTail = sc.stderrTail()
	}
	return err
}

// buildPlan holds the resolved paths for one Build call: the recipe's
// inputs/outputs (root-relative), their absolute forms, the per-output
// staging paths, and the output parent dirs the post-condition snapshot
// covers.
type buildPlan struct {
	outputs    []string // root-relative declared outputs
	finals     []string // absolute destinations, one per output
	stagePaths []string // absolute staging paths, one per output
	absInputs  []string // absolute resolved inputs
	parents    []string // de-duplicated output parent dirs
	stageDir   string   // the per-recipe staging dir
}

// stage resolves the target's inputs and outputs, validates and creates
// the per-recipe staging dir, and computes every absolute path Build
// needs. It returns a cleanup closure that removes the staging dir; the
// caller must defer it.
func (b *CustomBuilder) stage(target Target) (buildPlan, func(), error) {
	inputs, err := b.resolveInputs(target)
	if err != nil {
		return buildPlan{}, func() {}, err
	}
	outputs, err := b.resolveOutputs(target)
	if err != nil {
		return buildPlan{}, func() {}, err
	}

	stagingRoot, err := ensureStagingRoot(target.Root)
	if err != nil {
		return buildPlan{}, func() {}, err
	}
	stageDir, err := mkdirTempFn(stagingRoot, "recipe-")
	if err != nil {
		return buildPlan{}, func() {}, fmt.Errorf("creating staging dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(stageDir) }

	plan := buildPlan{
		outputs:    outputs,
		finals:     make([]string, len(outputs)),
		stagePaths: make([]string, len(outputs)),
		absInputs:  make([]string, len(inputs)),
		stageDir:   stageDir,
	}
	for i, rel := range outputs {
		// A flat file named by index, so a recipe writing to {outputs}[i]
		// writes inside the staging dir.
		plan.stagePaths[i] = filepath.Join(stageDir, fmt.Sprintf("out%d", i))
		plan.finals[i] = filepath.Join(target.Root, filepath.FromSlash(rel))
	}
	for i, in := range inputs {
		plan.absInputs[i] = filepath.Join(target.Root, filepath.FromSlash(in))
	}
	plan.parents = outputParents(plan.finals)
	return plan, cleanup, nil
}

// outputParents returns the de-duplicated set of parent directories of
// the declared output paths — the scope the post-condition snapshot
// covers.
func outputParents(finals []string) []string {
	seen := make(map[string]struct{}, len(finals))
	var out []string
	for _, f := range finals {
		dir := filepath.Dir(f)
		if _, dup := seen[dir]; dup {
			continue
		}
		seen[dir] = struct{}{}
		out = append(out, dir)
	}
	return out
}

// verifyOutputsExist confirms every declared output was produced in the
// staging dir as a regular file. A recipe that exits 0 without writing a
// declared output is a build failure; so is one that stages a symlink or
// a directory in place of the output, since committing a symlink would
// redirect the artifact's bytes outside the staging isolation.
func verifyOutputsExist(outputs, stagePaths []string) error {
	for i, rel := range outputs {
		info, err := os.Lstat(stagePaths[i])
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("recipe exited 0 but did not produce declared output %q", rel)
			}
			return fmt.Errorf("staging output %q: %w", rel, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("recipe staged declared output %q as a symlink; refusing to commit it", rel)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("recipe staged declared output %q as a non-regular file", rel)
		}
	}
	return nil
}

// verifyNoUndeclaredWrites re-snapshots the output parent dirs and fails
// the build if any file outside the declared finals was added, removed,
// or modified by the recipe. extraDeclared lists additional paths (e.g.
// from concurrent targets) that are exempt from the undeclared-write check.
func verifyNoUndeclaredWrites(before map[string]fileState, parents, finals, extraDeclared []string) error {
	after, err := snapshotDirsFn(parents, snapshotCap, before)
	if err != nil {
		return err
	}
	declared := make(map[string]struct{}, len(finals)+len(extraDeclared))
	for _, f := range finals {
		declared[f] = struct{}{}
	}
	for _, f := range extraDeclared {
		declared[f] = struct{}{}
	}
	violations := diffSnapshots(before, after, declared)
	if len(violations) == 0 {
		return nil
	}
	names := make([]string, 0, len(violations))
	for _, v := range violations {
		names = append(names, fmt.Sprintf("%s (%s)", v.path, v.kind))
	}
	return fmt.Errorf(
		"recipe wrote outside its declared outputs: %s", strings.Join(names, ", "),
	)
}

// resolveInputs resolves every inputs: entry. A literal entry is
// re-checked against the project root; a glob entry is expanded with
// doublestar, capped, and each match re-checked. Results are sorted and
// de-duplicated.
func (b *CustomBuilder) resolveInputs(target Target) ([]string, error) {
	seen := make(map[string]struct{})
	var out []string
	add := func(rel string) {
		if _, dup := seen[rel]; !dup {
			seen[rel] = struct{}{}
			out = append(out, rel)
		}
	}
	root := target.Root
	fsys := os.DirFS(root)
	for _, entry := range target.Inputs {
		if isGlob(entry) {
			matches, err := doublestar.Glob(fsys, entry)
			if err != nil {
				return nil, fmt.Errorf("inputs glob %q: %w", entry, err)
			}
			if err := builderGlobCapFn(len(matches)); err != nil {
				return nil, err
			}
			for _, m := range matches {
				rel, err := buildrule.ResolvePathInRoot(root, m, true)
				if err != nil {
					return nil, err
				}
				add(rel)
			}
			continue
		}
		rel, err := buildrule.ResolvePathInRoot(root, entry, true)
		if err != nil {
			return nil, err
		}
		add(rel)
	}
	sort.Strings(out)
	return out, nil
}

// resolveOutputs re-checks every outputs: entry against the project
// root. Outputs may not exist yet, so mustExist is false. An output under
// .mdsmith/ is refused here, not just in the MDS039 lint rule: the build
// pass must never let a recipe overwrite mdsmith's own state (the build
// cache, the trust marker, kinds/schemas/conventions) even when MDS039 is
// disabled or overridden in config.
func (b *CustomBuilder) resolveOutputs(target Target) ([]string, error) {
	out := make([]string, 0, len(target.Outputs))
	for _, entry := range target.Outputs {
		rel, err := buildrule.ResolvePathInRoot(target.Root, entry, false)
		if err != nil {
			return nil, err
		}
		if buildrule.UnderMdsmithDir(rel) {
			return nil, fmt.Errorf("output %q is under .mdsmith/; refusing to overwrite mdsmith state", rel)
		}
		out = append(out, rel)
	}
	return out, nil
}

// isGlob reports whether a path entry contains doublestar glob meta.
func isGlob(p string) bool {
	return strings.ContainsAny(p, "*?[{")
}

// expandArgv substitutes {param}, {inputs}, and {outputs} into the
// already-tokenized command. A {param} token is replaced in place; the
// collective {inputs} and {outputs} tokens each expand to one argv per
// resolved entry. Substitution happens after tokenization, so a param
// value containing whitespace stays a single argv entry.
func expandArgv(
	tokens []string, params map[string]string, inputs, outputs []string,
) []string {
	// {inputs}/{outputs} expand to one argv per entry, so size for the
	// worst case up front.
	argv := make([]string, 0, len(tokens)+len(inputs)+len(outputs))
	for _, tok := range tokens {
		switch tok {
		case "{inputs}":
			argv = append(argv, inputs...)
		case "{outputs}":
			argv = append(argv, outputs...)
		default:
			argv = append(argv, substituteParams(tok, params))
		}
	}
	return argv
}

// substituteParams replaces {name} placeholders in a single token with
// the matching param value. {inputs} and {outputs} are handled by the
// caller before this point, so a bare {inputs}/{outputs} embedded in a
// larger token is left untouched here (the MDS040 command validator
// already rejects embedded list placeholders).
func substituteParams(tok string, params map[string]string) string {
	if !strings.ContainsRune(tok, '{') {
		return tok
	}
	var b strings.Builder
	i := 0
	for i < len(tok) {
		if tok[i] != '{' {
			b.WriteByte(tok[i])
			i++
			continue
		}
		close := strings.IndexByte(tok[i:], '}')
		if close < 0 {
			b.WriteString(tok[i:])
			break
		}
		name := tok[i+1 : i+close]
		if name == "inputs" || name == "outputs" {
			// Embedded list placeholder; pass through literally.
			b.WriteString(tok[i : i+close+1])
			i += close + 1
			continue
		}
		v, ok := params[name]
		if !ok {
			// Optional params that are absent expand to empty.
			v = ""
		}
		b.WriteString(v)
		i += close + 1
	}
	return b.String()
}

// commitOutputs renames each staged file to its final destination,
// creating parent directories as needed. Before each replace it Lstats
// the destination and refuses to overwrite a symlink — a symlink there
// could redirect the write outside the project tree. The replace is
// atomic per file (os.Rename → rename(2) / MoveFileEx). Multi-output
// commit is *not* transactional: if rename N+1 fails after N succeeded,
// mdsmith reports the partial state and returns an error; the caller
// removes the staging dir and exits FAIL, and because no cache entry was
// written the next `fix` reruns the whole recipe.
func commitOutputs(finals, outputs, stagePaths []string) error {
	for i, rel := range outputs {
		stage := stagePaths[i]
		final := finals[i]
		if err := refuseSymlinkDest(final, rel); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
			return fmt.Errorf("creating output dir for %q: %w", rel, err)
		}
		if err := renameFn(stage, final); err != nil {
			// Cross-device rename can fail; fall back to copy.
			if cerr := copyFileImplFn(stage, final); cerr != nil {
				if i > 0 {
					return fmt.Errorf(
						"writing output %q (outputs 0..%d already committed; rerun fix): %w",
						rel, i-1, cerr,
					)
				}
				return fmt.Errorf("writing output %q: %w", rel, cerr)
			}
		}
	}
	return nil
}

// refuseSymlinkDest fails when the output destination is an existing
// symlink. Overwriting a symlink with os.Rename replaces the link itself,
// but a copy fallback (cross-device) would follow it and write through
// to the link target — possibly outside the project tree. Refusing up
// front keeps both paths safe and gives a clear diagnostic.
func refuseSymlinkDest(final, rel string) error {
	info, err := lstatFn(final)
	if err != nil {
		// ErrNotExist: nothing to replace. ENOTDIR: a parent component is a
		// file — there is no symlink at the destination, and the subsequent
		// MkdirAll reports the parent problem with a clearer message. Any
		// other error is surfaced.
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			return nil
		}
		return fmt.Errorf("inspecting output %q: %w", rel, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to replace output %q: it is a symlink", rel)
	}
	return nil
}

// copyFile copies src to dst, preserving nothing but content. Used as a
// fallback when os.Rename fails across filesystems (the staging temp
// dir may be on a different device than the project root). Content is
// streamed so a large artifact never has to fit in memory.
func copyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // src is a staged path under our temp dir
	if err != nil {
		return err
	}
	defer in.Close() //nolint:errcheck // read-only file
	// Mirror the staged file's permissions so the copy fallback matches
	// what os.Rename would have preserved.
	mode := os.FileMode(0o644)
	if info, err := in.Stat(); err == nil {
		mode = info.Mode().Perm()
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode) //nolint:gosec // mode from stage
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close() // copy error takes precedence
		return err
	}
	return out.Close()
}
