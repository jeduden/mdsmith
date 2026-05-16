package build

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFile parses inline markdown into a lint.File.
func newFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return f
}

// renderRecipe is a helper user-declared recipe used across tests.
var renderRecipe = recipeSchema{
	Required:     []string{"source"},
	Optional:     []string{"title"},
	BodyTemplate: "![{alt}]({output})",
}

// ruleWithRender returns a Rule pre-loaded with the "render" recipe.
func ruleWithRender() *Rule {
	return &Rule{recipes: map[string]recipeSchema{"render": renderRecipe}}
}

// --- Metadata ---

func TestRule_ID(t *testing.T) {
	assert.Equal(t, "MDS039", (&Rule{}).ID())
}

func TestRule_Name(t *testing.T) {
	assert.Equal(t, "build", (&Rule{}).Name())
}

func TestRule_Category(t *testing.T) {
	assert.Equal(t, "directive", (&Rule{}).Category())
}

// --- DefaultSettings / ApplySettings ---

func TestDefaultSettings_Empty(t *testing.T) {
	assert.Empty(t, (&Rule{}).DefaultSettings())
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"bogus": "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown setting")
}

func TestApplySettings_Recipes_Valid(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"recipes": map[string]any{
			"chart": map[string]any{
				"body-template": "![{alt}]({output})",
				"params": map[string]any{
					"required": []any{"data"},
					"optional": []any{"title"},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Contains(t, r.recipes, "chart")
	schema := r.recipes["chart"]
	assert.Equal(t, "![{alt}]({output})", schema.BodyTemplate)
	assert.Equal(t, []string{"data"}, schema.Required)
	assert.Equal(t, []string{"title"}, schema.Optional)
}

func TestApplySettings_Recipes_NotMap(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"recipes": "not-a-map"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "recipes must be a map")
}

// --- resolveRecipe ---

func TestResolveRecipe_UserDeclared(t *testing.T) {
	r := ruleWithRender()
	schema, ok := r.resolveRecipe("render")
	require.True(t, ok)
	assert.Equal(t, []string{"source"}, schema.Required)
}

func TestResolveRecipe_Unknown(t *testing.T) {
	r := &Rule{}
	_, ok := r.resolveRecipe("nonexistent")
	assert.False(t, ok)
}

func TestResolveRecipe_UnknownWhenNoRecipes(t *testing.T) {
	r := &Rule{} // no recipes loaded
	_, ok := r.resolveRecipe("render")
	assert.False(t, ok)
}

// --- validateHard ---

func TestValidateHard_MissingRecipe(t *testing.T) {
	r := ruleWithRender()
	diags := r.validateHard("test.md", 1, map[string]string{"output": "out.png"})
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `missing required "recipe"`)
}

func TestValidateHard_MissingOutput(t *testing.T) {
	r := ruleWithRender()
	diags := r.validateHard("test.md", 1, map[string]string{"recipe": "render", "source": "a.svg"})
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `missing required "output"`)
}

func TestValidateHard_DotDotOutput(t *testing.T) {
	r := ruleWithRender()
	diags := r.validateHard("test.md", 1, map[string]string{
		"recipe": "render", "source": "a.svg", "output": "../out/file.png",
	})
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `".." path component`)
}

func TestValidateHard_AbsoluteOutput(t *testing.T) {
	r := ruleWithRender()
	diags := r.validateHard("test.md", 1, map[string]string{
		"recipe": "render", "source": "a.svg", "output": "/tmp/out.png",
	})
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "relative path")
}

func TestValidateHard_WindowsDriveLetter(t *testing.T) {
	r := ruleWithRender()
	// C:\out.png and C:out.png must be rejected on all platforms.
	for _, p := range []string{`C:\out.png`, `C:out.png`, `d:file.txt`} {
		diags := r.validateHard("test.md", 1, map[string]string{
			"recipe": "render", "source": "a.svg", "output": p,
		})
		require.Len(t, diags, 1, "path=%q", p)
		assert.Contains(t, diags[0].Message, "relative path", "path=%q", p)
	}
}

func TestValidateHard_BackslashAbsoluteOutput(t *testing.T) {
	r := ruleWithRender()
	// Windows-style absolute paths are rejected even on non-Windows hosts.
	for _, p := range []string{`\tmp\out.png`, `\\server\share\out.png`} {
		diags := r.validateHard("test.md", 1, map[string]string{
			"recipe": "render", "source": "a.svg", "output": p,
		})
		require.Len(t, diags, 1, "path=%q", p)
		assert.Contains(t, diags[0].Message, "relative path", "path=%q", p)
	}
}

func TestValidateHard_EmptyRequiredParamValue(t *testing.T) {
	r := ruleWithRender()
	diags := r.validateHard("test.md", 1, map[string]string{
		"recipe": "render", "output": "out.png", "source": "   ",
	})
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `missing required parameter "source"`)
}

func TestValidateHard_UnknownRecipe(t *testing.T) {
	r := &Rule{}
	diags := r.validateHard("test.md", 1, map[string]string{
		"recipe": "nope", "output": "out.png",
	})
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `unknown recipe "nope"`)
}

func TestValidateHard_MissingRequiredParam(t *testing.T) {
	r := ruleWithRender()
	diags := r.validateHard("test.md", 1, map[string]string{
		"recipe": "render", "output": "out.png",
	})
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `missing required parameter "source"`)
}

func TestValidateHard_Valid(t *testing.T) {
	r := ruleWithRender()
	diags := r.validateHard("test.md", 1, map[string]string{
		"recipe": "render", "output": "out.png", "source": "diagram.svg",
	})
	assert.Empty(t, diags)
}

// --- warnUnknownParams ---

func TestWarnUnknownParams_Clean(t *testing.T) {
	r := ruleWithRender()
	diags := r.warnUnknownParams("test.md", 1, "render", renderRecipe, map[string]string{
		"recipe": "render", "output": "out.png", "source": "diagram.svg",
	})
	assert.Empty(t, diags)
}

func TestWarnUnknownParams_OptionalAllowed(t *testing.T) {
	r := ruleWithRender()
	diags := r.warnUnknownParams("test.md", 1, "render", renderRecipe, map[string]string{
		"recipe": "render", "output": "out.png", "source": "a.svg", "title": "My Chart",
	})
	assert.Empty(t, diags)
}

func TestWarnUnknownParams_Unknown(t *testing.T) {
	r := ruleWithRender()
	diags := r.warnUnknownParams("test.md", 1, "render", renderRecipe, map[string]string{
		"recipe": "render", "output": "out.png", "source": "a.svg", "extra": "val",
	})
	require.Len(t, diags, 1)
	assert.Equal(t, lint.Warning, diags[0].Severity)
	assert.Contains(t, diags[0].Message, `unknown parameter "extra"`)
}

func TestWarnUnknownParams_Sorted(t *testing.T) {
	r := ruleWithRender()
	diags := r.warnUnknownParams("test.md", 1, "render", renderRecipe, map[string]string{
		"recipe": "render", "output": "out.png", "source": "a.svg",
		"zzz": "1", "aaa": "2",
	})
	require.Len(t, diags, 2)
	assert.Contains(t, diags[0].Message, `"aaa"`)
	assert.Contains(t, diags[1].Message, `"zzz"`)
}

// --- generateBody ---

func TestGenerateBody_CustomTemplate(t *testing.T) {
	r := ruleWithRender()
	body, diags := r.generateBody("test.md", 1, map[string]string{
		"recipe": "render", "output": "docs/out.png", "source": "a.svg",
	})
	require.Empty(t, diags)
	assert.Equal(t, "![render output: docs/out.png](docs/out.png)\n", body)
}

func TestGenerateBody_DefaultTemplate(t *testing.T) {
	r := &Rule{recipes: map[string]recipeSchema{
		"plain": {Required: []string{"data"}},
	}}
	body, diags := r.generateBody("test.md", 1, map[string]string{
		"recipe": "plain", "output": "out.txt", "data": "input.csv",
	})
	require.Empty(t, diags)
	assert.Equal(t, "[out.txt](out.txt)\n", body)
}

func TestGenerateBody_AltDefault(t *testing.T) {
	r := ruleWithRender()
	body, _ := r.generateBody("test.md", 1, map[string]string{
		"recipe": "render", "output": "fig.png", "source": "a.svg",
	})
	assert.Equal(t, "![render output: fig.png](fig.png)\n", body)
}

// --- Check (integration) ---

func TestCheck_NoDirectives(t *testing.T) {
	r := &Rule{}
	f := newFile(t, "# Hello\n\nNo directives here.\n")
	assert.Empty(t, r.Check(f))
}

func TestCheck_CorrectBody(t *testing.T) {
	r := ruleWithRender()
	src := "# Demo\n\n<?build\nrecipe: render\nsource: a.svg\noutput: out.png\n?>\n" +
		"![render output: out.png](out.png)\n<?/build?>\n"
	f := newFile(t, src)
	assert.Empty(t, r.Check(f))
}

func TestCheck_StaleBody(t *testing.T) {
	r := ruleWithRender()
	src := "# Demo\n\n<?build\nrecipe: render\nsource: a.svg\noutput: out.png\n?>\nwrong\n<?/build?>\n"
	f := newFile(t, src)
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "out of date")
}

func TestCheck_UnknownRecipe(t *testing.T) {
	r := &Rule{}
	src := "# Test\n\n<?build\nrecipe: ghost\noutput: out.png\n?>\ncontent\n<?/build?>\n"
	f := newFile(t, src)
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `unknown recipe "ghost"`)
}

func TestCheck_UnknownParam_AndCorrectBody(t *testing.T) {
	r := ruleWithRender()
	src := "# Demo\n\n<?build\nrecipe: render\nsource: a.svg\noutput: out.png\nextra: val\n?>\n" +
		"![render output: out.png](out.png)\n<?/build?>\n"
	f := newFile(t, src)
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, lint.Warning, diags[0].Severity)
	assert.Contains(t, diags[0].Message, `unknown parameter "extra"`)
}

func TestCheck_UnknownParam_AndStaleBody(t *testing.T) {
	r := ruleWithRender()
	src := "# Demo\n\n<?build\nrecipe: render\nsource: a.svg\noutput: out.png\nextra: val\n?>\nwrong\n<?/build?>\n"
	f := newFile(t, src)
	diags := r.Check(f)
	require.Len(t, diags, 2)
	assert.Equal(t, lint.Warning, diags[0].Severity)
	assert.Contains(t, diags[0].Message, `unknown parameter "extra"`)
	assert.Equal(t, lint.Error, diags[1].Severity)
	assert.Contains(t, diags[1].Message, "out of date")
}

// --- Fix ---

func TestFix_RegeneratesBody(t *testing.T) {
	r := ruleWithRender()
	src := "# Demo\n\n<?build\nrecipe: render\nsource: a.svg\noutput: out.png\n?>\nwrong content\n<?/build?>\n"
	f := newFile(t, src)
	got := string(r.Fix(f))
	assert.Contains(t, got, "![render output: out.png](out.png)")
	assert.NotContains(t, got, "wrong content")
}

func TestFix_DefaultTemplate(t *testing.T) {
	r := &Rule{recipes: map[string]recipeSchema{
		"plain": {Required: []string{"data"}},
	}}
	src := "# Test\n\n<?build\nrecipe: plain\ndata: input.csv\noutput: out.txt\n?>\nstale\n<?/build?>\n"
	f := newFile(t, src)
	got := string(r.Fix(f))
	assert.Contains(t, got, "[out.txt](out.txt)")
}

func TestFix_SkipsInvalidDirective(t *testing.T) {
	r := &Rule{}
	src := "# Test\n\n<?build\nrecipe: ghost\noutput: out.png\n?>\ncontent\n<?/build?>\n"
	f := newFile(t, src)
	got := r.Fix(f)
	assert.Equal(t, src, string(got))
}

// --- hasDotDotSegment ---

func TestHasDotDotSegment(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"../out.png", true},
		{"a/../b.png", true},
		{"a/b/c.png", false},
		{"out.png", false},
		{"./out.png", false},
		{"..", true},
		{"a/b/..c.png", false},
		{`a\..\b.png`, true},
		{`a\b\c.png`, false},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, hasDotDotSegment(c.path), "path=%q", c.path)
	}
}

// --- Check with malformed directive YAML ---

func TestCheck_MalformedDirectiveYAML(t *testing.T) {
	r := ruleWithRender()
	// Invalid YAML in directive body causes ParseDirective to return nil + diagnostics.
	src := "# Test\n\n<?build\n{invalid: yaml: here\n?>\ncontent\n<?/build?>\n"
	f := newFile(t, src)
	diags := r.Check(f)
	// Should return parse diagnostics, not panic.
	require.NotEmpty(t, diags)
}

// --- parseRecipesSettings error branches ---

func TestApplySettings_Recipes_BodyTemplateNotString(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"recipes": map[string]any{
			"x": map[string]any{
				"body-template": 42,
			},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "body-template must be a string")
}

func TestApplySettings_Recipes_RecipeNotMap(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"recipes": map[string]any{
			"bad": "not-a-map",
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `recipe "bad" must be a map`)
}

func TestApplySettings_Recipes_ParamsNotMap(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"recipes": map[string]any{
			"x": map[string]any{
				"params": "not-a-map",
			},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `params must be a map`)
}

func TestApplySettings_Recipes_RequiredNotStringSlice(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"recipes": map[string]any{
			"x": map[string]any{
				"params": map[string]any{
					"required": []any{42},
				},
			},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "params.required")
}

func TestApplySettings_Recipes_OptionalNotStringSlice(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"recipes": map[string]any{
			"x": map[string]any{
				"params": map[string]any{
					"optional": []any{99},
				},
			},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "params.optional")
}

// --- toStringSlice edge cases ---

func TestToStringSlice_Nil(t *testing.T) {
	result, err := toStringSlice(nil)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestToStringSlice_StringSlice(t *testing.T) {
	result, err := toStringSlice([]string{"a", "b"})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, result)
}

func TestToStringSlice_AnySlice_NonString(t *testing.T) {
	_, err := toStringSlice([]any{"ok", 123})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "element 1")
}

func TestToStringSlice_WrongType(t *testing.T) {
	_, err := toStringSlice(42)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a string slice")
}

// --- output extension filter ---

func TestValidateHard_AnyExtension(t *testing.T) {
	r := ruleWithRender()
	for _, ext := range []string{"out.gif", "out.mp4", "out.svg", "out.txt", "out"} {
		diags := r.validateHard("test.md", 1, map[string]string{
			"recipe": "render", "output": ext, "source": "a.svg",
		})
		assert.Empty(t, diags, "extension %q should be accepted", ext)
	}
}
