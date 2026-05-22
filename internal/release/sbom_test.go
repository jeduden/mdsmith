package release

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sbomRunner records every RunCommand invocation and optionally
// returns err on the call at index errAt (-1 = never). Lets the
// SBOM tests inspect the exact `go run` argv without putting the
// pinned cyclonedx-gomod build on PATH.
type sbomRunner struct {
	calls []sbomCall
	errAt int
	err   error
}

type sbomCall struct {
	dir  string
	name string
	args []string
}

func (r *sbomRunner) RunCommand(dir, name string, args ...string) error {
	idx := len(r.calls)
	r.calls = append(r.calls, sbomCall{dir: dir, name: name, args: append([]string(nil), args...)})
	if idx == r.errAt {
		return r.err
	}
	return nil
}

func TestGenerateSBOM_InvokesGoRunPinned(t *testing.T) {
	runner := &sbomRunner{errAt: -1}
	tk := NewWithDeps(osFS{}, runner)

	require.NoError(t, tk.GenerateSBOM("/repo", "sbom.cdx.json"))
	require.Len(t, runner.calls, 1, "GenerateSBOM must issue exactly one command")

	call := runner.calls[0]
	assert.Equal(t, "go", call.name)
	assert.Equal(t, "/repo", call.dir)
	assert.Equal(t, []string{
		"run",
		"github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@" + cyclonedxGomodVersion,
		"mod", "-licenses", "-json", "-output", "sbom.cdx.json",
	}, call.args)
}

func TestGenerateSBOM_PropagatesGoRunFailure(t *testing.T) {
	runner := &sbomRunner{errAt: 0, err: errors.New("network down")}
	tk := NewWithDeps(osFS{}, runner)

	err := tk.GenerateSBOM("/repo", "sbom.cdx.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "emit SBOM via cyclonedx-gomod")
	assert.Contains(t, err.Error(), cyclonedxGomodVersion)
	assert.ErrorIs(t, err, runner.err)
}

func TestGenerateSBOM_InputValidation(t *testing.T) {
	tk := NewWithDeps(osFS{}, &sbomRunner{errAt: -1})

	assert.ErrorContains(t, tk.GenerateSBOM("", "out.json"), "empty root")
	assert.ErrorContains(t, tk.GenerateSBOM("/repo", ""), "empty out path")
}

// TestGenerateSBOM_PackageWrapper covers the package-level entry
// point that constructs a default Toolkit. The real osRunner
// would invoke `go run`, which is fine on a CI host with a Go
// toolchain but expensive in unit tests, so we don't assert on
// command success — only that the wrapper threads errors and
// validates input the same way the Toolkit method does.
func TestGenerateSBOM_PackageWrapper(t *testing.T) {
	assert.ErrorContains(t, GenerateSBOM("", "out.json"), "empty root")
	assert.ErrorContains(t, GenerateSBOM("/repo", ""), "empty out path")
}
