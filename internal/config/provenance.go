package config

import (
	"fmt"
	"sort"
	"strings"
)

// Provenance layer-source string forms. A layer source is a stable
// identifier that names one step in the rule-config merge pipeline.
// Layers are listed below in apply order (oldest → newest):
//
//   - "default"               built-in defaults: rules in cfg.Rules
//     that the user did not explicitly set
//   - "convention.<name>"     the markdown-flavor convention preset,
//     when set
//   - "user"                  the user's top-level rules block: rules
//     in cfg.Rules with an entry in
//     cfg.ExplicitRules
//   - "kinds.<name>"          a kind body in the file's effective
//     kind list
//   - "overrides[<i>]"        the i-th override entry that matched
//     this file
//   - "front-matter override" the file's own front-matter rule
//     overrides
//
// Splitting cfg.Rules into a "default" layer (defaults) and a
// "user" layer (explicit user entries) around the convention preset
// is what lets a convention enable a rule that is disabled by
// default. Without the split, the default's `Enabled: false` would
// land on top of the convention's `Enabled: true` and silently
// disable the rule. The "user" layer only appears when the user
// explicitly set at least one rule. "front-matter override" is
// reserved for the future per-file front-matter rules: feature.
const (
	layerSourceDefault     = "default"
	layerSourceUser        = "user"
	layerSourceFrontMatter = "front-matter override"
)

// KindAssignmentSource describes how a kind ended up in the effective list.
// Either "front-matter" or "kind-assignment[<i>]".
type KindAssignmentSource string

// ResolvedKind names a kind in the effective list and how it was assigned.
// Selector, when non-empty, describes the selectors that fired for a
// kind-assignment match ("glob a,b AND fields-present x"). It is empty
// for kinds declared via front matter. SourcePath, when set, is the
// file that defined the kind body (plan 208) — either `.mdsmith.yml`
// for inline kinds or `.mdsmith/kinds/<name>.{yaml,yml}` for
// file-defined kinds.
type ResolvedKind struct {
	Name       string
	Source     KindAssignmentSource
	Selector   string
	SourcePath string
	// SchemaSourcePath, when set, is the file that defined the kind's
	// schema, distinct from SourcePath (the kind's own file). It is
	// populated for a named registry reference (the
	// `.mdsmith/schemas/<name>.yaml` path, or `.mdsmith.yml` for an
	// inline-registry entry; plan 241) and for a `proto.md` schema
	// (the `rules.required-structure.schema:` path). Empty for an
	// inline-on-kind schema — SourcePath already names that file.
	SchemaSourcePath string
}

// ResolvedConvention names the active convention for a file and, for a
// user-defined convention, the file that defined it. Name is empty
// when no convention is selected. IsUser is true when the active
// convention is user-defined (declared inline in `.mdsmith.yml` or in
// a `.mdsmith/conventions/<name>.{yaml,yml}` file; plan 209) rather
// than a built-in. SourcePath is the defining file and is set only for
// a user convention — built-ins are compiled into the binary and carry
// no path.
type ResolvedConvention struct {
	Name       string
	IsUser     bool
	SourcePath string
}

// LayerEntry is one applicable merge layer for a single rule. Source
// identifies the layer; Set indicates whether this layer touched the rule;
// Value, when Set is true, is the rule's RuleCfg supplied by this layer.
// SourcePath, when set, is the file that defined the layer (plan 208) —
// populated for kind layers so audit output can name the file alongside
// the layer key (`kinds.<name>`). Empty for built-in defaults, the
// convention preset, the user layer, and override layers.
type LayerEntry struct {
	Source     string
	Set        bool
	Value      RuleCfg
	SourcePath string
}

// LeafChainEntry records a layer that set a single leaf, with the value
// the leaf had at that layer.
type LeafChainEntry struct {
	Source string
	Value  any
}

// Leaf bundles a leaf path (e.g., "enabled" or "settings.max"), its
// winning value, and the chain of layers that set it (oldest → newest).
type Leaf struct {
	Path  string
	Value any
	Chain []LeafChainEntry
}

// Source returns the winning layer source for this leaf — the source of
// the last entry in the chain. An empty string indicates the leaf has no
// source (which should not happen for leaves emitted by Resolve).
func (l Leaf) Source() string {
	if len(l.Chain) == 0 {
		return ""
	}
	return l.Chain[len(l.Chain)-1].Source
}

// RuleResolution describes the merge of one rule for one file.
type RuleResolution struct {
	Rule   string
	Final  RuleCfg
	Layers []LayerEntry
	Leaves []Leaf
}

// LeafByPath returns the Leaf with the given path, or nil if absent.
func (rr *RuleResolution) LeafByPath(path string) *Leaf {
	for i := range rr.Leaves {
		if rr.Leaves[i].Path == path {
			return &rr.Leaves[i]
		}
	}
	return nil
}

// FileResolution is the per-file resolution: the active convention
// (when one is selected), the kind list (with assignment sources), and
// per-rule resolution. Rules is keyed by rule name.
type FileResolution struct {
	File       string
	Convention ResolvedConvention
	Kinds      []ResolvedKind
	Rules      map[string]RuleResolution
	Categories map[string]bool
}

// ResolveFile builds the full provenance picture for a single file.
// fmKinds is the kinds: list parsed from the file's front matter;
// fmFields, when non-nil, is the parsed front matter and feeds the
// kind-assignment `fields-present:` selector.
func ResolveFile(cfg *Config, filePath string, fmKinds []string, fmFields map[string]any) *FileResolution {
	kinds := resolveKindsWithSources(cfg, filePath, fmKinds, fmFields)
	layers := buildLayers(cfg, filePath, kinds)

	names := allRuleNames(layers)
	rules := make(map[string]RuleResolution, len(names))
	for _, name := range names {
		rules[name] = buildRuleResolution(name, layers)
	}

	kindNames := make([]string, len(kinds))
	for i, k := range kinds {
		kindNames[i] = k.Name
	}
	cats := effectiveCats(cfg, filePath, kindNames)

	return &FileResolution{
		File:       filePath,
		Convention: resolveConvention(cfg),
		Kinds:      kinds,
		Rules:      rules,
		Categories: cats,
	}
}

// resolveConvention reports the active convention for a file: the
// selected convention name, whether it is user-defined, and (for a
// user convention) the file that defined it. It returns the zero
// ResolvedConvention when no convention is selected. A built-in
// convention is reported by name with no source path — built-ins are
// compiled into the binary. Mirrors how ResolvedKind carries a kind's
// defining file (plan 209).
func resolveConvention(cfg *Config) ResolvedConvention {
	if cfg.Convention == "" {
		return ResolvedConvention{}
	}
	rc := ResolvedConvention{Name: cfg.Convention}
	if uc, isUser := cfg.Conventions[cfg.Convention]; isUser {
		rc.IsUser = true
		rc.SourcePath = uc.SourcePath
	}
	return rc
}

// layerInfo captures one applicable merge layer's source and its rule
// settings. Layers that are not applicable to the file (non-matching
// overrides) are not included. SourcePath, when set, names the file
// the layer was loaded from — populated for kind layers only.
type layerInfo struct {
	Source     string
	SourcePath string
	Rules      map[string]RuleCfg
}

func buildLayers(cfg *Config, filePath string, kinds []ResolvedKind) []layerInfo {
	layers := make([]layerInfo, 0, 3+len(kinds)+len(cfg.Overrides))

	defaults, user := splitRulesByExplicit(cfg)

	if len(defaults) > 0 {
		layers = append(layers, layerInfo{
			Source: layerSourceDefault,
			Rules:  translateLayerRules(defaults),
		})
	}
	if cfg.Convention != "" && len(cfg.ConventionPreset) > 0 {
		source := "convention." + cfg.Convention
		if _, isUser := cfg.Conventions[cfg.Convention]; isUser {
			source += " (user)"
		}
		layers = append(layers, layerInfo{
			Source: source,
			Rules:  translateLayerRules(cfg.ConventionPreset),
		})
	}
	if len(user) > 0 {
		layers = append(layers, layerInfo{
			Source: layerSourceUser,
			Rules:  translateLayerRules(user),
		})
	}
	for _, k := range kinds {
		body, ok := cfg.Kinds[k.Name]
		if !ok {
			continue
		}
		layers = append(layers, layerInfo{
			Source:     "kinds." + k.Name,
			SourcePath: body.SourcePath,
			Rules:      kindLayerRules(k.Name, body, cfg.Kinds),
		})
	}
	for i, o := range cfg.Overrides {
		if matchesAny(o.Patterns(), filePath) {
			layers = append(layers, layerInfo{
				Source: fmt.Sprintf("overrides[%d]", i),
				Rules:  translateLayerRules(o.Rules),
			})
		}
	}
	return layers
}

// kindLayerRules returns the per-kind rules map seen by the
// provenance layer chain, mirroring the synthetic injections
// effectiveRules performs in merge.go. A kind body can configure
// required-structure outside body.Rules via two top-level fields
// on KindBody — `schema:` (an inline schema map) and
// `path-pattern:` — and the engine's merge layer translates each
// into a setting on the rule (`schema-sources` and `path-patterns`
// respectively). The body.Rules side also gets translated so user-
// written `schema:` / `inline-schema:` keys land in
// `schema-sources` ahead of the deep-merge. Without these mirrored
// translations, `mdsmith kinds resolve` and `--explain` output
// diverges from the rule config the engine actually applied.
//
// When the kind declares `extends:` (plan 135), the inline schema
// pushed to `schema-sources` is the chain-merged form rather than
// the kind's own block in isolation. The kinds map argument lets
// the resolver walk the chain; nil indicates "no chain context" and
// falls back to `body.Schema.Map()` directly (used by unit tests).
func kindLayerRules(
	kindName string, body KindBody, kinds map[string]KindBody,
) map[string]RuleCfg {
	inlineSchema := resolveLayerInlineSchema(kindName, body, kinds)
	if len(inlineSchema) == 0 && body.PathPattern == "" {
		return translateLayerRules(body.Rules)
	}
	out := make(map[string]RuleCfg, len(body.Rules)+1)
	for k, v := range body.Rules {
		out[k] = translateLayerSettings(k, v)
	}
	rs := out["required-structure"]
	rs.Enabled = true
	if rs.Settings == nil {
		rs.Settings = map[string]any{}
	} else {
		rs.Settings = cloneSettings(rs.Settings)
	}
	if len(inlineSchema) > 0 {
		entry := map[string]any{"inline": cloneSettings(inlineSchema)}
		existing, _ := rs.Settings["schema-sources"].([]any)
		rs.Settings["schema-sources"] = append(existing, entry)
	}
	if body.PathPattern != "" {
		entry := map[string]any{"kind": kindName, "pattern": body.PathPattern}
		existing, _ := rs.Settings["path-patterns"].([]any)
		rs.Settings["path-patterns"] = append(existing, entry)
	}
	out["required-structure"] = rs
	return out
}

// resolveLayerInlineSchema picks the inline schema map the provenance
// layer should attribute to one kind. It mirrors merge.go's
// resolvedInlineSchema but tolerates a nil kinds map so the
// table-driven unit tests for kindLayerRules don't have to
// construct a synthetic chain.
func resolveLayerInlineSchema(
	kindName string, body KindBody, kinds map[string]KindBody,
) map[string]any {
	if body.Extends == "" || kinds == nil {
		return body.Schema.Map()
	}
	resolved, err := ResolveKindInlineSchema(kinds, kindName)
	if err != nil {
		return body.Schema.Map()
	}
	return resolved
}

// translateLayerRules applies each rule's rule.SettingsTranslator
// (via translateLayerSettings) across a whole rules map so the
// provenance layer chain shows the same setting keys the engine
// merges on. Rules without a translator pass through unchanged.
// Provenance is not a hot path, so this always returns a fresh
// map rather than threading an allocation-free fast path.
func translateLayerRules(rules map[string]RuleCfg) map[string]RuleCfg {
	out := make(map[string]RuleCfg, len(rules))
	for k, v := range rules {
		out[k] = translateLayerSettings(k, v)
	}
	return out
}

// splitRulesByExplicit divides cfg.Rules into two maps using
// cfg.ExplicitRules as the discriminator: defaults (rules the user
// did not explicitly set) and user (rules with an entry in
// cfg.ExplicitRules). The split lets buildLayers emit "default" and
// "user" as distinct provenance layers.
func splitRulesByExplicit(cfg *Config) (defaults, user map[string]RuleCfg) {
	defaults = make(map[string]RuleCfg)
	user = make(map[string]RuleCfg)
	for k, v := range cfg.Rules {
		if cfg.ExplicitRules[k] {
			user[k] = v
		} else {
			defaults[k] = v
		}
	}
	return defaults, user
}

func allRuleNames(layers []layerInfo) []string {
	seen := map[string]bool{}
	for _, l := range layers {
		for name := range l.Rules {
			seen[name] = true
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func buildRuleResolution(name string, layers []layerInfo) RuleResolution {
	chain := make([]LayerEntry, 0, len(layers))
	var final RuleCfg
	var seen bool
	for _, l := range layers {
		v, ok := l.Rules[name]
		if ok {
			cp := copyRuleCfg(v)
			chain = append(chain, LayerEntry{
				Source:     l.Source,
				SourcePath: l.SourcePath,
				Set:        true,
				Value:      cp,
			})
			if !seen {
				final = cp
			} else {
				// Deep-merge later layers onto the running effective so
				// `final` mirrors the engine's merged config (e.g. a
				// bool-only kind toggling Enabled does not erase
				// inherited Settings).
				final = mergeRuleCfg(name, final, cp)
			}
			seen = true
		} else {
			chain = append(chain, LayerEntry{
				Source:     l.Source,
				SourcePath: l.SourcePath,
				Set:        false,
			})
		}
	}
	if !seen {
		// Rule never appears in any applicable layer; should not happen
		// when called via ResolveFile (allRuleNames filters to seen).
		return RuleResolution{Rule: name}
	}
	return RuleResolution{
		Rule:   name,
		Final:  final,
		Layers: chain,
		Leaves: buildLeaves(final, chain),
	}
}

func buildLeaves(final RuleCfg, chain []LayerEntry) []Leaf {
	paths := []string{"enabled"}
	if final.Settings != nil {
		keys := make([]string, 0, len(final.Settings))
		for k := range final.Settings {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			paths = append(paths, "settings."+k)
		}
	}

	leaves := make([]Leaf, 0, len(paths))
	for _, p := range paths {
		var leafChain []LeafChainEntry
		var winning any
		for _, layer := range chain {
			if !layer.Set {
				continue
			}
			v, ok := leafValue(layer.Value, p)
			if !ok {
				continue
			}
			leafChain = append(leafChain, LeafChainEntry{Source: layer.Source, Value: v})
			winning = v
		}
		leaves = append(leaves, Leaf{Path: p, Value: winning, Chain: leafChain})
	}
	return leaves
}

func leafValue(rc RuleCfg, path string) (any, bool) {
	if path == "enabled" {
		return rc.Enabled, true
	}
	const prefix = "settings."
	if strings.HasPrefix(path, prefix) {
		if rc.Settings == nil {
			return nil, false
		}
		v, ok := rc.Settings[path[len(prefix):]]
		return v, ok
	}
	return nil, false
}

// kindSchemaSourcePath returns the file that defined a kind's schema,
// when that file is distinct from the kind's own. A named registry
// reference carries the schema's origin on the ref (Schema.SourcePath
// — a `.mdsmith/schemas/<name>.yaml` path, or `.mdsmith.yml` for an
// inline-registry entry). A `proto.md` schema lives at the
// `rules.required-structure.schema:` path. An inline-on-kind schema
// has no separate file, so the result is empty and the kind's own
// SourcePath stands in.
func kindSchemaSourcePath(body KindBody) string {
	if body.Schema.SourcePath != "" {
		return body.Schema.SourcePath
	}
	if rs, ok := body.Rules["required-structure"]; ok {
		if set, path := schemaPathSetting(rs, true); set {
			return path
		}
	}
	return ""
}

func resolveKindsWithSources(cfg *Config, filePath string, fmKinds []string, fmFields map[string]any) []ResolvedKind {
	seen := make(map[string]bool)
	var result []ResolvedKind
	add := func(name string, source KindAssignmentSource, selector string) {
		if seen[name] {
			return
		}
		seen[name] = true
		// SourcePath comes from the kind body, not the assignment —
		// every assignment route to the same name resolves to the
		// same defining file. SchemaSourcePath, when the kind's schema
		// comes from a separate file (named YAML or proto.md), names
		// that file too.
		var srcPath, schemaSrc string
		if cfg != nil {
			if body, ok := cfg.Kinds[name]; ok {
				srcPath = body.SourcePath
				schemaSrc = kindSchemaSourcePath(body)
			}
		}
		result = append(result, ResolvedKind{
			Name: name, Source: source, Selector: selector,
			SourcePath: srcPath, SchemaSourcePath: schemaSrc,
		})
	}
	for _, k := range fmKinds {
		add(k, "front-matter", "")
	}
	for i, entry := range cfg.KindAssignment {
		matched, selector := matchKindAssignmentEntry(entry, filePath, fmFields)
		if !matched {
			continue
		}
		src := KindAssignmentSource(fmt.Sprintf("kind-assignment[%d]", i))
		for _, k := range entry.Kinds {
			add(k, src, selector)
		}
	}
	return result
}
