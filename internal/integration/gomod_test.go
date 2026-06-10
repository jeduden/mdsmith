package integration

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/mod/modfile"
)

// parseRootGoMod reads and parses the repository's root go.mod, the
// module definition `go install` and library consumers resolve.
func parseRootGoMod(t *testing.T) *modfile.File {
	t.Helper()
	data, err := os.ReadFile("../../go.mod")
	require.NoError(t, err)
	f, err := modfile.Parse("go.mod", data, nil)
	require.NoError(t, err)
	return f
}

// TestRootGoModStaysInstallable guards the `go install
// github.com/jeduden/mdsmith/cmd/mdsmith@<version>` channel
// (docs/development/release-channels/go.md). The go command refuses
// to build a module at a version whose go.mod carries replace
// directives, so a single replace breaks every toolchain install.
// v0.40.0 shipped broken this way: the vendored goldmark fork was
// wired via `replace github.com/yuin/goldmark => ./pkg/goldmark`
// instead of being imported by its in-module path.
func TestRootGoModStaysInstallable(t *testing.T) {
	f := parseRootGoMod(t)

	for _, r := range f.Replace {
		assert.Failf(t, "replace directive in root go.mod",
			"go.mod replaces %s with %s; `go install m@version` rejects modules "+
				"with replace directives — import the replacement by its in-module path instead",
			r.Old.Path, r.New.Path)
	}
}

// TestRootGoModCarriesNoDevTools keeps dev-only tools (golangci-lint,
// vhs, gobco) out of the module graph that `go install` and library
// consumers must resolve. A tool directive pulls the tool's full
// dependency tree into go.sum and raises the root `go` floor to the
// tool's own: vhs v0.11.0 declares `go 1.25.8` and dragged the whole
// module from 1.25.0 (the highest floor among real dependencies) up
// to 1.25.8, forcing a toolchain download on every consumer install.
// Dev tools belong in tools/go.mod, invoked via
// `go tool -modfile=tools/go.mod <tool>`.
func TestRootGoModCarriesNoDevTools(t *testing.T) {
	f := parseRootGoMod(t)

	for _, tool := range f.Tool {
		assert.Failf(t, "tool directive in root go.mod",
			"go.mod declares tool %s; move it to tools/go.mod so its dependency "+
				"tree and go-version floor stay out of the consumer-facing module graph",
			tool.Path)
	}
	if f.Toolchain != nil {
		assert.Failf(t, "toolchain directive in root go.mod",
			"go.mod pins toolchain %s; `go install m@version` honors it, forcing that "+
				"exact toolchain download on consumers — the go directive alone sets the floor",
			f.Toolchain.Name)
	}
}
