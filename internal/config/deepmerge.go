package config

import "github.com/jeduden/mdsmith/internal/rule"

// mergeRuleCfg deep-merges later on top of earlier and returns the
// result. Both layers configure the same rule; later sits after earlier
// in the layer chain (default → kinds in effective-list order →
// matching overrides).
//
// Semantics:
//
//   - Enabled: later's value wins.
//   - Settings: maps recurse key by key; scalars at a leaf are replaced
//     by later; lists are replaced by default and appended only when
//     the rule declares the key as MergeAppend via rule.ListMerger.
//   - A bool-only later layer (Settings == nil) toggles Enabled but
//     preserves earlier's Settings, so a kind that says
//     `line-length: false` does not erase a previously inherited
//     `max:` setting.
func mergeRuleCfg(ruleName string, earlier, later RuleCfg) RuleCfg {
	out := RuleCfg{Enabled: later.Enabled}
	switch {
	case later.Settings == nil && earlier.Settings == nil:
		// Nothing to merge.
	case later.Settings == nil:
		out.Settings = cloneSettings(earlier.Settings)
	case earlier.Settings == nil:
		out.Settings = cloneSettings(later.Settings)
	default:
		out.Settings = mergeSettingsMap(ruleName, earlier.Settings, later.Settings)
	}
	return out
}

// mergeSettingsMap deep-merges later onto earlier for a settings map.
func mergeSettingsMap(ruleName string, earlier, later map[string]any) map[string]any {
	out := make(map[string]any, len(earlier)+len(later))
	for k, v := range earlier {
		out[k] = cloneAny(v)
	}
	for k, lv := range later {
		ev, present := out[k]
		if !present {
			out[k] = cloneAny(lv)
			continue
		}
		out[k] = mergeAny(ruleName, k, ev, lv)
	}
	return out
}

// mergeAny merges later onto earlier for a single settings leaf or
// nested value. Maps recurse, lists honor the rule's declared merge
// mode, and everything else is replaced wholesale by later.
func mergeAny(ruleName, key string, earlier, later any) any {
	if em, ok := earlier.(map[string]any); ok {
		if lm, ok := later.(map[string]any); ok {
			return mergeSettingsMap(ruleName, em, lm)
		}
	}
	if el, ok := toAnySlice(earlier); ok {
		if ll, ok := toAnySlice(later); ok {
			if settingMergeMode(ruleName, key) == rule.MergeAppend {
				merged := make([]any, 0, len(el)+len(ll))
				merged = append(merged, el...)
				merged = append(merged, ll...)
				return merged
			}
			return append([]any(nil), ll...)
		}
	}
	return cloneAny(later)
}

// settingMergeMode returns the merge mode for a list-typed rule
// setting, defaulting to rule.MergeReplace when the rule does not
// implement rule.ListMerger or is not registered.
func settingMergeMode(ruleName, settingKey string) rule.MergeMode {
	r := rule.ByName(ruleName)
	if r == nil {
		return rule.MergeReplace
	}
	lm, ok := r.(rule.ListMerger)
	if !ok {
		return rule.MergeReplace
	}
	return lm.SettingMergeMode(settingKey)
}

// toAnySlice normalizes the common slice types found in YAML-decoded or
// programmatically constructed RuleCfg.Settings into []any. It returns
// false for non-slice values.
func toAnySlice(v any) ([]any, bool) {
	switch x := v.(type) {
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = cloneAny(e)
		}
		return out, true
	case []string:
		out := make([]any, len(x))
		for i, s := range x {
			out[i] = s
		}
		return out, true
	case []int:
		out := make([]any, len(x))
		for i, n := range x {
			out[i] = n
		}
		return out, true
	}
	return nil, false
}

// cloneSettings returns a deep copy of a settings map, isolating the
// caller from mutations to nested maps and slices.
func cloneSettings(s map[string]any) map[string]any {
	if s == nil {
		return nil
	}
	out := make(map[string]any, len(s))
	for k, v := range s {
		out[k] = cloneAny(v)
	}
	return out
}

// cloneAny deep-copies maps and slices recursively. Scalars and unknown
// types are returned as-is.
func cloneAny(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, vv := range x {
			out[k] = cloneAny(vv)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = cloneAny(e)
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
	default:
		return v
	}
}
