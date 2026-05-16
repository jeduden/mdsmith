package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const extractUnitCfg = `kinds:
  recipe:
    schema:
      sections:
        - heading: "Goal"
        - heading: "Steps"
          sections:
            - heading:
                regex: 'Step \#(digits)'
                repeat: { min: 1 }
        - heading: "Notes"
          content:
            - kind: code-block
            - kind: list
kind-assignment:
  - glob: ["recipes/*.md"]
    kinds: [recipe]
`

const extractUnitDoc = `# Cake

## Goal

Bake a cake.

## Steps

### Step 1

Mix it.

## Notes

` + "```go\npreheat()\n```" + `

- cool it
`

// extractUnitDir seeds a temp cwd with the recipe config, a .git
// marker (so config discovery does not walk up), and the given
// files, then chdirs into it.
func extractUnitDir(t *testing.T, cfg string, files map[string]string) string {
	t.Helper()
	dir := chdirToConfig(t, cfg)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	for rel, body := range files {
		full := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
	}
	return dir
}

func TestRunExtract_JSONSuccess(t *testing.T) {
	extractUnitDir(t, extractUnitCfg, map[string]string{
		"recipes/cake.md": extractUnitDoc,
	})
	var code int
	out := captureStdout(func() {
		code = runExtract([]string{"recipe", "recipes/cake.md", "--format", "json"})
	})
	require.Equal(t, 0, code)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	steps := got["steps"].(map[string]any)
	arr := steps["step"].([]any)
	assert.Equal(t, "1", arr[0].(map[string]any)["n"])
	notes := got["notes"].(map[string]any)
	assert.Equal(t, "preheat()", notes["code"])
}

func TestRunExtract_YAMLAndMsgpack(t *testing.T) {
	extractUnitDir(t, extractUnitCfg, map[string]string{
		"recipes/cake.md": extractUnitDoc,
	})
	for _, f := range []string{"yaml", "msgpack"} {
		var code int
		_ = captureStdout(func() {
			code = runExtract([]string{"recipe", "recipes/cake.md", "--format", f})
		})
		assert.Equal(t, 0, code, "format %s", f)
	}
}

func TestRunExtract_NonConformantExitsOne(t *testing.T) {
	doc := "# Cake\n\n## Goal\n\nx\n" // missing Steps + Notes
	extractUnitDir(t, extractUnitCfg, map[string]string{
		"recipes/cake.md": doc,
	})
	var code int
	out := captureStdout(func() {
		code = runExtract([]string{"recipe", "recipes/cake.md"})
	})
	assert.Equal(t, 1, code)
	assert.NotContains(t, out, "\"goal\"")
}

func TestRunExtract_ArgAndKindErrors(t *testing.T) {
	extractUnitDir(t, extractUnitCfg, map[string]string{
		"recipes/cake.md": extractUnitDoc,
		"notes.md":        "# T\n\n## X\n",
	})
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"missing args", []string{"recipe"}, 2},
		{"bad format", []string{"recipe", "recipes/cake.md", "--format", "lua"}, 2},
		{"unknown kind", []string{"nope", "recipes/cake.md"}, 2},
		{"not assigned", []string{"recipe", "notes.md"}, 2},
		{"missing file", []string{"recipe", "recipes/missing.md"}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code := runExtract(tc.args)
			assert.Equal(t, tc.want, code)
		})
	}
}

func TestRunExtract_FlagParsing(t *testing.T) {
	// Unknown flag → usage error exit 2.
	assert.Equal(t, 2, runExtract([]string{"--nope"}))
	// --help → handled by reportFlagParseErr, exit 0.
	assert.Equal(t, 0, runExtract([]string{"--help"}))
}

func TestRunExtract_KindWithoutSchema(t *testing.T) {
	cfg := `kinds:
  bare:
    rules:
      paragraph-readability: false
kind-assignment:
  - glob: ["notes/*.md"]
    kinds: [bare]
`
	extractUnitDir(t, cfg, map[string]string{
		"notes/a.md": "# Title\n\n## Section\n\nbody\n",
	})
	code := runExtract([]string{"bare", "notes/a.md"})
	assert.Equal(t, 2, code)
}

// composedSchemaFor must refuse when required-structure is absent
// or disabled for the file: gateExtractCheck would skip MDS020 in
// that configuration, so projecting would emit data for a
// never-validated file (Copilot review on extract.go).
func TestComposedSchemaFor_RefusesWhenRuleDisabled(t *testing.T) {
	f, err := lint.NewFile("doc.md", []byte("# T\n"))
	require.NoError(t, err)

	_, code := composedSchemaFor(f, &config.FileResolution{}, "k")
	assert.Equal(t, 2, code)

	res := &config.FileResolution{
		Rules: map[string]config.RuleResolution{
			"required-structure": {Final: config.RuleCfg{Enabled: false}},
		},
	}
	_, code = composedSchemaFor(f, res, "k")
	assert.Equal(t, 2, code)
}

func TestKindAssigned(t *testing.T) {
	kinds := []config.ResolvedKind{{Name: "a"}, {Name: "b"}}
	assert.True(t, kindAssigned(kinds, "b"))
	assert.False(t, kindAssigned(kinds, "c"))
}
