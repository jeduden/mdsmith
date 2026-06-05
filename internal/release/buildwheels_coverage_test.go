package release

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// everyBuildStagingRunner is a Runner that stands in for python
// across a full BuildWheels run without python on PATH. On each
// `python -m build --outdir <dir>` invocation it drops a uniquely
// named *.whl into <dir> so listWheels finds it and the empty-wheel
// guard passes; `python -m wheel tags` and anything else is a
// no-op. Unlike the single-shot wheelStagingRunner in fault_test.go
// (which stages only on its first call), this stages on every build
// so all five matrix entries succeed and BuildWheels reaches its
// final return nil.
type everyBuildStagingRunner struct {
	builds int
}

func (r *everyBuildStagingRunner) RunCommand(_, _ string, args ...string) error {
	isBuild := false
	for _, a := range args {
		if a == "build" {
			isBuild = true
			break
		}
	}
	if !isBuild {
		return nil // `wheel tags` and friends: nothing to do
	}
	for i, a := range args {
		if a == "--outdir" && i+1 < len(args) {
			r.builds++
			name := fmt.Sprintf("mdsmith-%d-py3-none-any.whl", r.builds)
			if err := os.WriteFile(filepath.Join(args[i+1], name), []byte("WHEEL"), 0o644); err != nil {
				return fmt.Errorf("everyBuildStagingRunner: stage wheel: %w", err)
			}
			break
		}
	}
	return nil
}

// TestBuildWheels_FullSuccess drives BuildWheels all the way to its
// final return nil — the success path the existing layout test only
// reaches when python, the build frontend, and hatchling are all on
// PATH. Here a staging Runner produces a wheel per matrix entry, so
// every buildOneWheel succeeds, each wheel is moved into outDir, and
// the loop completes. Asserts one wheel landed per platform.
func TestBuildWheels_FullSuccess(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root) // writes python/pyproject.toml + root LICENSE
	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts) // the five release-asset binaries
	out := filepath.Join(root, "wheels")

	require.NoError(t, NewWithDeps(osFS{}, &everyBuildStagingRunner{}).
		BuildWheels(root, artifacts, out))

	entries, err := os.ReadDir(out)
	require.NoError(t, err)
	var wheels []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".whl") {
			wheels = append(wheels, e.Name())
		}
	}
	assert.Len(t, wheels, len(wheelBuilds),
		"one wheel must land in outDir per platform matrix entry")
}

// TestBuildWheels_FailsWhenMkdirOutFails drives the MkdirAll(absOut)
// error branch: a regular file at the resolved out path makes the
// MkdirAll fail with ENOTDIR even as root, before any wheel build.
func TestBuildWheels_FailsWhenMkdirOutFails(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	artifacts := filepath.Join(root, "artifacts")
	fakeArtifacts(t, artifacts)
	// out's parent component is a regular file, so MkdirAll(out)
	// cannot create it.
	base := t.TempDir()
	regular := filepath.Join(base, "file")
	require.NoError(t, os.WriteFile(regular, []byte("x"), 0o644))
	out := filepath.Join(regular, "wheels")

	err := New().BuildWheels(root, artifacts, out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}
