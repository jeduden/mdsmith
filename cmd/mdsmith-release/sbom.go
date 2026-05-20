package main

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/release"
)

func runSBOM(root string, args []string) int {
	fs := flag.NewFlagSet("sbom", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release sbom <out-path>\n\n"+
			"Emit a CycloneDX SBOM of the Go module at the repo root to\n"+
			"<out-path>. Installs the pinned cyclonedx-gomod build into\n"+
			"GOBIN, then runs `cyclonedx-gomod mod -licenses -json`.\n"+
			"The release pipeline calls this after the build matrix so\n"+
			"the SBOM ships in the same checksums.txt / cosign-signed\n"+
			"asset set as the binaries.\n")
	}
	if err := fs.Parse(args); err != nil {
		if code := reportFlagParseErr(err, os.Stderr, "mdsmith-release: sbom"); code >= 0 {
			return code
		}
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}
	return reportError(release.GenerateSBOM(root, fs.Arg(0)))
}
