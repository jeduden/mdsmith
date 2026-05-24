package release

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ----- JSONStringField ----------------------------------------------------

func TestJSONStringField_ReadValue(t *testing.T) {
	body := []byte(`{
  "name": "mdsmith",
  "description": "old value",
  "version": "0.0.0-dev"
}
`)
	got, err := (JSONStringField{Key: "description"}).ReadValue(body)
	require.NoError(t, err)
	assert.Equal(t, "old value", got)
}

func TestJSONStringField_ReadValue_HandlesEscapes(t *testing.T) {
	body := []byte(`{"description": "with \"quotes\" and a \\ backslash"}`)
	got, err := (JSONStringField{Key: "description"}).ReadValue(body)
	require.NoError(t, err)
	assert.Equal(t, `with "quotes" and a \ backslash`, got)
}

func TestJSONStringField_ReadValue_FieldMissing(t *testing.T) {
	body := []byte(`{"name": "mdsmith"}`)
	_, err := (JSONStringField{Key: "description"}).ReadValue(body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"description" not found`)
}

func TestJSONStringField_PatchValue(t *testing.T) {
	body := []byte(`{
  "name": "mdsmith",
  "description": "old",
  "version": "0.0.0-dev"
}
`)
	out, err := (JSONStringField{Key: "description"}).PatchValue(body, "new value")
	require.NoError(t, err)
	assert.Contains(t, string(out), `"description": "new value"`)
	// Other fields untouched (byte-stable except the changed line).
	assert.Contains(t, string(out), `"name": "mdsmith"`)
	assert.Contains(t, string(out), `"version": "0.0.0-dev"`)
}

func TestJSONStringField_PatchValue_EscapesSpecialChars(t *testing.T) {
	body := []byte(`{"description": "x"}`)
	out, err := (JSONStringField{Key: "description"}).PatchValue(body,
		`has "quotes" and a \ backslash`)
	require.NoError(t, err)
	assert.Equal(t,
		`{"description": "has \"quotes\" and a \\ backslash"}`, string(out))
}

func TestJSONStringField_PatchValue_Idempotent(t *testing.T) {
	body := []byte(`{"description": "stable"}`)
	out1, err := (JSONStringField{Key: "description"}).PatchValue(body, "stable")
	require.NoError(t, err)
	assert.Equal(t, body, out1, "patch with same value should be byte-stable")
	out2, err := (JSONStringField{Key: "description"}).PatchValue(out1, "stable")
	require.NoError(t, err)
	assert.Equal(t, out1, out2)
}

// ----- TOMLStringField ----------------------------------------------------

const sampleTOML = `# Hugo config

baseURL = "https://example.test/"
title = "mdsmith"

[markup.highlight]
  style = "github-dark"

[params]
  description = "old description"
  githubRepo = "jeduden/mdsmith"
  version = "0.0.0-dev"

[outputs]
  home = ["HTML"]
`

func TestTOMLStringField_ReadValue_TableScoped(t *testing.T) {
	got, err := (TOMLStringField{
		Table: []string{"params"},
		Key:   "description",
	}).ReadValue([]byte(sampleTOML))
	require.NoError(t, err)
	assert.Equal(t, "old description", got)
}

func TestTOMLStringField_ReadValue_RootTable(t *testing.T) {
	got, err := (TOMLStringField{
		Key: "title",
	}).ReadValue([]byte(sampleTOML))
	require.NoError(t, err)
	assert.Equal(t, "mdsmith", got)
}

func TestTOMLStringField_ReadValue_KeyOutsideScopeNotFound(t *testing.T) {
	// `description` only exists under [params]; root-table read fails.
	_, err := (TOMLStringField{
		Key: "description",
	}).ReadValue([]byte(sampleTOML))
	require.Error(t, err)
}

func TestTOMLStringField_ReadValue_TableMissing(t *testing.T) {
	_, err := (TOMLStringField{
		Table: []string{"nope"},
		Key:   "description",
	}).ReadValue([]byte(sampleTOML))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "table nope not found")
}

func TestTOMLStringField_PatchValue_TableScoped(t *testing.T) {
	out, err := (TOMLStringField{
		Table: []string{"params"},
		Key:   "description",
	}).PatchValue([]byte(sampleTOML), "new description")
	require.NoError(t, err)
	assert.Contains(t, string(out), `description = "new description"`)
	// Sibling keys preserved.
	assert.Contains(t, string(out), `githubRepo = "jeduden/mdsmith"`)
	assert.Contains(t, string(out), `version = "0.0.0-dev"`)
	// Other tables preserved.
	assert.Contains(t, string(out), `[markup.highlight]`)
	assert.Contains(t, string(out), `style = "github-dark"`)
}

func TestTOMLStringField_PatchValue_EscapesSpecialChars(t *testing.T) {
	body := []byte("[params]\n  description = \"x\"\n")
	out, err := (TOMLStringField{
		Table: []string{"params"},
		Key:   "description",
	}).PatchValue(body, `with "quotes" \ slash`)
	require.NoError(t, err)
	assert.Contains(t, string(out),
		`description = "with \"quotes\" \\ slash"`)
}

func TestTOMLStringField_PatchValue_Idempotent(t *testing.T) {
	body := []byte(sampleTOML)
	out1, err := (TOMLStringField{
		Table: []string{"params"},
		Key:   "description",
	}).PatchValue(body, "old description")
	require.NoError(t, err)
	assert.Equal(t, body, out1)
}

// ----- YAMLFrontmatterField ----------------------------------------------

const sampleMDFrontmatter = `---
title: "mdsmith"
summary: "old summary"
hero:
  eyebrow: "old eyebrow"
  headline_pre: "Mark"
  headline_em: "down"
  headline_post: ", smithed."
  lead: >-
    old lead line one,
    line two,
    line three.
---
Body content stays untouched.
`

func TestYAMLFrontmatterField_ReadValue_TopLevel(t *testing.T) {
	got, err := (YAMLFrontmatterField{
		Path: []string{"summary"},
	}).ReadValue([]byte(sampleMDFrontmatter))
	require.NoError(t, err)
	assert.Equal(t, "old summary", got)
}

func TestYAMLFrontmatterField_ReadValue_Nested(t *testing.T) {
	got, err := (YAMLFrontmatterField{
		Path: []string{"hero", "eyebrow"},
	}).ReadValue([]byte(sampleMDFrontmatter))
	require.NoError(t, err)
	assert.Equal(t, "old eyebrow", got)
}

func TestYAMLFrontmatterField_ReadValue_FoldedScalar(t *testing.T) {
	got, err := (YAMLFrontmatterField{
		Path: []string{"hero", "lead"},
	}).ReadValue([]byte(sampleMDFrontmatter))
	require.NoError(t, err)
	// Folded scalar joins lines with spaces and trims trailing
	// newlines under `>-`.
	assert.Equal(t, "old lead line one, line two, line three.", got)
}

func TestYAMLFrontmatterField_PatchValue_TopLevel(t *testing.T) {
	out, err := (YAMLFrontmatterField{
		Path: []string{"summary"},
	}).PatchValue([]byte(sampleMDFrontmatter), "new summary")
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, `summary: "new summary"`)
	// Body preserved verbatim (bytes after the closing ---).
	assert.True(t, strings.HasSuffix(s,
		"---\nBody content stays untouched.\n"))
}

func TestYAMLFrontmatterField_PatchValue_Nested(t *testing.T) {
	out, err := (YAMLFrontmatterField{
		Path: []string{"hero", "eyebrow"},
	}).PatchValue([]byte(sampleMDFrontmatter), "new eyebrow")
	require.NoError(t, err)
	assert.Contains(t, string(out), `eyebrow: "new eyebrow"`)
	// Sibling fields preserved.
	assert.Contains(t, string(out), `headline_pre:`)
	assert.Contains(t, string(out), `headline_em:`)
}

func TestYAMLFrontmatterField_PatchValue_PreservesBody(t *testing.T) {
	out, err := (YAMLFrontmatterField{
		Path: []string{"summary"},
	}).PatchValue([]byte(sampleMDFrontmatter), "new summary")
	require.NoError(t, err)
	assert.Contains(t, string(out), "Body content stays untouched.")
}

func TestYAMLFrontmatterField_PatchValue_Idempotent(t *testing.T) {
	body := []byte(sampleMDFrontmatter)
	once, err := (YAMLFrontmatterField{
		Path: []string{"summary"},
	}).PatchValue(body, "new summary")
	require.NoError(t, err)
	twice, err := (YAMLFrontmatterField{
		Path: []string{"summary"},
	}).PatchValue(once, "new summary")
	require.NoError(t, err)
	assert.Equal(t, once, twice,
		"second patch with same value should be byte-stable")
}

func TestYAMLFrontmatterField_PatchValue_NoFrontmatter(t *testing.T) {
	body := []byte("Body only, no frontmatter.\n")
	_, err := (YAMLFrontmatterField{
		Path: []string{"summary"},
	}).PatchValue(body, "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not start with a YAML frontmatter")
}

func TestYAMLFrontmatterField_PatchValue_UnclosedFrontmatter(t *testing.T) {
	body := []byte("---\ntitle: x\n(no closing)\n")
	_, err := (YAMLFrontmatterField{
		Path: []string{"title"},
	}).PatchValue(body, "y")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no closing delimiter")
}

func TestYAMLFrontmatterField_PatchValue_MissingPath(t *testing.T) {
	_, err := (YAMLFrontmatterField{
		Path: []string{"nope"},
	}).PatchValue([]byte(sampleMDFrontmatter), "x")
	require.Error(t, err)
}

// ----- MarkdownFragment ---------------------------------------------------

func TestMarkdownFragment_PatchValue(t *testing.T) {
	out, err := MarkdownFragment{}.PatchValue(nil, "Hello world.")
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, "Generated by `mdsmith-release sync-messaging`")
	assert.True(t, strings.HasSuffix(s, "Hello world.\n"))
}

func TestMarkdownFragment_RoundTrip(t *testing.T) {
	value := "Multi-line\nvalue with — em-dash."
	out, err := MarkdownFragment{}.PatchValue(nil, value)
	require.NoError(t, err)
	got, err := MarkdownFragment{}.ReadValue(out)
	require.NoError(t, err)
	assert.Equal(t, value, got)
}

func TestMarkdownFragment_PatchValue_Idempotent(t *testing.T) {
	once, err := MarkdownFragment{}.PatchValue(nil, "Same content.")
	require.NoError(t, err)
	twice, err := MarkdownFragment{}.PatchValue(once, "Same content.")
	require.NoError(t, err)
	assert.Equal(t, once, twice)
}

func TestMarkdownFragment_ReadValue_StripsHeader(t *testing.T) {
	body := []byte(fragmentHeader + "\n\nThe content.\n")
	got, err := MarkdownFragment{}.ReadValue(body)
	require.NoError(t, err)
	assert.Equal(t, "The content.", got)
}
