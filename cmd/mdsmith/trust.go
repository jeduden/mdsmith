package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	flag "github.com/spf13/pflag"

	buildexec "github.com/jeduden/mdsmith/internal/build"
)

// runTrust implements the `mdsmith trust` subcommand. It shows the diff
// between the stored trust marker and the current .mdsmith.yml, then —
// unless --yes is passed — prompts for confirmation before rewriting the
// marker. Marking the config trusted is what lets `mdsmith fix` run the
// build pass on this clone.
func runTrust(args []string) int {
	return runTrustIO(args, os.Stdin, os.Stdout, os.Stderr)
}

// runTrustIO is the injectable form of runTrust; tests supply alternate
// streams to drive the prompt and capture output.
func runTrustIO(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("trust", flag.ContinueOnError)
	var (
		configPath string
		yes        bool
	)
	fs.StringVarP(&configPath, "config", "c", "", "Override config file path")
	fs.BoolVarP(&yes, "yes", "y", false, "Skip the confirmation prompt and trust immediately")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage: mdsmith trust [flags]\n\n"+
			"Review the .mdsmith.yml diff since it was last trusted and update the\n"+
			".mdsmith.yml.trust marker so `mdsmith fix` may run the build pass on this clone.\n\n"+
			"Flags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, stderr, "mdsmith: trust"); code >= 0 {
			return code
		}
	}

	// Pin the same config file the build pass would load, so `mdsmith trust`
	// and the gate agree even under -c / config discovery.
	cfgFile := discoverConfigPath(configPath)

	diff, changed, err := buildexec.TrustDiff(cfgFile)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "mdsmith: trust: %v\n", err)
		return 2
	}
	if !changed {
		_, _ = fmt.Fprintf(stdout, "%s is already trusted; no change.\n", buildexec.TrustMarkerPath(cfgFile))
		return 0
	}

	_, _ = fmt.Fprint(stdout, diff)
	if !yes && !confirmTrust(stdin, stdout) {
		_, _ = fmt.Fprintln(stdout, "Aborted; trust marker unchanged.")
		return 1
	}

	if err := buildexec.WriteTrustMarker(cfgFile); err != nil {
		_, _ = fmt.Fprintf(stderr, "mdsmith: trust: %v\n", err)
		return 2
	}
	_, _ = fmt.Fprintf(stdout, "Trusted %s.\n", buildexec.TrustMarkerPath(cfgFile))
	return 0
}

// confirmTrust prompts on stdout and reads a yes/no answer from stdin.
// Only an explicit "y"/"yes" (case-insensitive) confirms; anything else,
// including EOF, declines.
func confirmTrust(stdin io.Reader, stdout io.Writer) bool {
	_, _ = fmt.Fprint(stdout, "\nTrust this configuration? [y/N] ")
	sc := bufio.NewScanner(stdin)
	if !sc.Scan() {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(sc.Text()))
	return answer == "y" || answer == "yes"
}
