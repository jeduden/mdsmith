package main

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/release"
)

// githubRepoFromEnv reads the repository coordinates every GitHub
// Actions job exports.
func githubRepoFromEnv() release.GitHubRepoOptions {
	return release.GitHubRepoOptions{
		Repository: os.Getenv("GITHUB_REPOSITORY"),
		Token:      os.Getenv("GITHUB_TOKEN"),
		APIBaseURL: os.Getenv("GITHUB_API_URL"),
	}
}

func runRCVersion(_ string, args []string) int {
	fs := flag.NewFlagSet("rc-version", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release rc-version\n\n"+
			"Print the version the next release candidate carries: the\n"+
			"minor after the latest stable vX.Y.Z tag, suffixed -rc.N\n"+
			"with N one above the highest existing candidate for that\n"+
			"minor (v0.55.1 + v0.56.0-rc.2 -> v0.56.0-rc.3). Reads\n"+
			"GITHUB_REPOSITORY, GITHUB_TOKEN, and GITHUB_API_URL.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith-release: rc-version"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return 2
	}
	v, err := release.ResolveRCVersion(githubRepoFromEnv())
	if err != nil {
		return reportError(err)
	}
	fmt.Println(v)
	return 0
}

func runReleaseNotes(_ string, args []string) int {
	fs := flag.NewFlagSet("release-notes", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release release-notes <out-path>\n\n"+
			"Write GitHub-generated release notes for RELEASE_TAG at\n"+
			"GITHUB_SHA to <out-path>. The range starts at the last\n"+
			"stable tag below RELEASE_TAG, never at a release\n"+
			"candidate, so a stable release lists every merge since\n"+
			"the previous stable one. Reads GITHUB_REPOSITORY,\n"+
			"GITHUB_TOKEN, and GITHUB_API_URL.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith-release: release-notes"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	body, err := release.GenerateReleaseNotes(release.NotesOptions{
		GitHubRepoOptions: githubRepoFromEnv(),
		Tag:               os.Getenv("RELEASE_TAG"),
		Target:            os.Getenv("GITHUB_SHA"),
	})
	if err != nil {
		return reportError(err)
	}
	return reportError(os.WriteFile(fs.Arg(0), []byte(body+"\n"), 0o644))
}
