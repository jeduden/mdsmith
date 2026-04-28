package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// BuildConfig is the top-level build: section.
type BuildConfig struct {
	BaseURL string               `yaml:"base-url,omitempty"`
	Recipes map[string]RecipeCfg `yaml:"recipes,omitempty"`
}

// RecipeCfg is a single user-defined recipe declaration.
type RecipeCfg struct {
	Command      string   `yaml:"command"`
	BodyTemplate string   `yaml:"body-template,omitempty"`
	Params       ParamCfg `yaml:"params,omitempty"`
}

// ParamCfg names the params a recipe accepts.
type ParamCfg struct {
	Required []string `yaml:"required,omitempty"`
	Optional []string `yaml:"optional,omitempty"`
}

// reservedParams are variables available in body_template but forbidden in command.
// {alt} is reserved because it maps to the Markdown image alt-text field, which
// the framework injects from the surrounding Markdown syntax; it is not a
// user-supplied build parameter. {output} is intentionally NOT reserved — a
// recipe command may write to an output path declared as a regular parameter.
var reservedParams = map[string]bool{"alt": true}

// placeholderRe matches a {name} placeholder substring within a command token.
var placeholderRe = regexp.MustCompile(`\{([^{}]+)\}`)

// ValidateBuildConfig returns an error if any recipe command references
// an unknown param or uses a reserved param name.
// Recipe names are validated in sorted order for deterministic errors.
func ValidateBuildConfig(cfg *Config) error {
	if cfg == nil {
		return nil
	}
	names := make([]string, 0, len(cfg.Build.Recipes))
	for name := range cfg.Build.Recipes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		recipe := cfg.Build.Recipes[name]
		if recipe.Command == "" {
			continue
		}
		allowed := make(map[string]bool)
		for _, p := range recipe.Params.Required {
			allowed[p] = true
		}
		for _, p := range recipe.Params.Optional {
			allowed[p] = true
		}
		tokens := strings.Fields(recipe.Command)
		for _, tok := range tokens {
			for _, m := range placeholderRe.FindAllStringSubmatch(tok, -1) {
				param := m[1]
				if reservedParams[param] {
					return fmt.Errorf(
						"build.recipes.%s: command uses reserved placeholder {%s}; "+
							"reserved placeholders are only available in body_template",
						name, param,
					)
				}
				if !allowed[param] {
					return fmt.Errorf(
						"build.recipes.%s: command references undeclared placeholder {%s}; "+
							"declare it in params.required or params.optional",
						name, param,
					)
				}
			}
		}
	}
	return nil
}

// InjectBuildConfig copies cfg.Build.Recipes into the recipe-safety
// rule settings, alongside the config file path. This mirrors
// InjectArchetypeRoots: it is called after config loading in main so
// the rule receives its inputs through the normal ApplySettings path.
// cfgPath is the path to the loaded .mdsmith.yml; it is set in the
// config-path setting so MDS040 can report diagnostics against the
// right file.
func InjectBuildConfig(cfg *Config, cfgPath string) {
	if cfg == nil || len(cfg.Build.Recipes) == 0 {
		return
	}
	const name = "recipe-safety"
	rc, ok := cfg.Rules[name]
	if !ok || !rc.Enabled {
		return
	}
	if rc.Settings == nil {
		rc.Settings = make(map[string]any)
	}
	// Always overwrite: recipes must come from build:, not user rule settings.
	rc.Settings["recipes"] = serializeRecipes(cfg.Build.Recipes)
	if cfgPath != "" {
		rc.Settings["config-path"] = cfgPath
	}
	cfg.Rules[name] = rc
}

// serializeRecipes converts RecipeCfg map to map[string]any for transport
// through the generic ApplySettings mechanism.
func serializeRecipes(recipes map[string]RecipeCfg) map[string]any {
	out := make(map[string]any, len(recipes))
	for name, r := range recipes {
		m := map[string]any{"command": r.Command}
		if r.BodyTemplate != "" {
			m["body-template"] = r.BodyTemplate
		}
		params := map[string]any{}
		if len(r.Params.Required) > 0 {
			s := make([]any, len(r.Params.Required))
			for i, v := range r.Params.Required {
				s[i] = v
			}
			params["required"] = s
		}
		if len(r.Params.Optional) > 0 {
			s := make([]any, len(r.Params.Optional))
			for i, v := range r.Params.Optional {
				s[i] = v
			}
			params["optional"] = s
		}
		if len(params) > 0 {
			m["params"] = params
		}
		out[name] = m
	}
	return out
}
