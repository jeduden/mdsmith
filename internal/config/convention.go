package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jeduden/mdsmith/internal/convention"
	"github.com/jeduden/mdsmith/internal/rule"
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

	// Validate and convert all user-defined conventions upfront, even
	// when no convention is selected — this catches typos at load time.
	userMap, err := buildUserConventionMap(cfg)
	if err != nil {
		return err
	}

	if cfg.Convention == "" {
		return nil
	}
	conv, err := convention.Lookup(cfg.Convention, userMap)
	if err != nil {
		return fmt.Errorf("convention: %w", err)
	}
	if rc, ok := cfg.Rules["markdown-flavor"]; ok && conv.Flavor != convention.FlavorAny {
		// A convention with FlavorAny is renderer-agnostic (e.g.
		// no-llm-tells); it imposes no flavor and never conflicts with a
		// user's markdown-flavor selection, so the guard is skipped.
		userFlavor, err := stringSetting(
			rc.Settings, "flavor", "rules.markdown-flavor.flavor",
		)
		if err != nil {
			return err
		}
		if userFlavor != "" && userFlavor != conv.Flavor.String() {
			return fmt.Errorf(
				"rules.markdown-flavor: convention %q requires flavor %q, but flavor is set to %q",
				conv.Name, conv.Flavor, userFlavor,
			)
		}
	}

	preset := make(map[string]RuleCfg, len(conv.Rules))
	for ruleName, p := range conv.Rules {
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

// buildUserConventionMap validates every entry in cfg.Conventions and
// returns them as a map[string]convention.Convention ready for
// Lookup. Validation checks:
//   - The name must not be a reserved built-in name.
//   - The flavor must be a recognised flavor string.
//   - Each rule name must be registered.
//   - Each rule's settings must pass the rule's own ApplySettings
//     check (called on a cloned instance so the registry is unaffected).
//
// Returns nil when cfg.Conventions is empty.
func buildUserConventionMap(cfg *Config) (map[string]convention.Convention, error) {
	if len(cfg.Conventions) == 0 {
		return nil, nil
	}

	reserved := make(map[string]bool, len(convention.Names()))
	for _, name := range convention.Names() {
		reserved[name] = true
	}

	result := make(map[string]convention.Convention, len(cfg.Conventions))
	for name, uc := range cfg.Conventions {
		if reserved[name] {
			return nil, fmt.Errorf(
				"conventions.%s: name is reserved by a built-in convention",
				name,
			)
		}

		fl, ok := convention.ParseFlavor(uc.Flavor)
		if !ok {
			return nil, fmt.Errorf(
				"convention %q: unknown flavor %q",
				name, uc.Flavor,
			)
		}

		rules := make(map[string]convention.RulePreset, len(uc.Rules))
		for ruleName, rc := range uc.Rules {
			r := rule.ByName(ruleName)
			if r == nil {
				return nil, fmt.Errorf(
					"convention %q: unknown rule %q",
					name, ruleName,
				)
			}
			if len(rc.Settings) > 0 {
				if err := validateConventionRuleSettings(r, name, ruleName, rc.Settings); err != nil {
					return nil, err
				}
			}
			rules[ruleName] = convention.RulePreset{
				Enabled:  rc.Enabled,
				Settings: cloneSettings(rc.Settings),
			}
		}

		result[name] = convention.Convention{
			Name:   name,
			Flavor: fl,
			Rules:  rules,
		}
	}
	return result, nil
}

// validateConventionRuleSettings clones the rule and calls
// ApplySettings to check that settings is well-formed. Using a clone
// avoids mutating the shared singleton in the rule registry.
func validateConventionRuleSettings(
	r rule.Rule,
	conventionName, ruleName string,
	settings map[string]any,
) error {
	if _, ok := r.(rule.Configurable); !ok {
		return fmt.Errorf(
			"convention %q rule %q: rule has no configurable settings",
			conventionName, ruleName,
		)
	}
	cc := rule.CloneRule(r).(rule.Configurable)
	if err := cc.ApplySettings(settings); err != nil {
		return fmt.Errorf("convention %q rule %q: %w", conventionName, ruleName, err)
	}
	return nil
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
