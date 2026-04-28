package recipesafety

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFile creates a lint.File for the given path with empty content.
func newFile(t *testing.T, path string) *lint.File {
	t.Helper()
	f, err := lint.NewFile(path, []byte{})
	require.NoError(t, err)
	return f
}

// newRule builds a Rule already loaded with the given recipes map.
func newRule(recipes map[string]recipe) *Rule {
	return &Rule{Recipes: recipes}
}

// --- Metadata ---

func TestRule_ID(t *testing.T) {
	assert.Equal(t, "MDS040", (&Rule{}).ID())
}

func TestRule_Name(t *testing.T) {
	assert.Equal(t, "recipe-safety", (&Rule{}).Name())
}

func TestRule_Category(t *testing.T) {
	assert.Equal(t, "meta", (&Rule{}).Category())
}

func TestRule_IsConfigFileRule(t *testing.T) {
	assert.True(t, (&Rule{}).IsConfigFileRule())
}

// --- DefaultSettings / ApplySettings ---

func TestDefaultSettings_Empty(t *testing.T) {
	s := (&Rule{}).DefaultSettings()
	assert.Empty(t, s)
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"bogus": "value"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown setting")
}

func TestApplySettings_ConfigPath(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"config-path": "/some/path/.mdsmith.yml"})
	require.NoError(t, err)
	assert.Equal(t, "/some/path/.mdsmith.yml", r.ConfigPath)
}

func TestApplySettings_ConfigPath_NonString(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"config-path": 42})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config-path must be a string")
}

func TestApplySettings_Recipes_Valid(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"recipes": map[string]any{
			"mermaid": map[string]any{
				"command": "mmdc -i {input} -o {output}",
				"params": map[string]any{
					"required": []any{"input"},
					"optional": []any{"output"},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Contains(t, r.Recipes, "mermaid")
	assert.Equal(t, "mmdc -i {input} -o {output}", r.Recipes["mermaid"].Command)
	assert.Equal(t, []string{"input"}, r.Recipes["mermaid"].Required)
	assert.Equal(t, []string{"output"}, r.Recipes["mermaid"].Optional)
}

func TestApplySettings_Recipes_NonMap(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"recipes": "not a map"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "recipes must be a map")
}

func TestApplySettings_Recipes_RecipeNonMap(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"recipes": map[string]any{
			"bad": "not a map",
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `recipe "bad" must be a map`)
}

func TestApplySettings_Recipes_NoParams(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"recipes": map[string]any{
			"simple": map[string]any{
				"command": "tool run",
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "tool run", r.Recipes["simple"].Command)
	assert.Nil(t, r.Recipes["simple"].Required)
	assert.Nil(t, r.Recipes["simple"].Optional)
}

// --- toStringSlice ---

func TestToStringSlice_Nil(t *testing.T) {
	assert.Nil(t, toStringSlice(nil))
}

func TestToStringSlice_StringSlice(t *testing.T) {
	in := []string{"a", "b"}
	assert.Equal(t, []string{"a", "b"}, toStringSlice(in))
}

func TestToStringSlice_AnySlice(t *testing.T) {
	in := []any{"a", "b"}
	assert.Equal(t, []string{"a", "b"}, toStringSlice(in))
}

func TestToStringSlice_AnySlice_SkipsNonString(t *testing.T) {
	in := []any{"a", 42, "b"}
	assert.Equal(t, []string{"a", "b"}, toStringSlice(in))
}

func TestToStringSlice_UnknownType(t *testing.T) {
	assert.Nil(t, toStringSlice(42))
}

// --- Check — dual-mode gating ---

func TestCheck_NoRecipes_ReturnsNil(t *testing.T) {
	r := &Rule{}
	f := newFile(t, "test.md")
	assert.Nil(t, r.Check(f))
}

func TestCheck_WithConfigPath_WrongFile_ReturnsNil(t *testing.T) {
	r := newRule(map[string]recipe{
		"bad": {Command: "bash script.sh"},
	})
	r.ConfigPath = "/project/.mdsmith.yml"
	f := newFile(t, "not-the-config.md")
	assert.Nil(t, r.Check(f))
}

func TestCheck_WithConfigPath_CorrectFile_RunsChecks(t *testing.T) {
	r := newRule(map[string]recipe{
		"bad": {Command: "bash script.sh"},
	})
	r.ConfigPath = ".mdsmith.yml"
	f := newFile(t, ".mdsmith.yml")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "shell interpreter")
}

func TestCheck_NoConfigPath_AlwaysRunsChecks(t *testing.T) {
	r := newRule(map[string]recipe{
		"bad": {Command: "bash script.sh"},
	})
	f := newFile(t, "anything.md")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "shell interpreter")
}

// --- Check 1: non-empty command ---

func TestCheck_EmptyCommand_ReportsError(t *testing.T) {
	r := newRule(map[string]recipe{"empty": {Command: ""}})
	f := newFile(t, "f.md")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, lint.Error, diags[0].Severity)
	assert.Contains(t, diags[0].Message, `recipe "empty": command must not be empty`)
}

// --- Check 2: shell interpreter ---

func TestCheck_ShellInterpreter_Bash(t *testing.T) {
	r := newRule(map[string]recipe{"x": {Command: "bash script.sh"}})
	diags := r.Check(newFile(t, "f.md"))
	require.Len(t, diags, 1)
	assert.Equal(t, lint.Error, diags[0].Severity)
	assert.Contains(t, diags[0].Message, "shell interpreter")
	assert.Contains(t, diags[0].Message, `"bash"`)
}

func TestCheck_ShellInterpreter_Sh(t *testing.T) {
	r := newRule(map[string]recipe{"x": {Command: "sh -c echo"}})
	diags := r.Check(newFile(t, "f.md"))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `"sh"`)
}

func TestCheck_ShellInterpreter_AbsPath(t *testing.T) {
	interps := []string{
		"/bin/sh", "/bin/bash", "/bin/zsh", "/bin/ksh", "/bin/fish",
		"/usr/bin/sh", "/usr/bin/bash",
	}
	for _, interp := range interps {
		r := newRule(map[string]recipe{"x": {Command: interp + " script.sh"}})
		diags := r.Check(newFile(t, "f.md"))
		assert.Len(t, diags, 1, "expected shell interpreter error for %s", interp)
	}
}

func TestCheck_ShellInterpreter_Zsh(t *testing.T) {
	r := newRule(map[string]recipe{"x": {Command: "zsh run.sh"}})
	diags := r.Check(newFile(t, "f.md"))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "shell interpreter")
}

func TestCheck_ShellInterpreter_Fish(t *testing.T) {
	r := newRule(map[string]recipe{"x": {Command: "fish run.sh"}})
	diags := r.Check(newFile(t, "f.md"))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "shell interpreter")
}

func TestCheck_ShellInterpreter_Ksh(t *testing.T) {
	r := newRule(map[string]recipe{"x": {Command: "ksh run.sh"}})
	diags := r.Check(newFile(t, "f.md"))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "shell interpreter")
}

// --- Check 3: shell operators in static parts ---

func TestCheck_ShellOperator_And(t *testing.T) {
	r := newRule(map[string]recipe{"x": {Command: "make all && make install"}})
	diags := r.Check(newFile(t, "f.md"))
	require.GreaterOrEqual(t, len(diags), 1)
	assert.Equal(t, lint.Error, diags[0].Severity)
	assert.Contains(t, diags[0].Message, "shell operator")
	assert.Contains(t, diags[0].Message, `"&&"`)
}

func TestCheck_ShellOperator_Or(t *testing.T) {
	r := newRule(map[string]recipe{"x": {Command: "tool arg || fallback"}})
	diags := r.Check(newFile(t, "f.md"))
	require.GreaterOrEqual(t, len(diags), 1)
	assert.Contains(t, diags[0].Message, `"||"`)
}

func TestCheck_ShellOperator_Semicolon(t *testing.T) {
	r := newRule(map[string]recipe{"x": {Command: "tool;other"}})
	diags := r.Check(newFile(t, "f.md"))
	require.GreaterOrEqual(t, len(diags), 1)
	assert.Contains(t, diags[0].Message, "shell operator")
}

func TestCheck_ShellOperator_Pipe(t *testing.T) {
	r := newRule(map[string]recipe{"x": {Command: "tool|grep foo"}})
	diags := r.Check(newFile(t, "f.md"))
	require.GreaterOrEqual(t, len(diags), 1)
	assert.Contains(t, diags[0].Message, "shell operator")
}

func TestCheck_ShellOperator_Redirect(t *testing.T) {
	r := newRule(map[string]recipe{"x": {Command: "tool >out.txt"}})
	diags := r.Check(newFile(t, "f.md"))
	require.GreaterOrEqual(t, len(diags), 1)
	assert.Contains(t, diags[0].Message, "shell operator")
}

func TestCheck_ShellOperator_RedirectIn(t *testing.T) {
	r := newRule(map[string]recipe{"x": {Command: "tool <in.txt"}})
	diags := r.Check(newFile(t, "f.md"))
	require.GreaterOrEqual(t, len(diags), 1)
	assert.Contains(t, diags[0].Message, "shell operator")
}

func TestCheck_ShellOperator_AppendRedirect(t *testing.T) {
	r := newRule(map[string]recipe{"x": {Command: "tool >>log.txt"}})
	diags := r.Check(newFile(t, "f.md"))
	require.GreaterOrEqual(t, len(diags), 1)
	assert.Contains(t, diags[0].Message, "shell operator")
}

func TestCheck_ShellOperator_Stderr(t *testing.T) {
	r := newRule(map[string]recipe{"x": {Command: "tool 2>err.txt"}})
	diags := r.Check(newFile(t, "f.md"))
	require.GreaterOrEqual(t, len(diags), 1)
	assert.Contains(t, diags[0].Message, "shell operator")
}

func TestCheck_ShellOperator_Backtick(t *testing.T) {
	r := newRule(map[string]recipe{"x": {Command: "tool `cmd`"}})
	diags := r.Check(newFile(t, "f.md"))
	require.GreaterOrEqual(t, len(diags), 1)
	assert.Contains(t, diags[0].Message, "shell operator")
}

func TestCheck_ShellOperator_DollarParen(t *testing.T) {
	r := newRule(map[string]recipe{"x": {Command: "tool $(cmd)"}})
	diags := r.Check(newFile(t, "f.md"))
	require.GreaterOrEqual(t, len(diags), 1)
	assert.Contains(t, diags[0].Message, "shell operator")
}

func TestCheck_ShellOperator_DollarBrace(t *testing.T) {
	r := newRule(map[string]recipe{"x": {Command: "tool ${VAR}"}})
	diags := r.Check(newFile(t, "f.md"))
	require.GreaterOrEqual(t, len(diags), 1)
	assert.Contains(t, diags[0].Message, "shell operator")
}

func TestCheck_SinglePlaceholder_NoOperatorError(t *testing.T) {
	// A token that is purely a single placeholder must not trigger operator check.
	r := newRule(map[string]recipe{
		"x": {
			Command:  "tool {input}",
			Required: []string{"input"},
		},
	})
	diags := r.Check(newFile(t, "f.md"))
	assert.Empty(t, diags)
}

// --- Check 4: fused placeholders ---

func TestCheck_FusedPlaceholders(t *testing.T) {
	r := newRule(map[string]recipe{
		"x": {
			Command:  "tool {a}{b}",
			Required: []string{"a", "b"},
		},
	})
	diags := r.Check(newFile(t, "f.md"))
	require.Len(t, diags, 1)
	assert.Equal(t, lint.Error, diags[0].Severity)
	assert.Contains(t, diags[0].Message, "fused placeholders")
	assert.Contains(t, diags[0].Message, `"{a}{b}"`)
}

func TestCheck_FusedPlaceholders_Three(t *testing.T) {
	r := newRule(map[string]recipe{
		"x": {
			Command:  "tool {a}{b}{c}",
			Required: []string{"a", "b", "c"},
		},
	})
	diags := r.Check(newFile(t, "f.md"))
	require.GreaterOrEqual(t, len(diags), 1)
	assert.Contains(t, diags[0].Message, "fused placeholders")
}

// --- Check 5: no .. in executable ---

func TestCheck_DotDot_Executable(t *testing.T) {
	r := newRule(map[string]recipe{
		"x": {
			Command:  "../../scripts/run.sh {input}",
			Required: []string{"input"},
		},
	})
	diags := r.Check(newFile(t, "f.md"))
	require.Len(t, diags, 1)
	assert.Equal(t, lint.Error, diags[0].Severity)
	assert.Contains(t, diags[0].Message, "..")
}

func TestCheck_DotDot_NotInExecutable_NoError(t *testing.T) {
	// .. in a non-first token should not trigger the executable check.
	r := newRule(map[string]recipe{
		"x": {
			Command:  "tool ../some/path",
			Required: []string{},
		},
	})
	diags := r.Check(newFile(t, "f.md"))
	assert.Empty(t, diags)
}

// --- Check 6: unused params (warning) ---

func TestCheck_UnusedParam_Required(t *testing.T) {
	r := newRule(map[string]recipe{
		"x": {
			Command:  "mmdc -i {input}",
			Required: []string{"input", "theme"},
		},
	})
	diags := r.Check(newFile(t, "f.md"))
	require.Len(t, diags, 1)
	assert.Equal(t, lint.Warning, diags[0].Severity)
	assert.Contains(t, diags[0].Message, `"theme"`)
	assert.Contains(t, diags[0].Message, "not referenced")
}

func TestCheck_UnusedParam_Optional(t *testing.T) {
	r := newRule(map[string]recipe{
		"x": {
			Command:  "mmdc -i {input}",
			Required: []string{"input"},
			Optional: []string{"unused"},
		},
	})
	diags := r.Check(newFile(t, "f.md"))
	require.Len(t, diags, 1)
	assert.Equal(t, lint.Warning, diags[0].Severity)
	assert.Contains(t, diags[0].Message, `"unused"`)
}

func TestCheck_AllParamsUsed_NoWarning(t *testing.T) {
	r := newRule(map[string]recipe{
		"x": {
			Command:  "mmdc -i {input} -o {output} --theme {theme}",
			Required: []string{"input", "output"},
			Optional: []string{"theme"},
		},
	})
	diags := r.Check(newFile(t, "f.md"))
	assert.Empty(t, diags)
}

func TestCheck_UnusedParams_SortedAlphabetically(t *testing.T) {
	r := newRule(map[string]recipe{
		"x": {
			Command:  "tool",
			Required: []string{"zebra", "alpha", "middle"},
		},
	})
	diags := r.Check(newFile(t, "f.md"))
	require.Len(t, diags, 3)
	assert.Contains(t, diags[0].Message, `"alpha"`)
	assert.Contains(t, diags[1].Message, `"middle"`)
	assert.Contains(t, diags[2].Message, `"zebra"`)
}

// --- Good path: no diagnostics ---

func TestCheck_SafeCommand_NoDiagnostics(t *testing.T) {
	r := newRule(map[string]recipe{
		"mermaid": {
			Command:  "mmdc -i {input} -o {output}",
			Required: []string{"input", "output"},
		},
	})
	diags := r.Check(newFile(t, "f.md"))
	assert.Empty(t, diags)
}

func TestCheck_MultipleRecipes_SortedByName(t *testing.T) {
	r := newRule(map[string]recipe{
		"z-recipe": {Command: "bash z.sh"},
		"a-recipe": {Command: "bash a.sh"},
	})
	diags := r.Check(newFile(t, "f.md"))
	require.Len(t, diags, 2)
	assert.Contains(t, diags[0].Message, `"a-recipe"`)
	assert.Contains(t, diags[1].Message, `"z-recipe"`)
}

// --- Diagnostic fields ---

func TestCheck_DiagnosticFields(t *testing.T) {
	r := newRule(map[string]recipe{"x": {Command: "bash script.sh"}})
	r.ConfigPath = "myconfig.yml"
	f := newFile(t, "myconfig.yml")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	d := diags[0]
	assert.Equal(t, "myconfig.yml", d.File)
	assert.Equal(t, 1, d.Line)
	assert.Equal(t, 1, d.Column)
	assert.Equal(t, "MDS040", d.RuleID)
	assert.Equal(t, "recipe-safety", d.RuleName)
	assert.Equal(t, lint.Error, d.Severity)
}
