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
		"title":       "Cake",
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

const inlineCfg = `kinds:
  hero:
    schema:
      sections:
        - heading: { regex: '^Headline$' }
          content:
            - { kind: paragraph, projection: inline, required: true }
kind-assignment:
  - glob: ["hero/*.md"]
    kinds: [hero]
`

const inlineHero = "# Hero\n\n## Headline\n\nMark*down*, smithed.\n"

func TestE2E_Extract_InlineJSON(t *testing.T) {
	dir := kindsTestDir(t, inlineCfg, map[string]string{
		"hero/page.md": inlineHero,
	})
	stdout, stderr, code := runBinaryInDir(t, dir, "",
		"extract", "hero", "hero/page.md", "--format", "json")
	require.Equal(t, 0, code, "stderr=%s", stderr)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	want := map[string]any{
		"frontmatter": map[string]any{},
		"title":       "Hero",
		"headline": map[string]any{
			"inline": []any{
				map[string]any{"span": "text", "value": "Mark"},
				map[string]any{"span": "emphasis", "level": float64(1),
					"children": []any{
						map[string]any{"span": "text", "value": "down"},
					}},
				map[string]any{"span": "text", "value": ", smithed."},
			},
		},
	}
	assert.Equal(t, want, got)
}

// The inline projection is one in-memory tree; only the serializer
// changes. YAML and msgpack must carry the same span list as JSON.
func TestE2E_Extract_InlineYAMLMsgpack(t *testing.T) {
	dir := kindsTestDir(t, inlineCfg, map[string]string{
		"hero/page.md": inlineHero,
	})

	yOut, stderr, code := runBinaryInDir(t, dir, "",
		"extract", "hero", "hero/page.md", "--format", "yaml")
	require.Equal(t, 0, code, "stderr=%s", stderr)
	var yGot map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(yOut), &yGot))
	assertInlineHeadline(t, yGot)

	mOut, stderr, code := runBinaryInDir(t, dir, "",
		"extract", "hero", "hero/page.md", "--format", "msgpack")
	require.Equal(t, 0, code, "stderr=%s", stderr)
	var mGot map[string]any
	require.NoError(t, msgpack.Unmarshal([]byte(mOut), &mGot))
	assertInlineHeadline(t, mGot)
}

// assertInlineHeadline checks the Mark*down*, smithed. span list in a
// decoded tree, tolerating the numeric type each decoder picks for
// the emphasis level (yaml → int, msgpack → int8).
func assertInlineHeadline(t *testing.T, tree map[string]any) {
	t.Helper()
	spans := tree["headline"].(map[string]any)["inline"].([]any)
	require.Len(t, spans, 3)
	assert.Equal(t, "text", spans[0].(map[string]any)["span"])
	assert.Equal(t, "Mark", spans[0].(map[string]any)["value"])
	em := spans[1].(map[string]any)
	assert.Equal(t, "emphasis", em["span"])
	assert.EqualValues(t, 1, em["level"])
	kids := em["children"].([]any)
	require.Len(t, kids, 1)
	assert.Equal(t, "down", kids[0].(map[string]any)["value"])
	assert.Equal(t, ", smithed.", spans[2].(map[string]any)["value"])
}

// An unsupported inline node (an image) is a hard error from extract
// when the schema asks for inline projection: exit 1, no partial
// JSON, and a diagnostic that names the problem.
func TestE2E_Extract_InlineUnsupportedNode(t *testing.T) {
	dir := kindsTestDir(t, inlineCfg, map[string]string{
		"hero/page.md": "# Hero\n\n## Headline\n\n![alt](logo.png)\n",
	})
	stdout, stderr, code := runBinaryInDir(t, dir, "",
		"extract", "hero", "hero/page.md", "--format", "json")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "image")
	assert.NotContains(t, stdout, "\"inline\"")
}

// A wrapped paragraph projects a `break` span between the two text
// runs so the line structure survives `mdsmith extract`. The plain
// wrap is a soft break (`hard: false`).
func TestE2E_Extract_InlineSoftBreak(t *testing.T) {
	dir := kindsTestDir(t, inlineCfg, map[string]string{
		"hero/page.md": "# Hero\n\n## Headline\n\nfirst\nsecond\n",
	})
	stdout, stderr, code := runBinaryInDir(t, dir, "",
		"extract", "hero", "hero/page.md", "--format", "json")
	require.Equal(t, 0, code, "stderr=%s", stderr)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	want := map[string]any{
		"frontmatter": map[string]any{},
		"title":       "Hero",
		"headline": map[string]any{
			"inline": []any{
				map[string]any{"span": "text", "value": "first"},
				map[string]any{"span": "break", "hard": false},
				map[string]any{"span": "text", "value": "second"},
			},
		},
	}
	assert.Equal(t, want, got)
}
