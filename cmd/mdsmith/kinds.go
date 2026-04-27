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

const kindsUsage = `Usage: mdsmith kinds <subcommand> [flags] [args]

Subcommands:
  list [--json]                Print declared kinds with merged bodies.
  show <name> [--json]         Print one kind's merged body.
  path <name>                  Print the kind's required-structure schema path.
  resolve <file> [--json]      Resolve kinds + per-leaf rule provenance for a file.
  why <file> <rule> [--json]   Print the full merge chain for one rule on one file.

Kinds and kind-assignment come from .mdsmith.yml. See 'mdsmith help kinds'
for the concept page and 'mdsmith help kinds-cli' for a one-screen summary
of these subcommands.
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
		fmt.Fprintf(os.Stderr, "mdsmith: kinds: unknown subcommand %q\n\n%s",
			args[0], kindsUsage)
		return 2
	}
}

// parseKindsFlags peels --json (and --help) off args and returns the
// remaining positional args. On --help it returns code 0; on parse error
// it returns code 2.
func parseKindsFlags(verb string, args []string) (jsonOut bool, positional []string, code int) {
	fs := flag.NewFlagSet("kinds "+verb, flag.ContinueOnError)
	fs.BoolVar(&jsonOut, "json", false, "Emit structured JSON to stdout")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith kinds %s [--json] [args]\n", verb)
	}
	if err := fs.Parse(args); err != nil {
		return false, nil, 2
	}
	return jsonOut, fs.Args(), -1
}

// runKindsList prints declared kinds and their merged bodies.
func runKindsList(args []string) int {
	jsonOut, posArgs, code := parseKindsFlags("list", args)
	if code >= 0 {
		return code
	}
	if len(posArgs) > 0 {
		fmt.Fprintln(os.Stderr, "mdsmith: kinds list takes no positional arguments")
		return 2
	}
	cfg, _, err := loadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	if jsonOut {
		return emitKindsListJSON(cfg)
	}
	return emitKindsListText(cfg)
}

func sortedKindNames(cfg *config.Config) []string {
	names := make([]string, 0, len(cfg.Kinds))
	for n := range cfg.Kinds {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func emitKindsListText(cfg *config.Config) int {
	names := sortedKindNames(cfg)
	var sb strings.Builder
	for _, name := range names {
		body := cfg.Kinds[name]
		fmt.Fprintf(&sb, "%s\n", name)
		yml, err := yaml.Marshal(body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
			return 2
		}
		for _, line := range splitLines(string(yml)) {
			fmt.Fprintf(&sb, "  %s\n", line)
		}
	}
	return writeStdout(sb.String())
}

func emitKindsListJSON(cfg *config.Config) int {
	names := sortedKindNames(cfg)
	type kindOut struct {
		Name string          `json:"name"`
		Body config.KindBody `json:"body"`
	}
	out := struct {
		Kinds []kindOut `json:"kinds"`
	}{Kinds: make([]kindOut, 0, len(names))}
	for _, n := range names {
		out.Kinds = append(out.Kinds, kindOut{Name: n, Body: cfg.Kinds[n]})
	}
	return writeJSON(out)
}

// runKindsShow prints the merged body of one named kind.
func runKindsShow(args []string) int {
	jsonOut, posArgs, code := parseKindsFlags("show", args)
	if code >= 0 {
		return code
	}
	if len(posArgs) != 1 {
		fmt.Fprintln(os.Stderr, "mdsmith: kinds show requires exactly one kind name")
		return 2
	}
	name := posArgs[0]
	cfg, _, err := loadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	body, ok := cfg.Kinds[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "mdsmith: kinds show: unknown kind %q\n", name)
		return 2
	}
	if jsonOut {
		return writeJSON(struct {
			Name string          `json:"name"`
			Body config.KindBody `json:"body"`
		}{Name: name, Body: body})
	}
	yml, err := yaml.Marshal(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	if _, err := os.Stdout.Write(yml); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	return 0
}

// runKindsPath prints the filesystem path of a kind's required-structure
// schema, if any. Exits 2 when the kind is unknown or carries no schema.
func runKindsPath(args []string) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(os.Stderr, "Usage: mdsmith kinds path <name>")
		return 0
	}
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "mdsmith: kinds path requires exactly one kind name")
		return 2
	}
	name := args[0]
	cfg, cfgPath, err := loadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	body, ok := cfg.Kinds[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "mdsmith: kinds path: unknown kind %q\n", name)
		return 2
	}
	rs, ok := body.Rules["required-structure"]
	if !ok || rs.Settings == nil {
		fmt.Fprintf(os.Stderr,
			"mdsmith: kinds path: kind %q has no required-structure schema\n", name)
		return 2
	}
	schema, ok := rs.Settings["schema"].(string)
	if !ok || schema == "" {
		fmt.Fprintf(os.Stderr,
			"mdsmith: kinds path: kind %q has no required-structure schema\n", name)
		return 2
	}
	out := schema
	if rootDir := rootDirFromConfig(cfgPath); rootDir != "" && !filepath.IsAbs(schema) {
		out = filepath.Join(rootDir, schema)
	}
	return writeStdout(out + "\n")
}

// runKindsResolve prints the resolved kind list and the merged rule
// config for one file, with provenance per leaf.
func runKindsResolve(args []string) int {
	jsonOut, posArgs, code := parseKindsFlags("resolve", args)
	if code >= 0 {
		return code
	}
	if len(posArgs) != 1 {
		fmt.Fprintln(os.Stderr, "mdsmith: kinds resolve requires exactly one file path")
		return 2
	}
	file := posArgs[0]
	cfg, fmKinds, code := loadConfigAndFMKinds(file)
	if code >= 0 {
		return code
	}
	res := config.ResolveWithProvenance(cfg, file, fmKinds)
	if jsonOut {
		return writeJSON(resolutionToJSON(res))
	}
	return printResolutionText(res)
}

// runKindsWhy prints the full merge chain for one rule on one file.
func runKindsWhy(args []string) int {
	jsonOut, posArgs, code := parseKindsFlags("why", args)
	if code >= 0 {
		return code
	}
	if len(posArgs) != 2 {
		fmt.Fprintln(os.Stderr, "mdsmith: kinds why requires <file> <rule>")
		return 2
	}
	file, ruleName := posArgs[0], posArgs[1]
	cfg, fmKinds, code := loadConfigAndFMKinds(file)
	if code >= 0 {
		return code
	}
	chain := config.ChainForRule(cfg, file, fmKinds, ruleName)
	if jsonOut {
		return writeJSON(struct {
			File  string           `json:"file"`
			Rule  string           `json:"rule"`
			Chain []chainEntryJSON `json:"chain"`
			Final any              `json:"final"`
		}{
			File:  file,
			Rule:  ruleName,
			Chain: chainToJSON(chain),
			Final: finalRuleValue(cfg, file, fmKinds, ruleName),
		})
	}
	return printChainText(file, ruleName, chain, finalRuleValue(cfg, file, fmKinds, ruleName))
}

// loadConfigAndFMKinds loads config and parses the file's front-matter
// kinds. Returns code -1 on success (use cfg/fmKinds) or a non-negative
// exit code on failure.
func loadConfigAndFMKinds(file string) (*config.Config, []string, int) {
	cfg, _, err := loadConfig("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return nil, nil, 2
	}
	data, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return nil, nil, 2
	}
	prefix, _ := lint.StripFrontMatter(data)
	fmKinds, err := lint.ParseFrontMatterKinds(prefix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return nil, nil, 2
	}
	if err := config.ValidateFrontMatterKinds(cfg, file, fmKinds); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return nil, nil, 2
	}
	return cfg, fmKinds, -1
}

// finalRuleValue returns the canonical merged final RuleCfg as a JSON-
// friendly any (true/false/map[string]any).
func finalRuleValue(cfg *config.Config, file string, fmKinds []string, ruleName string) any {
	rules := config.Effective(cfg, file, fmKinds)
	rc, ok := rules[ruleName]
	if !ok {
		return nil
	}
	if !rc.Enabled && len(rc.Settings) == 0 {
		return false
	}
	if rc.Enabled && len(rc.Settings) == 0 {
		return true
	}
	out := map[string]any{}
	for k, v := range rc.Settings {
		out[k] = v
	}
	return out
}

// --- text printers ---

func printResolutionText(res *config.Resolution) int {
	var sb strings.Builder
	fmt.Fprintf(&sb, "file: %s\n", res.File)
	if len(res.EffectiveKinds) == 0 {
		sb.WriteString("kinds: (none)\n")
	} else {
		sb.WriteString("kinds:\n")
		for _, k := range res.EffectiveKinds {
			srcs := res.KindSources[k]
			fmt.Fprintf(&sb, "  - %s  (from %s)\n", k, joinComma(srcs))
		}
	}
	names := make([]string, 0, len(res.Rules))
	for n := range res.Rules {
		names = append(names, n)
	}
	sort.Strings(names)
	sb.WriteString("rules:\n")
	for _, name := range names {
		rp := res.Rules[name]
		fmt.Fprintf(&sb, "  %s:\n", name)
		leafKeys := make([]string, 0, len(rp.Leaves))
		for k := range rp.Leaves {
			leafKeys = append(leafKeys, k)
		}
		sort.Strings(leafKeys)
		for _, k := range leafKeys {
			leaf := rp.Leaves[k]
			fmt.Fprintf(&sb, "    %s = %v  (%s)\n",
				k, leaf.Final, leaf.WinningSource)
		}
	}
	return writeStdout(sb.String())
}

func printChainText(file, ruleName string, chain []config.ChainEntry, final any) int {
	var sb strings.Builder
	fmt.Fprintf(&sb, "file: %s\nrule: %s\nchain:\n", file, ruleName)
	for _, e := range chain {
		marker := "no-op"
		if e.Touched {
			marker = "set"
		}
		if e.Touched {
			fmt.Fprintf(&sb, "  - %s [%s]: %v\n", e.Source, marker, e.Value)
		} else {
			fmt.Fprintf(&sb, "  - %s [%s]\n", e.Source, marker)
		}
	}
	fmt.Fprintf(&sb, "final: %v\n", final)
	return writeStdout(sb.String())
}

// --- JSON shapes (stable for plan 95 schema test) ---

type chainEntryJSON struct {
	Layer   string `json:"layer"`
	Source  string `json:"source"`
	Value   any    `json:"value,omitempty"`
	Touched bool   `json:"touched"`
}

type leafJSON struct {
	Final         any              `json:"final"`
	WinningSource string           `json:"winning_source"`
	Chain         []chainEntryJSON `json:"chain"`
}

type ruleJSON struct {
	Final  any                 `json:"final"`
	Leaves map[string]leafJSON `json:"leaves"`
}

type resolutionJSON struct {
	File       string              `json:"file"`
	Kinds      []kindRefJSON       `json:"kinds"`
	Rules      map[string]ruleJSON `json:"rules"`
	Categories map[string]bool     `json:"categories"`
	Explicit   map[string]bool     `json:"explicit"`
}

type kindRefJSON struct {
	Name    string   `json:"name"`
	Sources []string `json:"sources"`
}

func chainToJSON(chain []config.ChainEntry) []chainEntryJSON {
	out := make([]chainEntryJSON, 0, len(chain))
	for _, e := range chain {
		entry := chainEntryJSON{
			Layer:   string(e.Layer),
			Source:  e.Source,
			Touched: e.Touched,
		}
		if e.Touched {
			entry.Value = e.Value
		}
		out = append(out, entry)
	}
	return out
}

func resolutionToJSON(res *config.Resolution) resolutionJSON {
	rj := resolutionJSON{
		File:       res.File,
		Rules:      map[string]ruleJSON{},
		Categories: res.Categories,
		Explicit:   res.Explicit,
	}
	for _, k := range res.EffectiveKinds {
		rj.Kinds = append(rj.Kinds, kindRefJSON{
			Name:    k,
			Sources: res.KindSources[k],
		})
	}
	for name, rp := range res.Rules {
		leaves := map[string]leafJSON{}
		for lk, lp := range rp.Leaves {
			leaves[lk] = leafJSON{
				Final:         lp.Final,
				WinningSource: lp.WinningSource,
				Chain:         chainToJSON(lp.Chain),
			}
		}
		rj.Rules[name] = ruleJSON{
			Final:  ruleCfgValueAny(rp.Final),
			Leaves: leaves,
		}
	}
	return rj
}

// ruleCfgValueAny renders a RuleCfg as the value form used in JSON
// output (false / true / map). Mirrors config.ruleCfgValue but is
// internal to this package; that helper is unexported.
func ruleCfgValueAny(rc config.RuleCfg) any {
	if !rc.Enabled && len(rc.Settings) == 0 {
		return false
	}
	if rc.Enabled && len(rc.Settings) == 0 {
		return true
	}
	out := map[string]any{}
	for k, v := range rc.Settings {
		out[k] = v
	}
	return out
}

// --- helpers ---

func writeJSON(v any) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	return 0
}

// writeStdout writes s to stdout and returns 2 on write error.
func writeStdout(s string) int {
	if _, err := os.Stdout.WriteString(s); err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	return 0
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func joinComma(s []string) string {
	if len(s) == 0 {
		return "?"
	}
	out := s[0]
	for _, x := range s[1:] {
		out += ", " + x
	}
	return out
}
