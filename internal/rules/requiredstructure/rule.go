package requiredstructure

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/fieldinterp"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/piparser"
	"github.com/jeduden/mdsmith/internal/placeholders"
	"github.com/jeduden/mdsmith/internal/rule"
	rulesettings "github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/jeduden/mdsmith/internal/yamlutil"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"gopkg.in/yaml.v3"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that a document's heading structure matches a schema.
//
// A rule instance carries an ordered list of schema sources (Sources)
// — one per layer (kind, override, or top-level rule entry) that
// declared a `schema:` (file) or `inline-schema:` (inline map). The
// rule loads each source at Check time and composes them via
// schema.Compose; a file resolving to multiple kinds therefore layers
// each kind's constraints rather than letting the last one win.
//
// Schema and InlineSchema mirror the first source's parsed form when
// exactly one source is present. They support tests that drive the
// rule directly through ApplySettings with the legacy single-source
// keys; the kind-level loader still rejects configurations that set
// both keys on the same layer.
type Rule struct {
	Schema       string         // first source's file path (single-source convenience)
	InlineSchema *schema.Schema // first source's parsed inline schema
	Sources      []SchemaSource // ordered list of schema sources (canonical)
	Placeholders []string       // placeholder tokens to treat as opaque
	PathPatterns []PathPattern  // kind-level path-pattern entries
}

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

// PathPattern records a kind's `path-pattern:` constraint: the kind
// that declared it and the glob the workspace-relative path of every
// file in the kind must match. Populated by the config merge layer
// from KindBody.PathPattern.
type PathPattern struct {
	Kind    string
	Pattern string
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS020" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "required-structure" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "structural" }

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

// isSchemaFile reports whether f is the rule's configured schema
// file. The compose code path uses isSchemaFileAt against an
// explicit path; this helper preserves the original single-source
// convenience for callers and tests.
func (r *Rule) isSchemaFile(f *lint.File) bool {
	return r.isSchemaFileAt(f, r.Schema)
}

// isLikelyArchetypeName reports whether s looks like a bare archetype
// name (a single identifier with no path separator and no file
// extension), which is the most common migration mistake when moving
// from `archetype:` to `schema:`.
func isLikelyArchetypeName(s string) bool {
	if s == "" {
		return false
	}
	if strings.ContainsAny(s, "/\\") {
		return false
	}
	return filepath.Ext(s) == ""
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

// parsePathPatterns reads the `path-patterns` rule setting: a list of
// {kind, pattern} maps installed by the config merge layer from each
// kind's `path-pattern:` field. The merge layer is the only documented
// producer; the parser still validates shape so a hand-written rule
// override fails loudly instead of silently dropping entries.
func parsePathPatterns(v any) ([]PathPattern, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf(
			"path-patterns must be a list of {kind, pattern} maps, got %T", v)
	}
	out := make([]PathPattern, 0, len(list))
	for i, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(
				"path-patterns[%d] must be a map, got %T", i, item)
		}
		kindV, hasKind := m["kind"]
		patV, hasPat := m["pattern"]
		if !hasKind || !hasPat {
			return nil, fmt.Errorf(
				"path-patterns[%d] must set both `kind` and `pattern`", i)
		}
		kind, ok := kindV.(string)
		if !ok || kind == "" {
			return nil, fmt.Errorf(
				"path-patterns[%d].kind must be a non-empty string, got %T", i, kindV)
		}
		pat, ok := patV.(string)
		if !ok || pat == "" {
			return nil, fmt.Errorf(
				"path-patterns[%d].pattern must be a non-empty string, got %T", i, patV)
		}
		// Validate the pattern as a doublestar glob at config time
		// so an unmatched bracket or other syntax error surfaces as
		// a config error instead of an MDS020 diagnostic on every
		// file assigned to the kind.
		if !doublestar.ValidatePattern(filepath.ToSlash(pat)) {
			return nil, fmt.Errorf(
				"path-patterns[%d].pattern %q is not a valid doublestar glob",
				i, pat)
		}
		out = append(out, PathPattern{Kind: kind, Pattern: pat})
	}
	return out, nil
}

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic

	// Warn when <?require?> appears in a non-schema file.
	if reqLine := findRequireDirectiveLine(f); reqLine > 0 {
		if !r.isAnySchemaFile(f) {
			d := makeDiag(f.Path, reqLine,
				"<?require?> is only recognized in schema files; this directive has no effect here")
			d.Severity = lint.Warning
			diags = append(diags, d)
		}
	}

	// Kind-level path-pattern constraints run independently of the
	// schema source: a kind may declare `path-pattern:` without an
	// attached schema, and a schema-bearing kind may add a pattern
	// on top of a `<?require filename:?>` directive.
	diags = append(diags, r.checkPathPatterns(f)...)

	sources := r.effectiveSources()
	if len(sources) == 0 {
		return diags
	}

	// Single-source: use the legacy paths so file schemas keep their
	// heading- and body-sync features (`# {id}: {name}`, body lines
	// under Meta-Information). Composition is irrelevant when only
	// one source is configured.
	//
	// Exception: when the file source declares `extends:`, route
	// through the multi-source compose path so plan-135 inheritance
	// applies. The legacy parser does not implement extends; without
	// this re-route the parent's constraints would silently drop.
	// Body-sync still runs through the per-source bodySyncDiagnostics
	// helper inside checkComposedSources, so the child's `{field}`
	// template features survive the switch.
	if len(sources) == 1 {
		src := sources[0]
		if src.Inline != nil {
			return append(diags, r.checkSingleInlineSchema(f, src.Inline)...)
		}
		if src.File != "" {
			return append(diags, r.dispatchSingleFileSchema(f, src.File, sources)...)
		}
		return diags
	}

	return append(diags, r.checkComposedSources(f, sources)...)
}

// dispatchSingleFileSchema loads the schema once, peeks at the
// front matter for the reserved `extends:` key, and routes to the
// legacy single-file path or the compose path accordingly. Loading
// here avoids the double read the previous helper introduced: the
// legacy path reuses the bytes via checkSingleFileSchemaFromData,
// and the compose path re-loads through schema.ParseFile only when
// extends actually applies.
func (r *Rule) dispatchSingleFileSchema(
	f *lint.File, schemaPath string, sources []SchemaSource,
) []lint.Diagnostic {
	data, schPath, err := r.loadSchemaAt(f, schemaPath)
	if err != nil {
		return []lint.Diagnostic{r.diag(f.Path, 1, err.Error())}
	}
	if schemaDataDeclaresExtends(data) {
		return r.checkComposedSources(f, sources)
	}
	return r.checkSingleFileSchemaFromData(f, schemaPath, data, schPath)
}

// schemaDataDeclaresExtends reports whether the raw schema bytes
// carry a reserved `extends:` key in their YAML front matter. The
// modern `schema.ParseFile` pipeline rejects malformed `extends:`
// values (non-string, empty/whitespace string) with clear errors;
// routing here on any present-and-non-null `extends:` lets those
// errors surface to the user instead of being silently swallowed
// by the legacy parser. An explicit YAML `null` matches the
// no-extends case so legacy diagnostics line up.
//
// A parse failure or missing front matter returns false — the
// legacy parser then surfaces those errors with its existing
// diagnostic shape.
func schemaDataDeclaresExtends(data []byte) bool {
	prefix, _ := lint.StripFrontMatter(data)
	if prefix == nil {
		return false
	}
	yamlBytes := extractYAML(prefix)
	var raw map[string]any
	if err := yamlutil.UnmarshalSafe(yamlBytes, &raw); err != nil {
		return false
	}
	v, ok := raw["extends"]
	if !ok {
		return false
	}
	// Explicit YAML `null` matches schema.ParseFile's no-extends
	// treatment; stay on the legacy path so diagnostics align.
	// Every other value (non-empty string, malformed string,
	// non-string type) routes to the compose path so the modern
	// parser can surface its specific error.
	return v != nil
}

// effectiveSources returns the rule's sources list, falling back to
// a single-entry list built from the legacy Schema / InlineSchema
// fields when Sources is empty. This lets tests drive the rule
// directly with the older fields while still routing through the
// new multi-source code path.
func (r *Rule) effectiveSources() []SchemaSource {
	if len(r.Sources) > 0 {
		return r.Sources
	}
	if r.InlineSchema != nil && !r.InlineSchema.IsEmpty() {
		return []SchemaSource{{Inline: r.InlineSchema}}
	}
	if r.Schema != "" {
		return []SchemaSource{{File: r.Schema}}
	}
	return nil
}

// isAnySchemaFile reports whether f matches any of the configured
// file sources. When a file plays the role of its own schema (e.g.
// rule-readme's proto.md), the warning-on-misplaced-<?require?>
// check must skip it.
func (r *Rule) isAnySchemaFile(f *lint.File) bool {
	for _, src := range r.effectiveSources() {
		if src.File == "" {
			continue
		}
		if r.isSchemaFileAt(f, src.File) {
			return true
		}
	}
	return false
}

// checkSingleInlineSchema runs the validator against a single inline
// schema. Inline schemas do not support frontmatter-body {field}
// sync (no source body content) so the legacy syncPoints code path
// is skipped.
func (r *Rule) checkSingleInlineSchema(f *lint.File, sch *schema.Schema) []lint.Diagnostic {
	diags := make([]lint.Diagnostic, 0, 8)
	docFMRaw, fmDiags := readDocFrontMatterRaw(f)
	diags = append(diags, fmDiags...)
	fmIsCUE := placeholders.HasCUEFrontmatter(r.Placeholders)
	diags = append(diags, schema.Validate(f, sch, docFMRaw, fmIsCUE, makeDiag)...)
	diags = append(diags, r.applyScopeRules(f, sch, docFMRaw)...)
	diags = append(diags, schema.ValidateCrossReferences(f, sch, makeDiag)...)
	diags = append(diags, schema.ValidateAcronyms(f, sch, docFMRaw, makeDiag)...)
	diags = append(diags, schema.ValidateIndex(f, sch, makeDiag)...)
	return diags
}

// Fix implements rule.FixableRule. For single file-based schemas it
// rewrites body lines whose {field} template matches but whose value
// disagrees with the document's front matter (body-sync fix). For
// any configured inline schema (single-source or composed across
// kinds) that declares an `index:` block, Fix also emits the JSON
// side-output next to the source file. `mdsmith check` skips the
// write, preserving check's read-only contract (plan 143).
//
// Fix swallows errors (composition and WriteIndex both). WriteIndex
// itself records any I/O failure in the package-level cache keyed
// by f.Path; the next Check reads that cache and surfaces the
// underlying error in place of the generic "missing / out of date"
// message, so users are not trapped in a fix loop without signal.
// Composition errors are similarly swallowed — they re-surface on
// the next Check pass through the same checkComposedSources path.
func (r *Rule) Fix(f *lint.File) []byte {
	sch, err := r.composedSchemaForFix(f)
	if err == nil && sch != nil && !sch.IsEmpty() && sch.Index != nil {
		_ = schema.WriteIndex(f, sch)
	}
	sources := r.effectiveSources()
	if len(sources) == 1 && sources[0].File != "" && !r.isSchemaFileAt(f, sources[0].File) {
		schData, schPath, loadErr := r.loadSchemaAt(f, sources[0].File)
		if loadErr == nil {
			parsedSch, parseErr := cachedParseSchema(f, schData, schPath)
			if parseErr == nil {
				docFMRaw, _ := readDocFrontMatterRaw(f)
				return fixBodySyncIn(f, parsedSch, docFMRaw)
			}
		}
	}
	return f.Source
}

// fixBodySyncIn rewrites body lines whose {field} template matches but
// whose resolved front-matter value disagrees with the document text.
// It returns f.Source unchanged when no lines need rewriting.
func fixBodySyncIn(f *lint.File, sch *parsedSchema, docFM map[string]any) []byte {
	if len(docFM) == 0 || len(sch.SyncPoints) == 0 {
		return f.Source
	}
	docHeadings := extractHeadings(f)
	work := make([][]byte, len(f.Lines))
	copy(work, f.Lines)
	modified := false
	docIdx := 0
	for schIdx, req := range sch.Headings {
		if isSectionWildcard(req) {
			continue
		}
		syncs := sch.SyncPoints[schIdx]
		if len(syncs) == 0 {
			_, docIdx = advanceToMatch(req, docHeadings, docIdx)
			continue
		}
		matchedDoc, newIdx := advanceToMatch(req, docHeadings, docIdx)
		docIdx = newIdx
		if matchedDoc < 0 {
			continue
		}
		dh := docHeadings[matchedDoc]
		startLine := dh.Line + 1
		endLine := len(f.Lines)
		if matchedDoc+1 < len(docHeadings) {
			endLine = docHeadings[matchedDoc+1].Line - 1
		}
		for _, sp := range syncs {
			if patchedLine, ok := resolveBodySyncLine(sp, docFM, work, startLine, endLine); ok {
				work[patchedLine.idx] = patchedLine.val
				modified = true
			}
		}
	}
	if !modified {
		return f.Source
	}
	return bytes.Join(work, []byte("\n"))
}

// patchedLine carries the index and new value for a line that needs rewriting.
type patchedLine struct {
	idx int
	val []byte
}

// resolveBodySyncLine returns the line index and replacement bytes for sp
// if the document contains a stale template-match line in [startLine, endLine).
// ok is false when sp is not a body sync point, the field is missing, the
// line already matches, or no template-matching line is found.
func resolveBodySyncLine(
	sp syncPoint, docFM map[string]any,
	work [][]byte, startLine, endLine int,
) (patchedLine, bool) {
	if !sp.InBody {
		return patchedLine{}, false
	}
	path := fieldinterp.ParseCUEPath(sp.Field)
	if path == nil {
		return patchedLine{}, false
	}
	if _, err := fieldinterp.ResolvePath(docFM, path); err != nil {
		return patchedLine{}, false
	}
	expected := resolveFields(sp.BodyText, docFM)
	re := sp.compiled
	for i := startLine - 1; i < endLine && i < len(work); i++ {
		trimmed := strings.TrimSpace(string(work[i]))
		if trimmed == expected {
			continue // already correct; keep scanning for stale duplicates
		}
		if re.MatchString(trimmed) {
			orig := string(work[i])
			leadLen := len(orig) - len(strings.TrimLeft(orig, " \t"))
			return patchedLine{idx: i, val: []byte(orig[:leadLen] + expected)}, true
		}
	}
	return patchedLine{}, false
}

// buildFieldPattern compiles a regex that matches a body line whose
// {field} placeholders have been replaced by any non-empty run.
// The pattern is always valid: each part is regexp.QuoteMeta'd and
// joined with ".+", so regexp.MustCompile never panics here.
func buildFieldPattern(bodyText string) *regexp.Regexp {
	parts := fieldinterp.SplitOnFields(bodyText)
	var patBuf strings.Builder
	patBuf.WriteString("^")
	for i, part := range parts {
		patBuf.WriteString(regexp.QuoteMeta(part))
		if i < len(parts)-1 {
			patBuf.WriteString(".+")
		}
	}
	patBuf.WriteString("$")
	return regexp.MustCompile(patBuf.String())
}

// buildSchemaHeading constructs a schemaHeading from a docHeading,
// pre-compiling the field-interpolation regex when the text contains
// {field} references so matchesSchema pays no per-call compile cost.
// Pattern construction is delegated to buildFieldPattern, which owns
// the QuoteMeta+".+" logic and its MustCompile invariant.
func buildSchemaHeading(h docHeading) schemaHeading {
	sh := schemaHeading{Level: h.Level, Text: h.Text}
	if fieldinterp.ContainsField(h.Text) {
		sh.compiled = buildFieldPattern(h.Text)
	}
	return sh
}

// composedSchemaForFix returns the same composed *schema.Schema
// that checkComposedSources validates against — but without
// running validation or body-sync (Fix doesn't need either).
// Returns nil with no error when the rule has no schema source or
// every source is empty / self-referential. A file source pointing
// at the file currently being fixed is skipped so a schema
// doesn't drive its own index side-output.
func (r *Rule) composedSchemaForFix(f *lint.File) (*schema.Schema, error) {
	sources := r.effectiveSources()
	if len(sources) == 0 {
		return nil, nil
	}
	parsed := make([]*schema.Schema, 0, len(sources))
	for _, src := range sources {
		if src.Inline != nil {
			if src.Inline.IsEmpty() {
				continue
			}
			parsed = append(parsed, src.Inline)
			continue
		}
		if src.File == "" {
			continue
		}
		sch, err := r.parseFileSchemaForCompose(f, src.File)
		if err != nil {
			return nil, err
		}
		if sch == nil {
			continue
		}
		parsed = append(parsed, sch)
	}
	if len(parsed) == 0 {
		return nil, nil
	}
	return schema.Compose(parsed...)
}

// ComposedSchema parses and composes every schema source the rule
// resolved for f and returns the composed schema, or nil when the
// rule has no schema source. Exposed for the `extract` subcommand,
// which projects the same composed schema MDS020 validates against;
// it reuses composedSchemaForFix so the two paths cannot drift.
func (r *Rule) ComposedSchema(f *lint.File) (*schema.Schema, error) {
	return r.composedSchemaForFix(f)
}

// checkSingleFileSchemaFromData runs the legacy validation with a
// pre-loaded schema buffer. The Check dispatch reads the schema
// once and routes the bytes here when extends is not declared, so
// the common single-source path avoids a second read.
func (r *Rule) checkSingleFileSchemaFromData(
	f *lint.File, schemaPath string, schData []byte, schPath string,
) []lint.Diagnostic {
	var diags []lint.Diagnostic
	sch, err := cachedParseSchema(f, schData, schPath)
	if err != nil {
		return append(diags, r.diag(f.Path, 1,
			fmt.Sprintf("invalid schema %q: %v", schemaPath, err)))
	}

	// Skip the schema file itself when schemas come from disk.
	if r.isSchemaFileAt(f, schemaPath) {
		return diags
	}

	docHeadings := extractHeadings(f)
	docFMRaw, fmDiags := readDocFrontMatterRaw(f)
	diags = append(diags, fmDiags...)

	// Check filename pattern.
	diags = append(diags, checkFilenamePattern(f, sch, r.Schema)...)

	// Check structure: required headings present and in order.
	diags = append(diags, checkStructure(f, sch, docHeadings, r.Schema)...)

	// Validate document front matter against schema-embedded CUE constraints,
	// unless the cue-frontmatter placeholder token is configured (which marks
	// the front-matter values as CUE expressions rather than concrete data).
	if !placeholders.HasCUEFrontmatter(r.Placeholders) {
		fmSch := &schema.Schema{
			Frontmatter:      sch.Config.Frontmatter,
			FrontmatterLines: sch.Config.FrontmatterLines,
			FrontmatterMeta:  sch.Config.FrontmatterMeta,
			Source:           r.Schema,
		}
		diags = append(diags, schema.ValidateFrontmatterDiags(f, fmSch, docFMRaw, makeDiag)...)
	}

	// Check frontmatter-body sync using raw map for nested access.
	diags = append(diags, checkSync(f, sch, docHeadings, docFMRaw)...)

	return diags
}

// checkComposedSources loads every source, composes them via
// schema.Compose, and validates the document against the composed
// schema. Each FILE source ALSO runs the legacy heading- and
// body-sync check (proto.md `# {id}: {name}` and Meta-Information
// body lines) — composition cannot express those checks today, so
// per-source legacy validation preserves them. Sources that name a
// file the rule is currently linting are skipped (self-validation).
func (r *Rule) checkComposedSources(f *lint.File, sources []SchemaSource) []lint.Diagnostic {
	var diags []lint.Diagnostic
	docFMRaw, fmDiags := readDocFrontMatterRaw(f)
	diags = append(diags, fmDiags...)

	parsed := make([]*schema.Schema, 0, len(sources))
	for _, src := range sources {
		if src.Inline != nil {
			if src.Inline.IsEmpty() {
				continue
			}
			parsed = append(parsed, src.Inline)
			continue
		}
		if src.File == "" {
			continue
		}
		// Per-source legacy body-sync. Loads via the legacy parser so
		// proto.md-style {field} interpolation and Meta-Information
		// body sync still fire for each file source.
		if !r.isSchemaFileAt(f, src.File) {
			diags = append(diags, r.bodySyncDiagnostics(f, src.File, docFMRaw)...)
		}
		sch, err := r.parseFileSchemaForCompose(f, src.File)
		if err != nil {
			diags = append(diags, r.diag(f.Path, 1, err.Error()))
			continue
		}
		if sch == nil {
			// f was the schema itself — skip its composition entry.
			continue
		}
		parsed = append(parsed, sch)
	}

	if len(parsed) == 0 {
		return diags
	}

	composed, err := schema.Compose(parsed...)
	if err != nil {
		return append(diags, r.diag(f.Path, 1,
			fmt.Sprintf("composing schemas: %v", err)))
	}
	// composed is non-nil here: parsed contains at least one
	// non-nil schema (the empty-source filter above guarantees it),
	// and schema.Compose returns its single input unchanged when
	// len(parsed) == 1. IsEmpty() can still hold when every parsed
	// schema was itself empty (e.g. a proto.md with no headings).
	if composed.IsEmpty() {
		return diags
	}

	fmIsCUE := placeholders.HasCUEFrontmatter(r.Placeholders)
	diags = append(diags, schema.Validate(f, composed, docFMRaw, fmIsCUE, makeDiag)...)
	diags = append(diags, r.applyScopeRules(f, composed, docFMRaw)...)
	diags = append(diags, schema.ValidateCrossReferences(f, composed, makeDiag)...)
	diags = append(diags, schema.ValidateAcronyms(f, composed, docFMRaw, makeDiag)...)
	diags = append(diags, schema.ValidateIndex(f, composed, makeDiag)...)
	return diags
}

// parseFileSchemaForCompose loads a proto.md file source via the
// unified schema.ParseFile parser so the result composes with inline
// schemas. Returns (nil, nil) when f is the schema file itself —
// the caller skips that entry so a schema doesn't validate against
// itself.
func (r *Rule) parseFileSchemaForCompose(f *lint.File, schemaPath string) (*schema.Schema, error) {
	if r.isSchemaFileAt(f, schemaPath) {
		return nil, nil
	}
	reader := &schema.FileReader{
		RootFS:   f.RootFS,
		RootDir:  f.RootDir,
		MaxBytes: f.MaxInputBytes,
	}
	sch, err := schema.ParseFile(reader, schemaPath)
	if err != nil {
		return nil, fmt.Errorf("cannot load schema %q: %v", schemaPath, err)
	}
	return sch, nil
}

// bodySyncDiagnostics runs only the heading- and body-sync portion
// of the legacy file-schema check for a single file source. The
// composed structure validation runs separately; calling
// checkSingleFileSchema here would double-report missing-section
// and frontmatter-CUE diagnostics.
func (r *Rule) bodySyncDiagnostics(f *lint.File, schemaPath string, docFMRaw map[string]any) []lint.Diagnostic {
	var diags []lint.Diagnostic
	if len(docFMRaw) == 0 {
		return diags
	}
	data, _, err := r.loadSchemaAt(f, schemaPath)
	if err != nil {
		return append(diags, r.diag(f.Path, 1, err.Error()))
	}
	sch, err := cachedParseSchema(f, data, schemaPath)
	if err != nil {
		// The compose path reports schema-parse errors separately;
		// avoid duplicating them here.
		return diags
	}
	docHeadings := extractHeadings(f)
	return append(diags, checkSync(f, sch, docHeadings, docFMRaw)...)
}

// loadSchemaAt reads the named schema file using the file's RootFS
// when configured, falling back to the OS filesystem.
func (r *Rule) loadSchemaAt(f *lint.File, schemaPath string) ([]byte, string, error) {
	data, err := readSchemaFile(f, schemaPath)
	if err != nil {
		return nil, "", fmt.Errorf("cannot read schema %q: %v", schemaPath, err)
	}
	return data, schemaPath, nil
}

// isSchemaFileAt reports whether f is the schema file at the named
// path. It normalizes f.Path against f.RootDir so the check still
// succeeds when mdsmith runs from a subdirectory while `schema:`
// paths remain project-root-relative.
func (r *Rule) isSchemaFileAt(f *lint.File, schemaPath string) bool {
	if schemaPath == "" {
		return false
	}
	if isSchemaFile(f.Path, schemaPath) {
		return true
	}
	if f.RootDir == "" {
		return false
	}
	abs, _ := filepath.Abs(f.Path)
	rel, err := filepath.Rel(f.RootDir, abs)
	if err != nil {
		return false
	}
	return isSchemaFile(rel, schemaPath)
}

func (r *Rule) diag(file string, line int, msg string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     file,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Error,
		Message:  msg,
	}
}

var (
	_ rule.Configurable       = (*Rule)(nil)
	_ rule.ListMerger         = (*Rule)(nil)
	_ rule.FixableRule        = (*Rule)(nil)
	_ rule.SettingsTranslator = (*Rule)(nil)
)

// schemaConfig holds the parsed schema frontmatter.
type schemaConfig struct {
	FrontMatterCUE  string
	FilenamePattern string // glob pattern the document basename must match

	// Frontmatter carries the per-key constraint expression
	// strings. Populated alongside FrontMatterCUE so the
	// SchemaDiagnostic emitter can render one diagnostic per CUE
	// error path with the source expression available for
	// "expected" extraction.
	Frontmatter map[string]string

	// FrontmatterLines records the 1-based line number of each
	// front-matter key in the schema source, when known. The
	// yaml.Node based parser populates this; the legacy
	// yaml.Unmarshal path leaves it empty.
	FrontmatterLines map[string]int

	// FrontmatterMeta captures plan 136's deprecation metadata when
	// the schema declares a field in the map form (`type:` +
	// `deprecated:` siblings). The key shape mirrors Frontmatter
	// (with the optional "?" suffix preserved) so the validator
	// can look up metadata using the same key it sees on the CUE
	// constraint.
	FrontmatterMeta map[string]schema.FieldMeta
}

// schemaHeading represents a required heading from the schema.
type schemaHeading struct {
	Level    int
	Text     string         // raw text, may contain {field} or ?
	compiled *regexp.Regexp // pre-compiled regex for {field} patterns; nil if not needed
}

// parsedSchema holds the full parsed schema.
type parsedSchema struct {
	Config   schemaConfig
	Headings []schemaHeading
	// syncPoints maps heading index to list of (field, expected text) pairs
	// for body sync checking.
	SyncPoints map[int][]syncPoint
}

// syncPoint represents a {field} reference in heading text.
type syncPoint struct {
	Field    string
	InBody   bool           // true if in body content, false if in heading
	BodyText string         // the full expected body line text with field substituted
	compiled *regexp.Regexp // pre-compiled pattern for BodyText; nil for non-body sync points
}

var cueIdentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

const sectionWildcard = "..."

// parseSchemaFrontMatter extracts the schema configuration from
// frontmatter. cache is the optional RunCache the CUE compile uses
// to share its result across schemas with identical CUE source —
// the inner validateCUESchemaSyntax routes through it when non-nil.
// Tests pass nil to keep the parser standalone.
func parseSchemaFrontMatter(prefix []byte, cache *lint.RunCache) (schemaConfig, error) {
	cfg := schemaConfig{}
	if prefix == nil {
		return cfg, nil
	}
	yamlBytes := extractYAML(prefix)
	derivedSchema, perKey, meta, err := deriveFrontMatterCUE(yamlBytes)
	if err != nil {
		return cfg, err
	}
	cfg.FrontMatterCUE = derivedSchema
	cfg.Frontmatter = perKey
	cfg.FrontmatterMeta = meta
	if err := validateCUESchemaSyntaxWith(cache, cfg.FrontMatterCUE); err != nil {
		return cfg, err
	}
	// Capture per-key source lines. The YAML body sits after the
	// opening "---\n" fence in the schema file, so line numbers
	// from yaml.Node are off by 1 relative to the schema source.
	node, nodeErr := yamlutil.UnmarshalNodeSafe(yamlBytes)
	if nodeErr == nil {
		lines := yamlutil.TopLevelMappingLines(&node, 1)
		// `extends:` is a schema directive (plan 135); strip it from
		// the per-key source-line map so MDS020 never tries to point
		// at the reserved key for a frontmatter validation error.
		delete(lines, "extends")
		cfg.FrontmatterLines = lines
	}
	return cfg, nil
}

// extractRequireDirective walks the schema AST for a <?require?> PI
// and parses its YAML body to extract constraints like filename.
func extractRequireDirective(f *lint.File) (string, error) {
	var filenamePattern string
	for c := f.AST.FirstChild(); c != nil; c = c.NextSibling() {
		pi, ok := c.(*piparser.ProcessingInstruction)
		if !ok || pi.Name != "require" {
			continue
		}
		// Extract YAML body from PI content.
		lines := pi.Lines()
		var body []byte
		if lines.Len() == 1 {
			// Single-line: <?require key: value ?>
			// Extract content between <?require and ?>
			// (ignoring any trailing text after ?>).
			seg := lines.At(0)
			line := strings.TrimSpace(string(seg.Value(f.Source)))
			line = strings.TrimPrefix(line, "<?require")
			if idx := strings.Index(line, "?>"); idx >= 0 {
				line = line[:idx]
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			body = []byte(line)
		} else {
			// Multi-line: skip first line (<?require),
			// remaining lines are YAML body before ?>
			for i := 1; i < lines.Len(); i++ {
				seg := lines.At(i)
				body = append(body, seg.Value(f.Source)...)
			}
		}
		var params map[string]string
		if err := yamlutil.UnmarshalSafe(body, &params); err != nil {
			return "", fmt.Errorf("invalid <?require?> directive: %w", err)
		}
		if fn, ok := params["filename"]; ok {
			filenamePattern = fn
		}
		break
	}
	return filenamePattern, nil
}

func deriveFrontMatterCUE(yamlBytes []byte) (string, map[string]string, map[string]schema.FieldMeta, error) {
	var raw map[string]any
	if err := yamlutil.UnmarshalSafe(yamlBytes, &raw); err != nil {
		return "", nil, nil, fmt.Errorf("parsing schema frontmatter: %w", err)
	}
	// `extends:` is a reserved schema-engine directive (plan 135),
	// not a document-frontmatter constraint. Strip it before the
	// CUE expression is built so the legacy body-sync path does not
	// require documents to carry the literal `extends:` value.
	delete(raw, "extends")
	if len(raw) == 0 {
		return "", nil, nil, nil
	}

	// Detach plan-136 metadata-form values so the CUE expression
	// only sees the embedded `type:` constraint. Without this the
	// `cueExprForMap` walker would emit the whole metadata map as a
	// struct constraint and reject any document value that isn't a
	// matching mapping.
	meta := map[string]schema.FieldMeta{}
	for k, v := range raw {
		expr, fm, isMeta, err := schema.ExtractFieldMeta(v)
		if err != nil {
			return "", nil, nil, fmt.Errorf("schema frontmatter %q: %w", k, err)
		}
		if !isMeta {
			continue
		}
		raw[k] = expr
		meta[k] = fm
	}

	expr, err := cueExprForMap(raw)
	if err != nil {
		return "", nil, nil, fmt.Errorf("parsing schema frontmatter constraints: %w", err)
	}
	perKey := make(map[string]string, len(raw))
	for k, v := range raw {
		ke, err := cueExprForValue(v)
		if err != nil {
			continue
		}
		perKey[k] = ke
	}
	if len(meta) == 0 {
		meta = nil
	}
	return "close(" + expr + ")", perKey, meta, nil
}

func cueExprForMap(m map[string]any) (string, error) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("{\n")
	for _, k := range keys {
		expr, err := cueExprForValue(m[k])
		if err != nil {
			return "", fmt.Errorf("field %q: %w", k, err)
		}
		b.WriteString("  ")
		fieldName, optional := strings.CutSuffix(k, "?")
		b.WriteString(cueFieldLabel(fieldName))
		if optional {
			b.WriteString("?")
		}
		b.WriteString(": ")
		b.WriteString(expr)
		b.WriteString("\n")
	}
	b.WriteString("}")
	return b.String(), nil
}

func cueExprForValue(v any) (string, error) {
	switch x := v.(type) {
	case map[string]any:
		return cueExprForMap(x)
	case []any:
		b, err := json.Marshal(x)
		if err != nil {
			return "", fmt.Errorf("marshal array value: %w", err)
		}
		return string(b), nil
	case string:
		expr := strings.TrimSpace(x)
		if expr == "" {
			return "", fmt.Errorf("schema expression must be non-empty")
		}
		return expr, nil
	case int, int64, float64, bool:
		b, err := json.Marshal(x)
		if err != nil {
			return "", fmt.Errorf("marshal scalar value: %w", err)
		}
		return string(b), nil
	default:
		return "", fmt.Errorf("unsupported schema value type %T", v)
	}
}

func cueFieldLabel(key string) string {
	if cueIdentPattern.MatchString(key) {
		return key
	}
	return strconv.Quote(key)
}

// collectBodySyncPoints scans body content for {field} references and
// adds them to the syncPoints map under their nearest preceding heading.
// Iterates over content bytes directly to avoid the bytes.Split allocation.
func collectBodySyncPoints(
	content []byte, headings []docHeading,
	syncPoints map[int][]syncPoint,
) {
	currentHeading := -1
	// inPIBlock is true while scanning the lines of a multi-line
	// processing-instruction block (`<?content?>`, `<?require?>`,
	// `<?include?>`, …). Plan 242 makes directive rows opaque to the
	// legacy body-sync collector: a `{field}` token inside a directive
	// body (e.g. a `bind:` value) must not become a body-sync point,
	// since the row is schema syntax, not body-sync template text.
	inPIBlock := false
	var fenceChar byte
	fenceLen := 0
	start := 0
	for i := 0; i <= len(content); i++ {
		if i < len(content) && content[i] != '\n' {
			continue
		}
		raw := content[start:i]
		lineB := bytes.TrimSpace(raw)
		start = i + 1
		if len(lineB) == 0 {
			continue
		}
		switch {
		case inPIBlock:
			// Mirror the block parser: a continuation line closes the
			// PI only when its trimmed text is exactly `?>` — a `?>`
			// substring inside a YAML value stays inside the block.
			inPIBlock = !bytes.Equal(lineB, piClose)
			continue
		case fenceLen > 0:
			// Inside a fenced code block the only state change is a
			// valid close (CommonMark: the opener's character, a run
			// at least as long, nothing else on the line). Fence
			// lines — markers included — fall through as ordinary
			// body text, as they did before the PI skip existed.
			if fenceClose(raw, lineB, fenceChar, fenceLen) {
				fenceChar, fenceLen = 0, 0
			}
		default:
			if c, n := fenceOpenRun(raw, lineB); n > 0 {
				// A fenced code block owns its lines before the PI
				// parser runs, so a directive opener shown inside a
				// fence is code, not a directive.
				fenceChar, fenceLen = c, n
				break
			}
			if !isPIOpenLine(raw) {
				break
			}
			// A single-line `<?name ... ?>` opens and closes on the
			// same line; only a multi-line opener leaves us inside the
			// block for subsequent lines. The parser closes an opener
			// on a `?>` anywhere in its line.
			inPIBlock = !bytes.Contains(lineB, piClose)
			continue
		}
		if lineB[0] == '#' {
			if idx := headingIndexForLine(headings, string(lineB)); idx >= 0 {
				currentHeading = idx
			}
			continue
		}
		if currentHeading >= 0 {
			appendBodySyncFields(syncPoints, currentHeading, string(lineB))
		}
	}
}

// headingIndexForLine returns the index of the first schema heading
// that matches the body line, or -1 when none does.
func headingIndexForLine(headings []docHeading, line string) int {
	for j, h := range headings {
		if headingMatchesLine(h, line) {
			return j
		}
	}
	return -1
}

// appendBodySyncFields records a body-sync point under heading idx for
// each `{field}` token found in the body line. Lines with no token add
// nothing.
func appendBodySyncFields(
	syncPoints map[int][]syncPoint, idx int, trimmed string,
) {
	fields := fieldinterp.Fields(trimmed)
	if len(fields) == 0 {
		return
	}
	compiled := buildFieldPattern(trimmed)
	for _, f := range fields {
		syncPoints[idx] = append(syncPoints[idx],
			syncPoint{Field: f, InBody: true, BodyText: trimmed, compiled: compiled})
	}
}

// piOpenPrefix / piClose are the literal markers
// collectBodySyncPoints scans for to skip processing-instruction
// blocks. They are package-level so the byte slices are allocated
// once rather than per call.
var (
	piOpenPrefix = []byte("<?")
	piClose      = []byte("?>")
)

// fenceOpenRun reports the marker character and run length when a
// body line opens a fenced code block the way the block parser would:
// at most three spaces of indentation and a run of at least three
// backticks or tildes. n is 0 when the line opens no fence. (The same
// open/close contract as include.rewriteSkippingCode.)
func fenceOpenRun(raw, lineB []byte) (byte, int) {
	if len(lineB) == 0 || (lineB[0] != '`' && lineB[0] != '~') {
		return 0, 0
	}
	if lineIndent(raw) > 3 {
		return 0, 0
	}
	n := fenceRun(lineB, lineB[0])
	if n < 3 {
		return 0, 0
	}
	return lineB[0], n
}

// fenceClose reports whether a body line closes the open fence per
// CommonMark: the opener's character, a run at least as long as the
// opener, nothing but the run on the trimmed line, and at most three
// spaces of indentation.
func fenceClose(raw, lineB []byte, ch byte, openLen int) bool {
	if lineIndent(raw) > 3 {
		return false
	}
	n := fenceRun(lineB, ch)
	return n >= openLen && n == len(lineB)
}

// fenceRun returns the length of the run of ch at the start of line.
func fenceRun(line []byte, ch byte) int {
	n := 0
	for n < len(line) && line[n] == ch {
		n++
	}
	return n
}

// lineIndent returns the number of leading spaces on the raw line.
func lineIndent(raw []byte) int {
	return len(raw) - len(bytes.TrimLeft(raw, " "))
}

// isPIOpenLine reports whether a raw body line opens a processing
// instruction, mirroring the block parser in pkg/markdown: at most
// three spaces of indentation, a `<?` opener, and a non-empty name
// (the bytes up to the first whitespace or `?>`). An indented code
// example showing a directive is therefore not mistaken for one.
func isPIOpenLine(raw []byte) bool {
	if lineIndent(raw) > 3 {
		return false
	}
	trimmed := bytes.TrimLeft(raw, " ")
	trimmed = bytes.TrimRight(trimmed, " \t\r\n")
	if !bytes.HasPrefix(trimmed, piOpenPrefix) {
		return false
	}
	rest := trimmed[len(piOpenPrefix):]
	if len(rest) == 0 {
		return false
	}
	if rest[0] == ' ' || rest[0] == '\t' {
		return false
	}
	if bytes.HasPrefix(rest, piClose) {
		return false
	}
	return true
}

// maxSchemaIncludeDepth is the maximum nesting depth for schema includes.
const maxSchemaIncludeDepth = 10

// parseSchema reads schema bytes, extracts frontmatter config and
// required headings. When schemaPath is non-empty, <?include?>
// directives are expanded and their headings spliced in.
//
// Tests call parseSchema with no cache; the rule's hot path goes
// through parseSchemaWithCache so the inner CUE compile shares its
// result across schemas with the same CUE source. parseSchema is
// the canonical name kept for the existing test surface; the
// cache-aware form is a thin wrapper that adds the cache argument
// and surfaces the include set the cache wrapper needs for
// dependency tracking.
func parseSchema(data []byte, schemaPath string, maxBytes int64) (*parsedSchema, error) {
	sch, _, err := parseSchemaWithCache(data, schemaPath, maxBytes, nil)
	return sch, err
}

// parseSchemaWithCache is parseSchema with an optional RunCache wired
// through to validateCUESchemaSyntax so the CUE compile can be shared
// across schemas with identical CUE source. A nil cache restores the
// original behaviour (one fresh cuecontext per call).
//
// The returned includes slice carries every fragment path the
// schema's <?include?> directives reached. Paths are in the SAME
// coordinate system as schemaPath — typically project-relative when
// the rule's loadSchemaAt passes a project-relative schemaPath. The
// cache-aware wrapper (cachedParseSchema) anchors them onto the
// workspace root via absoluteIncludes before storing them on
// schemaParseResult, so RunCache.Invalidate can match against the
// absolute paths the LSP passes. Returns nil when schemaPath is
// empty (no filesystem identity → no include resolution).
//
// On a parseSchemaFrontMatter error (the common LSP editing state
// where the schema's CUE is mid-edit and fails CompileString), the
// function still surfaces a partial *parsedSchema carrying the
// derived FrontMatterCUE alongside the error. The rule's caller
// discards a non-nil sch when err != nil, but the cache wrapper
// reads schemaCUESources(sch) for invalidation tracking — so the
// failed CompiledCUE entry the build cached is evicted when the
// next Invalidate(schemaPath) fires.
func parseSchemaWithCache(
	data []byte, schemaPath string, maxBytes int64, cache *lint.RunCache,
) (*parsedSchema, []string, error) {
	return parseSchemaWithRootFS(data, schemaPath, maxBytes, cache, nil)
}

// parseSchemaWithRootFS is parseSchemaWithCache plus the workspace RootFS
// used to read <?include?> fragments. The Session/WASM and LSP engines
// have no usable OS disk (os.ReadFile is "not implemented on js" in
// WASM), so schema includes must read through the in-memory workspace FS;
// the CLI passes a nil rootFS and reads from disk. cachedParseSchema
// supplies f.RootFS.
func parseSchemaWithRootFS(
	data []byte, schemaPath string, maxBytes int64, cache *lint.RunCache, rootFS fs.FS,
) (*parsedSchema, []string, error) {
	prefix, content := lint.StripFrontMatter(data)

	cfg, err := parseSchemaFrontMatter(prefix, cache)
	if err != nil {
		// parseSchemaFrontMatter populates cfg.FrontMatterCUE
		// before the validation step, so the partial schema we
		// surface here carries the source that just failed to
		// compile. The cache wrapper then records it in
		// schemaCUESources so RunCache.Invalidate(schemaPath)
		// drops the failed CompiledCUE entry on the next edit.
		return &parsedSchema{Config: cfg}, nil, err
	}

	// lint.NewFile returns a nil error in every code path
	// (internal/lint/file.go); the defensive check the previous
	// signature carried was unreachable from any test. Discard the
	// error here so the patch carries no untestable defensive
	// branch.
	f, _ := lint.NewFile("schema", content)

	// Extract <?require?> directive from schema body.
	filenamePattern, err := extractRequireDirective(f)
	if err != nil {
		// cfg.FrontMatterCUE was already compiled (and cached
		// via RunCache.CompiledCUE) before extractRequireDirective
		// ran, so surface a partial *parsedSchema so the cache
		// wrapper can record cueSources. Without this, an LSP
		// mid-edit state where <?require?> is malformed leaves
		// the CompiledCUE entry uncoupled from any schemaPath
		// and Invalidate(schemaPath) cannot evict it.
		return &parsedSchema{Config: cfg}, nil, err
	}
	cfg.FilenamePattern = filenamePattern

	// Extract headings, expanding <?include?> directives in the schema.
	var headings []docHeading
	var includes []string
	if schemaPath != "" {
		cleanPath := filepath.Clean(schemaPath)
		visited := map[string]bool{cleanPath: true}
		chain := []string{cleanPath}
		var fp string
		headings, fp, includes, err = extractSchemaHeadings(f, schemaPath, visited, chain, maxBytes, rootFS)
		if err != nil {
			// Surface a partial *parsedSchema (carrying the
			// frontmatter CUE source the cache will track via
			// schemaCUESources) AND the partial include set the
			// walk reached before erroring. The rule's caller
			// discards the partial schema on err, but the cache
			// wrapper records both pieces so a later
			// Invalidate(fragment) on a fixed include still
			// evicts the schema's failed-parse slot.
			return &parsedSchema{Config: cfg}, includes, err
		}
		if fp != "" && cfg.FilenamePattern == "" {
			cfg.FilenamePattern = fp
		}
	} else {
		headings = extractHeadings(f)
	}

	schHeadings := make([]schemaHeading, len(headings))
	syncPoints := make(map[int][]syncPoint)

	for i, h := range headings {
		schHeadings[i] = buildSchemaHeading(h)
		for _, f := range fieldinterp.Fields(h.Text) {
			syncPoints[i] = append(syncPoints[i], syncPoint{Field: f})
		}
	}

	collectBodySyncPoints(content, headings, syncPoints)

	return &parsedSchema{
		Config:     cfg,
		Headings:   schHeadings,
		SyncPoints: syncPoints,
	}, includes, nil
}

// extractSchemaHeadings walks the schema AST, collecting headings and
// expanding <?include?> PIs by splicing in the included file's headings.
// It uses a visited set for cycle detection.
//
// The returned includes slice carries the resolved path of every
// fragment reached during the walk — both direct includes and
// transitively-included ones. Paths are in the SAME coordinate
// system as schemaPath: resolveSchemaIncludePath joins each include
// onto filepath.Dir(schemaPath), so a project-relative schemaPath
// produces project-relative include entries (and an absolute
// schemaPath produces absolute ones). cachedParseSchemaWith later
// anchors them onto absRoot via absoluteIncludes before storing on
// schemaParseResult, so RunCache.Invalidate's absolute keys match.
// Order is source order at the current level, with each fragment's
// transitive set appended after the fragment's own path.
func extractSchemaHeadings(
	schemaFile *lint.File, schemaPath string,
	visited map[string]bool, chain []string, maxBytes int64, rootFS fs.FS,
) ([]docHeading, string, []string, error) {
	var headings []docHeading
	var filenamePattern string
	var includes []string

	err := ast.Walk(schemaFile.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch node := n.(type) {
		case *ast.Heading:
			text := headingText(node, schemaFile.Source)
			line := schemaFile.LineOfOffset(node.Lines().At(0).Start)
			headings = append(headings, docHeading{Level: node.Level, Text: text, Line: line})

		case *piparser.ProcessingInstruction:
			if node.Name != "include" {
				return ast.WalkContinue, nil
			}
			fragHeadings, fp, fragIncluded, subIncludes, walkErr := expandSchemaInclude(
				node, schemaFile.Source, schemaPath, visited, chain, maxBytes, rootFS)
			if walkErr != nil {
				// Record the broken-but-known fragment plus any
				// transitive sub-includes the recursive walk
				// reached before erroring. The outer
				// extractSchemaHeadings surfaces this partial
				// `includes` slice up to parseSchemaWithCache so
				// the cache wrapper's reverse-include index covers
				// the full dependency footprint, broken edges
				// included. Without this, Invalidate(fragment) on
				// a fix would not evict the schema's failed-parse
				// slot.
				if fragIncluded != "" {
					includes = append(includes, fragIncluded)
				}
				includes = append(includes, subIncludes...)
				return ast.WalkStop, walkErr
			}
			if fp != "" && filenamePattern == "" {
				filenamePattern = fp
			}
			headings = append(headings, fragHeadings...)
			if fragIncluded != "" {
				includes = append(includes, fragIncluded)
			}
			includes = append(includes, subIncludes...)
		}

		return ast.WalkContinue, nil
	})
	if err != nil {
		// Surface the partial include set so the cache wrapper can
		// still register dependents on fragments the walk reached
		// before erroring. A schema that fails to parse because of
		// a downstream <?include?> issue must still let
		// Invalidate(brokenFragment) evict the schema's failed-parse
		// slot when the fragment is fixed.
		return nil, "", includes, err
	}

	return headings, filenamePattern, includes, nil
}

// expandSchemaInclude resolves a single <?include?> PI in a schema file,
// reads the fragment, and returns its headings and any filename pattern.
// resolveSchemaIncludePath extracts and validates the file parameter from
// an include PI, returning the resolved filesystem path.
func resolveSchemaIncludePath(
	pi *piparser.ProcessingInstruction, source []byte, schemaPath string,
) (string, error) {
	fileParam, err := extractPIFileParam(pi, source)
	if err != nil {
		return "", fmt.Errorf("parsing include processing instruction: %w", err)
	}
	if strings.TrimSpace(fileParam) == "" {
		return "", fmt.Errorf("include processing instruction missing required 'file' attribute")
	}
	if filepath.IsAbs(fileParam) {
		return "", fmt.Errorf("schema include has absolute file path %q", fileParam)
	}
	for _, elem := range strings.Split(filepath.ToSlash(fileParam), "/") {
		if elem == ".." {
			return "", fmt.Errorf(
				"schema include file path %q contains \"..\" traversal", fileParam)
		}
	}
	dir := filepath.Dir(schemaPath)
	return filepath.Clean(filepath.Join(dir, fileParam)), nil
}

// expandSchemaInclude resolves and recursively expands a schema
// include PI. It returns the fragment's headings, any filename
// pattern declared on the fragment, the fragment's resolved path
// (in the same coordinate system as schemaPath — typically
// project-relative; cachedParseSchemaWith anchors it onto absRoot
// before storing), the fragment's transitive include set, and an
// error. The path-plus-subIncludes pair lets the caller record the
// full dependency footprint on RunCache's reverse-include index.
func expandSchemaInclude(
	pi *piparser.ProcessingInstruction, source []byte,
	schemaPath string, visited map[string]bool, chain []string, maxBytes int64, rootFS fs.FS,
) ([]docHeading, string, string, []string, error) {
	includedPath, err := resolveSchemaIncludePath(pi, source, schemaPath)
	if err != nil {
		return nil, "", "", nil, err
	}

	// Surface includedPath on every error past this point so the
	// caller can register the broken-but-known fragment in
	// RunCache's reverse-include index. When the user fixes the
	// fragment (creates the missing file, breaks the cycle, raises
	// the depth limit), Invalidate(fragmentAbs) then evicts this
	// schema's failed-parse slot and the next Check re-parses.
	if len(chain) > maxSchemaIncludeDepth {
		return nil, "", includedPath, nil, fmt.Errorf(
			"schema include depth exceeds maximum (%d)", maxSchemaIncludeDepth)
	}
	if visited[includedPath] {
		chainCopy := make([]string, len(chain), len(chain)+1)
		copy(chainCopy, chain)
		chainCopy = append(chainCopy, includedPath)
		return nil, "", includedPath, nil, fmt.Errorf(
			"cyclic include: %s", strings.Join(chainCopy, " -> "))
	}

	fragData, err := readSchemaInclude(rootFS, includedPath, maxBytes)
	if err != nil {
		return nil, "", includedPath, nil, fmt.Errorf(
			"cannot read schema include file %q: %w", includedPath, err)
	}

	_, fragContent := lint.StripFrontMatter(fragData)
	// lint.NewFile returns a nil error in every code path
	// (internal/lint/file.go); the previous defensive branch was
	// unreachable from any test. Discard the error so the patch
	// carries no untestable defensive code.
	fragFile, _ := lint.NewFile(includedPath, fragContent)

	fp, err := extractRequireDirective(fragFile)
	if err != nil {
		return nil, "", includedPath, nil, err
	}

	visited[includedPath] = true
	chain = append(chain, includedPath)
	fragHeadings, fp2, subIncludes, err := extractSchemaHeadings(
		fragFile, includedPath, visited, chain, maxBytes, rootFS)
	delete(visited, includedPath)
	if err != nil {
		// Surface includedPath plus whatever subIncludes the
		// recursive walk reached so the full dependency
		// footprint (broken fragment + every successful one
		// above it) is recorded in the cache's reverse-include
		// index.
		return nil, "", includedPath, subIncludes, err
	}
	if fp2 != "" && fp == "" {
		fp = fp2
	}

	return fragHeadings, fp, includedPath, subIncludes, nil
}

// extractPIFileParam parses the YAML body of an include PI to extract
// the "file" parameter.
func extractPIFileParam(pi *piparser.ProcessingInstruction, source []byte) (string, error) {
	lines := pi.Lines()
	var body string
	if lines.Len() == 1 {
		seg := lines.At(0)
		raw := strings.TrimSpace(string(seg.Value(source)))
		raw = strings.TrimPrefix(raw, "<?"+pi.Name)
		if idx := strings.Index(raw, "?>"); idx >= 0 {
			raw = raw[:idx]
		}
		body = strings.TrimSpace(raw)
	} else {
		var b strings.Builder
		for i := 1; i < lines.Len(); i++ {
			seg := lines.At(i)
			b.Write(seg.Value(source))
		}
		body = b.String()
	}

	if body == "" {
		return "", nil
	}

	var params map[string]string
	if err := yamlutil.UnmarshalSafe([]byte(body), &params); err != nil {
		return "", fmt.Errorf("invalid include directive YAML: %w", err)
	}

	return params["file"], nil
}

// docHeading represents a heading found in the document being checked.
type docHeading struct {
	Level int
	Text  string
	Line  int
}

// extractHeadings walks the AST and collects all headings.
func extractHeadings(f *lint.File) []docHeading {
	var headings []docHeading
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}

		text := headingText(h, f.Source)
		line := f.LineOfOffset(h.Lines().At(0).Start)

		headings = append(headings, docHeading{
			Level: h.Level,
			Text:  text,
			Line:  line,
		})
		return ast.WalkContinue, nil
	})
	return headings
}

// headingText extracts the plain text content of a heading node.
func headingText(h *ast.Heading, source []byte) string {
	var buf strings.Builder
	for c := h.FirstChild(); c != nil; c = c.NextSibling() {
		writeNodeText(c, source, &buf)
	}
	return buf.String()
}

// writeNodeText recursively writes the text content of an AST node.
func writeNodeText(n ast.Node, source []byte, buf *strings.Builder) {
	if t, ok := n.(*ast.Text); ok {
		buf.Write(t.Segment.Value(source))
		return
	}
	if _, ok := n.(*ast.CodeSpan); ok {
		// Code spans store text in child nodes.
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			writeNodeText(c, source, buf)
		}
		return
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		writeNodeText(c, source, buf)
	}
}

// headingMatchesLine checks if a docHeading matches a raw heading line.
func headingMatchesLine(h docHeading, line string) bool {
	// Strip leading # and spaces.
	stripped := strings.TrimLeft(line, "#")
	stripped = strings.TrimSpace(stripped)
	return stripped == h.Text
}

// checkStructure verifies required headings are present and in order.
func checkStructure(
	f *lint.File,
	sch *parsedSchema,
	docHeadings []docHeading,
	schemaSource string,
) []lint.Diagnostic {
	ref := buildSchemaRefForLegacy(schemaSource)
	requiredByText := buildRequiredByTextLegacy(sch.Headings)

	diags, docIdx, allowExtra := walkRequiredHeadings(
		f, sch, docHeadings, requiredByText, ref,
	)
	if !allowExtra {
		diags = append(diags, flagTrailingExtras(f, docHeadings, docIdx, ref)...)
	}
	return diags
}

// buildRequiredByTextLegacy maps each literal required heading text
// to its schema indices, so a doc heading seen at the "wrong"
// position can be recognized as an out-of-order required section
// rather than double-counted as both "unexpected" and "missing".
// Wildcard, `?`, and field-interpolated headings are excluded
// because their match depends on context.
func buildRequiredByTextLegacy(headings []schemaHeading) map[string][]int {
	out := map[string][]int{}
	for i, req := range headings {
		if isSectionWildcard(req) || req.Text == "?" {
			continue
		}
		if fieldinterp.ContainsField(req.Text) {
			continue
		}
		out[req.Text] = append(out[req.Text], i)
	}
	return out
}

// walkRequiredHeadings iterates the schema's heading list, matching
// each required entry against the document and emitting
// missing/unexpected/out-of-order diagnostics as it goes.
func walkRequiredHeadings(
	f *lint.File, sch *parsedSchema, docHeadings []docHeading,
	requiredByText map[string][]int, ref string,
) ([]lint.Diagnostic, int, bool) {
	var diags []lint.Diagnostic
	claimed := make(map[int]bool)
	docIdx := 0
	allowExtra := false
	for schIdx, req := range sch.Headings {
		if isSectionWildcard(req) {
			allowExtra = true
			continue
		}
		if claimed[schIdx] {
			continue
		}
		// Save the position before the scan so that a missing-section
		// anchor uses the last correctly-placed heading, not the last
		// heading consumed (which may be an out-of-order entry).
		preScanIdx := docIdx
		reqDiags, newIdx, found := matchRequired(
			f, sch, docHeadings, docIdx, schIdx,
			requiredByText, claimed, allowExtra, ref,
		)
		diags = append(diags, reqDiags...)
		docIdx = newIdx
		if found {
			allowExtra = false
		}
		if !found && !claimed[schIdx] {
			diags = append(diags, missingSectionDiagLegacy(
				f, req, ref, legacyPrecedingLine(docHeadings, preScanIdx)))
		}
	}
	return diags, docIdx, allowExtra
}

// flagTrailingExtras emits one diagnostic per leftover document
// heading when no wildcard allows trailing extras.
func flagTrailingExtras(
	f *lint.File, docHeadings []docHeading, docIdx int, ref string,
) []lint.Diagnostic {
	var diags []lint.Diagnostic
	for docIdx < len(docHeadings) {
		dh := docHeadings[docIdx]
		d := schema.SchemaDiagnostic{
			Field:     formatHeading(dh.Level, dh.Text),
			Actual:    "<present>",
			Expected:  "not declared in schema",
			SchemaRef: ref,
		}
		diags = append(diags, d.Emit(makeDiag, f.Path, dh.Line))
		docIdx++
	}
	return diags
}

// missingSectionDiagLegacy builds the SchemaDiagnostic for a
// required heading that the document lacks.
func missingSectionDiagLegacy(
	f *lint.File, req schemaHeading, ref string, precedingLine int,
) lint.Diagnostic {
	d := schema.SchemaDiagnostic{
		Field:     formatHeading(req.Level, req.Text),
		Actual:    "<missing>",
		Expected:  "section to be present",
		SchemaRef: ref,
	}
	// Anchor at the heading the missing section should follow so the
	// squiggle lands where the section belongs; MissingSectionAnchor
	// falls back to the non-body anchor when there is no preceding
	// heading or the insertion point sits inside a generated section
	// (where filterGeneratedDiags would otherwise drop it).
	return d.Emit(makeDiag, f.Path, schema.MissingSectionAnchor(f, precedingLine))
}

// legacyPrecedingLine returns the line of the document heading just
// before docIdx (the section a missing required heading should
// follow), or 0 when there is none.
func legacyPrecedingLine(docHeadings []docHeading, docIdx int) int {
	if idx := docIdx - 1; idx >= 0 && idx < len(docHeadings) {
		return docHeadings[idx].Line
	}
	return 0
}

// buildSchemaRefForLegacy returns the schema reference string
// used by SchemaDiagnostic in the file-schema (proto.md) code
// path. An empty source falls back to the generic "schema"
// label so every diagnostic still carries a reference suffix.
func buildSchemaRefForLegacy(source string) string {
	if source == "" {
		return "schema"
	}
	return source
}

// matchRequired advances docIdx to find a doc heading matching req.
// It emits diagnostics for intervening doc headings: "unexpected" when
// they don't match any required section, or "out of order" when they
// match a later required section (which is then claimed to avoid a
// follow-up "missing required" for the same text).
func matchRequired(
	f *lint.File,
	sch *parsedSchema,
	docHeadings []docHeading,
	docIdx, schIdx int,
	requiredByText map[string][]int,
	claimed map[int]bool,
	allowExtra bool,
	schemaRef string,
) ([]lint.Diagnostic, int, bool) {
	var diags []lint.Diagnostic
	req := sch.Headings[schIdx]
	for docIdx < len(docHeadings) {
		dh := docHeadings[docIdx]
		if matchesSchema(req, dh) {
			if dh.Level != req.Level {
				diags = append(diags, levelMismatchDiag(f, dh, req, schemaRef))
			}
			claimed[schIdx] = true
			return diags, docIdx + 1, true
		}
		if ooIdx := nextUnclaimed(requiredByText[dh.Text], claimed, schIdx+1); ooIdx >= 0 {
			other := sch.Headings[ooIdx]
			ooDiag := schema.SchemaDiagnostic{
				Field:     formatHeading(dh.Level, dh.Text),
				Actual:    "<out of order>",
				Expected:  "in declared order",
				Hint:      fmt.Sprintf("expected after %q", formatHeading(req.Level, req.Text)),
				SchemaRef: schemaRef,
			}
			diags = append(diags, ooDiag.Emit(makeDiag, f.Path, dh.Line))
			if dh.Level != other.Level {
				diags = append(diags, levelMismatchDiag(f, dh, other, schemaRef))
			}
			claimed[ooIdx] = true
			docIdx++
			continue
		}
		if !allowExtra {
			unexpDiag := schema.SchemaDiagnostic{
				Field:     formatHeading(dh.Level, dh.Text),
				Actual:    "<present>",
				Expected:  "not declared in schema",
				Hint:      fmt.Sprintf("expected %q here instead", formatHeading(req.Level, req.Text)),
				SchemaRef: schemaRef,
			}
			diags = append(diags, unexpDiag.Emit(makeDiag, f.Path, dh.Line))
		}
		docIdx++
	}
	return diags, docIdx, false
}

// levelMismatchDiag builds a heading level-mismatch diagnostic that
// names the offending heading so readers can locate it quickly.
func levelMismatchDiag(
	f *lint.File, dh docHeading, req schemaHeading, schemaRef string,
) lint.Diagnostic {
	d := schema.SchemaDiagnostic{
		Field:     dh.Text,
		Actual:    "h" + strconv.Itoa(dh.Level),
		Expected:  "h" + strconv.Itoa(req.Level),
		SchemaRef: schemaRef,
	}
	return d.Emit(makeDiag, f.Path, dh.Line)
}

// nextUnclaimed returns the first index in candidates that is >= minIdx
// and not yet claimed, or -1 if none qualifies.
func nextUnclaimed(candidates []int, claimed map[int]bool, minIdx int) int {
	for _, idx := range candidates {
		if idx >= minIdx && !claimed[idx] {
			return idx
		}
	}
	return -1
}

// matchesSchema checks if a document heading matches a schema heading.
// For headings with {field} references the regex was compiled once at
// schema parse time and stored in req.compiled, so no per-call NFA build.
// Invariant: req.compiled is non-nil whenever ContainsField(req.Text) is
// true — buildSchemaHeading guarantees this.
func matchesSchema(req schemaHeading, doc docHeading) bool {
	if req.Text == "?" {
		return true
	}
	if req.compiled != nil {
		return req.compiled.MatchString(doc.Text)
	}
	return doc.Text == req.Text
}

func isSectionWildcard(req schemaHeading) bool {
	return strings.TrimSpace(req.Text) == sectionWildcard
}

// resolveFields replaces {field} placeholders with frontmatter values
// using CUE path resolution for nested access.
func resolveFields(text string, docFM map[string]any) string {
	return fieldinterp.Interpolate(text, docFM)
}

// advanceToMatch advances docIdx to the next heading matching req.
// Returns the matched index (or -1) and the new docIdx.
func advanceToMatch(
	req schemaHeading, docHeadings []docHeading, docIdx int,
) (int, int) {
	for docIdx < len(docHeadings) {
		if matchesSchema(req, docHeadings[docIdx]) {
			return docIdx, docIdx + 1
		}
		docIdx++
	}
	return -1, docIdx
}

// checkSyncPoint checks a single sync point against the document.
func checkSyncPoint(
	f *lint.File, sp syncPoint, req schemaHeading,
	dh docHeading, matchedDoc int, docHeadings []docHeading,
	docFM map[string]any,
) []lint.Diagnostic {
	// Check if the field exists in front matter using CUE path resolution.
	path := fieldinterp.ParseCUEPath(sp.Field)
	if path == nil {
		return []lint.Diagnostic{makeDiag(f.Path, dh.Line,
			fmt.Sprintf("invalid CUE path in sync placeholder: %q", sp.Field))}
	}
	if _, err := fieldinterp.ResolvePath(docFM, path); err != nil {
		return []lint.Diagnostic{makeDiag(f.Path, dh.Line,
			fmt.Sprintf("sync placeholder %q refers to missing or invalid frontmatter path: %v",
				sp.Field, err))}
	}
	if !sp.InBody {
		expected := resolveFields(req.Text, docFM)
		if dh.Text != expected {
			return []lint.Diagnostic{makeDiag(f.Path, dh.Line,
				fmt.Sprintf("heading does not match frontmatter: expected %q (from %s), got %q",
					expected, sp.Field, dh.Text))}
		}
		return nil
	}
	expected := resolveFields(sp.BodyText, docFM)
	return checkBodySync(f, dh, matchedDoc, docHeadings, expected, sp.Field)
}

// checkSync verifies frontmatter-body synchronization.
func checkSync(
	f *lint.File,
	sch *parsedSchema,
	docHeadings []docHeading,
	docFM map[string]any,
) []lint.Diagnostic {
	if len(docFM) == 0 {
		return nil
	}

	var diags []lint.Diagnostic
	docIdx := 0

	for schIdx, req := range sch.Headings {
		if isSectionWildcard(req) {
			continue
		}

		syncs := sch.SyncPoints[schIdx]
		if len(syncs) == 0 {
			_, docIdx = advanceToMatch(req, docHeadings, docIdx)
			continue
		}

		matchedDoc, newIdx := advanceToMatch(req, docHeadings, docIdx)
		docIdx = newIdx
		if matchedDoc < 0 {
			continue
		}

		dh := docHeadings[matchedDoc]
		for _, sp := range syncs {
			diags = append(diags,
				checkSyncPoint(f, sp, req, dh, matchedDoc, docHeadings, docFM)...)
		}
	}

	return diags
}

// checkBodySync checks that expected body text appears under the heading.
// It joins consecutive non-blank lines into paragraphs so that soft-wrapped
// descriptions still match their single-line frontmatter value.
func checkBodySync(
	f *lint.File,
	dh docHeading,
	headingIdx int,
	allHeadings []docHeading,
	expected string,
	field string,
) []lint.Diagnostic {
	// Determine the line range for body content under this heading.
	startLine := dh.Line + 1
	endLine := len(f.Lines)
	if headingIdx+1 < len(allHeadings) {
		endLine = allHeadings[headingIdx+1].Line - 1
	}

	// Check each individual line first (fast path).
	for i := startLine - 1; i < endLine && i < len(f.Lines); i++ {
		line := strings.TrimSpace(string(f.Lines[i]))
		if line == expected {
			return nil
		}
	}

	// Join consecutive non-blank lines into paragraphs and check each.
	var para []string
	for i := startLine - 1; i <= endLine && i <= len(f.Lines); i++ {
		var line string
		if i < endLine && i < len(f.Lines) {
			line = strings.TrimSpace(string(f.Lines[i]))
		}
		if line == "" || i == endLine || i == len(f.Lines) {
			if len(para) > 0 {
				joined := strings.Join(para, " ")
				if joined == expected {
					return nil
				}
				para = para[:0]
			}
			continue
		}
		para = append(para, line)
	}

	return []lint.Diagnostic{makeDiag(f.Path, dh.Line,
		fmt.Sprintf("body does not match frontmatter field %q: expected %q", field, expected))}
}

// validateCUESchemaSyntax checks that schema compiles as CUE.
//
// The free function form (no RunCache) is the canonical entry point
// kept for tests. The cache-bound variant validateCUESchemaSyntaxWith
// is reached from parseSchemaFrontMatter when a cache is in scope,
// so two schemas with identical CUE source share one compile per
// Run.
func validateCUESchemaSyntax(schema string) error {
	return validateCUESchemaSyntaxWith(nil, schema)
}

// validateCUESchemaSyntaxWith is validateCUESchemaSyntax with the
// CompileString site routed through cache when non-nil. A nil cache
// compiles a fresh value (the test-direct path).
func validateCUESchemaSyntaxWith(cache *lint.RunCache, schema string) error {
	if strings.TrimSpace(schema) == "" {
		return nil
	}
	if err := cachedCompiledCUEWith(cache, schema).Err(); err != nil {
		return fmt.Errorf("invalid schema frontmatter CUE: %w", err)
	}
	return nil
}

func validateFrontMatterCUE(schema string, fm map[string]any) error {
	if strings.TrimSpace(schema) == "" {
		return nil
	}

	compiled := cachedCompiledCUEWith(nil, schema)
	if err := compiled.Err(); err != nil {
		return fmt.Errorf("invalid CUE schema: %w", err)
	}

	if fm == nil {
		fm = map[string]any{}
	}

	// Validate the front-matter map directly against the cached schema, no
	// JSON marshal round-trip (plan 218). The context-free schema Value is
	// shared safely with no per-file recompile.
	if err := compiled.Value.CompileMap(fm).Validate(); err != nil {
		return err
	}

	return nil
}

// readDocFrontMatterRaw reads YAML frontmatter from the document.
func readDocFrontMatterRaw(f *lint.File) (map[string]any, []lint.Diagnostic) {
	if len(f.FrontMatter) == 0 {
		return nil, nil
	}

	yamlBytes := extractYAML(f.FrontMatter)
	if yamlBytes == nil {
		return nil, nil
	}

	if err := yamlutil.RejectYAMLAliases(yamlBytes); err != nil {
		return nil, []lint.Diagnostic{makeDiag(f.Path, schema.NonBodyDiagLine(f),
			fmt.Sprintf("front matter: %v", err))}
	}
	var raw map[string]any
	if err := yaml.Unmarshal(yamlBytes, &raw); err != nil {
		return nil, []lint.Diagnostic{makeDiag(f.Path, schema.NonBodyDiagLine(f),
			fmt.Sprintf("front matter: invalid YAML: %v", err))}
	}
	return raw, nil
}

// extractYAML extracts the YAML content between --- delimiters.
// The closing fence is removed via TrimSuffix on the canonical
// `---\n` (or bare `---` for blocks that omit the trailing
// newline) rather than a strings.Index scan, so a YAML block
// scalar value (e.g. `notes: |\n  ---\n`) that legitimately
// contains the same sequence inside its body cannot truncate
// the YAML early. A block that carries no recognisable fence at
// all returns nil so the caller short-circuits on bad input.
func extractYAML(fmBlock []byte) []byte {
	body := bytes.TrimPrefix(fmBlock, []byte("---\n"))
	switch {
	case bytes.HasSuffix(body, []byte("---\n")):
		return body[:len(body)-len("---\n")]
	case bytes.HasSuffix(body, []byte("---")):
		return body[:len(body)-len("---")]
	}
	return nil
}

// findRequireDirectiveLine returns the 1-based line number of the first
// <?require?> PI in the file, or 0 if none is found.
func findRequireDirectiveLine(f *lint.File) int {
	for c := f.AST.FirstChild(); c != nil; c = c.NextSibling() {
		pi, ok := c.(*piparser.ProcessingInstruction)
		if ok && pi.Name == "require" {
			return f.LineOfOffset(pi.Lines().At(0).Start)
		}
	}
	return 0
}

// isSchemaFile checks if the document path is the configured schema.
func isSchemaFile(docPath, schemaPath string) bool {
	docInfo, errDoc := os.Stat(docPath)
	schemaInfo, errSchema := os.Stat(schemaPath)
	if errDoc == nil && errSchema == nil {
		return os.SameFile(docInfo, schemaInfo)
	}

	docAbs, errDocAbs := filepath.Abs(docPath)
	schemaAbs, errSchemaAbs := filepath.Abs(schemaPath)
	if errDocAbs != nil || errSchemaAbs != nil {
		return false
	}
	return filepath.Clean(docAbs) == filepath.Clean(schemaAbs)
}

// formatHeading returns a markdown-style heading string.
func formatHeading(level int, text string) string {
	return strings.Repeat("#", level) + " " + text
}

// checkPathPatterns validates the workspace-relative path of f
// against every kind-level `path-pattern:` configured on the rule.
// One diagnostic is emitted per failing pattern; a kind whose pattern
// matches contributes no diagnostic. Matching uses the same doublestar
// syntax as overrides:, ignore:, and kind-assignment:, anchored at the
// workspace root.
func (r *Rule) checkPathPatterns(f *lint.File) []lint.Diagnostic {
	if len(r.PathPatterns) == 0 {
		return nil
	}
	rel := filepath.ToSlash(workspaceRelPath(f))
	var diags []lint.Diagnostic
	for _, pp := range r.PathPatterns {
		// Match the full workspace-relative path with doublestar
		// directly. Going through globpath.Match would also try the
		// basename, which would let `path-pattern: README.md` pass
		// for `docs/README.md` — defeating the documented root-
		// anchored semantics.
		ok, err := doublestar.Match(filepath.ToSlash(pp.Pattern), rel)
		if err == nil && ok {
			continue
		}
		// path-pattern checks the workspace-relative path (which may
		// include directories), so the field label is "path" rather
		// than "filename". The latter is reserved for basename-only
		// checks emitted by validateFilename / checkFilenamePattern.
		d := schema.SchemaDiagnostic{
			Field:     "path",
			Actual:    fmt.Sprintf("%q", rel),
			Expected:  fmt.Sprintf("path matching glob %s", pp.Pattern),
			SchemaRef: fmt.Sprintf("kinds[%s] / path-pattern", pp.Kind),
		}
		diags = append(diags, d.Emit(makeDiag, f.Path, schema.NonBodyDiagLine(f)))
	}
	return diags
}

// workspaceRelPath returns the file path relative to the workspace
// root when RootDir is set, falling back to the file's own Path. The
// returned path is slash-normalized so glob patterns written with
// forward slashes match on every platform. Both RootDir and Path are
// resolved through filepath.Abs first so the relative computation
// works when the CLI was invoked with a relative `--config` path
// (e.g. `--config sub/.mdsmith.yml` makes RootDir relative). The
// `_ :=` discards mirror isSchemaFile's pattern above: filepath.Abs
// only fails when os.Getwd fails, which the engine would already
// have surfaced during file discovery.
func workspaceRelPath(f *lint.File) string {
	if f.RootDir == "" {
		return filepath.ToSlash(f.Path)
	}
	absRoot, _ := filepath.Abs(f.RootDir)
	absPath, _ := filepath.Abs(f.Path)
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return filepath.ToSlash(f.Path)
	}
	return filepath.ToSlash(rel)
}

// checkFilenamePattern checks that the document basename matches the
// schema's filename glob pattern (if configured).
func checkFilenamePattern(
	f *lint.File, sch *parsedSchema, schemaSource string,
) []lint.Diagnostic {
	pattern := sch.Config.FilenamePattern
	if pattern == "" {
		return nil
	}
	// Filename diagnostics describe the document as a whole;
	// use the non-body anchor so filterGeneratedDiags can't
	// drop them when body line 1 sits inside a generated
	// section.
	anchor := schema.NonBodyDiagLine(f)
	base := filepath.Base(f.Path)
	matched, err := filepath.Match(pattern, base)
	if err != nil {
		// Malformed glob in the schema. Surface it via the same
		// SchemaDiagnostic shape so the message carries the
		// schema reference and the user can jump to the
		// offending pattern.
		d := schema.SchemaDiagnostic{
			Field:     "filename pattern",
			Actual:    fmt.Sprintf("%q", pattern),
			Expected:  "valid glob",
			Hint:      err.Error(),
			SchemaRef: buildSchemaRefForLegacy(schemaSource),
		}
		return []lint.Diagnostic{d.Emit(makeDiag, f.Path, anchor)}
	}
	if !matched {
		// `glob` keeps the wording aligned with schema.validateFilename
		// and the path-pattern diagnostic; see the rationale there.
		d := schema.SchemaDiagnostic{
			Field:     "filename",
			Actual:    fmt.Sprintf("%q", base),
			Expected:  fmt.Sprintf("filename matching glob %s", pattern),
			SchemaRef: buildSchemaRefForLegacy(schemaSource),
		}
		return []lint.Diagnostic{d.Emit(makeDiag, f.Path, anchor)}
	}
	return nil
}

// readSchemaFile reads the schema file using the file's RootFS when available,
// falling back to os.ReadFile. When RootFS is set, absolute and
// parent-traversal paths are rejected to prevent reading outside the
// project root.
func readSchemaFile(f *lint.File, schema string) ([]byte, error) {
	if f.RootFS != nil {
		// Reject absolute paths and parent traversals.
		if filepath.IsAbs(schema) {
			return nil, fmt.Errorf("absolute schema path not allowed")
		}
		clean := filepath.ToSlash(filepath.Clean(schema))
		clean = strings.TrimPrefix(clean, "./")
		if clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, fmt.Errorf("schema path %q escapes project root", schema)
		}
		return bytelimit.ReadFSFileLimited(f.RootFS, clean, f.MaxInputBytes)
	}
	return bytelimit.ReadFileLimited(schema, f.MaxInputBytes)
}

// schemaRootFS returns f's workspace RootFS, or nil when f is nil. It is
// the seam cachedParseSchema uses to feed RootFS-based <?include?>
// fragment reads (see parseSchemaWithRootFS).
func schemaRootFS(f *lint.File) fs.FS {
	if f == nil {
		return nil
	}
	return f.RootFS
}

// readSchemaInclude reads a schema <?include?> fragment. It prefers the
// workspace RootFS — the Session/WASM and LSP path, where os.ReadFile is
// absent or "not implemented on js" — and falls back to the OS filesystem
// for the on-disk CLI. includedPath is project-relative
// (resolveSchemaIncludePath rejects ".."), so it is a valid fs.FS key.
func readSchemaInclude(rootFS fs.FS, includedPath string, maxBytes int64) ([]byte, error) {
	if rootFS != nil {
		return bytelimit.ReadFSFileLimited(rootFS, filepath.ToSlash(includedPath), maxBytes)
	}
	return bytelimit.ReadFileLimited(includedPath, maxBytes)
}

func makeDiag(file string, line int, msg string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     file,
		Line:     line,
		Column:   1,
		RuleID:   "MDS020",
		RuleName: "required-structure",
		Severity: lint.Error,
		Message:  msg,
	}
}

// FixTitle implements rule.QuickFixTitler.
func (r *Rule) FixTitle() string { return "Sync section to front matter" }
