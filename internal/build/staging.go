package build

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// stagingRootRel is the project-relative path to the build staging root.
// Per-recipe staging dirs are created under it with random suffixes.
var stagingRootRel = filepath.Join(".mdsmith", "build-staging")

// groupWorldWritableMask is the permission bits that, if set on the
// staging root, let another user plant files there. Either bit set is a
// refusal on Unix.
const groupWorldWritableMask = 0o022

// osMkdirAllFn and osChmodFn indirect the staging-root creation calls so
// tests can drive their error branches without filesystem tricks.
var (
	osMkdirAllFn = os.MkdirAll
	osChmodFn    = os.Chmod
)

// ensureStagingRoot validates (and, if absent, creates) the
// .mdsmith/build-staging/ directory under root, returning its absolute
// path. It refuses a staging root that is a symlink, is not a directory,
// or is group- or world-writable on Unix. A freshly created root is
// chmod'd to 0o700 so the process umask cannot leave it more permissive.
func ensureStagingRoot(root string) (string, error) {
	dir := filepath.Join(root, stagingRootRel)

	info, err := os.Lstat(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspecting staging root: %w", err)
		}
		// Create the parent (.mdsmith) and the staging dir, then tighten
		// the mode: MkdirAll's mode is filtered by the umask, so an
		// explicit Chmod is required to guarantee 0o700.
		if mkErr := osMkdirAllFn(dir, 0o700); mkErr != nil {
			return "", fmt.Errorf("creating staging root: %w", mkErr)
		}
		if chErr := osChmodFn(dir, 0o700); chErr != nil {
			return "", fmt.Errorf("securing staging root: %w", chErr)
		}
		return dir, nil
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("staging root %s is a symlink; refusing to use it", dir)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("staging root %s is not a directory", dir)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&groupWorldWritableMask != 0 {
		return "", fmt.Errorf(
			"staging root %s is group- or world-writable (mode %#o); run `chmod 700 %s`",
			dir, info.Mode().Perm(), dir,
		)
	}
	return dir, nil
}
