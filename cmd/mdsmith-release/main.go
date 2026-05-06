// mdsmith-release is the internal CLI the GitHub Actions release
// pipeline invokes. It is intentionally NOT part of the
// user-facing `mdsmith` binary — its commands (stamp tracked
// manifests, verify them, build npm sub-packages, build PyPI
// wheels) are only useful inside the workflow.
//
// Usage:
//
//	mdsmith-release stamp <version>
//	mdsmith-release check
//	mdsmith-release build-npm <artifacts-dir> <out-dir>
//	mdsmith-release build-wheels <artifacts-dir> <out-dir>
//
// Each subcommand operates relative to the current working
// directory, which is the repo root in CI.
package main

import (
	"fmt"
	"os"

	"github.com/jeduden/mdsmith/internal/release"
)

const usageText = `usage: mdsmith-release <command> [args]

Commands:
  stamp <version>                 Rewrite tracked manifests to <version>.
  check                           Verify tracked manifests are at the dev sentinel.
  build-npm <artifacts> <out>     Build npm platform sub-packages.
  build-wheels <artifacts> <out>  Build platform-tagged Python wheels.
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usageText)
		return 2
	}
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith-release: %v\n", err)
		return 1
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "-h", "--help", "help":
		fmt.Print(usageText)
		return 0
	case "stamp":
		if len(rest) != 1 {
			fmt.Fprint(os.Stderr, "usage: mdsmith-release stamp <version>\n")
			return 2
		}
		return reportError(release.Stamp(root, rest[0]))
	case "check":
		if len(rest) != 0 {
			fmt.Fprint(os.Stderr, "usage: mdsmith-release check\n")
			return 2
		}
		if err := release.Check(root); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("all manifests pinned at " + release.DevSentinel)
		return 0
	case "build-npm":
		if len(rest) != 2 {
			fmt.Fprint(os.Stderr, "usage: mdsmith-release build-npm <artifacts> <out>\n")
			return 2
		}
		return reportError(release.BuildNpmPlatforms(root, rest[0], rest[1]))
	case "build-wheels":
		if len(rest) != 2 {
			fmt.Fprint(os.Stderr, "usage: mdsmith-release build-wheels <artifacts> <out>\n")
			return 2
		}
		return reportError(release.BuildWheels(root, rest[0], rest[1]))
	default:
		fmt.Fprintf(os.Stderr, "mdsmith-release: unknown command %q\n%s", cmd, usageText)
		return 2
	}
}

func reportError(err error) int {
	if err == nil {
		return 0
	}
	fmt.Fprintf(os.Stderr, "mdsmith-release: %v\n", err)
	return 1
}
