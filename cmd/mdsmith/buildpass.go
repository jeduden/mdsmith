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
)

// buildPassOpts bundles the build-pass knobs parsed from the fix CLI
// flags. The build pass is CLI-only — it lives here in cmd/mdsmith and
// never runs from pkg/mdsmith, the WASM bindings, the LSP fix path, or
// the merge driver.
type buildPassOpts struct {
	noBuild   bool          // --no-build: skip the build pass entirely
	buildOnly bool          // --build-only: run only the build pass
	recipe    string        // --build-recipe: only this recipe's directives
	dryRun    bool          // --build-dry-run: enumerate targets, run nothing
	timeout   time.Duration // --build-timeout: per-recipe timeout
	maxBytes  int64         // file-size cap inherited from the fix run
}

// buildTarget pairs a resolved build.Target with the file and line it
// came from, so the per-target summary can name the source directive.
type buildTarget struct {
	file   string
	line   int
	target buildexec.Target
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

	failed := false
	for _, bt := range targets {
		label := fmt.Sprintf("%s:%d (%s)", bt.file, bt.line, bt.target.Recipe)
		if opts.dryRun {
			_, _ = fmt.Fprintf(w, "build %s: DRY-RUN\n", label)
			continue
		}
		if err := runOneTarget(builder, bt, timeout); err != nil {
			_, _ = fmt.Fprintf(w, "build %s: FAIL: %v\n", label, err)
			failed = true
			continue
		}
		_, _ = fmt.Fprintf(w, "build %s: OK\n", label)
	}

	if failed || len(errs) > 0 {
		return 2
	}
	return 0
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
