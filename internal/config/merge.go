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
	archetypes := defaults.Archetypes
	if len(loaded.Archetypes.Roots) > 0 {
		archetypes = ArchetypesCfg{Roots: copyStrings(loaded.Archetypes.Roots)}
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
		Archetypes:             archetypes,
		Kinds:                  copyKinds(loaded.Kinds),
		KindAssignment:         copyKindAssignment(loaded.KindAssignment),
		Convention:             loaded.Convention,
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
		Archetypes:             ArchetypesCfg{Roots: copyStrings(cfg.Archetypes.Roots)},
		Kinds:                  copyKinds(cfg.Kinds),
		KindAssignment:         copyKindAssignment(cfg.KindAssignment),
		Convention:             cfg.Convention,
		ConventionPreset:       copyConventionPreset(cfg.ConventionPreset),
	}
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
			Rules:      rules,
			Categories: copyCategories(body.Categories),
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
		if matchesAny(entry.Files, filePath) {
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
	result := make(map[string]RuleCfg, len(cfg.Rules))
	for k, v := range cfg.ConventionPreset {
		result[k] = copyRuleCfg(v)
	}
	apply := func(name string, layer RuleCfg) {
		if existing, ok := result[name]; ok {
			result[name] = mergeRuleCfg(name, existing, layer)
			return
		}
		result[name] = copyRuleCfg(layer)
	}
	for k, v := range cfg.Rules {
		apply(k, v)
	}
	for _, kindName := range kinds {
		body, ok := cfg.Kinds[kindName]
		if !ok {
			continue
		}
		for k, v := range body.Rules {
			apply(k, v)
		}
	}
	for _, o := range cfg.Overrides {
		if matchesAny(o.Files, filePath) {
			for k, v := range o.Rules {
				apply(k, v)
			}
		}
	}
	return result
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
	}
	for _, o := range cfg.Overrides {
		if matchesAny(o.Files, filePath) {
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
		if matchesAny(o.Files, filePath) {
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

// InjectArchetypeRoots copies cfg.Archetypes.Roots into every
// required-structure rule block (top-level, override, or kind) that does not
// already set its own archetype-roots. This is a no-op when no roots
// are configured at the top level. Rules with archetype-roots already
// specified are left untouched.
func InjectArchetypeRoots(cfg *Config) {
	if cfg == nil || len(cfg.Archetypes.Roots) == 0 {
		return
	}
	roots := cfg.Archetypes.Roots
	injectRoots(cfg.Rules, roots)
	for i := range cfg.Overrides {
		injectRoots(cfg.Overrides[i].Rules, roots)
	}
	for name, body := range cfg.Kinds {
		injectRoots(body.Rules, roots)
		cfg.Kinds[name] = body
	}
}

func injectRoots(rules map[string]RuleCfg, roots []string) {
	const name = "required-structure"
	rc, ok := rules[name]
	if !ok || !rc.Enabled {
		return
	}
	if rc.Settings == nil {
		rc.Settings = map[string]any{}
	}
	if _, exists := rc.Settings["archetype-roots"]; exists {
		return
	}
	rootsAny := make([]any, len(roots))
	for i, r := range roots {
		rootsAny[i] = r
	}
	rc.Settings["archetype-roots"] = rootsAny
	rules[name] = rc
}
