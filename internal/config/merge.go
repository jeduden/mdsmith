package config

import (
	"strconv"
	"strings"

	"github.com/jeduden/mdsmith/internal/rule"
)

// Merge merges a loaded config on top of defaults. The loaded config's rules
// override the defaults; any rule not mentioned in loaded keeps its default
// value. Ignore and Overrides come from the loaded config only.
// Categories from the loaded config are merged on top of defaults; any
// category not mentioned in loaded keeps its default value (true).
func Merge(defaults, loaded *Config) *Config {
	if loaded == nil {
		return copyConfig(defaults)
	}

	rules, explicit := mergeRules(defaults, loaded)

	fm := defaults.FrontMatter
	if loaded.FrontMatter != nil {
		fm = loaded.FrontMatter
	}
	cats := mergeCategories(defaults.Categories, loaded.Categories)

	files := copyStrings(defaults.Files)
	filesExplicit := false
	if loaded.FilesExplicit {
		files = copyStrings(loaded.Files)
		filesExplicit = true
	}

	maxInputSize := defaults.MaxInputSize
	if loaded.MaxInputSize != "" {
		maxInputSize = loaded.MaxInputSize
	}

	return &Config{
		Rules:                  rules,
		Ignore:                 loaded.Ignore,
		Overrides:              loaded.Overrides,
		FrontMatter:            fm,
		Categories:             cats,
		Files:                  files,
		FilesExplicit:          filesExplicit,
		ExplicitRules:          explicit,
		FollowSymlinks:         loaded.FollowSymlinks,
		LegacyNoFollowSymlinks: copyStrings(loaded.LegacyNoFollowSymlinks),
		Deprecations:           copyStrings(loaded.Deprecations),
		MaxInputSize:           maxInputSize,
		Kinds:                  copyKinds(loaded.Kinds),
		KindAssignment:         copyKindAssignment(loaded.KindAssignment),
		Build:                  copyBuildConfig(loaded.Build),
		Convention:             loaded.Convention,
		Conventions:            copyUserConventions(loaded.Conventions),
		ConventionPreset:       copyConventionPreset(loaded.ConventionPreset),
	}
}

// mergeRules combines the defaults' rule map with the loaded config's
// rules and reports which rules were explicitly set by the loaded
// layer (used downstream for category override resolution).
func mergeRules(defaults, loaded *Config) (map[string]RuleCfg, map[string]bool) {
	rules := make(map[string]RuleCfg, len(defaults.Rules))
	for k, v := range defaults.Rules {
		rules[k] = v
	}
	for k, v := range loaded.Rules {
		rules[k] = v
	}
	explicit := make(map[string]bool, len(loaded.Rules))
	for k := range loaded.Rules {
		explicit[k] = true
	}
	return rules, explicit
}

// copyConfig returns a shallow copy of a Config with copied slices and
// maps. Scalar fields and struct slices (Overrides) are shared by
// reference — the defaults case does not carry Ignore/Overrides data,
// but copying every field keeps the helper safe for any caller.
func copyConfig(cfg *Config) *Config {
	rules := make(map[string]RuleCfg, len(cfg.Rules))
	for k, v := range cfg.Rules {
		rules[k] = v
	}
	explicit := make(map[string]bool, len(cfg.ExplicitRules))
	for k, v := range cfg.ExplicitRules {
		explicit[k] = v
	}
	return &Config{
		Rules:                  rules,
		Ignore:                 copyStrings(cfg.Ignore),
		Overrides:              cfg.Overrides,
		FrontMatter:            cfg.FrontMatter,
		Categories:             copyCategories(cfg.Categories),
		Files:                  copyStrings(cfg.Files),
		FollowSymlinks:         cfg.FollowSymlinks,
		LegacyNoFollowSymlinks: copyStrings(cfg.LegacyNoFollowSymlinks),
		Deprecations:           copyStrings(cfg.Deprecations),
		MaxInputSize:           cfg.MaxInputSize,
		ExplicitRules:          explicit,
		FilesExplicit:          cfg.FilesExplicit,
		Kinds:                  copyKinds(cfg.Kinds),
		KindAssignment:         copyKindAssignment(cfg.KindAssignment),
		Build:                  copyBuildConfig(cfg.Build),
		Convention:             cfg.Convention,
		Conventions:            copyUserConventions(cfg.Conventions),
		ConventionPreset:       copyConventionPreset(cfg.ConventionPreset),
	}
}

// copyUserConventions returns a deep copy of a user-defined
// conventions map. Returns nil when the input is nil.
func copyUserConventions(m map[string]UserConvention) map[string]UserConvention {
	if m == nil {
		return nil
	}
	out := make(map[string]UserConvention, len(m))
	for k, v := range m {
		rules := make(map[string]RuleCfg, len(v.Rules))
		for rk, rv := range v.Rules {
			rules[rk] = copyRuleCfg(rv)
		}
		out[k] = UserConvention{Flavor: v.Flavor, Rules: rules, SourcePath: v.SourcePath}
	}
	return out
}

// copyBuildConfig returns a deep copy of a BuildConfig, duplicating the
// Recipes map and each recipe's Params slices so callers can mutate them
// independently.
func copyBuildConfig(b BuildConfig) BuildConfig {
	if len(b.Recipes) == 0 {
		return BuildConfig{BaseURL: b.BaseURL}
	}
	recipes := make(map[string]RecipeCfg, len(b.Recipes))
	for name, r := range b.Recipes {
		recipes[name] = RecipeCfg{
			Command:      r.Command,
			BodyTemplate: r.BodyTemplate,
			Params: ParamCfg{
				Required: copyStrings(r.Params.Required),
				Optional: copyStrings(r.Params.Optional),
			},
		}
	}
	return BuildConfig{BaseURL: b.BaseURL, Recipes: recipes}
}

// copyKinds returns a deep copy of a kinds map, including each RuleCfg's
// Settings map. Returns nil if input is nil.
func copyKinds(kinds map[string]KindBody) map[string]KindBody {
	if kinds == nil {
		return nil
	}
	result := make(map[string]KindBody, len(kinds))
	for name, body := range kinds {
		rules := make(map[string]RuleCfg, len(body.Rules))
		for k, v := range body.Rules {
			rules[k] = copyRuleCfg(v)
		}
		result[name] = KindBody{
			Rules:       rules,
			Categories:  copyCategories(body.Categories),
			Schema:      copySchemaRef(body.Schema),
			PathPattern: body.PathPattern,
			Extends:     body.Extends,
			SourcePath:  body.SourcePath,
		}
	}
	return result
}

// copyRuleCfg returns a copy of a RuleCfg with its Settings deeply cloned
// so that mutations to nested maps or slices do not affect the source.
func copyRuleCfg(rc RuleCfg) RuleCfg {
	rc.Settings = cloneSettings(rc.Settings)
	return rc
}

// copySchemaRef deep-copies a KindSchemaRef, cloning the resolved
// inline body so a mutation during merge cannot reach back into the
// loaded config. Name and SourcePath are scalars and copy by value.
// Resolution (resolveNamedSchemas) runs in Load before copyKinds, so
// a named ref's body is already filled in here and must survive the
// deep copy — hence cloning Map() rather than dropping it.
func copySchemaRef(ref KindSchemaRef) KindSchemaRef {
	return KindSchemaRef{
		Name:       ref.Name,
		SourcePath: ref.SourcePath,
		inline:     cloneSettings(ref.Map()),
	}
}

// copyKindAssignment returns a deep copy of a kind-assignment slice.
// Returns nil if input is nil.
func copyKindAssignment(entries []KindAssignmentEntry) []KindAssignmentEntry {
	if entries == nil {
		return nil
	}
	result := make([]KindAssignmentEntry, len(entries))
	for i, e := range entries {
		result[i] = KindAssignmentEntry{
			Glob:          copyStrings(e.Glob),
			Files:         copyStrings(e.Files),
			FieldsPresent: copyStrings(e.FieldsPresent),
			Kinds:         copyStrings(e.Kinds),
		}
	}
	return result
}

// copyStrings returns a copy of a string slice. Returns nil if the input is nil.
func copyStrings(s []string) []string {
	if s == nil {
		return nil
	}
	result := make([]string, len(s))
	copy(result, s)
	return result
}

// copyCategories returns a shallow copy of a categories map.
// Returns nil if the input is nil.
func copyCategories(cats map[string]bool) map[string]bool {
	if cats == nil {
		return nil
	}
	result := make(map[string]bool, len(cats))
	for k, v := range cats {
		result[k] = v
	}
	return result
}

// mergeCategories merges override categories on top of base categories.
// If base is nil, a copy of override is returned. If override is nil,
// a copy of base is returned. If both are nil, nil is returned.
func mergeCategories(base, override map[string]bool) map[string]bool {
	if base == nil && override == nil {
		return nil
	}
	result := make(map[string]bool, len(ValidCategories))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range override {
		result[k] = v
	}
	return result
}

// EffectiveKinds returns the same list resolveEffectiveKinds computes
// internally — the front-matter kinds plus every config-driven
// kind-assignment match in order, deduplicated. Exposed for callers
// outside the config package (e.g. the LSP symbol index) that need
// effective-kind resolution without re-implementing the merge rules.
//
// When cfg is nil there are no kind-assignment entries to apply, so
// the result is just fmKinds with duplicates dropped — preserving
// the dedup contract callers rely on.
//
// fmFields, when non-nil, is the file's parsed front matter; it is
// consumed by entries that set `fields-present:`. Pass nil when the
// caller has no FM info — such entries simply won't match.
func EffectiveKinds(cfg *Config, filePath string, fmKinds []string, fmFields map[string]any) []string {
	if cfg == nil {
		seen := make(map[string]bool, len(fmKinds))
		out := make([]string, 0, len(fmKinds))
		for _, k := range fmKinds {
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, k)
		}
		return out
	}
	return resolveEffectiveKinds(cfg, filePath, fmKinds, fmFields)
}

// resolveEffectiveKinds builds the ordered, deduplicated effective kind list
// for a file. fmKinds are the kinds declared in the file's front matter;
// they come first. kind-assignment matches are appended in config order.
// Duplicate names are dropped after their first occurrence.
func resolveEffectiveKinds(cfg *Config, filePath string, fmKinds []string, fmFields map[string]any) []string {
	seen := make(map[string]bool)
	var result []string

	add := func(name string) {
		if !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}

	for _, k := range fmKinds {
		add(k)
	}
	for _, entry := range cfg.KindAssignment {
		if matched, _ := matchKindAssignmentEntry(entry, filePath, fmFields); matched {
			for _, k := range entry.Kinds {
				add(k)
			}
		}
	}
	return result
}

// Effective returns the effective rule configuration for a given file path.
// It starts with the top-level rules, applies kinds in effective-list order
// (fmKinds from front matter first, then kind-assignment matches), and
// finally applies glob overrides. Later entries take precedence.
func Effective(cfg *Config, filePath string, fmKinds []string, fmFields map[string]any) map[string]RuleCfg {
	return effectiveRules(cfg, filePath, resolveEffectiveKinds(cfg, filePath, fmKinds, fmFields))
}

// EffectiveExplicitRules returns the set of rule names that were explicitly
// configured for a given file path. It includes rules from the top-level
// ExplicitRules, any rules set by matching kinds, and any rules set by
// matching overrides.
func EffectiveExplicitRules(cfg *Config, filePath string, fmKinds []string, fmFields map[string]any) map[string]bool {
	return effectiveExplicit(cfg, filePath, resolveEffectiveKinds(cfg, filePath, fmKinds, fmFields))
}

// EffectiveCategories returns the effective category settings for a given
// file path. It starts with the top-level categories, applies kinds in
// effective-list order, and then applies matching overrides. Categories not
// explicitly set default to true (enabled).
func EffectiveCategories(cfg *Config, filePath string, fmKinds []string, fmFields map[string]any) map[string]bool {
	return effectiveCats(cfg, filePath, resolveEffectiveKinds(cfg, filePath, fmKinds, fmFields))
}

// EffectiveAll returns the effective rule config, category settings, and
// explicit rule set for a file path while resolving effective kinds once and
// reusing that result across all three computations.
func EffectiveAll(
	cfg *Config, filePath string, fmKinds []string, fmFields map[string]any,
) (map[string]RuleCfg, map[string]bool, map[string]bool) {
	kinds := resolveEffectiveKinds(cfg, filePath, fmKinds, fmFields)
	return EffectiveAllForKinds(cfg, filePath, kinds)
}

// EffectiveAllForKinds is EffectiveAll with the effective kind list
// already resolved (see EffectiveSignature). The engine resolves kinds
// once to build a memo key, then passes them here so a cache miss does
// not re-resolve them.
func EffectiveAllForKinds(
	cfg *Config, filePath string, kinds []string,
) (map[string]RuleCfg, map[string]bool, map[string]bool) {
	return effectiveRules(cfg, filePath, kinds),
		effectiveCats(cfg, filePath, kinds),
		effectiveExplicit(cfg, filePath, kinds)
}

// EffectiveSignature returns a key that uniquely determines the result of
// EffectiveAll for (filePath, fmKinds, fmFields), along with the resolved
// kind list. filePath influences EffectiveAll only through the resolved
// kinds and the overrides whose patterns match it; effectiveRules,
// effectiveCats, and effectiveExplicit consult no other path-derived
// input. A key built from the ordered kind list plus the matching
// override indices is therefore sufficient — equal signatures yield
// identical (rules, categories, explicit) maps — so the engine can
// memoize on it and reuse the returned kinds for EffectiveAllForKinds.
func EffectiveSignature(
	cfg *Config, filePath string, fmKinds []string, fmFields map[string]any,
) (string, []string) {
	kinds := resolveEffectiveKinds(cfg, filePath, fmKinds, fmFields)
	var b strings.Builder
	for _, k := range kinds {
		b.WriteString(k)
		b.WriteByte(0x1f) // unit separator; kind names are YAML-parsed, so cannot contain control bytes
	}
	b.WriteByte(0x1e) // record separator between the kinds and the overrides
	for i := range cfg.Overrides {
		if matchesAny(cfg.Overrides[i].Patterns(), filePath) {
			b.WriteString(strconv.Itoa(i))
			b.WriteByte(',')
		}
	}
	return b.String(), kinds
}

func effectiveRules(cfg *Config, filePath string, kinds []string) map[string]RuleCfg {
	// Layer order, oldest → newest:
	//   1. defaults     (cfg.Rules entries the user did not explicitly set)
	//   2. convention   (cfg.ConventionPreset, when set)
	//   3. user         (cfg.Rules entries the user explicitly set)
	//   4. kinds        (each kind body in the file's effective list)
	//   5. overrides    (each matching override entry)
	//
	// Splitting cfg.Rules into "default" and "user" layers around
	// the convention is the only way a convention preset can flip
	// a rule that is disabled by default (e.g. MDS034 markdown-flavor
	// is opt-in; `convention: portable` should enable it). Without
	// the split, the default's `Enabled: false` would land on top of
	// the convention's `Enabled: true` and silently disable the rule
	// the user just asked the convention to enable.
	//
	// Some rules rewrite a layer's settings before deep-merge by
	// implementing rule.SettingsTranslator (required-structure
	// collapses `schema:` / `inline-schema:` into an append-mode
	// `schema-sources` list so multiple kinds compose, plan 156).
	// translateLayerSettings consults the rule registry, so this
	// merge code carries no rule-name special cases.
	result := make(map[string]RuleCfg, len(cfg.Rules))
	for k, v := range cfg.Rules {
		if !cfg.ExplicitRules[k] {
			result[k] = copyRuleCfg(translateLayerSettings(k, v))
		}
	}
	apply := func(name string, layer RuleCfg) {
		layer = translateLayerSettings(name, layer)
		if existing, ok := result[name]; ok {
			result[name] = mergeRuleCfg(name, existing, layer)
			return
		}
		result[name] = copyRuleCfg(layer)
	}
	for k, v := range cfg.ConventionPreset {
		apply(k, v)
	}
	for k, v := range cfg.Rules {
		if cfg.ExplicitRules[k] {
			apply(k, v)
		}
	}
	for _, kindName := range kinds {
		body, ok := cfg.Kinds[kindName]
		if !ok {
			continue
		}
		if resolved := resolvedInlineSchema(cfg.Kinds, kindName, body); len(resolved) > 0 {
			applyInlineSchemaSource(result, resolved, schemaSourcePath(body))
		}
		if body.PathPattern != "" {
			applyPathPattern(result, kindName, body.PathPattern)
		}
		for k, v := range body.Rules {
			apply(k, v)
		}
	}
	for _, o := range cfg.Overrides {
		if matchesAny(o.Patterns(), filePath) {
			for k, v := range o.Rules {
				apply(k, v)
			}
		}
	}
	return result
}

// translateLayerSettings applies a rule's rule.SettingsTranslator
// (when it implements one) to a single config layer's settings
// before deep-merge. Rules without the interface — or rules not
// registered in this binary — pass through unchanged, matching the
// existing rule.ByName-driven ListMerger lookup in deepmerge.go.
//
// The merge code calls this for every layer keyed only by rule
// name, so it carries no rule-specific behaviour itself: a rule
// that needs its settings rewritten (e.g. required-structure
// collapsing `schema:`/`inline-schema:` into `schema-sources`)
// declares that by implementing the interface.
func translateLayerSettings(ruleName string, rc RuleCfg) RuleCfg {
	if rc.Settings == nil {
		return rc
	}
	r := rule.ByName(ruleName)
	if r == nil {
		return rc
	}
	t, ok := r.(rule.SettingsTranslator)
	if !ok {
		return rc
	}
	out := rc
	out.Settings = t.TranslateLayerSettings(rc.Settings)
	return out
}

// applyPathPattern appends a {kind, pattern} entry to the
// `path-patterns` list setting on required-structure, creating the
// rule entry if missing. Each kind in the file's effective kind list
// that declares a `path-pattern:` contributes one entry, so the rule
// can validate the workspace-relative path against every applicable
// pattern and emit one diagnostic per failure.
func applyPathPattern(result map[string]RuleCfg, kindName, pattern string) {
	rs, ok := result["required-structure"]
	if !ok {
		rs = RuleCfg{Enabled: true}
	}
	if rs.Settings == nil {
		rs.Settings = map[string]any{}
	}
	entry := map[string]any{"kind": kindName, "pattern": pattern}
	existing, _ := rs.Settings["path-patterns"].([]any)
	rs.Settings["path-patterns"] = append(existing, entry)
	rs.Enabled = true
	result["required-structure"] = rs
}

// resolvedInlineSchema returns the inline schema map that should be
// pushed into `schema-sources` for one kind. When the kind has no
// `extends:`, the result is the kind's own body.Schema (or nil if
// it doesn't declare one). When `extends:` is set, the resolver
// walks the chain and merges parent+child via plan-135 semantics.
// ValidateKinds is the authoritative gate for conflicts and cycles;
// a resolver error here means the kinds map was mutated after
// validation. The fallback in that case is the kind's own schema —
// the chain is broken so we surface no parent constraints, but the
// child's own declaration still applies so the rule keeps validating
// what the user asked for.
func resolvedInlineSchema(
	kinds map[string]KindBody, kindName string, body KindBody,
) map[string]any {
	if body.Extends == "" {
		return body.Schema.Map()
	}
	resolved, err := ResolveKindInlineSchema(kinds, kindName)
	if err != nil {
		return body.Schema.Map()
	}
	return resolved
}

// schemaSourcePath returns the defining-source path the schema-sources
// entry should carry for one kind's schema. A named reference resolves
// to the schema's own origin (Schema.SourcePath — a `.mdsmith/schemas/
// <name>.yaml` path or `.mdsmith.yml` for an inline-registry entry), so
// "go to schema" lands on the schema rather than the referencing kind.
// An inline-on-kind body has no separate origin (Schema.SourcePath
// empty), so it falls back to the kind's own defining file.
func schemaSourcePath(body KindBody) string {
	if body.Schema.SourcePath != "" {
		return body.Schema.SourcePath
	}
	return body.SourcePath
}

// applyInlineSchemaSource appends a resolved schema body as a
// `schema-sources` entry, tagged with the schema's defining-source
// path. Multiple kinds with schemas thus accumulate rather than
// overwrite.
func applyInlineSchemaSource(result map[string]RuleCfg, schema map[string]any, sourcePath string) {
	rs, ok := result["required-structure"]
	if !ok {
		rs = RuleCfg{Enabled: true}
	}
	if rs.Settings == nil {
		rs.Settings = map[string]any{}
	}
	entry := map[string]any{"inline": cloneSettings(schema)}
	// Carry the kind's defining file so an inline-schema violation
	// points the reader at <.mdsmith.yml or kind file> rather than the
	// bare "inline kind schema" label (plan 230). The rule reads this
	// as the schema reference; it is not part of the schema content.
	if sourcePath != "" {
		entry["source"] = sourcePath
	}
	existing, _ := rs.Settings["schema-sources"].([]any)
	rs.Settings["schema-sources"] = append(existing, entry)
	rs.Enabled = true
	result["required-structure"] = rs
}

func effectiveExplicit(cfg *Config, filePath string, kinds []string) map[string]bool {
	result := make(map[string]bool, len(cfg.ExplicitRules))
	for k := range cfg.ExplicitRules {
		result[k] = true
	}
	for _, kindName := range kinds {
		body, ok := cfg.Kinds[kindName]
		if !ok {
			continue
		}
		for k := range body.Rules {
			result[k] = true
		}
		// An inline `schema:` block on the kind is an explicit
		// configuration of required-structure even though it lives
		// outside body.Rules. Without this, a `meta` category
		// disable would silently wipe an inline-schema kind's
		// effect — inconsistent with the file-source path that
		// lives under body.Rules. The same reasoning applies to
		// `path-pattern:`.
		if len(body.Schema.Map()) > 0 || body.PathPattern != "" {
			result["required-structure"] = true
		}
	}
	for _, o := range cfg.Overrides {
		if matchesAny(o.Patterns(), filePath) {
			for k := range o.Rules {
				result[k] = true
			}
		}
	}
	return result
}

func effectiveCats(cfg *Config, filePath string, kinds []string) map[string]bool {
	result := make(map[string]bool, len(ValidCategories))
	for _, cat := range ValidCategories {
		result[cat] = true
	}
	for k, v := range cfg.Categories {
		result[k] = v
	}
	for _, kindName := range kinds {
		body, ok := cfg.Kinds[kindName]
		if !ok {
			continue
		}
		for k, v := range body.Categories {
			result[k] = v
		}
	}
	for _, o := range cfg.Overrides {
		if matchesAny(o.Patterns(), filePath) {
			for k, v := range o.Categories {
				result[k] = v
			}
		}
	}
	return result
}

// ApplyCategories disables rules whose category is disabled, unless
// the rule has been explicitly configured (present in the explicit rules
// map). ruleCategory maps a rule name to its category string.
// The explicit map contains rule names that were explicitly set in config
// (not just inherited from defaults).
func ApplyCategories(
	rules map[string]RuleCfg,
	categories map[string]bool,
	ruleCategory func(ruleName string) string,
	explicit map[string]bool,
) map[string]RuleCfg {
	result := make(map[string]RuleCfg, len(rules))
	for name, cfg := range rules {
		cat := ruleCategory(name)
		enabled, catSet := categories[cat]
		if catSet && !enabled && !explicit[name] {
			// Category is disabled and rule is not explicitly configured.
			result[name] = RuleCfg{Enabled: false, Settings: cfg.Settings}
		} else {
			result[name] = cfg
		}
	}
	return result
}

// matchesAny returns true if filePath matches any of the given glob patterns.
// It delegates to globMatchAny which checks the raw path, the cleaned path,
// and the base name, consistent with how ignore patterns are matched.
func matchesAny(patterns []string, filePath string) bool {
	return globMatchAny(patterns, filePath)
}
