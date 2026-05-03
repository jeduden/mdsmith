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

// reservedParams are variables available in body-template but forbidden in command
// and in params declarations. {alt} is reserved because it maps to the Markdown
// image alt-text field, which the framework injects from the surrounding Markdown
// syntax; it is not a user-supplied build parameter. {output} is intentionally
// NOT reserved — a recipe command may write to an output path declared as a
// regular parameter.
var reservedParams = map[string]bool{"alt": true}

// placeholderRe matches a {name} placeholder where name is an identifier
// ([A-Za-z_][A-Za-z0-9_]*). Tokens like {a b} are intentionally not matched
// because commands are whitespace-tokenised and such a token would be split.
var placeholderRe = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// ValidateBuildConfig returns an error if any recipe declares a reserved param
// name or if its command references an unknown or reserved placeholder.
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
		if err := validateRecipe(name, cfg.Build.Recipes[name]); err != nil {
			return err
		}
	}
	return nil
}

func validateRecipe(name string, recipe RecipeCfg) error {
	if err := validateDeclaredParams(name, recipe.Params); err != nil {
		return err
	}
	if recipe.Command == "" {
		return nil
	}
	allowed := make(map[string]bool)
	for _, p := range recipe.Params.Required {
		allowed[p] = true
	}
	for _, p := range recipe.Params.Optional {
		allowed[p] = true
	}
	return validateCommandPlaceholders(name, recipe.Command, allowed)
}

func validateDeclaredParams(recipeName string, params ParamCfg) error {
	for _, p := range params.Required {
		if reservedParams[p] {
			return fmt.Errorf(
				"build.recipes.%s: params.required contains reserved name %q; "+
					"reserved names are only available in body-template",
				recipeName, p,
			)
		}
	}
	for _, p := range params.Optional {
		if reservedParams[p] {
			return fmt.Errorf(
				"build.recipes.%s: params.optional contains reserved name %q; "+
					"reserved names are only available in body-template",
				recipeName, p,
			)
		}
	}
	return nil
}

func validateCommandPlaceholders(recipeName, command string, allowed map[string]bool) error {
	for _, tok := range strings.Fields(command) {
		for _, m := range placeholderRe.FindAllStringSubmatch(tok, -1) {
			param := m[1]
			if reservedParams[param] {
				return fmt.Errorf(
					"build.recipes.%s: command uses reserved placeholder {%s}; "+
						"reserved placeholders are only available in body-template",
					recipeName, param,
				)
			}
			if !allowed[param] {
				return fmt.Errorf(
					"build.recipes.%s: command references undeclared placeholder {%s}; "+
						"declare it in params.required or params.optional",
					recipeName, param,
				)
			}
		}
	}
	return nil
}

// InjectBuildConfig copies cfg.Build.Recipes into the recipe-safety
// rule settings, alongside the config file path. It is called after
// config loading in main so the rule receives its inputs through the
// normal ApplySettings path. cfgPath is the path to the loaded
// .mdsmith.yml; it is set in the config-path setting so MDS040 can
// report diagnostics against the
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
