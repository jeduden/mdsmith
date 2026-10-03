package main

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/release"
)

func runTestJSWasm(root string, args []string) int {
	fs := flag.NewFlagSet("test-js-wasm", flag.ContinueOnError)
	all := fs.Bool("all", false, "run every test in <pkg> under Node, not only the js/wasm-only ones")
	requireJSOnly := fs.Bool("require-js-only", false,
		"with --all, fail when <pkg> has no js/wasm-only Test function")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release test-js-wasm [--all [--require-js-only]] <pkg>\n\n"+
			"Run the package's js/wasm-only tests under Node: list the test\n"+
			"files only a GOOS=js GOARCH=wasm build compiles, parse their\n"+
			"TestXxx(*testing.T) functions, run exactly those with go test\n"+
			"-json through go_js_wasm_exec, and fail unless every one passes\n"+
			"(a skipped test fails by name). <pkg> must match exactly one\n"+
			"package. Needs node on PATH.\n\n"+
			"With --all, run every test a js/wasm build compiles under Node,\n"+
			"untagged ones included, and fail on any go test failure, when\n"+
			"no test passes, or when a test from the js/wasm-only files does\n"+
			"not pass (it fails by name). A skip of a test the native build\n"+
			"shares is allowed. --require-js-only also fails, before go test,\n"+
			"when the package has no js/wasm-only Test function.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith-release: test-js-wasm"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	if *all {
		return reportError(release.RunJSWasmPackage(root, fs.Arg(0), *requireJSOnly, os.Stdout))
	}
	return reportError(release.RunJSWasmTests(root, fs.Arg(0), os.Stdout))
}
