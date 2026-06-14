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

// allowedRuleHelpers names the sibling helper packages a rule may
// import. Helpers answer a single shared question (fence position,
// table formatting, ast walks, settings parsing) and own no rule
// logic. Every other directory under internal/rules/ is a rule
// package and must not be imported by another rule. See
// docs/development/architecture/index.md.
var allowedRuleHelpers = map[string]struct{}{
	"astutil":       {},
	"settings":      {},
	"fencepos":      {},
	"tablefmt":      {},
	"buildpathutil": {},
}

// ruleBoundaryExempt names directories under internal/rules/ whose
// Go files are skipped by the boundary check. The `all` package is
// the documented blank-import barrel that re-exports rule init()
// side-effects to consumers; it is by design a list of every rule.
var ruleBoundaryExempt = map[string]struct{}{
	"all": {},
}

// TestRulesDoNotImportEachOther guards the architecture rule that
// rule packages sit at the lowest layer and never depend on each
// other. Permitted exceptions:
//
//   - The shared helper packages listed in allowedRuleHelpers.
//   - Sub-packages of the same rule (e.g. markdownflavor/ext from
//     a file in markdownflavor/).
//
// The check parses every non-test .go file under internal/rules/
// and inspects its import paths.
func TestRulesDoNotImportEachOther(t *testing.T) {
	rulesRoot := rulesPackageRoot(t)
	const prefix = "github.com/jeduden/mdsmith/internal/rules/"

	fset := token.NewFileSet()
	err := filepath.WalkDir(rulesRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, err := filepath.Rel(rulesRoot, path)
		if err != nil {
			return err
		}
		owningRule := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
		if _, exempt := ruleBoundaryExempt[owningRule]; exempt {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if !strings.HasPrefix(importPath, prefix) {
				continue
			}
			tail := strings.TrimPrefix(importPath, prefix)
			pkg := strings.SplitN(tail, "/", 2)[0]
			if pkg == owningRule {
				continue // same-rule sub-package
			}
			if _, ok := allowedRuleHelpers[pkg]; ok {
				continue
			}
			assert.Failf(t, "rule imports another rule",
				"%s imports %s; rules may only import sibling helpers (%s) or their own sub-packages",
				rel, importPath, helperList())
		}
		return nil
	})
	require.NoError(t, err)
}

// rulesPackageRoot returns the on-disk path of internal/rules/
// relative to this test file. WalkDir is used rather than
// parser.ParseDir so sub-packages are recursed into.
func rulesPackageRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	// Tests run from internal/integration/; rules live at
	// ../rules relative to that.
	root := filepath.Clean(filepath.Join(wd, "..", "rules"))
	info, err := os.Stat(root)
	require.NoError(t, err, "rules root %s must exist", root)
	require.True(t, info.IsDir(), "rules root %s must be a directory", root)
	return root
}

func helperList() string {
	names := make([]string, 0, len(allowedRuleHelpers))
	for k := range allowedRuleHelpers {
		names = append(names, k)
	}
	return strings.Join(names, ", ")
}

// TestBuildDoesNotImportRulePackages guards the DIP rule that
// internal/build (the build-pass executor) must not import specific
// rule packages. It may only import internal/rules/buildpathutil, the
// shared path-helper package both the executor and the build rule use.
// See docs/development/architecture/go.md §"Common violations to flag".
func TestBuildDoesNotImportRulePackages(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	buildPkg := filepath.Clean(filepath.Join(wd, "..", "build"))
	info, err := os.Stat(buildPkg)
	require.NoError(t, err, "internal/build must exist at %s", buildPkg)
	require.True(t, info.IsDir(), "internal/build must be a directory")

	const rulesPrefix = "github.com/jeduden/mdsmith/internal/rules/"
	const allowedHelper = rulesPrefix + "buildpathutil"

	fset := token.NewFileSet()
	err = filepath.WalkDir(buildPkg, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(buildPkg, path)
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(importPath, rulesPrefix) && importPath != allowedHelper {
				assert.Failf(t, "internal/build imports a rule package",
					"%s imports %s; internal/build may only import %s from internal/rules/",
					rel, importPath, allowedHelper)
			}
		}
		return nil
	})
	require.NoError(t, err)
}
