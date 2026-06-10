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
	diags := r.validateHard("test.md", 1, map[string]string{"outputs": "out.png"})
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `missing required "recipe"`)
}

func TestValidateHard_MissingOutputs(t *testing.T) {
	r := ruleWithRender()
	diags := r.validateHard("test.md", 1, map[string]string{"recipe": "render", "source": "a.svg"})
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `missing required "outputs" list`)
}

func TestValidateHard_EmptyOutputsEntry(t *testing.T) {
	r := ruleWithRender()
	// A trailing empty entry alongside a valid one is a diagnostic.
	// (gensection joins a YAML list with "\n", so "out.png\n" is a
	// two-entry list ["out.png", ""].)
	diags := r.validateHard("test.md", 1, map[string]string{
		"recipe": "render", "source": "a.svg", "outputs": "out.png\n",
	})
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "must not be empty")
}

func TestValidateHard_WhitespaceOnlyOutputEntry(t *testing.T) {
	r := ruleWithRender()
	// A single whitespace entry (joined value is non-empty) is flagged
	// as an empty entry, not as a missing list.
	diags := r.validateHard("test.md", 1, map[string]string{
		"recipe": "render", "source": "a.svg", "outputs": "   ",
	})
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "must not be empty")
}

func TestValidateHard_OutputsSingleEmptyEntryReadsAsMissing(t *testing.T) {
	r := ruleWithRender()
	// A list with a single empty-string entry joins to "" —
	// indistinguishable from an absent list — so it reads as the
	// missing-list diagnostic. The fixture exercises the multi-entry
	// case where the empty-entry message surfaces.
	diags := r.validateHard("test.md", 1, map[string]string{
		"recipe": "render", "source": "a.svg", "outputs": "",
	})
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `missing required "outputs" list`)
}

func TestValidateHard_DotDotOutput(t *testing.T) {
	r := ruleWithRender()
	diags := r.validateHard("test.md", 1, map[string]string{
		"recipe": "render", "source": "a.svg", "outputs": "../out/file.png",
	})
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `".." path component`)
}

func TestValidateHard_AbsoluteOutput(t *testing.T) {
	r := ruleWithRender()
	diags := r.validateHard("test.md", 1, map[string]string{
		"recipe": "render", "source": "a.svg", "outputs": "/tmp/out.png",
	})
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "relative path")
}

func TestValidateHard_GlobInOutputRejected(t *testing.T) {
	r := ruleWithRender()
	diags := r.validateHard("test.md", 1, map[string]string{
		"recipe": "render", "source": "a.svg", "outputs": "out*.png",
	})
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "glob characters")
}

func TestValidateHard_MultipleOutputs_OneInvalid(t *testing.T) {
	r := ruleWithRender()
	// A second, invalid entry is rejected even when the first is fine.
	diags := r.validateHard("test.md", 1, map[string]string{
		"recipe": "render", "source": "a.svg", "outputs": "ok.png\n../bad.png",
	})
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `".." path component`)
}

func TestValidateHard_WindowsDriveLetter(t *testing.T) {
	r := ruleWithRender()
	// C:\out.png and C:out.png must be rejected on all platforms.
	for _, p := range []string{`C:\out.png`, `C:out.png`, `d:file.txt`} {
		diags := r.validateHard("test.md", 1, map[string]string{
			"recipe": "render", "source": "a.svg", "outputs": p,
		})
		require.Len(t, diags, 1, "path=%q", p)
	}
}

func TestValidateHard_BackslashAbsoluteOutput(t *testing.T) {
	r := ruleWithRender()
	// Windows-style absolute paths are rejected even on non-Windows hosts.
	for _, p := range []string{`\tmp\out.png`, `\\server\share\out.png`} {
		diags := r.validateHard("test.md", 1, map[string]string{
			"recipe": "render", "source": "a.svg", "outputs": p,
		})
		require.Len(t, diags, 1, "path=%q", p)
	}
}

func TestValidateHard_InvalidInput(t *testing.T) {
	r := ruleWithRender()
	diags := r.validateHard("test.md", 1, map[string]string{
		"recipe": "render", "source": "a.svg", "outputs": "out.png",
		"inputs": "../escape.md",
	})
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `".." path component`)
}

func TestValidateHard_GlobInput_Accepted(t *testing.T) {
	r := ruleWithRender()
	diags := r.validateHard("test.md", 1, map[string]string{
		"recipe": "render", "source": "a.svg", "outputs": "out.png",
		"inputs": "chapters/*.md\n**/extra.md",
	})
	assert.Empty(t, diags)
}

func TestValidateHard_EmptyInputs_Accepted(t *testing.T) {
	r := ruleWithRender()
	// inputs is optional; an absent list is fine.
	diags := r.validateHard("test.md", 1, map[string]string{
		"recipe": "render", "source": "a.svg", "outputs": "out.png",
	})
	assert.Empty(t, diags)
}

func TestValidateHard_EmptyRequiredParamValue(t *testing.T) {
	r := ruleWithRender()
	diags := r.validateHard("test.md", 1, map[string]string{
		"recipe": "render", "outputs": "out.png", "source": "   ",
	})
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `missing required parameter "source"`)
}

func TestValidateHard_UnknownRecipe(t *testing.T) {
	r := &Rule{}
	diags := r.validateHard("test.md", 1, map[string]string{
		"recipe": "nope", "outputs": "out.png",
	})
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `unknown recipe "nope"`)
}

func TestValidateHard_MissingRequiredParam(t *testing.T) {
	r := ruleWithRender()
	diags := r.validateHard("test.md", 1, map[string]string{
		"recipe": "render", "outputs": "out.png",
	})
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `missing required parameter "source"`)
}

func TestValidateHard_Valid(t *testing.T) {
	r := ruleWithRender()
	diags := r.validateHard("test.md", 1, map[string]string{
		"recipe": "render", "outputs": "out.png", "source": "diagram.svg",
	})
	assert.Empty(t, diags)
}

// --- warnUnknownParams ---

func TestWarnUnknownParams_Clean(t *testing.T) {
	r := ruleWithRender()
	diags := r.warnUnknownParams("test.md", 1, "render", renderRecipe, map[string]string{
		"recipe": "render", "outputs": "out.png", "source": "diagram.svg",
	})
	assert.Empty(t, diags)
}

func TestWarnUnknownParams_InputsAllowed(t *testing.T) {
	r := ruleWithRender()
	// inputs is a reserved known param and must not warn.
	diags := r.warnUnknownParams("test.md", 1, "render", renderRecipe, map[string]string{
		"recipe": "render", "outputs": "out.png", "source": "a.svg", "inputs": "in.md",
	})
	assert.Empty(t, diags)
}

func TestWarnUnknownParams_OptionalAllowed(t *testing.T) {
	r := ruleWithRender()
	diags := r.warnUnknownParams("test.md", 1, "render", renderRecipe, map[string]string{
		"recipe": "render", "outputs": "out.png", "source": "a.svg", "title": "My Chart",
	})
	assert.Empty(t, diags)
}

func TestWarnUnknownParams_Unknown(t *testing.T) {
	r := ruleWithRender()
	diags := r.warnUnknownParams("test.md", 1, "render", renderRecipe, map[string]string{
		"recipe": "render", "outputs": "out.png", "source": "a.svg", "extra": "val",
	})
	require.Len(t, diags, 1)
	assert.Equal(t, lint.Warning, diags[0].Severity)
	assert.Contains(t, diags[0].Message, `unknown parameter "extra"`)
}

func TestWarnUnknownParams_OutputSingularIsUnknown(t *testing.T) {
	r := ruleWithRender()
	// The old singular "output" param is no longer known; it draws the
	// unknown-param warning.
	diags := r.warnUnknownParams("test.md", 1, "render", renderRecipe, map[string]string{
		"recipe": "render", "outputs": "out.png", "source": "a.svg", "output": "stray.png",
	})
	require.Len(t, diags, 1)
	assert.Equal(t, lint.Warning, diags[0].Severity)
	assert.Contains(t, diags[0].Message, `unknown parameter "output"`)
}

func TestWarnUnknownParams_Sorted(t *testing.T) {
	r := ruleWithRender()
	diags := r.warnUnknownParams("test.md", 1, "render", renderRecipe, map[string]string{
		"recipe": "render", "outputs": "out.png", "source": "a.svg",
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
		"recipe": "render", "outputs": "docs/out.png", "source": "a.svg",
	})
	require.Empty(t, diags)
	assert.Equal(t, "![render output: docs/out.png](docs/out.png)\n", body)
}

func TestGenerateBody_DefaultTemplate(t *testing.T) {
	r := &Rule{recipes: map[string]recipeSchema{
		"plain": {Required: []string{"data"}},
	}}
	body, diags := r.generateBody("test.md", 1, map[string]string{
		"recipe": "plain", "outputs": "out.txt", "data": "input.csv",
	})
	require.Empty(t, diags)
	assert.Equal(t, "[out.txt](out.txt)\n", body)
}

func TestGenerateBody_AltDefault(t *testing.T) {
	r := ruleWithRender()
	body, _ := r.generateBody("test.md", 1, map[string]string{
		"recipe": "render", "outputs": "fig.png", "source": "a.svg",
	})
	assert.Equal(t, "![render output: fig.png](fig.png)\n", body)
}

func TestGenerateBody_MultipleOutputs(t *testing.T) {
	r := ruleWithRender()
	// The body-template renders once per outputs entry, in declared
	// order, joined with newlines.
	body, diags := r.generateBody("test.md", 1, map[string]string{
		"recipe": "render", "source": "a.svg",
		"outputs": "book.html\nbook.epub",
	})
	require.Empty(t, diags)
	assert.Equal(t,
		"![render output: book.html](book.html)\n"+
			"![render output: book.epub](book.epub)\n",
		body)
}

func TestGenerateBody_MultipleOutputs_DefaultTemplate(t *testing.T) {
	r := &Rule{recipes: map[string]recipeSchema{"plain": {Required: []string{"data"}}}}
	body, _ := r.generateBody("test.md", 1, map[string]string{
		"recipe": "plain", "data": "in.csv",
		"outputs": "a.txt\nb.txt\nc.txt",
	})
	assert.Equal(t, "[a.txt](a.txt)\n[b.txt](b.txt)\n[c.txt](c.txt)\n", body)
}

// --- Check (integration) ---

func TestCheck_NoDirectives(t *testing.T) {
	r := &Rule{}
	f := newFile(t, "# Hello\n\nNo directives here.\n")
	assert.Empty(t, r.Check(f))
}

func TestCheck_CorrectBody(t *testing.T) {
	r := ruleWithRender()
	src := "# Demo\n\n<?build\nrecipe: render\nsource: a.svg\noutputs:\n  - out.png\n?>\n" +
		"![render output: out.png](out.png)\n<?/build?>\n"
	f := newFile(t, src)
	assert.Empty(t, r.Check(f))
}

func TestCheck_MultiOutputCorrectBody(t *testing.T) {
	r := ruleWithRender()
	src := "# Demo\n\n<?build\nrecipe: render\nsource: a.svg\noutputs:\n  - book.html\n  - book.epub\n?>\n" +
		"![render output: book.html](book.html)\n" +
		"![render output: book.epub](book.epub)\n<?/build?>\n"
	f := newFile(t, src)
	assert.Empty(t, r.Check(f))
}

func TestCheck_StaleBody(t *testing.T) {
	r := ruleWithRender()
	src := "# Demo\n\n<?build\nrecipe: render\nsource: a.svg\noutputs:\n  - out.png\n?>\nwrong\n<?/build?>\n"
	f := newFile(t, src)
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "out of date")
}

func TestCheck_OutputSingularRejected(t *testing.T) {
	r := ruleWithRender()
	// Only the old singular `output:` is present — `outputs:` is
	// required, so this is a hard error, and the stray `output:` would
	// also warn (but the hard error short-circuits).
	src := "# Demo\n\n<?build\nrecipe: render\nsource: a.svg\noutput: out.png\n?>\ncontent\n<?/build?>\n"
	f := newFile(t, src)
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, lint.Error, diags[0].Severity)
	assert.Contains(t, diags[0].Message, `missing required "outputs" list`)
}

func TestCheck_UnknownRecipe(t *testing.T) {
	r := &Rule{}
	src := "# Test\n\n<?build\nrecipe: ghost\noutputs:\n  - out.png\n?>\ncontent\n<?/build?>\n"
	f := newFile(t, src)
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `unknown recipe "ghost"`)
}

func TestCheck_UnknownParam_AndCorrectBody(t *testing.T) {
	r := ruleWithRender()
	src := "# Demo\n\n<?build\nrecipe: render\nsource: a.svg\noutputs:\n  - out.png\nextra: val\n?>\n" +
		"![render output: out.png](out.png)\n<?/build?>\n"
	f := newFile(t, src)
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, lint.Warning, diags[0].Severity)
	assert.Contains(t, diags[0].Message, `unknown parameter "extra"`)
}

func TestCheck_UnknownParam_AndStaleBody(t *testing.T) {
	r := ruleWithRender()
	src := "# Demo\n\n<?build\nrecipe: render\nsource: a.svg\noutputs:\n  - out.png\nextra: val\n?>\n" +
		"wrong\n<?/build?>\n"
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
	src := "# Demo\n\n<?build\nrecipe: render\nsource: a.svg\noutputs:\n  - out.png\n?>\nwrong content\n<?/build?>\n"
	f := newFile(t, src)
	got := string(r.Fix(f))
	assert.Contains(t, got, "![render output: out.png](out.png)")
	assert.NotContains(t, got, "wrong content")
}

func TestFix_MultiOutput(t *testing.T) {
	r := ruleWithRender()
	src := "# Demo\n\n<?build\nrecipe: render\nsource: a.svg\noutputs:\n  - book.html\n  - book.epub\n?>\n" +
		"stale\n<?/build?>\n"
	f := newFile(t, src)
	got := string(r.Fix(f))
	assert.Contains(t, got, "![render output: book.html](book.html)")
	assert.Contains(t, got, "![render output: book.epub](book.epub)")
	assert.NotContains(t, got, "stale")
}

func TestFix_DefaultTemplate(t *testing.T) {
	r := &Rule{recipes: map[string]recipeSchema{
		"plain": {Required: []string{"data"}},
	}}
	src := "# Test\n\n<?build\nrecipe: plain\ndata: input.csv\noutputs:\n  - out.txt\n?>\nstale\n<?/build?>\n"
	f := newFile(t, src)
	got := string(r.Fix(f))
	assert.Contains(t, got, "[out.txt](out.txt)")
}

func TestFix_SkipsInvalidDirective(t *testing.T) {
	r := &Rule{}
	src := "# Test\n\n<?build\nrecipe: ghost\noutputs:\n  - out.png\n?>\ncontent\n<?/build?>\n"
	f := newFile(t, src)
	got := r.Fix(f)
	assert.Equal(t, src, string(got))
}

// --- splitList ---

func TestSplitList(t *testing.T) {
	cases := []struct {
		raw  string
		want []string
	}{
		{"", nil},
		{"a.png", []string{"a.png"}},
		{"a.png\nb.png", []string{"a.png", "b.png"}},
		// Empty entries are preserved so validatePathEntry can flag them.
		{"a.png\n", []string{"a.png", ""}},
		{"\n", []string{"", ""}},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, splitList(c.raw), "raw=%q", c.raw)
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
			"recipe": "render", "outputs": ext, "source": "a.svg",
		})
		assert.Empty(t, diags, "extension %q should be accepted", ext)
	}
}

// --- validatePathEntry (path-shape rules) ---

func TestValidatePathEntry_Valid(t *testing.T) {
	for _, p := range []string{
		"out.png", "docs/out.png", "a/b/c.txt", "out", "._hidden.md",
	} {
		assert.Empty(t, validatePathEntry(p, false), "path %q should be accepted", p)
		assert.Empty(t, validatePathEntry(p, true), "path %q should be accepted (glob)", p)
	}
}

func TestValidatePathEntry_Empty(t *testing.T) {
	for _, p := range []string{"", "   ", "\t"} {
		assert.NotEmpty(t, validatePathEntry(p, false), "path %q should be rejected", p)
	}
}

func TestValidatePathEntry_LeadingTrailingWhitespace(t *testing.T) {
	for _, p := range []string{" out.png", "out.png ", "\tout.png", "out.png\t"} {
		assert.NotEmpty(t, validatePathEntry(p, false), "path %q should be rejected", p)
	}
}

func TestValidatePathEntry_NUL(t *testing.T) {
	assert.NotEmpty(t, validatePathEntry("out\x00.png", false))
}

func TestValidatePathEntry_Newline(t *testing.T) {
	assert.NotEmpty(t, validatePathEntry("out\n.png", false))
	assert.NotEmpty(t, validatePathEntry("out\r.png", false))
}

func TestValidatePathEntry_Backslash(t *testing.T) {
	assert.NotEmpty(t, validatePathEntry(`a\b.png`, false))
}

func TestValidatePathEntry_DriveLetter(t *testing.T) {
	for _, p := range []string{`C:\out.png`, `C:out.png`, `d:file.txt`} {
		assert.NotEmpty(t, validatePathEntry(p, false), "path %q should be rejected", p)
	}
}

func TestValidatePathEntry_UNC(t *testing.T) {
	for _, p := range []string{`\\?\out.png`, `\\server\share\f`} {
		assert.NotEmpty(t, validatePathEntry(p, false), "path %q should be rejected", p)
	}
}

func TestValidatePathEntry_NTFSADS(t *testing.T) {
	// foo:bar — NTFS alternate data stream syntax.
	assert.NotEmpty(t, validatePathEntry("foo:bar", false))
	assert.NotEmpty(t, validatePathEntry("dir/foo:bar.txt", false))
}

func TestValidatePathEntry_ReservedDeviceNames(t *testing.T) {
	for _, p := range []string{
		"CON", "PRN", "AUX", "NUL", "COM1", "COM9", "LPT1", "LPT9",
		"con", "Con", "nul.txt", "dir/CON", "COM1.log",
	} {
		assert.NotEmpty(t, validatePathEntry(p, false), "path %q should be rejected", p)
	}
}

func TestValidatePathEntry_ReservedDeviceNames_NotMatchedAsSubstring(t *testing.T) {
	// CONSOLE / NULLABLE are not reserved device names.
	for _, p := range []string{"CONSOLE.md", "NULLABLE", "COMPANY", "LPT10"} {
		assert.Empty(t, validatePathEntry(p, false), "path %q should be accepted", p)
	}
}

func TestValidatePathEntry_Absolute(t *testing.T) {
	for _, p := range []string{"/tmp/out.png", "/out.png"} {
		assert.NotEmpty(t, validatePathEntry(p, false), "path %q should be rejected", p)
	}
}

func TestValidatePathEntry_Tilde(t *testing.T) {
	for _, p := range []string{"~/out.png", "~"} {
		assert.NotEmpty(t, validatePathEntry(p, false), "path %q should be rejected", p)
	}
}

func TestValidatePathEntry_DotDot(t *testing.T) {
	// A path that escapes root after Clean (or is "..") is rejected.
	for _, p := range []string{"../out.png", "..", "a/../../b.png"} {
		assert.NotEmpty(t, validatePathEntry(p, false), "path %q should be rejected", p)
	}
}

func TestValidatePathEntry_InteriorDotDotThatCleansInBounds(t *testing.T) {
	// Per the plan's path-shape rule, the check is on the result of
	// path.Clean: "a/../b.png" cleans to "b.png", which stays in-root,
	// so it is accepted.
	assert.Empty(t, validatePathEntry("a/../b.png", false))
}

func TestValidatePathEntry_UnderMdsmithDir(t *testing.T) {
	for _, p := range []string{".mdsmith/state", ".mdsmith/out.png"} {
		assert.NotEmpty(t, validatePathEntry(p, false), "path %q should be rejected", p)
	}
}

func TestValidatePathEntry_OutputsRejectGlobChars(t *testing.T) {
	// allowGlob=false: glob meta-characters are rejected.
	for _, p := range []string{"out*.png", "out?.png", "out[1].png"} {
		assert.NotEmpty(t, validatePathEntry(p, false), "path %q should be rejected for outputs", p)
	}
}

func TestValidatePathEntry_InputsAcceptGlobChars(t *testing.T) {
	// allowGlob=true: doublestar globs are accepted.
	for _, p := range []string{
		"src/*.md", "**/*.md", "chapters/[0-9]*.md", "a?b.md", "{a,b}.md",
	} {
		assert.Empty(t, validatePathEntry(p, true), "path %q should be accepted for inputs", p)
	}
}
