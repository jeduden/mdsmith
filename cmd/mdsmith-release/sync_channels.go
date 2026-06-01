package main

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/release"
)

// runSyncChannels is the entry point for the `sync-channels`
// subcommand. It projects every distribution-channel file through
// `mdsmith extract` and regenerates website/data/channels.yaml,
// the data the install picker reads. With --check it makes no
// edits and exits non-zero when the file drifts from the source.
func runSyncChannels(root string, args []string) int {
	fs := flag.NewFlagSet("sync-channels", flag.ContinueOnError)
	check := fs.Bool("check", false,
		"exit non-zero if the website channel data drifts from the source (no edits)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release sync-channels [--check]\n\n"+
			"Project docs/development/release-channels/*.md through\n"+
			"`mdsmith extract` and regenerate website/data/channels.yaml,\n"+
			"the data the install picker reads. Without --check, writes the\n"+
			"file in place. With --check, makes no edits, reports drift, and\n"+
			"exits non-zero.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr,
			"mdsmith-release: sync-channels"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return 2
	}
	chs, err := release.LoadChannels(root)
	if err != nil {
		return reportError(err)
	}
	if *check {
		return runSyncChannelsCheck(root, chs)
	}
	return runSyncChannelsApply(root, chs)
}

func runSyncChannelsCheck(root string, chs []release.Channel) int {
	drift, err := release.CheckChannelsData(root, chs)
	if err != nil {
		return reportError(err)
	}
	if drift {
		_, _ = fmt.Fprintln(os.Stderr,
			"channels: website/data/channels.yaml is out of date; "+
				"run `mdsmith-release sync-channels`")
		return 1
	}
	_, _ = fmt.Fprintln(os.Stdout,
		"channels: website/data/channels.yaml matches the source")
	return 0
}

func runSyncChannelsApply(root string, chs []release.Channel) int {
	changed, err := release.WriteChannelsData(root, chs)
	if err != nil {
		return reportError(err)
	}
	if changed {
		_, _ = fmt.Fprintln(os.Stdout,
			"channels: wrote website/data/channels.yaml")
	} else {
		_, _ = fmt.Fprintln(os.Stdout,
			"channels: website/data/channels.yaml already up to date")
	}
	return 0
}
