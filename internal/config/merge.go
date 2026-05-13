package config

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
		out[k] = UserConvention{Flavor: v.Flavor, Rules: rules}
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
			Schema:      cloneSettings(body.Schema),
			PathPattern: body.PathPattern,
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

// copyKindAssignment returns a deep copy of a kind-assignment slice.
// Returns nil if input is nil.
func copyKindAssignment(entries []KindAssignmentEntry) []KindAssignmentEntry {
	if entries == nil {
		return nil
	}
	result := make([]KindAssignmentEntry, len(entries))
	for i, e := range entries {
		result[i] = KindAssignmentEntry{
			Glob:  copyStrings(e.Glob),
			Files: copyStrings(e.Files),
			Kinds: copyStrings(e.Kinds),
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
// When cfg is nil there are no kind-assignment globs to apply, so
// the result is just fmKinds with duplicates dropped — preserving
// the dedup contract callers rely on.
func EffectiveKinds(cfg *Config, filePath string, fmKinds []string) []string {
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
	return resolveEffectiveKinds(cfg, filePath, fmKinds)
}

// resolveEffectiveKinds builds the ordered, deduplicated effective kind list
// for a file. fmKinds are the kinds declared in the file's front matter;
// they come first. kind-assignment matches are appended in config order.
// Duplicate names are dropped after their first occurrence.
func resolveEffectiveKinds(cfg *Config, filePath string, fmKinds []string) []string {
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
		if matchesAny(entry.Patterns(), filePath) {
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
func Effective(cfg *Config, filePath string, fmKinds []string) map[string]RuleCfg {
	return effectiveRules(cfg, filePath, resolveEffectiveKinds(cfg, filePath, fmKinds))
}

// EffectiveExplicitRules returns the set of rule names that were explicitly
// configured for a given file path. It includes rules from the top-level
// ExplicitRules, any rules set by matching kinds, and any rules set by
// matching overrides.
func EffectiveExplicitRules(cfg *Config, filePath string, fmKinds []string) map[string]bool {
	return effectiveExplicit(cfg, filePath, resolveEffectiveKinds(cfg, filePath, fmKinds))
}

// EffectiveCategories returns the effective category settings for a given
// file path. It starts with the top-level categories, applies kinds in
// effective-list order, and then applies matching overrides. Categories not
// explicitly set default to true (enabled).
func EffectiveCategories(cfg *Config, filePath string, fmKinds []string) map[string]bool {
	return effectiveCats(cfg, filePath, resolveEffectiveKinds(cfg, filePath, fmKinds))
}

// EffectiveAll returns the effective rule config, category settings, and
// explicit rule set for a file path while resolving effective kinds once and
// reusing that result across all three computations.
func EffectiveAll(
	cfg *Config, filePath string, fmKinds []string,
) (map[string]RuleCfg, map[string]bool, map[string]bool) {
	kinds := resolveEffectiveKinds(cfg, filePath, fmKinds)
	return effectiveRules(cfg, filePath, kinds),
		effectiveCats(cfg, filePath, kinds),
		effectiveExplicit(cfg, filePath, kinds)
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
	result := make(map[string]RuleCfg, len(cfg.Rules))
	for k, v := range cfg.Rules {
		if !cfg.ExplicitRules[k] {
			result[k] = copyRuleCfg(v)
		}
	}
	apply := func(name string, layer RuleCfg) {
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
		// When a kind sets either schema source, treat it as a
		// fresh schema declaration: clear any prior schema state
		// on required-structure so the last source wins
		// unambiguously across mixed kinds.
		if kindDeclaresSchema(body) {
			clearSchemaState(result)
		}
		if len(body.Schema) > 0 {
			applyInlineSchema(result, body.Schema)
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
			if overrideDeclaresSchema(o) {
				clearSchemaState(result)
			}
			for k, v := range o.Rules {
				apply(k, v)
			}
		}
	}
	return result
}

// kindDeclaresSchema reports whether a kind body declares a schema
// source — either inline (KindBody.Schema) or via the
// rules.required-structure.{schema,inline-schema} settings.
func kindDeclaresSchema(body KindBody) bool {
	if len(body.Schema) > 0 {
		return true
	}
	return rulesDeclareSchema(body.Rules)
}

// overrideDeclaresSchema reports whether a glob override sets a
// schema source on required-structure. Both schema sources count —
// without this, an inline schema installed by an override would
// leave a prior file-schema path intact and "last source wins"
// could yield ambiguous configs.
func overrideDeclaresSchema(o Override) bool {
	return rulesDeclareSchema(o.Rules)
}

// rulesDeclareSchema reports whether a per-layer rules map sets
// either schema source on required-structure. A bool-only entry
// (`required-structure: true/false`) leaves Settings nil; bail
// before the lookups to keep the contract explicit.
func rulesDeclareSchema(rules map[string]RuleCfg) bool {
	rs, ok := rules["required-structure"]
	if !ok {
		return false
	}
	if rs.Settings == nil {
		return false
	}
	if path, ok := rs.Settings["schema"].(string); ok && path != "" {
		return true
	}
	if m, ok := rs.Settings["inline-schema"].(map[string]any); ok && len(m) > 0 {
		return true
	}
	return false
}

// clearSchemaState removes any prior schema source from the
// accumulated effective config for required-structure. Both the
// inline-schema map and the file-schema path are cleared so the
// incoming layer can install its own source unambiguously.
func clearSchemaState(result map[string]RuleCfg) {
	rs, ok := result["required-structure"]
	if !ok {
		return
	}
	if rs.Settings == nil {
		return
	}
	delete(rs.Settings, "schema")
	delete(rs.Settings, "inline-schema")
	result["required-structure"] = rs
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

// applyInlineSchema installs an inline schema (a YAML map) as the
// `inline-schema` setting on required-structure, creating the rule
// entry if missing.
func applyInlineSchema(result map[string]RuleCfg, schema map[string]any) {
	rs, ok := result["required-structure"]
	if !ok {
		rs = RuleCfg{Enabled: true}
	}
	if rs.Settings == nil {
		rs.Settings = map[string]any{}
	}
	rs.Settings["inline-schema"] = cloneSettings(schema)
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
		if len(body.Schema) > 0 || body.PathPattern != "" {
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
