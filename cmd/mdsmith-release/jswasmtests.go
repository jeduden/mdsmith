package main

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/release"
)

func runTestJSWasm(root string, args []string) int {
	fs := flag.NewFlagSet("test-js-wasm", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release test-js-wasm <pkg>\n\n"+
			"Run the package's js/wasm-only tests under Node: list the test\n"+
			"files only a GOOS=js GOARCH=wasm build compiles, parse their\n"+
			"TestXxx(*testing.T) functions, run exactly those with go test\n"+
			"-json through go_js_wasm_exec, and fail unless every one passes\n"+
			"(a skipped test fails by name). Needs node on PATH.\n")
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
	return reportError(release.RunJSWasmTests(root, fs.Arg(0), os.Stdout))
}
