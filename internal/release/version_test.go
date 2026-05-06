package release

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureManifests writes the same set of manifests the release
// workflow expects to find under repo root. Each starts at the
// dev sentinel so the rewrite path is observable.
func fixtureManifests(t *testing.T, root string) {
	t.Helper()
	write := func(rel, body string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
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
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}

func TestStampRewritesEveryManifest(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)

	if err := Stamp(root, "1.2.3"); err != nil {
		t.Fatalf("Stamp: %v", err)
	}

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
	}
	for _, c := range cases {
		body := mustRead(t, filepath.Join(root, c.path))
		if !strings.Contains(body, c.want) {
			t.Errorf("%s: missing %q\n%s", c.path, c.want, body)
		}
		if strings.Contains(body, DevSentinel) {
			t.Errorf("%s: still contains %q after Stamp\n%s", c.path, DevSentinel, body)
		}
	}
}

func TestStampIsIdempotent(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)

	if err := Stamp(root, "9.9.9"); err != nil {
		t.Fatalf("first Stamp: %v", err)
	}
	paths := []string{
		"editors/vscode/package.json",
		"npm/mdsmith/package.json",
		"npm/platforms/linux-x64/package.json",
		"python/pyproject.toml",
	}
	first := make(map[string]string, len(paths))
	for _, p := range paths {
		first[p] = mustRead(t, filepath.Join(root, p))
	}

	if err := Stamp(root, "9.9.9"); err != nil {
		t.Fatalf("second Stamp: %v", err)
	}
	for _, p := range paths {
		got := mustRead(t, filepath.Join(root, p))
		if got != first[p] {
			t.Errorf("%s changed on second Stamp\n--- first ---\n%s\n--- second ---\n%s", p, first[p], got)
		}
	}
}

func TestStampRejectsLeadingV(t *testing.T) {
	err := Stamp(t.TempDir(), "v1.2.3")
	if err == nil {
		t.Fatal("expected leading-v rejection")
	}
	if !strings.Contains(err.Error(), "must not start with 'v'") {
		t.Errorf("error did not mention leading v: %v", err)
	}
}

func TestStampRejectsLeadingZeros(t *testing.T) {
	// Each of MAJOR/MINOR/PATCH and any purely-numeric prerelease
	// identifier must reject a leading zero. Build metadata IS
	// allowed leading zeros per spec.
	bad := []string{"01.2.3", "1.02.3", "1.2.03", "1.2.3-01", "1.2.3-rc.01"}
	for _, v := range bad {
		err := Stamp(t.TempDir(), v)
		if err == nil {
			t.Errorf("%s: expected semver rejection", v)
			continue
		}
		if !strings.Contains(err.Error(), "not valid semver") {
			t.Errorf("%s: error did not mention semver: %v", v, err)
		}
	}
}

func TestStampAcceptsValidSemverShapes(t *testing.T) {
	// `rc01` is alphanumeric so the leading 0 is fine; build
	// metadata identifiers (`+build.001`) are allowed leading
	// zeros outright.
	for _, v := range []string{"1.2.3", "1.2.3-rc01", "1.2.3-rc.1", "1.2.3+build.001", "1.2.3-rc.1+build.5"} {
		root := t.TempDir()
		fixtureManifests(t, root)
		if err := Stamp(root, v); err != nil {
			t.Errorf("%s: unexpected failure: %v", v, err)
		}
	}
}

func TestStampRejectsNonSemver(t *testing.T) {
	err := Stamp(t.TempDir(), "1.2")
	if err == nil {
		t.Fatal("expected non-semver rejection")
	}
	if !strings.Contains(err.Error(), "not valid semver") {
		t.Errorf("error did not mention semver: %v", err)
	}
}

func TestStampFailsWhenManifestHasNoVersionField(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	// Drop the version key so the regex no-ops; without the guard
	// the rewrite would silently leave 0.0.0-dev in place.
	if err := os.WriteFile(filepath.Join(root, "editors/vscode/package.json"),
		[]byte(`{"name": "mdsmith"}`), 0o644); err != nil {
		t.Fatalf("rewrite vscode manifest: %v", err)
	}

	err := Stamp(root, "1.2.3")
	if err == nil {
		t.Fatal("expected missing-version error")
	}
	if !strings.Contains(err.Error(), "no version field") {
		t.Errorf("error did not flag the missing field: %v", err)
	}
}

func TestStampFailsWhenOptionalDepsBlockMissing(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	// Drop the @mdsmith/* pins. The npm root must always advertise
	// its platform sub-packages, so a missing block is a hard
	// error rather than a silent no-op.
	if err := os.WriteFile(filepath.Join(root, "npm/mdsmith/package.json"), []byte(`{
  "name": "@mdsmith/cli",
  "version": "0.0.0-dev"
}
`), 0o644); err != nil {
		t.Fatalf("rewrite npm root manifest: %v", err)
	}

	err := Stamp(root, "1.2.3")
	if err == nil {
		t.Fatal("expected missing optional-deps error")
	}
	if !strings.Contains(err.Error(), "no @mdsmith/* optionalDependencies") {
		t.Errorf("error did not flag the missing optional-deps block: %v", err)
	}
}

func TestStampFailsWhenManifestMissing(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	if err := os.Remove(filepath.Join(root, "editors/vscode/package.json")); err != nil {
		t.Fatalf("remove vscode manifest: %v", err)
	}

	err := Stamp(root, "1.2.3")
	if err == nil {
		t.Fatal("expected missing-manifest error")
	}
	if !strings.Contains(err.Error(), "required manifest missing") {
		t.Errorf("error did not flag the missing file: %v", err)
	}
}

func TestCheckAcceptsDevSentinel(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	if err := Check(root); err != nil {
		t.Errorf("Check rejected the dev sentinel: %v", err)
	}
}

func TestCheckRejectsHandEdit(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	// Simulate a forgotten edit: vscode manifest still carries a
	// real version, every other manifest is at 0.0.0-dev.
	if err := os.WriteFile(filepath.Join(root, "editors/vscode/package.json"), []byte(`{
  "name": "mdsmith",
  "version": "0.1.2",
  "publisher": "jeduden"
}
`), 0o644); err != nil {
		t.Fatalf("rewrite vscode manifest: %v", err)
	}

	err := Check(root)
	if err == nil {
		t.Fatal("expected drift rejection")
	}
	if !strings.Contains(err.Error(), "editors/vscode/package.json") {
		t.Errorf("error did not name the offending file: %v", err)
	}
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
	if err := os.WriteFile(filepath.Join(root, "npm/mdsmith/package.json"), []byte(mismatched), 0o644); err != nil {
		t.Fatalf("rewrite npm root manifest: %v", err)
	}

	err := Check(root)
	if err == nil {
		t.Fatal("expected optional-dep drift rejection")
	}
	if !strings.Contains(err.Error(), `@mdsmith/linux-x64 pin "1.2.3"`) {
		t.Errorf("error did not name the drifted pin: %v", err)
	}
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
	if err := os.WriteFile(filepath.Join(root, "npm/mdsmith/package.json"), []byte(missing), 0o644); err != nil {
		t.Fatalf("rewrite npm root manifest: %v", err)
	}

	err := Check(root)
	if err == nil {
		t.Fatal("expected missing-key rejection")
	}
	if !strings.Contains(err.Error(), "@mdsmith/win32-x64") {
		t.Errorf("error did not flag the missing key: %v", err)
	}
}

func TestCheckFailsOnMissingManifest(t *testing.T) {
	root := t.TempDir()
	fixtureManifests(t, root)
	if err := os.Remove(filepath.Join(root, "editors/vscode/package.json")); err != nil {
		t.Fatalf("remove vscode manifest: %v", err)
	}

	err := Check(root)
	if err == nil {
		t.Fatal("expected missing-manifest rejection")
	}
	if !strings.Contains(err.Error(), "required manifest missing") {
		t.Errorf("error did not flag the missing file: %v", err)
	}
}
