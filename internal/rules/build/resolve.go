package build

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jeduden/mdsmith/internal/oscompat"
)

// evalSymlinks is a var so tests can override it to drive the
// no-existing-prefix branch of resolveLongestExistingPrefix, which a real
// filesystem never reaches because the root always resolves.
var evalSymlinks = oscompat.EvalSymlinks

// MaxGlobMatches is the per-entry cap on how many files one inputs:
// glob may resolve to. An author who needs more declares several
// narrower patterns. The build executor (a later plan) enforces this
// during glob resolution via CheckGlobMatchCap.
const MaxGlobMatches = 10000

// CheckGlobMatchCap returns an error when an inputs: glob resolved to
// more than MaxGlobMatches files. n is the number of matched paths.
func CheckGlobMatchCap(n int) error {
	if n > MaxGlobMatches {
		return fmt.Errorf("inputs glob matched %d files, exceeding the limit of %d; use narrower patterns",
			n, MaxGlobMatches)
	}
	return nil
}

// ResolvePathInRoot resolves rel — a project-root-relative, slash-
// separated outputs: or inputs: entry that already passed the
// path-shape rules — against root and verifies the symlink-resolved
// result stays inside root. It returns the in-root path normalised
// back to forward slashes.
//
// When mustExist is true (inputs, which must exist), the full path is
// resolved with filepath.EvalSymlinks; a missing path is an error.
// When mustExist is false (outputs, which may not exist yet), the
// longest existing prefix is resolved with EvalSymlinks and the
// remaining segments are joined on with filepath.Join. The check uses
// OS-native separators internally and normalises back to forward
// slashes before comparison, keeping the slash-only invariant on
// Windows. A path that resolves outside root is an error.
func ResolvePathInRoot(root, rel string, mustExist bool) (string, error) {
	resolvedRoot, err := evalSymlinks(root)
	if err != nil {
		// Fall back to the lexical absolute root when the root itself
		// cannot be resolved (e.g. it does not exist in a unit test of
		// an unrelated branch); Abs only fails without a working dir.
		resolvedRoot, _ = filepath.Abs(root)
		resolvedRoot = filepath.Clean(resolvedRoot)
	}

	abs := filepath.Join(resolvedRoot, filepath.FromSlash(rel))

	var resolved string
	if mustExist {
		resolved, err = evalSymlinks(abs)
		if err != nil {
			return "", fmt.Errorf("cannot resolve %q: %w", rel, err)
		}
	} else {
		resolved = resolveLongestExistingPrefix(abs)
	}

	inRoot, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil || escapesRoot(inRoot) {
		return "", fmt.Errorf("%q resolves outside the project root", rel)
	}
	return filepath.ToSlash(inRoot), nil
}

// resolveLongestExistingPrefix resolves the longest existing ancestor
// of abs with filepath.EvalSymlinks, then rejoins the non-existent
// trailing segments. This lets an output path that does not exist yet
// still have any symlinked parent directory resolved, so a parent that
// escapes the root is caught.
func resolveLongestExistingPrefix(abs string) string {
	abs = filepath.Clean(abs)
	missing := []string{}
	cur := abs
	for {
		if resolved, err := evalSymlinks(cur); err == nil {
			if len(missing) == 0 {
				return resolved
			}
			parts := append([]string{resolved}, reversed(missing)...)
			return filepath.Join(parts...)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			// Reached the filesystem root without an existing prefix;
			// return the lexical clean path.
			return abs
		}
		missing = append(missing, filepath.Base(cur))
		cur = parent
	}
}

// reversed returns a new slice with the elements of s in reverse order.
func reversed(s []string) []string {
	out := make([]string, len(s))
	for i, v := range s {
		out[len(s)-1-i] = v
	}
	return out
}

// escapesRoot reports whether a root-relative path produced by
// filepath.Rel leaves the root (".." or a "../"-prefixed path). "."
// is the root itself and stays in-root.
func escapesRoot(rel string) bool {
	return rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
