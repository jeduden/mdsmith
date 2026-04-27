package config

import (
	"github.com/jeduden/mdsmith/internal/rule"
)

// MergeModeFunc returns the list merge mode for a (rule, setting key)
// pair. Defaults to rule.ListReplace when no rule has opted in.
type MergeModeFunc func(ruleName, key string) rule.ListMergeMode

// defaultMergeModes consults the rule registry for the named rule and
// asks if it implements MergeModes. Used by Effective when the caller
// does not supply its own resolver.
func defaultMergeModes(ruleName, key string) rule.ListMergeMode {
	r := ruleByName(ruleName)
	if r == nil {
		return rule.ListReplace
	}
	mm, ok := r.(rule.MergeModes)
	if !ok {
		return rule.ListReplace
	}
	modes := mm.ListMergeModes()
	if modes == nil {
		return rule.ListReplace
	}
	if mode, ok := modes[key]; ok {
		return mode
	}
	return rule.ListReplace
}

// ruleByName scans the rule registry for a rule whose Name() matches
// the given string. Returns nil if no such rule is registered. The
// registry is keyed by ID; this helper does a linear scan over names.
func ruleByName(name string) rule.Rule {
	for _, r := range rule.All() {
		if r.Name() == name {
			return r
		}
	}
	return nil
}

// deepMergeRuleCfg merges layer onto base and returns the result.
//
// Semantics:
//   - Enabled is replaced by the layer's value (a layer always
//     declares its enable bit when it touches a rule).
//   - Settings is merged key-by-key:
//   - shared map keys recurse,
//   - scalars are replaced by the later layer's value,
//   - lists default to replace; opt into append via mergeMode.
//
// If layer.Settings is nil the merge replaces only Enabled — the prior
// Settings map survives intact, so a layer that simply disables a
// rule does not erase sibling settings established earlier.
func deepMergeRuleCfg(
	ruleName string, base, layer RuleCfg, mergeMode MergeModeFunc,
) RuleCfg {
	result := RuleCfg{
		Enabled:  layer.Enabled,
		Settings: copyAnyMap(base.Settings),
	}
	if layer.Settings == nil {
		return result
	}
	if result.Settings == nil {
		result.Settings = make(map[string]any, len(layer.Settings))
	}
	for k, v := range layer.Settings {
		result.Settings[k] = deepMergeValue(
			ruleName, k, result.Settings[k], v, mergeMode,
		)
	}
	return result
}

// deepMergeValue merges layer onto base for one settings entry.
// The (ruleName, key) pair is forwarded so list-typed values can ask
// the merge-mode resolver whether to append or replace.
func deepMergeValue(
	ruleName, key string, base, layer any, mergeMode MergeModeFunc,
) any {
	// Map/map → recurse.
	bm, baseIsMap := base.(map[string]any)
	lm, layerIsMap := layer.(map[string]any)
	if baseIsMap && layerIsMap {
		out := make(map[string]any, len(bm)+len(lm))
		for k, v := range bm {
			out[k] = v
		}
		for k, v := range lm {
			out[k] = deepMergeValue(ruleName, key, out[k], v, mergeMode)
		}
		return out
	}

	// List/list → mode-dependent.
	bs, baseIsList := base.([]any)
	ls, layerIsList := layer.([]any)
	if baseIsList && layerIsList {
		mode := rule.ListReplace
		if mergeMode != nil {
			mode = mergeMode(ruleName, key)
		}
		switch mode {
		case rule.ListAppend:
			out := make([]any, 0, len(bs)+len(ls))
			out = append(out, bs...)
			out = append(out, ls...)
			return out
		default:
			out := make([]any, len(ls))
			copy(out, ls)
			return out
		}
	}

	// Type mismatch or scalar → replace.
	return layer
}

// copyAnyMap returns a shallow copy of an any-valued map. Map and list
// values are themselves copied one level deep so the merge result does
// not alias the source.
func copyAnyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = copyAnyValue(v)
	}
	return out
}

// copyAnyValue copies a settings value one level deep. Maps and lists
// are duplicated; scalars are returned as-is.
func copyAnyValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		return copyAnyMap(x)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = copyAnyValue(e)
		}
		return out
	default:
		return v
	}
}
