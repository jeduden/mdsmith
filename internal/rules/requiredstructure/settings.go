package requiredstructure

import (
	"fmt"

	"github.com/jeduden/mdsmith/internal/placeholders"
	"github.com/jeduden/mdsmith/internal/rule"
	rulesettings "github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/jeduden/mdsmith/internal/schema"
)

// SchemaSource is one entry in the rule's schema-sources list. Either
// File or Inline is set, never both. Inline schemas are pre-parsed at
// ApplySettings time so a malformed schema surfaces as a config-load
// error rather than a per-file diagnostic at Check time. File sources
// stay as paths because the rule reads them through the lint.File's
// RootFS at Check time.
type SchemaSource struct {
	File   string
	Inline *schema.Schema
}

// ApplySettings implements rule.Configurable.
//
// Three input shapes are accepted, all collapsed into Sources:
//
//   - `schema-sources` (canonical, set by the merge layer): a list of
//     {file: path} / {inline: map} entries in source order.
//   - `schema` (legacy single-source): a file path; equivalent to a
//     one-entry schema-sources list.
//   - `inline-schema` (legacy single-source): a YAML map; equivalent
//     to a one-entry inline schema-sources list.
//
// When called via the merge layer the rule sees only schema-sources;
// the legacy keys are retained for tests and direct callers. Mixing
// `schema` and `inline-schema` in the same settings call is rejected
// as before — the merge layer only produces schema-sources, so the
// guard fires only on hand-authored configs.
func (r *Rule) ApplySettings(settings map[string]any) error {
	if err := rejectDualSchemaSettings(settings); err != nil {
		return err
	}
	r.Sources = nil
	for k, v := range settings {
		if err := r.applySetting(k, v); err != nil {
			return err
		}
	}
	return nil
}

func (r *Rule) applySetting(key string, value any) error {
	switch key {
	case "schema":
		return r.applySchemaSetting(value)
	case "inline-schema":
		return r.applyInlineSchemaSetting(value)
	case "schema-sources":
		return r.applySchemaSourcesSetting(value)
	case "placeholders":
		return r.applyPlaceholdersSetting(value)
	case "path-patterns":
		pp, err := parsePathPatterns(value)
		if err != nil {
			return fmt.Errorf("required-structure: %w", err)
		}
		r.PathPatterns = pp
		return nil
	case "archetype", "archetype-roots":
		return fmt.Errorf(
			"required-structure: setting %q has been removed; "+
				"use `schema:` with an explicit path, or declare a kind "+
				"under `kinds:` — see docs/guides/file-kinds.md", key)
	default:
		return fmt.Errorf("required-structure: unknown setting %q", key)
	}
}

func (r *Rule) applySchemaSetting(v any) error {
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("required-structure: schema must be a string, got %T", v)
	}
	if s == "" {
		return nil
	}
	if isLikelyArchetypeName(s) {
		return fmt.Errorf(
			"required-structure: schema %q looks like a bare name; "+
				"name-based lookup has been removed — set `schema:` to "+
				"an explicit path (e.g. schemas/%s.md), or declare a "+
				"kind under `kinds:` — see docs/guides/file-kinds.md", s, s)
	}
	r.Schema = s
	r.Sources = append(r.Sources, SchemaSource{File: s})
	return nil
}

func (r *Rule) applyInlineSchemaSetting(v any) error {
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf(
			"required-structure: inline-schema must be a mapping, got %T", v)
	}
	if len(m) == 0 {
		return nil
	}
	sch, err := schema.ParseInline(m, "inline kind schema")
	if err != nil {
		return fmt.Errorf("required-structure: invalid inline-schema: %w", err)
	}
	r.InlineSchema = sch
	r.Sources = append(r.Sources, SchemaSource{Inline: sch})
	return nil
}

func (r *Rule) applySchemaSourcesSetting(v any) error {
	sources, err := parseSchemaSources(v)
	if err != nil {
		return fmt.Errorf("required-structure: %w", err)
	}
	r.Sources = append(r.Sources, sources...)
	r.reflectSingleSource()
	return nil
}

func (r *Rule) applyPlaceholdersSetting(v any) error {
	toks, ok := rulesettings.ToStringSlice(v)
	if !ok {
		return fmt.Errorf(
			"required-structure: placeholders must be a list of strings, got %T", v,
		)
	}
	if err := placeholders.Validate(toks); err != nil {
		return fmt.Errorf("required-structure: %w", err)
	}
	r.Placeholders = toks
	return nil
}

// reflectSingleSource keeps Schema and InlineSchema in sync with
// Sources when exactly one source is configured. Multi-source
// configs leave both as their previous values — callers must read
// Sources to enumerate the list.
func (r *Rule) reflectSingleSource() {
	if len(r.Sources) != 1 {
		return
	}
	switch {
	case r.Sources[0].File != "":
		r.Schema = r.Sources[0].File
		r.InlineSchema = nil
	case r.Sources[0].Inline != nil:
		r.InlineSchema = r.Sources[0].Inline
		r.Schema = ""
	}
}

// parseSchemaSources reads the `schema-sources` rule setting: a list
// of `{file: path}` / `{inline: map}` entries installed by the merge
// layer. Inline maps are parsed eagerly so a malformed schema fails
// at config-load time rather than per file at Check time.
func parseSchemaSources(v any) ([]SchemaSource, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf(
			"schema-sources must be a list of {file|inline} entries, got %T", v)
	}
	out := make([]SchemaSource, 0, len(list))
	for i, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("schema-sources[%d]: entry must be a map, got %T", i, item)
		}
		filePath, hasFile := m["file"]
		inlineV, hasInline := m["inline"]
		if hasFile && hasInline {
			return nil, fmt.Errorf(
				"schema-sources[%d]: entry may set only one of `file` or `inline`", i)
		}
		switch {
		case hasFile:
			fp, ok := filePath.(string)
			if !ok || fp == "" {
				return nil, fmt.Errorf(
					"schema-sources[%d].file must be a non-empty string, got %T", i, filePath)
			}
			out = append(out, SchemaSource{File: fp})
		case hasInline:
			im, ok := inlineV.(map[string]any)
			if !ok || len(im) == 0 {
				return nil, fmt.Errorf(
					"schema-sources[%d].inline must be a non-empty mapping, got %T", i, inlineV)
			}
			// The merge layer (config.applyInlineSchemaSource) attaches
			// the kind's defining file as `source` so the schema
			// reference is navigable; fall back to the generic label
			// when it is absent (e.g. a direct `inline-schema:` setting).
			label := "inline kind schema"
			if src, ok := m["source"].(string); ok && src != "" {
				label = src
			}
			sch, err := schema.ParseInline(im, label)
			if err != nil {
				return nil, fmt.Errorf("schema-sources[%d].inline: %w", i, err)
			}
			out = append(out, SchemaSource{Inline: sch})
		default:
			return nil, fmt.Errorf(
				"schema-sources[%d]: entry must set `file` or `inline`", i)
		}
	}
	return out, nil
}

// rejectDualSchemaSettings refuses a settings map that supplies both
// `schema` (file path) and `inline-schema` (inline map). The merge
// layer clears the prior source when a later layer installs a new
// one, so the rule normally sees only one — this guard catches the
// case where a single config layer lists both.
func rejectDualSchemaSettings(settings map[string]any) error {
	pathV, hasPath := settings["schema"]
	mapV, hasInline := settings["inline-schema"]
	if !hasPath || !hasInline {
		return nil
	}
	path, _ := pathV.(string)
	inline, _ := mapV.(map[string]any)
	if path == "" || len(inline) == 0 {
		return nil
	}
	return fmt.Errorf(
		"required-structure: cannot set both `schema` (%q) and "+
			"`inline-schema` on the same layer — pick one source",
		path)
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"schema":       "",
		"placeholders": []string{},
	}
}

// SettingMergeMode implements rule.ListMerger.
func (r *Rule) SettingMergeMode(key string) rule.MergeMode {
	if key == "placeholders" {
		return rule.MergeAppend
	}
	if key == "path-patterns" {
		return rule.MergeAppend
	}
	if key == "schema-sources" {
		return rule.MergeAppend
	}
	return rule.MergeReplace
}

// TranslateLayerSettings implements rule.SettingsTranslator. It
// collapses one config layer's user-facing `schema:` (file path)
// or `inline-schema:` (map) keys into a single-entry
// `schema-sources` list and strips the legacy keys. Because the
// rule declares `schema-sources` as MergeAppend, layers that pass
// through deep-merge then accumulate their sources instead of
// scalar-replacing the previous layer — which is what lets a file
// resolving to several kinds compose every kind's schema
// (plan 156).
//
// Empty values (`schema: ""`, `inline-schema: {}`) are stripped
// without contributing a source, so the rule's own DefaultSettings
// `schema: ""` placeholder never pollutes the composed list. The
// input map is treated as read-only; a new map is returned only
// when a legacy key is present.
func (r *Rule) TranslateLayerSettings(settings map[string]any) map[string]any {
	// A single layer that sets BOTH a non-empty `schema:` and a
	// non-empty `inline-schema:` is a config error. Pass the layer
	// through untouched so the keys survive deep-merge and the
	// rule's own rejectDualSchemaSettings (run from ApplySettings)
	// still surfaces the original error — stripping them here would
	// silently drop the inline source. Cross-layer composition is
	// unaffected: this only fires when one map carries both.
	if hasDualSchemaSource(settings) {
		return settings
	}
	source, hadKey := extractSchemaSourceFromSettings(settings)
	if !hadKey {
		return settings
	}
	out := cloneSettingsDeep(settings)
	delete(out, "schema")
	delete(out, "inline-schema")
	if source != nil {
		existing, _ := out["schema-sources"].([]any)
		out["schema-sources"] = append(existing, source)
	}
	return out
}

// hasDualSchemaSource reports whether one settings map sets both a
// non-empty `schema:` path and a non-empty `inline-schema:` map.
// It mirrors rejectDualSchemaSettings' non-empty semantics so the
// translator and the rule's guard agree on what counts as a
// dual-source layer.
func hasDualSchemaSource(s map[string]any) bool {
	path, _ := s["schema"].(string)
	inline, _ := s["inline-schema"].(map[string]any)
	return path != "" && len(inline) > 0
}

// extractSchemaSourceFromSettings inspects a settings map for a
// schema-source declaration. It returns (source, true) when either
// legacy key is present — even if the value is empty / no-op — so
// the caller strips the key; (nil, false) means no schema key
// appears at all and the settings pass through untouched.
func extractSchemaSourceFromSettings(s map[string]any) (any, bool) {
	hadKey := false
	if v, ok := s["schema"]; ok {
		hadKey = true
		if path, ok := v.(string); ok && path != "" {
			return map[string]any{"file": path}, true
		}
	}
	if v, ok := s["inline-schema"]; ok {
		hadKey = true
		if m, ok := v.(map[string]any); ok && len(m) > 0 {
			return map[string]any{"inline": cloneSettingsDeep(m)}, true
		}
	}
	if !hadKey {
		return nil, false
	}
	return nil, true
}

// cloneSettingsDeep deep-copies a settings map so a translated
// layer never aliases the caller's nested maps or slices.
func cloneSettingsDeep(s map[string]any) map[string]any {
	if s == nil {
		return nil
	}
	out := make(map[string]any, len(s))
	for k, v := range s {
		out[k] = cloneSettingsValue(v)
	}
	return out
}

func cloneSettingsValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, vv := range x {
			out[k] = cloneSettingsValue(vv)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = cloneSettingsValue(e)
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
