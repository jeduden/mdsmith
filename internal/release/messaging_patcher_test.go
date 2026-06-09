package release

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"
)

// ----- JSONStringField ----------------------------------------------------

func TestMust_PanicsOnError(t *testing.T) {
	// The `must` helper wraps library calls that, for the inputs
	// we pass, cannot fail in practice (json.Marshal of a string,
	// json.Indent of a RawMessage we already decoded, etc.). Drive
	// the panic path directly so the helper's `if err != nil`
	// stays in the coverage report.
	defer func() {
		r := recover()
		require.NotNil(t, r)
		assert.Contains(t, fmt.Sprintf("%v", r), "impossible error")
	}()
	_ = must([]byte(nil), errors.New("boom"))
}

func TestMust_PassesValueThroughOnNilError(t *testing.T) {
	got := must([]byte("x"), nil)
	assert.Equal(t, []byte("x"), got)
}

func TestMustErr_PanicsOnError(t *testing.T) {
	defer func() {
		r := recover()
		require.NotNil(t, r)
	}()
	mustErr(errors.New("boom"))
}

func TestMustErr_NoOpOnNil(t *testing.T) {
	mustErr(nil)
}

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

func TestJSONStringField_ReadValue_TruncatedValue(t *testing.T) {
	// `{"k": tru` — invalid value token mid-decode. The
	// dec.Decode error path in decodeOrderedJSON should fire.
	_, err := (JSONStringField{Key: "k"}).ReadValue([]byte(`{"k": tru`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse json")
}

func TestJSONStringField_ReadValue_UnclosedObject(t *testing.T) {
	// Open object with no closing `}` — the closing-token
	// branch in decodeOrderedJSON returns an error.
	_, err := (JSONStringField{Key: "k"}).ReadValue([]byte(`{"k": "v"`))
	require.Error(t, err)
}

func TestJSONStringField_ReadValue_RejectsTrailingGarbage(t *testing.T) {
	// `{...} garbage` would silently parse if the decoder
	// stopped at `dec.More() == false` without consuming the
	// closing `}` and checking for EOF.
	body := []byte(`{"description": "x"} extra junk`)
	_, err := (JSONStringField{Key: "description"}).ReadValue(body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trailing")
}

func TestJSONStringField_PatchValue_PreservesTopLevelOrder(t *testing.T) {
	body := []byte(`{
  "name": "mdsmith",
  "description": "old",
  "version": "0.0.0-dev"
}
`)
	out, err := (JSONStringField{Key: "description"}).PatchValue(body, "new value")
	require.NoError(t, err)
	s := string(out)
	// Top-level keys keep the source order.
	nameIdx := strings.Index(s, `"name"`)
	descIdx := strings.Index(s, `"description"`)
	versionIdx := strings.Index(s, `"version"`)
	require.NotEqual(t, -1, nameIdx)
	require.NotEqual(t, -1, descIdx)
	require.NotEqual(t, -1, versionIdx)
	assert.Less(t, nameIdx, descIdx)
	assert.Less(t, descIdx, versionIdx)
	// New value lands at the description key.
	assert.Contains(t, s, `"description": "new value"`)
}

func TestJSONStringField_PatchValue_EscapesSpecialChars(t *testing.T) {
	body := []byte(`{"description": "x"}`)
	out, err := (JSONStringField{Key: "description"}).PatchValue(body,
		`has "quotes" and a \ backslash`)
	require.NoError(t, err)
	// json.Marshal escapes both characters.
	assert.Contains(t, string(out),
		`"description": "has \"quotes\" and a \\ backslash"`)
}

func TestJSONStringField_PatchValue_IdempotentAfterFirstNormalization(t *testing.T) {
	body := []byte(`{
  "name": "mdsmith",
  "description": "stable",
  "version": "0.0.0-dev"
}
`)
	out1, err := (JSONStringField{Key: "description"}).PatchValue(body, "stable")
	require.NoError(t, err)
	// First apply may normalize whitespace; second apply must be
	// byte-stable.
	out2, err := (JSONStringField{Key: "description"}).PatchValue(out1, "stable")
	require.NoError(t, err)
	assert.Equal(t, out1, out2, "second apply with same value should be byte-stable")
}

func TestJSONStringField_PatchValue_FieldMissing(t *testing.T) {
	body := []byte(`{"name": "mdsmith"}`)
	_, err := (JSONStringField{Key: "description"}).PatchValue(body, "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"description" not found`)
}

func TestJSONStringField_RejectsNonObjectRoot(t *testing.T) {
	_, err := (JSONStringField{Key: "k"}).PatchValue([]byte(`[1, 2]`), "v")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected object")
}

func TestJSONStringField_ReadValue_MalformedJSON(t *testing.T) {
	_, err := (JSONStringField{Key: "k"}).ReadValue([]byte(`{not json`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse json")
}

func TestJSONStringField_ReadValue_NonStringValue(t *testing.T) {
	// `k` is a number; ReadValue should reject — only string
	// fields are part of the messaging surface contract.
	_, err := (JSONStringField{Key: "k"}).ReadValue([]byte(`{"k": 42}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a string")
}

// ----- TOMLStringField ----------------------------------------------------

const sampleTOML = `baseURL = "https://example.test/"
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

func TestTOMLStringField_ReadValue_KeyMissing(t *testing.T) {
	_, err := (TOMLStringField{
		Table: []string{"params"},
		Key:   "nope",
	}).ReadValue([]byte(sampleTOML))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "params.nope not found")
}

func TestTOMLStringField_PatchValue_SetsTargetValue(t *testing.T) {
	out, err := (TOMLStringField{
		Table: []string{"params"},
		Key:   "description",
	}).PatchValue([]byte(sampleTOML), "new description")
	require.NoError(t, err)
	s := string(out)
	// Re-parse and confirm the value landed.
	got, err := (TOMLStringField{
		Table: []string{"params"},
		Key:   "description",
	}).ReadValue([]byte(s))
	require.NoError(t, err)
	assert.Equal(t, "new description", got)
	// Sibling keys preserved.
	assert.Contains(t, s, "githubRepo")
	assert.Contains(t, s, "version")
}

func TestTOMLStringField_PatchValue_EscapesSpecialChars(t *testing.T) {
	body := []byte("[params]\n  description = \"x\"\n")
	out, err := (TOMLStringField{
		Table: []string{"params"},
		Key:   "description",
	}).PatchValue(body, `with "quotes" \ slash`)
	require.NoError(t, err)
	// Round-trip the patched value through ReadValue to confirm
	// escapes survive — string equality on the emitted bytes is
	// fragile because go-toml v1 may pick its own escape form.
	got, err := (TOMLStringField{
		Table: []string{"params"},
		Key:   "description",
	}).ReadValue(out)
	require.NoError(t, err)
	assert.Equal(t, `with "quotes" \ slash`, got)
}

func TestTOMLStringField_PatchValue_IdempotentAfterFirstNormalization(t *testing.T) {
	body := []byte(sampleTOML)
	once, err := (TOMLStringField{
		Table: []string{"params"},
		Key:   "description",
	}).PatchValue(body, "stable description")
	require.NoError(t, err)
	twice, err := (TOMLStringField{
		Table: []string{"params"},
		Key:   "description",
	}).PatchValue(once, "stable description")
	require.NoError(t, err)
	assert.Equal(t, once, twice,
		"second apply with same value should be byte-stable")
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
	assert.Equal(t, "old lead line one, line two, line three.", got)
}

func TestYAMLFrontmatterField_PatchValue_TopLevel(t *testing.T) {
	out, err := (YAMLFrontmatterField{
		Path: []string{"summary"},
	}).PatchValue([]byte(sampleMDFrontmatter), "new summary")
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, `summary: "new summary"`)
	assert.True(t, strings.HasSuffix(s,
		"---\nBody content stays untouched.\n"))
}

func TestYAMLFrontmatterField_PatchValue_Nested(t *testing.T) {
	out, err := (YAMLFrontmatterField{
		Path: []string{"hero", "eyebrow"},
	}).PatchValue([]byte(sampleMDFrontmatter), "new eyebrow")
	require.NoError(t, err)
	assert.Contains(t, string(out), `eyebrow: "new eyebrow"`)
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
	assert.Equal(t, once, twice)
}

func TestYAMLFrontmatterField_PatchValue_AcceptsCRLFFrontmatter(t *testing.T) {
	// Windows-style line endings on the opening / closing
	// delimiters; the patcher should match the delimiter, not
	// assume `\n`.
	body := []byte("---\r\ntitle: x\r\nsummary: old\r\n---\r\nBody.\r\n")
	out, err := (YAMLFrontmatterField{
		Path: []string{"summary"},
	}).PatchValue(body, "a new summary")
	require.NoError(t, err)
	assert.Contains(t, string(out), "a new summary")
	assert.True(t, strings.HasSuffix(string(out), "Body.\r\n"),
		"body bytes after frontmatter must be preserved verbatim")
}

func TestYAMLFrontmatterField_PatchValue_AcceptsCloserAtEOF(t *testing.T) {
	// Frontmatter that ends with `---` and no trailing newline
	// (no body after the closer). The previous slice arithmetic
	// would panic on this input.
	body := []byte("---\ntitle: x\nsummary: old\n---")
	out, err := (YAMLFrontmatterField{
		Path: []string{"summary"},
	}).PatchValue(body, "fresh summary")
	require.NoError(t, err)
	assert.Contains(t, string(out), "fresh summary")
}

func TestYAMLFrontmatterField_PatchValue_NoFrontmatter(t *testing.T) {
	body := []byte("Body only, no frontmatter.\n")
	_, err := (YAMLFrontmatterField{
		Path: []string{"summary"},
	}).PatchValue(body, "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not start with a yaml frontmatter")
}

func TestYAMLFrontmatterField_PatchValue_UnclosedFrontmatter(t *testing.T) {
	body := []byte("---\ntitle: x\n(no closing)\n")
	_, err := (YAMLFrontmatterField{
		Path: []string{"title"},
	}).PatchValue(body, "y")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no closing delimiter")
}

func TestTOMLStringField_ReadValue_MalformedTOML(t *testing.T) {
	_, err := (TOMLStringField{Key: "k"}).ReadValue([]byte(`= broken`))
	require.Error(t, err)
}

func TestTOMLStringField_ReadValue_NonStringValue(t *testing.T) {
	_, err := (TOMLStringField{Key: "k"}).ReadValue([]byte("k = 42\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a string")
}

func TestTOMLStringField_PatchValue_MalformedTOML(t *testing.T) {
	_, err := (TOMLStringField{Key: "k"}).PatchValue([]byte(`= broken`), "v")
	require.Error(t, err)
}

func TestTOMLStringField_PatchValue_KeyNotFound(t *testing.T) {
	_, err := (TOMLStringField{Key: "absent"}).PatchValue([]byte("k = \"v\"\n"), "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestYAMLFrontmatterField_PatchValue_MalformedYAML(t *testing.T) {
	// Malformed YAML inside the delimiters; PatchValue must
	// surface the yaml.Unmarshal error rather than panic.
	body := []byte("---\n: : :\n---\nBody.\n")
	_, err := (YAMLFrontmatterField{
		Path: []string{"summary"},
	}).PatchValue(body, "x")
	require.Error(t, err)
}

func TestYAMLFrontmatterField_ReadValue_NoFrontmatter(t *testing.T) {
	_, err := (YAMLFrontmatterField{
		Path: []string{"summary"},
	}).ReadValue([]byte("no frontmatter here\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not start with a yaml frontmatter")
}

func TestYAMLFrontmatterField_ReadValue_MissingPath(t *testing.T) {
	body := []byte("---\ntitle: x\n---\n")
	_, err := (YAMLFrontmatterField{
		Path: []string{"missing"},
	}).ReadValue(body)
	require.Error(t, err)
}

func TestYAMLFrontmatterField_ReadValue_NonScalar(t *testing.T) {
	body := []byte("---\nlist:\n  - a\n  - b\n---\n")
	_, err := (YAMLFrontmatterField{
		Path: []string{"list"},
	}).ReadValue(body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a scalar")
}

func TestYAMLFrontmatterField_PatchValue_NonScalar(t *testing.T) {
	body := []byte("---\nlist:\n  - a\n  - b\n---\n")
	_, err := (YAMLFrontmatterField{
		Path: []string{"list"},
	}).PatchValue(body, "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a scalar")
}

func TestYAMLFrontmatterField_PatchValue_MissingPath(t *testing.T) {
	_, err := (YAMLFrontmatterField{
		Path: []string{"nope"},
	}).PatchValue([]byte(sampleMDFrontmatter), "x")
	require.Error(t, err)
}

func TestYAMLFrontmatterField_ReadValue_MalformedYAML(t *testing.T) {
	// Invalid YAML inside the delimiters fails Unmarshal.
	body := []byte("---\n: : :\n---\nBody.\n")
	_, err := (YAMLFrontmatterField{
		Path: []string{"summary"},
	}).ReadValue(body)
	require.Error(t, err)
}

func TestYAMLFrontmatterField_ReadValue_EmptyFrontmatter(t *testing.T) {
	// Empty frontmatter block makes findYAMLNode hit the
	// "empty frontmatter" branch.
	body := []byte("---\n---\nBody.\n")
	_, err := (YAMLFrontmatterField{
		Path: []string{"summary"},
	}).ReadValue(body)
	require.Error(t, err)
}

func TestYAMLFrontmatterField_PatchValue_ParentNotAMap(t *testing.T) {
	// Path traversal hits a scalar before reaching the leaf;
	// findYAMLNode reports "parent is not a map".
	body := []byte("---\nhero: a string, not a map\n---\nBody.\n")
	_, err := (YAMLFrontmatterField{
		Path: []string{"hero", "lead"},
	}).PatchValue(body, "x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a map")
}

func TestYAMLFrontmatterField_PatchValue_PreservesCRLFOpener(t *testing.T) {
	// CRLF opener / CRLF body must survive a round-trip — no
	// mixed-EOL output. splitFrontmatter now returns the opener
	// bytes; PatchValue re-emits them.
	body := []byte("---\r\nsummary: old\r\n---\r\nBody.\r\n")
	out, err := (YAMLFrontmatterField{
		Path: []string{"summary"},
	}).PatchValue(body, "new value")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(out), "---\r\n"),
		"opener line ending must stay CRLF, got %q",
		string(out[:min(len(out), 12)]))
}

func TestYAMLFrontmatterField_RejectsAliases(t *testing.T) {
	// Internal/yamlutil rejects anchors/aliases to prevent the
	// billion-laughs class of DoS on untrusted PR frontmatter.
	body := []byte("---\n" +
		"a: &x foo\n" +
		"b: *x\n" +
		"---\nBody.\n")
	_, err := (YAMLFrontmatterField{
		Path: []string{"a"},
	}).ReadValue(body)
	require.Error(t, err)
}

func TestFindYAMLNode_EmptyDocumentContent(t *testing.T) {
	// Construct a DocumentNode directly with no Content child;
	// the guard returns a structured error rather than panicking
	// on Content[0]. yaml.v3's Unmarshal does not produce this
	// shape for well-formed input today, but the guard locks the
	// invariant in case a future version changes that.
	root := &yaml.Node{Kind: yaml.DocumentNode}
	_, err := findYAMLNode(root, []string{"summary"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty yaml frontmatter")
}
