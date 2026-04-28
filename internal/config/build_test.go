package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ValidateBuildConfig ---

func TestValidateBuildConfig_Nil(t *testing.T) {
	assert.NoError(t, ValidateBuildConfig(nil))
}

func TestValidateBuildConfig_NoBuild(t *testing.T) {
	cfg := &Config{}
	assert.NoError(t, ValidateBuildConfig(cfg))
}

func TestValidateBuildConfig_EmptyCommand_Skipped(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"x": {Command: ""},
			},
		},
	}
	assert.NoError(t, ValidateBuildConfig(cfg))
}

func TestValidateBuildConfig_ValidCommand(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"mermaid": {
					Command: "mmdc -i {input} --theme {theme}",
					Params: ParamCfg{
						Required: []string{"input"},
						Optional: []string{"theme"},
					},
				},
			},
		},
	}
	assert.NoError(t, ValidateBuildConfig(cfg))
}

func TestValidateBuildConfig_UndeclaredPlaceholder(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"x": {Command: "tool {unknown}"},
			},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "undeclared placeholder")
	assert.Contains(t, err.Error(), "{unknown}")
}

func TestValidateBuildConfig_ReservedAlt(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"x": {
					Command: "tool {alt}",
					Params:  ParamCfg{Optional: []string{"alt"}},
				},
			},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved placeholder")
	assert.Contains(t, err.Error(), "{alt}")
}

func TestValidateBuildConfig_OutputParam_Allowed(t *testing.T) {
	// {output} is NOT reserved — a recipe command may write to a declared output param.
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"x": {
					Command: "tool -o {output}",
					Params:  ParamCfg{Required: []string{"output"}},
				},
			},
		},
	}
	assert.NoError(t, ValidateBuildConfig(cfg))
}

func TestValidateBuildConfig_RequiredParam_Allowed(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"x": {
					Command: "tool {input}",
					Params:  ParamCfg{Required: []string{"input"}},
				},
			},
		},
	}
	assert.NoError(t, ValidateBuildConfig(cfg))
}

func TestValidateBuildConfig_OptionalParam_Allowed(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"x": {
					Command: "tool {theme}",
					Params:  ParamCfg{Optional: []string{"theme"}},
				},
			},
		},
	}
	assert.NoError(t, ValidateBuildConfig(cfg))
}

// --- InjectBuildConfig ---

func TestInjectBuildConfig_Nil(t *testing.T) {
	// Must not panic.
	InjectBuildConfig(nil, "")
}

func TestInjectBuildConfig_NoRecipes(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"recipe-safety": {Enabled: true},
		},
	}
	InjectBuildConfig(cfg, ".mdsmith.yml")
	// Settings must remain nil/empty — nothing to inject.
	assert.Nil(t, cfg.Rules["recipe-safety"].Settings)
}

func TestInjectBuildConfig_RuleDisabled(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"x": {Command: "tool {input}", Params: ParamCfg{Required: []string{"input"}}},
			},
		},
		Rules: map[string]RuleCfg{
			"recipe-safety": {Enabled: false},
		},
	}
	InjectBuildConfig(cfg, ".mdsmith.yml")
	// Disabled rule must not be injected.
	assert.Nil(t, cfg.Rules["recipe-safety"].Settings)
}

func TestInjectBuildConfig_RuleNotPresent(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"x": {Command: "tool {input}", Params: ParamCfg{Required: []string{"input"}}},
			},
		},
		Rules: map[string]RuleCfg{},
	}
	InjectBuildConfig(cfg, ".mdsmith.yml")
	// Rule not in map — no injection, no panic.
	assert.NotContains(t, cfg.Rules, "recipe-safety")
}

func TestInjectBuildConfig_InjectsRecipesAndPath(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"mermaid": {
					Command:      "mmdc -i {input} -o {output}",
					BodyTemplate: "![alt]({output})",
					Params: ParamCfg{
						Required: []string{"input"},
						Optional: []string{"output"},
					},
				},
			},
		},
		Rules: map[string]RuleCfg{
			"recipe-safety": {Enabled: true},
		},
	}
	InjectBuildConfig(cfg, "/project/.mdsmith.yml")

	rc := cfg.Rules["recipe-safety"]
	require.NotNil(t, rc.Settings)
	assert.Equal(t, "/project/.mdsmith.yml", rc.Settings["config-path"])

	recipesAny, ok := rc.Settings["recipes"]
	require.True(t, ok, "recipes key must be present")
	recipes, ok := recipesAny.(map[string]any)
	require.True(t, ok)
	require.Contains(t, recipes, "mermaid")

	mermaid, ok := recipes["mermaid"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "mmdc -i {input} -o {output}", mermaid["command"])
	assert.Equal(t, "![alt]({output})", mermaid["body-template"])

	params, ok := mermaid["params"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []any{"input"}, params["required"])
	assert.Equal(t, []any{"output"}, params["optional"])
}

func TestInjectBuildConfig_OverwritesExistingSettings(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"x": {Command: "tool", Params: ParamCfg{}},
			},
		},
		Rules: map[string]RuleCfg{
			"recipe-safety": {
				Enabled:  true,
				Settings: map[string]any{"recipes": "old value"},
			},
		},
	}
	InjectBuildConfig(cfg, "cfg.yml")
	rc := cfg.Rules["recipe-safety"]
	// recipes must be overwritten by serialized form, not the old string value.
	_, isMap := rc.Settings["recipes"].(map[string]any)
	assert.True(t, isMap, "recipes should be overwritten with a map")
}

func TestInjectBuildConfig_EmptyCfgPath_NoPathInjected(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"x": {Command: "tool"},
			},
		},
		Rules: map[string]RuleCfg{
			"recipe-safety": {Enabled: true},
		},
	}
	InjectBuildConfig(cfg, "")
	rc := cfg.Rules["recipe-safety"]
	_, hasPath := rc.Settings["config-path"]
	assert.False(t, hasPath, "config-path must not be set when cfgPath is empty")
}

// --- serializeRecipes ---

func TestSerializeRecipes_Empty(t *testing.T) {
	out := serializeRecipes(map[string]RecipeCfg{})
	assert.Empty(t, out)
}

func TestSerializeRecipes_NoBodyTemplate_NoParams(t *testing.T) {
	out := serializeRecipes(map[string]RecipeCfg{
		"simple": {Command: "tool run"},
	})
	m, ok := out["simple"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "tool run", m["command"])
	assert.NotContains(t, m, "body-template")
	assert.NotContains(t, m, "params")
}

func TestSerializeRecipes_WithBodyTemplate(t *testing.T) {
	out := serializeRecipes(map[string]RecipeCfg{
		"x": {Command: "tool", BodyTemplate: "![out]({output})"},
	})
	m := out["x"].(map[string]any)
	assert.Equal(t, "![out]({output})", m["body-template"])
}

func TestSerializeRecipes_RequiredOnly(t *testing.T) {
	out := serializeRecipes(map[string]RecipeCfg{
		"x": {Command: "tool {input}", Params: ParamCfg{Required: []string{"input"}}},
	})
	m := out["x"].(map[string]any)
	params := m["params"].(map[string]any)
	assert.Equal(t, []any{"input"}, params["required"])
	assert.NotContains(t, params, "optional")
}

func TestSerializeRecipes_OptionalOnly(t *testing.T) {
	out := serializeRecipes(map[string]RecipeCfg{
		"x": {Command: "tool", Params: ParamCfg{Optional: []string{"theme"}}},
	})
	m := out["x"].(map[string]any)
	params := m["params"].(map[string]any)
	assert.Equal(t, []any{"theme"}, params["optional"])
	assert.NotContains(t, params, "required")
}

func TestSerializeRecipes_BothParams(t *testing.T) {
	out := serializeRecipes(map[string]RecipeCfg{
		"x": {
			Command: "tool {a} {b}",
			Params: ParamCfg{
				Required: []string{"a"},
				Optional: []string{"b"},
			},
		},
	})
	m := out["x"].(map[string]any)
	params := m["params"].(map[string]any)
	assert.Equal(t, []any{"a"}, params["required"])
	assert.Equal(t, []any{"b"}, params["optional"])
}

// --- copyBuildConfig isolation ---

func TestCopyBuildConfig_MutationDoesNotAliasOriginal(t *testing.T) {
	orig := BuildConfig{
		BaseURL: "https://example.com",
		Recipes: map[string]RecipeCfg{
			"x": {Command: "tool {a}", Params: ParamCfg{Required: []string{"a"}}},
		},
	}
	cp := copyBuildConfig(orig)

	// Mutate the copy's map and slice.
	cp.Recipes["x"] = RecipeCfg{Command: "changed"}
	cp.Recipes["new"] = RecipeCfg{Command: "other"}

	// Original must be unaffected.
	assert.Equal(t, "tool {a}", orig.Recipes["x"].Command)
	assert.NotContains(t, orig.Recipes, "new")
}

func TestCopyBuildConfig_Empty(t *testing.T) {
	cp := copyBuildConfig(BuildConfig{})
	assert.Empty(t, cp.Recipes)
	assert.Equal(t, "", cp.BaseURL)
}

// --- Build survives Merge ---

func TestMerge_PreservesBuild(t *testing.T) {
	defaults := &Config{
		Rules: map[string]RuleCfg{
			"recipe-safety": {Enabled: true},
		},
	}
	loaded := &Config{
		Rules: map[string]RuleCfg{
			"recipe-safety": {Enabled: true},
		},
		Build: BuildConfig{
			BaseURL: "https://example.com",
			Recipes: map[string]RecipeCfg{
				"mermaid": {
					Command: "mmdc -i {input}",
					Params:  ParamCfg{Required: []string{"input"}},
				},
			},
		},
	}
	merged := Merge(defaults, loaded)
	require.NotNil(t, merged)
	assert.Equal(t, "https://example.com", merged.Build.BaseURL)
	require.Contains(t, merged.Build.Recipes, "mermaid")
	assert.Equal(t, "mmdc -i {input}", merged.Build.Recipes["mermaid"].Command)
}
