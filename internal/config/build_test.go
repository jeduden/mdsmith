package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- build.base-url removal ---

func TestLoad_RejectsLingeringBaseURL(t *testing.T) {
	yml := []byte("build:\n  base-url: https://example.com\n")
	_, err := loadFromBytes(yml, "", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "build.base-url was removed in plan 2606101546")
}

func TestLoad_RecipesWithoutBaseURLOK(t *testing.T) {
	yml := []byte("build:\n  recipes:\n    r:\n      command: tool\n")
	cfg, err := loadFromBytes(yml, "", false)
	require.NoError(t, err)
	require.Contains(t, cfg.Build.Recipes, "r")
}

func TestLoad_RejectsInvalidBuildConfig(t *testing.T) {
	// A recipe that embeds a collective placeholder in a larger token is
	// invalid. loadFromBytes must surface the ValidateBuildConfig error.
	yml := []byte("build:\n  recipes:\n    x:\n      command: tool -o{outputs}\n")
	_, err := loadFromBytes(yml, "", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outputs")
}

func TestCheckBuildConfig_NoBuildKey(t *testing.T) {
	// A config with no build: key should pass checkBuildConfig without error.
	yml := []byte("rules: {}\n")
	_, err := loadFromBytes(yml, "", false)
	require.NoError(t, err)
}

func TestRejectRemovedBuildKeys_BuildNullValue(t *testing.T) {
	// build: with a null value (YAML scalar node, not a mapping) must not
	// error in rejectRemovedBuildKeys; the base-url scan only applies when
	// the build: value is itself a mapping.
	yml := []byte("build:\n")
	_, err := loadFromBytes(yml, "", false)
	require.NoError(t, err)
}

func TestRejectRemovedBuildKeys_NullDocumentRoot(t *testing.T) {
	// A null document root (ScalarNode, not MappingNode) must not error in
	// rejectRemovedBuildKeys — the mapping-kind guard returns nil early.
	yml := []byte("null\n")
	_, err := loadFromBytes(yml, "", false)
	require.NoError(t, err)
}

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

func TestValidateBuildConfig_EmptyCommand_ReservedParam_Rejected(t *testing.T) {
	// Reserved names in params must be rejected even when command is empty.
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"x": {Command: "", Params: ParamCfg{Required: []string{"alt"}}},
			},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved name")
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

func TestValidateBuildConfig_CollectivePlaceholdersAllowed(t *testing.T) {
	// {outputs} and {inputs} are the collective argv placeholders; the
	// build executor expands them from the directive's outputs:/inputs:
	// lists, so a command may use them without declaring them as params.
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"copy": {Command: "cp {inputs} {outputs}"},
			},
		},
	}
	assert.NoError(t, ValidateBuildConfig(cfg))
}

func TestValidateBuildConfig_EmbeddedCollectivePlaceholderRejected(t *testing.T) {
	// A collective placeholder must stand alone as its own argv token;
	// expanding a list inside a token fragment has no meaning.
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"x": {Command: "tool -o{outputs}"},
			},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outputs")
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

func TestValidateBuildConfig_ReservedAlt_InCommand(t *testing.T) {
	// {alt} used in command without being declared in params.
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"x": {Command: "tool {alt}"},
			},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved placeholder")
	assert.Contains(t, err.Error(), "{alt}")
}

func TestValidateBuildConfig_ReservedAlt_InRequired(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"x": {
					Command: "tool",
					Params:  ParamCfg{Required: []string{"alt"}},
				},
			},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved name")
	assert.Contains(t, err.Error(), "alt")
}

func TestValidateBuildConfig_ReservedAlt_InOptional(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"x": {
					Command: "tool",
					Params:  ParamCfg{Optional: []string{"alt"}},
				},
			},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved name")
	assert.Contains(t, err.Error(), "alt")
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

func TestInjectBuildConfig_EmptyRecipes_ClearsExistingSettings(t *testing.T) {
	// Even with no build.recipes, InjectBuildConfig must overwrite any
	// user-supplied recipes setting so rules cannot receive recipes via
	// rule settings alone.
	cfg := &Config{
		Build: BuildConfig{}, // no recipes
		Rules: map[string]RuleCfg{
			"build": {
				Enabled:  true,
				Settings: map[string]any{"recipes": map[string]any{"sneaky": map[string]any{}}},
			},
		},
	}
	InjectBuildConfig(cfg, "")
	rc := cfg.Rules["build"]
	require.NotNil(t, rc.Settings)
	recipes, ok := rc.Settings["recipes"].(map[string]any)
	require.True(t, ok)
	assert.Empty(t, recipes, "recipes must be cleared when build.recipes is empty")
}

func TestInjectBuildConfig_NoRecipes(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"recipe-safety": {Enabled: true},
		},
	}
	InjectBuildConfig(cfg, ".mdsmith.yml")
	// An empty recipes map is still injected to overwrite any user-supplied settings.
	rc := cfg.Rules["recipe-safety"]
	require.NotNil(t, rc.Settings)
	recipes, ok := rc.Settings["recipes"].(map[string]any)
	require.True(t, ok)
	assert.Empty(t, recipes)
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

func TestInjectBuildConfig_BuildRule(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"render": {
					Command:      "myrenderer {source} -o {output}",
					BodyTemplate: "![{alt}]({output})",
					Params: ParamCfg{
						Required: []string{"source"},
						Optional: []string{"output"},
					},
				},
			},
		},
		Rules: map[string]RuleCfg{
			"build": {Enabled: true},
		},
	}
	InjectBuildConfig(cfg, ".mdsmith.yml")

	rc := cfg.Rules["build"]
	require.NotNil(t, rc.Settings)
	recipesAny, ok := rc.Settings["recipes"]
	require.True(t, ok, "recipes key must be present in build rule settings")
	recipes, ok := recipesAny.(map[string]any)
	require.True(t, ok)
	assert.Contains(t, recipes, "render")
	// config-path must NOT be injected into the build rule (only recipe-safety gets it)
	_, hasPath := rc.Settings["config-path"]
	assert.False(t, hasPath)
}

func TestInjectBuildConfig_BuildRule_NilSettings(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"x": {Command: "tool"},
			},
		},
		Rules: map[string]RuleCfg{
			"build": {Enabled: true, Settings: nil},
		},
	}
	InjectBuildConfig(cfg, "cfg.yml")
	rc := cfg.Rules["build"]
	require.NotNil(t, rc.Settings)
	_, ok := rc.Settings["recipes"]
	assert.True(t, ok)
}

func TestInjectBuildConfig_BuildRule_Disabled(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"x": {Command: "tool"},
			},
		},
		Rules: map[string]RuleCfg{
			"build": {Enabled: false},
		},
	}
	InjectBuildConfig(cfg, "cfg.yml")
	assert.Nil(t, cfg.Rules["build"].Settings)
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
	require.Contains(t, merged.Build.Recipes, "mermaid")
	assert.Equal(t, "mmdc -i {input}", merged.Build.Recipes["mermaid"].Command)
}
