package integration

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/mod/modfile"
)

// TestRootGoModStaysInstallable guards the `go install
// github.com/jeduden/mdsmith/cmd/mdsmith@<version>` channel
// (docs/development/release-channels/go.md). The go command refuses
// to build a module at a version whose go.mod carries replace
// directives, so a single replace breaks every toolchain install.
// v0.40.0 shipped broken this way: the vendored goldmark fork was
// wired via `replace github.com/yuin/goldmark => ./pkg/goldmark`
// instead of being imported by its in-module path.
func TestRootGoModStaysInstallable(t *testing.T) {
	data, err := os.ReadFile("../../go.mod")
	require.NoError(t, err)
	f, err := modfile.Parse("go.mod", data, nil)
	require.NoError(t, err)

	for _, r := range f.Replace {
		assert.Failf(t, "replace directive in root go.mod",
			"go.mod replaces %s with %s; `go install m@version` rejects modules "+
				"with replace directives — import the replacement by its in-module path instead",
			r.Old.Path, r.New.Path)
	}
}
