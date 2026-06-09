package cuelitetest

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/errors"

	"github.com/jeduden/mdsmith/cue/cuelite"
)

// benchCase is the representative schema-plus-data the benchmarks and
// their correctness guard all run: a small front-matter-shaped struct
// with a string literal, a bounded int, and a regex-constrained slug —
// the constraint atoms MDS020 and the query surface actually use.
func benchCase() Case {
	return Case{
		Name:   "front matter",
		Schema: `{status: "✅", weight: >=0 & <=100, slug: =~"^[a-z-]+$"}`,
		Data:   `{"status": "✅", "weight": 42, "slug": "release-gating"}`,
	}
}

// TestBenchCaseAccepted guards the benchmarks: the in-house and oracle
// paths must agree (and accept) on benchCase, so the benchmark arms
// measure the same accepting workload. It reuses the harness's Compare
// rather than hand-rolling an agreement check, so the guard and the CI
// differential run share one definition of agreement.
func TestBenchCaseAccepted(t *testing.T) {
	c := benchCase()
	if !Compare(t, CueLitePath, OraclePath, c) {
		t.Fatal("benchmark case must agree across paths")
	}
	if !CueLitePath(c).Accepted() {
		t.Fatal("benchmark case must be accepted")
	}
}

// BenchmarkCompileValidate measures the cold path — compile schema,
// compile data, unify, validate, every iteration — of the cuelite façade
// against the direct CUE oracle. Both arms pay one context per operation
// (cuelite per Value, the oracle per call), so the comparison is
// symmetric and honest.
func BenchmarkCompileValidate(b *testing.B) {
	c := benchCase()
	b.Run("cuelite", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if !CueLitePath(c).Accepted() {
				b.Fatal("cuelite path rejected the benchmark case")
			}
		}
	})
	b.Run("cue", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if !OraclePath(c).Accepted() {
				b.Fatal("oracle path rejected the benchmark case")
			}
		}
	})
}

// BenchmarkValidate measures the hot path the phase-2/4 flip is judged
// against: one schema compiled once, then many documents validated
// against it. The schema compile is hoisted out of the timed loop in
// both arms, so the loop measures data compile + unify + validate only.
func BenchmarkValidate(b *testing.B) {
	c := benchCase()
	b.Run("cuelite", func(b *testing.B) {
		schema, err := cuelite.Compile(c.Schema)
		if err != nil {
			b.Fatal(err)
		}
		dataBytes := []byte(c.Data)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			data, err := cuelite.CompileJSON(dataBytes)
			if err != nil {
				b.Fatal(err)
			}
			if schema.Unify(data).Validate() != nil {
				b.Fatal("cuelite path rejected the benchmark case")
			}
		}
	})
	b.Run("cue", func(b *testing.B) {
		ctx := cuecontext.New()
		schema := ctx.CompileString(c.Schema)
		if schema.Err() != nil {
			b.Fatal(schema.Err())
		}
		// Hoist the []byte conversion out of the timed loop so this arm
		// stays symmetric with the cuelite arm, which times CompileJSON over
		// a pre-built dataBytes.
		dataBytes := []byte(c.Data)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			data, err := oracleData(ctx, dataBytes)
			if err != nil {
				b.Fatal(err)
			}
			if verr := schema.Unify(data).Validate(cue.Concrete(true)); verr != nil {
				b.Fatal(errors.Errors(verr)[0])
			}
		}
	})
}
