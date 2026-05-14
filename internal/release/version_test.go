package release

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureManifests writes the same set of manifests the release
// workflow expects to find under repo root. Each starts at the
// dev sentinel so the rewrite path is observable.
func fixtureManifests(t *testing.T, root string) {
	t.Helper()
	write := func(rel, body string) {
		full := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
	}

	write("editors/vscode/package.json", `{
  "name": "mdsmith",
  "version": "0.0.0-dev",
  "publisher": "jeduden"
}
`)
	write("npm/mdsmith/package.json", `{
  "name": "@mdsmith/cli",
  "version": "0.0.0-dev",
  "bin": { "mdsmith": "./bin/mdsmith.js" },
  "optionalDependencies": {
    "@mdsmith/linux-x64": "0.0.0-dev",
    "@mdsmith/linux-arm64": "0.0.0-dev",
    "@mdsmith/darwin-x64": "0.0.0-dev",
    "@mdsmith/darwin-arm64": "0.0.0-dev",
    "@mdsmith/win32-x64": "0.0.0-dev"
  }
}
`)
	for _, plat := range []string{"linux-x64", "linux-arm64", "darwin-x64", "darwin-arm64", "win32-x64"} {
		write(filepath.Join("npm/platforms", plat, "package.json"), fmt.Sprintf(`{
  "name": "@mdsmith/%s",
  "version": "0.0.0-dev"
}
`, plat))
	}
	write("python/pyproject.toml", `[project]
name = "mdsmith"
version = "0.0.0-dev"
`)
	write("website/hugo.toml", `baseURL = "https://mdsmith.dev/"
title = "mdsmith"
[params]
  version = "0.0.0-dev"
`)
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err, "read %s", path)
	return string(body)
}

func TestStampRewritesEveryManifest(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	require.NoError(t, Stamp(root, "1.2.3"))

	cases := []struct {
		path string
		want string
	}{
		{"editors/vscode/package.json", `"version": "1.2.3"`},
		{"npm/mdsmith/package.json", `"version": "1.2.3"`},
		{"npm/mdsmith/package.json", `"@mdsmith/linux-x64": "1.2.3"`},
		{"npm/mdsmith/package.json", `"@mdsmith/win32-x64": "1.2.3"`},
		{"npm/platforms/linux-x64/package.json", `"version": "1.2.3"`},
		{"npm/platforms/darwin-arm64/package.json", `"version": "1.2.3"`},
		{"python/pyproject.toml", `version = "1.2.3"`},
		{"website/hugo.toml", `version = "1.2.3"`},
	}
	for _, c := range cases {
		body := mustRead(t, filepath.Join(root, c.path))
		assert.Contains(t, body, c.want, "missing rewrite in %s", c.path)
		assert.NotContains(t, body, DevSentinel, "%s still carries dev sentinel", c.path)
	}
}

func TestStampIsIdempotent(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	require.NoError(t, Stamp(root, "9.9.9"))

	paths := []string{
		"editors/vscode/package.json",
		"npm/mdsmith/package.json",
		"npm/platforms/linux-x64/package.json",
		"python/pyproject.toml",
		"website/hugo.toml",
	}
	first := make(map[string]string, len(paths))
	for _, p := range paths {
		first[p] = mustRead(t, filepath.Join(root, p))
	}
	require.NoError(t, Stamp(root, "9.9.9"))
	for _, p := range paths {
		assert.Equal(t, first[p], mustRead(t, filepath.Join(root, p)), "%s changed on second Stamp", p)
	}
}

func TestStampRejectsLeadingV(t *testing.T) {
	err := Stamp(t.TempDir(), "v1.2.3")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not start with 'v'")
}

func TestStampRejectsLeadingZeros(t *testing.T) {
	// Each of MAJOR/MINOR/PATCH and any purely-numeric prerelease
	// identifier must reject a leading zero. Build metadata IS
	// allowed leading zeros per spec.
	for _, v := range []string{"01.2.3", "1.02.3", "1.2.03", "1.2.3-01", "1.2.3-rc.01"} {
		err := Stamp(t.TempDir(), v)
		require.Error(t, err, v)
		assert.Contains(t, err.Error(), "not valid semver", v)
	}
}

func TestStampAcceptsValidSemverShapes(t *testing.T) {
	// `rc01` is alphanumeric so the leading 0 is fine; build
	// metadata identifiers (`+build.001`) are allowed leading
	// zeros outright.
	for _, v := range []string{"1.2.3", "1.2.3-rc01", "1.2.3-rc.1", "1.2.3+build.001", "1.2.3-rc.1+build.5"} {
		root := t.TempDir()
		fixtureManifests(t, root)
		assert.NoError(t, Stamp(root, v), v)
	}
}

func TestStampRejectsNonSemver(t *testing.T) {
	err := Stamp(t.TempDir(), "1.2")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid semver")
}

func TestValidateSemverRejectsEmpty(t *testing.T) {
	err := ValidateSemver("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-empty")
}

func TestStampFailsWhenManifestHasNoVersionField(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	// Drop the version key so the regex no-ops; without the guard
	// the rewrite would silently leave 0.0.0-dev in place.
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "editors/vscode/package.json"),
		[]byte(`{"name": "mdsmith"}`), 0o644))

	err := Stamp(root, "1.2.3")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no version field")
}

func TestStampFailsWhenOptionalDepsBlockMissing(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	// Drop the @mdsmith/* pins. The npm root must always advertise
	// its platform sub-packages, so a missing block is a hard
	// error rather than a silent no-op.
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "npm/mdsmith/package.json"), []byte(`{
  "name": "@mdsmith/cli",
  "version": "0.0.0-dev"
}
`), 0o644))

	err := Stamp(root, "1.2.3")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no @mdsmith/* optionalDependencies")
}

func TestStampFailsWhenManifestMissing(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	require.NoError(t, os.Remove(filepath.Join(root, "editors/vscode/package.json")))

	err := Stamp(root, "1.2.3")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required manifest missing")
}

func TestCheckAcceptsDevSentinel(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	assert.NoError(t, Check(root))
}

func TestCheckRejectsHandEdit(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	// Simulate a forgotten edit: vscode manifest still carries a
	// real version, every other manifest is at 0.0.0-dev.
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "editors/vscode/package.json"), []byte(`{
  "name": "mdsmith",
  "version": "0.1.2",
  "publisher": "jeduden"
}
`), 0o644))

	err := Check(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "editors/vscode/package.json")
}

func TestCheckRejectsOptionalDepDrift(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	mismatched := `{
  "name": "@mdsmith/cli",
  "version": "0.0.0-dev",
  "optionalDependencies": {
    "@mdsmith/linux-x64": "1.2.3",
    "@mdsmith/linux-arm64": "0.0.0-dev",
    "@mdsmith/darwin-x64": "0.0.0-dev",
    "@mdsmith/darwin-arm64": "0.0.0-dev",
    "@mdsmith/win32-x64": "0.0.0-dev"
  }
}
`
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "npm/mdsmith/package.json"), []byte(mismatched), 0o644))

	err := Check(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `@mdsmith/linux-x64 pin "1.2.3"`)
}

func TestCheckRejectsMissingOptionalDepKey(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	// Drop one platform pin entirely. Without the per-key check
	// the dev-sentinel scan would still pass on the remaining
	// pins and the missing key would only surface at publish.
	missing := `{
  "name": "@mdsmith/cli",
  "version": "0.0.0-dev",
  "optionalDependencies": {
    "@mdsmith/linux-x64": "0.0.0-dev",
    "@mdsmith/linux-arm64": "0.0.0-dev",
    "@mdsmith/darwin-x64": "0.0.0-dev",
    "@mdsmith/darwin-arm64": "0.0.0-dev"
  }
}
`
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "npm/mdsmith/package.json"), []byte(missing), 0o644))

	err := Check(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "@mdsmith/win32-x64")
}

func TestCheckFailsOnMissingManifest(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	require.NoError(t, os.Remove(filepath.Join(root, "editors/vscode/package.json")))

	err := Check(root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required manifest missing")
}

// TestTrackedManifestsListsPlatformSubpackages mirrors the
// release workflow's expectation that platform sub-packages
// under npm/platforms/ are picked up by both Stamp and Check.
// The fixture creates the platform tree, so the helper must
// return all of them — exercising the readdir branch.
func TestTrackedManifestsListsPlatformSubpackages(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	got := TrackedManifests(root)

	want := []string{
		filepath.Join(root, "editors", "vscode", "package.json"),
		filepath.Join(root, "npm", "mdsmith", "package.json"),
		filepath.Join(root, "npm", "platforms", "darwin-arm64", "package.json"),
		filepath.Join(root, "npm", "platforms", "darwin-x64", "package.json"),
		filepath.Join(root, "npm", "platforms", "linux-arm64", "package.json"),
		filepath.Join(root, "npm", "platforms", "linux-x64", "package.json"),
		filepath.Join(root, "npm", "platforms", "win32-x64", "package.json"),
		filepath.Join(root, "python", "pyproject.toml"),
		filepath.Join(root, "website", "hugo.toml"),
	}
	gotPaths := make([]string, len(got))
	for i, m := range got {
		gotPaths[i] = m.Path
	}
	assert.ElementsMatch(t, want, gotPaths)
}
