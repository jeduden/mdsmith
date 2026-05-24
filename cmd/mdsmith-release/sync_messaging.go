package main

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/release"
)

// runSyncMessaging is the entry point for the `sync-messaging`
// subcommand. Loads docs/brand/messaging.md via `mdsmith
// extract`, then either applies the canonical values to every
// tracked surface (default) or reports drift (--check).
func runSyncMessaging(root string, args []string) int {
	fs := flag.NewFlagSet("sync-messaging", flag.ContinueOnError)
	check := fs.Bool("check", false,
		"exit non-zero if any tracked surface drifts from the source (no edits)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release sync-messaging [--check]\n\n"+
			"Project docs/brand/messaging.md through `mdsmith extract`\n"+
			"and propagate the slogan, lead, tagline, and per-surface\n"+
			"descriptions into every tracked surface (READMEs, package\n"+
			"manifests, hugo.toml, hero front matter, plugin manifests).\n"+
			"Without --check, edits files in place. With --check, only\n"+
			"reports drift and exits non-zero on the first diff.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr,
			"mdsmith-release: sync-messaging"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return 2
	}
	m, err := release.LoadMessaging(root)
	if err != nil {
		return reportError(err)
	}
	if *check {
		return runSyncMessagingCheck(root, m)
	}
	return runSyncMessagingApply(root, m)
}

func runSyncMessagingApply(root string, m *release.Messaging) int {
	results, err := release.ApplyMessaging(root, m)
	if err != nil {
		return reportError(err)
	}
	changed := 0
	for _, r := range results {
		marker := "·"
		if r.Changed {
			marker = "✓"
			changed++
		}
		_, _ = fmt.Fprintf(os.Stdout, "  %s %s\n", marker, r.Target.Label)
	}
	_, _ = fmt.Fprintf(os.Stdout,
		"messaging: %d target(s) updated, %d unchanged\n",
		changed, len(results)-changed)
	return 0
}

func runSyncMessagingCheck(root string, m *release.Messaging) int {
	drifts, err := release.CheckMessaging(root, m)
	if err != nil {
		return reportError(err)
	}
	if len(drifts) == 0 {
		_, _ = fmt.Fprintln(os.Stdout,
			"messaging: every tracked surface matches the source")
		return 0
	}
	_, _ = fmt.Fprint(os.Stderr, release.FormatDrift(drifts))
	return 1
}
