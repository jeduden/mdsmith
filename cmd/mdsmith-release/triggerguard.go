package main

import (
	"fmt"
	"io"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/release"
)

func runCheckReleaseTrigger(_ string, args []string) int {
	fs := flag.NewFlagSet("check-release-trigger", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release check-release-trigger\n\n"+
			"Inspect the current GitHub Actions event and write\n"+
			"`should_run` / `create_release_is_draft` outputs for\n"+
			"release.yml's create-event guard. Reads EVENT_NAME,\n"+
			"CREATE_REF_TYPE, RELEASE_TAG, GITHUB_REPOSITORY,\n"+
			"GITHUB_TOKEN, GITHUB_API_URL, and GITHUB_OUTPUT from\n"+
			"the environment.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith-release: check-release-trigger"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return 2
	}

	res, err := release.CheckReleaseTrigger(release.TriggerGuardOptions{
		EventName:  os.Getenv("EVENT_NAME"),
		Repository: os.Getenv("GITHUB_REPOSITORY"),
		RefName:    os.Getenv("RELEASE_TAG"),
		RefType:    os.Getenv("CREATE_REF_TYPE"),
		Token:      os.Getenv("GITHUB_TOKEN"),
		APIBaseURL: os.Getenv("GITHUB_API_URL"),
	})
	if err != nil {
		return reportError(err)
	}
	if err := writeReleaseTriggerGuardOutput(os.Stdout, os.Getenv("GITHUB_OUTPUT"), res); err != nil {
		return reportError(err)
	}
	return 0
}

func writeReleaseTriggerGuardOutput(stdout io.Writer, path string, res release.TriggerGuardResult) error {
	lines := fmt.Sprintf(
		"should_run=%t\ncreate_release_is_draft=%t\n",
		res.ShouldRun,
		res.CreateReleaseIsDraft,
	)
	if path == "" {
		_, err := io.WriteString(stdout, lines)
		return err
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	_, err = f.WriteString(lines)
	return err
}
