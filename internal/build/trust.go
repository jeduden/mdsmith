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

// configFileName is the workspace config file whose bytes the trust gate
// pins. It is the only file that can declare a build: section, so it is
// the only file the gate must cover.
const configFileName = ".mdsmith.yml"

// trustMarkerName is the per-clone trust marker sitting beside the
// config. It is .gitignored: trust is a property of a checkout, not of
// the repository.
const trustMarkerName = ".mdsmith.yml.trust"

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

// TrustMarkerPath returns the absolute path to the trust marker beside
// the config in root.
func TrustMarkerPath(root string) string {
	return filepath.Join(root, trustMarkerName)
}

// configPath returns the absolute path to the workspace config in root.
func configPath(root string) string {
	return filepath.Join(root, configFileName)
}

// CheckTrust decides whether the build pass may run recipes in root. The
// envLookup callback reports whether a named environment variable is set
// to a non-empty value; production passes a closure over os.Getenv so
// tests can inject a controlled environment.
//
// Trust is granted when either:
//   - MDSMITH_TRUST_BUILD is set (ViaEnv), or
//   - .mdsmith.yml.trust exists and its bytes are identical to the
//     current .mdsmith.yml.
//
// Any drift, a missing marker, or a missing/unreadable config denies
// trust with a human-readable Reason.
func CheckTrust(root string, envLookup func(name string) bool) TrustResult {
	if envLookup != nil && envLookup(envTrustBuild) {
		return TrustResult{Trusted: true, ViaEnv: true}
	}

	cfg, err := os.ReadFile(configPath(root)) //nolint:gosec // path is the workspace config
	if err != nil {
		return TrustResult{Reason: fmt.Sprintf("cannot read %s: %v", configFileName, err)}
	}

	marker, err := os.ReadFile(TrustMarkerPath(root)) //nolint:gosec // path is the workspace trust marker
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return TrustResult{Reason: fmt.Sprintf(
				"build not trusted: %s is missing; review %s and run `mdsmith trust`"+
					" (or set %s=1 in a sandboxed CI)",
				trustMarkerName, configFileName, envTrustBuild,
			)}
		}
		return TrustResult{Reason: fmt.Sprintf("cannot read %s: %v", trustMarkerName, err)}
	}

	if !bytes.Equal(cfg, marker) {
		return TrustResult{Reason: fmt.Sprintf(
			"build not trusted: %s changed since it was trusted; review the diff with"+
				" `mdsmith trust` and re-trust",
			configFileName,
		)}
	}

	return TrustResult{Trusted: true}
}

// WriteTrustMarker copies the current .mdsmith.yml bytes into the trust
// marker, recording the config as trusted. It is written 0o600: trust is
// a per-user decision and the marker should not be group- or
// world-readable. The write is atomic (temp file plus rename) so a
// crash mid-write never leaves a half-written marker that would falsely
// fail the byte comparison.
func WriteTrustMarker(root string) error {
	cfg, err := os.ReadFile(configPath(root)) //nolint:gosec // path is the workspace config
	if err != nil {
		return fmt.Errorf("reading %s: %w", configFileName, err)
	}
	dst := TrustMarkerPath(root)
	tmp, err := os.CreateTemp(root, trustMarkerName+".*")
	if err != nil {
		return fmt.Errorf("creating trust marker: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) //nolint:errcheck // best-effort cleanup if rename succeeds it is gone
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("setting trust marker mode: %w", err)
	}
	if _, err := tmp.Write(cfg); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing trust marker: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing trust marker: %w", err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return fmt.Errorf("installing trust marker: %w", err)
	}
	return nil
}

// TrustDiff returns a unified diff between the stored trust marker (old)
// and the current config (new), whether the two differ, and any read
// error. A missing marker is treated as an empty old side, so the diff
// shows the whole config as added and changed is true.
func TrustDiff(root string) (diff string, changed bool, err error) {
	cfg, err := os.ReadFile(configPath(root)) //nolint:gosec // path is the workspace config
	if err != nil {
		return "", false, fmt.Errorf("reading %s: %w", configFileName, err)
	}
	var marker []byte
	markerBytes, merr := os.ReadFile(TrustMarkerPath(root)) //nolint:gosec // path is the workspace trust marker
	switch {
	case merr == nil:
		marker = markerBytes
	case errors.Is(merr, os.ErrNotExist):
		marker = nil
	default:
		return "", false, fmt.Errorf("reading %s: %w", trustMarkerName, merr)
	}

	if bytes.Equal(cfg, marker) {
		return "", false, nil
	}

	edits := myers.ComputeEdits(span.URIFromPath(trustMarkerName), string(marker), string(cfg))
	unified := fmt.Sprint(gotextdiff.ToUnified(trustMarkerName, configFileName, string(marker), edits))
	return unified, true, nil
}
