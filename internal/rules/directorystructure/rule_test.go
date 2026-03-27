package directorystructure

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

func newRule(t *testing.T, allowed []string) *Rule {
	t.Helper()
	r := &Rule{}
	settings := map[string]any{"allowed": allowed}
	if err := r.ApplySettings(settings); err != nil {
		t.Fatalf("newRule: %v", err)
	}
	return r
}

func TestCheck_AllowedDirectory_NoViolation(t *testing.T) {
	r := newRule(t, []string{"docs/**"})
	src := []byte("# Title\n")
	f, err := lint.NewFile("docs/guide.md", src)
	if err != nil {
		t.Fatal(err)
	}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d: %+v", len(diags), diags)
	}
}

func TestCheck_DisallowedDirectory_Violation(t *testing.T) {
	r := newRule(t, []string{"docs/**"})
	src := []byte("# Title\n")
	f, err := lint.NewFile("src/notes.md", src)
	if err != nil {
		t.Fatal(err)
	}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %+v", len(diags), diags)
	}
	if diags[0].RuleID != "MDS033" {
		t.Errorf("expected rule ID MDS033, got %s", diags[0].RuleID)
	}
	if diags[0].Severity != lint.Warning {
		t.Errorf("expected warning severity, got %s", diags[0].Severity)
	}
}

func TestCheck_RootFile_WithDotPattern(t *testing.T) {
	r := newRule(t, []string{"."})
	src := []byte("# README\n")
	f, err := lint.NewFile("README.md", src)
	if err != nil {
		t.Fatal(err)
	}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for root file with '.' pattern, got %d: %+v", len(diags), diags)
	}
}

func TestCheck_RootFile_Disallowed(t *testing.T) {
	r := newRule(t, []string{"docs/**"})
	src := []byte("# README\n")
	f, err := lint.NewFile("README.md", src)
	if err != nil {
		t.Fatal(err)
	}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d: %+v", len(diags), diags)
	}
}

func TestCheck_NestedGlob(t *testing.T) {
	r := newRule(t, []string{"internal/**/testdata/**"})
	src := []byte("# Test\n")
	f, err := lint.NewFile("internal/rules/foo/testdata/good/test.md", src)
	if err != nil {
		t.Fatal(err)
	}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d: %+v", len(diags), diags)
	}
}

func TestCheck_Unconfigured_NoOp(t *testing.T) {
	r := &Rule{}
	src := []byte("# Title\n")
	f, err := lint.NewFile("docs/guide.md", src)
	if err != nil {
		t.Fatal(err)
	}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics (unconfigured rule is no-op), got %d: %+v", len(diags), diags)
	}
}

func TestCheck_EmptyAllowed_WarnsOnEveryFile(t *testing.T) {
	r := newRule(t, []string{})
	src := []byte("# Title\n")
	f, err := lint.NewFile("docs/guide.md", src)
	if err != nil {
		t.Fatal(err)
	}
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic (empty allowed = nothing allowed), got %d: %+v", len(diags), diags)
	}
}

func TestCheck_MultiplePatterns(t *testing.T) {
	r := newRule(t, []string{"docs/**", "plan/**", "."})
	tests := []struct {
		path  string
		wantN int
	}{
		{"docs/guide.md", 0},
		{"plan/roadmap.md", 0},
		{"README.md", 0},
		{"src/notes.md", 1},
	}
	for _, tt := range tests {
		f, err := lint.NewFile(tt.path, []byte("# Title\n"))
		if err != nil {
			t.Fatal(err)
		}
		diags := r.Check(f)
		if len(diags) != tt.wantN {
			t.Errorf("path %q: expected %d diagnostics, got %d: %+v",
				tt.path, tt.wantN, len(diags), diags)
		}
	}
}

func TestEnabledByDefault(t *testing.T) {
	r := &Rule{}
	if r.EnabledByDefault() {
		t.Error("directory-structure should be disabled by default")
	}
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS033" {
		t.Errorf("expected MDS033, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "directory-structure" {
		t.Errorf("expected directory-structure, got %s", r.Name())
	}
}

func TestApplySettings(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"allowed": []any{"docs/**", "plan/**"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Allowed) != 2 {
		t.Fatalf("expected 2 allowed patterns, got %d", len(r.Allowed))
	}
	if r.Allowed[0] != "docs/**" {
		t.Errorf("expected docs/**, got %s", r.Allowed[0])
	}
}

func TestApplySettings_StringSlice(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"allowed": []string{"docs/**"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Allowed) != 1 {
		t.Fatalf("expected 1 allowed pattern, got %d", len(r.Allowed))
	}
}

func TestApplySettings_InvalidGlob(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"allowed": []any{"[invalid"},
	})
	if err == nil {
		t.Error("expected error for invalid glob pattern")
	}
	// Rule must remain unconfigured after error.
	if r.configured {
		t.Error("rule should remain unconfigured after invalid glob error")
	}
	f, _ := lint.NewFile("anywhere.md", []byte("# Title\n"))
	if diags := r.Check(f); len(diags) != 0 {
		t.Errorf("expected no-op after error, got %d diagnostics", len(diags))
	}
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"bogus": true})
	if err == nil {
		t.Error("expected error for unknown setting")
	}
	// Rule must remain unconfigured after error.
	if r.configured {
		t.Error("rule should remain unconfigured after unknown key error")
	}
	f, _ := lint.NewFile("anywhere.md", []byte("# Title\n"))
	if diags := r.Check(f); len(diags) != 0 {
		t.Errorf("expected no-op after error, got %d diagnostics", len(diags))
	}
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	s := r.DefaultSettings()
	if _, ok := s["allowed"]; ok {
		t.Error("default settings should not include 'allowed' key (rule stays unconfigured/no-op)")
	}
}

func TestApplyDefaultSettings_RemainsUnconfigured(t *testing.T) {
	// Simulate the CloneRule/fixture-harness flow: configure the rule,
	// then restore defaults. The rule should return to unconfigured/no-op.
	r := newRule(t, []string{"docs/**"})
	err := r.ApplySettings(r.DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	src := []byte("# Title\n")
	f, err := lint.NewFile("anywhere/guide.md", src)
	if err != nil {
		t.Fatal(err)
	}
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics (restored to unconfigured no-op), got %d: %+v", len(diags), diags)
	}
}
