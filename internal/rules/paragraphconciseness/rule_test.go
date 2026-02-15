package paragraphconciseness

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

func verboseParagraph() string {
	return "In order to make sure that we are all on the same page, " +
		"it is important to note that this is basically a very simple " +
		"update that we might adjust later in most cases. In order to " +
		"make sure that it lands, we will probably revisit the same " +
		"point again."
}

func conciseParagraph() string {
	return "The build tool reads each markdown file, checks heading order, " +
		"verifies links, and reports exact lines so teams can fix " +
		"issues quickly without extra review cycles."
}

func newDefaultRule() *Rule {
	return &Rule{
		MaxVerbosity:   defaultMaxVerbosity,
		MinWords:       defaultMinWords,
		MinContentRate: defaultMinContentRate,
		FillerWords:    append([]string(nil), defaultFillerWords...),
		HedgePhrases:   append([]string(nil), defaultHedgePhrases...),
		VerbosePhrases: append([]string(nil), defaultVerbosePhrases...),
	}
}

func TestCheck_OverThreshold(t *testing.T) {
	src := []byte(verboseParagraph() + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := newDefaultRule()
	r.MaxVerbosity = 20
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}

	d := diags[0]
	if d.RuleID != "MDS026" {
		t.Errorf("expected rule ID MDS026, got %s", d.RuleID)
	}
	if d.RuleName != "paragraph-conciseness" {
		t.Errorf(
			"expected rule name paragraph-conciseness, got %s",
			d.RuleName,
		)
	}
	if d.Severity != lint.Warning {
		t.Errorf("expected severity warning, got %s", d.Severity)
	}
	if d.Column != 1 {
		t.Errorf("expected column 1, got %d", d.Column)
	}
	if !strings.Contains(d.Message, "conciseness") {
		t.Errorf(
			"expected message to include conciseness score, got %q",
			d.Message,
		)
	}
	if !strings.Contains(d.Message, "target >=") {
		t.Errorf(
			"expected message to include target guidance, got %q",
			d.Message,
		)
	}
	if !strings.Contains(d.Message, "trim phrases like") {
		t.Errorf(
			"expected message to include phrase examples, got %q",
			d.Message,
		)
	}
}

func TestCheck_UnderThreshold(t *testing.T) {
	src := []byte(conciseParagraph() + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := newDefaultRule()
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheck_ShortParagraphSkipped(t *testing.T) {
	src := []byte(verboseParagraph() + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := newDefaultRule()
	r.MinWords = 200
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf(
			"expected 0 diagnostics for short paragraph, got %d",
			len(diags),
		)
	}
}

func TestCheck_DiagnosticLine(t *testing.T) {
	src := []byte("# Heading\n\n" + verboseParagraph() + "\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := newDefaultRule()
	r.MaxVerbosity = 20
	diags := r.Check(f)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Line != 3 {
		t.Errorf("expected line 3, got %d", diags[0].Line)
	}
}

func TestCheck_TableSkipped(t *testing.T) {
	src := []byte("| Setting | Type | Default |\n" +
		"|---------|------|---------|\n" +
		"| max-verbosity | float | 35 |\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		t.Fatal(err)
	}

	r := newDefaultRule()
	r.MinWords = 1
	diags := r.Check(f)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics for table, got %d", len(diags))
	}
}

func TestApplySettings_Valid(t *testing.T) {
	r := newDefaultRule()
	err := r.ApplySettings(map[string]any{
		"max-verbosity":     40.0,
		"min-words":         24,
		"min-content-ratio": 0.5,
		"filler-words":      []any{"basically", "really"},
		"hedge-phrases":     []any{"in most cases"},
		"verbose-phrases":   []any{"in order to"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.MaxVerbosity != 40.0 {
		t.Errorf("expected MaxVerbosity=40.0, got %f", r.MaxVerbosity)
	}
	if r.MinWords != 24 {
		t.Errorf("expected MinWords=24, got %d", r.MinWords)
	}
	if r.MinContentRate != 0.5 {
		t.Errorf(
			"expected MinContentRate=0.5, got %f",
			r.MinContentRate,
		)
	}
	if len(r.FillerWords) != 2 {
		t.Errorf("expected 2 filler words, got %d", len(r.FillerWords))
	}
	if len(r.HedgePhrases) != 1 {
		t.Errorf("expected 1 hedge phrase, got %d", len(r.HedgePhrases))
	}
	if len(r.VerbosePhrases) != 1 {
		t.Errorf(
			"expected 1 verbose phrase, got %d",
			len(r.VerbosePhrases),
		)
	}
}

func TestApplySettings_InvalidType(t *testing.T) {
	r := newDefaultRule()
	err := r.ApplySettings(map[string]any{
		"max-verbosity": "high",
	})
	if err == nil {
		t.Fatal("expected error for non-number max-verbosity")
	}
}

func TestApplySettings_InvalidRange(t *testing.T) {
	r := newDefaultRule()
	err := r.ApplySettings(map[string]any{
		"min-content-ratio": 2.0,
	})
	if err == nil {
		t.Fatal("expected error for out-of-range min-content-ratio")
	}
}

func TestApplySettings_InvalidFillerWords(t *testing.T) {
	r := newDefaultRule()
	err := r.ApplySettings(map[string]any{
		"filler-words": []any{"in order"},
	})
	if err == nil {
		t.Fatal("expected error for multi-word filler entry")
	}
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := newDefaultRule()
	err := r.ApplySettings(map[string]any{"unknown": true})
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	if ds["max-verbosity"] != defaultMaxVerbosity {
		t.Errorf(
			"expected max-verbosity=%v, got %v",
			defaultMaxVerbosity,
			ds["max-verbosity"],
		)
	}
	if ds["min-words"] != defaultMinWords {
		t.Errorf("expected min-words=%d, got %v", defaultMinWords, ds["min-words"])
	}
	if ds["min-content-ratio"] != defaultMinContentRate {
		t.Errorf(
			"expected min-content-ratio=%v, got %v",
			defaultMinContentRate,
			ds["min-content-ratio"],
		)
	}
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS026" {
		t.Errorf("expected MDS026, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "paragraph-conciseness" {
		t.Errorf(
			"expected paragraph-conciseness, got %s",
			r.Name(),
		)
	}
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	if r.Category() != "meta" {
		t.Errorf("expected meta, got %s", r.Category())
	}
}
