package cuelite

import "testing"

// bench_test.go holds the row/schema benchmarks and the absolute performance
// guard that replaced the deleted internal/cuelitetest factor gate (plan 240).
// The factor gate compared the in-house engine's speed against the cuelang
// oracle (a relative factor); with cuelang gone there is no oracle to divide
// by, so the regression guard is now ABSOLUTE: the schema validate path must
// stay within a fixed allocs/op ceiling. This keeps the performance regression
// the factor gate guarded against without the oracle.

// benchSchema and benchData are a representative schema/data pair: a closed
// struct with a bound, a regex, and a defaulted disjunction — the shapes the
// real MDS020 hot path validates. The data is a map[string]any, the form the
// front-matter decoder produces, so the benchmark hits CompileMap's
// JSON-round-trip-free hot path rather than CompileJSON.
const benchSchemaSrc = `close({n: int & >=0, slug: =~"^[a-z]+$", m: string | *""})`

func benchData() map[string]any {
	return map[string]any{"n": int64(3), "slug": "abc", "m": "x"}
}

// maxValidateAllocs is the absolute allocs/op ceiling for the compile-once,
// validate-many hot path: a compiled schema validating a fresh front-matter
// map. The ceiling catches a regression (an accidental per-validate
// allocation) without depending on a CUE oracle to compare against. Measured
// well under this with headroom for map-iteration ordering noise.
const maxValidateAllocs = 60

// TestValidateAllocBudget is the absolute allocation-regression guard. It
// compiles the schema once (the real hot path compiles per unique schema, not
// per file) and measures the allocs of the per-data CompileMap+Validate step,
// the work MDS020 repeats for every file sharing a schema.
func TestValidateAllocBudget(t *testing.T) {
	schema, err := Compile(benchSchemaSrc)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	data := benchData()
	avg := testing.AllocsPerRun(200, func() {
		_ = schema.CompileMap(data).Validate()
	})
	if avg > maxValidateAllocs {
		t.Errorf("validate hot path allocates %.0f/op, over the %d ceiling", avg, maxValidateAllocs)
	}
}

// BenchmarkCompileValidate measures the full compile-schema, compile-data,
// unify, validate path.
func BenchmarkCompileValidate(b *testing.B) {
	data := benchData()
	for b.Loop() {
		schema, err := Compile(benchSchemaSrc)
		if err != nil {
			b.Fatal(err)
		}
		_ = schema.CompileMap(data).Validate()
	}
}

// BenchmarkValidate measures the compile-once, validate-many hot path: the
// schema is compiled once and only the per-data CompileMap+Validate is timed.
func BenchmarkValidate(b *testing.B) {
	schema, err := Compile(benchSchemaSrc)
	if err != nil {
		b.Fatal(err)
	}
	data := benchData()
	b.ResetTimer()
	for b.Loop() {
		_ = schema.CompileMap(data).Validate()
	}
}

// BenchmarkRenderRow measures the compile-once, render-many row-expression hot
// path the catalog uses.
func BenchmarkRenderRow(b *testing.B) {
	tpl, err := CompileRow(`"\(id) - \(name)"`)
	if err != nil {
		b.Fatal(err)
	}
	scope := map[string]any{"id": "MDS001", "name": "rule-name"}
	b.ResetTimer()
	for b.Loop() {
		if _, err := tpl.Render(scope); err != nil {
			b.Fatal(err)
		}
	}
}
