package commandsshowoutput

import "testing"

// TestStripPromptAfter_UnchangedLineNoAlloc confirms stripPromptAfter
// returns a blank or non-prompt content line as-is instead of copying it
// through string(line). Fix's line loop calls stripPromptAfter once per
// content line of every flagged block, and blank separator lines between
// commands (see TestFix_PreservesBlankLines) are common inside a flagged
// block — before this fix, every one of those unchanged lines paid a
// full-line string() copy it never used for anything but a verbatim
// write-back. Measured baseline before the fix: 1 alloc/op.
func TestStripPromptAfter_UnchangedLineNoAlloc(t *testing.T) {
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	line := []byte("   ")
	allocs := testing.AllocsPerRun(200, func() {
		_ = stripPromptAfter(line, 0)
	})
	t.Logf("stripPromptAfter (unchanged) allocs/op = %.0f", allocs)
	if allocs > 0 {
		t.Fatalf("stripPromptAfter (unchanged) allocs/op = %.0f, want 0 (return line as-is, not string(line))", allocs)
	}
}
