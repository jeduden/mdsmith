//go:build !(js && wasm)

package main

import (
	"go/types"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

const (
	modulePath  = "github.com/jeduden/mdsmith"
	refactorPkg = modulePath + "/internal/refactor"
)

// TestWasmBundleNeverCallsFileOpExecute restores the compile-time
// guarantee that the wasm FileOp.Execute stub
// (internal/refactor/fileop_exec_wasm.go) gave up: before the stub, any
// wasm-reachable call to Execute failed to build. The stub always
// errors, so a call from code the bridge links would compile and fail
// only in a user session. This test type-checks every module package
// the GOOS=js GOARCH=wasm bridge links and fails if one uses
// FileOp.Execute (a call or a method value). Plan 2610020725.
func TestWasmBundleNeverCallsFileOpExecute(t *testing.T) {
	if testing.Short() {
		t.Skip("type-checks the wasm dependency graph")
	}
	// os/exec keeps the last value of a duplicated key, so these win.
	env := append(os.Environ(), "GOOS=js", "GOARCH=wasm")

	graph, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedImports | packages.NeedDeps,
		Env:  env,
	}, ".")
	require.NoError(t, err)
	require.Len(t, graph, 1)

	var local []string
	packages.Visit(graph, nil, func(p *packages.Package) {
		if strings.HasPrefix(p.PkgPath, modulePath+"/") && p.PkgPath != refactorPkg {
			local = append(local, p.PkgPath)
		}
	})
	require.NotEmpty(t, local)

	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedSyntax | packages.NeedImports,
		Env: env,
	}, local...)
	require.NoError(t, err)
	for _, p := range pkgs {
		require.Empty(t, p.Errors, "type-check %s", p.PkgPath)
		for id, obj := range p.TypesInfo.Uses {
			if isFileOpExecute(obj) {
				t.Errorf("%s: %s uses refactor.FileOp.Execute, which always fails under wasm",
					p.Fset.Position(id.Pos()), p.PkgPath)
			}
		}
	}
}

// isFileOpExecute reports whether obj is the Execute method of
// internal/refactor.FileOp.
func isFileOpExecute(obj types.Object) bool {
	fn, ok := obj.(*types.Func)
	if !ok || fn.Name() != "Execute" || fn.Pkg() == nil || fn.Pkg().Path() != refactorPkg {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return false
	}
	recv := sig.Recv().Type()
	if p, ok := recv.(*types.Pointer); ok {
		recv = p.Elem()
	}
	named, ok := recv.(*types.Named)
	return ok && named.Obj().Name() == "FileOp"
}
