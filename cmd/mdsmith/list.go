package main

import (
	"fmt"
	"os"
)

const listUsage = `Usage: mdsmith list <subcommand> [flags] [args]

Subcommands:
  query <cue-expr>      Select Markdown files whose front matter
                        satisfies a CUE expression.
  backlinks <target>    List workspace links that point at a file
                        (optionally scoped to an anchor).

Run 'mdsmith list <subcommand> --help' for flags and exit codes.
`

// runList dispatches the list subcommand to its selection-style
// children (query, backlinks, …). Each child runs an independent walk
// of the workspace and emits matches in its own shape; the parent is
// just a router.
func runList(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, listUsage)
		return 0
	}
	switch args[0] {
	case "--help", "-h":
		fmt.Fprint(os.Stderr, listUsage)
		return 0
	case "query":
		return runQuery(args[1:])
	case "backlinks":
		return runBacklinks(args[1:])
	default:
		fmt.Fprintf(os.Stderr,
			"mdsmith: list: unknown subcommand %q\n\n%s",
			args[0], listUsage)
		return 2
	}
}
