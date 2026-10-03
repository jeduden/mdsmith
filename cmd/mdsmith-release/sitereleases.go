package main

import (
	"fmt"
	"os"
	"path/filepath"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/release"
)

func runSyncReleases(root string, args []string) int {
	fs := flag.NewFlagSet("sync-releases", flag.ContinueOnError)
	out := fs.String("out", filepath.Join("website", "data", "releases.json"),
		"data file to write (relative paths resolve against the repo root)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release sync-releases [--out <path>]\n\n"+
			"List every published GitHub release and write the\n"+
			"mdsmith.dev /releases/ page's data file: stable releases\n"+
			"and release candidates, each newest first, with their\n"+
			"notes. Drafts are skipped. Reads GITHUB_REPOSITORY,\n"+
			"GITHUB_TOKEN, and GITHUB_API_URL.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith-release: sync-releases"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return 2
	}
	path := *out
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	return reportError(release.SyncReleases(githubRepoFromEnv(), path))
}
