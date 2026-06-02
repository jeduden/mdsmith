package mdsmith

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

func TestToExplanationNil(t *testing.T) {
	if got := toExplanation(nil); got != nil {
		t.Fatalf("toExplanation(nil) = %+v, want nil", got)
	}
}

// TestToExplanationConverts covers the non-nil branch and the leaf loop:
// a lint.Explanation maps field-for-field to the public Explanation.
func TestToExplanationConverts(t *testing.T) {
	in := &lint.Explanation{
		Rule: "MDS001",
		Leaves: []lint.ExplanationLeaf{
			{Path: "max", Value: 80, Source: "default"},
			{Path: "exclude", Value: "tables", Source: "kind:doc"},
		},
	}
	got := toExplanation(in)
	if got == nil {
		t.Fatal("toExplanation returned nil for a non-nil input")
	}
	if got.Rule != "MDS001" {
		t.Fatalf("Rule = %q, want MDS001", got.Rule)
	}
	if len(got.Leaves) != 2 {
		t.Fatalf("Leaves = %d, want 2", len(got.Leaves))
	}
	if got.Leaves[0].Path != "max" || got.Leaves[0].Value != 80 || got.Leaves[0].Source != "default" {
		t.Fatalf("Leaves[0] = %+v, want {max 80 default}", got.Leaves[0])
	}
	if got.Leaves[1].Path != "exclude" || got.Leaves[1].Source != "kind:doc" {
		t.Fatalf("Leaves[1] = %+v, want {exclude tables kind:doc}", got.Leaves[1])
	}
}

// TestToDiagnosticsClampsNonPositiveLine covers the public-API clamp:
// plan 230's line-0 sentinel must not leak through the Session/WASM API;
// it surfaces as 1, while a RelatedLocation line of 0 ("unknown") is
// preserved.
func TestToDiagnosticsClampsNonPositiveLine(t *testing.T) {
	out := toDiagnostics([]lint.Diagnostic{{
		File: "gen.md", Line: 0, Column: 3, RuleID: "MDS020",
		RelatedLocations: []lint.RelatedLocation{{File: "p.md", Line: 0}},
	}})
	if len(out) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(out))
	}
	if out[0].Line != 1 || out[0].Column != 3 {
		t.Fatalf("public Line/Column = %d/%d, want 1/3 (line clamped, column kept)",
			out[0].Line, out[0].Column)
	}
	if len(out[0].RelatedLocations) != 1 || out[0].RelatedLocations[0].Line != 0 {
		t.Fatalf("related location line not preserved as 0 (unknown)")
	}
}
