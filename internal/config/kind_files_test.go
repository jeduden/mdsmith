package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDiscoverKinds_EmptyWorkspaceReturnsEmpty pins the no-op
// branch: a workspace without `.mdsmith/kinds/` returns an empty
// map and no error, so callers can blindly merge the result.
func TestDiscoverKinds_EmptyWorkspaceReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	got, err := discoverKinds(dir)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestDiscoverKinds_EmptyKindsDirReturnsEmpty pins the case where
// `.mdsmith/kinds/` exists but holds no YAML files (e.g. a
// freshly-created tree before any kind file lands).
func TestDiscoverKinds_EmptyKindsDirReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".mdsmith", "kinds"), 0o755))
	got, err := discoverKinds(dir)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestDiscoverKinds_LoadsFullBody covers the happy path: a single
// kind file whose basename is the kind name, parsed into a full
// KindBody — schema, rules, path-pattern, extends.
func TestDiscoverKinds_LoadsFullBody(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".mdsmith", "kinds"), 0o755))
	body := `schema:
  frontmatter:
    title: 'string & != ""'
  closed: false
  sections:
    - heading: null
path-pattern: "docs/**/*.md"
rules:
  max-file-length:
    max: 600
`
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "kinds", "audit-log.yaml"),
		[]byte(body), 0o644))

	got, err := discoverKinds(dir)
	require.NoError(t, err)
	require.Contains(t, got, "audit-log")
	dk := got["audit-log"]
	assert.Equal(t, "docs/**/*.md", dk.body.PathPattern)
	assert.Equal(t, 600, dk.body.Rules["max-file-length"].Settings["max"])
	require.NotNil(t, dk.body.Schema)
	assert.Contains(t, dk.body.Schema, "frontmatter")
	assert.Equal(t,
		filepath.Join(dir, ".mdsmith", "kinds", "audit-log.yaml"),
		dk.sourcePath)
}

// TestDiscoverKinds_AcceptsBothExtensions covers the
// `*.yaml` and `*.yml` glob — both are scanned and produce kinds
// keyed by basename.
func TestDiscoverKinds_AcceptsBothExtensions(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".mdsmith", "kinds"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "kinds", "foo.yaml"),
		[]byte("rules: {}\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "kinds", "bar.yml"),
		[]byte("rules: {}\n"), 0o644))

	got, err := discoverKinds(dir)
	require.NoError(t, err)
	assert.Contains(t, got, "foo")
	assert.Contains(t, got, "bar")
}

// TestDiscoverKinds_RejectsExtensionCollision pins the
// basename-collision check across `.yaml` and `.yml`. The error
// names both files so the user can pick which to keep.
func TestDiscoverKinds_RejectsExtensionCollision(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".mdsmith", "kinds"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "kinds", "foo.yaml"),
		[]byte("rules: {}\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "kinds", "foo.yml"),
		[]byte("rules: {}\n"), 0o644))

	_, err := discoverKinds(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foo")
	assert.Contains(t, err.Error(), "foo.yaml")
	assert.Contains(t, err.Error(), "foo.yml")
}

// TestDiscoverKinds_RejectsSubdirectory pins the
// no-subdirectories rule — a nested file is a config error rather
// than being silently ignored.
func TestDiscoverKinds_RejectsSubdirectory(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, ".mdsmith", "kinds", "nested")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(nested, "foo.yaml"), []byte("rules: {}\n"), 0o644))

	_, err := discoverKinds(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subdirector")
	assert.Contains(t, err.Error(), "nested")
}

// TestDiscoverKinds_RejectsBadBasename pins the
// `[a-z][a-z0-9-]*` basename rule. Uppercase, leading digit, and
// underscore each fail.
func TestDiscoverKinds_RejectsBadBasename(t *testing.T) {
	cases := []string{"Foo.yaml", "1bad.yaml", "bad_name.yaml", "BAD.yaml"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(dir, ".mdsmith", "kinds"), 0o755))
			require.NoError(t, os.WriteFile(
				filepath.Join(dir, ".mdsmith", "kinds", name),
				[]byte("rules: {}\n"), 0o644))

			_, err := discoverKinds(dir)
			require.Error(t, err)
			assert.Contains(t, err.Error(), name)
		})
	}
}

// TestDiscoverKinds_RejectsUnknownKey pins the strict-decoding
// rule — a top-level key outside KindBody errors with the file
// and key name so the user can fix the typo.
func TestDiscoverKinds_RejectsUnknownKey(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".mdsmith", "kinds"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "kinds", "foo.yaml"),
		[]byte("not-a-real-key: bar\nrules: {}\n"), 0o644))

	_, err := discoverKinds(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foo.yaml")
	assert.Contains(t, err.Error(), "not-a-real-key")
}

// TestDiscoverKinds_IgnoresNonYAMLFiles pins the
// non-`.yaml`/`.yml` skip branch: a stray `.txt` or `.md`
// alongside the kind files must be silently ignored rather
// than treated as a malformed kind.
func TestDiscoverKinds_IgnoresNonYAMLFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".mdsmith", "kinds"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "kinds", "foo.yaml"),
		[]byte("rules: {}\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "kinds", "README.md"),
		[]byte("Notes\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "kinds", ".keep"),
		[]byte(""), 0o644))

	got, err := discoverKinds(dir)
	require.NoError(t, err)
	require.Contains(t, got, "foo")
	assert.Len(t, got, 1, "non-YAML files must not produce extra kinds")
}

// TestLoad_PropagatesKindFileDiscoveryError pins the
// error-propagation path: when discoverKinds returns an
// error, Load wraps it and aborts rather than continuing with
// a partial kinds map.
func TestLoad_PropagatesKindFileDiscoveryError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".mdsmith", "kinds"), 0o755))
	// A bad basename triggers a discoverKinds error.
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "kinds", "BadName.yaml"),
		[]byte("rules: {}\n"), 0o644))
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("rules: {}\n"), 0o644))

	_, err := Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading kind files")
	assert.Contains(t, err.Error(), "BadName.yaml")
}

// TestParseKindFile_PropagatesReadError pins the
// os.ReadFile error branch: a file that cannot be read
// surfaces a "reading <path>" error rather than panicking.
// Skipped on platforms or test users where chmod cannot
// produce an unreadable file (e.g. running as root).
func TestParseKindFile_PropagatesReadError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unreadable.yaml")
	require.NoError(t, os.WriteFile(path, []byte("rules: {}\n"), 0o644))
	require.NoError(t, os.Chmod(path, 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	if _, err := os.ReadFile(path); err == nil {
		t.Skip("test user can read mode-0000 files (likely running as root)")
	}

	_, err := parseKindFile(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading")
	assert.Contains(t, err.Error(), "unreadable.yaml")
}

// TestDiscoverKinds_RejectsYAMLAnchors pins that the
// anchor/alias guard fires on kind files (defence against
// billion-laughs payloads, parallel to UnmarshalSafe on
// `.mdsmith.yml`).
func TestDiscoverKinds_RejectsYAMLAnchors(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".mdsmith", "kinds"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "kinds", "foo.yaml"),
		[]byte("rules: &anchor {}\ncategories: *anchor\n"), 0o644))

	_, err := discoverKinds(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foo.yaml")
	assert.Contains(t, err.Error(), "anchors/aliases")
}

// TestDiscoverKinds_RejectsKindFileWithBadYAML pins the
// decode-error path: a `.mdsmith/kinds/<name>.yaml` whose body
// is not valid YAML surfaces the parse error with the file
// name so the user can jump straight to it.
func TestDiscoverKinds_RejectsKindFileWithBadYAML(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".mdsmith", "kinds"), 0o755)) // not the file
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "kinds", "foo.yaml"),
		[]byte("rules: [this is: not valid yaml\n"), 0o644))

	_, err := discoverKinds(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foo.yaml")
}

// TestDiscoverKinds_RejectsKindsPathIsFile pins the
// non-IsNotExist ReadDir branch. When the workspace contains a
// regular file at `.mdsmith/kinds`, the ReadDir call errors with
// ENOTDIR — the discoverer must propagate that error rather than
// silently returning an empty map (which would mask a misconfigured
// workspace as "no kinds").
func TestDiscoverKinds_RejectsKindsPathIsFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".mdsmith"), 0o755))
	// Plant a regular file where the kinds directory would live.
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "kinds"),
		[]byte("not a directory\n"), 0o644))

	_, err := discoverKinds(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".mdsmith/kinds")
}

// TestLoad_KindFileMergesIntoConfig verifies the end-to-end load
// path: a file kind merges into cfg.Kinds, indexable by basename.
func TestLoad_KindFileMergesIntoConfig(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".mdsmith", "kinds"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "kinds", "audit-log.yaml"),
		[]byte("rules:\n  max-file-length:\n    max: 700\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"),
		[]byte("rules: {}\n"), 0o644))

	cfg, err := Load(filepath.Join(dir, ".mdsmith.yml"))
	require.NoError(t, err)
	require.Contains(t, cfg.Kinds, "audit-log")
	assert.Equal(t, 700,
		cfg.Kinds["audit-log"].Rules["max-file-length"].Settings["max"])
	assert.Equal(t,
		filepath.Join(dir, ".mdsmith", "kinds", "audit-log.yaml"),
		cfg.Kinds["audit-log"].SourcePath)
}

// TestLoad_InlineKindCarriesConfigPath ensures inline kinds also
// get a SourcePath — the `.mdsmith.yml` path — so provenance can
// attribute either source uniformly.
func TestLoad_InlineKindCarriesConfigPath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
kinds:
  plan:
    rules:
      line-length:
        max: 200
`), 0o644))

	cfg, err := Load(cfgPath)
	require.NoError(t, err)
	require.Contains(t, cfg.Kinds, "plan")
	assert.Equal(t, cfgPath, cfg.Kinds["plan"].SourcePath)
}

// TestLoad_KindFileInlineCollision pins the dual-source error.
// The same kind name in both a file and inline must error naming
// both sources so the user can resolve the ambiguity.
func TestLoad_KindFileInlineCollision(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".mdsmith", "kinds"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "kinds", "plan.yaml"),
		[]byte("rules:\n  line-length:\n    max: 200\n"), 0o644))
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
kinds:
  plan:
    rules:
      line-length:
        max: 100
`), 0o644))

	_, err := Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plan")
	assert.Contains(t, err.Error(), ".mdsmith.yml")
	assert.Contains(t, err.Error(), "plan.yaml")
}

// TestLoad_KindFileRejectsDualSchemaSources is the file-defined
// parallel of TestKindRejectsDualSchemaSources: inline `schema:`
// plus `rules.required-structure.schema:` (file path) errors at
// load time (acceptance criterion #6, source A + B).
func TestLoad_KindFileRejectsDualSchemaSources(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".mdsmith", "kinds"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "kinds", "rfc.yaml"),
		[]byte(`schema:
  sections:
    - heading: "Overview"
rules:
  required-structure:
    schema: schemas/rfc.md
`), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"), []byte("rules: {}\n"), 0o644))

	_, err := Load(filepath.Join(dir, ".mdsmith.yml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rfc")
	assert.Contains(t, err.Error(), "schemas/rfc.md")
}

// TestLoad_KindFileRejectsInlineMapInRules is the file-defined
// parallel of TestKindRejectsInlineMapInRules: inline `schema:`
// plus `rules.required-structure.inline-schema:` errors at load
// time (acceptance criterion #6, source A + C).
func TestLoad_KindFileRejectsInlineMapInRules(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".mdsmith", "kinds"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "kinds", "rfc.yaml"),
		[]byte(`schema:
  sections:
    - heading: "Overview"
rules:
  required-structure:
    inline-schema:
      sections:
        - heading: "Other"
`), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"), []byte("rules: {}\n"), 0o644))

	_, err := Load(filepath.Join(dir, ".mdsmith.yml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rfc")
	assert.Contains(t, err.Error(), "inline-schema")
}

// TestLoad_KindFileRejectsBothSchemaAndInlineUnderRules is the
// file-defined parallel of
// TestKindRejectsBothSchemaAndInlineUnderRules: under a kind's
// rules.required-structure, setting both `schema:` (file) and
// `inline-schema:` (map) errors at load time (acceptance
// criterion #6, source B + C).
func TestLoad_KindFileRejectsBothSchemaAndInlineUnderRules(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".mdsmith", "kinds"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "kinds", "rfc.yaml"),
		[]byte(`rules:
  required-structure:
    schema: schemas/rfc.md
    inline-schema:
      sections:
        - heading: "Overview"
`), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"), []byte("rules: {}\n"), 0o644))

	_, err := Load(filepath.Join(dir, ".mdsmith.yml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rfc")
	assert.Contains(t, err.Error(), "schema:")
	assert.Contains(t, err.Error(), "inline-schema:")
}

// TestLoad_KindFileExtendsInlineKind confirms a file kind may
// extend an inline kind (acceptance criterion #7), including
// cross-source cycle detection.
func TestLoad_KindFileExtendsInlineKind(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".mdsmith", "kinds"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "kinds", "rfc-ratified.yaml"),
		[]byte(`extends: rfc-base
schema:
  frontmatter:
    status: '"ratified"'
`), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"), []byte(`
kinds:
  rfc-base:
    schema:
      frontmatter:
        id: '=~"^RFC-[0-9]{4}$"'
`), 0o644))

	cfg, err := Load(filepath.Join(dir, ".mdsmith.yml"))
	require.NoError(t, err)
	require.Contains(t, cfg.Kinds, "rfc-ratified")
	require.Contains(t, cfg.Kinds, "rfc-base")
	assert.Equal(t, "rfc-base", cfg.Kinds["rfc-ratified"].Extends)
}

// TestProvenance_KindSourcePath verifies that the kind layer's
// SourcePath rides through buildLayers into LayerEntry and that
// the resolved kind list reports the same path. Audit consumers
// (`kinds resolve`, JSON output) read these fields to print the
// defining-source path next to each kind (plan 208).
func TestProvenance_KindSourcePath(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".mdsmith", "kinds"), 0o755))
	kindPath := filepath.Join(dir, ".mdsmith", "kinds", "audit-log.yaml")
	require.NoError(t, os.WriteFile(kindPath,
		[]byte("rules:\n  max-file-length:\n    max: 700\n"), 0o644))
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
kind-assignment:
  - glob: ["docs/**/*.md"]
    kinds: [audit-log]
`), 0o644))

	loaded, err := Load(cfgPath)
	require.NoError(t, err)
	defaults := &Config{Rules: map[string]RuleCfg{}}
	cfg := Merge(defaults, loaded)

	res := ResolveFile(cfg, "docs/x.md", nil, nil)
	require.Len(t, res.Kinds, 1)
	assert.Equal(t, "audit-log", res.Kinds[0].Name)
	assert.Equal(t, kindPath, res.Kinds[0].SourcePath)

	rr, ok := res.Rules["max-file-length"]
	require.True(t, ok)
	// Find the kinds.audit-log layer entry and confirm its
	// SourcePath was threaded through buildRuleResolution.
	var found bool
	for _, l := range rr.Layers {
		if l.Source == "kinds.audit-log" {
			assert.True(t, l.Set)
			assert.Equal(t, kindPath, l.SourcePath)
			found = true
		}
	}
	assert.True(t, found, "kinds.audit-log layer must appear in the chain")
}

// TestProvenance_InlineKindSourcePath pins the same path
// surfacing for inline kinds — the `.mdsmith.yml` path tags
// every inline kind body so audit output is uniform across
// sources.
func TestProvenance_InlineKindSourcePath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
kinds:
  plan:
    rules:
      max-file-length:
        max: 500
kind-assignment:
  - glob: ["plan/*.md"]
    kinds: [plan]
`), 0o644))

	loaded, err := Load(cfgPath)
	require.NoError(t, err)
	defaults := &Config{Rules: map[string]RuleCfg{}}
	cfg := Merge(defaults, loaded)

	res := ResolveFile(cfg, "plan/foo.md", nil, nil)
	require.Len(t, res.Kinds, 1)
	assert.Equal(t, cfgPath, res.Kinds[0].SourcePath)

	rr := res.Rules["max-file-length"]
	var found bool
	for _, l := range rr.Layers {
		if l.Source == "kinds.plan" {
			assert.Equal(t, cfgPath, l.SourcePath)
			found = true
		}
	}
	assert.True(t, found)
}

// TestLoad_KindFileExtendsCycleAcrossSources catches a
// cross-source cycle (file -> inline -> file). The existing cycle
// detector must run on the merged map.
func TestLoad_KindFileExtendsCycleAcrossSources(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".mdsmith", "kinds"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "kinds", "a.yaml"),
		[]byte("extends: b\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"), []byte(`
kinds:
  b:
    extends: a
`), 0o644))

	_, err := Load(filepath.Join(dir, ".mdsmith.yml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}
