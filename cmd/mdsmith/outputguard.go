package main

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/jeduden/mdsmith/internal/mdpath"
)

// runInputs describes the files a check or fix run reads, for the -o
// guard: the files it resolved, plus what decides which files a
// later run would pick up.
type runInputs struct {
	// files are the resolved input files.
	files []string
	// args are the explicit file, directory, and glob arguments.
	args []string
	// patterns are the config files: patterns of a discovery run,
	// matched relative to the working directory.
	patterns []string
	// stdin is the file `check -` reads, or nil on other runs.
	stdin *os.File
}

// guardOutput vets a check or fix run's -o path before any file is
// linted or fixed, and prints a usage error and returns 2 for a path
// the run must not write. It refuses a path that is, or once written
// would be, one of the run's inputs (see outputIsInput): the report
// would overwrite a file the run lints or fixes, or turn up as an
// input of the next run. It also refuses a path the report cannot be
// created at (see outputCreatable), so fix does not rewrite its files
// and only then fail to open the report. The file itself is neither
// created nor opened here, so a run that stops first leaves none. It
// returns -1 when the path is safe; "" and "-" name no file.
func guardOutput(cmd, output string, in runInputs) int {
	if output == "" || output == "-" {
		return -1
	}
	if outputIsInput(output, in) {
		fmt.Fprintf(os.Stderr,
			"mdsmith: %s: refusing --output %q: it is an input of this run, or would be once written\n",
			cmd, output)
		return 2
	}
	if err := outputCreatable(output); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %s: cannot write --output %q: %v\n", cmd, output, err)
		return 2
	}
	return -1
}

// outputCreatable returns nil when the report can be opened at output:
// an existing file or device, or a missing name in an existing
// directory, a dangling symlink's target included. Otherwise it says
// why not: output is a directory, or its directory is missing or is
// not one. Permissions are left to the open.
func outputCreatable(output string) error {
	if info, err := os.Stat(output); err == nil {
		if info.IsDir() {
			return errors.New("it is a directory")
		}
		return nil
	}
	target, err := createTarget(output)
	if err != nil {
		return err
	}
	dir := filepath.Dir(target)
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return nil
	}
	return fmt.Errorf("%s is not a directory", dir)
}

// outputIsInput reports whether output names one of the run's inputs.
// An existing output is compared with each resolved input, and with
// the file stdin reads on `check -`, by file identity (os.SameFile),
// so a relative spelling, a symlink, a hard link, or a
// case-insensitive file system cannot hide a match. A pipe on stdin
// has no identity to match, so `cat a.md | mdsmith check - -o a.md`
// goes undetected. A missing output, or a dangling symlink, is
// checked by wouldBeInput.
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
	if in.stdin != nil {
		if si, err := in.stdin.Stat(); err == nil && os.SameFile(info, si) {
			return true
		}
	}
	return false
}

// wouldBeInput reports whether a missing output, once the report
// creates it, is a file the same run would pick up: a Markdown name
// inside a directory argument or matching a glob argument, or, on a
// discovery run, any name matching a files: pattern under the working
// directory, since discovery takes whatever its patterns match. The
// name checked is the one the open creates (see
// createTarget), so a dangling symlink is judged by its final target,
// not its own name. Directories are compared by identity. Names are
// matched without regard to case, so the check also holds on a
// case-insensitive file system. Ignore rules are not consulted, so a
// Markdown output name in the linted tree is refused even where
// .gitignore would skip it. A path the open cannot create at all, as
// under a missing directory, is left to preflightOutput.
func wouldBeInput(output string, in runInputs) bool {
	target, err := createTarget(output)
	if err != nil {
		return false
	}
	// The resolver takes only Markdown files from the arguments.
	if mdpath.HasMarkdownExt(filepath.Ext(target)) {
		for _, arg := range in.args {
			if argWouldTake(arg, target) {
				return true
			}
		}
	}
	if len(in.patterns) == 0 {
		return false
	}
	rel, ok := relUnder(target, ".")
	return ok && matchesFolded(in.patterns, rel)
}

// maxLinkHops caps the symlinks createTarget follows, as the kernel
// caps a path lookup (40 on Linux).
const maxLinkHops = 40

// errTooManyLinks is createTarget's error for a symlink chain longer
// than maxLinkHops, such as a loop; the open fails on it too.
var errTooManyLinks = errors.New("too many levels of symbolic links")

// createTarget returns the absolute path, with no symlink in its
// directory part, of the file that opening the missing path output
// with O_CREATE creates. A dangling symlink at output is followed,
// hop by hop, to its final target, which is the file the open
// creates; a relative link target is read from the link's own
// directory. It fails when the open would fail too: on a missing
// directory, or on more than maxLinkHops links.
func createTarget(output string) (string, error) {
	p := output
	for hops := 0; ; hops++ {
		parent, err := filepath.EvalSymlinks(filepath.Dir(p))
		if err != nil {
			return "", err
		}
		// Absolute, so relUnder can walk every ancestor. Abs fails only
		// without a working directory; the relative path then still
		// covers arguments at or below ".".
		if abs, err := filepath.Abs(parent); err == nil {
			parent = abs
		}
		p = filepath.Join(parent, filepath.Base(p))
		dest, err := os.Readlink(p)
		if err != nil {
			// Not a symlink: the open creates p itself.
			return p, nil
		}
		if hops == maxLinkHops {
			return "", errTooManyLinks
		}
		if !filepath.IsAbs(dest) {
			dest = filepath.Join(parent, dest)
		}
		p = dest
	}
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
