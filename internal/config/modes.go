package config

import "github.com/jeduden/mdsmith/internal/rule"

// init wires the default merge-mode lookup so Effective uses the rule
// registry to discover per-rule merge declarations. Tests that want
// custom merge behavior can call effectiveRulesWithModes directly.
func init() {
	defaultMergeModes = registryMergeModes
}

// registryMergeModes returns the per-key merge-mode table for ruleName,
// or nil if the rule isn't registered or doesn't implement
// rule.SettingsMerger.
func registryMergeModes(ruleName string) map[string]MergeMode {
	for _, r := range rule.All() {
		if r.Name() != ruleName {
			continue
		}
		sm, ok := r.(rule.SettingsMerger)
		if !ok {
			return nil
		}
		raw := sm.SettingsMergeModes()
		if len(raw) == 0 {
			return nil
		}
		out := make(map[string]MergeMode, len(raw))
		for k, v := range raw {
			out[k] = fromRuleMode(v)
		}
		return out
	}
	return nil
}

// fromRuleMode converts a rule.SettingsMergeMode to the internal
// config.MergeMode used by deepMergeRuleCfg.
func fromRuleMode(m rule.SettingsMergeMode) MergeMode {
	switch m {
	case rule.MergeAppend:
		return MergeAppend
	default:
		return MergeReplace
	}
}
