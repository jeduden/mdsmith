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
// user-defined conventions must not shadow. Collisions are rejected at
// config load time so docs and tutorials retain their meaning.
var reservedConventionNames = map[string]bool{
	"portable": true,
	"github":   true,
	"plain":    true,
}

// ValidateUserConventions validates the cfg.Conventions block and
// populates cfg.UserConventions with the converted markdownflavor.Convention
// values. Validation errors name the convention and the offending rule.
//
// Validation rules:
//   - Reserved names ("portable", "github", "plain") are rejected.
//   - The `flavor` field must be a recognized flavor string.
//   - Each key in `rules` must name a registered rule.
//   - Each rule's settings must pass the rule's own ApplySettings validator.
func ValidateUserConventions(cfg *Config) error {
	if len(cfg.Conventions) == 0 {
		return nil
	}

	// Sort names so validation errors are deterministic.
	names := make([]string, 0, len(cfg.Conventions))
	for name := range cfg.Conventions {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make(map[string]markdownflavor.Convention, len(cfg.Conventions))
	for _, name := range names {
		conv, err := validateUserConvention(name, cfg.Conventions[name])
		if err != nil {
			return err
		}
		out[name] = conv
	}
	cfg.UserConventions = out
	return nil
}

// validFlavorNames is the human-readable list of valid flavor names,
// used in error messages when an unrecognized flavor is supplied.
const validFlavorNames = "commonmark, gfm, goldmark, any, pandoc, phpextra, multimarkdown, myst"

// validateUserConvention validates and converts a single user-defined
// convention entry. It returns a markdownflavor.Convention ready for
// Lookup, or an error naming the convention and the offending field.
func validateUserConvention(name string, entry UserConventionCfg) (markdownflavor.Convention, error) {
	if reservedConventionNames[name] {
		return markdownflavor.Convention{}, fmt.Errorf(
			"conventions.%s: %q is a reserved built-in convention name",
			name, name,
		)
	}
	flavor, ok := markdownflavor.ParseFlavor(entry.Flavor)
	if !ok {
		return markdownflavor.Convention{}, fmt.Errorf(
			"conventions.%s: unknown flavor %q (valid: %s)",
			name, entry.Flavor, validFlavorNames,
		)
	}
	rules, err := validateConventionRules(name, entry.Rules)
	if err != nil {
		return markdownflavor.Convention{}, err
	}
	return markdownflavor.Convention{
		Name:   name,
		Flavor: flavor,
		Rules:  rules,
	}, nil
}

// validateConventionRules validates the rules block of a user-defined
// convention and returns a markdownflavor.RulePreset map. Each rule name
// must be registered, and each rule's settings (if any) must pass the
// rule's own ApplySettings validator.
func validateConventionRules(
	conventionName string,
	ruleCfgs map[string]RuleCfg,
) (map[string]markdownflavor.RulePreset, error) {
	// Sort rule names for deterministic validation order.
	ruleNames := make([]string, 0, len(ruleCfgs))
	for rn := range ruleCfgs {
		ruleNames = append(ruleNames, rn)
	}
	sort.Strings(ruleNames)

	out := make(map[string]markdownflavor.RulePreset, len(ruleCfgs))
	for _, ruleName := range ruleNames {
		rc := ruleCfgs[ruleName]
		preset, err := validateConventionRule(conventionName, ruleName, rc)
		if err != nil {
			return nil, err
		}
		out[ruleName] = preset
	}
	return out, nil
}

// validateConventionRule validates a single rule entry inside a
// user-defined convention. It returns the preset or an error naming the
// convention and the rule.
func validateConventionRule(conventionName, ruleName string, rc RuleCfg) (markdownflavor.RulePreset, error) {
	r := rule.ByName(ruleName)
	if r == nil {
		return markdownflavor.RulePreset{}, fmt.Errorf(
			"convention %q rule %q: unknown rule name",
			conventionName, ruleName,
		)
	}
	if rc.Settings != nil {
		if err := validateRuleSettings(conventionName, ruleName, r, rc.Settings); err != nil {
			return markdownflavor.RulePreset{}, err
		}
	}
	return markdownflavor.RulePreset{
		Enabled:  rc.Enabled,
		Settings: cloneSettings(rc.Settings),
	}, nil
}

// validateRuleSettings calls the rule's ApplySettings on a clone to
// confirm the settings are valid, without modifying the registry entry.
func validateRuleSettings(conventionName, ruleName string, r rule.Rule, settings map[string]any) error {
	c, ok := r.(rule.Configurable)
	if !ok {
		return fmt.Errorf(
			"convention %q rule %q: rule does not accept settings",
			conventionName, ruleName,
		)
	}
	_ = c // checked above; clone for actual call
	clone := rule.CloneRule(r)
	if cloneCfg, ok := clone.(rule.Configurable); ok {
		if err := cloneCfg.ApplySettings(settings); err != nil {
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
	convention, err := markdownflavor.Lookup(cfg.Convention, cfg.UserConventions)
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
