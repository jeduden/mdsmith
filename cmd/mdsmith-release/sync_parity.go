package main

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/release"

	// Register the production rule set so the generator can resolve
	// each parity rule's MDS id and default-enabled state.
	_ "github.com/jeduden/mdsmith/internal/rules/all"
)

// runSyncParityRules is the entry point for the `sync-parity-rules`
// subcommand. It regenerates the parity-rules fragment at
// docs/research/benchmarks/parity-rules.fragment.md from the
// <linter>-parity conventions. The conventions reference and the
// benchmark page <?include?> that fragment, so one regen keeps both
// docs in sync with the conventions. With --check it makes no edits
// and exits non-zero if the fragment has drifted.
func runSyncParityRules(root string, args []string) int {
	fs := flag.NewFlagSet("sync-parity-rules", flag.ContinueOnError)
	check := fs.Bool("check", false,
		"exit non-zero if the parity-rules fragment has drifted from the conventions (no edits)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release sync-parity-rules [--check]\n\n"+
			"Render "+release.ParityRulesFragmentFile+" from the <linter>-parity\n"+
			"conventions (one table per convention: each rule it sets, with its\n"+
			"MDS id, mdsmith default state, and whether parity enables or\n"+
			"disables it). Without --check, edits the file in place. With\n"+
			"--check, makes no edits; reports drift and exits non-zero.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr,
			"mdsmith-release: sync-parity-rules"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return 2
	}
	if *check {
		return runSyncParityRulesCheck(root)
	}
	return runSyncParityRulesApply(root)
}

func runSyncParityRulesApply(root string) int {
	changed, err := release.ApplyParityRulesFragment(root)
	if err != nil {
		return reportError(err)
	}
	if changed {
		_, _ = fmt.Fprintln(os.Stdout,
			"parity rules: rewrote "+release.ParityRulesFragmentFile)
	} else {
		_, _ = fmt.Fprintln(os.Stdout,
			"parity rules: "+release.ParityRulesFragmentFile+" already in sync")
	}
	return 0
}

func runSyncParityRulesCheck(root string) int {
	msg, err := release.CheckParityRulesFragment(root)
	if err != nil {
		return reportError(err)
	}
	if msg == "" {
		_, _ = fmt.Fprintln(os.Stdout,
			"parity rules: "+release.ParityRulesFragmentFile+" matches the convention")
		return 0
	}
	_, _ = fmt.Fprintln(os.Stderr, msg)
	return 1
}
