package tableformat

import "testing"

// TestCheck_EarlyExitOnNoPipe covers the plan-195 file-level
// early-exit on `bytes.IndexByte(f.Source, '|') < 0`. A regression
// that removes the guard would still produce zero diagnostics on
// this fixture (no tables, no violations to report), but the
// early-exit explicitly returns nil before paying the code-block
// AST walk and the per-line table-detection pass. Pinning the
// behaviour anchors the optimisation against an accidental
// rollback.
func TestCheck_EarlyExitOnNoPipe(t *testing.T) {
	f := newTestFile(t, "# Title\n\nProse without tables.\n")
	r := &Rule{}
	if diags := r.Check(f); diags != nil {
		t.Fatalf("Check on table-free file returned %d diagnostics, want nil",
			len(diags))
	}
}
