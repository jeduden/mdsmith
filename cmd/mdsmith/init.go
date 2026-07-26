package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/markdownlint"
	"github.com/jeduden/mdsmith/internal/pack"
	"github.com/jeduden/mdsmith/internal/starter"
	"github.com/jeduden/mdsmith/internal/yamlutil"
)

// setInitUsage installs the `mdsmith init` usage text on fs, writing to w.
// It also calls fs.SetOutput(w) so fs.PrintDefaults() flows to the same writer.
func setInitUsage(fs *flag.FlagSet, w io.Writer) {
	fs.SetOutput(w)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(w,
			"Usage: mdsmith init [--starter <name>] [--from-markdownlint[=path]]"+
				" [--apm] [--add <pack>] [--force] [--list]\n\n"+
				"Write .mdsmith.yml in the current directory and, optionally, scaffold\n"+
				"additive .mdsmith/ packs beside it.\n\n"+
				"Config source (pick at most one; the built-in defaults if omitted):\n"+
				"  --starter <name>      a ready-to-edit workflow config (available: %s)\n"+
				"  --from-markdownlint   convert a markdownlint config (.markdownlint.jsonc/\n"+
				"                        .json/.yaml/.yml or .markdownlintrc; =path names one)\n\n"+
				"An existing .mdsmith.yml is left unchanged unless --force is given.\n\n"+
				"APM coexistence:\n"+
				"  --apm                 scaffold the APM kind pack and write the\n"+
				"                        coexistence ignore: posture to .mdsmith.yml\n\n"+
				"Additive packs (repeatable; never overwrite existing files):\n"+
				"  --add <pack>          scaffold a curated .mdsmith/ bundle (available: %s)\n\n"+
				"Run `mdsmith init --list` to print every starter and pack.\n\nFlags:\n",
			strings.Join(starter.Names(), ", "), strings.Join(pack.Names(), ", "))
		fs.PrintDefaults()
	}
}

// runInit implements the "init" subcommand. It writes .mdsmith.yml from
// one config source — the built-in defaults, a --starter scaffold, or a
// --from-markdownlint conversion — and then applies any additive --add
// packs of .mdsmith/ sidecar files. The two axes are independent: an
// existing config is left unchanged (unless --force) while packs still
// apply, so init composes and stays idempotent on a second run.
// --apm additionally scaffolds the APM kind pack and writes the
// coexistence ignore: posture to .mdsmith.yml (or prints a merge hint
// when the config already exists).
func runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	var fromMarkdownlint string
	fs.StringVar(&fromMarkdownlint, "from-markdownlint", "",
		"Convert a markdownlint config instead of writing defaults (optionally --from-markdownlint=<path>)")
	fs.Lookup("from-markdownlint").NoOptDefVal = "auto"
	var starterName string
	fs.StringVar(&starterName, "starter", "",
		"Scaffold a workflow config instead of the defaults (e.g. --starter okf)")
	var addPacks []string
	fs.StringSliceVar(&addPacks, "add", nil,
		"Scaffold an additive .mdsmith/ pack (repeatable, e.g. --add wordlists)")
	var force bool
	fs.BoolVar(&force, "force", false,
		"Overwrite an existing .mdsmith.yml instead of leaving it unchanged")
	var list bool
	fs.BoolVar(&list, "list", false,
		"List the available starters and packs, then exit")
	var apmFlag bool
	fs.BoolVar(&apmFlag, "apm", false,
		"Scaffold the APM kind pack and write the coexistence ignore: posture")
	setInitUsage(fs, os.Stderr)

	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith: init"); code >= 0 {
			return code
		}
	}

	if list {
		printInitCatalog(os.Stdout)
		return 0
	}

	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr,
			"mdsmith: init takes no arguments; use flags like --starter, --from-markdownlint, or --add\n")
		return 2
	}

	// --apm implicitly scaffolds the kind pack via the "apm" pack entry.
	if apmFlag {
		addPacks = append(addPacks, "apm")
	}
	// Normalize comma-joined values: pflag keeps surrounding spaces, so
	// trim and drop empties before validating or applying.
	addPacks = normalizePackNames(addPacks)
	// Validate all pack names before touching the filesystem.
	for _, name := range addPacks {
		if _, ok := pack.Get(name); !ok {
			fmt.Fprintf(os.Stderr, "mdsmith: %v\n", pack.ErrUnknown(name))
			return 2
		}
	}
	if err := runInitConfig(".mdsmith.yml", fromMarkdownlint, starterName, force, apmFlag, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	if err := applyPacks(addPacks, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	return 0
}

// runInitConfig writes .mdsmith.yml from the chosen source, then handles
// the APM posture when apmFlag is set: it checks whether the file existed
// before writing so it can append the ignore: block (fresh) or print a
// merge hint (existing). Progress and error output go to w.
func runInitConfig(configFile, fromMarkdownlint, starterName string, force, apmFlag bool, w io.Writer) error {
	configExisted, err := statTarget(configFile)
	if err != nil {
		return err
	}
	if err := writeInitConfig(configFile, fromMarkdownlint, starterName, force, w); err != nil {
		return err
	}
	if apmFlag {
		return applyAPMPosture(configFile, configExisted, force, w)
	}
	return nil
}

// applyAPMPosture writes or prints the APM coexistence posture. When the
// config already existed and was not forced, it prints a merge hint to w.
// Otherwise it appends the ignore: block to configFile.
func applyAPMPosture(configFile string, configExisted, force bool, w io.Writer) error {
	globs := apmIgnoreGlobs()
	if configExisted && !force {
		printAPMMergeHint(w, globs)
		return nil
	}
	return appendAPMPosture(configFile, globs)
}

// apmIgnoreGlobs returns the ignore glob patterns for APM-deployed files,
// scoped to the harness directories actually present in the current directory.
// The APM module cache and compiled root files are always included; per-harness
// directories are added only when the harness directory exists on disk.
func apmIgnoreGlobs() []string {
	out := make([]string, 0, 12)
	// APM module cache and compiled root files are always included.
	// Compiled roots are ignored until plan 2607082049
	// (foreign-managed-regions) provides inline protection.
	out = append(out, "apm_modules/**", "AGENTS.md", "CLAUDE.md", "GEMINI.md")
	// Harness-scoped entries: add each glob set only when the harness
	// directory is present, replicating the detection `apm targets` uses.
	type harness struct {
		dir   string
		globs []string
	}
	for _, h := range []harness{
		{".github", []string{
			".github/prompts/**",
			".github/instructions/**",
			".github/copilot-instructions.md",
		}},
		{".claude", []string{".claude/rules/**"}},
		{".agents", []string{".agents/skills/**"}},
		{".windsurf", []string{".windsurf/rules/**"}},
		{".kiro", []string{".kiro/steering/**"}},
		{".cursor", []string{".cursor/rules/**"}},
	} {
		if _, err := os.Stat(h.dir); err == nil {
			out = append(out, h.globs...)
		}
	}
	return out
}

// apmPostureBlock formats the APM coexistence posture as a YAML block
// with a comment header, ready to append to .mdsmith.yml.
func apmPostureBlock(ignoreGlobs []string) []byte {
	var b strings.Builder
	b.WriteString("\n# APM coexistence posture — written by mdsmith init --apm\n")
	b.WriteString("# Keeps mdsmith fix off APM-deployed and APM-compiled files.\n")
	b.WriteString("# Remove entries for harness directories your repo does not use.\n")
	b.WriteString("ignore:\n")
	for _, g := range ignoreGlobs {
		fmt.Fprintf(&b, "  - %s\n", g)
	}
	return []byte(b.String())
}

// appendAPMPosture writes the APM coexistence posture into configFile.
// The default-config output always emits "ignore: []" for an empty ignore
// list; blindly appending a second "ignore:" key would produce a YAML file
// with duplicate keys that every YAML parser rejects. To handle this, the
// function reads the file, replaces the empty "ignore: []" line in-place
// with the APM block, and rewrites. If no such line is present (e.g. a
// --starter or --from-markdownlint output), it falls back to appending the
// full posture block at the end of the file.
func appendAPMPosture(configFile string, ignoreGlobs []string) error {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("appending APM posture to %s: %w", configFile, err)
	}
	posture := apmPostureBlock(ignoreGlobs)
	// Replace the empty "ignore: []" line emitted by the default marshaler so
	// we never produce two "ignore:" keys in the same YAML document.
	const emptyIgnore = "\nignore: []\n"
	if updated := strings.Replace(string(data), emptyIgnore, string(posture), 1); updated != string(data) {
		return os.WriteFile(configFile, []byte(updated), 0o644)
	}
	// No empty ignore list found (starter or converted config); append.
	f, err := os.OpenFile(configFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("appending APM posture to %s: %w", configFile, err)
	}
	defer f.Close() //nolint:errcheck // best-effort close on defer
	if _, err := f.Write(posture); err != nil {
		return fmt.Errorf("appending APM posture to %s: %w", configFile, err)
	}
	return nil
}

// printAPMMergeHint writes the APM posture block to w with a preamble
// instructing the user to merge it by hand into their existing .mdsmith.yml.
// This is called when the config already exists and was not overwritten.
func printAPMMergeHint(w io.Writer, ignoreGlobs []string) {
	_, _ = fmt.Fprintln(w, "mdsmith: --apm: .mdsmith.yml already exists; merge the posture block by hand:")
	_, _ = w.Write(apmPostureBlock(ignoreGlobs))
}

// printInitCatalog lists every config starter and additive pack init can
// produce, the human-facing answer to `mdsmith init --list`.
func printInitCatalog(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Starters (mdsmith init --starter <name>):")
	for _, name := range starter.Names() {
		_, _ = fmt.Fprintf(w, "  %-12s %s\n", name, starter.Describe(name))
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Packs (mdsmith init --add <name>):")
	for _, p := range pack.All() {
		_, _ = fmt.Fprintf(w, "  %-12s %s\n", p.Name, p.Summary)
	}
}

// writeInitConfig writes .mdsmith.yml from the selected config source —
// the defaults, a --starter scaffold, or a --from-markdownlint
// conversion. An existing config is left unchanged and noted unless
// force is set, so init stays idempotent and never silently clobbers a
// project's config. Progress and any conversion notes go to w.
func writeInitConfig(configFile, fromMarkdownlint, starterName string, force bool, w io.Writer) error {
	exists, err := statTarget(configFile)
	if err != nil {
		return err
	}
	if exists && !force {
		_, _ = fmt.Fprintf(w,
			"mdsmith: %s already exists, leaving it unchanged (use --force to overwrite)\n", configFile)
		return nil
	}
	data, source, err := initConfigBytes(fromMarkdownlint, starterName, w)
	if err != nil {
		return err
	}
	if err := os.WriteFile(configFile, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", configFile, err)
	}
	if source != "" {
		_, _ = fmt.Fprintf(w, "mdsmith: created %s from %s\n", configFile, source)
	} else {
		_, _ = fmt.Fprintf(w, "mdsmith: created %s\n", configFile)
	}
	return nil
}

// normalizePackNames trims surrounding whitespace from each --add value
// and drops the empties, so a comma-joined value pflag splits into
// ["wordlists", " stopwords"] — or a stray trailing comma — resolves to
// clean pack names.
func normalizePackNames(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// applyPacks scaffolds each named pack's sidecar files under the current
// directory. Packs are additive and non-clobbering, so applying them
// over an already-configured project — or re-running init — never
// overwrites a file the project already has. Names are validated by the
// caller; the lookup here is defensive. Progress lines go to w.
func applyPacks(names []string, w io.Writer) error {
	for _, name := range names {
		p, ok := pack.Get(name)
		if !ok {
			return pack.ErrUnknown(name)
		}
		if err := writeScaffolds(p.Files(), w); err != nil {
			return err
		}
	}
	return nil
}

// writeScaffolds writes each sidecar file under the current directory,
// creating parent directories as needed. An existing target is left
// untouched and noted, so a re-run never clobbers a project's edits.
// Existence is checked with statTarget, which refuses a symlinked target
// rather than following it — a planted link must not divert the write
// outside the pack directory. Each path is validated against the pack
// contract (relative, under .mdsmith/) and every existing parent
// component is checked for a symlink first, so a buggy or hostile pack
// cannot write out of tree. Progress lines go to w.
func writeScaffolds(files []pack.File, w io.Writer) error {
	for _, f := range files {
		if err := validatePackPath(f.Path); err != nil {
			return err
		}
		if err := refuseSymlinkedParents(f.Path); err != nil {
			return err
		}
		if dir := filepath.Dir(f.Path); dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("creating %s: %w", dir, err)
			}
		}
		exists, err := statTarget(f.Path)
		if err != nil {
			return err
		}
		if exists {
			_, _ = fmt.Fprintf(w, "mdsmith: %s already exists, skipping\n", f.Path)
			continue
		}
		if err := os.WriteFile(f.Path, f.Data, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", f.Path, err)
		}
		_, _ = fmt.Fprintf(w, "mdsmith: created %s\n", f.Path)
	}
	return nil
}

// refuseSymlinkedParents fails if any existing directory component of p —
// from the top-level .mdsmith down to its immediate parent — is a
// symlink. A symlinked component (.mdsmith itself, or an intermediate
// like .mdsmith/wordlists -> /tmp/out) would let the following MkdirAll
// and WriteFile resolve p to a location outside .mdsmith/, so pack writes
// must refuse it before touching the filesystem. Components that do not
// exist yet are fine: MkdirAll creates them as real directories, and once
// a component is absent everything below it is absent too.
func refuseSymlinkedParents(p string) error {
	dir := filepath.Dir(p)
	if dir == "." {
		return nil
	}
	components := strings.Split(dir, string(filepath.Separator))
	for i := range components {
		prefix := filepath.Join(components[:i+1]...)
		info, err := os.Lstat(prefix)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return nil
		case err != nil:
			return fmt.Errorf("checking %s: %w", prefix, err)
		case info.Mode()&fs.ModeSymlink != 0:
			return fmt.Errorf("%s is a symlink; refusing to write pack files through it", prefix)
		}
	}
	return nil
}

// mdsmithDir is the workspace subdirectory every additive pack writes
// under; validatePackPath keeps pack files confined to it.
const mdsmithDir = ".mdsmith"

// validatePackPath enforces the pack contract at the write boundary: a
// pack file must be a relative path under .mdsmith/. A pack that returns
// an absolute path, or one that escapes the workspace with "..", is
// rejected before any directory is created or file written, so init's
// writes cannot be diverted out of tree. A "../" that stays within
// .mdsmith after cleaning is harmless and allowed; any escape leaves a
// first component other than .mdsmith and is refused.
func validatePackPath(p string) error {
	clean := filepath.Clean(p)
	if filepath.IsAbs(clean) {
		return fmt.Errorf("pack file path %q must be relative", p)
	}
	first, _, _ := strings.Cut(clean, string(filepath.Separator))
	if first != mdsmithDir {
		return fmt.Errorf("pack file path %q must be under %s/", p, mdsmithDir)
	}
	return nil
}

// statTarget reports whether path already exists as a regular file — the
// only kind of entry init treats as "already there, leave it alone". It
// uses Lstat so a symlink is never followed: a symlink, even a dangling
// one, is refused with an error rather than written through, which would
// let a planted link divert an init write outside the working directory.
// A missing path returns (false, nil). A non-regular, non-symlink entry
// such as a directory also returns (false, nil), so the caller's write
// runs and surfaces a clear "is a directory" failure rather than being
// silently skipped. Any other lstat failure becomes a "checking" error.
func statTarget(path string) (exists bool, err error) {
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("checking %s: %w", path, err)
	case info.Mode()&fs.ModeSymlink != 0:
		return false, fmt.Errorf("%s is a symlink; refusing to write through it", path)
	default:
		return info.Mode().IsRegular(), nil
	}
}

// initConfigBytes produces the .mdsmith.yml contents for init. With a
// starterName it returns that starter's embedded template; with a
// fromMarkdownlint path ("auto" = discover in the current directory) it
// converts that config, echoing notes to w; otherwise it dumps the full
// defaults. source reports the provenance for the confirmation message.
// --starter and --from-markdownlint are mutually exclusive.
func initConfigBytes(fromMarkdownlint, starterName string, w io.Writer) (data []byte, source string, err error) {
	switch {
	case starterName != "" && fromMarkdownlint != "":
		return nil, "", errors.New("--starter and --from-markdownlint are mutually exclusive")
	case starterName != "":
		b, ok := starter.Get(starterName)
		if !ok {
			return nil, "", starter.ErrUnknown(starterName)
		}
		return b, "the " + starterName + " starter", nil
	case fromMarkdownlint != "":
		return convertedConfigBytes(fromMarkdownlint, w)
	default:
		data, err = defaultConfigBytes()
		return data, "", err
	}
}

// defaultConfigBytes marshals the built-in defaults, the plain
// `mdsmith init` output.
func defaultConfigBytes() ([]byte, error) {
	cfg := config.DumpDefaults()

	// Set front-matter: true as default.
	fm := true
	cfg.FrontMatter = &fm

	return yamlutil.Marshal(cfg)
}

// convertedConfigBytes resolves the markdownlint config path ("auto" =
// discover in the current directory), converts it via
// markdownlint.ConvertFile, and echoes the conversion notes to w.
func convertedConfigBytes(path string, w io.Writer) (data []byte, source string, err error) {
	source = path
	if path == "auto" {
		source, err = markdownlint.Discover(".")
		if err != nil {
			return nil, "", err
		}
	}
	data, notes, err := markdownlint.ConvertFile(source)
	if err != nil {
		return nil, "", err
	}
	for _, note := range notes {
		_, _ = fmt.Fprintf(w, "mdsmith: note: %s\n", note)
	}
	return data, source, nil
}
