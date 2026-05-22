package main

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/jeduden/mdsmith/internal/release"
)

// generateSBOMFunc is the function runSBOM calls to emit the SBOM.
// Production uses release.GenerateSBOM (which goes through the
// default Toolkit and shells out to cyclonedx-gomod); tests swap
// in a fake so the cmd-side dispatcher's success and error
// branches are reachable without exec.
var generateSBOMFunc = release.GenerateSBOM

func runSBOM(root string, args []string) int {
	fs := flag.NewFlagSet("sbom", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: mdsmith-release sbom <out-path>\n\n"+
			"Emit a CycloneDX SBOM of the Go module at the repo root to\n"+
			"<out-path>. Runs `go run github.com/CycloneDX/cyclonedx-gomod\n"+
			"/cmd/cyclonedx-gomod@<pinned> mod -licenses -json` — `go run`\n"+
			"fetches the pinned tool through the module cache so no PATH\n"+
			"setup is required beyond the Go toolchain itself. The release\n"+
			"pipeline calls this after the build matrix so the SBOM ships\n"+
			"in the same checksums.txt / cosign-signed asset set as the\n"+
			"binaries.\n")
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
	return reportError(generateSBOMFunc(root, fs.Arg(0)))
}
