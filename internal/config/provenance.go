package config

import (
	"fmt"
	"sort"
	"strings"
)

// Provenance layer-source string forms. A layer source is a stable
// identifier that names one step in the rule-config merge pipeline.
//
//   - "default"               built-in defaults plus the user's top-level rules:
//   - "kinds.<name>"          a kind body in the file's effective kind list
//   - "overrides[<i>]"        the i-th override entry that matched this file
//   - "front-matter override" the file's own front-matter rule overrides
//
// "default" collapses built-in defaults and the user's top-level rules:
// block, since cfg.Rules already has them merged. "front-matter override"
// is reserved for the future per-file front-matter rules: feature.
const (
	layerSourceDefault     = "default"
	layerSourceFrontMatter = "front-matter override"
)

// KindAssignmentSource describes how a kind ended up in the effective list.
// Either "front-matter" or "kind-assignment[<i>]".
type KindAssignmentSource string

// ResolvedKind names a kind in the effective list and how it was assigned.
type ResolvedKind struct {
	Name   string
	Source KindAssignmentSource
}

// LayerEntry is one applicable merge layer for a single rule. Source
// identifies the layer; Set indicates whether this layer touched the rule;
// Value, when Set is true, is the rule's RuleCfg supplied by this layer.
type LayerEntry struct {
	Source string
	Set    bool
	Value  RuleCfg
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

// FileResolution is the per-file resolution: kind list (with assignment
// sources) and per-rule resolution. Rules is keyed by rule name.
type FileResolution struct {
	File       string
	Kinds      []ResolvedKind
	Rules      map[string]RuleResolution
	Categories map[string]bool
}

// ResolveFile builds the full provenance picture for a single file.
// fmKinds is the kinds: list parsed from the file's front matter.
func ResolveFile(cfg *Config, filePath string, fmKinds []string) *FileResolution {
	kinds := resolveKindsWithSources(cfg, filePath, fmKinds)
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
		Kinds:      kinds,
		Rules:      rules,
		Categories: cats,
	}
}

// layerInfo captures one applicable merge layer's source and its rule
// settings. Layers that are not applicable to the file (non-matching
// overrides) are not included.
type layerInfo struct {
	Source string
	Rules  map[string]RuleCfg
}

func buildLayers(cfg *Config, filePath string, kinds []ResolvedKind) []layerInfo {
	layers := []layerInfo{
		{Source: layerSourceDefault, Rules: cfg.Rules},
	}
	for _, k := range kinds {
		body, ok := cfg.Kinds[k.Name]
		if !ok {
			continue
		}
		layers = append(layers, layerInfo{
			Source: "kinds." + k.Name,
			Rules:  body.Rules,
		})
	}
	for i, o := range cfg.Overrides {
		if matchesAny(o.Files, filePath) {
			layers = append(layers, layerInfo{
				Source: fmt.Sprintf("overrides[%d]", i),
				Rules:  o.Rules,
			})
		}
	}
	return layers
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
			chain = append(chain, LayerEntry{Source: l.Source, Set: true, Value: cp})
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
			chain = append(chain, LayerEntry{Source: l.Source, Set: false})
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

func resolveKindsWithSources(cfg *Config, filePath string, fmKinds []string) []ResolvedKind {
	seen := make(map[string]bool)
	var result []ResolvedKind
	add := func(name string, source KindAssignmentSource) {
		if seen[name] {
			return
		}
		seen[name] = true
		result = append(result, ResolvedKind{Name: name, Source: source})
	}
	for _, k := range fmKinds {
		add(k, "front-matter")
	}
	for i, entry := range cfg.KindAssignment {
		if matchesAny(entry.Files, filePath) {
			src := KindAssignmentSource(fmt.Sprintf("kind-assignment[%d]", i))
			for _, k := range entry.Kinds {
				add(k, src)
			}
		}
	}
	return result
}
