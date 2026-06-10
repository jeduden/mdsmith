package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMergeSchemaFiles_InlineOnly tags every inline `schemas:` entry
// with `.mdsmith.yml` and returns them when no schema files exist.
func TestMergeSchemaFiles_InlineOnly(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	cfg := &Config{
		Schemas: map[string]map[string]any{
			"rfc-v1": {"filename": "RFC-*.md"},
		},
	}
	reg, err := mergeSchemaFiles(cfg, cfgPath)
	require.NoError(t, err)
	require.Contains(t, reg, "rfc-v1")
	assert.Equal(t, cfgPath, reg["rfc-v1"].sourcePath)
	assert.Equal(t, "RFC-*.md", reg["rfc-v1"].body["filename"])
}

// TestMergeSchemaFiles_FileOnly tags a file-defined schema with its
// `.yaml` path.
func TestMergeSchemaFiles_FileOnly(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(dir, ".mdsmith", "schemas"), 0o755))
	schemaPath := filepath.Join(dir, ".mdsmith", "schemas", "runbook.yaml")
	require.NoError(t, os.WriteFile(schemaPath, []byte("filename: \"RUN-*.md\"\n"), 0o644))
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	cfg := &Config{}
	reg, err := mergeSchemaFiles(cfg, cfgPath)
	require.NoError(t, err)
	require.Contains(t, reg, "runbook")
	assert.Equal(t, schemaPath, reg["runbook"].sourcePath)
}

// TestMergeSchemaFiles_RejectsInlineVsFileCollision pins that a name
// declared both inline under `schemas:` AND as a file errors, naming
// both sources — the same rule kinds and conventions carry.
func TestMergeSchemaFiles_RejectsInlineVsFileCollision(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(dir, ".mdsmith", "schemas"), 0o755))
	schemaPath := filepath.Join(dir, ".mdsmith", "schemas", "rfc-v1.yaml")
	require.NoError(t, os.WriteFile(schemaPath, []byte("filename: \"a.md\"\n"), 0o644))
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	cfg := &Config{
		Schemas: map[string]map[string]any{
			"rfc-v1": {"filename": "b.md"},
		},
	}
	_, err := mergeSchemaFiles(cfg, cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rfc-v1")
	assert.Contains(t, err.Error(), ".mdsmith.yml")
	assert.Contains(t, err.Error(), "rfc-v1.yaml")
}

// TestMergeSchemaFiles_PropagatesDiscoveryError pins that a bad schema
// file (e.g. a basename violation) surfaces through mergeSchemaFiles.
func TestMergeSchemaFiles_PropagatesDiscoveryError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(dir, ".mdsmith", "schemas"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "schemas", "Bad.yaml"),
		[]byte("filename: \"a.md\"\n"), 0o644))
	cfg := &Config{}
	_, err := mergeSchemaFiles(cfg, filepath.Join(dir, ".mdsmith.yml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Bad.yaml")
}

// TestResolveNamedSchemas_HappyPath replaces a kind's named ref with
// the registry body and stamps the ref's SourcePath from the entry.
func TestResolveNamedSchemas_HappyPath(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"rfc": {Schema: KindSchemaRef{Name: "rfc-v1"}},
		},
	}
	reg := map[string]discoveredSchema{
		"rfc-v1": {
			body:       map[string]any{"filename": "RFC-*.md"},
			sourcePath: "/ws/.mdsmith/schemas/rfc-v1.yaml",
		},
	}
	require.NoError(t, resolveNamedSchemas(cfg, reg))
	got := cfg.Kinds["rfc"]
	require.NotNil(t, got.Schema.Map())
	assert.Equal(t, "RFC-*.md", got.Schema.Map()["filename"])
	assert.Equal(t, "/ws/.mdsmith/schemas/rfc-v1.yaml", got.Schema.SourcePath)
	assert.Equal(t, "rfc-v1", got.Schema.Name, "the name is retained for audit")
}

// TestResolveNamedSchemas_OneSchemaTwoKinds pins that one registry
// entry can drive several kinds.
func TestResolveNamedSchemas_OneSchemaTwoKinds(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"rfc":          {Schema: KindSchemaRef{Name: "rfc-v1"}},
			"rfc-internal": {Schema: KindSchemaRef{Name: "rfc-v1"}},
		},
	}
	reg := map[string]discoveredSchema{
		"rfc-v1": {body: map[string]any{"filename": "RFC-*.md"}, sourcePath: "x"},
	}
	require.NoError(t, resolveNamedSchemas(cfg, reg))
	assert.Equal(t, "RFC-*.md", cfg.Kinds["rfc"].Schema.Map()["filename"])
	assert.Equal(t, "RFC-*.md", cfg.Kinds["rfc-internal"].Schema.Map()["filename"])
}

// TestResolveNamedSchemas_UndeclaredNameErrors pins that an undeclared
// `schema:` name errors, naming the kind and the missing schema.
func TestResolveNamedSchemas_UndeclaredNameErrors(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"rfc": {Schema: KindSchemaRef{Name: "ghost"}},
		},
	}
	err := resolveNamedSchemas(cfg, map[string]discoveredSchema{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rfc")
	assert.Contains(t, err.Error(), "ghost")
}

// TestResolveNamedSchemas_InlinePassesThrough pins that a kind with an
// inline body (no Name) is left untouched — and keeps an empty
// SourcePath so the kind's own file applies later.
func TestResolveNamedSchemas_InlinePassesThrough(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"draft": {Schema: inlineSchemaRef(map[string]any{"filename": "DRAFT-*.md"})},
		},
	}
	require.NoError(t, resolveNamedSchemas(cfg, map[string]discoveredSchema{}))
	got := cfg.Kinds["draft"]
	assert.Equal(t, "DRAFT-*.md", got.Schema.Map()["filename"])
	assert.Empty(t, got.Schema.SourcePath)
	assert.Empty(t, got.Schema.Name)
}

// TestResolveNamedSchemas_NoKinds is a no-op on a config with no
// kinds.
func TestResolveNamedSchemas_NoKinds(t *testing.T) {
	cfg := &Config{}
	require.NoError(t, resolveNamedSchemas(cfg, map[string]discoveredSchema{}))
}

// TestLoad_ResolvesNamedSchemaEndToEnd pins the full Load path: a kind
// references a file schema by name, and the resolved body reaches the
// kind so MDS020 sees one inline body.
func TestLoad_ResolvesNamedSchemaEndToEnd(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(dir, ".mdsmith", "schemas"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "schemas", "rfc-v1.yaml"),
		[]byte("filename: \"RFC-[0-9][0-9][0-9][0-9].md\"\n"), 0o644))
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`kinds:
  rfc:
    schema: rfc-v1
`), 0o644))

	cfg, err := Load(cfgPath)
	require.NoError(t, err)
	body := cfg.Kinds["rfc"]
	require.NotNil(t, body.Schema.Map())
	assert.Equal(t, "RFC-[0-9][0-9][0-9][0-9].md", body.Schema.Map()["filename"])
	assert.Equal(t,
		filepath.Join(dir, ".mdsmith", "schemas", "rfc-v1.yaml"),
		body.Schema.SourcePath)
}

// TestLoad_UndeclaredNamedSchemaErrors pins that Load aborts when a
// kind references a schema name that no inline or file entry declares.
func TestLoad_UndeclaredNamedSchemaErrors(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`kinds:
  rfc:
    schema: ghost
`), 0o644))
	_, err := Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ghost")
	assert.Contains(t, err.Error(), "rfc")
}

// TestParseBytes_ResolvesInlineSchemaRegistry pins that the in-memory
// path (no disk discovery) still resolves a named ref against an
// inline `schemas:` registry.
func TestParseBytes_ResolvesInlineSchemaRegistry(t *testing.T) {
	cfg, err := ParseBytes([]byte(`schemas:
  rfc-v1:
    filename: "RFC-*.md"
kinds:
  rfc:
    schema: rfc-v1
`))
	require.NoError(t, err)
	body := cfg.Kinds["rfc"]
	require.NotNil(t, body.Schema.Map())
	assert.Equal(t, "RFC-*.md", body.Schema.Map()["filename"])
}
