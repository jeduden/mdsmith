package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/markdownflavor"
)

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
//   - User-defined convention names must not collide with built-in
//     names ("portable", "github", "plain") — collision → config
//     error.
//   - Each user-defined convention's flavor must be a valid flavor
//     string.
//   - Each rule name inside a user-defined convention must name a
//     registered rule.
//   - Each rule's settings must validate via ApplySettings.
//   - Unknown convention name → error naming the field and listing
//     valid names.
//   - Convention and a user-supplied rules.markdown-flavor.flavor
//     disagree → error naming both values. A convention sets a
//     flavor; a user-supplied flavor that does not match is
//     rejected at config load so the error surfaces once, not on
//     every check.
func applyConvention(cfg *Config) error {
	if cfg == nil {
		return nil
	}

	// Validate and convert user-defined conventions.
	userConventions, err := validateAndConvertUserConventions(cfg.Conventions)
	if err != nil {
		return err
	}

	if cfg.Convention == "" {
		return nil
	}

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

// reservedConventionNames is the set of built-in convention names that
// users must not redefine.
var reservedConventionNames = func() map[string]bool {
	reserved := make(map[string]bool)
	for _, name := range markdownflavor.ConventionNames() {
		reserved[name] = true
	}
	return reserved
}()

// validateAndConvertUserConventions validates every entry in the
// user-defined conventions map and converts it to the
// markdownflavor.Convention type used by Lookup. It returns an error
// for any reserved name, invalid flavor, unknown rule name, or invalid
// rule setting.
func validateAndConvertUserConventions(
	userBody map[string]UserConventionBody,
) (map[string]markdownflavor.Convention, error) {
	if len(userBody) == 0 {
		return nil, nil
	}
	out := make(map[string]markdownflavor.Convention, len(userBody))
	for name, body := range userBody {
		if reservedConventionNames[name] {
			return nil, fmt.Errorf(
				"conventions.%s: name %q is reserved by a built-in convention",
				name, name,
			)
		}
		flavor, ok := markdownflavor.ParseFlavor(body.Flavor)
		if !ok {
			return nil, fmt.Errorf(
				"conventions.%s: invalid flavor %q (valid: commonmark, gfm, goldmark)",
				name, body.Flavor,
			)
		}
		rules := make(map[string]markdownflavor.RulePreset, len(body.Rules))
		for ruleName, rc := range body.Rules {
			r := rule.ByName(ruleName)
			if r == nil {
				return nil, fmt.Errorf(
					"conventions.%s: unknown rule %q",
					name, ruleName,
				)
			}
			if rc.Enabled {
				if c, ok := r.(rule.Configurable); ok && len(rc.Settings) > 0 {
					// ApplySettings validates the settings by calling the
					// rule's own schema check.
					if err := validateRuleSettings(c, ruleName, rc.Settings, name); err != nil {
						return nil, err
					}
				}
			}
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
	return out, nil
}

// validateRuleSettings calls ApplySettings on a fresh clone of the
// rule to validate user-supplied settings inside a user convention.
func validateRuleSettings(
	c rule.Configurable,
	ruleName string,
	settings map[string]any,
	convName string,
) error {
	if err := c.ApplySettings(settings); err != nil {
		return fmt.Errorf("convention %q rule %q: %w", convName, ruleName, err)
	}
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
