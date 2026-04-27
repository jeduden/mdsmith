package config

import (
	"github.com/jeduden/mdsmith/internal/rule"
)

// Layer identifies which config layer contributed a leaf setting to
// the effective rule config. It is used by EffectiveWithProvenance to
// surface where a value originated from.
type Layer int

const (
	// LayerDefault is the top-level rules: block (and the registered
	// rule defaults under it).
	LayerDefault Layer = iota
	// LayerKind is a kind body merged in via the effective-kind list.
	LayerKind
	// LayerOverride is a glob-matched overrides: entry.
	LayerOverride
)

// LayerSource records which layer set a particular leaf, plus enough
// detail to identify it (kind name or override file glob index).
type LayerSource struct {
	Layer Layer
	// KindName is set when Layer == LayerKind.
	KindName string
	// OverrideIndex is the position of the override entry in
	// cfg.Overrides; set when Layer == LayerOverride.
	OverrideIndex int
}

// listModeFunc returns the merge mode for a (rule, setting key) pair.
// Unknown rules and unknown keys return ListReplace.
type listModeFunc func(ruleName, key string) rule.ListMergeMode

// defaultListMode looks up the merge mode by consulting the registered
// rule (if it implements rule.ListMerger). Returns ListReplace for any
// rule that does not implement the interface, or for unknown keys.
func defaultListMode(ruleName, key string) rule.ListMergeMode {
	for _, r := range rule.All() {
		if r.Name() != ruleName {
			continue
		}
		lm, ok := r.(rule.ListMerger)
		if !ok {
			return rule.ListReplace
		}
		return lm.ListMergeMode(key)
	}
	return rule.ListReplace
}

// deepMergeRuleCfg merges src into dst in place and returns the result.
// Maps are merged key by key (recursing on nested maps); scalars are
// replaced; lists follow listMode for the rule+key. A src entry whose
// Settings is nil and Enabled=false acts as a hard disable: it sets
// dst.Enabled=false and clears dst.Settings.
func deepMergeRuleCfg(
	dst, src RuleCfg, ruleName string, listMode listModeFunc,
) RuleCfg {
	// Disable wins (no settings means: hard set, drop accumulated).
	if !src.Enabled && src.Settings == nil {
		return RuleCfg{Enabled: false}
	}

	out := RuleCfg{Enabled: src.Enabled}
	if dst.Settings == nil && src.Settings == nil {
		return out
	}

	merged := make(map[string]any, len(dst.Settings)+len(src.Settings))
	for k, v := range dst.Settings {
		merged[k] = cloneValue(v)
	}
	for k, sv := range src.Settings {
		dv, present := merged[k]
		if !present {
			merged[k] = cloneValue(sv)
			continue
		}
		merged[k] = mergeValue(dv, sv, ruleName, k, listMode)
	}
	out.Settings = merged
	return out
}

// mergeValue dispatches on the source value's type. Maps recurse,
// lists follow the rule+key merge mode, scalars are replaced.
func mergeValue(
	dst, src any, ruleName, key string, listMode listModeFunc,
) any {
	switch s := src.(type) {
	case map[string]any:
		d, ok := dst.(map[string]any)
		if !ok {
			return cloneValue(src)
		}
		out := make(map[string]any, len(d)+len(s))
		for k, v := range d {
			out[k] = cloneValue(v)
		}
		for k, sv := range s {
			dv, present := out[k]
			if !present {
				out[k] = cloneValue(sv)
				continue
			}
			out[k] = mergeValue(dv, sv, ruleName, key, listMode)
		}
		return out
	case []any:
		// Honor the rule+key declared list merge mode.
		mode := rule.ListReplace
		if listMode != nil {
			mode = listMode(ruleName, key)
		}
		if mode == rule.ListAppend {
			d, ok := dst.([]any)
			if !ok {
				return cloneList(s)
			}
			out := make([]any, 0, len(d)+len(s))
			out = append(out, d...)
			out = append(out, cloneList(s)...)
			return out
		}
		return cloneList(s)
	default:
		return src
	}
}

// cloneValue returns a deep-ish copy of v. Maps and slices are copied
// recursively; everything else is returned by value.
func cloneValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, vv := range t {
			out[k] = cloneValue(vv)
		}
		return out
	case []any:
		return cloneList(t)
	default:
		return v
	}
}

func cloneList(in []any) []any {
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = cloneValue(v)
	}
	return out
}

// EffectiveWithProvenance returns the effective rule configuration for
// a file path along with a leaf-level provenance map. The provenance
// records, for each rule, which layer contributed the final value of
// each leaf setting key (and the special "_enabled" pseudo-key for the
// boolean Enabled flag).
//
// Provenance is built by replaying the same layer chain used by
// Effective: defaults, kinds in effective order, then matching glob
// overrides. The latest layer to touch a leaf wins; for append lists
// the layer recorded is the latest one to contribute elements.
func EffectiveWithProvenance(
	cfg *Config, filePath string, fmKinds []string,
) (map[string]RuleCfg, map[string]map[string]LayerSource) {
	kinds := resolveEffectiveKinds(cfg, filePath, fmKinds)
	rules, prov := effectiveRulesWithProvenance(cfg, filePath, kinds)
	return rules, prov
}

// effectiveRulesWithProvenance is the deep-merge core: it walks the
// layer chain (defaults, kinds, overrides) and, for each layer that
// touches a rule, deep-merges the rule body onto the accumulator.
// Provenance is recorded per leaf as the most recent contributing
// layer.
func effectiveRulesWithProvenance(
	cfg *Config, filePath string, kinds []string,
) (map[string]RuleCfg, map[string]map[string]LayerSource) {
	result := make(map[string]RuleCfg, len(cfg.Rules))
	prov := make(map[string]map[string]LayerSource, len(cfg.Rules))

	apply := func(name string, body RuleCfg, src LayerSource) {
		dst, exists := result[name]
		if !exists {
			dst = RuleCfg{}
		}
		merged := deepMergeRuleCfg(dst, body, name, defaultListMode)
		result[name] = merged
		recordProvenance(prov, name, dst, body, merged, src)
	}

	for k, v := range cfg.Rules {
		apply(k, v, LayerSource{Layer: LayerDefault})
	}
	for _, kindName := range kinds {
		body, ok := cfg.Kinds[kindName]
		if !ok {
			continue
		}
		for k, v := range body.Rules {
			apply(k, v, LayerSource{Layer: LayerKind, KindName: kindName})
		}
	}
	for i, o := range cfg.Overrides {
		if !matchesAny(o.Files, filePath) {
			continue
		}
		for k, v := range o.Rules {
			apply(k, v, LayerSource{Layer: LayerOverride, OverrideIndex: i})
		}
	}
	return result, prov
}

// recordProvenance updates prov[ruleName] with a LayerSource for every
// leaf that the most recent layer touched. Leaves are tracked at the
// top level of Settings only; a nested map on the right-hand side is
// recorded as a single leaf at the top key (deep-merged maps still
// trace back to whichever layer set the outer key last).
func recordProvenance(
	prov map[string]map[string]LayerSource,
	ruleName string,
	prev, layer, merged RuleCfg,
	src LayerSource,
) {
	leaves, ok := prov[ruleName]
	if !ok {
		leaves = make(map[string]LayerSource)
		prov[ruleName] = leaves
	}
	// Track Enabled changes separately under the "_enabled" pseudo-key.
	if prev.Enabled != merged.Enabled || !ok {
		leaves["_enabled"] = src
	}
	for k := range layer.Settings {
		leaves[k] = src
	}
}
