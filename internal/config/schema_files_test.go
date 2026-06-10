package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mkSchemaDir creates `.mdsmith/schemas/` under a fresh temp dir and
// returns the temp dir. It is the shared setup for discoverSchemas
// tests, mirroring the discoverKinds fixtures.
func mkSchemaDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".mdsmith", "schemas"), 0o755))
	return dir
}

// writeSchema writes one schema file under `.mdsmith/schemas/`.
func writeSchema(t *testing.T, dir, name, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "schemas", name),
		[]byte(body), 0o644))
}

// TestDiscoverSchemas_EmptyWorkspaceReturnsEmpty pins the no-op
// branch: a workspace without `.mdsmith/schemas/` returns an empty
// map and no error, so Load can blindly merge the result.
func TestDiscoverSchemas_EmptyWorkspaceReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	got, err := discoverSchemas(dir)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestDiscoverSchemas_EmptyDirReturnsEmpty pins the case where the
// directory exists but holds no YAML files.
func TestDiscoverSchemas_EmptyDirReturnsEmpty(t *testing.T) {
	dir := mkSchemaDir(t)
	got, err := discoverSchemas(dir)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestDiscoverSchemas_LoadsBody covers the happy path: a schema file
// whose basename is the schema name, parsed into the raw body map and
// tagged with its source path.
func TestDiscoverSchemas_LoadsBody(t *testing.T) {
	dir := mkSchemaDir(t)
	writeSchema(t, dir, "rfc-v1.yaml", `filename: "RFC-[0-9][0-9][0-9][0-9].md"
frontmatter:
  title: 'string & != ""'
sections:
  - heading: "Overview"
  - heading: "Decision"
`)
	got, err := discoverSchemas(dir)
	require.NoError(t, err)
	require.Contains(t, got, "rfc-v1")
	ds := got["rfc-v1"]
	require.NotNil(t, ds.body)
	assert.Contains(t, ds.body, "filename")
	assert.Contains(t, ds.body, "frontmatter")
	assert.Contains(t, ds.body, "sections")
	assert.Equal(t,
		filepath.Join(dir, ".mdsmith", "schemas", "rfc-v1.yaml"),
		ds.sourcePath)
}

// TestDiscoverSchemas_AcceptsBothExtensions covers the
// `*.yaml`/`*.yml` glob — both are scanned and keyed by basename.
func TestDiscoverSchemas_AcceptsBothExtensions(t *testing.T) {
	dir := mkSchemaDir(t)
	writeSchema(t, dir, "foo.yaml", "filename: \"a.md\"\n")
	writeSchema(t, dir, "bar.yml", "filename: \"b.md\"\n")
	got, err := discoverSchemas(dir)
	require.NoError(t, err)
	assert.Contains(t, got, "foo")
	assert.Contains(t, got, "bar")
}

// TestDiscoverSchemas_RejectsExtensionCollision pins the
// basename-collision check across `.yaml` and `.yml`. The error names
// both files so the user can pick which to keep.
func TestDiscoverSchemas_RejectsExtensionCollision(t *testing.T) {
	dir := mkSchemaDir(t)
	writeSchema(t, dir, "foo.yaml", "filename: \"a.md\"\n")
	writeSchema(t, dir, "foo.yml", "filename: \"b.md\"\n")
	_, err := discoverSchemas(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foo")
	assert.Contains(t, err.Error(), "foo.yaml")
	assert.Contains(t, err.Error(), "foo.yml")
}

// TestDiscoverSchemas_RejectsSubdirectory pins the no-subdirectories
// rule — a nested file is a config error, not silently ignored.
func TestDiscoverSchemas_RejectsSubdirectory(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, ".mdsmith", "schemas", "nested")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(nested, "foo.yaml"), []byte("filename: \"a.md\"\n"), 0o644))
	_, err := discoverSchemas(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subdirector")
	assert.Contains(t, err.Error(), "nested")
}

// TestDiscoverSchemas_RejectsBadBasename pins the `[a-z][a-z0-9-]*`
// basename rule. Uppercase, leading digit, and underscore each fail.
func TestDiscoverSchemas_RejectsBadBasename(t *testing.T) {
	cases := []string{"Foo.yaml", "1bad.yaml", "bad_name.yaml", "BAD.yaml"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			dir := mkSchemaDir(t)
			writeSchema(t, dir, name, "filename: \"a.md\"\n")
			_, err := discoverSchemas(dir)
			require.Error(t, err)
			assert.Contains(t, err.Error(), name)
		})
	}
}

// TestDiscoverSchemas_RejectsUnknownKey pins the allowed-top-level-key
// rule — a key outside the schema vocabulary errors with the file and
// key name so a typo surfaces at load.
func TestDiscoverSchemas_RejectsUnknownKey(t *testing.T) {
	dir := mkSchemaDir(t)
	writeSchema(t, dir, "foo.yaml", "not-a-real-key: bar\nfilename: \"a.md\"\n")
	_, err := discoverSchemas(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foo.yaml")
	assert.Contains(t, err.Error(), "not-a-real-key")
}

// TestDiscoverSchemas_AcceptsAllVocabularyKeys pins that every key the
// schema vocabulary allows decodes without an unknown-key error.
func TestDiscoverSchemas_AcceptsAllVocabularyKeys(t *testing.T) {
	dir := mkSchemaDir(t)
	writeSchema(t, dir, "full.yaml", `filename: "a.md"
closed: true
frontmatter:
  title: 'string'
sections:
  - heading: "Overview"
cross-references: []
acronyms: {}
index: {}
`)
	got, err := discoverSchemas(dir)
	require.NoError(t, err)
	require.Contains(t, got, "full")
}

// TestDiscoverSchemas_IgnoresNonYAMLFiles pins the
// non-`.yaml`/`.yml` skip branch.
func TestDiscoverSchemas_IgnoresNonYAMLFiles(t *testing.T) {
	dir := mkSchemaDir(t)
	writeSchema(t, dir, "foo.yaml", "filename: \"a.md\"\n")
	writeSchema(t, dir, "README.md", "Notes\n")
	writeSchema(t, dir, ".keep", "")
	got, err := discoverSchemas(dir)
	require.NoError(t, err)
	require.Contains(t, got, "foo")
	assert.Len(t, got, 1, "non-YAML files must not produce extra schemas")
}

// TestDiscoverSchemas_RejectsYAMLAnchors pins the anchor/alias guard,
// parallel to discoverKinds (defence against billion-laughs).
func TestDiscoverSchemas_RejectsYAMLAnchors(t *testing.T) {
	dir := mkSchemaDir(t)
	writeSchema(t, dir, "foo.yaml",
		"frontmatter: &anchor {}\nindex: *anchor\n")
	_, err := discoverSchemas(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foo.yaml")
	assert.Contains(t, err.Error(), "anchors/aliases")
}

// TestDiscoverSchemas_RejectsBadYAML pins the decode-error path: a
// schema file whose body is not valid YAML surfaces the parse error
// with the file name.
func TestDiscoverSchemas_RejectsBadYAML(t *testing.T) {
	dir := mkSchemaDir(t)
	writeSchema(t, dir, "foo.yaml", "filename: [this is: not valid yaml\n")
	_, err := discoverSchemas(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foo.yaml")
}

// TestDiscoverSchemas_RejectsEmptyFile pins that an empty (or
// comments-only) schema file is a config error rather than a silent
// no-op — an empty schema can't constrain anything.
func TestDiscoverSchemas_RejectsEmptyFile(t *testing.T) {
	dir := mkSchemaDir(t)
	writeSchema(t, dir, "foo.yaml", "# just a comment\n")
	_, err := discoverSchemas(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foo.yaml")
}
