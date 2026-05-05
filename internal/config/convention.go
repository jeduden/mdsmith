package config

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/markdownflavor"
)

// UserConventionBody is the YAML shape for one entry in the top-level
// `conventions:` map. It mirrors the shape of the built-in convention
// table: a flavor plus a map of rule presets.
type UserConventionBody struct {
	// Flavor is the Markdown flavor this convention targets. Must be
	// one of "commonmark", "gfm", or "goldmark".
	Flavor string `yaml:"flavor"`
	// Rules maps rule names to preset RuleCfg values.
	Rules map[string]RuleCfg `yaml:"rules"`
}

// reservedConventionNames is the set of built-in convention names that
// users must not redefine.
var reservedConventionNames = map[string]bool{
	"portable": true,
	"github":   true,
	"plain":    true,
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
	userMap := buildUserConventionMap(cfg.UserConventions)
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

// buildUserConventionMap converts a map[string]UserConventionBody into
// a map[string]markdownflavor.Convention for use with Lookup.
func buildUserConventionMap(uc map[string]UserConventionBody) map[string]markdownflavor.Convention {
	if len(uc) == 0 {
		return nil
	}
	out := make(map[string]markdownflavor.Convention, len(uc))
	for name, body := range uc {
		rules := make(map[string]markdownflavor.RulePreset, len(body.Rules))
		for rname, rc := range body.Rules {
			rules[rname] = markdownflavor.RulePreset{
				Enabled:  rc.Enabled,
				Settings: cloneSettings(rc.Settings),
			}
		}
		flavor, _ := markdownflavor.ParseFlavor(body.Flavor)
		out[name] = markdownflavor.Convention{
			Name:   name,
			Flavor: flavor,
			Rules:  rules,
		}
	}
	return out
}

// validateUserConventions validates all entries in cfg.UserConventions.
// It rejects reserved names, unknown flavors, unknown rule names, and
// invalid rule settings. Errors name the convention and (where applicable)
// the rule so users see the problem at config load time.
func validateUserConventions(cfg *Config) error {
	if len(cfg.UserConventions) == 0 {
		return nil
	}

	// Validate flavor values.
	validFlavors := map[string]bool{
		"commonmark": true,
		"gfm":        true,
		"goldmark":   true,
	}

	// Sort names for deterministic error ordering.
	names := make([]string, 0, len(cfg.UserConventions))
	for name := range cfg.UserConventions {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		body := cfg.UserConventions[name]

		// Reject reserved names.
		if reservedConventionNames[name] {
			return fmt.Errorf(
				"conventions.%s: %q is a reserved built-in convention name",
				name, name,
			)
		}

		// Validate flavor.
		if body.Flavor != "" && !validFlavors[body.Flavor] {
			return fmt.Errorf(
				"convention %q: invalid flavor %q (valid: commonmark, gfm, goldmark)",
				name, body.Flavor,
			)
		}

		// Validate each rule preset.
		for ruleName, rc := range body.Rules {
			r := rule.ByName(ruleName)
			if r == nil {
				return fmt.Errorf(
					"convention %q rule %q: unknown rule",
					name, ruleName,
				)
			}
			c, ok := r.(rule.Configurable)
			if !ok {
				continue
			}
			if err := c.ApplySettings(rc.Settings); err != nil {
				return fmt.Errorf(
					"convention %q rule %q: %w",
					name, ruleName, err,
				)
			}
		}
	}
	return nil
}

// IsUserConvention reports whether name is a user-defined convention in
// the config's UserConventions map.
func IsUserConvention(cfg *Config, name string) bool {
	if cfg == nil {
		return false
	}
	_, ok := cfg.UserConventions[name]
	return ok
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
