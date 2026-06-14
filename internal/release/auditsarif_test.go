package release

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// auditNow is a fixed "today" for the selection tests: any audit dated
// after this is treated as a future-dated typo and ignored.
var auditNow = time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

// makeAudit creates securityDir/<name>/ and, when withSarif, a regular
// findings.sarif inside it.
func makeAudit(t *testing.T, securityDir, name string, withSarif bool) {
	t.Helper()
	dir := filepath.Join(securityDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if withSarif {
		if err := os.WriteFile(filepath.Join(dir, "findings.sarif"), []byte("{}"), 0o644); err != nil {
			t.Fatalf("write sarif: %v", err)
		}
	}
}

// selectAuditCase is one SelectAuditSarifs table row. dirs maps an audit
// directory name to whether it carries a findings.sarif.
type selectAuditCase struct {
	name string
	dirs map[string]bool
	want []string
}

var selectAuditSarifsCases = []selectAuditCase{
	{
		name: "single audit with sarif",
		dirs: map[string]bool{"2026-06-12-full-repo-audit": true},
		want: []string{"2026-06-12-full-repo-audit"},
	},
	{
		name: "newest date wins over older dates",
		dirs: map[string]bool{
			"2026-04-05-adversarial":     true,
			"2026-05-12-supply-chain":    true,
			"2026-06-09-full-repo-audit": true,
			"2026-06-12-full-repo-audit": true,
		},
		want: []string{"2026-06-12-full-repo-audit"},
	},
	{
		name: "every directory sharing the newest date is returned, sorted",
		dirs: map[string]bool{
			"2026-06-12-git-lsp-audit":   true,
			"2026-06-12-full-repo-audit": true,
			"2026-06-09-full-repo-audit": true,
		},
		want: []string{"2026-06-12-full-repo-audit", "2026-06-12-git-lsp-audit"},
	},
	{
		name: "directory without findings.sarif is excluded",
		dirs: map[string]bool{
			"2026-06-12-full-repo-audit": false,
			"2026-06-09-full-repo-audit": true,
		},
		want: []string{"2026-06-09-full-repo-audit"},
	},
	{
		name: "non-date directory names are ignored",
		dirs: map[string]bool{
			"proto":                      true,
			"notes":                      true,
			"2026-06-12-full-repo-audit": true,
		},
		want: []string{"2026-06-12-full-repo-audit"},
	},
	{
		name: "date run into the slug without a separator is ignored",
		dirs: map[string]bool{
			"2026-06-12xfull": true,
			"2026-06-09-real": true,
		},
		want: []string{"2026-06-09-real"},
	},
	{
		name: "bare date directory name without a slug qualifies",
		dirs: map[string]bool{
			"2026-06-12":      true,
			"2026-06-09-real": true,
		},
		want: []string{"2026-06-12"},
	},
	{
		name: "invalid calendar date is ignored",
		dirs: map[string]bool{
			"2026-13-40-bogus": true,
			"2026-06-09-real":  true,
		},
		want: []string{"2026-06-09-real"},
	},
	{
		name: "future-dated directory does not hijack selection",
		dirs: map[string]bool{
			"2099-01-01-typo":            true,
			"2026-06-12-full-repo-audit": true,
		},
		want: []string{"2026-06-12-full-repo-audit"},
	},
	{
		name: "no qualifying directory yields empty",
		dirs: map[string]bool{"2026-06-12-full-repo-audit": false},
		want: nil,
	},
}

func TestSelectAuditSarifs(t *testing.T) {
	for _, tt := range selectAuditSarifsCases {
		t.Run(tt.name, func(t *testing.T) {
			securityDir := t.TempDir()
			for name, withSarif := range tt.dirs {
				makeAudit(t, securityDir, name, withSarif)
			}
			got, err := SelectAuditSarifs(securityDir, auditNow)
			if err != nil {
				t.Fatalf("SelectAuditSarifs: unexpected error: %v", err)
			}
			if !equalStrings(got, tt.want) {
				t.Errorf("SelectAuditSarifs = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSelectAuditSarifsAbsentDir: a missing securityDir is not an error
// — there is simply nothing to upload.
func TestSelectAuditSarifsAbsentDir(t *testing.T) {
	got, err := SelectAuditSarifs(filepath.Join(t.TempDir(), "nope"), auditNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

// TestSelectAuditSarifsSarifIsDir: a findings.sarif that is a directory
// (not a regular file) does not qualify.
func TestSelectAuditSarifsSarifIsDir(t *testing.T) {
	securityDir := t.TempDir()
	dir := filepath.Join(securityDir, "2026-06-12-full-repo-audit", "findings.sarif")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got, err := SelectAuditSarifs(securityDir, auditNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

// TestSelectAuditSarifsSarifIsSymlink: a findings.sarif that is a symlink
// (even to a regular file) does not qualify — Lstat rejects it rather
// than following it.
func TestSelectAuditSarifsSarifIsSymlink(t *testing.T) {
	securityDir := t.TempDir()
	dir := filepath.Join(securityDir, "2026-06-12-full-repo-audit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	target := filepath.Join(securityDir, "real.sarif")
	if err := os.WriteFile(target, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "findings.sarif")); err != nil {
		t.Skipf("symlinks unsupported here: %v", err)
	}
	got, err := SelectAuditSarifs(securityDir, auditNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("symlinked findings.sarif should be excluded, got %v", got)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
