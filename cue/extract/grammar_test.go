package extract

import (
	"strings"
	"testing"
)

// TestSource pins that the embedded grammar is non-empty and carries
// the `#Block` definition the differential test unifies against. Go's
// coverage tooling does not attribute the cross-package calls the
// internal/extract differential test makes, so this in-package test
// keeps the embed accessor covered and guards against an empty embed
// (a missing or renamed grammar.cue would surface here rather than as
// a confusing CUE compile failure across the module boundary).
func TestSource(t *testing.T) {
	src := Source()
	if src == "" {
		t.Fatal("Source() returned empty; grammar.cue did not embed")
	}
	if !strings.Contains(src, "#Block") {
		t.Errorf("Source() missing the #Block definition:\n%s", src)
	}
	if !strings.Contains(src, "#Span") {
		t.Errorf("Source() missing the #Span definition")
	}
}
