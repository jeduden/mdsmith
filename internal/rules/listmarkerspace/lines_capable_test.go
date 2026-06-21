package listmarkerspace

import "testing"

// TestLinesCapable pins the rule.LinesChecker marker: the rule's Check serves
// the parse-skip (nil-AST) path itself, so the engine routes it there instead
// of dropping it. The value is constant true.
func TestLinesCapable(t *testing.T) {
	if !(&Rule{}).LinesCapable() {
		t.Fatal("LinesCapable must be true so the parse-skip path runs the rule")
	}
}
