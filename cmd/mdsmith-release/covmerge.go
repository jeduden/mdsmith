package main

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/release"
)

func runMergeCoverage(_ string, args []string) int {
	fs := flag.NewFlagSet("merge-coverage", flag.ContinueOnError)
	var out string
	fs.StringVarP(&out, "output", "o", "", "Path to write the merged profile")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr,
			"Usage: mdsmith-release merge-coverage -o <out> <profile>...\n\n"+
				"Merge Go coverage profiles by summing per-block hit\n"+
				"counts. Used by CI to combine the per-package unit\n"+
				"profile with the e2e subprocess-binary profile: a\n"+
				"plain concatenation leaves duplicate cmd/mdsmith\n"+
				"blocks and Codecov takes the last (often zero) one,\n"+
				"so summing is required for a correct patch number.\n")
	}
	if err := fs.Parse(args); err != nil {
		// reportFlagParseErr only returns a negative sentinel for a
		// nil error; inside this `err != nil` block it is always
		// 0 (help) or 2, so return it directly.
		return reportFlagParseErr(err, os.Stderr, "mdsmith-release: merge-coverage")
	}
	if out == "" || fs.NArg() == 0 {
		fs.Usage()
		return 2
	}
	return reportError(release.MergeCoverage(fs.Args(), out))
}
