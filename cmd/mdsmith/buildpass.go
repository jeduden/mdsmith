package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	buildexec "github.com/jeduden/mdsmith/internal/build"
	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

// buildPassOpts bundles the build-pass knobs parsed from the fix CLI
// flags. The build pass is CLI-only — it lives here in cmd/mdsmith and
// never runs from pkg/mdsmith, the WASM bindings, the LSP fix path, or
// the merge driver.
type buildPassOpts struct {
	noBuild            bool          // --no-build: skip the build pass entirely
	buildOnly          bool          // --build-only: run only the build pass
	recipe             string        // --build-recipe: only this recipe's directives
	dryRun             bool          // --build-dry-run: enumerate targets, run nothing
	force              bool          // --build-force: rebuild every target
	checkStale         bool          // --build-check-stale: report stale targets, run nothing
	noCache            bool          // --build-no-cache: ignore and do not write the cache
	timeout            time.Duration // --build-timeout: per-recipe timeout
	maxBytes           int64         // file-size cap inherited from the fix run
	noHooks            bool          // --build-no-hooks: skip both before/after hook lists
	skipHooksWhenFresh bool          // --build-skip-hooks-when-fresh: skip hooks when no target is stale
}

// buildTarget pairs a resolved build.Target with the file and line it
// came from, so the per-target summary can name the source directive.
type buildTarget struct {
	file   string
	line   int
	target buildexec.Target
}

// checkMDS040Gate runs MDS040 (recipe-safety) against the current config and
// returns true (gate open) when no errors are found. If errors exist it prints
// each one to w and returns false. The gate is skipped when cfg.Rules does not
// have "recipe-safety" enabled (i.e. MDS040 is disabled by the user).
func checkMDS040Gate(cfg *config.Config, cfgPath string, w io.Writer) bool {
	rc, ok := cfg.Rules["recipe-safety"]
	if !ok || !rc.Enabled {
		return true
	}
	r := rule.ByID("MDS040")
	if r == nil {
		return true
	}
	clone := rule.CloneRule(r)
	c, ok := clone.(interface {
		ApplySettings(map[string]any) error
		Check(f *lint.File) []lint.Diagnostic
	})
	if !ok {
		return true
	}
	if rc.Settings != nil {
		if err := c.ApplySettings(rc.Settings); err != nil {
			_, _ = fmt.Fprintf(w, "mdsmith: MDS040 settings error: %v\n", err)
			return false
		}
	}
	// Use an empty config file as the synthetic lint target. MDS040 is a
	// ConfigTarget rule: it checks only when f.Path matches its ConfigPath.
	f, _ := lint.NewFile(cfgPath, []byte(""))
	diags := c.Check(f)
	hasErrors := false
	for _, d := range diags {
		if d.Severity == lint.Error {
			_, _ = fmt.Fprintf(w, "%s:%d: %s [%s]\n", d.File, d.Line, d.Message, d.RuleID)
			hasErrors = true
		}
	}
	return !hasErrors
}

// runBuildPass collects every <?build?> directive across files, then
// dispatches each to its recipe's CustomBuilder. It prints a per-target
// OK | FAIL summary on w and returns a non-zero exit code on any build
// failure. When opts.dryRun is set it enumerates targets without running
// any recipe.
//
// The pass is a no-op (exit 0) when no recipes are declared and no build
// directives exist, so a fix run on a repo with no builds is unchanged.
func runBuildPass(
	cfg *config.Config, cfgPath string, files []string, opts buildPassOpts, w io.Writer,
) int {
	root := rootDirFromConfig(cfgPath)

	// Gate: refuse to run if MDS040 emits any error against build.hooks or
	// build.recipes. --no-build bypasses this check (no build pass runs).
	if !checkMDS040Gate(cfg, cfgPath, w) {
		return 2
	}

	// Filter out ignored files so the build pass honours the same
	// cfg.Ignore patterns the lint pass applies internally. When a
	// directory argument is expanded (e.g. "mdsmith fix .") the file
	// list may include fixture or demo files that lint skips silently;
	// without this filter the build pass would try to dispatch directives
	// in those files, failing on undefined recipes.
	nonIgnored := make([]string, 0, len(files))
	for _, f := range files {
		if !config.IsIgnored(cfg.Ignore, workspaceRelativePath(f, root)) {
			nonIgnored = append(nonIgnored, f)
		}
	}

	recipes := buildRecipeSpecs(cfg)

	targets, errs := collectBuildTargets(nonIgnored, root, opts.recipe, opts.maxBytes)
	for _, err := range errs {
		_, _ = fmt.Fprintf(w, "mdsmith: %v\n", err)
	}

	// Overlapping outputs across directives is a hard error: run no recipe.
	if err := detectOverlap(targets); err != nil {
		_, _ = fmt.Fprintf(w, "mdsmith: %v\n", err)
		return 2
	}

	if len(targets) == 0 {
		if len(errs) > 0 {
			return 2
		}
		return 0
	}

	builder := buildexec.NewCustomBuilder(recipes)
	timeout := opts.timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	cache := loadBuildCache(root, opts, w)
	return dispatchWithHooks(builder, targets, cfg, root, opts, cache, timeout, errs, w)
}

// dispatchWithHooks coordinates the hook lifecycle around target dispatch:
// resolve whether hooks run, execute before-hooks (aborting on failure),
// dispatch recipe targets, then execute after-hooks. It returns the combined
// exit code following the priority rules (collection > before > recipe > after).
func dispatchWithHooks(
	builder buildexec.Builder, targets []buildTarget, cfg *config.Config,
	root string, opts buildPassOpts, cache *buildexec.Cache,
	timeout time.Duration, errs []error, w io.Writer,
) int {
	// Resolve whether to skip hooks. Dry-run and check-stale never run hooks.
	runHooks := !opts.noHooks && !opts.dryRun && !opts.checkStale

	// --build-skip-hooks-when-fresh: skip hooks only when every target is
	// fresh (i.e. nothing would rebuild). Evaluate staleness without running
	// recipes first.
	if runHooks && opts.skipHooksWhenFresh && allFresh(targets, cfg, cache, opts) {
		runHooks = false
	}

	// Run before-hooks. A failure aborts the recipe pass and after-hooks.
	// Use the same per-operation timeout as recipes so a hung hook does not
	// block the build pass indefinitely.
	if runHooks && len(cfg.Build.Hooks.Before) > 0 {
		before := resolveHooks(cfg.Build.Hooks.Before)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		result := buildexec.RunHooks(ctx, before, root, w)
		cancel()
		if result != nil {
			// Collection errors (len(errs)>0) take priority: always exit 2.
			if len(errs) > 0 {
				return 2
			}
			return result.ExitCode
		}
	}

	// Dry-run: list hooks alongside recipes, run nothing.
	if opts.dryRun {
		listHooksForDryRun("before", cfg.Build.Hooks.Before, w)
	}

	code := dispatchTargets(builder, targets, cfg, root, opts, cache, timeout, w)

	// Dry-run: list after-hooks.
	if opts.dryRun {
		listHooksForDryRun("after", cfg.Build.Hooks.After, w)
	}

	// Run after-hooks regardless of recipe result (but not on abort from before-fail).
	afterCode := 0
	if runHooks && len(cfg.Build.Hooks.After) > 0 {
		after := resolveHooks(cfg.Build.Hooks.After)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		result := buildexec.RunAfterHooks(ctx, after, root, w)
		cancel()
		if result != nil {
			afterCode = result.ExitCode
		}
	}

	if len(errs) > 0 && code == 0 && afterCode == 0 {
		return 2
	}
	if code != 0 {
		return code
	}
	return afterCode
}

// resolveHooks converts []config.HookCfg to []buildexec.HookEntry by
// tokenizing each command and substituting params.
func resolveHooks(hooks []config.HookCfg) []buildexec.HookEntry {
	out := make([]buildexec.HookEntry, 0, len(hooks))
	for _, h := range hooks {
		tokens := buildexec.TokenizeHook(h.Command, h.Params)
		if len(tokens) == 0 {
			continue
		}
		out = append(out, buildexec.HookEntry{
			Tokens: tokens,
			Name:   h.Name,
		})
	}
	return out
}

// allFresh returns true when every target's staleness verdict is Fresh.
// This is used to decide whether to skip hooks under --build-skip-hooks-when-fresh.
// --build-force and --build-no-cache always force Stale, so hooks run unconditionally.
func allFresh(targets []buildTarget, cfg *config.Config, cache *buildexec.Cache, opts buildPassOpts) bool {
	if opts.force || opts.noCache {
		return false
	}
	for _, bt := range targets {
		stin := stalenessFor(bt, cfg)
		verdict, err := buildexec.CheckStaleness(stin, cache)
		if err != nil {
			return false
		}
		if verdict.Verdict != buildexec.Fresh {
			return false
		}
	}
	return true
}

// listHooksForDryRun writes one line per hook to w, prefixed with listName
// ("before" or "after"). Used during --build-dry-run.
func listHooksForDryRun(listName string, hooks []config.HookCfg, w io.Writer) {
	for i, h := range hooks {
		name := h.Name
		if name == "" {
			if fields := strings.Fields(h.Command); len(fields) > 0 {
				name = fields[0]
			}
		}
		_, _ = fmt.Fprintf(w, "hook %s[%d] %s: DRY-RUN\n", listName, i, name)
	}
}

// detectOverlap maps the collected targets to OverlapTargets and runs the
// build package's overlap detector.
func detectOverlap(targets []buildTarget) error {
	ot := make([]buildexec.OverlapTarget, 0, len(targets))
	for _, bt := range targets {
		ot = append(ot, buildexec.OverlapTarget{
			File:    bt.file,
			Line:    bt.line,
			Outputs: bt.target.Outputs,
		})
	}
	return buildexec.DetectOutputOverlap(ot)
}

// loadBuildCache loads the build cache unless --build-no-cache is set. A
// corrupt cache is reported but treated as empty so the run can proceed
// (every target then rebuilds).
func loadBuildCache(root string, opts buildPassOpts, w io.Writer) *buildexec.Cache {
	if opts.noCache {
		return buildexec.NewCache()
	}
	cache, err := buildexec.LoadCache(root)
	if err != nil {
		_, _ = fmt.Fprintf(w, "mdsmith: %v; treating all targets as stale\n", err)
		return buildexec.NewCache()
	}
	return cache
}

// stalenessFor builds the StalenessInput for one target, resolving the
// recipe's default-inputs (param tokens to their relative path values).
func stalenessFor(bt buildTarget, cfg *config.Config) buildexec.StalenessInput {
	recipeCfg := cfg.Build.Recipes[bt.target.Recipe]
	defaults := resolveDefaultInputs(recipeCfg.DefaultInputs, bt.target.Params)
	return buildexec.StalenessInput{
		Target:        bt.target,
		Command:       recipeCfg.Command,
		DefaultInputs: defaults,
	}
}

// resolveDefaultInputs maps each default-inputs entry to a relative path.
// A {param} token resolves to the param's value (the relative path it
// supplies); a literal entry passes through unchanged.
func resolveDefaultInputs(entries []string, params map[string]string) []string {
	if len(entries) == 0 {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if len(e) > 2 && e[0] == '{' && e[len(e)-1] == '}' {
			name := e[1 : len(e)-1]
			if v, ok := params[name]; ok {
				out = append(out, v)
				continue
			}
		}
		out = append(out, e)
	}
	return out
}

// targetOutcome is the per-target result of one dispatch loop iteration.
type targetOutcome int

const (
	outcomeNeutral targetOutcome = iota // reported, no state change (dry-run, skip, fresh-stale-report)
	outcomeFailed                       // a failure was reported
	outcomeStale                        // --build-check-stale found this target stale
	outcomeRebuilt                      // recipe ran and the cache entry was refreshed
)

// dispatchTargets runs the staleness check, dispatch, and cache refresh
// loop. It returns the build pass exit code.
func dispatchTargets(
	builder buildexec.Builder, targets []buildTarget, cfg *config.Config,
	root string, opts buildPassOpts, cache *buildexec.Cache,
	timeout time.Duration, w io.Writer,
) int {
	failed := false
	anyStale := false
	rebuilt := false
	for _, bt := range targets {
		switch dispatchOne(builder, bt, cfg, opts, cache, timeout, w) {
		case outcomeFailed:
			failed = true
		case outcomeStale:
			anyStale = true
		case outcomeRebuilt:
			rebuilt = true
		case outcomeNeutral:
		}
	}

	if opts.checkStale {
		if anyStale {
			return 2
		}
		return 0
	}
	if rebuilt && !opts.noCache {
		if err := cache.Save(root); err != nil {
			_, _ = fmt.Fprintf(w, "mdsmith: saving build cache: %v\n", err)
			return 2
		}
	}
	if failed {
		return 2
	}
	return 0
}

// dispatchOne handles a single target: staleness verdict, then report or
// rebuild per the active flags. It returns the outcome the caller folds
// into the run-level state.
func dispatchOne(
	builder buildexec.Builder, bt buildTarget, cfg *config.Config,
	opts buildPassOpts, cache *buildexec.Cache, timeout time.Duration, w io.Writer,
) targetOutcome {
	label := fmt.Sprintf("%s:%d (%s)", bt.file, bt.line, bt.target.Recipe)
	stin := stalenessFor(bt, cfg)

	verdict, serr := targetVerdict(stin, cache, opts)
	if serr != nil {
		_, _ = fmt.Fprintf(w, "build %s: FAIL: %v\n", label, serr)
		return outcomeFailed
	}

	if opts.checkStale {
		if verdict == buildexec.Stale {
			_, _ = fmt.Fprintf(w, "build %s: STALE\n", label)
			return outcomeStale
		}
		return outcomeNeutral
	}
	if opts.dryRun {
		_, _ = fmt.Fprintf(w, "build %s: %s\n", label, verdict)
		return outcomeNeutral
	}
	if verdict == buildexec.Fresh {
		_, _ = fmt.Fprintf(w, "build %s: SKIP\n", label)
		return outcomeNeutral
	}
	if err := runOneTarget(builder, bt, timeout); err != nil {
		_, _ = fmt.Fprintf(w, "build %s: FAIL: %v\n", label, err)
		return outcomeFailed
	}
	if err := refreshCacheEntry(stin, cache, opts); err != nil {
		_, _ = fmt.Fprintf(w, "build %s: FAIL: %v\n", label, err)
		return outcomeFailed
	}
	_, _ = fmt.Fprintf(w, "build %s: OK\n", label)
	return outcomeRebuilt
}

// targetVerdict returns the staleness verdict for one target, honouring
// --build-force and --build-no-cache (both force a Stale verdict).
func targetVerdict(
	stin buildexec.StalenessInput, cache *buildexec.Cache, opts buildPassOpts,
) (buildexec.Verdict, error) {
	if opts.force || opts.noCache {
		// Still resolve inputs so a missing input or empty glob is an error.
		if _, err := buildexec.CheckStaleness(stin, buildexec.NewCache()); err != nil {
			return buildexec.Stale, err
		}
		return buildexec.Stale, nil
	}
	res, err := buildexec.CheckStaleness(stin, cache)
	if err != nil {
		return buildexec.Stale, err
	}
	return res.Verdict, nil
}

// refreshCacheEntry records a rebuilt target's cache entry, stamping
// built-at. With --build-no-cache it is a no-op.
func refreshCacheEntry(
	stin buildexec.StalenessInput, cache *buildexec.Cache, opts buildPassOpts,
) error {
	if opts.noCache {
		return nil
	}
	entry, err := buildexec.RecordBuild(stin)
	if err != nil {
		return err
	}
	entry.BuiltAt = time.Now().UTC().Format(time.RFC3339)
	cache.Put(entry)
	return nil
}

// runOneTarget dispatches a single target with a per-recipe timeout.
func runOneTarget(b buildexec.Builder, bt buildTarget, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return b.Build(ctx, bt.target)
}

// buildRecipeSpecs converts the loaded config's build.recipes into the
// build package's RecipeSpec map.
func buildRecipeSpecs(cfg *config.Config) map[string]buildexec.RecipeSpec {
	out := make(map[string]buildexec.RecipeSpec, len(cfg.Build.Recipes))
	for name, r := range cfg.Build.Recipes {
		out[name] = buildexec.RecipeSpec{Command: r.Command}
	}
	return out
}

// collectBuildTargets parses each file, walks its <?build?> directives,
// and turns each well-formed one into a buildTarget. A directive missing
// its required recipe/outputs is skipped (MDS039 already reports it as a
// lint error); a recipe filter restricts the set to one recipe name.
// Returns the targets in (file, line) order plus any per-file read or
// parse errors.
func collectBuildTargets(
	files []string, root, recipeFilter string, maxBytes int64,
) ([]buildTarget, []error) {
	var targets []buildTarget
	var errs []error
	for _, path := range files {
		src, err := bytelimit.ReadFileLimited(path, maxBytes)
		if err != nil {
			errs = append(errs, fmt.Errorf("reading %s: %w", path, err))
			continue
		}
		f, _ := lint.NewFile(path, src) // NewFile never errors; goldmark always produces an AST
		targets = append(targets, targetsFromFile(f, root, recipeFilter)...)
	}
	sort.SliceStable(targets, func(i, j int) bool {
		if targets[i].file != targets[j].file {
			return targets[i].file < targets[j].file
		}
		return targets[i].line < targets[j].line
	})
	return targets, errs
}

// targetsFromFile walks one parsed file's <?build?> marker pairs and
// returns a buildTarget per well-formed directive. Directives that fail
// the minimal recipe/outputs precondition are skipped silently — the
// lint-fix pass already surfaced them as MDS039 diagnostics.
func targetsFromFile(f *lint.File, root, recipeFilter string) []buildTarget {
	pairs, _ := gensection.FindMarkerPairs(f, "build", "MDS039", "build")
	var out []buildTarget
	for _, mp := range pairs {
		dir, diags := gensection.ParseDirective(f.Path, mp, "MDS039", "build")
		if dir == nil || len(diags) > 0 {
			continue
		}
		recipe := strings.TrimSpace(dir.Params["recipe"])
		if recipe == "" {
			continue
		}
		if recipeFilter != "" && recipe != recipeFilter {
			continue
		}
		outputs := splitDirectiveList(dir.Params["outputs"])
		if len(outputs) == 0 {
			continue
		}
		inputs := splitDirectiveList(dir.Params["inputs"])
		out = append(out, buildTarget{
			file: f.Path,
			line: mp.StartLine,
			target: buildexec.Target{
				Recipe:  recipe,
				Params:  directiveParams(dir.Params),
				Root:    root,
				Inputs:  inputs,
				Outputs: outputs,
			},
		})
	}
	return out
}

// directiveParams copies the directive's param map, dropping the
// structural keys (recipe, inputs, outputs) so only named recipe params
// remain for {param} substitution.
func directiveParams(params map[string]string) map[string]string {
	out := make(map[string]string, len(params))
	for k, v := range params {
		switch k {
		case "recipe", "inputs", "outputs":
			continue
		default:
			out[k] = v
		}
	}
	return out
}

// splitDirectiveList splits a newline-joined directive list value (the
// form gensection produces for a YAML sequence) into trimmed, non-empty
// entries.
func splitDirectiveList(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// stderrBuildWriter is the default destination for the build summary.
var stderrBuildWriter io.Writer = os.Stderr
