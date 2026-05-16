package main_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"gopkg.in/yaml.v3"
)

const extractCfg = `kinds:
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

const conformantRecipe = `# Cake

## Goal

Bake a cake.

## Steps

### Step 1

Mix it.

### Step 2

Bake it.

## Notes

` + "```go\npreheat()\n```" + `

- cool it
- serve
`

const nonConformantRecipe = `# Cake

## Goal

Bake a cake.

## Notes

` + "```go\npreheat()\n```" + `

- cool it
`

func expectedRecipeTree() map[string]any {
	return map[string]any{
		"frontmatter": map[string]any{},
		"goal":        map[string]any{},
		"steps": map[string]any{
			"step": []any{
				map[string]any{"n": "1"},
				map[string]any{"n": "2"},
			},
		},
		"notes": map[string]any{
			"code":  "preheat()",
			"items": []any{"cool it", "serve"},
		},
	}
}

func TestE2E_Extract_JSON(t *testing.T) {
	dir := kindsTestDir(t, extractCfg, map[string]string{
		"recipes/cake.md": conformantRecipe,
	})
	stdout, stderr, code := runBinaryInDir(t, dir, "",
		"extract", "recipe", "recipes/cake.md", "--format", "json")
	require.Equal(t, 0, code, "stderr=%s", stderr)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	assert.Equal(t, expectedRecipeTree(), got)
}

func TestE2E_Extract_YAML_Msgpack_Equivalent(t *testing.T) {
	dir := kindsTestDir(t, extractCfg, map[string]string{
		"recipes/cake.md": conformantRecipe,
	})
	want := expectedRecipeTree()

	yOut, stderr, code := runBinaryInDir(t, dir, "",
		"extract", "recipe", "recipes/cake.md", "--format", "yaml")
	require.Equal(t, 0, code, "stderr=%s", stderr)
	var yGot map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(yOut), &yGot))
	assert.Equal(t, want, yGot)

	mOut, stderr, code := runBinaryInDir(t, dir, "",
		"extract", "recipe", "recipes/cake.md", "--format", "msgpack")
	require.Equal(t, 0, code, "stderr=%s", stderr)
	var mGot map[string]any
	require.NoError(t, msgpack.Unmarshal([]byte(mOut), &mGot))
	assert.Equal(t, want, mGot)
}

func TestE2E_Extract_NonConformantExitsNonZero(t *testing.T) {
	dir := kindsTestDir(t, extractCfg, map[string]string{
		"recipes/cake.md": nonConformantRecipe,
	})
	stdout, stderr, code := runBinaryInDir(t, dir, "",
		"extract", "recipe", "recipes/cake.md", "--format", "json")
	assert.Equal(t, 1, code)
	// Same diagnostics as `mdsmith check`: the missing Steps section.
	assert.Contains(t, stderr, "Steps")
	assert.NotContains(t, stdout, "\"goal\"")
}

func TestE2E_Extract_UnknownKind(t *testing.T) {
	dir := kindsTestDir(t, extractCfg, map[string]string{
		"recipes/cake.md": conformantRecipe,
	})
	_, stderr, code := runBinaryInDir(t, dir, "",
		"extract", "nope", "recipes/cake.md")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "unknown kind")
}

func TestE2E_Extract_BadFormat(t *testing.T) {
	dir := kindsTestDir(t, extractCfg, map[string]string{
		"recipes/cake.md": conformantRecipe,
	})
	_, stderr, code := runBinaryInDir(t, dir, "",
		"extract", "recipe", "recipes/cake.md", "--format", "lua")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "unknown format")
}

func TestE2E_Extract_MissingArgs(t *testing.T) {
	dir := kindsTestDir(t, extractCfg, map[string]string{
		"recipes/cake.md": conformantRecipe,
	})
	_, stderr, code := runBinaryInDir(t, dir, "", "extract", "recipe")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "requires <kind> and <file>")
}

func TestE2E_Extract_KindWithoutSchema(t *testing.T) {
	cfg := `kinds:
  bare:
    rules:
      paragraph-readability: false
kind-assignment:
  - glob: ["notes/*.md"]
    kinds: [bare]
`
	dir := kindsTestDir(t, cfg, map[string]string{
		"notes/a.md": "# Title\n\n## Section\n\nbody\n",
	})
	_, stderr, code := runBinaryInDir(t, dir, "",
		"extract", "bare", "notes/a.md")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "no schema")
}

func TestE2E_Extract_KindNotAssigned(t *testing.T) {
	dir := kindsTestDir(t, extractCfg, map[string]string{
		"notes.md": "## Goal\n\nx\n",
	})
	_, stderr, code := runBinaryInDir(t, dir, "",
		"extract", "recipe", "notes.md")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "not assigned")
}
