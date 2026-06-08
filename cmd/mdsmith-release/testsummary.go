package main

import (
	"fmt"
	"io"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/release"
)

// runTestSummary reads a `go test -json` stream on stdin, echoes a
// terse test log to stdout, and appends a per-layer count table to
// the GitHub step summary (or prints it when run outside CI).
func runTestSummary(root string, args []string) int {
	fs := flag.NewFlagSet("test-summary", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr,
			"Usage: go test -json ./... | mdsmith-release test-summary\n\n"+
				"Read a `go test -json` event stream on stdin, re-emit a\n"+
				"terse test log to stdout, and append a unit/integration/e2e\n"+
				"count table to $GITHUB_STEP_SUMMARY (or print it when the\n"+
				"variable is unset). Tests are classified by file location:\n"+
				"internal/integration is the integration layer, *e2e*_test.go\n"+
				"files are the e2e layer, everything else is unit.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith-release: test-summary"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return 2
	}
	counts, err := release.SummarizeTestRun(root, os.Stdin, os.Stdout)
	if err != nil {
		return reportError(err)
	}
	table := release.RenderTestSummaryMarkdown(counts)
	if path := os.Getenv("GITHUB_STEP_SUMMARY"); path != "" {
		if err := appendFile(path, table); err != nil {
			return reportError(err)
		}
		// The table is on the summary page; keep the console to a recap.
		fmt.Print(release.RenderTestSummaryLine(counts))
	} else {
		fmt.Print(table)
	}
	return 0
}

// appendFile appends s to the file at path, creating it if needed.
func appendFile(path, s string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // append-only; write error is reported below
	_, err = io.WriteString(f, s)
	return err
}
