package release

import (
	"fmt"
)

// cyclonedxGomodVersion pins the cyclonedx-gomod build the
// release pipeline installs to emit the SBOM. Bumping this
// constant is the single point of truth — release.yml only
// runs `go run ./cmd/mdsmith-release sbom`, never `go install`.
const cyclonedxGomodVersion = "v1.9.0"

// GenerateSBOM writes a CycloneDX SBOM for the Go module at root
// to outPath. The pipeline:
//
//  1. `go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@<pinned>`
//  2. `cyclonedx-gomod mod -licenses -json -output <outPath>`
//
// Both calls go through the Toolkit's Runner so tests can
// drive the install-failed and emit-failed branches without
// putting the tool on PATH.
func (t *Toolkit) GenerateSBOM(root, outPath string) error {
	if root == "" {
		return fmt.Errorf("sbom: empty root")
	}
	if outPath == "" {
		return fmt.Errorf("sbom: empty out path")
	}
	if err := t.runner.RunCommand(root, "go", "install",
		"github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@"+cyclonedxGomodVersion,
	); err != nil {
		return fmt.Errorf("install cyclonedx-gomod %s: %w", cyclonedxGomodVersion, err)
	}
	if err := t.runner.RunCommand(root, "cyclonedx-gomod",
		"mod", "-licenses", "-json", "-output", outPath,
	); err != nil {
		return fmt.Errorf("emit SBOM: %w", err)
	}
	return nil
}

// GenerateSBOM delegates to a default-OS Toolkit (see Stamp).
func GenerateSBOM(root, outPath string) error {
	return New().GenerateSBOM(root, outPath)
}
