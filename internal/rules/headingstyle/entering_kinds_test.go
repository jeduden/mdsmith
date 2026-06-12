package headingstyle

import (
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// TestEnteringKinds pins the rule.KindScopedChecker declaration the
// engine's shared walk dispatches on: the exact node kinds CheckNode
// reacts to. The cross-package soundness gate lives in
// internal/integration/kindscope_test.go; this in-package test keeps
// the declaration covered where the method is defined.
func TestEnteringKinds(t *testing.T) {
	want := []ast.NodeKind{ast.KindHeading}
	got := (&Rule{}).EnteringKinds()
	if len(got) != len(want) {
		t.Fatalf("EnteringKinds() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("EnteringKinds()[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}
