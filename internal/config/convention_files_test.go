package config

import (
	"os"
	"path/filepath"
	"testing"

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
	assert.Equal(t,
		filepath.Join(dir, ".mdsmith", "conventions", "portable-strict.yaml"),
		dc.sourcePath)
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
