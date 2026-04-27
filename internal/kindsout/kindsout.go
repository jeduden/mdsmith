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
	"gopkg.in/yaml.v3"
)

// --- JSON shapes ---

// BodyJSON is the JSON form of a kind body, used by `kinds list` and
// `kinds show`.
type BodyJSON struct {
	Name       string                 `json:"name"`
	Rules      map[string]RuleCfgJSON `json:"rules"`
	Categories map[string]bool        `json:"categories,omitempty"`
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

// MakeBodyJSON renders a KindBody as a JSON-friendly value.
func MakeBodyJSON(name string, body config.KindBody) BodyJSON {
	rules := make(map[string]RuleCfgJSON, len(body.Rules))
	for k, v := range body.Rules {
		rules[k] = RuleCfgJSON{v: RuleCfgValue(v)}
	}
	return BodyJSON{
		Name:       name,
		Rules:      rules,
		Categories: body.Categories,
	}
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
// assigned ("front-matter" or "kind-assignment[<i>]").
type ResolvedKindJSON struct {
	Name   string `json:"name"`
	Source string `json:"source"`
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
	for _, k := range res.Kinds {
		out.Kinds = append(out.Kinds, ResolvedKindJSON{
			Name: k.Name, Source: string(k.Source),
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

// WriteBodyText prints a kind body as YAML, wrapped with a header line
// naming the kind.
func WriteBodyText(w io.Writer, name string, body config.KindBody) error {
	if _, err := fmt.Fprintf(w, "%s:\n", sanitizeControl(name)); err != nil {
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
			if _, err := fmt.Fprintf(w, "  - %s (from %s)\n",
				sanitizeControl(k.Name), sanitizeControl(string(k.Source))); err != nil {
				return err
			}
		}
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
