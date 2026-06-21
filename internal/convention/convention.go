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
	// no-llm-tells ships the mechanical layer of the docs-author
	// anti-slop catalog as a one-key convention. It enables MDS056
	// (forbidden-text) with a curated list of LLM vocabulary and phrase
	// tells, MDS055 (forbidden-paragraph-starts) with the banned
	// sentence openers, and tightens MDS023 (max-words-per-sentence) and
	// MDS024 (paragraph-readability max-index) for non-native readers.
	// MDS027 (descriptive-link-text) rounds out the bundle.
	//
	// Flavor is left unset (FlavorAny): anti-slop is renderer-agnostic,
	// so the convention must not force a GFM or Obsidian project off its
	// flavor. The convention does not enable markdown-flavor (MDS034), so
	// the flavor field never reports; the loader skips the flavor-conflict
	// guard for a convention whose flavor is FlavorAny.
	//
	// The curated lists live in nollmtells.go; their source of truth is
	// .claude/skills/docs-author/slop-patterns.md, kept in sync by the
	// drift-checker integration test.
	"no-llm-tells": {
		Name:   "no-llm-tells",
		Flavor: FlavorAny,
		Rules: map[string]RulePreset{
			"forbidden-text": {
				Enabled: true,
				Settings: map[string]any{
					"contains": toAnySlice(llmVocabularyAndPhrases()),
				},
			},
			"forbidden-paragraph-starts": {
				Enabled: true,
				Settings: map[string]any{
					"starts": toAnySlice(llmParagraphOpeners()),
				},
			},
			"paragraph-structure": {
				Enabled: true,
				Settings: map[string]any{
					"max-words-per-sentence": 25,
				},
			},
			"paragraph-readability": {
				Enabled:  true,
				Settings: map[string]any{"max-index": 12.0},
			},
			"descriptive-link-text": {Enabled: true},
		},
	},
	// The <linter>-parity family configures mdsmith to run the same
	// rule set a specific peer Markdown linter runs by default, so a
	// benchmark of mdsmith against that peer measures the same work,
	// not a different rule count. There is one convention per peer:
	// gomarklint-parity, mado-parity, rumdl-parity, markdownlint-parity.
	//
	// Each set is derived from the per-rule peer mappings in the rule
	// README front matter (the `gomarklint:`/`mado:`/`rumdl:`/
	// `markdownlint:` blocks, surfaced in Go by rules.ListRules). A
	// convention ENABLES the mdsmith opt-in rules the peer runs by
	// default, and DISABLES the mdsmith default-on rules the peer does
	// not — leaving mdsmith's effective rule set equal to the peer's
	// default-on set. Only FULL covers count: a mapping marked
	// `partial: true` means the peer rule checks less than the mdsmith
	// rule, so parity does not run the heavier mdsmith rule on its
	// behalf. internal/integration verifies each convention against that
	// front matter, so the lists below cannot drift from the coverage
	// matrix.
	//
	// docs/research/benchmarks/bench-<linter>-parity.mdsmith.yml selects
	// each one, and the per-convention rule tables in the conventions
	// reference and the benchmark page are generated from these maps
	// (`mdsmith-release sync-parity-rules`).
	//
	// Flavor is gfm — the peers target GFM — but the conventions do not
	// enable markdown-flavor (MDS034 stays opt-in), so the flavor only
	// matters if the user also sets rules.markdown-flavor.flavor, which
	// must then agree.
	//
	// Note: every parity set disables cross-file-reference-integrity
	// (MDS027). The peers' `link-fragments`/MD051 rules resolve only
	// same-file anchors, while mdsmith's MDS027 also walks the workspace
	// for cross-file links, so those mappings are partial. Dropping
	// MDS027 keeps gomarklint-parity and mado-parity fully
	// parse-skip-safe; a future same-file-anchors rule could restore a
	// like-for-like anchor check (plan
	// 2606210840_same-file-anchor-resolution-rule.md).
	"gomarklint-parity": {
		Name:   "gomarklint-parity",
		Flavor: FlavorGFM,
		Rules: map[string]RulePreset{
			// Enable the 3 mdsmith opt-in rules gomarklint runs by default.
			"emphasis-style":    {Enabled: true},
			"list-marker-style": {Enabled: true},
			"single-h1":         {Enabled: true},
			// Disable the 25 mdsmith defaults gomarklint does not run by default.
			"atx-heading-whitespace":         {Enabled: false},
			"blockquote-whitespace":          {Enabled: false},
			"build":                          {Enabled: false},
			"catalog":                        {Enabled: false},
			"code-block-style":               {Enabled: false},
			"commands-show-output":           {Enabled: false},
			"cross-file-reference-integrity": {Enabled: false},
			"empty-section-body":             {Enabled: false},
			"first-line-heading":             {Enabled: false},
			"include":                        {Enabled: false},
			"line-length":                    {Enabled: false},
			"list-indent":                    {Enabled: false},
			"list-marker-space":              {Enabled: false},
			"max-file-length":                {Enabled: false},
			"no-trailing-spaces":             {Enabled: false},
			"no-undefined-reference-labels":  {Enabled: false},
			"no-unused-link-definitions":     {Enabled: false},
			"paragraph-readability":          {Enabled: false},
			"recipe-safety":                  {Enabled: false},
			"required-structure":             {Enabled: false},
			"table-format":                   {Enabled: false},
			"table-readability":              {Enabled: false},
			"toc":                            {Enabled: false},
			"token-budget":                   {Enabled: false},
			"unique-frontmatter":             {Enabled: false},
		},
	},
	"mado-parity": {
		Name:   "mado-parity",
		Flavor: FlavorGFM,
		Rules: map[string]RulePreset{
			// Enable the 8 mdsmith opt-in rules mado runs by default.
			"ambiguous-emphasis":     {Enabled: true},
			"horizontal-rule-style":  {Enabled: true},
			"list-marker-style":      {Enabled: true},
			"no-inline-html":         {Enabled: true},
			"no-space-in-code-spans": {Enabled: true},
			"no-space-in-link-text":  {Enabled: true},
			"ordered-list-numbering": {Enabled: true},
			"single-h1":              {Enabled: true},
			// Disable the 23 mdsmith defaults mado does not run by default.
			"blank-line-around-lists":        {Enabled: false},
			"build":                          {Enabled: false},
			"catalog":                        {Enabled: false},
			"cross-file-reference-integrity": {Enabled: false},
			"empty-section-body":             {Enabled: false},
			"fenced-code-style":              {Enabled: false},
			"heading-style":                  {Enabled: false},
			"include":                        {Enabled: false},
			"link-validity":                  {Enabled: false},
			"list-indent":                    {Enabled: false},
			"max-file-length":                {Enabled: false},
			"no-empty-alt-text":              {Enabled: false},
			"no-undefined-reference-labels":  {Enabled: false},
			"no-unused-link-definitions":     {Enabled: false},
			"paragraph-readability":          {Enabled: false},
			"recipe-safety":                  {Enabled: false},
			"required-structure":             {Enabled: false},
			"table-format":                   {Enabled: false},
			"table-readability":              {Enabled: false},
			"toc":                            {Enabled: false},
			"token-budget":                   {Enabled: false},
			"unclosed-code-block":            {Enabled: false},
			"unique-frontmatter":             {Enabled: false},
		},
	},
	"rumdl-parity": {
		Name:   "rumdl-parity",
		Flavor: FlavorGFM,
		Rules: map[string]RulePreset{
			// Enable the 12 mdsmith opt-in rules rumdl runs by default.
			"ambiguous-emphasis":     {Enabled: true},
			"descriptive-link-text":  {Enabled: true},
			"emphasis-style":         {Enabled: true},
			"horizontal-rule-style":  {Enabled: true},
			"link-style":             {Enabled: true},
			"list-marker-style":      {Enabled: true},
			"no-inline-html":         {Enabled: true},
			"no-space-in-code-spans": {Enabled: true},
			"no-space-in-link-text":  {Enabled: true},
			"ordered-list-numbering": {Enabled: true},
			"proper-names":           {Enabled: true},
			"single-h1":              {Enabled: true},
			// Disable the 13 mdsmith defaults rumdl does not run by default.
			"build":                          {Enabled: false},
			"catalog":                        {Enabled: false},
			"cross-file-reference-integrity": {Enabled: false},
			"empty-section-body":             {Enabled: false},
			"include":                        {Enabled: false},
			"max-file-length":                {Enabled: false},
			"paragraph-readability":          {Enabled: false},
			"recipe-safety":                  {Enabled: false},
			"table-readability":              {Enabled: false},
			"toc":                            {Enabled: false},
			"token-budget":                   {Enabled: false},
			"unclosed-code-block":            {Enabled: false},
			"unique-frontmatter":             {Enabled: false},
		},
	},
	"markdownlint-parity": {
		Name:   "markdownlint-parity",
		Flavor: FlavorGFM,
		Rules: map[string]RulePreset{
			// Enable the 12 mdsmith opt-in rules markdownlint runs by default.
			"ambiguous-emphasis":     {Enabled: true},
			"descriptive-link-text":  {Enabled: true},
			"emphasis-style":         {Enabled: true},
			"horizontal-rule-style":  {Enabled: true},
			"link-style":             {Enabled: true},
			"list-marker-style":      {Enabled: true},
			"no-inline-html":         {Enabled: true},
			"no-space-in-code-spans": {Enabled: true},
			"no-space-in-link-text":  {Enabled: true},
			"ordered-list-numbering": {Enabled: true},
			"proper-names":           {Enabled: true},
			"single-h1":              {Enabled: true},
			// Disable the 13 mdsmith defaults markdownlint does not run by default.
			"build":                          {Enabled: false},
			"catalog":                        {Enabled: false},
			"cross-file-reference-integrity": {Enabled: false},
			"empty-section-body":             {Enabled: false},
			"include":                        {Enabled: false},
			"max-file-length":                {Enabled: false},
			"paragraph-readability":          {Enabled: false},
			"recipe-safety":                  {Enabled: false},
			"table-readability":              {Enabled: false},
			"toc":                            {Enabled: false},
			"token-budget":                   {Enabled: false},
			"unclosed-code-block":            {Enabled: false},
			"unique-frontmatter":             {Enabled: false},
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
