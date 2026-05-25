package main

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/release"
)

// runSyncCoverageMatrix is the entry point for the
// `sync-coverage-matrix` subcommand. Reads each rule README's
// peer-linter mappings from front matter and rewrites the
// generated coverage page at
// docs/research/markdownlint-coverage/README.md. With --check
// it makes no edits and exits non-zero if the file has
// drifted from the source.
func runSyncCoverageMatrix(root string, args []string) int {
	fs := flag.NewFlagSet("sync-coverage-matrix", flag.ContinueOnError)
	check := fs.Bool("check", false,
		"exit non-zero if the coverage matrix has drifted from the source (no edits)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release sync-coverage-matrix [--check]\n\n"+
			"Render "+release.CoverageMatrixFile+" from each rule\n"+
			"README's front matter (markdownlint/rumdl/mado/panache\n"+
			"mappings, plus the upstream default-enabled state per\n"+
			"peer). Without --check, edits the file in place. With\n"+
			"--check, makes no edits; reports drift and exits non-zero.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr,
			"mdsmith-release: sync-coverage-matrix"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return 2
	}
	if *check {
		return runSyncCoverageMatrixCheck(root)
	}
	return runSyncCoverageMatrixApply(root)
}

func runSyncCoverageMatrixApply(root string) int {
	changed, err := release.ApplyCoverageMatrix(root)
	if err != nil {
		return reportError(err)
	}
	if changed {
		_, _ = fmt.Fprintln(os.Stdout,
			"coverage matrix: rewrote "+release.CoverageMatrixFile)
	} else {
		_, _ = fmt.Fprintln(os.Stdout,
			"coverage matrix: "+release.CoverageMatrixFile+" already in sync")
	}
	return 0
}

func runSyncCoverageMatrixCheck(root string) int {
	msg, err := release.CheckCoverageMatrix(root)
	if err != nil {
		return reportError(err)
	}
	if msg == "" {
		_, _ = fmt.Fprintln(os.Stdout,
			"coverage matrix: "+release.CoverageMatrixFile+" matches the source")
		return 0
	}
	_, _ = fmt.Fprintln(os.Stderr, msg)
	return 1
}
