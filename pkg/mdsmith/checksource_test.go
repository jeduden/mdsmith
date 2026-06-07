package mdsmith

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCheckSourceFindsDiagnostic verifies CheckSource lints an in-memory
// source and returns diagnostics. This is the stdin path: the CLI passes
// the raw source bytes rather than a file path.
func TestCheckSourceFindsDiagnostic(t *testing.T) {
	s := newTestSession(t, "", nil)

	// Trailing spaces on line 3 trigger MDS006.
	src := []byte("# Title\n\ntrailing   \n")
	res := s.CheckSource("<stdin>", src, BatchOptions{})
	var found bool
	for _, d := range res.Diagnostics {
		if d.RuleID == "MDS006" {
			found = true
		}
	}
	if !found {
		t.Fatalf("CheckSource: expected MDS006 diagnostic for trailing spaces, got %+v", res.Diagnostics)
	}
}

// TestCheckSourceClean verifies CheckSource returns zero diagnostics for
// a conformant in-memory source.
func TestCheckSourceClean(t *testing.T) {
	s := newTestSession(t, "", nil)

	src := []byte("# Title\n\nBody paragraph.\n")
	res := s.CheckSource("<stdin>", src, BatchOptions{})
	if len(res.Diagnostics) != 0 {
		t.Fatalf("CheckSource(clean): expected no diagnostics, got %+v", res.Diagnostics)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("CheckSource(clean): expected no errors, got %+v", res.Errors)
	}
}

// TestCheckSourceMaxInputBytesOverride verifies a per-call MaxInputBytes
// in BatchOptions limits the source size checked, consistent with
// CheckPaths behaviour.
func TestCheckSourceMaxInputBytesOverride(t *testing.T) {
	s := newTestSession(t, "", nil)

	// Source is 25 bytes; cap at 8 to force a "file too large" error.
	src := []byte("# Title\n\nBody paragraph.\n")
	res := s.CheckSource("<stdin>", src, BatchOptions{MaxInputBytes: 8})
	if len(res.Errors) == 0 {
		t.Fatalf("CheckSource(MaxInputBytes=8): expected a 'file too large' error, got none")
	}
}

// TestCheckSourceConfigPath verifies CheckSource works when the session
// was built from a config file path and that the loaded config is applied
// to the in-memory source. A tight line-length cap (max: 10) triggers
// MDS001 on a long source line, proving the disk config was read and
// applied rather than silently falling back to defaults.
func TestCheckSourceConfigPath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	if err := os.WriteFile(cfgPath, []byte("rules:\n  line-length:\n    max: 10\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s, err := NewSession(SessionOptions{
		Workspace: OSWorkspace{Root: dir},
		Config:    ConfigPath(cfgPath),
	})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(s.Dispose)

	// Line 3 is longer than 10 chars, so MDS001 must fire if the config
	// was loaded correctly.
	src := []byte("# Title\n\nThis line is definitely longer than ten.\n")
	res := s.CheckSource("<stdin>", src, BatchOptions{})
	var found bool
	for _, d := range res.Diagnostics {
		if d.RuleID == "MDS001" {
			found = true
		}
	}
	if !found {
		t.Fatalf("CheckSource with ConfigPath(max=10): expected MDS001 diagnostic, got %+v", res.Diagnostics)
	}
}
