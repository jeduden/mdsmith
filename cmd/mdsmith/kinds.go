package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/kindsout"
	"github.com/jeduden/mdsmith/internal/lint"
)

const kindsUsage = `Usage: mdsmith kinds <subcommand> [args]

Subcommands:
  list                  Print declared kinds with their merged bodies.
  show <name>           Print one kind's merged body.
  path <name>           Print the resolved schema path of the kind's
                        required-structure rule, if any.
  resolve <file>        Print the resolved kind list and merged rule
                        config for a file, with per-leaf provenance.
  why <file> <rule>     Print the full merge chain for one rule on
                        one file, including no-op layers.

Each subcommand accepts --json for stable structured output.
`

// runKinds dispatches the kinds subcommand.
func runKinds(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, kindsUsage)
		return 0
	}
	switch args[0] {
	case "--help", "-h":
		fmt.Fprint(os.Stderr, kindsUsage)
		return 0
	case "list":
		return runKindsList(os.Stdout, args[1:])
	case "show":
		return runKindsShow(os.Stdout, args[1:])
	case "path":
		return runKindsPath(os.Stdout, args[1:])
	case "resolve":
		return runKindsResolve(os.Stdout, args[1:])
	case "why":
		return runKindsWhy(os.Stdout, args[1:])
	default:
		fmt.Fprintf(os.Stderr,
			"mdsmith: kinds: unknown subcommand %q\n\n%s",
			args[0], kindsUsage)
		return 2
	}
}

// kindsConfig loads the merged config and returns it. Errors are
// printed to stderr and a non-zero exit code is returned.
func kindsConfig() (*config.Config, string, int) {
	cfg, cfgPath, err := loadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return nil, "", 2
	}
	return cfg, cfgPath, 0
}

func sortedKindNames(cfg *config.Config) []string {
	names := make([]string, 0, len(cfg.Kinds))
	for name := range cfg.Kinds {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// runKindsList prints declared kinds with their merged bodies.
func runKindsList(stdout io.Writer, args []string) int {
	fs := flag.NewFlagSet("kinds list", flag.ContinueOnError)
	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, "Emit JSON output")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: mdsmith kinds list [--json]")
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "mdsmith: kinds list takes no positional arguments")
		return 2
	}

	cfg, _, code := kindsConfig()
	if code != 0 {
		return code
	}

	names := sortedKindNames(cfg)

	if asJSON {
		out := struct {
			Kinds []kindsout.BodyJSON `json:"kinds"`
		}{Kinds: make([]kindsout.BodyJSON, 0, len(names))}
		for _, name := range names {
			out.Kinds = append(out.Kinds, kindsout.MakeBodyJSON(name, cfg.Kinds[name]))
		}
		return writeJSON(stdout, out)
	}

	if len(names) == 0 {
		fmt.Fprintln(os.Stderr, "mdsmith: no kinds declared in config")
		return 0
	}
	for i, name := range names {
		if i > 0 {
			if _, err := fmt.Fprintln(stdout); err != nil {
				return printErr(err)
			}
		}
		if err := kindsout.WriteBodyText(stdout, name, cfg.Kinds[name]); err != nil {
			return printErr(err)
		}
	}
	return 0
}

// runKindsShow prints one kind's merged body.
func runKindsShow(stdout io.Writer, args []string) int {
	fs := flag.NewFlagSet("kinds show", flag.ContinueOnError)
	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, "Emit JSON output")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: mdsmith kinds show <name> [--json]")
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "mdsmith: kinds show requires exactly one kind name")
		return 2
	}
	name := fs.Arg(0)

	cfg, _, code := kindsConfig()
	if code != 0 {
		return code
	}

	body, ok := cfg.Kinds[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "mdsmith: unknown kind %q\n", name)
		return 2
	}

	if asJSON {
		return writeJSON(stdout, kindsout.MakeBodyJSON(name, body))
	}

	if err := kindsout.WriteBodyText(stdout, name, body); err != nil {
		return printErr(err)
	}
	return 0
}

// kindSchemaPath extracts a kind's required-structure.schema setting,
// returning a clear stderr error and a non-zero exit code on every
// way the kind can fail to resolve to a schema string.
func kindSchemaPath(body config.KindBody, name string) (string, int) {
	rs, ok := body.Rules["required-structure"]
	if !ok || !rs.Enabled {
		fmt.Fprintf(os.Stderr,
			"mdsmith: kind %q does not configure required-structure\n", name)
		return "", 2
	}
	rawSchema, hasSchema := rs.Settings["schema"]
	if !hasSchema {
		fmt.Fprintf(os.Stderr,
			"mdsmith: kind %q has no required-structure.schema set\n", name)
		return "", 2
	}
	schema, ok := rawSchema.(string)
	if !ok {
		fmt.Fprintf(os.Stderr,
			"mdsmith: kind %q required-structure.schema must be a string, got %T (%v)\n",
			name, rawSchema, rawSchema)
		return "", 2
	}
	if schema == "" {
		fmt.Fprintf(os.Stderr,
			"mdsmith: kind %q has no required-structure.schema set\n", name)
		return "", 2
	}
	return schema, 0
}

// runKindsPath prints the resolved schema path of the kind's
// required-structure rule. Exits 2 when the kind is unknown or the
// kind does not configure a schema.
func runKindsPath(stdout io.Writer, args []string) int {
	fs := flag.NewFlagSet("kinds path", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: mdsmith kinds path <name>")
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "mdsmith: kinds path requires exactly one kind name")
		return 2
	}
	name := fs.Arg(0)

	cfg, cfgPath, code := kindsConfig()
	if code != 0 {
		return code
	}

	body, ok := cfg.Kinds[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "mdsmith: unknown kind %q\n", name)
		return 2
	}

	schema, code := kindSchemaPath(body, name)
	if code != 0 {
		return code
	}
	resolved := schema
	if !filepath.IsAbs(schema) {
		root := rootDirFromConfig(cfgPath)
		if root != "" {
			resolved = filepath.Join(root, schema)
		}
	}
	if _, err := fmt.Fprintln(stdout, resolved); err != nil {
		return printErr(err)
	}
	return 0
}

// readFrontMatterKinds reads a file and parses its front-matter kinds: list.
// Returns nil kinds when the file has no front matter or no kinds: field.
func readFrontMatterKinds(path string, maxBytes int64) ([]string, error) {
	data, err := lint.ReadFileLimited(path, maxBytes)
	if err != nil {
		return nil, err
	}
	prefix, _ := lint.StripFrontMatter(data)
	return lint.ParseFrontMatterKinds(prefix)
}

// resolveFileFromCLI loads config, parses the file's front matter for
// kinds: and returns a FileResolution. Errors are printed to stderr.
//
// When the config disables front matter (`front-matter: false`), front
// matter is neither parsed nor validated so the resolution mirrors the
// engine's behavior. The file is still opened and read (and checked
// against max-input-size) to surface readability errors and to match
// the engine's rejection of oversized or unreadable paths.
func resolveFileFromCLI(path string) (*config.FileResolution, *config.Config, int) {
	cfg, _, code := kindsConfig()
	if code != 0 {
		return nil, nil, code
	}

	var fmKinds []string
	if frontMatterEnabled(cfg) {
		maxBytes, err := resolveMaxInputBytes(cfg, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
			return nil, nil, 2
		}
		fmKinds, err = readFrontMatterKinds(path, maxBytes)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mdsmith: reading %s: %v\n", path, err)
			return nil, nil, 2
		}
		if err := config.ValidateFrontMatterKinds(cfg, path, fmKinds); err != nil {
			fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
			return nil, nil, 2
		}
	} else {
		// front-matter disabled: no kinds from front matter, but still
		// attempt an open/read to mirror the engine's readability and
		// max-input-size checks (os.Stat passes on directories and
		// unreadable paths).
		maxBytes, err := resolveMaxInputBytes(cfg, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
			return nil, nil, 2
		}
		if _, err := lint.ReadFileLimited(path, maxBytes); err != nil {
			fmt.Fprintf(os.Stderr, "mdsmith: reading %s: %v\n", path, err)
			return nil, nil, 2
		}
	}

	return config.ResolveFile(cfg, path, fmKinds), cfg, 0
}

// runKindsResolve prints the resolved kind list and merged rule config
// for a single file, with per-leaf provenance.
func runKindsResolve(stdout io.Writer, args []string) int {
	fs := flag.NewFlagSet("kinds resolve", flag.ContinueOnError)
	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, "Emit JSON output")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: mdsmith kinds resolve <file> [--json]")
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "mdsmith: kinds resolve requires exactly one file argument")
		return 2
	}
	path := fs.Arg(0)

	res, _, code := resolveFileFromCLI(path)
	if code != 0 {
		return code
	}

	if asJSON {
		return writeJSON(stdout, kindsout.FileResolution(res))
	}
	if err := kindsout.WriteFileResolutionText(stdout, res); err != nil {
		return printErr(err)
	}
	return 0
}

// runKindsWhy prints the full merge chain for one rule on one file.
func runKindsWhy(stdout io.Writer, args []string) int {
	fs := flag.NewFlagSet("kinds why", flag.ContinueOnError)
	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, "Emit JSON output")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: mdsmith kinds why <file> <rule> [--json]")
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "mdsmith: kinds why requires <file> and <rule>")
		return 2
	}
	path, ruleName := fs.Arg(0), fs.Arg(1)

	res, _, code := resolveFileFromCLI(path)
	if code != 0 {
		return code
	}

	rr, ok := res.Rules[ruleName]
	if !ok {
		fmt.Fprintf(os.Stderr, "mdsmith: rule %q not found in effective config for %s\n",
			ruleName, path)
		return 2
	}

	if asJSON {
		return writeJSON(stdout, kindsout.RuleResolution(res.File, rr))
	}
	if err := kindsout.WriteRuleResolutionText(stdout, res.File, rr); err != nil {
		return printErr(err)
	}
	return 0
}

// writeJSON emits v as pretty-printed JSON. Returns a non-zero exit
// code on encoding error.
func writeJSON(w io.Writer, v any) int {
	if err := kindsout.WriteJSON(w, v); err != nil {
		return printErr(err)
	}
	return 0
}

func printErr(err error) int {
	fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
	return 2
}
