// Package kindsout renders the output of the 'mdsmith kinds'
// subcommand surface: declared-kind bodies, per-file resolutions,
// and per-rule merge chains. It exposes both stable JSON shapes
// (for LSPs and other tools) and human-readable text.
package kindsout

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/jeduden/mdsmith/internal/yamlutil"
)

// --- JSON shapes ---

// BodyJSON is the JSON form of a kind body, used by `kinds list` and
// `kinds show`. When the kind declares `extends:`, the JSON carries
// the inheritance chain (child-first) and the resolved
// per-frontmatter-field provenance so audit tools can see which
// layer contributed each constraint without reading every schema.
type BodyJSON struct {
	Name                 string                 `json:"name"`
	Rules                map[string]RuleCfgJSON `json:"rules"`
	Categories           map[string]bool        `json:"categories,omitempty"`
	PathPattern          string                 `json:"path-pattern,omitempty"`
	Extends              string                 `json:"extends,omitempty"`
	ExtendsChain         []string               `json:"extends-chain,omitempty"`
	EffectiveFrontmatter []FrontmatterLeafJSON  `json:"effective-frontmatter,omitempty"`
	// SourcePath, when set, is the file that defined the kind body
	// (`.mdsmith.yml` for inline kinds, `.mdsmith/kinds/<name>.{yaml,yml}`
	// for file kinds; plan 208).
	SourcePath string `json:"source-path,omitempty"`
}

// FrontmatterLeafJSON describes one effective frontmatter key after
// the extends chain has been resolved: the key, the unified CUE
// expression, and the kind that contributed it (the bottom-most
// kind in the chain that declared this key).
type FrontmatterLeafJSON struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Source string `json:"source"`
}

// RuleCfgJSON serializes a config.RuleCfg using its YAML union form:
// false (disabled), true (enabled, no settings), or the settings map.
type RuleCfgJSON struct {
	v any
}

// MarshalJSON implements json.Marshaler.
func (r RuleCfgJSON) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.v)
}

// MakeBodyJSON renders a KindBody as a JSON-friendly value. Pass
// the project's `kinds` map so callers that have it can populate
// the inheritance chain and per-frontmatter-field provenance; a nil
// map yields the kind's own body without inheritance metadata,
// matching the pre-extends shape. The audit fields
// (`ExtendsChain`, `EffectiveFrontmatter`) populate only when the
// kind itself declares `extends:` — for non-inheriting kinds the
// JSON shape is the pre-plan-135 form, so audit tooling can detect
// inheritance by checking for the new fields.
func MakeBodyJSON(name string, body config.KindBody, kinds map[string]config.KindBody) BodyJSON {
	rules := make(map[string]RuleCfgJSON, len(body.Rules))
	for k, v := range body.Rules {
		rules[k] = RuleCfgJSON{v: RuleCfgValue(v)}
	}
	out := BodyJSON{
		Name:        name,
		Rules:       rules,
		Categories:  body.Categories,
		PathPattern: body.PathPattern,
		Extends:     body.Extends,
		SourcePath:  body.SourcePath,
	}
	if kinds == nil || body.Extends == "" {
		return out
	}
	out.ExtendsChain = config.KindExtendsChain(kinds, name)
	if leaves := effectiveFrontmatterLeaves(kinds, name); len(leaves) > 0 {
		out.EffectiveFrontmatter = leaves
	}
	return out
}

// effectiveFrontmatterLeaves resolves the inline schema for `name`
// across its extends chain and projects each frontmatter key into
// a provenance leaf naming the contributing kind. A key whose
// expression is the unified form (`(parent) & (child)`) is
// attributed to the child — that is the layer the diagnostic should
// point at when the value is rejected — and the chain entries that
// contributed the parent constraint are still available via the
// catalog of `kinds show` runs along the chain.
func effectiveFrontmatterLeaves(
	kinds map[string]config.KindBody, name string,
) []FrontmatterLeafJSON {
	resolved, err := config.ResolveKindInlineSchema(kinds, name)
	if err != nil || resolved == nil {
		return nil
	}
	fm, _ := resolved["frontmatter"].(map[string]any)
	if len(fm) == 0 {
		return nil
	}
	owners := frontmatterKeyOwners(kinds, name)
	keys := make([]string, 0, len(fm))
	for k := range fm {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]FrontmatterLeafJSON, 0, len(keys))
	for _, k := range keys {
		// Coerce non-string values (a raw YAML number, an
		// unrecognised shortcut name) to their canonical CUE form
		// so the audit output never carries an empty `value`. The
		// resolver already normalises strings; this catches the
		// fallback cases where MergeRawMap left a raw value
		// because frontmatterExpr could not resolve it.
		out = append(out, FrontmatterLeafJSON{
			Key:    k,
			Value:  schema.NormalizeFrontmatterValue(fm[k]),
			Source: owners[k],
		})
	}
	return out
}

// frontmatterKeyOwners walks the extends chain from root to child
// and records the last kind that declared each frontmatter key.
// Last-writer-wins matches the "child overrides parent" intent: an
// effective constraint that survived through `&` unification is
// surfaced as the child's leaf (the child is the layer the user
// edits to relax or tighten the constraint), while a key declared
// only by a parent is attributed to that parent.
func frontmatterKeyOwners(
	kinds map[string]config.KindBody, name string,
) map[string]string {
	chain := config.KindExtendsChain(kinds, name)
	owners := map[string]string{}
	// Walk root → child so the child's declaration wins.
	for i := len(chain) - 1; i >= 0; i-- {
		k := chain[i]
		body := kinds[k]
		fm, _ := body.Schema.Map()["frontmatter"].(map[string]any)
		for key := range fm {
			owners[key] = k
		}
	}
	return owners
}

// RuleCfgValue returns the JSON-friendly value of a RuleCfg, matching
// its YAML marshalling: false, true, or the settings map.
func RuleCfgValue(rc config.RuleCfg) any {
	// A disabled rule maps to `false` regardless of inherited Settings.
	// Deep-merge can produce {Enabled: false, Settings: <inherited>}
	// when a bool-only later layer toggles the rule off; reporting
	// `final: true` (or the settings map) in that case would
	// contradict the `enabled` leaf. The per-leaf chain still carries
	// the inherited values for tooling that needs them.
	if !rc.Enabled {
		return false
	}
	if len(rc.Settings) > 0 {
		return rc.Settings
	}
	return true
}

// ResolvedKindJSON names a kind in the effective list and how it was
// assigned ("front-matter" or "kind-assignment[<i>]"). Selector, when
// non-empty, describes the selectors that fired on a kind-assignment
// match ("glob a,b AND fields-present x"). SourcePath, when set, is
// the file that defined the kind body (`.mdsmith.yml` for inline
// kinds, `.mdsmith/kinds/<name>.{yaml,yml}` for file kinds; plan 208).
// SchemaSourcePath, when set, is the file that defined the kind's
// schema when distinct from the kind itself: a `.mdsmith/schemas/
// <name>.yaml` path (or `.mdsmith.yml` for an inline-registry entry)
// for a named reference, or the `rules.required-structure.schema:`
// path for a proto.md schema; omitted for an inline-on-kind schema
// (plan 241).
type ResolvedKindJSON struct {
	Name             string `json:"name"`
	Source           string `json:"source"`
	Selector         string `json:"selector,omitempty"`
	SourcePath       string `json:"source-path,omitempty"`
	SchemaSourcePath string `json:"schema-source-path,omitempty"`
}

// ResolvedConventionJSON names the active convention and, for a user
// convention, the file that defined it. User is true for a user
// convention (inline in `.mdsmith.yml` or a
// `.mdsmith/conventions/<name>.{yaml,yml}` file; plan 209); SourcePath
// is set only then — built-in conventions carry no path. Absent from
// the file resolution when no convention is selected.
type ResolvedConventionJSON struct {
	Name       string `json:"name"`
	User       bool   `json:"user,omitempty"`
	SourcePath string `json:"source-path,omitempty"`
}

// LeafJSON is one effective leaf with its winning source and the chain
// of layers that set it.
type LeafJSON struct {
	Path   string          `json:"path"`
	Value  any             `json:"value"`
	Source string          `json:"source"`
	Chain  []LeafChainJSON `json:"chain,omitempty"`
}

// LeafChainJSON is one layer in a leaf's merge chain.
type LeafChainJSON struct {
	Source string `json:"source"`
	Value  any    `json:"value"`
}

// LayerJSON describes one applicable merge layer for a rule. When Set
// is false the layer did not touch the rule and Value is omitted.
type LayerJSON struct {
	Source string `json:"source"`
	Set    bool   `json:"set"`
	Value  any    `json:"value,omitempty"`
}

// RuleResolutionJSON is the JSON form of a per-rule merge chain.
type RuleResolutionJSON struct {
	File   string      `json:"file"`
	Rule   string      `json:"rule"`
	Final  any         `json:"final"`
	Layers []LayerJSON `json:"layers"`
	Leaves []LeafJSON  `json:"leaves"`
}

// RuleSummaryJSON is the per-rule summary inside a file resolution:
// the final config and per-leaf provenance.
type RuleSummaryJSON struct {
	Final  any        `json:"final"`
	Leaves []LeafJSON `json:"leaves"`
}

// FileResolutionJSON is the JSON form of a file's effective config.
type FileResolutionJSON struct {
	File       string                     `json:"file"`
	Convention *ResolvedConventionJSON    `json:"convention,omitempty"`
	Kinds      []ResolvedKindJSON         `json:"kinds"`
	Categories map[string]bool            `json:"categories,omitempty"`
	Rules      map[string]RuleSummaryJSON `json:"rules"`
}

// FileResolution converts a config.FileResolution to its JSON shape.
func FileResolution(res *config.FileResolution) FileResolutionJSON {
	out := FileResolutionJSON{
		File:       res.File,
		Kinds:      make([]ResolvedKindJSON, 0, len(res.Kinds)),
		Categories: res.Categories,
		Rules:      make(map[string]RuleSummaryJSON, len(res.Rules)),
	}
	if res.Convention.Name != "" {
		out.Convention = &ResolvedConventionJSON{
			Name:       res.Convention.Name,
			User:       res.Convention.IsUser,
			SourcePath: res.Convention.SourcePath,
		}
	}
	for _, k := range res.Kinds {
		out.Kinds = append(out.Kinds, ResolvedKindJSON{
			Name:             k.Name,
			Source:           string(k.Source),
			Selector:         k.Selector,
			SourcePath:       k.SourcePath,
			SchemaSourcePath: k.SchemaSourcePath,
		})
	}
	for name, rr := range res.Rules {
		out.Rules[name] = RuleSummaryJSON{
			Final:  RuleCfgValue(rr.Final),
			Leaves: leavesJSON(rr.Leaves),
		}
	}
	return out
}

// RuleResolution converts a config.RuleResolution to its JSON shape.
func RuleResolution(file string, rr config.RuleResolution) RuleResolutionJSON {
	layers := make([]LayerJSON, 0, len(rr.Layers))
	for _, l := range rr.Layers {
		entry := LayerJSON{Source: l.Source, Set: l.Set}
		if l.Set {
			entry.Value = RuleCfgValue(l.Value)
		}
		layers = append(layers, entry)
	}
	return RuleResolutionJSON{
		File:   file,
		Rule:   rr.Rule,
		Final:  RuleCfgValue(rr.Final),
		Layers: layers,
		Leaves: leavesJSON(rr.Leaves),
	}
}

func leavesJSON(leaves []config.Leaf) []LeafJSON {
	out := make([]LeafJSON, 0, len(leaves))
	for _, l := range leaves {
		entry := LeafJSON{
			Path:   l.Path,
			Value:  l.Value,
			Source: l.Source(),
		}
		for _, c := range l.Chain {
			entry.Chain = append(entry.Chain, LeafChainJSON{
				Source: c.Source, Value: c.Value,
			})
		}
		out = append(out, entry)
	}
	return out
}

// WriteJSON emits v as pretty-printed JSON.
func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// --- Text rendering ---

// WriteBodyText prints a kind body as YAML, wrapped with a header
// line naming the kind. When the optional kinds map is supplied and
// the kind declares `extends:`, the output is preceded by the
// inheritance chain and followed by the resolved effective
// frontmatter with per-field provenance — the audit surface plan
// 135 calls for.
func WriteBodyText(
	w io.Writer, name string, body config.KindBody, kinds map[string]config.KindBody,
) error {
	if _, err := fmt.Fprintf(w, "%s:\n", sanitizeControl(name)); err != nil {
		return err
	}
	if body.SourcePath != "" {
		if _, err := fmt.Fprintf(w, "  defined-in: %s\n",
			sanitizeControl(body.SourcePath)); err != nil {
			return err
		}
	}
	if err := writeExtendsHeader(w, name, body, kinds); err != nil {
		return err
	}
	wrap := struct {
		Rules       map[string]config.RuleCfg `yaml:"rules,omitempty"`
		Categories  map[string]bool           `yaml:"categories,omitempty"`
		PathPattern string                    `yaml:"path-pattern,omitempty"`
	}{
		Rules:       body.Rules,
		Categories:  body.Categories,
		PathPattern: body.PathPattern,
	}
	data, err := yamlutil.Marshal(wrap)
	if err != nil {
		return err
	}
	rendered := strings.TrimRight(string(data), "\n")
	bodyEmpty := len(data) == 0 || strings.TrimSpace(string(data)) == "{}"
	if bodyEmpty && body.Extends == "" {
		if _, err := fmt.Fprintln(w, "  (empty)"); err != nil {
			return err
		}
	} else if !bodyEmpty {
		for _, line := range strings.Split(rendered, "\n") {
			if _, err := fmt.Fprintf(w, "  %s\n", line); err != nil {
				return err
			}
		}
	}
	// effective-frontmatter is the extends audit surface — only
	// surface it when the kind itself declares an extends parent.
	// A non-inheriting kind's frontmatter is already visible via
	// its own `schema:` block.
	if kinds != nil && body.Extends != "" {
		if err := writeEffectiveFrontmatter(w, kinds, name); err != nil {
			return err
		}
	}
	return nil
}

// writeExtendsHeader renders the inheritance chain and the parent
// name as the leading lines of `kinds show`. The chain is rendered
// child-first; a kind with no parent omits both lines so non-
// inheriting output is unchanged.
func writeExtendsHeader(
	w io.Writer, name string, body config.KindBody, kinds map[string]config.KindBody,
) error {
	if body.Extends == "" {
		return nil
	}
	if _, err := fmt.Fprintf(w, "  extends: %s\n",
		sanitizeControl(body.Extends)); err != nil {
		return err
	}
	if kinds == nil {
		return nil
	}
	chain := config.KindExtendsChain(kinds, name)
	if len(chain) > 1 {
		if _, err := fmt.Fprintf(w, "  extends-chain: %s\n",
			sanitizeControl(strings.Join(chain, " -> "))); err != nil {
			return err
		}
	}
	return nil
}

// writeEffectiveFrontmatter prints the resolved frontmatter for the
// extends chain, one line per key with the contributing kind in a
// trailing comment so the reader sees the layer without re-reading
// every schema. A kind without an inline schema or without
// frontmatter prints nothing.
func writeEffectiveFrontmatter(
	w io.Writer, kinds map[string]config.KindBody, name string,
) error {
	leaves := effectiveFrontmatterLeaves(kinds, name)
	if len(leaves) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "  effective-frontmatter:"); err != nil {
		return err
	}
	for _, leaf := range leaves {
		if _, err := fmt.Fprintf(w, "    %s: %s  # from %s\n",
			sanitizeControl(leaf.Key),
			sanitizeControl(leaf.Value),
			sanitizeControl(leaf.Source)); err != nil {
			return err
		}
	}
	return nil
}

// writeConventionLine prints the active-convention line for a file
// resolution — `convention: <name> (user) defined-in <path>` —
// mirroring a kind's `defined-in` suffix. Nothing is written when no
// convention is selected; a built-in convention prints its name with
// no `(user)` tag and no path (built-ins are compiled into the binary).
func writeConventionLine(w io.Writer, c config.ResolvedConvention) error {
	if c.Name == "" {
		return nil
	}
	line := "convention: " + sanitizeControl(c.Name)
	if c.IsUser {
		line += " (user)"
	}
	if c.SourcePath != "" {
		line += " defined-in " + sanitizeControl(c.SourcePath)
	}
	_, err := fmt.Fprintln(w, line)
	return err
}

// WriteFileResolutionText renders a per-file resolution as text, with
// effective kinds and per-leaf source info for every rule.
func WriteFileResolutionText(w io.Writer, res *config.FileResolution) error {
	if _, err := fmt.Fprintf(w, "file: %s\n", sanitizeControl(res.File)); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "effective kinds:"); err != nil {
		return err
	}
	if len(res.Kinds) == 0 {
		if _, err := fmt.Fprintln(w, "  (none)"); err != nil {
			return err
		}
	} else {
		for _, k := range res.Kinds {
			src := sanitizeControl(string(k.Source))
			if k.Selector != "" {
				src = src + ": " + sanitizeControl(k.Selector)
			}
			suffix := ""
			if k.SourcePath != "" {
				suffix = " defined-in " + sanitizeControl(k.SourcePath)
			}
			// When the schema lives in a separate file (named YAML or
			// proto.md), name it too so a reader can jump to the schema
			// rather than the referencing kind.
			if k.SchemaSourcePath != "" {
				suffix += " schema-in " + sanitizeControl(k.SchemaSourcePath)
			}
			if _, err := fmt.Fprintf(w, "  - %s (from %s)%s\n",
				sanitizeControl(k.Name), src, suffix); err != nil {
				return err
			}
		}
	}

	if err := writeConventionLine(w, res.Convention); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w, "rules:"); err != nil {
		return err
	}
	names := make([]string, 0, len(res.Rules))
	for name := range res.Rules {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		rr := res.Rules[name]
		if _, err := fmt.Fprintf(w, "  %s:\n", sanitizeControl(name)); err != nil {
			return err
		}
		for _, leaf := range rr.Leaves {
			if _, err := fmt.Fprintf(w, "    %s = %s  (from %s)\n",
				sanitizeControl(leaf.Path), FormatValue(leaf.Value),
				sanitizeControl(leaf.Source())); err != nil {
				return err
			}
		}
	}
	return nil
}

// WriteRuleResolutionText renders a per-rule merge chain as text,
// including no-op layers and the chain for every leaf.
func WriteRuleResolutionText(w io.Writer, file string, rr config.RuleResolution) error {
	if _, err := fmt.Fprintf(w, "file: %s\nrule: %s\n\nmerge chain (oldest -> newest):\n",
		sanitizeControl(file), sanitizeControl(rr.Rule)); err != nil {
		return err
	}
	for _, l := range rr.Layers {
		var line string
		if l.Set {
			line = fmt.Sprintf("  %-30s set    %s\n",
				sanitizeControl(l.Source), FormatValue(RuleCfgValue(l.Value)))
		} else {
			line = fmt.Sprintf("  %-30s no-op  (rule untouched)\n",
				sanitizeControl(l.Source))
		}
		if _, err := fmt.Fprint(w, line); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "\nper-leaf provenance:"); err != nil {
		return err
	}
	for _, leaf := range rr.Leaves {
		if _, err := fmt.Fprintf(w, "  %s = %s  (winning source: %s)\n",
			sanitizeControl(leaf.Path), FormatValue(leaf.Value),
			sanitizeControl(leaf.Source())); err != nil {
			return err
		}
		for _, c := range leaf.Chain {
			if _, err := fmt.Fprintf(w, "    %-28s %s\n",
				sanitizeControl(c.Source), FormatValue(c.Value)); err != nil {
				return err
			}
		}
	}
	return nil
}

// FormatValue renders a leaf value compactly (JSON-like) so settings
// maps, lists, and scalars all print on one line.
func FormatValue(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// sanitizeControl strips C0/C1 control characters from s so that
// user-controlled strings (kind names, rule names, file paths, source
// labels) cannot inject newlines or ANSI escapes into text output.
func sanitizeControl(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return -1
		}
		return r
	}, s)
}
