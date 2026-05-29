package convention

import (
	"fmt"
	"sort"
	"strings"
)

// Convention is an opinionated bundle that pairs a Markdown flavor
// with a table of rule presets. Selecting a convention in config
// applies both: MDS034 runs against the named flavor, and the named
// rule presets are applied as a base layer beneath the user's own
// rule config.
//
// Conventions are codebase-versioned. The Rules table may reference
// rules that are not yet registered (presets for upcoming MDS04x
// rules ship alongside the convention so that adding the rule does
// not require updating every consumer's config). The config loader
// stores those presets in the merged config; the rule engine
// iterates the registered rules at check time and silently skips
// any preset that does not name one. The settings activate
// automatically once the rule lands and registers.
type Convention struct {
	// Name is the lowercase identifier used in YAML config.
	Name string
	// Flavor is the Markdown flavor MDS034 should validate against.
	Flavor Flavor
	// Rules maps rule name (e.g. "no-inline-html") to the preset that
	// the convention applies for that rule.
	Rules map[string]RulePreset
}

// RulePreset is a convention's preset for a single rule. It mirrors
// the shape of config.RuleCfg without depending on the config
// package, so this package can declare convention tables without the
// import cycle that would otherwise result.
type RulePreset struct {
	Enabled  bool
	Settings map[string]any
}

// conventions is the built-in convention table. Each entry pairs a
// target flavor with rule-by-rule presets. New conventions are added
// here; the table is consulted via Lookup.
var conventions = map[string]Convention{
	"portable": {
		Name:   "portable",
		Flavor: FlavorCommonMark,
		Rules: map[string]RulePreset{
			"markdown-flavor": {
				Enabled:  true,
				Settings: map[string]any{"flavor": "commonmark"},
			},
			"no-inline-html": {Enabled: true},
			"no-reference-style": {
				Enabled:  true,
				Settings: map[string]any{"allow-footnotes": false},
			},
			"emphasis-style": {
				Enabled: true,
				Settings: map[string]any{
					"bold":   "asterisk",
					"italic": "underscore",
				},
			},
			"horizontal-rule-style": {
				Enabled: true,
				Settings: map[string]any{
					"style":               "dash",
					"length":              3,
					"require-blank-lines": true,
				},
			},
			"list-marker-style": {
				Enabled:  true,
				Settings: map[string]any{"style": "dash"},
			},
			"ordered-list-numbering": {
				Enabled: true,
				Settings: map[string]any{
					"style": "sequential",
					"start": 1,
				},
			},
			"ambiguous-emphasis": {
				Enabled:  true,
				Settings: map[string]any{"max-run": 2},
			},
		},
	},
	"github": {
		Name:   "github",
		Flavor: FlavorGFM,
		Rules: map[string]RulePreset{
			"markdown-flavor": {
				Enabled:  true,
				Settings: map[string]any{"flavor": "gfm"},
			},
			"no-inline-html": {
				Enabled:  true,
				Settings: map[string]any{"allow": []any{"details", "summary"}},
			},
			"emphasis-style": {
				Enabled: true,
				Settings: map[string]any{
					"bold":   "asterisk",
					"italic": "underscore",
				},
			},
			"list-marker-style": {
				Enabled:  true,
				Settings: map[string]any{"style": "dash"},
			},
		},
	},
	"obsidian": {
		Name:   "obsidian",
		Flavor: FlavorGFM,
		Rules: map[string]RulePreset{
			"markdown-flavor": {
				Enabled:  true,
				Settings: map[string]any{"flavor": "gfm"},
			},
			"cross-file-reference-integrity": {
				Enabled: true,
				Settings: map[string]any{
					"wikilinks":      true,
					"wikilink-style": "obsidian",
				},
			},
			"callout-type": {Enabled: true},
		},
	},
	"plain": {
		Name:   "plain",
		Flavor: FlavorCommonMark,
		Rules: map[string]RulePreset{
			"markdown-flavor": {
				Enabled:  true,
				Settings: map[string]any{"flavor": "commonmark"},
			},
			"no-inline-html": {
				Enabled:  true,
				Settings: map[string]any{"allow-comments": false},
			},
			"no-reference-style": {
				Enabled:  true,
				Settings: map[string]any{"allow-footnotes": false},
			},
			"emphasis-style": {
				Enabled: true,
				Settings: map[string]any{
					"bold":   "asterisk",
					"italic": "underscore",
				},
			},
			"horizontal-rule-style": {
				Enabled: true,
				Settings: map[string]any{
					"style":               "dash",
					"length":              3,
					"require-blank-lines": true,
				},
			},
			"list-marker-style": {
				Enabled:  true,
				Settings: map[string]any{"style": "dash"},
			},
			"ordered-list-numbering": {
				Enabled: true,
				Settings: map[string]any{
					"style": "sequential",
					"start": 1,
				},
			},
			"ambiguous-emphasis": {
				Enabled:  true,
				Settings: map[string]any{"max-run": 2},
			},
		},
	},
	// parity restricts mdsmith to the markdownlint-compatible rule
	// class — the structural style rules the Rust markdownlint ports
	// (mado, rumdl) also run — by turning off every mdsmith-only rule:
	// the cross-file link graph, the readability / structure / token
	// budgets, the generated-section directives, the project-layout and
	// size policies, and the repo-specific content policies. MDS020 and
	// MDS027 are disabled too: they carry markdownlint analogs (MD043,
	// MD051) but cover them at higher fidelity, so parity drops them
	// rather than claim a like-for-like match.
	//
	// This is the source of truth for the parity class:
	// docs/research/benchmarks/bench-parity.mdsmith.yml selects it, and
	// the disabled-rule table in the conventions reference and the
	// benchmark page is generated from it
	// (`mdsmith-release sync-parity-rules`). A rule added here flows to
	// all three with no hand edit.
	//
	// Flavor is gfm — mado and rumdl target GFM — but parity does not
	// enable markdown-flavor (MDS034 stays opt-in), so the flavor only
	// matters if the user also sets rules.markdown-flavor.flavor, which
	// must then agree.
	"parity": {
		Name:   "parity",
		Flavor: FlavorGFM,
		Rules: map[string]RulePreset{
			// Cross-file graph + workspace walk (no markdownlint analog).
			"cross-file-reference-integrity": {Enabled: false},
			// Readability / structure / budget metrics (no analog).
			"paragraph-readability": {Enabled: false},
			"paragraph-structure":   {Enabled: false},
			"token-budget":          {Enabled: false},
			"conciseness-scoring":   {Enabled: false},
			"duplicated-content":    {Enabled: false},
			// Generated-section directives (no analog).
			"catalog":            {Enabled: false},
			"include":            {Enabled: false},
			"required-structure": {Enabled: false},
			"build":              {Enabled: false},
			"recipe-safety":      {Enabled: false},
			"toc":                {Enabled: false},
			"toc-directive":      {Enabled: false},
			"git-hook-sync":      {Enabled: false},
			// Project-layout / size policies (no analog).
			"directory-structure": {Enabled: false},
			"max-file-length":     {Enabled: false},
			"max-section-length":  {Enabled: false},
			"empty-section-body":  {Enabled: false},
			// Repo-specific content policies (no analog).
			"forbidden-paragraph-starts": {Enabled: false},
			"forbidden-text":             {Enabled: false},
			"required-text-patterns":     {Enabled: false},
			"required-mentions":          {Enabled: false},
			// mdsmith-only style rule with no markdownlint counterpart.
			"no-reference-style": {Enabled: false},
			// Table readability heuristic (markdownlint's table rules
			// differ in kind; excluded to avoid claiming false parity).
			"table-readability": {Enabled: false},
		},
	},
}

// Lookup returns the convention with the given name. It first checks
// userConventions (user-defined entries from .mdsmith.yml), then the
// built-in table. Returns a deep copy so callers can mutate the
// result without corrupting the shared tables. Error lists both sets
// of valid names when neither matches.
//
// userConventions may be nil; a nil map is treated as empty.
func Lookup(name string, userConventions map[string]Convention) (Convention, error) {
	if c, ok := userConventions[name]; ok {
		return cloneConvention(c), nil
	}
	c, ok := conventions[name]
	if !ok {
		names := namesWithUser(userConventions)
		return Convention{}, fmt.Errorf(
			"unknown convention %q (valid: %s)",
			name, strings.Join(names, ", "),
		)
	}
	return cloneConvention(c), nil
}

// namesWithUser returns a sorted list of all available convention
// names — built-in names plus user-defined names.
func namesWithUser(userConventions map[string]Convention) []string {
	names := Names()
	for name := range userConventions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// cloneConvention returns a deep copy of c. Each rule preset's
// Settings map is cloned recursively so callers cannot mutate the
// package-level table by writing through the returned value.
func cloneConvention(c Convention) Convention {
	rules := make(map[string]RulePreset, len(c.Rules))
	for k, v := range c.Rules {
		rules[k] = RulePreset{
			Enabled:  v.Enabled,
			Settings: cloneAny(v.Settings),
		}
	}
	return Convention{
		Name:   c.Name,
		Flavor: c.Flavor,
		Rules:  rules,
	}
}

// cloneAny deep-copies a settings map, recursing into nested maps
// and slices. Scalar leaf values are returned as-is. Returns nil if
// the input is nil.
func cloneAny(v map[string]any) map[string]any {
	if v == nil {
		return nil
	}
	out := make(map[string]any, len(v))
	for k, val := range v {
		out[k] = cloneValue(val)
	}
	return out
}

func cloneValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = cloneValue(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = cloneValue(e)
		}
		return out
	case []string:
		out := make([]string, len(x))
		copy(out, x)
		return out
	case []int:
		out := make([]int, len(x))
		copy(out, x)
		return out
	case []bool:
		out := make([]bool, len(x))
		copy(out, x)
		return out
	default:
		return v
	}
}

// Names returns the sorted list of built-in convention names.
func Names() []string {
	names := make([]string, 0, len(conventions))
	for k := range conventions {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
