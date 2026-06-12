package build

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
)

// defaultConfigFileName is the conventional workspace config filename.
// The trust gate pins whichever config file the run actually loaded
// (which may differ under `mdsmith fix -c other.yml`), but this name is
// used in diagnostics and to locate the default config under a root.
const defaultConfigFileName = ".mdsmith.yml"

// trustMarkerSuffix is appended to the loaded config's path to name its
// per-clone trust marker (e.g. .mdsmith.yml -> .mdsmith.yml.trust). The
// marker is .gitignored: trust is a property of a checkout, not of the
// repository.
const trustMarkerSuffix = ".trust"

// envTrustBuild, when set to a non-empty value, grants build trust
// without a marker file. CI environments are presumed sandboxed and opt
// in this way rather than committing a marker.
const envTrustBuild = "MDSMITH_TRUST_BUILD"

// TrustResult is the verdict of the trust gate.
type TrustResult struct {
	// Trusted is true when the build pass may run recipes.
	Trusted bool
	// ViaEnv is true when trust came from MDSMITH_TRUST_BUILD rather than
	// a matching marker file.
	ViaEnv bool
	// Reason explains why trust was denied. It is empty when Trusted.
	Reason string
}

// ConfigPathForRoot returns the default config path under root. It is the
// fallback the trust subcommand uses when no explicit --config was given.
func ConfigPathForRoot(root string) string {
	return filepath.Join(root, defaultConfigFileName)
}

// TrustMarkerPath returns the trust marker path for a given loaded config
// path (configPath + ".trust"). An empty configPath falls back to the
// default config name in the current directory, so the marker is always
// named after the file the gate actually pins.
func TrustMarkerPath(configPath string) string {
	if configPath == "" {
		configPath = defaultConfigFileName
	}
	return configPath + trustMarkerSuffix
}

// CheckTrust decides whether the build pass may run recipes for the
// config at configPath. The envLookup callback reports whether a named
// environment variable is set to a non-empty value; production passes a
// closure over os.Getenv so tests can inject a controlled environment.
//
// Trust is granted when either:
//   - MDSMITH_TRUST_BUILD is set (ViaEnv), or
//   - the marker <configPath>.trust exists and its bytes are identical to
//     the current config.
//
// Any drift, a missing marker, or a missing/unreadable config denies
// trust with a human-readable Reason. Pinning the *loaded* config path
// (not a hardcoded .mdsmith.yml) keeps the gate correct under
// `mdsmith fix -c other.yml`.
func CheckTrust(configPath string, envLookup func(name string) bool) TrustResult {
	if envLookup != nil && envLookup(envTrustBuild) {
		return TrustResult{Trusted: true, ViaEnv: true}
	}
	if configPath == "" {
		configPath = defaultConfigFileName
	}
	name := filepath.Base(configPath)

	cfg, err := os.ReadFile(configPath) //nolint:gosec // path is the loaded workspace config
	if err != nil {
		return TrustResult{Reason: fmt.Sprintf("cannot read %s: %v", name, err)}
	}

	markerPath := TrustMarkerPath(configPath)
	marker, err := os.ReadFile(markerPath) //nolint:gosec // path is the workspace trust marker
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return TrustResult{Reason: fmt.Sprintf(
				"build not trusted: %s is missing; review %s and run `mdsmith trust`"+
					" (or set %s=1 in a sandboxed CI)",
				filepath.Base(markerPath), name, envTrustBuild,
			)}
		}
		return TrustResult{Reason: fmt.Sprintf("cannot read %s: %v", filepath.Base(markerPath), err)}
	}

	if !bytes.Equal(cfg, marker) {
		return TrustResult{Reason: fmt.Sprintf(
			"build not trusted: %s changed since it was trusted; review the diff with"+
				" `mdsmith trust` and re-trust",
			name,
		)}
	}

	return TrustResult{Trusted: true}
}

// WriteTrustMarker copies the current config bytes into its trust marker,
// recording the config at configPath as trusted. The marker is written
// 0o600: trust is a per-user decision and should not be group- or
// world-readable. The write is atomic (temp file plus rename) so a crash
// mid-write never leaves a half-written marker that would falsely fail
// the byte comparison.
func WriteTrustMarker(configPath string) error {
	if configPath == "" {
		configPath = defaultConfigFileName
	}
	name := filepath.Base(configPath)
	cfg, err := os.ReadFile(configPath) //nolint:gosec // path is the loaded workspace config
	if err != nil {
		return fmt.Errorf("reading %s: %w", name, err)
	}
	if err := atomicWriteFile(TrustMarkerPath(configPath), 0o600, cfg); err != nil {
		return fmt.Errorf("installing trust marker: %w", err)
	}
	return nil
}

// TrustDiff returns a unified diff between the stored trust marker (old)
// and the current config at configPath (new), whether the two differ, and
// any read error. A missing marker is treated as an empty old side, so
// the diff shows the whole config as added and changed is true.
func TrustDiff(configPath string) (diff string, changed bool, err error) {
	if configPath == "" {
		configPath = defaultConfigFileName
	}
	name := filepath.Base(configPath)
	cfg, err := os.ReadFile(configPath) //nolint:gosec // path is the loaded workspace config
	if err != nil {
		return "", false, fmt.Errorf("reading %s: %w", name, err)
	}
	markerPath := TrustMarkerPath(configPath)
	markerName := filepath.Base(markerPath)
	var marker []byte
	markerBytes, merr := os.ReadFile(markerPath) //nolint:gosec // path is the workspace trust marker
	switch {
	case merr == nil:
		marker = markerBytes
	case errors.Is(merr, os.ErrNotExist):
		marker = nil
	default:
		return "", false, fmt.Errorf("reading %s: %w", markerName, merr)
	}

	if bytes.Equal(cfg, marker) {
		return "", false, nil
	}

	edits := myers.ComputeEdits(span.URIFromPath(markerName), string(marker), string(cfg))
	unified := fmt.Sprint(gotextdiff.ToUnified(markerName, name, string(marker), edits))
	return unified, true, nil
}
