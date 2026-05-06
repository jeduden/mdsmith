// Package release provides the helper logic the release workflow
// invokes via cmd/mdsmith-release/. The package was originally a
// set of bash scripts under scripts/, but JSON/TOML editing in
// perl regex and indirect testing through `bash <script>` made
// them brittle. The Go port runs under one toolchain, has direct
// tests, and reports actionable errors without shelling out.
package release

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DevSentinel is the version every tracked manifest carries
// between releases. Stamp rewrites it to the cleaned tag during
// release; Check fails the version-guard CI job when a hand-edit
// pushes anything else to main.
const DevSentinel = "0.0.0-dev"

// PlatformPackages enumerates the @mdsmith/<platform> optional-
// dependency keys the npm root must list. Stays in lock-step with
// the build matrix in .github/workflows/release.yml.
var PlatformPackages = []string{
	"@mdsmith/linux-x64",
	"@mdsmith/linux-arm64",
	"@mdsmith/darwin-x64",
	"@mdsmith/darwin-arm64",
	"@mdsmith/win32-x64",
}

// ManifestKind tells the rewrite/check helpers which syntax to
// expect: JSON for npm package manifests, TOML for pyproject.
type ManifestKind int

const (
	// ManifestJSON marks JSON manifests (npm package.json files).
	ManifestJSON ManifestKind = iota
	// ManifestTOML marks TOML manifests (python pyproject.toml).
	ManifestTOML
)

// Manifest is one tracked file. OptionalDeps marks the npm root
// — only that file carries @mdsmith/* pins.
type Manifest struct {
	Path         string
	Kind         ManifestKind
	OptionalDeps bool
}

// TrackedManifests returns the set of manifests Stamp rewrites
// and Check verifies. Platform sub-package manifests under
// npm/platforms/ only exist after BuildNpmPlatforms runs; they
// are stamped if present.
func TrackedManifests(root string) []Manifest {
	out := []Manifest{
		{filepath.Join(root, "editors", "vscode", "package.json"), ManifestJSON, false},
		{filepath.Join(root, "npm", "mdsmith", "package.json"), ManifestJSON, true},
	}
	platformsDir := filepath.Join(root, "npm", "platforms")
	if entries, err := os.ReadDir(platformsDir); err == nil {
		for _, e := range entries {
			p := filepath.Join(platformsDir, e.Name(), "package.json")
			if _, err := os.Stat(p); err == nil {
				out = append(out, Manifest{p, ManifestJSON, false})
			}
		}
	}
	out = append(out, Manifest{filepath.Join(root, "python", "pyproject.toml"), ManifestTOML, false})
	return out
}

// semverRE matches the full semver.org grammar:
//   - MAJOR/MINOR/PATCH each "0" or [1-9][0-9]*
//   - optional pre-release: dot-separated identifiers; numeric
//     identifiers also forbid leading zeros (so "-01" is rejected
//     but "-rc01" stays valid because it's alphanumeric)
//   - optional build metadata: dot-separated [0-9A-Za-z-]+
//     identifiers. Build metadata IS allowed leading zeros.
var (
	num      = `(?:0|[1-9][0-9]*)`
	prerelID = `(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)`
	buildID  = `[0-9A-Za-z-]+`
	semverRE = regexp.MustCompile(`^` + num + `\.` + num + `\.` + num +
		`(?:-` + prerelID + `(?:\.` + prerelID + `)*)?` +
		`(?:\+` + buildID + `(?:\.` + buildID + `)*)?$`)
	jsonVersionRE = regexp.MustCompile(`(?m)^([ \t]*"version"[ \t]*:[ \t]*")([^"]+)(")`)
	jsonOptDepRE  = regexp.MustCompile(`(?m)^([ \t]*"@mdsmith/[^"]+"[ \t]*:[ \t]*")([^"]+)(")`)
	tomlVersionRE = regexp.MustCompile(`(?m)^([ \t]*version[ \t]*=[ \t]*")([^"]+)(")`)
)

// ValidateSemver rejects empty strings, leading-v tags ("v1.2.3"),
// and any version that isn't conforming SemVer.
func ValidateSemver(v string) error {
	if v == "" {
		return errors.New("version must be non-empty")
	}
	if v[0] == 'v' {
		return fmt.Errorf("version must not start with 'v' (got %q)", v)
	}
	if !semverRE.MatchString(v) {
		return fmt.Errorf("version %q is not valid semver", v)
	}
	return nil
}

// Stamp rewrites every tracked manifest under root from the dev
// sentinel to version. Idempotent: running with the same version
// twice produces no further change. A required manifest that's
// missing a version field — or, for the npm root, missing the
// @mdsmith/* optionalDependencies block — is a hard error.
func Stamp(root, version string) error {
	if err := ValidateSemver(version); err != nil {
		return err
	}
	for _, m := range TrackedManifests(root) {
		if err := stampManifest(m, version); err != nil {
			return err
		}
	}
	return nil
}

func stampManifest(m Manifest, version string) error {
	body, err := os.ReadFile(m.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("required manifest missing: %s", m.Path)
		}
		return fmt.Errorf("read %s: %w", m.Path, err)
	}
	out, err := rewriteVersion(body, m.Kind, version)
	if err != nil {
		return fmt.Errorf("%s: %w", m.Path, err)
	}
	if m.OptionalDeps {
		if !jsonOptDepRE.Match(out) {
			return fmt.Errorf("%s: no @mdsmith/* optionalDependencies pins found", m.Path)
		}
		out = jsonOptDepRE.ReplaceAllFunc(out, func(match []byte) []byte {
			sub := jsonOptDepRE.FindSubmatchIndex(match)
			return concat(match[:sub[4]], []byte(version), match[sub[5]:])
		})
	}
	return os.WriteFile(m.Path, out, 0o644)
}

func rewriteVersion(body []byte, kind ManifestKind, version string) ([]byte, error) {
	re := versionRegexp(kind)
	idx := re.FindSubmatchIndex(body)
	if idx == nil {
		return nil, errors.New("no version field found")
	}
	// idx[4]:idx[5] spans the value (group 2). Replace just that.
	return concat(body[:idx[4]], []byte(version), body[idx[5]:]), nil
}

func versionRegexp(k ManifestKind) *regexp.Regexp {
	if k == ManifestTOML {
		return tomlVersionRE
	}
	return jsonVersionRE
}

// Check accumulates every problem with the tracked manifests
// (missing file, missing field, drifted version, missing or
// drifted @mdsmith/* pin) into one multi-line error. The
// version-guard CI job uses Check; reporting all problems at once
// is more useful than failing on the first.
func Check(root string) error {
	var problems []string
	note := func(msg string) { problems = append(problems, msg) }
	for _, m := range TrackedManifests(root) {
		checkManifest(m, note)
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "\n"))
	}
	return nil
}

func checkManifest(m Manifest, note func(string)) {
	body, err := os.ReadFile(m.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			note(fmt.Sprintf("%s: required manifest missing", m.Path))
			return
		}
		note(fmt.Sprintf("read %s: %v", m.Path, err))
		return
	}
	sub := versionRegexp(m.Kind).FindSubmatch(body)
	if sub == nil {
		note(fmt.Sprintf("%s: no version field found", m.Path))
		return
	}
	if string(sub[2]) != DevSentinel {
		note(fmt.Sprintf("%s: version is %q, want %q", m.Path, sub[2], DevSentinel))
	}
	if m.OptionalDeps {
		for _, key := range PlatformPackages {
			keyRE := regexp.MustCompile(`(?m)^[ \t]*"` + regexp.QuoteMeta(key) +
				`"[ \t]*:[ \t]*"([^"]+)"`)
			kSub := keyRE.FindSubmatch(body)
			if kSub == nil {
				note(fmt.Sprintf("%s: optionalDependencies missing key %s", m.Path, key))
				continue
			}
			if string(kSub[1]) != DevSentinel {
				note(fmt.Sprintf("%s: %s pin %q, want %q", m.Path, key, kSub[1], DevSentinel))
			}
		}
	}
}

func concat(parts ...[]byte) []byte {
	n := 0
	for _, p := range parts {
		n += len(p)
	}
	out := make([]byte, 0, n)
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}
