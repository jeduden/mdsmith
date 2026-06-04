package integration

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFixDoesNotImportEngine guards the architecture rule that
// internal/fix must not import internal/engine. The engine layer sits
// above the fix layer, so the dependency arrow must point downward
// (fix → shared helpers → engine), not upward (fix → engine).
func TestFixDoesNotImportEngine(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)

	// Tests run from internal/integration/; fix lives at ../fix relative to that.
	fixRoot := filepath.Clean(filepath.Join(wd, "..", "fix"))
	info, err := os.Stat(fixRoot)
	require.NoError(t, err, "fix package root %s must exist", fixRoot)
	require.True(t, info.IsDir(), "fix package root %s must be a directory", fixRoot)

	const forbidden = "github.com/jeduden/mdsmith/internal/engine"
	fset := token.NewFileSet()

	err = filepath.WalkDir(fixRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(fixRoot, path)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if importPath == forbidden {
				assert.Failf(t, "fix imports engine",
					"internal/fix/%s imports internal/engine; "+
						"engine sits above fix — move shared helpers to a lower package",
					rel)
			}
		}
		return nil
	})
	require.NoError(t, err)
}
