package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jeduden/mdsmith/internal/concepts"
	ruledocs "github.com/jeduden/mdsmith/internal/rules"
)

const helpUsageText = `Usage: mdsmith help <topic>

Topics:
  rule [id|name]        Show rule documentation
  metrics [id|name]     Show metric documentation
  kinds                 Show concept page for file kinds
  kinds-cli             Summarize the 'kinds' subcommand surface
  placeholder-grammar   Show placeholder vocabulary reference
  patterns              Show maintainability patterns across rules
`

// runHelp implements the "help" subcommand.
func runHelp(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, helpUsageText)
		return 0
	}

	switch args[0] {
	case "rule":
		return runHelpRule(args[1:])
	case "metrics":
		return runHelpMetrics(args[1:])
	case "kinds":
		return runHelpKinds()
	case "kinds-cli":
		return runHelpKindsCLI()
	case "placeholder-grammar":
		return runHelpConcept("placeholder-grammar")
	case "patterns":
		return runHelpPatterns(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "mdsmith: help: unknown topic %q\n", args[0])
		return 2
	}
}

// listRulesForHelp is the ruledocs.ListRules dependency, indirected through
// a package var so tests can substitute a fault-injecting lister and exercise
// the error-handling branches in the help subcommands.
var listRulesForHelp = ruledocs.ListRules

type patternRec struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Signal        string `json:"signal"`
	Fix           string `json:"fix"`
	ForDiagnostic bool   `json:"for-diagnostic"`
}

func runHelpPatterns(args []string) int {
	format, code, ok := parsePatternsFormat(args)
	if !ok {
		return code
	}
	rules, err := listRulesForHelp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	items := make([]patternRec, 0, len(rules))
	for _, r := range rules {
		if r.Maintainability == nil {
			continue
		}
		items = append(items, patternRec{
			ID:            r.ID,
			Name:          r.Name,
			Signal:        r.Maintainability.Signal,
			Fix:           r.Maintainability.Fix,
			ForDiagnostic: r.Maintainability.ForDiagnostic,
		})
	}
	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(items); err != nil {
			fmt.Fprintf(os.Stderr, "mdsmith: writing json: %v\n", err)
			return 2
		}
		return 0
	}
	for _, it := range items {
		fmt.Printf("%s %s\n  signal: %s\n  fix: %s\n  for-diagnostic: %t\n\n",
			it.ID, it.Name, it.Signal, it.Fix, it.ForDiagnostic)
	}
	return 0
}

// parsePatternsFormat extracts the --format value from runHelpPatterns args.
// Returns (format, exitCode, ok); when ok is false the caller should return
// exitCode immediately. Accepts: no args (text), `-f|--format <text|json>`.
func parsePatternsFormat(args []string) (string, int, bool) {
	if len(args) == 0 {
		return "text", 0, true
	}
	if args[0] != "-f" && args[0] != "--format" {
		fmt.Fprintf(os.Stderr, "mdsmith: help patterns: unexpected argument %q\n", args[0])
		return "", 2, false
	}
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "mdsmith: help patterns: %s requires a value (text or json)\n", args[0])
		return "", 2, false
	}
	if len(args) > 2 {
		fmt.Fprintf(os.Stderr, "mdsmith: help patterns: unexpected trailing argument %q\n", args[2])
		return "", 2, false
	}
	format := args[1]
	if format != "text" && format != "json" {
		fmt.Fprintf(os.Stderr, "mdsmith: help patterns: unknown format %q (valid: text, json)\n", format)
		return "", 2, false
	}
	return format, 0, true
}

const helpKindsText = `File Kinds

A kind is a named bundle of rule settings that can be applied to a set of
files. Kinds let you share per-rule tuning across files that serve the same
purpose (schema, template, fragment, prompt, …) without repeating overrides.

DECLARATION

Declare kinds under the kinds: key. The body has the same shape as an
override entry (rules:, categories:) — minus glob:, since files are bound
to kinds separately:

  kinds:
    plan:
      rules:
        required-structure:
          schema: plan/proto.md
        paragraph-readability: false
    proto:
      rules:
        paragraph-readability: false
        first-line-heading: false

Kind names are project-chosen. mdsmith ships no built-in kinds.

ASSIGNMENT

A file's effective kind list is built from two sources, concatenated in
this order:

  1. Front-matter kinds: field (YAML list).
  2. Matching entries in kind-assignment: (config order; each entry's kinds
     in the order listed).

Duplicate names are dropped after their first occurrence. Referencing an
undeclared kind is a config error.

  kind-assignment:
    - glob: ["plan/[0-9]*_*.md"]
      kinds: [plan]
    - glob: ["**/proto.md"]
      kinds: [proto]

MERGE ORDER

Kinds apply after top-level rules and before glob overrides:

  top-level rules → kinds (effective-list order) → glob overrides

Within kinds, the later kind in the effective list replaces the earlier
kind's entire rule config for that rule — no deep-merge, same as overrides.
A file's own glob overrides apply last and take highest precedence.

COMPOSABILITY

Rules never reference kind names. New kinds cannot regress existing behavior.
`

// runHelpKinds prints the kinds concept page.
func runHelpKinds() int {
	fmt.Print(helpKindsText)
	return 0
}

const helpKindsCLIText = `Kinds Subcommand

mdsmith kinds <subcommand> [args]

Subcommands:
  list                  Print declared kinds with their merged bodies.
  show <name>           Print one kind's merged body.
  path <name>           Print the resolved schema path of the kind's
                        required-structure rule, if any.
  resolve <file>        Print the resolved kind list and merged rule
                        config for a file, with per-leaf provenance.
  why <file> <rule>     Print the full merge chain for one rule on
                        one file: every applicable layer, including
                        no-ops, with the value at each step.

Each subcommand accepts --json for stable structured output. The
schema is documented in docs/reference/cli.md.

Provenance layers are: 'default' (top-level rules: + built-ins),
'kinds.<name>' (one per kind in the effective list), and
'overrides[<i>]' (one per matching glob override entry).

See also: 'mdsmith check --explain' / 'mdsmith fix --explain' to
attach the same provenance trailer to each diagnostic.
`

// runHelpKindsCLI prints the kinds-cli help topic.
func runHelpKindsCLI() int {
	fmt.Print(helpKindsCLIText)
	return 0
}

// runHelpConcept prints the named concept page.
func runHelpConcept(name string) int {
	content, err := concepts.Lookup(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	fmt.Print(content)
	return 0
}

// runHelpRule implements "help rule [id|name]".
func runHelpRule(args []string) int {
	if len(args) == 0 {
		return listAllRules()
	}
	return showRule(args[0])
}

func listAllRules() int {
	rules, err := listRulesForHelp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}

	for _, r := range rules {
		fmt.Printf("%-6s %-40s %-10s %s\n", r.ID, r.Name, r.Status, r.Description)
	}
	return 0
}

func showRule(query string) int {
	info, err := ruledocs.LookupRuleInfo(query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdsmith: %v\n", err)
		return 2
	}
	content := ruledocs.StripFrontMatter(info.Content)
	if m := info.Maintainability; m != nil {
		content += "\n\n## Maintainability pattern\n\n"
		content += fmt.Sprintf("- Signal: %s\n- Fix: %s\n- For diagnostic: %t\n",
			m.Signal, m.Fix, m.ForDiagnostic)
	}
	fmt.Print(content)
	return 0
}
