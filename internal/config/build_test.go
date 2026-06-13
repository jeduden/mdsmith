package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfigPath(t *testing.T) {
	assert.Equal(t, filepath.Join("/some/root", ".mdsmith.yml"), DefaultConfigPath("/some/root"))
	assert.Equal(t, filepath.Join(".", ".mdsmith.yml"), DefaultConfigPath("."))
}

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

// --- build.exec ---

func TestLoad_ExecConfigParses(t *testing.T) {
	yml := []byte("build:\n  exec:\n    path: \"/opt/bin:/bin\"\n" +
		"    env-pass-through: [HOME, LANG, SOURCE_DATE_EPOCH]\n")
	cfg, err := loadFromBytes(yml, "", false)
	require.NoError(t, err)
	assert.Equal(t, "/opt/bin:/bin", cfg.Build.Exec.Path)
	assert.Equal(t, []string{"HOME", "LANG", "SOURCE_DATE_EPOCH"}, cfg.Build.Exec.EnvPassThrough)
}

func TestValidateBuildConfig_ExecEmptyPassThroughName(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Exec: ExecCfg{EnvPassThrough: []string{"HOME", ""}},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "env-pass-through")
	assert.Contains(t, err.Error(), "empty")
}

func TestValidateBuildConfig_ExecPassThroughNameWithEquals(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Exec: ExecCfg{EnvPassThrough: []string{"HOME=evil"}},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "env-pass-through")
	assert.Contains(t, err.Error(), "=")
}

func TestValidateBuildConfig_ExecPassThroughNameWithControlChar(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Exec: ExecCfg{EnvPassThrough: []string{"FOO\nBAR"}},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "env-pass-through")
	assert.Contains(t, err.Error(), "newline")
}

func TestValidateBuildConfig_ExecValid(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Exec: ExecCfg{Path: "/usr/bin:/bin", EnvPassThrough: []string{"HOME", "LANG"}},
		},
	}
	assert.NoError(t, ValidateBuildConfig(cfg))
}

func TestMerge_PreservesExecConfigWithoutRecipes(t *testing.T) {
	loaded := &Config{
		Build: BuildConfig{
			Exec: ExecCfg{Path: "/custom/bin", EnvPassThrough: []string{"FOO"}},
		},
	}
	merged := Merge(Defaults(), loaded)
	assert.Equal(t, "/custom/bin", merged.Build.Exec.Path)
	assert.Equal(t, []string{"FOO"}, merged.Build.Exec.EnvPassThrough)
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

// --- default-inputs ---

func TestLoad_RecipeDefaultInputs(t *testing.T) {
	yml := []byte("build:\n  recipes:\n    vhs:\n      command: vhs {tape}\n" +
		"      params:\n        required: [tape]\n      default-inputs: [\"{tape}\"]\n")
	cfg, err := loadFromBytes(yml, "", false)
	require.NoError(t, err)
	require.Contains(t, cfg.Build.Recipes, "vhs")
	assert.Equal(t, []string{"{tape}"}, cfg.Build.Recipes["vhs"].DefaultInputs)
}

func TestValidateBuildConfig_DefaultInputsParamToken_Allowed(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"vhs": {
					Command:       "vhs {tape}",
					Params:        ParamCfg{Required: []string{"tape"}},
					DefaultInputs: []string{"{tape}"},
				},
			},
		},
	}
	assert.NoError(t, ValidateBuildConfig(cfg))
}

func TestValidateBuildConfig_DefaultInputsLiteralPath_Allowed(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"x": {
					Command:       "tool {outputs}",
					DefaultInputs: []string{"assets/header.css"},
				},
			},
		},
	}
	assert.NoError(t, ValidateBuildConfig(cfg))
}

func TestValidateBuildConfig_DefaultInputsUndeclaredParam_Rejected(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"x": {
					Command:       "tool {outputs}",
					DefaultInputs: []string{"{missing}"},
				},
			},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default-inputs")
	assert.Contains(t, err.Error(), "missing")
}

func TestValidateBuildConfig_DefaultInputsReservedParam_Rejected(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"x": {
					Command:       "tool {outputs}",
					DefaultInputs: []string{"{inputs}"},
				},
			},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default-inputs")
}

func TestValidateBuildConfig_DefaultInputsAbsolutePath_Rejected(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"x": {
					Command:       "tool {outputs}",
					DefaultInputs: []string{"/etc/passwd"},
				},
			},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default-inputs")
}

func TestValidateBuildConfig_DefaultInputsParentEscape_Rejected(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"x": {
					Command:       "tool {outputs}",
					DefaultInputs: []string{"../secret.txt"},
				},
			},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default-inputs")
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

func TestCopyBuildConfig_HooksSurviveCopy(t *testing.T) {
	orig := BuildConfig{
		Hooks: HooksCfg{
			Before: []HookCfg{{Command: "make start", Name: "start server"}},
			After:  []HookCfg{{Command: "make stop"}},
		},
	}
	cp := copyBuildConfig(orig)
	require.Len(t, cp.Hooks.Before, 1)
	assert.Equal(t, "make start", cp.Hooks.Before[0].Command)
	assert.Equal(t, "start server", cp.Hooks.Before[0].Name)
	require.Len(t, cp.Hooks.After, 1)
	assert.Equal(t, "make stop", cp.Hooks.After[0].Command)
}

func TestCopyBuildConfig_HookParams_CopiedAndIsolated(t *testing.T) {
	orig := BuildConfig{
		Hooks: HooksCfg{
			Before: []HookCfg{{
				Command: "scripts/wait {port}",
				Params:  map[string]string{"port": "3000"},
			}},
		},
	}
	cp := copyBuildConfig(orig)
	require.Len(t, cp.Hooks.Before, 1)
	assert.Equal(t, "3000", cp.Hooks.Before[0].Params["port"])
	// Mutation of the copy's params must not affect the original.
	cp.Hooks.Before[0].Params["port"] = "changed"
	assert.Equal(t, "3000", orig.Hooks.Before[0].Params["port"])
}

func TestCopyBuildConfig_HooksMutationIsolated(t *testing.T) {
	orig := BuildConfig{
		Hooks: HooksCfg{
			Before: []HookCfg{{Command: "make start"}},
		},
	}
	cp := copyBuildConfig(orig)
	cp.Hooks.Before[0] = HookCfg{Command: "changed"}
	// Original must be unaffected.
	assert.Equal(t, "make start", orig.Hooks.Before[0].Command)
}

func TestMerge_PreservesHooks(t *testing.T) {
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
			Hooks: HooksCfg{
				Before: []HookCfg{{Command: "make start"}},
				After:  []HookCfg{{Command: "make stop"}},
			},
		},
	}
	merged := Merge(defaults, loaded)
	require.NotNil(t, merged)
	require.Len(t, merged.Build.Hooks.Before, 1)
	assert.Equal(t, "make start", merged.Build.Hooks.Before[0].Command)
	require.Len(t, merged.Build.Hooks.After, 1)
	assert.Equal(t, "make stop", merged.Build.Hooks.After[0].Command)
}

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

// --- defaultInputPathShape branch coverage ---

func TestDefaultInputPathShape_EmptyString(t *testing.T) {
	assert.Equal(t, "must not be empty", defaultInputPathShape(""))
}

func TestDefaultInputPathShape_WhitespaceOnly(t *testing.T) {
	assert.Equal(t, "must not be empty", defaultInputPathShape("   "))
}

func TestDefaultInputPathShape_LeadingWhitespace(t *testing.T) {
	assert.Equal(t, "must not have leading or trailing whitespace", defaultInputPathShape(" valid/path"))
}

func TestDefaultInputPathShape_TrailingWhitespace(t *testing.T) {
	assert.Equal(t, "must not have leading or trailing whitespace", defaultInputPathShape("valid/path "))
}

func TestDefaultInputPathShape_NULByte(t *testing.T) {
	assert.Equal(t, "must not contain NUL, newline, or carriage return", defaultInputPathShape("pa\x00th"))
}

func TestDefaultInputPathShape_Newline(t *testing.T) {
	assert.Equal(t, "must not contain NUL, newline, or carriage return", defaultInputPathShape("pa\nth"))
}

func TestDefaultInputPathShape_CarriageReturn(t *testing.T) {
	assert.Equal(t, "must not contain NUL, newline, or carriage return", defaultInputPathShape("pa\rth"))
}

func TestDefaultInputPathShape_Backslash(t *testing.T) {
	assert.Equal(t, "must use forward-slash separators only", defaultInputPathShape(`path\file`))
}

func TestDefaultInputPathShape_TildePrefix(t *testing.T) {
	assert.Equal(t, "must be a relative path", defaultInputPathShape("~/home/file"))
}

func TestDefaultInputPathShape_JustDotDot(t *testing.T) {
	assert.NotEmpty(t, defaultInputPathShape(".."))
}

func TestDefaultInputPathShape_EmbeddedDotDot(t *testing.T) {
	assert.NotEmpty(t, defaultInputPathShape("a/../../b"))
}

func TestDefaultInputPathShape_SuffixDotDot(t *testing.T) {
	assert.NotEmpty(t, defaultInputPathShape("a/b/.."))
}

func TestDefaultInputPathShape_ValidPath(t *testing.T) {
	assert.Equal(t, "", defaultInputPathShape("assets/logo.svg"))
}

func TestValidateBuildConfig_DefaultInputsReservedAlt_Rejected(t *testing.T) {
	// {alt} is in reservedParams (not collectivePlaceholders) — hits the
	// reservedParams branch in validateDefaultInputs.
	cfg := &Config{
		Build: BuildConfig{
			Recipes: map[string]RecipeCfg{
				"x": {
					Command:       "tool {outputs}",
					DefaultInputs: []string{"{alt}"},
				},
			},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default-inputs")
	assert.Contains(t, err.Error(), "alt")
}

// --- serializeRecipes DefaultInputs ---

func TestSerializeRecipes_WithDefaultInputs(t *testing.T) {
	out := serializeRecipes(map[string]RecipeCfg{
		"vhs": {
			Command:       "vhs {tape}",
			Params:        ParamCfg{Required: []string{"tape"}},
			DefaultInputs: []string{"{tape}"},
		},
	})
	m, ok := out["vhs"].(map[string]any)
	require.True(t, ok)
	di, hasDI := m["default-inputs"]
	require.True(t, hasDI, "default-inputs must be serialized")
	assert.Equal(t, []any{"{tape}"}, di)
}

// --- Hook config ---

func TestLoad_HooksBeforeAndAfter(t *testing.T) {
	yml := []byte(`build:
  hooks:
    before:
      - command: "make dev-server-start"
      - command: "scripts/wait-for-port {port}"
        params:
          port: "3000"
    after:
      - command: "make dev-server-stop"
`)
	cfg, err := loadFromBytes(yml, "", false)
	require.NoError(t, err)
	require.Len(t, cfg.Build.Hooks.Before, 2)
	assert.Equal(t, "make dev-server-start", cfg.Build.Hooks.Before[0].Command)
	assert.Equal(t, "scripts/wait-for-port {port}", cfg.Build.Hooks.Before[1].Command)
	assert.Equal(t, map[string]string{"port": "3000"}, cfg.Build.Hooks.Before[1].Params)
	require.Len(t, cfg.Build.Hooks.After, 1)
	assert.Equal(t, "make dev-server-stop", cfg.Build.Hooks.After[0].Command)
}

func TestLoad_HookName(t *testing.T) {
	yml := []byte(`build:
  hooks:
    before:
      - command: "make start"
        name: "start dev server"
`)
	cfg, err := loadFromBytes(yml, "", false)
	require.NoError(t, err)
	require.Len(t, cfg.Build.Hooks.Before, 1)
	assert.Equal(t, "start dev server", cfg.Build.Hooks.Before[0].Name)
}

func TestLoad_NoHooks_ParsesCleanly(t *testing.T) {
	yml := []byte("build:\n  recipes:\n    r:\n      command: tool\n")
	cfg, err := loadFromBytes(yml, "", false)
	require.NoError(t, err)
	assert.Empty(t, cfg.Build.Hooks.Before)
	assert.Empty(t, cfg.Build.Hooks.After)
}

func TestValidateBuildConfig_Hook_EmptyCommand_Rejected(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Hooks: HooksCfg{
				Before: []HookCfg{{Command: ""}},
			},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command must not be empty")
}

func TestValidateBuildConfig_Hook_ValidCommand(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Hooks: HooksCfg{
				Before: []HookCfg{{Command: "make dev-server-start"}},
				After:  []HookCfg{{Command: "make dev-server-stop"}},
			},
		},
	}
	assert.NoError(t, ValidateBuildConfig(cfg))
}

func TestValidateBuildConfig_Hook_WithParams(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Hooks: HooksCfg{
				Before: []HookCfg{
					{
						Command: "scripts/wait-for-port {port}",
						Params:  map[string]string{"port": "3000"},
					},
				},
			},
		},
	}
	assert.NoError(t, ValidateBuildConfig(cfg))
}

func TestValidateBuildConfig_Hook_UndeclaredParam_Rejected(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Hooks: HooksCfg{
				Before: []HookCfg{
					{Command: "tool {unknown}"},
				},
			},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "undeclared")
}

func TestValidateBuildConfig_Hook_ReservedInputs_Rejected(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Hooks: HooksCfg{
				Before: []HookCfg{
					{Command: "tool {inputs}"},
				},
			},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inputs")
}

func TestValidateBuildConfig_Hook_ReservedOutputs_Rejected(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Hooks: HooksCfg{
				Before: []HookCfg{
					{Command: "tool {outputs}"},
				},
			},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outputs")
}

func TestValidateBuildConfig_Hook_ParamValue_NUL_Rejected(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Hooks: HooksCfg{
				Before: []HookCfg{
					{Command: "tool {p}", Params: map[string]string{"p": "val\x00ue"}},
				},
			},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NUL")
}

func TestValidateBuildConfig_Hook_ParamValue_Newline_Rejected(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Hooks: HooksCfg{
				Before: []HookCfg{
					{Command: "tool {p}", Params: map[string]string{"p": "val\nue"}},
				},
			},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "newline")
}

func TestValidateBuildConfig_Hook_ParamValue_LeadingWhitespace_Rejected(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Hooks: HooksCfg{
				Before: []HookCfg{
					{Command: "tool {p}", Params: map[string]string{"p": " value"}},
				},
			},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "whitespace")
}

func TestValidateBuildConfig_Hook_ParamValue_TooLong_Rejected(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Hooks: HooksCfg{
				Before: []HookCfg{
					{Command: "tool {p}", Params: map[string]string{"p": string(make([]byte, 4097))}},
				},
			},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "4 KB")
}

func TestValidateBuildConfig_Hook_UnusedParam_Warning(t *testing.T) {
	// Unused params are a warning at MDS040 level, not a config-level error.
	// ValidateBuildConfig should not error on unused hook params.
	cfg := &Config{
		Build: BuildConfig{
			Hooks: HooksCfg{
				Before: []HookCfg{
					{Command: "make start", Params: map[string]string{"unused": "value"}},
				},
			},
		},
	}
	// Config-level validate should pass; MDS040 is where warnings are emitted.
	assert.NoError(t, ValidateBuildConfig(cfg))
}

func TestValidateBuildConfig_Hook_AfterHook_EmptyCommand_Rejected(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Hooks: HooksCfg{
				After: []HookCfg{{Command: ""}},
			},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command must not be empty")
	assert.Contains(t, err.Error(), "after")
}

func TestValidateBuildConfig_Hook_ReservedAlt_InHookCommand_Rejected(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Hooks: HooksCfg{
				Before: []HookCfg{{
					Command: "scripts/gen {alt}",
					Params:  map[string]string{"alt": "desc"},
				}},
			},
		},
	}
	err := ValidateBuildConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved placeholder")
	assert.Contains(t, err.Error(), "{alt}")
}

// --- InjectBuildConfig with hooks ---

func TestInjectBuildConfig_HooksInjected(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{
			Hooks: HooksCfg{
				Before: []HookCfg{
					{Command: "make start", Name: "start server"},
					{Command: "scripts/wait {port}", Params: map[string]string{"port": "3000"}},
				},
				After: []HookCfg{
					{Command: "make stop"},
				},
			},
		},
		Rules: map[string]RuleCfg{
			"recipe-safety": {Enabled: true},
		},
	}
	InjectBuildConfig(cfg, ".mdsmith.yml")

	rc := cfg.Rules["recipe-safety"]
	require.NotNil(t, rc.Settings)

	rawBefore, ok := rc.Settings["hooks-before"]
	require.True(t, ok, "hooks-before key must be present")
	before, ok := rawBefore.([]any)
	require.True(t, ok)
	require.Len(t, before, 2)

	h0, ok := before[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "make start", h0["command"])
	assert.Equal(t, "start server", h0["name"])

	rawAfter, ok := rc.Settings["hooks-after"]
	require.True(t, ok, "hooks-after key must be present")
	after, ok := rawAfter.([]any)
	require.True(t, ok)
	require.Len(t, after, 1)
}

func TestInjectBuildConfig_EmptyHooks_StillInjects(t *testing.T) {
	cfg := &Config{
		Build: BuildConfig{},
		Rules: map[string]RuleCfg{
			"recipe-safety": {Enabled: true},
		},
	}
	InjectBuildConfig(cfg, "")
	rc := cfg.Rules["recipe-safety"]
	require.NotNil(t, rc.Settings)
	rawBefore, ok := rc.Settings["hooks-before"]
	require.True(t, ok)
	before, ok := rawBefore.([]any)
	require.True(t, ok)
	assert.Empty(t, before)
}

// --- SerializeHooks ---

func TestSerializeHooks_Empty(t *testing.T) {
	out := SerializeHooks(nil)
	assert.Empty(t, out)
}

func TestSerializeHooks_Command(t *testing.T) {
	out := SerializeHooks([]HookCfg{{Command: "make start"}})
	require.Len(t, out, 1)
	m, ok := out[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "make start", m["command"])
	assert.NotContains(t, m, "name")
	assert.NotContains(t, m, "params")
}

func TestSerializeHooks_WithNameAndParams(t *testing.T) {
	out := SerializeHooks([]HookCfg{{
		Command: "scripts/wait {port}",
		Name:    "wait for port",
		Params:  map[string]string{"port": "3000"},
	}})
	require.Len(t, out, 1)
	m, ok := out[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "scripts/wait {port}", m["command"])
	assert.Equal(t, "wait for port", m["name"])
	params, ok := m["params"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "3000", params["port"])
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
