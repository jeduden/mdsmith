package main

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/release"
)

func runPublishRelease(_ string, args []string) int {
	fs := flag.NewFlagSet("publish-release", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release publish-release\n\n"+
			"Flip the draft GitHub release for the current tag to a\n"+
			"published release. The `release` job uploads every asset\n"+
			"to a draft (still mutable); this is the final atomic step\n"+
			"that publishes it, yielding an immutable release. Reads\n"+
			"GITHUB_REPOSITORY, RELEASE_TAG, GITHUB_TOKEN, and\n"+
			"GITHUB_API_URL from the environment. Idempotent.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith-release: publish-release"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return 2
	}

	return reportError(release.PublishRelease(release.PublishReleaseOptions{
		Repository: os.Getenv("GITHUB_REPOSITORY"),
		Tag:        os.Getenv("RELEASE_TAG"),
		Token:      os.Getenv("GITHUB_TOKEN"),
		APIBaseURL: os.Getenv("GITHUB_API_URL"),
	}))
}
