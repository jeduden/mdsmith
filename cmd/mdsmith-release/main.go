// mdsmith-release is the internal CLI the GitHub Actions release
// pipeline invokes. It is intentionally NOT part of the
// user-facing `mdsmith` binary — its commands (stamp tracked
// manifests, verify them, build npm sub-packages, build PyPI
// wheels, secret-rotation reminder, record rotation) are only
// useful inside the workflow.
//
// Usage:
//
//	mdsmith-release stamp <version>
//	mdsmith-release check
//	mdsmith-release build-npm <artifacts-dir> <out-dir>
//	mdsmith-release build-wheels <artifacts-dir> <out-dir>
//	mdsmith-release build-flatpak <artifacts-dir> <out-dir>
//	mdsmith-release package-obsidian <dist-dir> <out-dir>
//	mdsmith-release sync-docs <src-dir> <dst-dir>
//	mdsmith-release build-website [--no-fix] [src-dir] [dst-dir]
//	mdsmith-release verify-website-links --dir <html-dir> [--base-url <url>]
//	mdsmith-release verify-install-picker --dir <html-dir>
//	mdsmith-release publish-release
//	mdsmith-release sbom <out-path>
//	mdsmith-release check-secret-rotations
//	mdsmith-release record-rotation <ENTRY_TITLE> <YYYY-MM-DD>
//	mdsmith-release merge-coverage -o <out> <profile>...
//	mdsmith-release bench [workdir]
//	mdsmith-release pull-site-assets
//	mdsmith-release sync-messaging [--check]
//
// Each subcommand operates relative to the current working
// directory, which is the repo root in CI.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/release"
)

const usageText = `Usage: mdsmith-release <command> [args]

Commands:
  stamp <version>                 Rewrite tracked manifests to <version>.
  check                           Verify tracked manifests are at the dev sentinel.
  build-npm <artifacts> <out>     Build npm platform sub-packages.
  build-wheels <artifacts> <out>  Build platform-tagged Python wheels.
  build-flatpak <art> <out>       Stage the .flatpak bundle's manifest + Linux binaries.
  package-obsidian <dist> <out>   Build the Obsidian plugin release zip from <dist>.
  sync-docs <src> <dst>           Snapshot docs/ into a Hugo content tree.
  build-website [--no-fix] [src] [dst]
                                  mdsmith fix (unless --no-fix) + sync-docs.
  verify-website-links --dir <dir> [--base-url <url>]
                                  Probe rendered HTML for render-link regressions.
  verify-install-picker --dir <dir>
                                  Check the rendered install picker matches the channel docs.
  publish-release                 Flip the tag's draft release to published.
  sbom <out-path>                 Emit a CycloneDX SBOM of the Go module to <out-path>.
  check-secret-rotations          Open GitHub issues for secrets due for rotation.
  record-rotation <title> <date>  Update lastRotated in a per-secret rotation file.
  merge-coverage -o <out> <p>...  Merge coverage profiles by summing hit counts.
  bench [workdir]                 Run the pinned cross-tool benchmark; promote JSON + fragments.
  pull-site-assets                Fetch the published demo GIF + benchmark numbers for the site build.
  sync-messaging [--check]        Propagate docs/brand/messaging.md into every tracked surface (or check drift).
  sync-parity-rules [--check]     Regenerate the parity-convention disabled-rules fragment
                                  (or check drift).
  sync-channels [--check]         Regenerate website/data/channels.yaml from the channel files
                                  (or check drift).
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
	return dispatch(cmd, root, rest)
}

// dispatch routes one subcommand to its runner. Split out of run
// so each is a short, single-purpose function (run owns argv/cwd
// preconditions; dispatch owns the command table).
func dispatch(cmd, root string, rest []string) int {
	switch cmd {
	case "-h", "--help", "help":
		fmt.Print(usageText)
		return 0
	case "stamp":
		return runStamp(root, rest)
	case "check":
		return runCheck(root, rest)
	case "build-npm":
		return runBuildNpm(root, rest)
	case "build-wheels":
		return runBuildWheels(root, rest)
	case "build-flatpak":
		return runBuildFlatpak(root, rest)
	case "package-obsidian":
		return runPackageObsidian(root, rest)
	case "sync-docs":
		return runSyncDocs(root, rest)
	case "build-website":
		return runBuildWebsite(root, rest)
	case "verify-website-links":
		return runVerifyWebsiteLinks(root, rest)
	case "verify-install-picker":
		return runVerifyInstallPicker(root, rest)
	case "publish-release":
		return runPublishRelease(root, rest)
	case "sbom":
		return runSBOM(root, rest)
	case "check-secret-rotations":
		return runCheckSecretRotations(root, rest)
	case "record-rotation":
		return runRecordRotation(root, rest)
	case "merge-coverage":
		return runMergeCoverage(root, rest)
	case "bench":
		return runBench(root, rest)
	case "pull-site-assets":
		return runPullSiteAssets(root, rest)
	default:
		return dispatchGenerators(cmd, root, rest)
	}
}

// dispatchGenerators routes the doc-generation subcommands, split
// from dispatch so each routing function stays short. Unknown
// commands fall through to the usage error here.
func dispatchGenerators(cmd, root string, rest []string) int {
	switch cmd {
	case "sync-messaging":
		return runSyncMessaging(root, rest)
	case "sync-parity-rules":
		return runSyncParityRules(root, rest)
	case "sync-channels":
		return runSyncChannels(root, rest)
	default:
		fmt.Fprintf(os.Stderr, "mdsmith-release: unknown command %q\n%s", cmd, usageText)
		return 2
	}
}

func runStamp(root string, args []string) int {
	fs := flag.NewFlagSet("stamp", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release stamp <version>\n\n"+
			"Rewrite every tracked manifest's version field to <version>\n"+
			"(no leading 'v'). Idempotent.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith-release: stamp"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	return reportError(release.Stamp(root, fs.Arg(0)))
}

func runCheck(root string, args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release check\n\n"+
			"Verify every tracked manifest is pinned at the dev\n"+
			"sentinel ("+release.DevSentinel+"). Used by the\n"+
			"version-guard CI job.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith-release: check"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return 2
	}
	if err := release.Check(root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("all manifests pinned at " + release.DevSentinel)
	return 0
}

func runBuildNpm(root string, args []string) int {
	fs := flag.NewFlagSet("build-npm", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release build-npm <artifacts-dir> <out-dir>\n\n"+
			"Emit one ready-to-publish npm sub-package per supported\n"+
			"platform under <out-dir>, pulling each binary from\n"+
			"<artifacts-dir>.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith-release: build-npm"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 2 {
		fs.Usage()
		return 2
	}
	return reportError(release.BuildNpmPlatforms(root, fs.Arg(0), fs.Arg(1)))
}

func runBuildWheels(root string, args []string) int {
	fs := flag.NewFlagSet("build-wheels", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release build-wheels <artifacts-dir> <out-dir>\n\n"+
			"Build one platform-tagged wheel per supported host under\n"+
			"<out-dir>, staging each binary from <artifacts-dir>.\n"+
			"Requires python -m build, python -m wheel, and the\n"+
			"hatchling build backend on PATH.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith-release: build-wheels"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 2 {
		fs.Usage()
		return 2
	}
	return reportError(release.BuildWheels(root, fs.Arg(0), fs.Arg(1)))
}

func runBuildFlatpak(_ string, args []string) int {
	fs := flag.NewFlagSet("build-flatpak", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr,
			"Usage: mdsmith-release build-flatpak <artifacts-dir> <out-dir>\n\n"+
				"Stage the flatpak-builder manifest for\n"+
				"io.github.jeduden.mdsmith and the x86_64 Linux release binary\n"+
				"it references from <artifacts-dir> into <out-dir>. The\n"+
				"release.yml flatpak job then runs flatpak-builder and\n"+
				"`flatpak build-bundle` to produce the single-file .flatpak\n"+
				"attached to the GitHub release. x86_64 is the only arch CI\n"+
				"bundles.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith-release: build-flatpak"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 2 {
		fs.Usage()
		return 2
	}
	return reportError(release.BuildFlatpak(fs.Arg(0), fs.Arg(1)))
}

func runSyncDocs(_ string, args []string) int {
	fs := flag.NewFlagSet("sync-docs", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release sync-docs <src-dir> <dst-dir>\n\n"+
			"Snapshot <src-dir> into <dst-dir> for a Hugo build,\n"+
			"applying the transforms the rendered site needs:\n"+
			"  - drop proto.md schema templates\n"+
			"  - rename index.md to _index.md\n"+
			"  - skip non-markdown, non-image files\n"+
			"  - escape {{< ... >}} and {{%% ... %%}} patterns so\n"+
			"    Hugo renders them verbatim instead of resolving\n"+
			"    them as shortcodes\n"+
			"  - promote the first body H1 to front-matter\n"+
			"    title: and remove it from the body\n"+
			"  - strip mdsmith <?...?> directive markers (kept\n"+
			"    when shown inside code fences or inline code)\n\n"+
			"<dst-dir> is removed before the copy. Idempotent.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith-release: sync-docs"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 2 {
		fs.Usage()
		return 2
	}
	return reportError(release.SyncDocs(fs.Arg(0), fs.Arg(1)))
}

func runBuildWebsite(_ string, args []string) int {
	fs := flag.NewFlagSet("build-website", flag.ContinueOnError)
	noFix := fs.Bool("no-fix", false,
		"skip the `mdsmith fix` pass and only snapshot the source tree")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release build-website [--no-fix] [src-dir] [dst-dir]\n\n"+
			"Prepare the Hugo content tree in one step:\n"+
			"  1. unless --no-fix, run `mdsmith fix <src-dir>` so every\n"+
			"     <?catalog?>/<?include?> body is current\n"+
			"  2. snapshot <src-dir> into <dst-dir> (see sync-docs)\n\n"+
			"src-dir defaults to ./docs, dst-dir to\n"+
			"./website/content/docs. Run from the repo root.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith-release: build-website"); code >= 0 {
			return code
		}
	}
	if fs.NArg() > 2 {
		fs.Usage()
		return 2
	}
	src := "./docs"
	dst := "./website/content/docs"
	if fs.NArg() >= 1 {
		src = fs.Arg(0)
	}
	if fs.NArg() == 2 {
		dst = fs.Arg(1)
	}
	return reportError(release.BuildWebsite(src, dst, !*noFix))
}

func runVerifyWebsiteLinks(_ string, args []string) int {
	fs := flag.NewFlagSet("verify-website-links", flag.ContinueOnError)
	dir := fs.String("dir", "",
		"path to the Hugo output directory (usually website/public)")
	baseURL := fs.String("base-url", "",
		"baseURL Hugo was built with — its path component is the expected prefix on site-absolute hrefs")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr,
			"Usage: mdsmith-release verify-website-links --dir <dir> [--base-url <url>]\n\n"+
				"Probe the rendered HTML in <dir> for the rewrites the\n"+
				"render-link hook is responsible for: sibling `.md` →\n"+
				"page permalink, `index.md` drop → section URL, no\n"+
				"unpublished `README.md` hrefs in rule pages,\n"+
				"javascript:/data: hrefs neutralized, and the configured\n"+
				"baseURL prefix on site-absolute hrefs. Exits non-zero on\n"+
				"the first failed probe.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith-release: verify-website-links"); code >= 0 {
			return code
		}
	}
	if *dir == "" {
		fs.Usage()
		return 2
	}
	return reportError(release.VerifyWebsiteLinks(*dir, *baseURL))
}

func runVerifyInstallPicker(root string, args []string) int {
	fs := flag.NewFlagSet("verify-install-picker", flag.ContinueOnError)
	dir := fs.String("dir", "",
		"path to the Hugo output directory (usually website/public)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr,
			"Usage: mdsmith-release verify-install-picker --dir <dir>\n\n"+
				"Probe the rendered homepage in <dir> for the install\n"+
				"picker contract: one data-cmd-default row per channel,\n"+
				"and for every channel that declares command-windows a\n"+
				"matching data-cmd-windows attribute plus a <noscript>\n"+
				"fallback. The expected commands come from the picker's\n"+
				"own input, website/data/channels.yaml, so a Hugo-template\n"+
				"regression that the channels.yaml round-trip cannot see\n"+
				"fails here. Exits non-zero on the first mismatch.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith-release: verify-install-picker"); code >= 0 {
			return code
		}
	}
	if *dir == "" {
		fs.Usage()
		return 2
	}
	// Compare the render against its actual input (channels.yaml),
	// not a re-derivation from the docs — docs<->yaml drift is the
	// sync-channels check's job, kept separate so this probe points
	// only at template regressions.
	channels, err := release.LoadChannelsFromDataFile(root)
	if err != nil {
		return reportError(err)
	}
	return reportError(release.VerifyInstallPicker(*dir, channels))
}

// reportFlagParseErr mirrors the helper in cmd/mdsmith/main.go:
// pflag with ContinueOnError silently returns the parse error
// without writing to fs.Output(), so subcommands that just
// `return 2` left the user with nothing on stderr.
//
//   - nil           → -1 (caller continues)
//   - flag.ErrHelp  →  0 (Usage was already printed by pflag)
//   - any other err →  2 with `<prefix>: <err>` on stderr
func reportFlagParseErr(err error, stderr io.Writer, prefix string) int {
	if err == nil {
		return -1
	}
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	_, _ = fmt.Fprintf(stderr, "%s: %v\n", prefix, err)
	return 2
}

func runCheckSecretRotations(root string, args []string) int {
	fs := flag.NewFlagSet("check-secret-rotations", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release check-secret-rotations\n\n"+
			"Walk docs/development/secret-rotations/, classify each\n"+
			"secret by (today - lastRotated), and open a labelled\n"+
			"GitHub issue for any secret within the reminder window\n"+
			"or already overdue. Idempotent: never opens a duplicate.\n"+
			"Requires `gh` on PATH with GH_TOKEN set. Reads\n"+
			"REMINDER_ASSIGNEE from the environment; if empty, the\n"+
			"--assignee flag is omitted entirely.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith-release: check-secret-rotations"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return 2
	}
	res, err := release.CheckSecretRotations(root, release.CheckRotationsOptions{
		Now: time.Now(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith-release: %v\n", err)
		return 1
	}
	printCheckResult(os.Stdout, res)
	return 0
}

// printCheckResult writes the human-readable summary of a
// CheckSecretRotations run to out. Extracted from
// runCheckSecretRotations so the three formatting branches
// (some opened, some skipped, neither → "no secrets due") are
// unit-testable without a fake `gh` binary.
func printCheckResult(out io.Writer, res release.CheckSecretRotationsResult) {
	if len(res.Opened) > 0 {
		_, _ = fmt.Fprintf(out, "opened secret-rotation reminders for: %v\n", res.Opened)
	}
	if len(res.Skipped) > 0 {
		_, _ = fmt.Fprintf(out, "existing open reminders (skipped): %v\n", res.Skipped)
	}
	if len(res.Opened) == 0 && len(res.Skipped) == 0 {
		_, _ = fmt.Fprintln(out, "no secrets due for rotation")
	}
}

func runRecordRotation(root string, args []string) int {
	fs := flag.NewFlagSet("record-rotation", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release record-rotation <ENTRY_TITLE> <YYYY-MM-DD>\n\n"+
			"Update the `lastRotated` date for the per-secret file in\n"+
			"docs/development/secret-rotations/ whose front matter\n"+
			"`title` matches <ENTRY_TITLE>. Exit 0 with no rewrite if\n"+
			"the date is already at the requested value.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith-release: record-rotation"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 2 {
		fs.Usage()
		return 2
	}
	entryTitle := fs.Arg(0)
	date := fs.Arg(1)
	changed, err := release.RecordRotation(root, entryTitle, date)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith-release: %v\n", err)
		return 1
	}
	if !changed {
		fmt.Printf("%s lastRotated already at %s; no change\n", entryTitle, date)
		return 0
	}
	fmt.Printf("updated %s lastRotated -> %s\n", entryTitle, date)
	return 0
}

func runBench(root string, args []string) int {
	fs := flag.NewFlagSet("bench", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release bench [workdir]\n\n"+
			"Run the pinned, integrity-verified cross-tool Markdown\n"+
			"linter benchmark from the repo root: build mdsmith,\n"+
			"fetch + SHA-256-verify hyperfine/mado/panache/rumdl,\n"+
			"`npm ci` markdownlint-cli2 from the committed lockfile,\n"+
			"build the repo + neutral corpora, drive hyperfine,\n"+
			"promote the JSON into "+"docs/research/benchmarks/data/,\n"+
			"and regenerate the doc fragments via gen_fragments.py.\n"+
			"workdir caches built/fetched binaries and the corpora\n"+
			"(default "+"/tmp/mdsmith-bench); CI passes a fresh path.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith-release: bench"); code >= 0 {
			return code
		}
	}
	if fs.NArg() > 1 {
		fs.Usage()
		return 2
	}
	workdir := ""
	if fs.NArg() == 1 {
		workdir = fs.Arg(0)
	}
	return reportError(release.Bench(root, workdir))
}

func runPullSiteAssets(root string, args []string) int {
	fs := flag.NewFlagSet("pull-site-assets", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release pull-site-assets\n\n"+
			"Fetch the published demo GIF and cross-tool benchmark\n"+
			"numbers from the orphan `assets` branch into the working\n"+
			"tree before the Hugo build. The demo GIF is required;\n"+
			"the benchmark fragments fall back to the committed\n"+
			"in-repo snapshot when the assets branch has not yet\n"+
			"published them. Run from the repo root.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith-release: pull-site-assets"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return 2
	}
	return reportError(release.PullSiteAssets(root))
}

func reportError(err error) int {
	if err == nil {
		return 0
	}
	fmt.Fprintf(os.Stderr, "mdsmith-release: %v\n", err)
	return 1
}
