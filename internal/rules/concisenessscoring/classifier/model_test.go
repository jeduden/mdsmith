package classifier

import (
	"math"
	"testing"
)

func TestLoadEmbedded(t *testing.T) {
	model, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded returned error: %v", err)
	}

	if model.ModelID() == "" {
		t.Fatal("expected non-empty model id")
	}
	if model.Version() == "" {
		t.Fatal("expected non-empty version")
	}
	if model.Threshold() <= 0 || model.Threshold() >= 1 {
		t.Fatalf("expected threshold in (0,1), got %v", model.Threshold())
	}
	if EmbeddedArtifactBytes() <= 0 {
		t.Fatalf("expected embedded artifact bytes > 0")
	}
}

func TestClassifyDeterministic(t *testing.T) {
	model, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded returned error: %v", err)
	}

	text := "It seems this approach may potentially help in many cases."
	base := model.Classify(text)
	for i := 0; i < 20; i++ {
		got := model.Classify(text)
		if got.Label != base.Label {
			t.Fatalf("run %d label mismatch: got %q want %q", i, got.Label, base.Label)
		}
		if math.Abs(got.RiskScore-base.RiskScore) > 1e-12 {
			t.Fatalf(
				"run %d risk mismatch: got %.12f want %.12f",
				i,
				got.RiskScore,
				base.RiskScore,
			)
		}
	}
}

func TestClassifySeparatesVerboseAndDirect(t *testing.T) {
	model, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded returned error: %v", err)
	}

	verbose := model.Classify(
		"Basically, it seems we might want to consider this approach " +
			"in order to improve outcomes in many situations.",
	)
	direct := model.Classify(
		"Run go test ./... and publish checksums for release artifacts.",
	)

	if verbose.RiskScore <= direct.RiskScore {
		t.Fatalf(
			"expected verbose risk > direct risk, got %.4f <= %.4f",
			verbose.RiskScore,
			direct.RiskScore,
		)
	}
	if verbose.Label != "verbose-actionable" {
		t.Fatalf("expected verbose label, got %q", verbose.Label)
	}
	if direct.Label != "acceptable" {
		t.Fatalf("expected acceptable label, got %q", direct.Label)
	}
}
