package release

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sbomRunner records every RunCommand invocation and optionally
// returns err on the call at index errAt (-1 = never). Distinct
// from fault_test.go's helpers: this one couples recording with
// scriptable per-index failure so the SBOM pipeline's two-step
// install + emit sequence can be inspected on both the happy path
// and each failure point.
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

func TestGenerateSBOM_Pipeline(t *testing.T) {
	runner := &sbomRunner{errAt: -1}
	tk := NewWithDeps(osFS{}, runner)

	require.NoError(t, tk.GenerateSBOM("/repo", "sbom.cdx.json"))
	require.Len(t, runner.calls, 2)

	assert.Equal(t, "go", runner.calls[0].name)
	assert.Equal(t, []string{
		"install",
		"github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@" + cyclonedxGomodVersion,
	}, runner.calls[0].args)
	assert.Equal(t, "/repo", runner.calls[0].dir)

	assert.Equal(t, "cyclonedx-gomod", runner.calls[1].name)
	assert.Equal(t, []string{
		"mod", "-licenses", "-json", "-output", "sbom.cdx.json",
	}, runner.calls[1].args)
}

func TestGenerateSBOM_InstallFailure(t *testing.T) {
	runner := &sbomRunner{errAt: 0, err: errors.New("no network")}
	tk := NewWithDeps(osFS{}, runner)

	err := tk.GenerateSBOM("/repo", "sbom.cdx.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "install cyclonedx-gomod")
	assert.Len(t, runner.calls, 1, "second call must not run after install fails")
}

func TestGenerateSBOM_EmitFailure(t *testing.T) {
	runner := &sbomRunner{errAt: 1, err: errors.New("scan failed")}
	tk := NewWithDeps(osFS{}, runner)

	err := tk.GenerateSBOM("/repo", "sbom.cdx.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "emit SBOM")
}

func TestGenerateSBOM_InputValidation(t *testing.T) {
	tk := NewWithDeps(osFS{}, &sbomRunner{errAt: -1})

	assert.ErrorContains(t, tk.GenerateSBOM("", "out.json"), "empty root")
	assert.ErrorContains(t, tk.GenerateSBOM("/repo", ""), "empty out path")
}
