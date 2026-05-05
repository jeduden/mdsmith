// Package release exercises the helper scripts that the release
// workflow runs before publishing each channel. The tests live here
// so a regression in scripts/set-version.sh or scripts/check-versions.sh
// fails `go test ./...` rather than only the release job itself.
package release

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const devSentinel = "0.0.0-dev"

// fixtureManifests writes the same set of manifests the release
// workflow expects to find. Each file starts at the dev sentinel so
// the rewrite path is observable.
func fixtureManifests(t *testing.T, root string) {
	t.Helper()
	mustWrite := func(rel, body string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}

	mustWrite("editors/vscode/package.json", `{
  "name": "mdsmith",
  "version": "0.0.0-dev",
  "publisher": "jeduden"
}
`)

	mustWrite("npm/mdsmith/package.json", `{
  "name": "mdsmith",
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
		mustWrite(filepath.Join("npm/platforms", plat, "package.json"), `{
  "name": "@mdsmith/`+plat+`",
  "version": "0.0.0-dev"
}
`)
	}

	mustWrite("python/pyproject.toml", `[project]
name = "mdsmith"
version = "0.0.0-dev"
`)
}

func runScript(t *testing.T, script string, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{script}, args...)...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func projectRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// internal/release/ -> repo root
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func TestSetVersionRewritesEveryManifest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("set-version.sh requires a POSIX shell")
	}
	repo := projectRoot(t)
	script := filepath.Join(repo, "scripts", "set-version.sh")
	root := t.TempDir()
	fixtureManifests(t, root)

	if _, stderr, err := runScript(t, script, "1.2.3", "--root", root); err != nil {
		t.Fatalf("set-version.sh failed: %v\nstderr: %s", err, stderr)
	}

	checks := []struct {
		path, want string
	}{
		{"editors/vscode/package.json", `"version": "1.2.3"`},
		{"npm/mdsmith/package.json", `"version": "1.2.3"`},
		{"npm/mdsmith/package.json", `"@mdsmith/linux-x64": "1.2.3"`},
		{"npm/mdsmith/package.json", `"@mdsmith/win32-x64": "1.2.3"`},
		{"npm/platforms/linux-x64/package.json", `"version": "1.2.3"`},
		{"npm/platforms/darwin-arm64/package.json", `"version": "1.2.3"`},
		{"python/pyproject.toml", `version = "1.2.3"`},
	}
	for _, c := range checks {
		body := readFile(t, filepath.Join(root, c.path))
		if !strings.Contains(body, c.want) {
			t.Errorf("%s: missing %q\n%s", c.path, c.want, body)
		}
		if strings.Contains(body, devSentinel) {
			t.Errorf("%s: still contains %q after rewrite\n%s", c.path, devSentinel, body)
		}
	}
}

func TestSetVersionIsIdempotent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("set-version.sh requires a POSIX shell")
	}
	repo := projectRoot(t)
	script := filepath.Join(repo, "scripts", "set-version.sh")
	root := t.TempDir()
	fixtureManifests(t, root)

	if _, stderr, err := runScript(t, script, "9.9.9", "--root", root); err != nil {
		t.Fatalf("first run failed: %v\nstderr: %s", err, stderr)
	}

	manifests := []string{
		"editors/vscode/package.json",
		"npm/mdsmith/package.json",
		"npm/platforms/linux-x64/package.json",
		"npm/platforms/win32-x64/package.json",
		"python/pyproject.toml",
	}
	first := make(map[string]string, len(manifests))
	for _, m := range manifests {
		first[m] = readFile(t, filepath.Join(root, m))
	}

	if _, stderr, err := runScript(t, script, "9.9.9", "--root", root); err != nil {
		t.Fatalf("second run failed: %v\nstderr: %s", err, stderr)
	}

	for _, m := range manifests {
		got := readFile(t, filepath.Join(root, m))
		if got != first[m] {
			t.Errorf("%s changed on second run\nfirst:\n%s\nsecond:\n%s", m, first[m], got)
		}
	}
}

func TestSetVersionRejectsLeadingV(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("set-version.sh requires a POSIX shell")
	}
	repo := projectRoot(t)
	script := filepath.Join(repo, "scripts", "set-version.sh")
	root := t.TempDir()
	fixtureManifests(t, root)

	_, stderr, err := runScript(t, script, "v1.2.3", "--root", root)
	if err == nil {
		t.Fatal("expected failure for 'v'-prefixed version")
	}
	if !strings.Contains(stderr, "must not start with 'v'") {
		t.Errorf("stderr did not explain the leading-v rejection:\n%s", stderr)
	}
}

func TestSetVersionRejectsNonSemver(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("set-version.sh requires a POSIX shell")
	}
	repo := projectRoot(t)
	script := filepath.Join(repo, "scripts", "set-version.sh")
	root := t.TempDir()
	fixtureManifests(t, root)

	_, stderr, err := runScript(t, script, "1.2", "--root", root)
	if err == nil {
		t.Fatal("expected failure for non-semver version")
	}
	if !strings.Contains(stderr, "not valid semver") {
		t.Errorf("stderr did not explain the semver rejection:\n%s", stderr)
	}
}

func TestCheckVersionsAcceptsDevSentinel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("check-versions.sh requires a POSIX shell")
	}
	repo := projectRoot(t)
	script := filepath.Join(repo, "scripts", "check-versions.sh")
	root := t.TempDir()
	fixtureManifests(t, root)

	if stdout, stderr, err := runScript(t, script, "--root", root); err != nil {
		t.Fatalf("check-versions.sh failed for dev sentinel: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
}

func TestCheckVersionsRejectsHandEdit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("check-versions.sh requires a POSIX shell")
	}
	repo := projectRoot(t)
	script := filepath.Join(repo, "scripts", "check-versions.sh")
	root := t.TempDir()
	fixtureManifests(t, root)

	// Simulate a forgotten edit: the vscode manifest still carries the
	// old hand-rolled version, every other manifest is at the dev
	// sentinel. The guard must fail and name the offending file.
	if err := os.WriteFile(filepath.Join(root, "editors/vscode/package.json"), []byte(`{
  "name": "mdsmith",
  "version": "0.1.2",
  "publisher": "jeduden"
}
`), 0o644); err != nil {
		t.Fatalf("rewrite vscode manifest: %v", err)
	}

	_, stderr, err := runScript(t, script, "--root", root)
	if err == nil {
		t.Fatal("expected failure when a manifest deviates from the dev sentinel")
	}
	if !strings.Contains(stderr, "editors/vscode/package.json") {
		t.Errorf("stderr did not name the offending file:\n%s", stderr)
	}
}

func TestCheckVersionsFailsOnMissingManifest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("check-versions.sh requires a POSIX shell")
	}
	repo := projectRoot(t)
	script := filepath.Join(repo, "scripts", "check-versions.sh")
	root := t.TempDir()
	fixtureManifests(t, root)

	// Simulate a renamed/deleted manifest. The version-guard's whole
	// point is to keep these in sync, so a vanished file must fail
	// the check rather than silently pass.
	if err := os.Remove(filepath.Join(root, "editors/vscode/package.json")); err != nil {
		t.Fatalf("remove vscode manifest: %v", err)
	}

	_, stderr, err := runScript(t, script, "--root", root)
	if err == nil {
		t.Fatal("expected failure when a tracked manifest is missing")
	}
	if !strings.Contains(stderr, "editors/vscode/package.json") {
		t.Errorf("stderr did not name the missing file:\n%s", stderr)
	}
	if !strings.Contains(stderr, "required manifest missing") {
		t.Errorf("stderr did not flag the missing manifest:\n%s", stderr)
	}
}

func TestCheckVersionsRejectsOptionalDepDrift(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("check-versions.sh requires a POSIX shell")
	}
	repo := projectRoot(t)
	script := filepath.Join(repo, "scripts", "check-versions.sh")
	root := t.TempDir()
	fixtureManifests(t, root)

	// Simulate a half-finished version bump: the root advertises a
	// real version for one platform pin but every other manifest is
	// still on the dev sentinel.
	mismatched := `{
  "name": "mdsmith",
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

	_, stderr, err := runScript(t, script, "--root", root)
	if err == nil {
		t.Fatal("expected failure when an optional-dep pin drifts from the dev sentinel")
	}
	if !strings.Contains(stderr, "optionalDependencies pin '1.2.3'") {
		t.Errorf("stderr did not flag the drifted optional-dep pin:\n%s", stderr)
	}
}
