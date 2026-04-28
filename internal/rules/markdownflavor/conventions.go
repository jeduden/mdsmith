package markdownflavor

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
// treats presets for unregistered rules as a no-op at check time;
// the settings remain in the merged config and activate
// automatically once the rule lands.
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
// package, so the markdownflavor package can declare convention
// tables without the import cycle that would otherwise result.
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
}

// Lookup returns the convention table entry for name. It returns an
// error naming the field and listing valid names when name is not a
// known convention, matching the failure-mode contract in plan 112.
func Lookup(name string) (Convention, error) {
	c, ok := conventions[name]
	if !ok {
		return Convention{}, fmt.Errorf(
			"unknown convention %q (valid: %s)",
			name, strings.Join(ConventionNames(), ", "),
		)
	}
	return c, nil
}

// ConventionNames returns the sorted list of built-in convention
// names.
func ConventionNames() []string {
	names := make([]string, 0, len(conventions))
	for k := range conventions {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
