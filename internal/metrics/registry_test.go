package metrics

import (
	"math"
	"strings"
	"testing"
)

func TestParseScope(t *testing.T) {
	scope, err := ParseScope("file")
	if err != nil {
		t.Fatalf("ParseScope(file): %v", err)
	}
	if scope != ScopeFile {
		t.Fatalf("scope = %q, want %q", scope, ScopeFile)
	}

	if _, err := ParseScope("paragraph"); err == nil {
		t.Fatal("expected error for unsupported scope")
	}
}

func TestParseOrder(t *testing.T) {
	order, err := ParseOrder("asc")
	if err != nil {
		t.Fatalf("ParseOrder(asc): %v", err)
	}
	if order != OrderAsc {
		t.Fatalf("order = %q, want %q", order, OrderAsc)
	}

	order, err = ParseOrder("")
	if err != nil {
		t.Fatalf("ParseOrder(empty): %v", err)
	}
	if order != OrderDesc {
		t.Fatalf("default order = %q, want %q", order, OrderDesc)
	}

	if _, err := ParseOrder("sideways"); err == nil {
		t.Fatal("expected error for invalid order")
	}
}

func TestResolve_Defaults(t *testing.T) {
	defs, err := Resolve(ScopeFile, nil)
	if err != nil {
		t.Fatalf("Resolve defaults: %v", err)
	}
	if len(defs) == 0 {
		t.Fatal("expected default metrics")
	}
	if defs[0].ID != "MET001" {
		t.Fatalf("first default metric = %q, want MET001", defs[0].ID)
	}
}

func TestResolve_UnknownMetricHasActionableError(t *testing.T) {
	_, err := Resolve(ScopeFile, []string{"bogus"})
	if err == nil {
		t.Fatal("expected unknown metric error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "unknown metric") {
		t.Fatalf("error = %q, expected unknown metric message", msg)
	}
	if !strings.Contains(msg, "available:") {
		t.Fatalf("error = %q, expected available list", msg)
	}
}

func TestSplitList(t *testing.T) {
	got := SplitList(" bytes, lines , ,words ")
	want := []string{"bytes", "lines", "words"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("item %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestBuiltins_Computable(t *testing.T) {
	src := []byte("# Title\n\none two three four\n")
	doc := NewDocument("test.md", src)

	defs := ForScope(ScopeFile)
	if len(defs) == 0 {
		t.Fatal("expected file metrics")
	}

	values := make(map[string]Value, len(defs))
	for _, def := range defs {
		v, err := def.Compute(doc)
		if err != nil {
			t.Fatalf("compute(%s): %v", def.Name, err)
		}
		if !v.Available {
			t.Fatalf("metric %s unexpectedly unavailable", def.Name)
		}
		values[def.Name] = v
	}

	if values["bytes"].Number != float64(len(src)) {
		t.Fatalf("bytes = %.0f, want %d", values["bytes"].Number, len(src))
	}
	if values["lines"].Number != 3 {
		t.Fatalf("lines = %.0f, want 3", values["lines"].Number)
	}

	wantTokens := math.Round(values["words"].Number * 0.75)
	if values["token-estimate"].Number != wantTokens {
		t.Fatalf(
			"token-estimate = %.0f, want %.0f",
			values["token-estimate"].Number,
			wantTokens,
		)
	}
}

func TestConciseness_DenseBeatsVerbose(t *testing.T) {
	def, ok := LookupScope(ScopeFile, "conciseness")
	if !ok {
		t.Fatal("conciseness metric not found")
	}

	verbose := []byte(
		"In order to make sure we are on the same page, it is important to note " +
			"that we might update this process in most cases.\n",
	)
	dense := []byte(
		"The synchronization algorithm enforces linearizability " +
			"via monotonic commit indices.\n",
	)

	verboseVal, err := def.Compute(NewDocument("verbose.md", verbose))
	if err != nil {
		t.Fatalf("verbose conciseness: %v", err)
	}
	denseVal, err := def.Compute(NewDocument("dense.md", dense))
	if err != nil {
		t.Fatalf("dense conciseness: %v", err)
	}

	if denseVal.Number <= verboseVal.Number {
		t.Fatalf(
			"dense score %.1f should be greater than verbose %.1f",
			denseVal.Number,
			verboseVal.Number,
		)
	}
}
