package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/markdownflavor"
)

// reservedConventionNames are the built-in names that user-defined
// conventions must not shadow.
var reservedConventionNames = map[string]bool{
	"portable": true,
	"github":   true,
	"plain":    true,
}

// validateUserConventions validates the user-defined conventions map:
//   - Reserved names ("portable", "github", "plain") are rejected.
//   - Each convention's flavor must be a known flavor string.
//   - Each rule key must name a registered rule.
//   - Each rule's settings must pass ApplySettings validation.
//
// It also builds and returns the markdownflavor.Convention map that
// the Lookup call site needs.
func validateUserConventions(
	userCfg map[string]UserConventionCfg,
) (map[string]markdownflavor.Convention, error) {
	if len(userCfg) == 0 {
		return nil, nil
	}

	out := make(map[string]markdownflavor.Convention, len(userCfg))
	for name, entry := range userCfg {
		if reservedConventionNames[name] {
			return nil, fmt.Errorf(
				"conventions.%s: name is reserved; built-in convention names "+
					"(portable, github, plain) cannot be redefined",
				name,
			)
		}

		fl, ok := markdownflavor.ParseFlavor(entry.Flavor)
		if !ok {
			return nil, fmt.Errorf(
				"conventions.%s: flavor %q is not a known flavor "+
					"(valid: commonmark, gfm, goldmark, any, pandoc, "+
					"phpextra, multimarkdown, myst)",
				name, entry.Flavor,
			)
		}

		if err := validateUserConventionRules(name, entry.Rules); err != nil {
			return nil, err
		}

		rules := make(map[string]markdownflavor.RulePreset, len(entry.Rules))
		for ruleName, rc := range entry.Rules {
			rules[ruleName] = markdownflavor.RulePreset{
				Enabled:  rc.Enabled,
				Settings: cloneSettings(rc.Settings),
			}
		}
		out[name] = markdownflavor.Convention{
			Name:   name,
			Flavor: fl,
			Rules:  rules,
		}
	}
	return out, nil
}

// validateUserConventionRules validates every rule entry in a
// user-defined convention: the rule must be registered, and its
// settings must pass ApplySettings validation against a fresh instance.
func validateUserConventionRules(conventionName string, rules map[string]RuleCfg) error {
	for ruleName, rc := range rules {
		r := rule.ByName(ruleName)
		if r == nil {
			return fmt.Errorf(
				"convention %q rule %q: unknown rule name",
				conventionName, ruleName,
			)
		}
		if len(rc.Settings) == 0 {
			continue
		}
		c, ok := r.(rule.Configurable)
		if !ok {
			// Rule exists but is not configurable; extra settings are
			// silently ignored (same as the top-level rules: block).
			continue
		}
		if err := c.ApplySettings(cloneSettings(rc.Settings)); err != nil {
			return fmt.Errorf(
				"convention %q rule %q: %w",
				conventionName, ruleName, err,
			)
		}
	}
	return nil
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
//   - Reserved user-convention names → error.
//   - Unknown convention name → error naming the field and listing
//     valid names (built-in and user-defined).
//   - Convention and a user-supplied rules.markdown-flavor.flavor
//     disagree → error naming both values. A convention sets a
//     flavor; a user-supplied flavor that does not match is
//     rejected at config load so the error surfaces once, not on
//     every check.
func applyConvention(cfg *Config) error {
	if cfg == nil {
		return nil
	}

	userMap, err := validateUserConventions(cfg.Conventions)
	if err != nil {
		return fmt.Errorf("validating user conventions: %w", err)
	}
	cfg.UserConventions = userMap

	if cfg.Convention == "" {
		return nil
	}
	convention, err := markdownflavor.Lookup(cfg.Convention, userMap)
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
