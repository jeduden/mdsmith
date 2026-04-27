package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	flag "github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	"github.com/jeduden/mdsmith/internal/config"
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
		return runKindsList(args[1:])
	case "show":
		return runKindsShow(args[1:])
	case "path":
		return runKindsPath(args[1:])
	case "resolve":
		return runKindsResolve(args[1:])
	case "why":
		return runKindsWhy(args[1:])
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

// kindBodyForJSON renders a KindBody as a JSON-friendly value with
// rules and categories separated, using the rule config's marshal
// form (settings map or bool).
type kindBodyJSON struct {
	Name       string                 `json:"name"`
	Rules      map[string]ruleCfgJSON `json:"rules"`
	Categories map[string]bool        `json:"categories,omitempty"`
}

// ruleCfgJSON serializes a RuleCfg to JSON. A disabled rule is the
// boolean false; an enabled rule with settings is its settings map;
// an enabled rule without settings is the boolean true.
type ruleCfgJSON struct {
	v any
}

// MarshalJSON implements json.Marshaler for ruleCfgJSON.
func (r ruleCfgJSON) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.v)
}

func makeKindBodyJSON(name string, body config.KindBody) kindBodyJSON {
	rules := make(map[string]ruleCfgJSON, len(body.Rules))
	for k, v := range body.Rules {
		rules[k] = ruleCfgJSON{v: ruleCfgValue(v)}
	}
	return kindBodyJSON{
		Name:       name,
		Rules:      rules,
		Categories: body.Categories,
	}
}

// ruleCfgValue returns the JSON-friendly value of a RuleCfg, matching
// its YAML marshalling: false, true, or the settings map.
func ruleCfgValue(rc config.RuleCfg) any {
	if !rc.Enabled && rc.Settings == nil {
		return false
	}
	if rc.Enabled && len(rc.Settings) > 0 {
		return rc.Settings
	}
	return true
}

// runKindsList prints declared kinds with their merged bodies.
func runKindsList(args []string) int {
	fs := flag.NewFlagSet("kinds list", flag.ContinueOnError)
	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, "Emit JSON output")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: mdsmith kinds list [--json]")
	}
	if err := fs.Parse(args); err != nil {
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
			Kinds []kindBodyJSON `json:"kinds"`
		}{Kinds: make([]kindBodyJSON, 0, len(names))}
		for _, name := range names {
			out.Kinds = append(out.Kinds, makeKindBodyJSON(name, cfg.Kinds[name]))
		}
		return writeJSON(os.Stdout, out)
	}

	if len(names) == 0 {
		fmt.Fprintln(os.Stderr, "mdsmith: no kinds declared in config")
		return 0
	}
	for i, name := range names {
		if i > 0 {
			if _, err := fmt.Fprintln(os.Stdout); err != nil {
				return printErr(err)
			}
		}
		if err := writeKindBodyText(os.Stdout, name, cfg.Kinds[name]); err != nil {
			return printErr(err)
		}
	}
	return 0
}

// writeKindBodyText renders a kind body as YAML with a header line
// naming the kind. Useful for both list and show.
func writeKindBodyText(w *os.File, name string, body config.KindBody) error {
	if _, err := fmt.Fprintf(w, "%s:\n", name); err != nil {
		return err
	}
	wrap := struct {
		Rules      map[string]config.RuleCfg `yaml:"rules,omitempty"`
		Categories map[string]bool           `yaml:"categories,omitempty"`
	}{
		Rules:      body.Rules,
		Categories: body.Categories,
	}
	data, err := yaml.Marshal(wrap)
	if err != nil {
		return err
	}
	if len(data) == 0 || strings.TrimSpace(string(data)) == "{}" {
		_, err := fmt.Fprintln(w, "  (empty)")
		return err
	}
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if _, err := fmt.Fprintf(w, "  %s\n", line); err != nil {
			return err
		}
	}
	return nil
}

// runKindsShow prints one kind's merged body.
func runKindsShow(args []string) int {
	fs := flag.NewFlagSet("kinds show", flag.ContinueOnError)
	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, "Emit JSON output")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: mdsmith kinds show <name> [--json]")
	}
	if err := fs.Parse(args); err != nil {
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
		return writeJSON(os.Stdout, makeKindBodyJSON(name, body))
	}

	if err := writeKindBodyText(os.Stdout, name, body); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	return 0
}

// runKindsPath prints the resolved schema path of the kind's
// required-structure rule. Exits 2 when the kind is unknown or the
// kind does not configure a schema.
func runKindsPath(args []string) int {
	fs := flag.NewFlagSet("kinds path", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: mdsmith kinds path <name>")
	}
	if err := fs.Parse(args); err != nil {
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

	rs, ok := body.Rules["required-structure"]
	if !ok || !rs.Enabled {
		fmt.Fprintf(os.Stderr,
			"mdsmith: kind %q does not configure required-structure\n", name)
		return 2
	}
	schema, _ := rs.Settings["schema"].(string)
	if schema == "" {
		fmt.Fprintf(os.Stderr,
			"mdsmith: kind %q has no required-structure.schema set\n", name)
		return 2
	}
	resolved := schema
	if !filepath.IsAbs(schema) {
		root := rootDirFromConfig(cfgPath)
		if root != "" {
			resolved = filepath.Join(root, schema)
		}
	}
	if _, err := fmt.Fprintln(os.Stdout, resolved); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
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
func resolveFileFromCLI(path string) (*config.FileResolution, *config.Config, int) {
	cfg, _, code := kindsConfig()
	if code != 0 {
		return nil, nil, code
	}
	maxBytes, err := resolveMaxInputBytes(cfg, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return nil, nil, 2
	}

	fmKinds, err := readFrontMatterKinds(path, maxBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: reading %s: %v\n", path, err)
		return nil, nil, 2
	}
	if err := config.ValidateFrontMatterKinds(cfg, path, fmKinds); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return nil, nil, 2
	}

	return config.ResolveFile(cfg, path, fmKinds), cfg, 0
}

// runKindsResolve prints the resolved kind list and merged rule config
// for a single file, with per-leaf provenance.
func runKindsResolve(args []string) int {
	fs := flag.NewFlagSet("kinds resolve", flag.ContinueOnError)
	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, "Emit JSON output")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: mdsmith kinds resolve <file> [--json]")
	}
	if err := fs.Parse(args); err != nil {
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
		return writeJSON(os.Stdout, fileResolutionJSON(res))
	}
	return writeFileResolutionText(os.Stdout, res)
}

// runKindsWhy prints the full merge chain for one rule on one file.
func runKindsWhy(args []string) int {
	fs := flag.NewFlagSet("kinds why", flag.ContinueOnError)
	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, "Emit JSON output")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: mdsmith kinds why <file> <rule> [--json]")
	}
	if err := fs.Parse(args); err != nil {
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
		return writeJSON(os.Stdout, ruleResolutionJSON(res.File, rr))
	}
	return writeRuleResolutionText(os.Stdout, res.File, rr)
}

// writeJSON emits v as pretty-printed JSON. Returns a non-zero exit
// code on encoding error.
func writeJSON(w *os.File, v any) int {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	return 0
}

// --- JSON shape for resolve / why ---

type resolvedKindJSON struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

type leafJSON struct {
	Path   string          `json:"path"`
	Value  any             `json:"value"`
	Source string          `json:"source"`
	Chain  []leafChainJSON `json:"chain,omitempty"`
}

type leafChainJSON struct {
	Source string `json:"source"`
	Value  any    `json:"value"`
}

type layerJSON struct {
	Source string `json:"source"`
	Set    bool   `json:"set"`
	Value  any    `json:"value,omitempty"`
}

type ruleResolutionJSONShape struct {
	File   string      `json:"file"`
	Rule   string      `json:"rule"`
	Final  any         `json:"final"`
	Layers []layerJSON `json:"layers"`
	Leaves []leafJSON  `json:"leaves"`
}

type fileResolutionJSONShape struct {
	File       string                 `json:"file"`
	Kinds      []resolvedKindJSON     `json:"kinds"`
	Categories map[string]bool        `json:"categories,omitempty"`
	Rules      map[string]ruleSummary `json:"rules"`
}

type ruleSummary struct {
	Final  any        `json:"final"`
	Leaves []leafJSON `json:"leaves"`
}

func fileResolutionJSON(res *config.FileResolution) fileResolutionJSONShape {
	out := fileResolutionJSONShape{
		File:       res.File,
		Kinds:      make([]resolvedKindJSON, 0, len(res.Kinds)),
		Categories: res.Categories,
		Rules:      make(map[string]ruleSummary, len(res.Rules)),
	}
	for _, k := range res.Kinds {
		out.Kinds = append(out.Kinds, resolvedKindJSON{
			Name: k.Name, Source: string(k.Source),
		})
	}
	for name, rr := range res.Rules {
		out.Rules[name] = ruleSummary{
			Final:  ruleCfgValue(rr.Final),
			Leaves: leavesJSON(rr.Leaves),
		}
	}
	return out
}

func ruleResolutionJSON(file string, rr config.RuleResolution) ruleResolutionJSONShape {
	layers := make([]layerJSON, 0, len(rr.Layers))
	for _, l := range rr.Layers {
		entry := layerJSON{Source: l.Source, Set: l.Set}
		if l.Set {
			entry.Value = ruleCfgValue(l.Value)
		}
		layers = append(layers, entry)
	}
	return ruleResolutionJSONShape{
		File:   file,
		Rule:   rr.Rule,
		Final:  ruleCfgValue(rr.Final),
		Layers: layers,
		Leaves: leavesJSON(rr.Leaves),
	}
}

func leavesJSON(leaves []config.Leaf) []leafJSON {
	out := make([]leafJSON, 0, len(leaves))
	for _, l := range leaves {
		entry := leafJSON{
			Path:   l.Path,
			Value:  l.Value,
			Source: l.Source(),
		}
		for _, c := range l.Chain {
			entry.Chain = append(entry.Chain, leafChainJSON{
				Source: c.Source, Value: c.Value,
			})
		}
		out = append(out, entry)
	}
	return out
}

// --- Text rendering for resolve / why ---

// fpf is a write-with-error-check helper that swallows the byte count
// from fmt.Fprintf so callers only need to track the error.
func fpf(w *os.File, format string, args ...any) error {
	_, err := fmt.Fprintf(w, format, args...)
	return err
}

func writeFileResolutionText(w *os.File, res *config.FileResolution) int {
	if err := fpf(w, "file: %s\n", res.File); err != nil {
		return printErr(err)
	}
	if err := fpf(w, "effective kinds:\n"); err != nil {
		return printErr(err)
	}
	if len(res.Kinds) == 0 {
		if err := fpf(w, "  (none)\n"); err != nil {
			return printErr(err)
		}
	} else {
		for _, k := range res.Kinds {
			if err := fpf(w, "  - %s (from %s)\n", k.Name, k.Source); err != nil {
				return printErr(err)
			}
		}
	}

	if err := fpf(w, "rules:\n"); err != nil {
		return printErr(err)
	}
	names := make([]string, 0, len(res.Rules))
	for name := range res.Rules {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		rr := res.Rules[name]
		if err := fpf(w, "  %s:\n", name); err != nil {
			return printErr(err)
		}
		for _, leaf := range rr.Leaves {
			if err := fpf(w, "    %s = %s  (from %s)\n",
				leaf.Path, formatValue(leaf.Value), leaf.Source()); err != nil {
				return printErr(err)
			}
		}
	}
	return 0
}

func writeRuleResolutionText(w *os.File, file string, rr config.RuleResolution) int {
	if err := fpf(w, "file: %s\nrule: %s\n\nmerge chain (oldest -> newest):\n",
		file, rr.Rule); err != nil {
		return printErr(err)
	}
	for _, l := range rr.Layers {
		var line string
		if l.Set {
			line = fmt.Sprintf("  %-30s set    %s\n",
				l.Source, formatValue(ruleCfgValue(l.Value)))
		} else {
			line = fmt.Sprintf("  %-30s no-op  (rule untouched)\n", l.Source)
		}
		if err := fpf(w, "%s", line); err != nil {
			return printErr(err)
		}
	}
	if err := fpf(w, "\nper-leaf provenance:\n"); err != nil {
		return printErr(err)
	}
	for _, leaf := range rr.Leaves {
		if err := fpf(w, "  %s = %s  (winning source: %s)\n",
			leaf.Path, formatValue(leaf.Value), leaf.Source()); err != nil {
			return printErr(err)
		}
		for _, c := range leaf.Chain {
			if err := fpf(w, "    %-28s %s\n", c.Source, formatValue(c.Value)); err != nil {
				return printErr(err)
			}
		}
	}
	return 0
}

// formatValue renders a leaf value compactly (JSON-like) so settings
// maps / lists / scalars all print on one line.
func formatValue(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

func printErr(err error) int {
	fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
	return 2
}
