package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/release"
)

// runSyncMessaging is the entry point for the `sync-messaging`
// subcommand. Task 3 (this commit) implements the read path:
// project docs/brand/messaging.md through mdsmith extract and
// print a human-readable summary. Task 5 wires the apply path
// (patches every tracked surface); task 6 wires --check (drift
// detection).
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
		// Apply path and drift check land in tasks 5 and 6. For
		// now task 3 only proves the load works.
		fmt.Fprintln(os.Stderr,
			"mdsmith-release: sync-messaging --check not yet implemented (plan 209 task 6)")
		return 2
	}
	printMessagingSummary(os.Stdout, m)
	return 0
}

// printMessagingSummary writes a one-line-per-field summary of m
// to out. Used in task 3 as the read-path smoke test; tasks 5/6
// replace the apply path with file patches.
func printMessagingSummary(out io.Writer, m *release.Messaging) {
	_, _ = fmt.Fprintf(out, "messaging fields loaded from %s:\n",
		release.MessagingSourceFile)
	rows := []struct {
		label, value string
	}{
		{"title", m.Title},
		{"summary", m.Summary},
		{"eyebrow", m.Eyebrow},
		{"headline", m.HeadlinePre + "_" + m.HeadlineEm + "_" + m.HeadlinePost},
		{"lead", m.Lead},
		{"tagline", m.Tagline},
		{"vscode-description", m.VSCodeDescription},
		{"claude-code-lsp-description", m.ClaudeCodeLSPDescription},
		{"claude-code-skills-description", m.ClaudeCodeSkillsDescription},
		{"claude-code-audit-description", m.ClaudeCodeAuditDescription},
	}
	for _, r := range rows {
		_, _ = fmt.Fprintf(out, "  %-32s %s\n", r.label+":", oneline(r.value))
	}
}

func oneline(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 80 {
		return s[:77] + "..."
	}
	return s
}
