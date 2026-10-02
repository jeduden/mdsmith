//go:build !wasm

package refactor

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Execute performs the file relocation described by op, with From and
// To resolved against rootDir. When op.From is tracked in a Git work
// tree, it runs `git mv` so the rename is staged in the index;
// otherwise it falls back to a plain filesystem rename. The
// destination's parent directory is created first either way.
//
// A tracked file whose `git mv` fails aborts with that error and leaves
// the source in place — no half-moved state — so a caller can apply the
// text edits and this move as one all-or-nothing operation. Git being
// absent, or the file being untracked, is not a failure: the plain
// rename covers those.
//
// Execute is a subprocess boundary and so lives outside the pure engine
// and outside the wasm build (GOOS=js GOARCH=wasm has no subprocesses);
// wasm hosts perform the move through their own platform API.
func (op FileOp) Execute(rootDir string) error {
	to := filepath.Join(rootDir, filepath.FromSlash(op.To))
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return fmt.Errorf("creating destination directory: %w", err)
	}
	if gitTracked(rootDir, op.From) {
		return gitMove(rootDir, op.From, op.To)
	}
	// git mv refuses an existing destination, so mirror that guard on
	// the plain-rename path: os.Rename silently clobbers `to`, which
	// would destroy a file the planner's collision check could not read
	// (e.g. one over the max-input-size limit) and so did not see.
	if _, err := os.Lstat(to); err == nil {
		return fmt.Errorf("moving %s: destination already exists: %s", op.From, op.To)
	}
	from := filepath.Join(rootDir, filepath.FromSlash(op.From))
	if err := os.Rename(from, to); err != nil {
		return fmt.Errorf("moving %s: %w", op.From, err)
	}
	return nil
}

// gitTracked reports whether rel (workspace-relative) is a file Git
// tracks in the work tree rooted at rootDir. A missing git binary, a
// directory that is not a repository, or an untracked path all return
// false, so the caller falls back to a plain rename.
func gitTracked(rootDir, rel string) bool {
	cmd := exec.Command("git", "-C", rootDir, "ls-files", "--error-unmatch", "--", filepath.FromSlash(rel))
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// gitMove runs `git mv from to` in the repository at rootDir, staging
// the rename. A non-zero exit is returned as an error carrying git's
// own message, so a destination collision or an untracked source
// surfaces the reason.
func gitMove(rootDir, from, to string) error {
	cmd := exec.Command("git", "-C", rootDir, "mv", "--",
		filepath.FromSlash(from), filepath.FromSlash(to))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git mv: %w: %s", err, bytes.TrimSpace(out))
	}
	return nil
}
