package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSchemaFile_DiagnosticsMatchInline pins plan 241's acceptance
// criterion #1 and #2: a kind that references a schema by name
// (`schema: rfc-v1`, body in `.mdsmith/schemas/rfc-v1.yaml`) produces
// byte-equal diagnostics to a kind that carries the same body inline.
// Two parallel workspaces share the same Markdown input; only the
// schema-source location differs, so a user moving an inline schema
// into a file sees no behavior shift (LSP: substitutable).
func TestSchemaFile_DiagnosticsMatchInline(t *testing.T) {
	// A doc missing the required "Decision" section so MDS020 fires a
	// real structure diagnostic under both sources.
	body := "# RFC\n\n## Overview\n\nText.\n"

	// schemaBody is the section schema both forms apply, indented for
	// the inline mapping under `kinds.rfc.schema:`.
	const inlineCfg = `
kinds:
  rfc:
    schema:
      sections:
        - heading: "Overview"
        - heading: "Decision"
kind-assignment:
  - glob: ["doc.md"]
    kinds: [rfc]
`
	inlineDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(inlineDir, ".mdsmith.yml"), []byte(inlineCfg), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(inlineDir, "doc.md"), []byte(body), 0o644))

	const fileCfg = `
kinds:
  rfc:
    schema: rfc-v1
kind-assignment:
  - glob: ["doc.md"]
    kinds: [rfc]
`
	fileDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(fileDir, ".mdsmith.yml"), []byte(fileCfg), 0o644))
	require.NoError(t, os.MkdirAll(
		filepath.Join(fileDir, ".mdsmith", "schemas"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(fileDir, ".mdsmith", "schemas", "rfc-v1.yaml"),
		[]byte("sections:\n  - heading: \"Overview\"\n  - heading: \"Decision\"\n"),
		0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(fileDir, "doc.md"), []byte(body), 0o644))

	inlineDiags := runCheckOnDoc(t, inlineDir)
	fileDiags := runCheckOnDoc(t, fileDir)

	require.NotEmpty(t, inlineDiags,
		"the inline schema must produce at least one diagnostic for a doc "+
			"missing a required section (otherwise the test proves nothing)")
	require.Equal(t, len(inlineDiags), len(fileDiags),
		"named-YAML schema must emit the same number of diagnostics as inline")
	for i := range inlineDiags {
		require.Equal(t, inlineDiags[i], fileDiags[i],
			"diagnostic %d must match between schema sources", i)
	}
}

// TestSchemaFile_InlineRegistryMatchesFile pins that the inline
// `schemas:` registry and a `.mdsmith/schemas/` file produce the same
// diagnostics for the same named reference — the registry is just the
// inline split of the file.
func TestSchemaFile_InlineRegistryMatchesFile(t *testing.T) {
	body := "# RFC\n\n## Overview\n\nText.\n"

	const registryCfg = `
schemas:
  rfc-v1:
    sections:
      - heading: "Overview"
      - heading: "Decision"
kinds:
  rfc:
    schema: rfc-v1
kind-assignment:
  - glob: ["doc.md"]
    kinds: [rfc]
`
	registryDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(registryDir, ".mdsmith.yml"), []byte(registryCfg), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(registryDir, "doc.md"), []byte(body), 0o644))

	const fileCfg = `
kinds:
  rfc:
    schema: rfc-v1
kind-assignment:
  - glob: ["doc.md"]
    kinds: [rfc]
`
	fileDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(fileDir, ".mdsmith.yml"), []byte(fileCfg), 0o644))
	require.NoError(t, os.MkdirAll(
		filepath.Join(fileDir, ".mdsmith", "schemas"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(fileDir, ".mdsmith", "schemas", "rfc-v1.yaml"),
		[]byte("sections:\n  - heading: \"Overview\"\n  - heading: \"Decision\"\n"),
		0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(fileDir, "doc.md"), []byte(body), 0o644))

	registryDiags := runCheckOnDoc(t, registryDir)
	fileDiags := runCheckOnDoc(t, fileDir)

	require.NotEmpty(t, registryDiags)
	require.Equal(t, len(registryDiags), len(fileDiags))
	for i := range registryDiags {
		require.Equal(t, registryDiags[i], fileDiags[i],
			"diagnostic %d must match between inline registry and file", i)
	}
}
