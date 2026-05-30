package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/convention"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDiscoverConventions_EmptyWorkspaceReturnsEmpty pins the
// no-op branch: a workspace without `.mdsmith/conventions/`
// returns an empty map and no error so callers can blindly
// merge the result.
func TestDiscoverConventions_EmptyWorkspaceReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	got, err := discoverConventions(dir)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestDiscoverConventions_EmptyDirReturnsEmpty pins the case
// where `.mdsmith/conventions/` exists but holds no YAML files.
func TestDiscoverConventions_EmptyDirReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(dir, ".mdsmith", "conventions"), 0o755))
	got, err := discoverConventions(dir)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// TestDiscoverConventions_LoadsFullBody covers the happy path:
// a single convention file whose basename is the convention
// name, parsed into a full UserConvention — flavor + rules.
func TestDiscoverConventions_LoadsFullBody(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(dir, ".mdsmith", "conventions"), 0o755))
	body := `flavor: commonmark
rules:
  line-length:
    max: 72
  no-bare-urls: true
`
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "conventions", "portable-strict.yaml"),
		[]byte(body), 0o644))

	got, err := discoverConventions(dir)
	require.NoError(t, err)
	require.Contains(t, got, "portable-strict")
	dc := got["portable-strict"]
	assert.Equal(t, "commonmark", dc.body.Flavor)
	assert.Equal(t, 72, dc.body.Rules["line-length"].Settings["max"])
	assert.True(t, dc.body.Rules["no-bare-urls"].Enabled)
	wantPath := filepath.Join(dir, ".mdsmith", "conventions", "portable-strict.yaml")
	assert.Equal(t, wantPath, dc.sourcePath)
	assert.Equal(t, wantPath, dc.body.SourcePath,
		"body.SourcePath is the field that flows into cfg.Conventions")
}

// TestDiscoverConventions_AcceptsBothExtensions covers the
// `*.yaml` and `*.yml` glob — both are scanned and produce
// conventions keyed by basename.
func TestDiscoverConventions_AcceptsBothExtensions(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(dir, ".mdsmith", "conventions"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "conventions", "foo.yaml"),
		[]byte("flavor: commonmark\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "conventions", "bar.yml"),
		[]byte("flavor: gfm\n"), 0o644))

	got, err := discoverConventions(dir)
	require.NoError(t, err)
	assert.Contains(t, got, "foo")
	assert.Contains(t, got, "bar")
}

// TestDiscoverConventions_RejectsExtensionCollision pins the
// basename-collision check across `.yaml` and `.yml`. The error
// names both files so the user can pick which to keep.
func TestDiscoverConventions_RejectsExtensionCollision(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(dir, ".mdsmith", "conventions"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "conventions", "foo.yaml"),
		[]byte("flavor: commonmark\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "conventions", "foo.yml"),
		[]byte("flavor: gfm\n"), 0o644))

	_, err := discoverConventions(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foo")
	assert.Contains(t, err.Error(), "foo.yaml")
	assert.Contains(t, err.Error(), "foo.yml")
}

// TestDiscoverConventions_RejectsSubdirectory pins the
// no-subdirectories rule — a nested file is a config error
// rather than being silently ignored.
func TestDiscoverConventions_RejectsSubdirectory(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, ".mdsmith", "conventions", "nested")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(nested, "foo.yaml"), []byte("flavor: commonmark\n"), 0o644))

	_, err := discoverConventions(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subdirector")
	assert.Contains(t, err.Error(), "nested")
}

// TestDiscoverConventions_RejectsBadBasename pins the
// `[a-z][a-z0-9-]*` basename rule. Uppercase, leading digit,
// and underscore each fail.
func TestDiscoverConventions_RejectsBadBasename(t *testing.T) {
	cases := []string{"Foo.yaml", "1bad.yaml", "bad_name.yaml", "BAD.yaml"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.MkdirAll(
				filepath.Join(dir, ".mdsmith", "conventions"), 0o755))
			require.NoError(t, os.WriteFile(
				filepath.Join(dir, ".mdsmith", "conventions", name),
				[]byte("flavor: commonmark\n"), 0o644))

			_, err := discoverConventions(dir)
			require.Error(t, err)
			assert.Contains(t, err.Error(), name)
		})
	}
}

// TestDiscoverConventions_RejectsUnknownKey pins the strict-
// decoding rule — a top-level key outside UserConvention errors
// with the file and key name so the user can fix the typo.
func TestDiscoverConventions_RejectsUnknownKey(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(dir, ".mdsmith", "conventions"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "conventions", "foo.yaml"),
		[]byte("not-a-real-key: bar\nflavor: commonmark\n"), 0o644))

	_, err := discoverConventions(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foo.yaml")
	assert.Contains(t, err.Error(), "not-a-real-key")
}

// TestDiscoverConventions_IgnoresNonYAMLFiles pins the
// non-`.yaml`/`.yml` skip branch: a stray `.txt` or `.md`
// alongside the convention files must be silently ignored.
func TestDiscoverConventions_IgnoresNonYAMLFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(dir, ".mdsmith", "conventions"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "conventions", "foo.yaml"),
		[]byte("flavor: commonmark\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "conventions", "README.md"),
		[]byte("Notes\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "conventions", ".keep"),
		[]byte(""), 0o644))

	got, err := discoverConventions(dir)
	require.NoError(t, err)
	require.Contains(t, got, "foo")
	assert.Len(t, got, 1, "non-YAML files must not produce extra conventions")
}

// TestDiscoverConventions_RejectsYAMLAnchors pins that the
// anchor/alias guard fires on convention files (defence against
// billion-laughs payloads, parallel to UnmarshalSafe on
// `.mdsmith.yml`).
func TestDiscoverConventions_RejectsYAMLAnchors(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(dir, ".mdsmith", "conventions"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "conventions", "foo.yaml"),
		[]byte("rules: &anchor {}\ncategories: *anchor\n"), 0o644))

	_, err := discoverConventions(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foo.yaml")
	assert.Contains(t, err.Error(), "anchors/aliases")
}

// TestDiscoverConventions_RejectsBadYAML pins the decode-error
// path: a `.mdsmith/conventions/<name>.yaml` whose body is not
// valid YAML surfaces the parse error with the file name.
func TestDiscoverConventions_RejectsBadYAML(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(dir, ".mdsmith", "conventions"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "conventions", "foo.yaml"),
		[]byte("rules: [this is: not valid yaml\n"), 0o644))

	_, err := discoverConventions(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foo.yaml")
}

// TestDiscoverConventions_RejectsPathIsFile pins the
// non-IsNotExist ReadDir branch. When the workspace contains a
// regular file at `.mdsmith/conventions`, ReadDir errors with
// ENOTDIR — the discoverer must propagate that rather than
// returning an empty map (which would mask a misconfigured
// workspace as "no conventions").
func TestDiscoverConventions_RejectsPathIsFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".mdsmith"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "conventions"),
		[]byte("not a directory\n"), 0o644))

	_, err := discoverConventions(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".mdsmith/conventions")
}

// TestLoad_ConventionFileMergesIntoConfig verifies the
// end-to-end load path: a file convention merges into
// cfg.Conventions, keyed by basename, with its SourcePath set.
func TestLoad_ConventionFileMergesIntoConfig(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(dir, ".mdsmith", "conventions"), 0o755))
	convPath := filepath.Join(dir, ".mdsmith", "conventions", "portable-strict.yaml")
	require.NoError(t, os.WriteFile(convPath,
		[]byte("flavor: commonmark\nrules:\n  line-length:\n    max: 72\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"),
		[]byte("rules: {}\n"), 0o644))

	cfg, err := Load(filepath.Join(dir, ".mdsmith.yml"))
	require.NoError(t, err)
	require.Contains(t, cfg.Conventions, "portable-strict")
	assert.Equal(t, "commonmark", cfg.Conventions["portable-strict"].Flavor)
	assert.Equal(t, 72,
		cfg.Conventions["portable-strict"].Rules["line-length"].Settings["max"])
	assert.Equal(t, convPath, cfg.Conventions["portable-strict"].SourcePath)
}

// TestLoad_InlineConventionCarriesConfigPath ensures inline
// conventions also get a SourcePath — the `.mdsmith.yml` path —
// so provenance can attribute either source uniformly.
func TestLoad_InlineConventionCarriesConfigPath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
conventions:
  our-team:
    flavor: gfm
    rules:
      line-length:
        max: 100
`), 0o644))

	cfg, err := Load(cfgPath)
	require.NoError(t, err)
	require.Contains(t, cfg.Conventions, "our-team")
	assert.Equal(t, cfgPath, cfg.Conventions["our-team"].SourcePath)
}

// TestLoad_InlineAndFileConventionsCoexist verifies that a
// non-colliding inline convention and a file convention both
// survive the merge, each carrying its own SourcePath. The file
// uses a `.yml` extension so the end-to-end merge is also
// exercised through Load for that extension (the other Load-level
// tests use `.yaml`).
func TestLoad_InlineAndFileConventionsCoexist(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(dir, ".mdsmith", "conventions"), 0o755))
	filePath := filepath.Join(dir, ".mdsmith", "conventions", "team-b.yml")
	require.NoError(t, os.WriteFile(filePath, []byte("flavor: gfm\n"), 0o644))
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
conventions:
  team-a:
    flavor: commonmark
`), 0o644))

	cfg, err := Load(cfgPath)
	require.NoError(t, err)
	require.Contains(t, cfg.Conventions, "team-a")
	require.Contains(t, cfg.Conventions, "team-b")
	assert.Equal(t, "commonmark", cfg.Conventions["team-a"].Flavor)
	assert.Equal(t, "gfm", cfg.Conventions["team-b"].Flavor)
	assert.Equal(t, cfgPath, cfg.Conventions["team-a"].SourcePath)
	assert.Equal(t, filePath, cfg.Conventions["team-b"].SourcePath)
}

// TestLoad_ConventionFileInlineCollision pins the dual-source
// error. The same convention name in both a file and inline must
// error naming both sources so the user can resolve the
// ambiguity (acceptance criterion #2).
func TestLoad_ConventionFileInlineCollision(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(dir, ".mdsmith", "conventions"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "conventions", "our-team.yaml"),
		[]byte("flavor: commonmark\n"), 0o644))
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
conventions:
  our-team:
    flavor: gfm
`), 0o644))

	_, err := Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "our-team")
	assert.Contains(t, err.Error(), ".mdsmith.yml")
	assert.Contains(t, err.Error(), "our-team.yaml")
}

// TestLoad_ConventionFileBuiltinCollision pins the built-in
// collision error. A file convention named like a built-in
// (portable, github, plain, …) must error naming the file
// (acceptance criterion #4).
func TestLoad_ConventionFileBuiltinCollision(t *testing.T) {
	// Iterate the real built-in set rather than a hardcoded subset
	// so a drift in convention.Names() — e.g. a newly added built-in
	// like `obsidian` or `parity` — is covered automatically.
	for _, builtin := range convention.Names() {
		t.Run(builtin, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.MkdirAll(
				filepath.Join(dir, ".mdsmith", "conventions"), 0o755))
			require.NoError(t, os.WriteFile(
				filepath.Join(dir, ".mdsmith", "conventions", builtin+".yaml"),
				[]byte("flavor: gfm\n"), 0o644))
			require.NoError(t, os.WriteFile(
				filepath.Join(dir, ".mdsmith.yml"), []byte("rules: {}\n"), 0o644))

			_, err := Load(filepath.Join(dir, ".mdsmith.yml"))
			require.Error(t, err)
			assert.Contains(t, err.Error(), builtin)
			assert.Contains(t, err.Error(), "reserved")
			assert.Contains(t, err.Error(), builtin+".yaml")
		})
	}
}

// TestLoad_PropagatesConventionFileDiscoveryError pins the
// error-propagation path: when discoverConventions returns an
// error, Load wraps it and aborts rather than continuing with a
// partial conventions map.
func TestLoad_PropagatesConventionFileDiscoveryError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(dir, ".mdsmith", "conventions"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "conventions", "BadName.yaml"),
		[]byte("flavor: commonmark\n"), 0o644))
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("rules: {}\n"), 0o644))

	_, err := Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading convention files")
	assert.Contains(t, err.Error(), "BadName.yaml")
}

// TestLoad_ConventionFileSelectable confirms a file convention
// can be selected via the top-level `convention:` key and that
// its preset lands in ConventionPreset.
func TestLoad_ConventionFileSelectable(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(dir, ".mdsmith", "conventions"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith", "conventions", "our-team.yaml"),
		[]byte("flavor: gfm\nrules:\n  markdown-flavor:\n    flavor: gfm\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"),
		[]byte("convention: our-team\n"), 0o644))

	cfg, err := Load(filepath.Join(dir, ".mdsmith.yml"))
	require.NoError(t, err)
	require.NotNil(t, cfg.ConventionPreset)
	mf, ok := cfg.ConventionPreset["markdown-flavor"]
	require.True(t, ok)
	assert.Equal(t, "gfm", mf.Settings["flavor"])
}
