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

// pruneOrphanLogsFn is the buildexec.PruneOrphanLogs implementation; tests may replace it.
var pruneOrphanLogsFn = buildexec.PruneOrphanLogs

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
	stream             bool          // --build-stream: live-forward recipe streams to the terminal
	verify             bool          // --build-verify: run each recipe twice and diff outputs
	jobs               int           // --build-jobs N: concurrent recipe dispatch (default 1)
	explain            string        // --build-explain TARGET: print ActionID inputs; run nothing
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
// each one to w and returns false.
//
// The gate is non-bypassable: when cfg.Build.Recipes is non-empty the
// shell-safety check always runs regardless of whether the recipe-safety rule
// toggle is enabled or disabled in cfg.Rules. The rule toggle controls only
// diagnostic reporting; it must never suppress the pre-execution safety gate.
func checkMDS040Gate(cfg *config.Config, cfgPath string, w io.Writer) bool {
	rc, ok := cfg.Rules["recipe-safety"]

	// When the rule is absent or disabled AND there is nothing to execute
	// (no recipes and no hooks), the gate is open.
	noRecipes := len(cfg.Build.Recipes) == 0
	noHooks := len(cfg.Build.Hooks.Before) == 0 && len(cfg.Build.Hooks.After) == 0
	if (!ok || !rc.Enabled) && noRecipes && noHooks {
		return true
	}

	r := rule.ByID("MDS040")
	if r == nil {
		return true
	}
	clone := rule.CloneRule(r)
	c, ok2 := clone.(interface {
		ApplySettings(map[string]any) error
		Check(f *lint.File) []lint.Diagnostic
	})
	if !ok2 {
		return true
	}

	// Determine the settings to apply. When the rule is enabled, use its
	// settings directly (InjectBuildConfig already populated recipes/hooks).
	// When the rule is disabled or absent, build settings from the live config
	// so the gate still validates shell-safety for both recipes and hooks even
	// though diagnostic reporting is suppressed.
	var settings map[string]any
	if ok && rc.Enabled {
		settings = rc.Settings
	} else {
		// Build a minimal settings map covering all executable surfaces.
		// Only "command" is included per recipe; the gate's mandate is
		// shell-safety (interpreter/operator detection), not code-quality
		// checks such as reserved-param name validation (inputs/outputs).
		// ValidateBuildConfig enforces other param constraints at config
		// load time.
		recipes := make(map[string]any, len(cfg.Build.Recipes))
		for name, r := range cfg.Build.Recipes {
			recipes[name] = map[string]any{"command": r.Command}
		}
		settings = map[string]any{
			"recipes":      recipes,
			"hooks-before": config.SerializeHooks(cfg.Build.Hooks.Before),
			"hooks-after":  config.SerializeHooks(cfg.Build.Hooks.After),
		}
		if cfgPath != "" {
			settings["config-path"] = cfgPath
		}
	}
	if err := c.ApplySettings(settings); err != nil {
		_, _ = fmt.Fprintf(w, "mdsmith: MDS040 settings error: %v\n", err)
		return false
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

	if opts.explain != "" {
		cache := loadBuildCache(root, opts, w)
		return explainTarget(targets, opts.explain, cfg, cache, w)
	}

	if len(targets) == 0 {
		if len(errs) > 0 {
			return 2
		}
		return 0
	}

	if !ensureTrusted(opts, cfgPath, root, w) {
		return 2
	}

	builder := buildexec.NewCustomBuilderExec(recipes, buildExecConfig(cfg))
	timeout := opts.timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	cache := loadBuildCache(root, opts, w)
	if !opts.noCache {
		if err := pruneOrphanLogsFn(root, cache); err != nil {
			_, _ = fmt.Fprintf(w, "mdsmith: %v\n", err)
		}
	}
	return dispatchWithHooks(builder, targets, cfg, root, opts, cache, timeout, errs, w)
}

// ensureTrusted checks the build trust gate and returns false (printing a
// denial message on w) when recipes must not execute. It is a no-op and
// returns true when opts suppresses recipe execution (dryRun or checkStale).
func ensureTrusted(opts buildPassOpts, cfgPath, root string, w io.Writer) bool {
	if opts.dryRun || opts.checkStale {
		return true
	}
	// Pin the config file the run actually loaded (cfgPath), so the gate
	// is correct under `mdsmith fix -c other.yml`. A defaults-only run
	// (cfgPath == "") falls back to the default config name under root.
	trustPath := cfgPath
	if trustPath == "" {
		trustPath = buildexec.ConfigPathForRoot(root)
	}
	if trust := buildexec.CheckTrust(trustPath, envIsSet); !trust.Trusted {
		_, _ = fmt.Fprintf(w, "mdsmith: %s\n", trust.Reason)
		return false
	}
	return true
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
// loop. It returns the build pass exit code. With opts.jobs > 1 recipes
// run concurrently; cache entries apply serially in declared order after
// all recipes finish.
func dispatchTargets(
	builder buildexec.Builder, targets []buildTarget, cfg *config.Config,
	root string, opts buildPassOpts, cache *buildexec.Cache,
	timeout time.Duration, w io.Writer,
) int {
	var failed, anyStale, rebuilt bool
	var fold = func(o targetOutcome) {
		switch o {
		case outcomeFailed:
			failed = true
		case outcomeStale:
			anyStale = true
		case outcomeRebuilt:
			rebuilt = true
		case outcomeNeutral:
		}
	}

	if opts.jobs > 1 && opts.dryRun {
		_, _ = fmt.Fprintf(w, "mdsmith: --build-jobs ignored with --build-dry-run\n")
	}
	if opts.jobs > 1 && opts.checkStale {
		_, _ = fmt.Fprintf(w, "mdsmith: --build-jobs ignored with --build-check-stale\n")
	}
	if opts.jobs > 1 && !opts.checkStale && !opts.dryRun {
		runConcurrent(builder, targets, cfg, opts, cache, timeout, w, fold)
	} else {
		for _, bt := range targets {
			fold(dispatchOne(builder, bt, cfg, opts, cache, timeout, w))
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

// dispatchOne handles a single target end to end against a shared cache,
// used by the serial path. It computes the verdict, reports or rebuilds,
// and applies the cache entry directly.
func dispatchOne(
	builder buildexec.Builder, bt buildTarget, cfg *config.Config,
	opts buildPassOpts, cache *buildexec.Cache, timeout time.Duration, w io.Writer,
) targetOutcome {
	stin := stalenessFor(bt, cfg)
	verdict, serr := targetVerdict(stin, cache, opts)
	outcome, entry := decideAndRun(builder, bt, opts, stin, verdict, serr, timeout, nil, w)
	if entry != nil {
		cache.Put(*entry)
	}
	return outcome
}

// decideAndRun decides the per-target action from a precomputed verdict
// and, when a rebuild is warranted, runs the recipe. It returns the
// outcome and an optional cache entry to apply (nil when nothing changed
// or --build-no-cache is set). It touches no shared cache, so concurrent
// callers can run it in parallel and apply the returned entries serially.
// allFinals lists all declared final paths across concurrent targets for
// the concurrent-safe post-condition check; nil on the serial path.
func decideAndRun(
	builder buildexec.Builder, bt buildTarget,
	opts buildPassOpts, stin buildexec.StalenessInput, verdict buildexec.Verdict, verdictErr error,
	timeout time.Duration, allFinals []string, w io.Writer,
) (targetOutcome, *buildexec.CacheEntry) {
	label := fmt.Sprintf("%s:%d (%s)", bt.file, bt.line, bt.target.Recipe)

	if verdictErr != nil {
		_, _ = fmt.Fprintf(w, "build %s: FAIL: %v\n", label, verdictErr)
		return outcomeFailed, nil
	}
	if opts.checkStale {
		if verdict == buildexec.Stale {
			_, _ = fmt.Fprintf(w, "build %s: STALE\n", label)
			return outcomeStale, nil
		}
		return outcomeNeutral, nil
	}
	if opts.dryRun {
		_, _ = fmt.Fprintf(w, "build %s: %s\n", label, verdict)
		return outcomeNeutral, nil
	}
	if verdict == buildexec.Fresh {
		_, _ = fmt.Fprintf(w, "SKIP %s\n", targetName(bt))
		return outcomeNeutral, nil
	}

	var id string
	if !opts.noCache {
		var idErr error
		id, idErr = buildexec.ComputeActionID(stin)
		if idErr != nil {
			reportBuildFailure(bt, targetRunResult{Result: buildexec.Result{Err: idErr}}, w)
			return outcomeFailed, nil
		}
	}
	res := runOneTarget(builder, bt, id, opts, timeout, allFinals, w)
	if res.Err != nil {
		reportBuildFailure(bt, res, w)
		return outcomeFailed, nil
	}
	if opts.verify {
		verifyTarget(builder, bt, id, opts, timeout, &res, w)
	}
	entry, err := buildCacheEntry(stin, opts, res.Unstable)
	if err != nil {
		_, _ = fmt.Fprintf(w, "build %s: FAIL: %v\n", label, err)
		return outcomeFailed, nil
	}
	_, _ = fmt.Fprintf(w, "OK %s\n", targetName(bt))
	return outcomeRebuilt, entry
}

// targetName returns the target's display name: its first declared output
// path, or the source label when no output is declared.
func targetName(bt buildTarget) string {
	if len(bt.target.Outputs) > 0 {
		return bt.target.Outputs[0]
	}
	return fmt.Sprintf("%s:%d", bt.file, bt.line)
}

// targetVerdict returns the staleness verdict for one target, honouring
// --build-force and --build-no-cache (both force a Stale verdict).
func targetVerdict(
	stin buildexec.StalenessInput, cache *buildexec.Cache, opts buildPassOpts,
) (buildexec.Verdict, error) {
	if opts.force || opts.noCache {
		// Validate inputs without hashing — cheaper than a full staleness check
		// when the verdict is already known to be Stale.
		if err := buildexec.ValidateInputs(stin); err != nil {
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

// buildCacheEntry records a rebuilt target's cache entry, stamping
// built-at and the unstable flag. With --build-no-cache it returns nil.
func buildCacheEntry(
	stin buildexec.StalenessInput, opts buildPassOpts, unstable bool,
) (*buildexec.CacheEntry, error) {
	if opts.noCache {
		return nil, nil
	}
	entry, err := buildexec.RecordBuild(stin)
	if err != nil {
		return nil, err
	}
	entry.BuiltAt = time.Now().UTC().Format(time.RFC3339)
	entry.Unstable = unstable
	return &entry, nil
}

// targetRunResult is one recipe run's outcome plus the unstable flag set
// by --build-verify.
type targetRunResult struct {
	buildexec.Result
	Unstable bool
}

// runOneTarget dispatches a single target with a per-recipe timeout. When id
// is non-empty, streams are captured to the action-id log file under root.
// opts.stream forwards recipe lines live to w. allFinals lists all declared
// final paths across concurrent targets for the post-condition check; nil on
// the serial path.
func runOneTarget(
	b buildexec.Builder, bt buildTarget, id string,
	opts buildPassOpts, timeout time.Duration, allFinals []string, w io.Writer,
) targetRunResult {
	bopts := buildexec.Options{TargetName: targetName(bt), AllFinals: allFinals}
	if id != "" {
		bopts.LogRoot = bt.target.Root
		bopts.ActionID = id
	}
	if opts.stream {
		bopts.LiveSink = w
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return targetRunResult{Result: b.BuildWithResult(ctx, bt.target, bopts)}
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

// buildExecConfig maps the loaded config's build.exec section into the
// build package's ExecConfig. Empty fields fall through to the build
// executor's compiled defaults.
func buildExecConfig(cfg *config.Config) buildexec.ExecConfig {
	return buildexec.ExecConfig{
		Path:           cfg.Build.Exec.Path,
		EnvPassThrough: cfg.Build.Exec.EnvPassThrough,
	}
}

// envIsSet reports whether the named environment variable is set to a
// truthy value. It is the production environment lookup passed to the
// trust gate, so an explicit MDSMITH_TRUST_BUILD=0 (or false/no/off)
// does NOT grant trust — only an affirmative value does. This avoids the
// footgun where a user who sets the variable to a disabling value still
// has the build pass run.
func envIsSet(name string) bool {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return false
	}
	switch strings.ToLower(v) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
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
