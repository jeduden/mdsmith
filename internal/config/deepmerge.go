package config

// MergeMode controls how a per-rule list setting combines across config
// layers (defaults → kinds → overrides). Map and scalar settings always
// merge by recursive replace; only list settings can opt in to append.
type MergeMode int

const (
	// MergeReplace — the later layer's value replaces the earlier one
	// wholesale. This is the default for any setting.
	MergeReplace MergeMode = iota
	// MergeAppend — a list setting concatenates the later layer's
	// value onto the earlier one. Only meaningful for list values.
	MergeAppend
)

// deepMergeRuleCfg returns the result of deep-merging layer onto base.
//
// Semantics:
//   - If layer.Enabled is false and layer.Settings is nil, the rule is
//     disabled and any prior settings are dropped.
//   - If layer.Settings is nil and layer.Enabled is true, base settings
//     are preserved (a bare `true` only flips Enabled).
//   - If layer.Settings is non-nil, each key in layer is merged into the
//     accumulator: maps merge recursively, scalars replace, lists follow
//     the per-key MergeMode (default replace).
//
// modes maps a rule's setting key to its merge mode. Unlisted keys use
// MergeReplace. Pass nil for the default (everything replaces).
func deepMergeRuleCfg(base, layer RuleCfg, modes map[string]MergeMode) RuleCfg {
	// Disabled layer with no settings = full reset.
	if !layer.Enabled && layer.Settings == nil {
		return RuleCfg{Enabled: false}
	}
	out := RuleCfg{Enabled: layer.Enabled}
	// Copy base settings (so we don't mutate the source).
	if base.Settings != nil {
		out.Settings = make(map[string]any, len(base.Settings))
		for k, v := range base.Settings {
			out.Settings[k] = v
		}
	}
	if layer.Settings == nil {
		return out
	}
	if out.Settings == nil {
		out.Settings = make(map[string]any, len(layer.Settings))
	}
	for k, lv := range layer.Settings {
		bv, present := out.Settings[k]
		if !present {
			out.Settings[k] = lv
			continue
		}
		out.Settings[k] = mergeValues(bv, lv, modes[k])
	}
	return out
}

// mergeValues merges layer onto base for a single setting value.
// Maps recurse, lists follow mode, anything else replaces.
func mergeValues(base, layer any, mode MergeMode) any {
	// Map recursion: only when both sides are maps.
	if bm, ok := base.(map[string]any); ok {
		if lm, ok := layer.(map[string]any); ok {
			return mergeMaps(bm, lm)
		}
	}
	if mode == MergeAppend {
		if merged, ok := mergeAppendLists(base, layer); ok {
			return merged
		}
	}
	return layer
}

// mergeMaps deep-merges two map[string]any. Sub-merge modes are not
// propagated: list settings inside nested maps always replace. Rules that
// need append-mode for nested lists can be extended later.
func mergeMaps(base, layer map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(layer))
	for k, v := range base {
		out[k] = v
	}
	for k, lv := range layer {
		bv, present := out[k]
		if !present {
			out[k] = lv
			continue
		}
		out[k] = mergeValues(bv, lv, MergeReplace)
	}
	return out
}

// mergeAppendLists concatenates two list values. Returns (result, true)
// when both base and layer are list-like; otherwise (nil, false) so the
// caller can fall back to replace.
func mergeAppendLists(base, layer any) (any, bool) {
	bs, baseOK := toAnySlice(base)
	ls, layerOK := toAnySlice(layer)
	if !baseOK || !layerOK {
		return nil, false
	}
	out := make([]any, 0, len(bs)+len(ls))
	out = append(out, bs...)
	out = append(out, ls...)
	return out, true
}

// toAnySlice normalizes a YAML-loaded list to []any. It accepts either
// []any (the typical YAML decode) or []string (typical when produced by
// a rule's DefaultSettings). Other types return false.
func toAnySlice(v any) ([]any, bool) {
	switch s := v.(type) {
	case []any:
		return s, true
	case []string:
		out := make([]any, len(s))
		for i, x := range s {
			out[i] = x
		}
		return out, true
	}
	return nil, false
}
