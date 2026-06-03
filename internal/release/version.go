// Package release provides the helper logic the release workflow
// invokes via cmd/mdsmith-release/. The package was originally a
// set of bash scripts under scripts/, but JSON/TOML editing in
// perl regex and indirect testing through `bash <script>` made
// them brittle. The Go port runs under one toolchain, has direct
// tests, and reports actionable errors without shelling out.
//
// Production code constructs a Toolkit via New(); tests can
// substitute a fake FS via NewWithFS to exercise IO error
// branches the real OS cannot reliably trigger.
package release

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
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

// Manifest is one tracked file. RequiredMdsmithPins lists the
// @mdsmith/* dependency keys this manifest must declare at the
// dev sentinel. When non-empty, Stamp rewrites every @mdsmith/*
// pin found in the file (so a future extra entry stays in sync)
// and Check verifies each listed key is present at DevSentinel.
type Manifest struct {
	Path                string
	Kind                ManifestKind
	RequiredMdsmithPins []string
}

// TrackedManifests returns the set of manifests Stamp rewrites
// and Check verifies via the Toolkit's FS. The npm/platforms/
// directory is not in source control — BuildNpmPlatforms emits
// the platform sub-packages under its own outDir argument
// (npm/dist in the release workflow). Anything that has
// already materialised npm/platforms/<plat>/package.json (a
// previous local Stamp+BuildNpmPlatforms invocation, a custom
// staging step) is stamped here too; if the directory is
// absent the helper just returns the four checked-in manifests.
//
// The VS Code extension manifest pins @mdsmith/cli as an
// optionalDependency so the release-time `bun install` pulls
// the just-published binary into node_modules and build.ts
// bundles it into the .vsix. Stamp keeps that pin in lockstep
// with the release version; without the rewrite, bun would
// still chase the 0.0.0-dev sentinel and silently skip it.
func (t *Toolkit) TrackedManifests(root string) []Manifest {
	out := []Manifest{
		{filepath.Join(root, "editors", "vscode", "package.json"), ManifestJSON, []string{"@mdsmith/cli"}},
		// The Obsidian plugin bundles the WASM engine in-tree (no
		// @mdsmith/* npm pin), so only its version field is stamped.
		// manifest.json is the version Obsidian reads; package.json keeps
		// the dev tooling in lockstep. The release `obsidian` job runs
		// `mdsmith-release stamp` and zips the stamped copies.
		{filepath.Join(root, "editors", "obsidian", "manifest.json"), ManifestJSON, nil},
		{filepath.Join(root, "editors", "obsidian", "package.json"), ManifestJSON, nil},
		{filepath.Join(root, "npm", "mdsmith", "package.json"), ManifestJSON, PlatformPackages},
	}
	platformsDir := filepath.Join(root, "npm", "platforms")
	if entries, err := t.fs.ReadDir(platformsDir); err == nil {
		for _, e := range entries {
			p := filepath.Join(platformsDir, e.Name(), "package.json")
			if _, err := t.fs.Stat(p); err == nil {
				out = append(out, Manifest{p, ManifestJSON, nil})
			}
		}
	}
	out = append(out, Manifest{filepath.Join(root, "python", "pyproject.toml"), ManifestTOML, nil})
	// website/hugo.toml carries the version the deployed
	// mdsmith.dev site renders in topnav and elsewhere. The
	// pages-deploy workflow re-runs Stamp before `hugo --minify`
	// so the site always reflects the release tag rather than
	// the dev sentinel.
	out = append(out, Manifest{filepath.Join(root, "website", "hugo.toml"), ManifestTOML, nil})
	return out
}

// TrackedManifests is a thin wrapper over the default Toolkit so
// callers without an explicit Toolkit can still introspect the
// tracked set.
func TrackedManifests(root string) []Manifest { return New().TrackedManifests(root) }

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
// missing a version field — or missing the @mdsmith/*
// optionalDependencies block on a manifest that declares any
// RequiredMdsmithPins — is a hard error.
func (t *Toolkit) Stamp(root, version string) error {
	if err := ValidateSemver(version); err != nil {
		return err
	}
	for _, m := range t.TrackedManifests(root) {
		if err := t.stampManifest(m, version); err != nil {
			return err
		}
	}
	return nil
}

// Stamp delegates to a default-OS Toolkit so callers without an
// explicit Toolkit (for example, the cmd binary) can stay
// terse. Tests that need fault injection construct a Toolkit
// via NewWithFS instead.
func Stamp(root, version string) error { return New().Stamp(root, version) }

func (t *Toolkit) stampManifest(m Manifest, version string) error {
	body, err := t.fs.ReadFile(m.Path)
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
	if len(m.RequiredMdsmithPins) > 0 {
		if !jsonOptDepRE.Match(out) {
			return fmt.Errorf("%s: no @mdsmith/* optionalDependencies pins found", m.Path)
		}
		out = jsonOptDepRE.ReplaceAllFunc(out, func(match []byte) []byte {
			sub := jsonOptDepRE.FindSubmatchIndex(match)
			return concat(match[:sub[4]], []byte(version), match[sub[5]:])
		})
	}
	return t.fs.WriteFile(m.Path, out, 0o644)
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
func (t *Toolkit) Check(root string) error {
	var problems []string
	note := func(msg string) { problems = append(problems, msg) }
	for _, m := range t.TrackedManifests(root) {
		t.checkManifest(m, note)
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "\n"))
	}
	return nil
}

// Check delegates to a default-OS Toolkit (see Stamp).
func Check(root string) error { return New().Check(root) }

// pinRECache caches compiled optionalDependencies pin regexes by package name so
// repeated checkManifest calls for the same key set skip NFA recompilation.
var pinRECache sync.Map // map[string]*regexp.Regexp

func pinRE(key string) *regexp.Regexp {
	if v, ok := pinRECache.Load(key); ok {
		return v.(*regexp.Regexp)
	}
	re := regexp.MustCompile(`(?m)^[ \t]*"` + regexp.QuoteMeta(key) + `"[ \t]*:[ \t]*"([^"]+)"`)
	pinRECache.Store(key, re)
	return re
}

func (t *Toolkit) checkManifest(m Manifest, note func(string)) {
	body, err := t.fs.ReadFile(m.Path)
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
	for _, key := range m.RequiredMdsmithPins {
		kSub := pinRE(key).FindSubmatch(body)
		if kSub == nil {
			note(fmt.Sprintf("%s: optionalDependencies missing key %s", m.Path, key))
			continue
		}
		if string(kSub[1]) != DevSentinel {
			note(fmt.Sprintf("%s: %s pin %q, want %q", m.Path, key, kSub[1], DevSentinel))
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
