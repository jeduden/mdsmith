package release

import (
	"fmt"
)

// cyclonedxGomodVersion pins the cyclonedx-gomod build the release
// pipeline runs to emit the SBOM. Bumping this constant is the
// single point of truth — release.yml only runs
// `go run ./cmd/mdsmith-release sbom`, never `go install`.
const cyclonedxGomodVersion = "v1.9.0"

// GenerateSBOM writes a CycloneDX SBOM for the Go module at root
// to outPath. Implemented as a single `go run
// github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@<pinned>
// mod -licenses -json -output <outPath>` invocation. `go run`
// fetches the pinned tool through the Go module cache and
// executes it — no `go install` and no PATH-dependent second hop,
// so the command works on any runner with a Go toolchain on PATH.
// The Runner is injectable so tests can drive the success and
// failure branches without invoking go.
func (t *Toolkit) GenerateSBOM(root, outPath string) error {
	if root == "" {
		return fmt.Errorf("sbom: empty root")
	}
	if outPath == "" {
		return fmt.Errorf("sbom: empty out path")
	}
	if err := t.runner.RunCommand(root, "go", "run",
		"github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@"+cyclonedxGomodVersion,
		"mod", "-licenses", "-json", "-output", outPath,
	); err != nil {
		return fmt.Errorf("emit SBOM via cyclonedx-gomod %s: %w",
			cyclonedxGomodVersion, err)
	}
	return nil
}

// GenerateSBOM delegates to a default-OS Toolkit (see Stamp).
func GenerateSBOM(root, outPath string) error {
	return New().GenerateSBOM(root, outPath)
}
