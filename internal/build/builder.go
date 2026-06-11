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
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	buildrule "github.com/jeduden/mdsmith/internal/rules/build"
)

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

// Builder dispatches a single build Target.
type Builder interface {
	Build(ctx context.Context, target Target) error
}

// CustomBuilder is the sole Builder implementation. It dispatches a
// directive's recipe command via os/exec.
type CustomBuilder struct {
	recipes map[string]RecipeSpec
}

// NewCustomBuilder returns a CustomBuilder over the given recipe map.
func NewCustomBuilder(recipes map[string]RecipeSpec) *CustomBuilder {
	return &CustomBuilder{recipes: recipes}
}

var _ Builder = (*CustomBuilder)(nil)

// Build resolves the target's inputs and outputs against the project
// root, stages every output in a per-target temp dir, runs the recipe
// with the staged paths, and renames the staged files into place on
// success. On any failure the temp dir is removed and no declared
// output is touched.
func (b *CustomBuilder) Build(ctx context.Context, target Target) error {
	spec, ok := b.recipes[target.Recipe]
	if !ok {
		return fmt.Errorf("unknown recipe %q", target.Recipe)
	}
	tokens := strings.Fields(spec.Command)
	if len(tokens) == 0 {
		return fmt.Errorf("recipe %q has an empty command", target.Recipe)
	}

	inputs, err := b.resolveInputs(target)
	if err != nil {
		return err
	}
	outputs, err := b.resolveOutputs(target)
	if err != nil {
		return err
	}

	stageDir, err := os.MkdirTemp("", "mdsmith-build-")
	if err != nil {
		return fmt.Errorf("creating staging dir: %w", err)
	}
	defer os.RemoveAll(stageDir) //nolint:errcheck // best-effort cleanup

	// Stage path per output: a flat file named by index, so a recipe
	// writing to {outputs}[i] writes inside the staging dir.
	stagePaths := make([]string, len(outputs))
	for i := range outputs {
		stagePaths[i] = filepath.Join(stageDir, fmt.Sprintf("out%d", i))
	}

	// Absolute input paths recomposed against root.
	absInputs := make([]string, len(inputs))
	for i, in := range inputs {
		absInputs[i] = filepath.Join(target.Root, filepath.FromSlash(in))
	}

	argv := expandArgv(tokens, target.Params, absInputs, stagePaths)

	if err := runRecipe(ctx, target.Root, argv); err != nil {
		return fmt.Errorf("recipe %q failed: %w", target.Recipe, err)
	}

	return commitOutputs(target.Root, outputs, stagePaths)
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
			if err := buildrule.CheckGlobMatchCap(len(matches)); err != nil {
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
// root. Outputs may not exist yet, so mustExist is false.
func (b *CustomBuilder) resolveOutputs(target Target) ([]string, error) {
	out := make([]string, 0, len(target.Outputs))
	for _, entry := range target.Outputs {
		rel, err := buildrule.ResolvePathInRoot(target.Root, entry, false)
		if err != nil {
			return nil, err
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

// runRecipe execs argv with the project root as the working directory.
// No shell is invoked: argv[0] is the program and argv[1:] its
// arguments. The context bounds the run; on cancellation the process is
// killed.
func runRecipe(ctx context.Context, root string, argv []string) error {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // argv is explicit; user-declared recipe
	cmd.Dir = root
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("%w (timed out)", ctx.Err())
		}
		return err
	}
	return nil
}

// commitOutputs renames each staged file to its final in-root location,
// creating parent directories as needed. A staged file that the recipe
// did not write is an error (the recipe failed to produce a declared
// output). On any rename failure the already-committed renames are not
// rolled back here — the per-target temp dir guarantees nothing was
// touched until this point, and the basic contract (plan 2606101548
// hardens it further) treats a partial commit failure as a hard error.
func commitOutputs(root string, outputs, stagePaths []string) error {
	for i, rel := range outputs {
		stage := stagePaths[i]
		if _, err := os.Stat(stage); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("recipe did not produce declared output %q", rel)
			}
			return fmt.Errorf("staging output %q: %w", rel, err)
		}
		final := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
			return fmt.Errorf("creating output dir for %q: %w", rel, err)
		}
		if err := os.Rename(stage, final); err != nil {
			// Cross-device rename can fail; fall back to copy.
			if cerr := copyFile(stage, final); cerr != nil {
				return fmt.Errorf("writing output %q: %w", rel, cerr)
			}
		}
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
