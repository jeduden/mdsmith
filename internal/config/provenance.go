package config

import "fmt"

// Layer identifies which configuration layer set a given leaf value.
// The layers apply in order: default, kind, override, front-matter
// override. Front-matter overrides apply to the file's own front matter
// when overrides: keys appear there (currently a no-op surface; the type
// is reserved so plan 96+ can introduce it without changing the model).
type Layer string

// Layer identifiers for provenance tracking.
const (
	LayerDefault         Layer = "default"
	LayerKind            Layer = "kind"
	LayerOverride        Layer = "override"
	LayerFrontMatter     Layer = "front-matter override"
	LayerCategoryDisable Layer = "category"
)

// ChainEntry records that one layer touched a particular leaf, with the
// value the layer wrote and a human-readable Source label.
//
// Source labels:
//   - "default"
//   - "kinds.<name>"
//   - "overrides[i]"
//   - "front-matter override"
//   - "categories.<name>"
type ChainEntry struct {
	Layer   Layer
	Source  string
	Value   any
	Touched bool
}

// LeafProvenance holds the merge chain for one effective leaf setting on
// one file, plus the final value and the label of the layer that won.
type LeafProvenance struct {
	Final         any
	WinningSource string
	Chain         []ChainEntry
}

// RuleProvenance holds the per-leaf provenance for one rule on one file
// plus the final RuleCfg. Leaves keys are setting names; the synthetic
// key "enabled" carries the on/off bit.
type RuleProvenance struct {
	Final  RuleCfg
	Leaves map[string]LeafProvenance
}

// Resolution is the per-file output of ResolveWithProvenance. It carries
// the effective kind list (with sources), the merged rule config, and
// per-rule per-leaf provenance.
type Resolution struct {
	File           string
	EffectiveKinds []string
	// KindSources maps each effective kind name to the layers that
	// contributed it: "front-matter" or "kind-assignment[i]".
	KindSources map[string][]string
	Rules       map[string]RuleProvenance
	Categories  map[string]bool
	Explicit    map[string]bool
}

// ResolveWithProvenance walks the merge pipeline for a single file and
// records, per leaf setting, every layer that wrote it. The output is
// equivalent to Effective + EffectiveCategories + EffectiveExplicit but
// also exposes the chain of layers that produced each leaf's final
// value.
func ResolveWithProvenance(cfg *Config, filePath string, fmKinds []string) *Resolution {
	if cfg == nil {
		return &Resolution{File: filePath, Rules: map[string]RuleProvenance{}}
	}

	effKinds := resolveEffectiveKinds(cfg, filePath, fmKinds)
	rules := mergeRulesWithProvenance(cfg, filePath, effKinds)

	// Compute final RuleCfg map and compute categories/explicit using the
	// same paths as Effective so callers see exactly the same answer.
	finalRules, cats, explicit := EffectiveAll(cfg, filePath, fmKinds)

	// Replace each rule's Final with the canonical merged value (so
	// the caller sees the same struct as Effective() returns) and
	// add any rule that only exists in finalRules but not in our chain
	// (defensive: all paths above already handle this).
	for name, rc := range finalRules {
		rp, ok := rules[name]
		if !ok {
			rp = RuleProvenance{Leaves: map[string]LeafProvenance{}}
		}
		rp.Final = rc
		rules[name] = rp
	}

	return &Resolution{
		File:           filePath,
		EffectiveKinds: effKinds,
		KindSources:    buildKindSources(cfg, filePath, fmKinds, effKinds),
		Rules:          rules,
		Categories:     cats,
		Explicit:       explicit,
	}
}

// mergeRulesWithProvenance walks the rule layers (default, kinds,
// overrides) for one file and records every per-leaf write. The
// returned map's Final fields are still raw copies; callers that want
// the canonical post-EffectiveAll RuleCfg should overwrite Final with
// it before returning.
func mergeRulesWithProvenance(cfg *Config, filePath string, effKinds []string) map[string]RuleProvenance {
	rules := map[string]RuleProvenance{}

	apply := func(ruleName string, rc RuleCfg, layer Layer, source string) {
		rp, ok := rules[ruleName]
		if !ok {
			rp = RuleProvenance{Leaves: map[string]LeafProvenance{}}
		}
		rp.Final = copyRuleCfg(rc)
		recordLeaf(rp.Leaves, "enabled", rc.Enabled, layer, source)
		for k, v := range rc.Settings {
			recordLeaf(rp.Leaves, k, v, layer, source)
		}
		rules[ruleName] = rp
	}

	for name, rc := range cfg.Rules {
		apply(name, rc, LayerDefault, "default")
	}
	for _, kindName := range effKinds {
		body, ok := cfg.Kinds[kindName]
		if !ok {
			continue
		}
		for ruleName, rc := range body.Rules {
			apply(ruleName, rc, LayerKind, "kinds."+kindName)
		}
	}
	for i, o := range cfg.Overrides {
		if !matchesAny(o.Files, filePath) {
			continue
		}
		src := fmt.Sprintf("overrides[%d]", i)
		for ruleName, rc := range o.Rules {
			apply(ruleName, rc, LayerOverride, src)
		}
	}
	return rules
}

// recordLeaf appends a chain entry for one leaf and updates Final and
// WinningSource.
func recordLeaf(leaves map[string]LeafProvenance, key string, value any, layer Layer, source string) {
	lp, ok := leaves[key]
	if !ok {
		lp = LeafProvenance{}
	}
	lp.Chain = append(lp.Chain, ChainEntry{
		Layer:   layer,
		Source:  source,
		Value:   value,
		Touched: true,
	})
	lp.Final = value
	lp.WinningSource = source
	leaves[key] = lp
}

// buildKindSources walks fmKinds and cfg.KindAssignment to record where
// each effective kind name was contributed from.
func buildKindSources(
	cfg *Config, filePath string, fmKinds, effKinds []string,
) map[string][]string {
	sources := make(map[string][]string, len(effKinds))
	for _, k := range fmKinds {
		sources[k] = append(sources[k], "front-matter")
	}
	for i, entry := range cfg.KindAssignment {
		if !matchesAny(entry.Files, filePath) {
			continue
		}
		src := fmt.Sprintf("kind-assignment[%d]", i)
		for _, k := range entry.Kinds {
			sources[k] = append(sources[k], src)
		}
	}
	return sources
}

// ChainForRule returns every layer that potentially touched the given
// rule on the given file, in apply order, regardless of whether the
// layer actually wrote any leaf. No-op layers are reported with
// Touched=false. Suitable for `mdsmith kinds why <file> <rule>`.
func ChainForRule(cfg *Config, filePath string, fmKinds []string, ruleName string) []ChainEntry {
	if cfg == nil {
		return nil
	}
	var chain []ChainEntry

	// Default layer.
	if rc, ok := cfg.Rules[ruleName]; ok {
		chain = append(chain, ChainEntry{
			Layer: LayerDefault, Source: "default",
			Value: ruleCfgValue(rc), Touched: true,
		})
	} else {
		chain = append(chain, ChainEntry{
			Layer: LayerDefault, Source: "default", Touched: false,
		})
	}

	// Kinds layer.
	effKinds := resolveEffectiveKinds(cfg, filePath, fmKinds)
	for _, kindName := range effKinds {
		body, present := cfg.Kinds[kindName]
		if !present {
			continue
		}
		rc, touched := body.Rules[ruleName]
		entry := ChainEntry{
			Layer:   LayerKind,
			Source:  "kinds." + kindName,
			Touched: touched,
		}
		if touched {
			entry.Value = ruleCfgValue(rc)
		}
		chain = append(chain, entry)
	}

	// Overrides layer (only those that match).
	for i, o := range cfg.Overrides {
		if !matchesAny(o.Files, filePath) {
			continue
		}
		rc, touched := o.Rules[ruleName]
		entry := ChainEntry{
			Layer:   LayerOverride,
			Source:  fmt.Sprintf("overrides[%d]", i),
			Touched: touched,
		}
		if touched {
			entry.Value = ruleCfgValue(rc)
		}
		chain = append(chain, entry)
	}

	return chain
}

// ruleCfgValue produces a comparable snapshot of a RuleCfg suitable for
// the Value field of ChainEntry. For a disabled rule, returns false; for
// an enabled rule with no settings, returns true; otherwise returns the
// settings map.
func ruleCfgValue(rc RuleCfg) any {
	if !rc.Enabled && len(rc.Settings) == 0 {
		return false
	}
	if rc.Enabled && len(rc.Settings) == 0 {
		return true
	}
	out := map[string]any{}
	for k, v := range rc.Settings {
		out[k] = v
	}
	return out
}
