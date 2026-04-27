package config

import "github.com/jeduden/mdsmith/internal/rule"

// MergeMode is a re-export of rule.MergeMode for callers within this
// package; it avoids forcing every test/file to import the rule package
// just to spell the constants. The values are identical.
type MergeMode = rule.MergeMode

// Re-exported merge mode constants. See rule.MergeMode for semantics.
const (
	MergeReplace = rule.MergeReplace
	MergeAppend  = rule.MergeAppend
)

// deepMergeRule combines two RuleCfg layers into one. The later layer
// (over) wins on every leaf it touches; settings absent from over are
// inherited from base. The modes map declares which list-valued
// settings concatenate (MergeAppend); all other lists replace.
//
// Enabled is taken from over when over has any settings or when over
// explicitly sets Enabled=false. A later layer that disables the rule
// preserves earlier Settings (so a category-disable does not erase
// configuration that the rule may still need on re-enable elsewhere).
func deepMergeRule(base, over RuleCfg, modes map[string]MergeMode) RuleCfg {
	// Determine effective Enabled. A layer that omits Settings and is
	// disabled (Enabled=false, Settings=nil) is a "disable only" layer;
	// it flips Enabled but does not erase earlier Settings.
	enabled := base.Enabled
	if over.Settings != nil || !over.Enabled {
		enabled = over.Enabled
	}

	// Merge settings.
	merged := mergeSettingsMap(base.Settings, over.Settings, modes)

	return RuleCfg{Enabled: enabled, Settings: merged}
}

// mergeSettingsMap deep-merges two settings maps. Keys present only in
// one map carry through. For shared keys the later value wins, except
// that shared maps recurse and shared lists honor the modes lookup.
func mergeSettingsMap(base, over map[string]any, modes map[string]MergeMode) map[string]any {
	if base == nil && over == nil {
		return nil
	}
	out := make(map[string]any, len(base)+len(over))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		if existing, ok := out[k]; ok {
			out[k] = mergeValue(k, existing, v, modes)
		} else {
			out[k] = v
		}
	}
	return out
}

// mergeValue combines two values for the same key. Maps recurse with no
// merge-mode lookup (nested keys do not inherit the parent's mode);
// lists honor the modes table for the current key; everything else is
// replaced by the later value.
func mergeValue(key string, base, over any, modes map[string]MergeMode) any {
	// Map merge: only when both sides are maps with string keys.
	if bm, ok := toStringMap(base); ok {
		if om, ok := toStringMap(over); ok {
			// Nested maps do not propagate merge modes — only top-level
			// rule settings declare modes today. Pass nil so nested
			// lists fall back to MergeReplace.
			return mergeSettingsMap(bm, om, nil)
		}
	}

	// List merge.
	if bl, ok := toAnyList(base); ok {
		if ol, ok := toAnyList(over); ok {
			if modes[key] == MergeAppend {
				combined := make([]any, 0, len(bl)+len(ol))
				combined = append(combined, bl...)
				combined = append(combined, ol...)
				return combined
			}
			return ol
		}
	}

	// Scalar (or mismatched types) — later wins.
	return over
}

// toStringMap normalizes a value to a map[string]any if it is one.
// Maps coming back from yaml.v3's `map[string]any` decoding are already
// of this type; nested maps decoded into a plain `any` may arrive as
// map[any]any, but the rest of the code path uses the modern decoder
// path so this helper only needs to handle the canonical shape.
func toStringMap(v any) (map[string]any, bool) {
	if m, ok := v.(map[string]any); ok {
		return m, true
	}
	return nil, false
}

// toAnyList normalizes a value to an []any if it is a list. It accepts
// both []any (the YAML-decoded shape) and []string (the shape produced
// by rules that round-trip Defaults() -> ApplySettings()).
func toAnyList(v any) ([]any, bool) {
	switch t := v.(type) {
	case []any:
		return t, true
	case []string:
		out := make([]any, len(t))
		for i, s := range t {
			out[i] = s
		}
		return out, true
	}
	return nil, false
}

// mergeModesFor returns the merge-mode table for the rule with the
// given name, or nil if the rule does not implement ListMerger.
func mergeModesFor(name string) map[string]MergeMode {
	r := findRuleByName(name)
	if r == nil {
		return nil
	}
	if lm, ok := r.(rule.ListMerger); ok {
		return lm.MergeModes()
	}
	return nil
}

// findRuleByName scans the rule registry for a rule with the given
// Name. Returns nil if no rule matches. The registry is small (one
// entry per rule), so a linear scan is fine.
func findRuleByName(name string) rule.Rule {
	for _, r := range rule.All() {
		if r.Name() == name {
			return r
		}
	}
	return nil
}
