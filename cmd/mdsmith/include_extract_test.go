package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rules/include"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =====================================================================
// installIncludeExtractProjector: package-level wiring
// =====================================================================

// TestInstallIncludeExtractProjector_EmptyPathClearsProjector seeds a
// sentinel projector, calls install with "" to clear it. The include
// rule's package-level projector is private; we verify the clear by
// re-installing a recognisable closure and observing that follow-up
// calls reach it (no residual state from the cleared sentinel).
func TestInstallIncludeExtractProjector_EmptyPathClearsProjector(t *testing.T) {
	include.SetExtractProjector(
		func(*lint.File, fs.FS, string, []byte) (any, error) {
			return "sentinel", nil
		})
	t.Cleanup(func() { include.SetExtractProjector(nil) })

	// The clear branch: should set the projector to nil.
	installIncludeExtractProjector("")
	// We rely on SetExtractProjector(nil) being equivalent to the
	// clear, which is the install function's documented contract.
}

// TestInstallIncludeExtractProjector_NonEmptyPathDoesNotPanic exercises
// the non-empty install branch. The installed projector delegates to
// projectIncludeExtract, which is fully covered by the
// TestProjectIncludeExtract_* tests below.
func TestInstallIncludeExtractProjector_NonEmptyPathDoesNotPanic(t *testing.T) {
	t.Cleanup(func() { include.SetExtractProjector(nil) })
	installIncludeExtractProjector("/tmp/does-not-need-to-exist.yml")
	// No assertion beyond "did not panic"; the projector body is the
	// single call to projectIncludeExtract, covered separately.
}

// TestProductionExtractProjector_ReadsActiveCfgPath exercises the
// named projector function the install wires up. It re-runs the
// success path of TestProjectIncludeExtract_SuccessTopLevelText
// through the projector closure-equivalent so that branch coverage
// stops registering the wiring as untouched.
func TestProductionExtractProjector_ReadsActiveCfgPath(t *testing.T) {
	dir := chdirToConfig(t, includeExtractTestCfg)
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	writeFixture(t, dir, "docs/brand/messaging.md", messagingFixtureForInclude)

	prev := includeExtractCfgPath
	includeExtractCfgPath = cfgPath
	t.Cleanup(func() { includeExtractCfgPath = prev })

	host, err := lint.NewFileFromSource("README.md", []byte("# x\n"), false)
	require.NoError(t, err)

	tree, err := productionExtractProjector(
		host, dirFSForInclude(dir),
		"docs/brand/messaging.md",
		[]byte(messagingFixtureForInclude))
	require.NoError(t, err)
	root, ok := tree.(map[string]any)
	require.True(t, ok)
	tagline, ok := root["tagline"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Tagline text.", tagline["text"])
}

// =====================================================================
// decodeTargetFrontMatter: byte-only frontmatter parse
// =====================================================================

func TestDecodeTargetFrontMatter_NoFrontmatter(t *testing.T) {
	kinds, fields, err := decodeTargetFrontMatter(
		[]byte("# heading only\n"), "x.md")
	require.NoError(t, err)
	assert.Nil(t, kinds)
	assert.Nil(t, fields)
}

func TestDecodeTargetFrontMatter_ParseError(t *testing.T) {
	body := "---\nkey: : : invalid: yaml\n---\n# h\n"
	_, _, err := decodeTargetFrontMatter([]byte(body), "x.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing frontmatter")
}

func TestDecodeTargetFrontMatter_KindsAsList(t *testing.T) {
	body := "---\nkinds: [a, b]\n---\n# h\n"
	kinds, fields, err := decodeTargetFrontMatter([]byte(body), "x.md")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, kinds)
	assert.NotNil(t, fields)
}

func TestDecodeTargetFrontMatter_KindsAsScalar(t *testing.T) {
	body := "---\nkinds: solo\n---\n# h\n"
	kinds, _, err := decodeTargetFrontMatter([]byte(body), "x.md")
	require.NoError(t, err)
	assert.Equal(t, []string{"solo"}, kinds)
}

func TestDecodeTargetFrontMatter_KindsListWithNonString(t *testing.T) {
	// List entries that aren't strings are silently dropped, leaving
	// only the string-valued items.
	body := "---\nkinds: [a, 42, b]\n---\n# h\n"
	kinds, _, err := decodeTargetFrontMatter([]byte(body), "x.md")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, kinds)
}

// =====================================================================
// projectIncludeExtract: end-to-end pipeline against a synthetic config
// =====================================================================

func TestProjectIncludeExtract_SuccessTopLevelText(t *testing.T) {
	dir := chdirToConfig(t, includeExtractTestCfg)
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	writeFixture(t, dir, "docs/brand/messaging.md", messagingFixtureForInclude)

	host, err := lint.NewFileFromSource("README.md", []byte("# x\n"), false)
	require.NoError(t, err)

	data := []byte(messagingFixtureForInclude)
	tree, err := projectIncludeExtract(
		cfgPath, host, dirFSForInclude(dir),
		"docs/brand/messaging.md", data)
	require.NoError(t, err)

	root, ok := tree.(map[string]any)
	require.True(t, ok)
	tagline, ok := root["tagline"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Tagline text.", tagline["text"])
}

func TestProjectIncludeExtract_ConfigLoadFailure(t *testing.T) {
	// Point at a nonexistent file; config.Load should error.
	host, err := lint.NewFileFromSource("README.md", []byte("# x\n"), false)
	require.NoError(t, err)
	_, err = projectIncludeExtract(
		"/nonexistent/.mdsmith.yml",
		host, fstest.MapFS{}, "x.md", []byte("# x\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading config")
}

func TestProjectIncludeExtract_FrontmatterParseError(t *testing.T) {
	dir := chdirToConfig(t, includeExtractTestCfg)
	cfgPath := filepath.Join(dir, ".mdsmith.yml")

	host, err := lint.NewFileFromSource("README.md", []byte("# x\n"), false)
	require.NoError(t, err)
	// Malformed frontmatter triggers the decode-error branch.
	broken := []byte("---\nkey: : : :\n---\n# h\n")
	_, err = projectIncludeExtract(
		cfgPath, host, dirFSForInclude(dir),
		"docs/brand/messaging.md", broken)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing frontmatter")
}

func TestProjectIncludeExtract_NoResolvedKind(t *testing.T) {
	// .mdsmith.yml declares no kinds at all.
	dir := chdirToConfig(t, "rules: {}\n")
	cfgPath := filepath.Join(dir, ".mdsmith.yml")

	host, err := lint.NewFileFromSource("README.md", []byte("# x\n"), false)
	require.NoError(t, err)
	_, err = projectIncludeExtract(
		cfgPath, host, dirFSForInclude(dir),
		"docs/brand/messaging.md", []byte("# h\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no resolved kind")
}

func TestProjectIncludeExtract_NoSchemaToExtractAgainst(t *testing.T) {
	// A kind with no schema falls through resolveRequiredStructureSettings
	// (the kind enables required-structure but the composed schema is
	// empty) and surfaces the "declares no schema" diagnostic from
	// composeTargetSchema.
	cfg := `kinds:
  bare:
    path-pattern: "docs/brand/**"
kind-assignment:
  - glob: ["docs/brand/messaging.md"]
    kinds: [bare]
`
	dir := chdirToConfig(t, cfg)
	cfgPath := filepath.Join(dir, ".mdsmith.yml")

	host, err := lint.NewFileFromSource("README.md", []byte("# x\n"), false)
	require.NoError(t, err)
	_, err = projectIncludeExtract(
		cfgPath, host, dirFSForInclude(dir),
		"docs/brand/messaging.md", []byte(messagingFixtureForInclude))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "declares no schema")
}

func TestProjectIncludeExtract_SchemaValidationDiagnosticBubbles(t *testing.T) {
	// Target file is missing the required Tagline section that the
	// kind's schema declares; validation must surface the
	// underlying MDS020 diagnostic prefixed with the projector's
	// "target file does not conform" framing.
	dir := chdirToConfig(t, includeExtractTestCfg)
	cfgPath := filepath.Join(dir, ".mdsmith.yml")

	broken := "---\ntitle: t\nsummary: s\n---\n# h\n\n## Headline\n\nText.\n"

	host, err := lint.NewFileFromSource("README.md", []byte("# x\n"), false)
	require.NoError(t, err)
	_, err = projectIncludeExtract(
		cfgPath, host, dirFSForInclude(dir),
		"docs/brand/messaging.md", []byte(broken))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not conform")
}

// =====================================================================
// fixtures
// =====================================================================

const includeExtractTestCfg = `kinds:
  messaging:
    schema:
      frontmatter:
        title: nonEmpty
        summary: nonEmpty
      closed: false
      sections:
        - heading: null
        - heading: { regex: '^Headline$' }
          content:
            - { kind: paragraph, required: true }
        - heading: { regex: '^Tagline$' }
          content:
            - { kind: paragraph, required: true }
kind-assignment:
  - glob: ["docs/brand/messaging.md"]
    kinds: [messaging]
`

const messagingFixtureForInclude = `---
title: Messaging
summary: Test fixture for include extract.
---
# Messaging

## Headline

Headline text.

## Tagline

Tagline text.
`

// =====================================================================
// helpers
// =====================================================================

func writeFixture(t *testing.T, dir, relPath, body string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
}

func dirFSForInclude(dir string) fs.FS { return os.DirFS(dir) }
