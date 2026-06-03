package main

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/release"
)

func runPackageObsidian(_ string, args []string) int {
	fs := flag.NewFlagSet("package-obsidian", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr,
			"Usage: mdsmith-release package-obsidian <dist-dir> <out-dir>\n\n"+
				"Build the Obsidian plugin release zip from the built\n"+
				"<dist-dir>. The plugin version is read from\n"+
				"<dist-dir>/manifest.json, and the zip is written to\n"+
				"<out-dir>/mdsmith-obsidian-<version>.zip with exactly the\n"+
				"five files Obsidian loads (main.js, manifest.json,\n"+
				"styles.css, mdsmith.wasm, wasm_exec.js), stored flat.\n"+
				"Built with archive/zip — no `zip` binary required.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith-release: package-obsidian"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 2 {
		fs.Usage()
		return 2
	}
	_, err := release.PackageObsidian(fs.Arg(0), fs.Arg(1))
	return reportError(err)
}
