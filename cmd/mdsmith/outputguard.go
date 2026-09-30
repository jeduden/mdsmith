package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/jeduden/mdsmith/internal/mdpath"
)

// runInputs describes the Markdown files a check or fix run reads, for
// the -o guard: the files it resolved, plus what decides which files a
// later run would pick up.
type runInputs struct {
	// files are the resolved input files.
	files []string
	// args are the explicit file, directory, and glob arguments.
	args []string
	// patterns are the config files: patterns of a discovery run,
	// matched relative to the working directory.
	patterns []string
}

// refuseOutputOverInput stops a check or fix run whose -o path is, or
// once written would be, one of the run's Markdown inputs: the report
// would overwrite a file the run lints or fixes, or turn up as an
// input of the next run. It prints a usage error and returns 2 before
// any file is linted, or -1 when the path is safe. "" and "-" name no
// file.
func refuseOutputOverInput(cmd, output string, in runInputs) int {
	if output == "" || output == "-" || !outputIsInput(output, in) {
		return -1
	}
	printInputRefusal(cmd, output)
	return 2
}

// refuseOutputOverStdin stops a `check -` run whose -o path is the
// file stdin reads from, as in `check - -o a.md < a.md`: the report
// would replace the file just linted. The two are compared by file
// identity before stdin is read. A pipe has no file identity, so
// `cat a.md | mdsmith check - -o a.md` cannot be detected. It prints
// the same usage error as refuseOutputOverInput and returns 2, or -1.
func refuseOutputOverStdin(cmd, output string, stdin *os.File) int {
	if output == "" || output == "-" {
		return -1
	}
	oi, oerr := os.Stat(output)
	si, serr := stdin.Stat()
	if oerr != nil || serr != nil || !os.SameFile(oi, si) {
		return -1
	}
	printInputRefusal(cmd, output)
	return 2
}

// printInputRefusal prints the usage error for an -o path that is, or
// would be, an input of the run.
func printInputRefusal(cmd, output string) {
	fmt.Fprintf(os.Stderr,
		"mdsmith: %s: refusing --output %q: it is an input of this run, or would be once written\n",
		cmd, output)
}

// outputIsInput reports whether output names one of the run's inputs.
// An existing output is compared with each resolved input by file
// identity (os.SameFile), so a relative spelling, a symlink, a hard
// link, or a case-insensitive file system cannot hide a match. A
// missing output is checked by wouldBeInput.
func outputIsInput(output string, in runInputs) bool {
	info, err := os.Stat(output)
	if err != nil {
		return wouldBeInput(output, in)
	}
	// Devices such as /dev/stdout are never inputs: the resolver
	// takes regular files only.
	if !info.Mode().IsRegular() {
		return false
	}
	for _, f := range in.files {
		if fi, err := os.Stat(f); err == nil && os.SameFile(info, fi) {
			return true
		}
	}
	return false
}

// wouldBeInput reports whether a missing output, once the report
// creates it, is a file the same run would pick up: a Markdown name
// inside a directory argument, one matching a glob argument, or, on a
// discovery run, one matching a files: pattern under the working
// directory. Directories are compared by identity after resolving
// symlinks in the output's parent. Names are matched without regard
// to case, so the check also holds on a case-insensitive file system.
// Ignore rules are not consulted, so a Markdown output name in the
// linted tree is refused even where .gitignore would skip it. A parent
// directory that does not exist makes the report fail to open anyway.
func wouldBeInput(output string, in runInputs) bool {
	if !mdpath.HasMarkdownExt(filepath.Ext(output)) {
		return false
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(output))
	if err != nil {
		return false
	}
	// Absolute, so relUnder can walk every ancestor. Abs fails only
	// without a working directory; the relative path then still
	// covers arguments at or below ".".
	if abs, err := filepath.Abs(parent); err == nil {
		parent = abs
	}
	target := filepath.Join(parent, filepath.Base(output))
	for _, arg := range in.args {
		if argWouldTake(arg, target) {
			return true
		}
	}
	if len(in.patterns) == 0 {
		return false
	}
	rel, ok := relUnder(target, ".")
	return ok && matchesFolded(in.patterns, rel)
}

// argWouldTake reports whether the explicit argument arg would take
// the Markdown file target. A directory argument is walked for every
// Markdown file below it. A glob argument takes the files it matches,
// and every Markdown file below a directory it matches. A symlinked
// argument is skipped, as the resolver skips it; a plain file argument
// already exists and was compared by identity.
func argWouldTake(arg, target string) bool {
	if li, err := os.Lstat(arg); err == nil {
		if !li.IsDir() {
			return false
		}
		_, ok := relUnder(target, arg)
		return ok
	}
	base, pattern := doublestar.SplitPattern(filepath.ToSlash(filepath.Clean(arg)))
	rel, ok := relUnder(target, filepath.FromSlash(base))
	if !ok {
		return false
	}
	for candidate := rel; candidate != "."; candidate = path.Dir(candidate) {
		if matchesFolded([]string{pattern}, candidate) {
			return true
		}
	}
	return false
}

// relUnder returns target's path relative to the directory dir, in
// slash form, when target lies below dir. target must be absolute with
// no symlink in its directory part. Its ancestors are compared with
// dir by identity, so a symlinked, differently cased, or relative dir
// still matches.
func relUnder(target, dir string) (string, bool) {
	want, err := os.Stat(dir)
	if err != nil {
		return "", false
	}
	for d := filepath.Dir(target); ; d = filepath.Dir(d) {
		if fi, err := os.Stat(d); err == nil && os.SameFile(fi, want) {
			// d is a lexical prefix of target, so Rel cannot fail.
			rel, _ := filepath.Rel(d, target)
			return filepath.ToSlash(rel), true
		}
		if filepath.Dir(d) == d {
			return "", false
		}
	}
}

// matchesFolded reports whether the slash path rel matches any of the
// doublestar patterns, ignoring case in both.
func matchesFolded(patterns []string, rel string) bool {
	rel = strings.ToLower(rel)
	for _, p := range patterns {
		if ok, err := doublestar.Match(strings.ToLower(p), rel); err == nil && ok {
			return true
		}
	}
	return false
}
