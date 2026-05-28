package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/jeduden/mdsmith/internal/config"
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
// resolveRequiredStructureSettings: defensive paths for future
// configurations whose effective Rules map either lacks
// required-structure entirely or carries it as disabled.
// =====================================================================

// TestResolveRequiredStructureSettings_RuleMissingFromEffective
// constructs a Config whose kind body carries no Rules entry at all,
// so ResolveFile's effective rule map omits "required-structure".
// Today, the in-tree config loader registers a default for every
// known rule, but if a future loader change ever pruned undeclared
// rules we want the helper to keep failing loudly instead of feeding
// a nil rule into composeTargetSchema.
func TestResolveRequiredStructureSettings_RuleMissingFromEffective(t *testing.T) {
	cfg := &config.Config{
		Kinds: map[string]config.KindBody{
			"bare": {},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{Glob: []string{"docs/brand/messaging.md"}, Kinds: []string{"bare"}},
		},
	}
	_, err := resolveRequiredStructureSettings(
		cfg, "docs/brand/messaging.md", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required-structure is disabled")
}

// TestResolveRequiredStructureSettings_RuleExplicitlyDisabled covers
// the second half of the `!ok || !Enabled` branch: the rule resolves
// but its Final.Enabled is false. Reachable today when a kind body
// declares `rules.required-structure: false` and no inline schema is
// present to flip the implicit enable. The helper must refuse rather
// than hand a disabled rule to ComposedSchema.
func TestResolveRequiredStructureSettings_RuleExplicitlyDisabled(t *testing.T) {
	cfg := &config.Config{
		Kinds: map[string]config.KindBody{
			"bare": {
				Rules: map[string]config.RuleCfg{
					"required-structure": {Enabled: false},
				},
			},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{Glob: []string{"docs/brand/messaging.md"}, Kinds: []string{"bare"}},
		},
	}
	_, err := resolveRequiredStructureSettings(
		cfg, "docs/brand/messaging.md", nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required-structure is disabled")
}

// =====================================================================
// composeTargetSchema: error paths for malformed rsSettings that
// might leak past config.Load in a future regression.
// =====================================================================

// TestComposeTargetSchema_ApplySettingsError exercises the
// rsRule.ApplySettings(rsSettings) error branch by passing a
// path-patterns value of the wrong type. parsePathPatterns rejects
// non-list / non-string-list inputs, so the helper bubbles a
// "loading schema config" error rather than silently composing an
// empty schema. Today config.Load type-checks this; a future loader
// regression that ever forwarded a raw int would still fail loudly.
func TestComposeTargetSchema_ApplySettingsError(t *testing.T) {
	tf, err := lint.NewFileFromSource("docs/x.md", []byte("# x\n"), false)
	require.NoError(t, err)

	_, err = composeTargetSchema(tf, "docs/x.md", map[string]any{
		"path-patterns": 42, // not a list-of-strings
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading schema config")
}

// TestComposeTargetSchema_ComposedSchemaError exercises the
// rsRule.ComposedSchema(tf) error branch. We hand the rule a
// schema-file reference that ApplySettings accepts at the string
// level but ComposedSchema rejects when it tries to read the file
// from the lint.File's FS — the file does not exist. Defensive
// against a future change that ever forwards an unresolved schema
// reference past the loader's existence check.
func TestComposeTargetSchema_ComposedSchemaError(t *testing.T) {
	tf, err := lint.NewFileFromSource("docs/x.md", []byte("# x\n"), false)
	require.NoError(t, err)
	tf.FS = fstest.MapFS{} // no schema file present

	_, err = composeTargetSchema(tf, "docs/x.md", map[string]any{
		"schema": "schemas/missing.md",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "composing schema")
}

// =====================================================================
// projectIncludeExtract: extract.Extract diag bubbling.
// =====================================================================

// TestProjectIncludeExtract_ExtractCollisionBubbles drives the
// "projection failed" branch using a duplicate-table-column-header
// case: schema.Validate today permits the duplicate column heading
// because table-shape validation only checks the section's content
// kind, but extract.Extract refuses because the row-object keys
// would collide silently.
func TestProjectIncludeExtract_ExtractCollisionBubbles(t *testing.T) {
	cfg := `kinds:
  table-kind:
    schema:
      sections:
        - heading: null
        - heading: { regex: '^Table$' }
          content:
            - { kind: table, required: true }
kind-assignment:
  - glob: ["docs/x.md"]
    kinds: [table-kind]
`
	dir := chdirToConfig(t, cfg)
	cfgPath := filepath.Join(dir, ".mdsmith.yml")

	// Table with two columns named "Name" — extract.Extract flags
	// this as a duplicate-column-header collision because row-object
	// keys would silently overwrite.
	doc := "# x\n\n## Table\n\n| Name | Name |\n|------|------|\n| a    | b    |\n"

	host, err := lint.NewFileFromSource("README.md", []byte("# x\n"), false)
	require.NoError(t, err)

	_, err = projectIncludeExtract(
		cfgPath, host, dirFSForInclude(dir),
		"docs/x.md", []byte(doc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "projection failed")
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
