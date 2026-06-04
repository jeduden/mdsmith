package mdsmith

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	vlog "github.com/jeduden/mdsmith/internal/log"
)

// TestCheckPathsLintsFilesOnDisk drives the native batch op the CLI's
// check subcommand uses: CheckPaths reads the given on-disk files
// through the engine Runner and returns the aggregate Result with
// diagnostics sorted by file/line. A workspace-rooted at the temp dir
// means the engine resolves cross-file content the same way the CLI
// does.
func TestCheckPathsLintsFilesOnDisk(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.md")
	// A line with trailing spaces (MDS006) — fixable, default-enabled.
	if err := os.WriteFile(bad, []byte("# Title\n\ntrailing   \n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s, err := NewSession(SessionOptions{
		Workspace: OSWorkspace{Root: dir},
		Config:    ConfigYAML(""),
	})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Dispose()

	res := s.CheckPaths([]string{bad}, BatchOptions{})
	if res.FilesChecked != 1 {
		t.Fatalf("FilesChecked = %d, want 1", res.FilesChecked)
	}
	if len(res.Diagnostics) == 0 {
		t.Fatalf("CheckPaths: expected a diagnostic for trailing spaces, got none")
	}
}

// TestCheckPathsExplainAttachesProvenance verifies the Explain batch
// option threads through to the Runner so diagnostics carry per-leaf
// provenance — the --explain CLI flag.
func TestCheckPathsExplainAttachesProvenance(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	cfg := "rules:\n  line-length:\n    max: 10\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatalf("WriteFile cfg: %v", err)
	}
	long := filepath.Join(dir, "long.md")
	if err := os.WriteFile(long, []byte("# Title\n\nthis line is definitely longer than ten\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s, err := NewSession(SessionOptions{
		Workspace: OSWorkspace{Root: dir},
		Config:    ConfigPath(cfgPath),
	})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Dispose()

	res := s.CheckPaths([]string{long}, BatchOptions{Explain: true})
	var found bool
	for _, d := range res.Diagnostics {
		if d.RuleID == "MDS001" && d.Explanation != nil {
			found = true
		}
	}
	if !found {
		t.Fatalf("CheckPaths(Explain): expected an MDS001 diagnostic with provenance, got %+v", res.Diagnostics)
	}
}

// TestCheckPathsLoggerReceivesVerboseOutput verifies the Logger batch
// option threads to the Runner so `-v` output reaches the caller's
// writer.
func TestCheckPathsLoggerReceivesVerboseOutput(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "a.md")
	if err := os.WriteFile(doc, []byte("# Title\n\nBody paragraph.\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	var buf bytes.Buffer
	s, err := NewSession(SessionOptions{Workspace: OSWorkspace{Root: dir}, Config: ConfigYAML("")})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Dispose()

	s.CheckPaths([]string{doc}, BatchOptions{Logger: &vlog.Logger{Enabled: true, W: &buf}})
	if !bytes.Contains(buf.Bytes(), []byte("file:")) {
		t.Fatalf("CheckPaths(Logger): expected verbose 'file:' line, got %q", buf.String())
	}
}

// TestFixPathsRewritesFilesOnDisk drives the native batch op the CLI's
// fix subcommand uses: FixPaths fixes the given on-disk files in place
// and reports the modified set. A trailing-space line is rewritten.
func TestFixPathsRewritesFilesOnDisk(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "bad.md")
	if err := os.WriteFile(doc, []byte("# Title\n\ntrailing   \n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s, err := NewSession(SessionOptions{Workspace: OSWorkspace{Root: dir}, Config: ConfigYAML("")})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Dispose()

	res := s.FixPaths([]string{doc}, BatchOptions{})
	if len(res.Modified) != 1 {
		t.Fatalf("FixPaths Modified = %v, want exactly [bad.md]", res.Modified)
	}
	got, err := os.ReadFile(doc)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if bytes.Contains(got, []byte("trailing   ")) {
		t.Fatalf("FixPaths did not strip trailing spaces: %q", got)
	}
}

// TestFixPathsDryRunWritesNothing verifies the DryRun batch option runs
// the fix pipeline but leaves the file untouched while still reporting a
// would-fix preview.
func TestFixPathsDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "bad.md")
	original := []byte("# Title\n\ntrailing   \n")
	if err := os.WriteFile(doc, original, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s, err := NewSession(SessionOptions{Workspace: OSWorkspace{Root: dir}, Config: ConfigYAML("")})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Dispose()

	res := s.FixPaths([]string{doc}, BatchOptions{DryRun: true})
	if len(res.Modified) != 0 {
		t.Fatalf("FixPaths(DryRun) Modified = %v, want empty", res.Modified)
	}
	if res.WouldFix == 0 {
		t.Fatalf("FixPaths(DryRun): expected WouldFix > 0")
	}
	got, err := os.ReadFile(doc)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("FixPaths(DryRun) modified the file on disk: %q", got)
	}
}

// TestCheckPathsMaxInputBytesOverrides verifies a per-call
// MaxInputBytes in BatchOptions overrides the session default so the
// CLI can pass its resolved --max-input-size.
func TestCheckPathsMaxInputBytesOverrides(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "big.md")
	if err := os.WriteFile(doc, []byte("# A heading longer than sixteen bytes\n\nBody.\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s, err := NewSession(SessionOptions{Workspace: OSWorkspace{Root: dir}, Config: ConfigYAML("")})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Dispose()

	res := s.CheckPaths([]string{doc}, BatchOptions{MaxInputBytes: 16})
	if len(res.Errors) == 0 {
		t.Fatalf("CheckPaths(MaxInputBytes=16): expected a 'file too large' error, got none")
	}
}
