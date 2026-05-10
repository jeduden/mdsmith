package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/markdownflavor"
)

// reservedConventionNames is the set of built-in convention names that
// users must not redefine. Defining any of these under conventions: is
// a config error.
var reservedConventionNames = map[string]bool{
	"portable": true,
	"github":   true,
	"plain":    true,
}

// validateUserConventions checks all entries in cfg.Conventions for:
//   - reserved names (portable, github, plain)
//   - valid flavor values
//   - valid rule names (must be registered)
//   - valid rule settings (via ApplySettings on an empty rule instance)
func validateUserConventions(cfg *Config) error {
	for name, uc := range cfg.Conventions {
		if reservedConventionNames[name] {
			return fmt.Errorf(
				"conventions.%s: %q is a reserved built-in convention name",
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
			if err := validateRuleSettings(name, ruleName, rc); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateRuleSettings checks that ruleName is a registered rule and
// that settings are valid by calling ApplySettings on a fresh clone.
// Errors name the convention and rule for clear user messages.
func validateRuleSettings(conventionName, ruleName string, rc RuleCfg) error {
	rl := rule.ByName(ruleName)
	if rl == nil {
		// Unknown rules are silently allowed for forward-compatibility
		// (same policy as the built-in convention table — presets for
		// upcoming rules ship early). Only validate settings for rules
		// that are registered now.
		return nil
	}
	if rc.Settings == nil {
		return nil
	}
	c, ok := rl.(rule.Configurable)
	if !ok {
		return nil
	}
	clone := rule.CloneRule(rl)
	if cc, ok := clone.(rule.Configurable); ok {
		if err := cc.ApplySettings(rc.Settings); err != nil {
			return fmt.Errorf(
				"convention %q rule %q: %w",
				conventionName, ruleName, err,
			)
		}
	} else {
		// Shouldn't happen: original was Configurable, clone should be too.
		if err := c.ApplySettings(rc.Settings); err != nil {
			return fmt.Errorf(
				"convention %q rule %q: %w",
				conventionName, ruleName, err,
			)
		}
	}
	return nil
}

// userConventionsToMarkdownFlavor converts the config's Conventions
// map into the markdownflavor.Convention map expected by LookupWithUser.
func userConventionsToMarkdownFlavor(cfgConventions map[string]UserConvention) map[string]markdownflavor.Convention {
	if len(cfgConventions) == 0 {
		return nil
	}
	result := make(map[string]markdownflavor.Convention, len(cfgConventions))
	for name, uc := range cfgConventions {
		var flavor markdownflavor.Flavor
		if uc.Flavor != "" {
			// ParseFlavor already validated at validateUserConventions
			// time, so an unknown flavor here is a programmer error;
			// ignore the bool.
			flavor, _ = markdownflavor.ParseFlavor(uc.Flavor)
		}
		rules := make(map[string]markdownflavor.RulePreset, len(uc.Rules))
		for ruleName, rc := range uc.Rules {
			rules[ruleName] = markdownflavor.RulePreset{
				Enabled:  rc.Enabled,
				Settings: cloneSettings(rc.Settings),
			}
		}
		result[name] = markdownflavor.Convention{
			Name:   name,
			Flavor: flavor,
			Rules:  rules,
		}
	}
	return result
}

// isUserConvention reports whether name is present in cfg.Conventions.
func isUserConvention(cfg *Config, name string) bool {
	if cfg.Conventions == nil {
		return false
	}
	_, ok := cfg.Conventions[name]
	return ok
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
//     valid names (built-in + user-defined).
//   - Convention and a user-supplied rules.markdown-flavor.flavor
//     disagree → error naming both values. A convention sets a
//     flavor; a user-supplied flavor that does not match is
//     rejected at config load so the error surfaces once, not on
//     every check.
func applyConvention(cfg *Config) error {
	if cfg == nil || cfg.Convention == "" {
		return nil
	}
	userMap := userConventionsToMarkdownFlavor(cfg.Conventions)
	convention, err := markdownflavor.LookupWithUser(cfg.Convention, userMap)
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

	// Mark whether this is a user-defined convention so provenance can
	// show a "(user)" suffix in the layer source.
	cfg.ConventionIsUser = isUserConvention(cfg, cfg.Convention)
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
