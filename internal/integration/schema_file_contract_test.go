package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Contract tests for the `.mdsmith/schemas/` directory surface
// (plan 241). Each case locks one rule the public surface promises so
// the contract survives refactor pressure. Per
// docs/development/architecture/cross-system.md every public surface
// ships with a contract test.

// schemaFileContractFixture stages a workspace with a config file and
// optional schema files under `.mdsmith/schemas/`. Returns the path of
// the loaded `.mdsmith.yml`.
func schemaFileContractFixture(
	t *testing.T, configBody string, schemaFiles map[string]string,
) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"), []byte(configBody), 0o644))
	if len(schemaFiles) > 0 {
		require.NoError(t, os.MkdirAll(
			filepath.Join(dir, ".mdsmith", "schemas"), 0o755))
		for name, body := range schemaFiles {
			require.NoError(t, os.WriteFile(
				filepath.Join(dir, ".mdsmith", "schemas", name),
				[]byte(body), 0o644))
		}
	}
	return filepath.Join(dir, ".mdsmith.yml")
}

// TestSchemaFileContract_LayoutAndReference locks the surface
// invariant: schemas live at `.mdsmith/schemas/<name>.{yaml,yml}`, a
// kind references one by name, and the resolved body reaches the kind.
func TestSchemaFileContract_LayoutAndReference(t *testing.T) {
	cfgPath := schemaFileContractFixture(t, `
kinds:
  rfc:
    schema: rfc-v1
`, map[string]string{
		"rfc-v1.yaml": "filename: \"RFC-[0-9][0-9][0-9][0-9].md\"\n",
	})
	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	body := cfg.Kinds["rfc"]
	require.NotNil(t, body.Schema.Map())
	assert.Equal(t, "RFC-[0-9][0-9][0-9][0-9].md", body.Schema.Map()["filename"])
	assert.Equal(t,
		filepath.Join(filepath.Dir(cfgPath), ".mdsmith", "schemas", "rfc-v1.yaml"),
		body.Schema.SourcePath,
		"a named file schema must carry its own .yaml path")
}

// TestSchemaFileContract_OneSchemaTwoKinds locks that one schema can
// drive several kinds.
func TestSchemaFileContract_OneSchemaTwoKinds(t *testing.T) {
	cfgPath := schemaFileContractFixture(t, `
kinds:
  rfc:
    schema: rfc-v1
  rfc-internal:
    schema: rfc-v1
`, map[string]string{
		"rfc-v1.yaml": "filename: \"RFC-*.md\"\n",
	})
	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "RFC-*.md", cfg.Kinds["rfc"].Schema.Map()["filename"])
	assert.Equal(t, "RFC-*.md", cfg.Kinds["rfc-internal"].Schema.Map()["filename"])
}

// TestSchemaFileContract_RejectsBadBasename locks the
// `[a-z][a-z0-9-]*` basename rule.
func TestSchemaFileContract_RejectsBadBasename(t *testing.T) {
	cases := []string{"Rfc_V1.yaml", "1rfc.yaml", "RfcV1.yaml"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			cfgPath := schemaFileContractFixture(t, "rules: {}\n",
				map[string]string{name: "filename: \"a.md\"\n"})
			_, err := config.Load(cfgPath)
			require.Error(t, err)
			assert.Contains(t, err.Error(), name)
		})
	}
}

// TestSchemaFileContract_RejectsSubdirectory locks the
// one-schema-per-file rule.
func TestSchemaFileContract_RejectsSubdirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"), []byte("rules: {}\n"), 0o644))
	nested := filepath.Join(dir, ".mdsmith", "schemas", "more")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(nested, "foo.yaml"), []byte("filename: \"a.md\"\n"), 0o644))
	_, err := config.Load(filepath.Join(dir, ".mdsmith.yml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subdirector")
}

// TestSchemaFileContract_RejectsSymlink locks the no-symlink rule.
// Skipped on platforms where the test user cannot create symlinks.
func TestSchemaFileContract_RejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"), []byte("rules: {}\n"), 0o644))
	schemasDir := filepath.Join(dir, ".mdsmith", "schemas")
	require.NoError(t, os.MkdirAll(schemasDir, 0o755))
	target := filepath.Join(dir, "real.yaml")
	require.NoError(t, os.WriteFile(target, []byte("filename: \"a.md\"\n"), 0o644))
	if err := os.Symlink(target, filepath.Join(schemasDir, "link.yaml")); err != nil {
		t.Skipf("cannot create symlink on this platform: %v", err)
	}
	_, err := config.Load(filepath.Join(dir, ".mdsmith.yml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
}

// TestSchemaFileContract_RejectsUnknownTopLevelKey locks the schema
// vocabulary: a key outside the allowed set errors naming the key and
// the file.
func TestSchemaFileContract_RejectsUnknownTopLevelKey(t *testing.T) {
	cfgPath := schemaFileContractFixture(t, "rules: {}\n", map[string]string{
		"rfc-v1.yaml": "section: []\n", // typo for "sections"
	})
	_, err := config.Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rfc-v1.yaml")
	assert.Contains(t, err.Error(), "section")
}

// TestSchemaFileContract_RejectsExtensionCollision locks the
// `.yaml`/`.yml` collision rule. Both filenames must surface.
func TestSchemaFileContract_RejectsExtensionCollision(t *testing.T) {
	cfgPath := schemaFileContractFixture(t, "rules: {}\n", map[string]string{
		"rfc-v1.yaml": "filename: \"a.md\"\n",
		"rfc-v1.yml":  "filename: \"b.md\"\n",
	})
	_, err := config.Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rfc-v1.yaml")
	assert.Contains(t, err.Error(), "rfc-v1.yml")
}

// TestSchemaFileContract_AcceptsBothExtensions locks that `.yaml` and
// `.yml` are both scanned.
func TestSchemaFileContract_AcceptsBothExtensions(t *testing.T) {
	cfgPath := schemaFileContractFixture(t, `
kinds:
  a:
    schema: foo
  b:
    schema: bar
`, map[string]string{
		"foo.yaml": "filename: \"a.md\"\n",
		"bar.yml":  "filename: \"b.md\"\n",
	})
	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "a.md", cfg.Kinds["a"].Schema.Map()["filename"])
	assert.Equal(t, "b.md", cfg.Kinds["b"].Schema.Map()["filename"])
}

// TestSchemaFileContract_RejectsInlineVsFileCollision locks the rule
// that a name declared both inline under `schemas:` and as a file
// errors naming both sources.
func TestSchemaFileContract_RejectsInlineVsFileCollision(t *testing.T) {
	cfgPath := schemaFileContractFixture(t, `
schemas:
  rfc-v1:
    filename: "inline.md"
`, map[string]string{
		"rfc-v1.yaml": "filename: \"file.md\"\n",
	})
	_, err := config.Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rfc-v1")
	assert.Contains(t, err.Error(), ".mdsmith.yml")
	assert.Contains(t, err.Error(), "rfc-v1.yaml")
}

// TestSchemaFileContract_RejectsUndeclaredName locks the rule that a
// `schema:` referencing an unknown name errors naming the kind and the
// missing schema.
func TestSchemaFileContract_RejectsUndeclaredName(t *testing.T) {
	cfgPath := schemaFileContractFixture(t, `
kinds:
  rfc:
    schema: ghost
`, nil)
	_, err := config.Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rfc")
	assert.Contains(t, err.Error(), "ghost")
}

// TestSchemaFileContract_RejectsNamedPlusFileSchema locks the first
// dual-source rejection: a named `schema:` plus a
// `rules.required-structure.schema:` file path.
func TestSchemaFileContract_RejectsNamedPlusFileSchema(t *testing.T) {
	cfgPath := schemaFileContractFixture(t, `
kinds:
  rfc:
    schema: rfc-v1
    rules:
      required-structure:
        schema: schemas/rfc.md
`, map[string]string{
		"rfc-v1.yaml": "filename: \"a.md\"\n",
	})
	_, err := config.Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rfc")
	assert.Contains(t, err.Error(), "pick one source")
}

// TestSchemaFileContract_RejectsNamedPlusInlineSchema locks the second
// dual-source rejection: a named `schema:` plus a
// `rules.required-structure.inline-schema:`.
func TestSchemaFileContract_RejectsNamedPlusInlineSchema(t *testing.T) {
	cfgPath := schemaFileContractFixture(t, `
kinds:
  rfc:
    schema: rfc-v1
    rules:
      required-structure:
        inline-schema:
          sections:
            - heading: "Overview"
`, map[string]string{
		"rfc-v1.yaml": "filename: \"a.md\"\n",
	})
	_, err := config.Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rfc")
	assert.Contains(t, err.Error(), "pick one source")
}
