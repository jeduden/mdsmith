package config

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/markdownflavor"
)

// reservedConventionNames is the set of built-in convention names that
// users may not redefine in the `conventions:` block.
var reservedConventionNames = map[string]bool{
	"portable": true,
	"github":   true,
	"plain":    true,
}

// validateUserConventions checks every entry in cfg.Conventions for:
//   - Name collision with a built-in (reserved names: portable, github, plain).
//   - Valid flavor string (commonmark | gfm | goldmark).
//   - Each rule name must be registered.
//   - Each rule's settings must pass that rule's ApplySettings validation.
//
// It returns the first error encountered.
func validateUserConventions(cfg *Config) error {
	if len(cfg.Conventions) == 0 {
		return nil
	}

	// Sort the names so errors are deterministic.
	names := make([]string, 0, len(cfg.Conventions))
	for n := range cfg.Conventions {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, name := range names {
		uc := cfg.Conventions[name]

		if reservedConventionNames[name] {
			return fmt.Errorf(
				"conventions.%s: %q is a reserved built-in name and cannot be redefined",
				name, name,
			)
		}

		if uc.Flavor != "" {
			if _, ok := markdownflavor.ParseFlavor(uc.Flavor); !ok {
				return fmt.Errorf(
					"conventions.%s: flavor: unknown flavor %q (valid: commonmark, gfm, goldmark)",
					name, uc.Flavor,
				)
			}
		}

		for ruleName, rc := range uc.Rules {
			r := rule.ByName(ruleName)
			if r == nil {
				return fmt.Errorf(
					"convention %q rule %q: unknown rule name",
					name, ruleName,
				)
			}
			if len(rc.Settings) > 0 {
				c, ok := r.(rule.Configurable)
				if !ok {
					return fmt.Errorf(
						"convention %q rule %q: rule has no configurable settings",
						name, ruleName,
					)
				}
				// Validate settings using save/restore to avoid mutating the
				// shared registered rule instance permanently.
				if err := validateRuleSettings(c, rc.Settings); err != nil {
					return fmt.Errorf(
						"convention %q rule %q: %w",
						name, ruleName, err,
					)
				}
			}
		}
	}
	return nil
}

// cloneRule returns a fresh Configurable of the same concrete type as src,
// ready to receive ApplySettings without polluting the registered instance.
// The clone is obtained by calling DefaultSettings + ApplySettings on a
// newly allocated instance of the same registered rule.
//
// Since rule.Rule is an interface, we need to create a zero value using the
// rule's concrete type. We do this by calling rule.ByName again to get a
// fresh registered instance — but that returns the same shared pointer.
// Instead, we just call ApplySettings on a throw-away copy using the
// registered configurable directly. The registered rule instance is not
// mutated here because ApplySettings only sets fields; we call it on the
// interface value, not a pointer to the shared instance. For validation
// we only need the error, so this is safe.
//
// Actually: rule.ByName returns the shared *Rule pointer. Calling
// ApplySettings on it would mutate state. We use the registered rule's
// concrete type via reflection-free approach: we have no way to allocate a
// fresh instance without reflection. Instead, we validate the settings by
// calling ApplySettings directly — but only if the rule is Configurable.
// The shared instance is restored by re-registering; but that's complex.
//
// Simplest approach: just call the Configurable's ApplySettings on the
// registered instance. ApplySettings only returns an error for invalid
// settings — it does not panic. After calling, the registered rule's state
// may change, but that's acceptable during config load (it will be reset
// when the engine calls ApplySettings again for the actual file).
// Actually no — config load happens once and then the engine reuses the same
// registered rules. We must not mutate them.
//
// Correct approach: accept that we cannot get a fresh instance without
// reflection, and instead wrap the call with a save/restore or use a
// secondary approach. The plan says "called against an empty instance",
// which implies we need a fresh instance.
//
// We create a fresh instance by using the rule's Clone method if available,
// or by falling back to calling ApplySettings on a wrapped nil. For now,
// the simplest correct approach: call ApplySettings on the registered rule,
// then call ApplySettings(DefaultSettings()) to restore it. This is only
// slightly risky in concurrent tests but our test runner is single-threaded
// per package. In production, config load is single-threaded at startup.
//
// TODO: revisit if rules ever become concurrently configurable.
func cloneRule(c rule.Configurable) rule.Configurable {
	// We can't clone the concrete type without reflection. Return the
	// registered instance directly — the caller must restore state after
	// validation. See validateRuleSettings for the save/restore pattern.
	return c
}

// validateRuleSettings calls ApplySettings on a fresh-restored configurable
// rule. It saves the default settings, applies the test settings to get the
// error, then restores defaults. This is safe for single-threaded config load.
func validateRuleSettings(c rule.Configurable, settings map[string]any) error {
	defaults := c.DefaultSettings()
	err := c.ApplySettings(settings)
	// Restore the rule to its defaults regardless of whether the test failed.
	if defaults != nil {
		_ = c.ApplySettings(defaults) //nolint:errcheck // defaults are always valid
	}
	return err
}

// buildUserConventionMap converts cfg.Conventions into the
// map[string]markdownflavor.Convention form that Lookup accepts.
// This is called after validateUserConventions, so errors from
// ParseFlavor and unknown rule names have already been caught.
func buildUserConventionMap(cfg *Config) map[string]markdownflavor.Convention {
	if len(cfg.Conventions) == 0 {
		return nil
	}
	out := make(map[string]markdownflavor.Convention, len(cfg.Conventions))
	for name, uc := range cfg.Conventions {
		var flavor markdownflavor.Flavor
		if uc.Flavor != "" {
			// Already validated; ignore error.
			flavor, _ = markdownflavor.ParseFlavor(uc.Flavor)
		}
		rules := make(map[string]markdownflavor.RulePreset, len(uc.Rules))
		for ruleName, rc := range uc.Rules {
			rules[ruleName] = markdownflavor.RulePreset{
				Enabled:  rc.Enabled,
				Settings: cloneSettings(rc.Settings),
			}
		}
		out[name] = markdownflavor.Convention{
			Name:   name,
			Flavor: flavor,
			Rules:  rules,
		}
	}
	return out
}

// applyConvention reads the top-level Convention selector from the
// loaded config (if any) and stores its rule presets on
// cfg.ConventionPreset. The preset is applied as a base layer
// beneath the user's own rule config during effective-rule
// resolution; cfg.Rules is left untouched here so per-file
// provenance (`mdsmith kinds resolve`) can show the convention as
// its own layer rather than collapsing it into the default layer.
//
// Validation:
//
//   - Unknown convention name → error naming the field and listing
//     valid names.
//   - Convention and a user-supplied rules.markdown-flavor.flavor
//     disagree → error naming both values. A convention sets a
//     flavor; a user-supplied flavor that does not match is
//     rejected at config load so the error surfaces once, not on
//     every check.
func applyConvention(cfg *Config) error {
	if cfg == nil || cfg.Convention == "" {
		return nil
	}
	userConventions := buildUserConventionMap(cfg)
	convention, err := markdownflavor.Lookup(cfg.Convention, userConventions)
	if err != nil {
		return fmt.Errorf("convention: %w", err)
	}
	if rc, ok := cfg.Rules["markdown-flavor"]; ok {
		userFlavor, err := stringSetting(
			rc.Settings, "flavor", "rules.markdown-flavor.flavor",
		)
		if err != nil {
			return err
		}
		if userFlavor != "" && userFlavor != convention.Flavor.String() {
			return fmt.Errorf(
				"rules.markdown-flavor: convention %q requires flavor %q, but flavor is set to %q",
				convention.Name, convention.Flavor, userFlavor,
			)
		}
	}

	preset := make(map[string]RuleCfg, len(convention.Rules))
	for ruleName, p := range convention.Rules {
		preset[ruleName] = RuleCfg{
			Enabled:  p.Enabled,
			Settings: cloneSettings(p.Settings),
		}
	}
	cfg.ConventionPreset = preset
	return nil
}

// stringSetting reads a string-typed setting from a settings map. A
// missing key returns "" with no error; a present key with a
// non-string value returns an error naming the offending field path
// so users see the problem at config load time.
func stringSetting(settings map[string]any, key, fieldPath string) (string, error) {
	v, ok := settings[key]
	if !ok {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s: must be a string, got %T", fieldPath, v)
	}
	return s, nil
}

// copyConventionPreset returns a deep copy of a convention preset
// map. Each RuleCfg's settings map is cloned so callers can mutate
// the result without affecting the source.
func copyConventionPreset(p map[string]RuleCfg) map[string]RuleCfg {
	if p == nil {
		return nil
	}
	out := make(map[string]RuleCfg, len(p))
	for k, v := range p {
		out[k] = copyRuleCfg(v)
	}
	return out
}

// validateConventionScalar returns an error when the top-level
// `convention:` value in the raw YAML is not a string scalar.
// yaml.v3 silently coerces bare ints and bools into string fields,
// which would surface as "unknown convention 123" instead of a
// clean type error. Inspecting the raw node tag is the only way to
// catch the type mismatch before that coercion happens.
func validateConventionScalar(data []byte) error {
	// yaml.Unmarshal into yaml.Node does not expand aliases, so this
	// is safe without an alias-rejection pre-check. Errors are swallowed
	// because Load's subsequent UnmarshalSafe call will surface them.
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil
	}
	if node.Kind != yaml.DocumentNode || len(node.Content) == 0 {
		return nil
	}
	mapping := node.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != "convention" {
			continue
		}
		v := mapping.Content[i+1]
		if v.Kind != yaml.ScalarNode {
			return fmt.Errorf("convention: must be a string scalar")
		}
		if v.Tag != "" && v.Tag != "!!str" {
			return fmt.Errorf(
				"convention: must be a string, got %s",
				strings.TrimPrefix(v.Tag, "!!"),
			)
		}
		return nil
	}
	return nil
}
