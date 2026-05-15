package release

import "fmt"

// BuildWebsite prepares the Hugo content tree: optionally runs
// `mdsmith fix` against srcDir so every <?catalog?>/<?include?>
// body is current, then snapshots srcDir into dstDir via
// SyncDocs. It is the canonical implementation behind the
// website sync — both local dev and the pages-deploy workflow
// call this rather than carrying the fix+sync sequence as inline
// shell (see docs/development/release-tooling.md: every workflow
// that needs runtime logic goes through this binary).
//
// The fix step shells out to `go run ./cmd/mdsmith` through the
// Runner seam, mirroring how BuildWheels invokes python; it
// expects the caller's working directory to be the repo root
// (true in CI and for the documented local invocation).
func (t *Toolkit) BuildWebsite(srcDir, dstDir string, runFix bool) error {
	if runFix {
		if err := t.runner.RunCommand("", "go", "run", "./cmd/mdsmith", "fix", srcDir); err != nil {
			return fmt.Errorf("mdsmith fix %s: %w", srcDir, err)
		}
	}
	if err := t.SyncDocs(srcDir, dstDir); err != nil {
		return fmt.Errorf("sync %s -> %s: %w", srcDir, dstDir, err)
	}
	return nil
}

// BuildWebsite delegates to a default-OS Toolkit (see Stamp).
func BuildWebsite(srcDir, dstDir string, runFix bool) error {
	return New().BuildWebsite(srcDir, dstDir, runFix)
}
