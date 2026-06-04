package mdsmith

import (
	"strings"
	"testing"
)

// TestFixRuleAppliesOnlyNamedRule is the LSP per-rule quick-fix
// contract: FixRule applies only the named rule(s), leaving other
// fixable violations in place. Here trailing whitespace (MDS006) is
// fixed while a separate fixable issue the rule does not own is left
// untouched.
func TestFixRuleAppliesOnlyNamedRule(t *testing.T) {
	s := newTestSession(t, "", nil)
	// Two independent fixable issues: trailing spaces (no-trailing-spaces)
	// and a bare URL the bare-url rule would linkify. Fixing only
	// no-trailing-spaces must strip the spaces but leave the bare URL.
	src := []byte("# Title\n\nSee https://example.com now   \n")

	res, err := s.FixRule("a.md", src, []string{"no-trailing-spaces"})
	if err != nil {
		t.Fatalf("FixRule: %v", err)
	}
	if !res.Changed {
		t.Fatal("FixRule: expected Changed=true")
	}
	if strings.Contains(res.Source, "now   ") {
		t.Fatalf("FixRule(no-trailing-spaces): trailing spaces not removed: %q", res.Source)
	}
	if !strings.Contains(res.Source, "https://example.com") {
		t.Fatalf("FixRule must not touch other rules; bare URL line changed: %q", res.Source)
	}
}

// TestFixRuleNoOpReportsUnchanged verifies FixRule reports Changed=false
// and returns the input unchanged when the named rule has nothing to
// fix — the LSP suppresses a no-op quick-fix action.
func TestFixRuleNoOpReportsUnchanged(t *testing.T) {
	s := newTestSession(t, "", nil)
	src := []byte("# Title\n\nClean body paragraph.\n")

	res, err := s.FixRule("a.md", src, []string{"no-trailing-spaces"})
	if err != nil {
		t.Fatalf("FixRule: %v", err)
	}
	if res.Changed {
		t.Fatalf("FixRule(no-op): expected Changed=false, got source %q", res.Source)
	}
	if res.Source != string(src) {
		t.Fatalf("FixRule(no-op): source changed: %q", res.Source)
	}
}

// TestFixRuleEmptyNamesNoOp verifies an empty rule list produces no
// change (matching fix.SourceWithRules).
func TestFixRuleEmptyNamesNoOp(t *testing.T) {
	s := newTestSession(t, "", nil)
	src := []byte("# Title\n\ntrailing   \n")

	res, err := s.FixRule("a.md", src, nil)
	if err != nil {
		t.Fatalf("FixRule: %v", err)
	}
	if res.Changed {
		t.Fatal("FixRule(no names): expected Changed=false")
	}
}
