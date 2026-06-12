package main

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/release"
)

func runPGO(root string, args []string) int {
	fs := flag.NewFlagSet("pgo", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release pgo [workdir]\n\n"+
			"Generate a PGO profile from the repo root and write it to\n"+
			"cmd/mdsmith/default.pgo (gitignored): build mdsmith, build the\n"+
			"repo + neutral corpora, record a CPU profile over each corpus\n"+
			"in both the default and parity configurations via the\n"+
			"MDSMITH_CPUPROFILE hook, then merge the four profiles with\n"+
			"`go tool pprof -proto`. The release workflow runs this before\n"+
			"the build matrix and uploads the profile as an artifact so the\n"+
			"published binaries are profile-guided without a tracked file.\n"+
			"workdir caches the built binary and corpora (default "+
			"/tmp/mdsmith-bench); CI passes a fresh path.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith-release: pgo"); code >= 0 {
			return code
		}
	}
	if fs.NArg() > 1 {
		fs.Usage()
		return 2
	}
	workdir := ""
	if fs.NArg() == 1 {
		workdir = fs.Arg(0)
	}
	return reportError(release.PGO(root, workdir))
}
