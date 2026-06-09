package cuelitetest

import (
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// recorder is a testing.TB that captures failures instead of failing, so
// the harness's own disagreement path can be asserted. It embeds
// testing.TB to satisfy the sealed interface; only the methods the
// harness calls are overridden, and any other call would panic on the
// nil embed — which the harness never makes.
type recorder struct {
	testing.TB
	helperCalls int
	failures    []string
}

func (r *recorder) Helper() { r.helperCalls++ }

func (r *recorder) Errorf(format string, args ...any) {
	r.failures = append(r.failures, format)
}

func TestOutcome_Accepted(t *testing.T) {
	assert.True(t, Outcome{Stage: StageAccepted}.Accepted())
	assert.False(t, Outcome{Stage: StageValidate}.Accepted())
}

func TestOutcome_Equal(t *testing.T) {
	t.Run("same non-validate stage ignores paths", func(t *testing.T) {
		a := Outcome{Stage: StageAccepted, Paths: [][]string{{"x"}}}
		b := Outcome{Stage: StageAccepted}
		assert.True(t, a.Equal(b))
	})
	t.Run("different stage differs", func(t *testing.T) {
		assert.False(t, Outcome{Stage: StageAccepted}.Equal(Outcome{Stage: StageValidate}))
	})
	t.Run("compile-schema and compile-data differ", func(t *testing.T) {
		// The whole point of the Stage discriminator: a schema the engine
		// could not parse must not look like a data rejection.
		assert.False(t,
			Outcome{Stage: StageCompileSchema}.Equal(Outcome{Stage: StageCompileData}))
	})
	t.Run("validate reject with same paths are equal", func(t *testing.T) {
		a := Outcome{Stage: StageValidate, Paths: [][]string{{"status"}}}
		b := Outcome{Stage: StageValidate, Paths: [][]string{{"status"}}}
		assert.True(t, a.Equal(b))
	})
	t.Run("validate reject with different path differ", func(t *testing.T) {
		a := Outcome{Stage: StageValidate, Paths: [][]string{{"status"}}}
		b := Outcome{Stage: StageValidate, Paths: [][]string{{"title"}}}
		assert.False(t, a.Equal(b))
	})
	t.Run("dropping a later leaf differs", func(t *testing.T) {
		// The reason Outcome carries every path: an engine that gets the
		// first leaf right but drops the second must not look equal.
		a := Outcome{Stage: StageValidate, Paths: [][]string{{"a"}, {"b"}}}
		b := Outcome{Stage: StageValidate, Paths: [][]string{{"a"}}}
		assert.False(t, a.Equal(b))
	})
	t.Run("nil paths equals empty paths", func(t *testing.T) {
		a := Outcome{Stage: StageValidate, Paths: nil}
		b := Outcome{Stage: StageValidate, Paths: [][]string{}}
		assert.True(t, a.Equal(b))
	})
}

func TestStage_String(t *testing.T) {
	cases := []struct {
		stage Stage
		want  string
	}{
		{StageAccepted, "Accepted"},
		{StageCompileSchema, "CompileSchema"},
		{StageCompileData, "CompileData"},
		{StageValidate, "Validate"},
		{StageError, "Error"},
		{Stage(99), "Stage(99)"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, tc.stage.String())
	}
}

// TestPaths exercises both the in-house and the oracle Path through the
// same table, so the two stay symmetric stage for stage.
func TestPaths(t *testing.T) {
	paths := map[string]Path{"cuelite": CueLitePath, "oracle": OraclePath}
	cases := []struct {
		name  string
		c     Case
		stage Stage
		paths [][]string
	}{
		{"accepts conforming data",
			Case{Schema: `{status: string}`, Data: `{"status": "done"}`}, StageAccepted, nil},
		{"validate reject carries field path",
			Case{Schema: `{status: "✅"}`, Data: `{"status": "🔲"}`}, StageValidate, [][]string{{"status"}}},
		{"multi-leaf reject carries every sorted path",
			Case{Schema: `{a: "x", b: "y"}`, Data: `{"a": "p", "b": "q"}`}, StageValidate, [][]string{{"a"}, {"b"}}},
		{"schema compile error",
			Case{Schema: `{status: =}`, Data: `{"status": "x"}`}, StageCompileSchema, nil},
		{"data compile error",
			Case{Schema: `{status: string}`, Data: `{not json`}, StageCompileData, nil},
		{"non-JSON data rejected at the data stage",
			Case{Schema: `{n: int}`, Data: `{n: 3}`}, StageCompileData, nil},
		{"duplicate-key data rejected at the data stage",
			Case{Schema: `{a: int}`, Data: `{"a":1,"a":2}`}, StageCompileData, nil},
	}
	for name, path := range paths {
		for _, tc := range cases {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				o := path(tc.c)
				assert.Equal(t, tc.stage, o.Stage)
				if tc.stage == StageValidate {
					assert.Equal(t, tc.paths, o.Paths)
				}
			})
		}
	}
}

func TestValidateOutcome(t *testing.T) {
	t.Run("nil error accepts", func(t *testing.T) {
		assert.Equal(t, StageAccepted, validateOutcome(nil).Stage)
	})
	t.Run("non-PathError records StageError", func(t *testing.T) {
		// A future engine bug returning some other error shape must not
		// panic the harness; it records a diff-able StageError instead.
		o := validateOutcome(stderrors.New("not a path error"))
		assert.Equal(t, StageError, o.Stage)
	})
}

func TestCompare(t *testing.T) {
	t.Run("agreement records no failure", func(t *testing.T) {
		r := &recorder{}
		ok := Compare(r, CueLitePath, OraclePath, Case{Schema: `{a: string}`, Data: `{"a": "x"}`})
		assert.True(t, ok)
		assert.Empty(t, r.failures)
		assert.Positive(t, r.helperCalls)
	})
	t.Run("disagreement records a failure", func(t *testing.T) {
		r := &recorder{}
		accept := func(Case) Outcome { return Outcome{Stage: StageAccepted} }
		reject := func(Case) Outcome { return Outcome{Stage: StageValidate} }
		ok := Compare(r, accept, reject, Case{Name: "mismatch"})
		assert.False(t, ok)
		assert.Len(t, r.failures, 1)
	})
}

func TestRun(t *testing.T) {
	r := &recorder{}
	Run(r, []Case{
		{Name: "ok", Schema: `{a: string}`, Data: `{"a": "x"}`},
		{Name: "bad", Schema: `{a: "✅"}`, Data: `{"a": "🔲"}`},
	})
	assert.Empty(t, r.failures)
	assert.Positive(t, r.helperCalls)
}

// TestRun_corpus is the CI-visible differential run: the in-house path
// and the oracle must agree on every case in the representative corpus.
func TestRun_corpus(t *testing.T) {
	Run(t, corpus())
}

// corpus is a representative set of schema/data cases spanning accept,
// scalar-mismatch reject, and nested-field reject.
func corpus() []Case {
	return []Case{
		{Name: "string ok", Schema: `{status: string}`, Data: `{"status": "done"}`},
		{Name: "int bound ok", Schema: `{n: >=0}`, Data: `{"n": 3}`},
		{Name: "int bound reject", Schema: `{n: >=0}`, Data: `{"n": -1}`},
		{Name: "literal reject", Schema: `{status: "✅"}`, Data: `{"status": "🔲"}`},
		{Name: "regex ok", Schema: `{slug: =~"^[a-z]+$"}`, Data: `{"slug": "abc"}`},
		{Name: "regex reject", Schema: `{slug: =~"^[a-z]+$"}`, Data: `{"slug": "AB1"}`},
		{Name: "nested reject", Schema: `{meta: {status: "✅"}}`, Data: `{"meta": {"status": "x"}}`},
		{Name: "multi-leaf reject", Schema: `{a: "x", b: "y"}`, Data: `{"a": "p", "b": "q"}`},
		{Name: "duplicate key reject", Schema: `{a: int}`, Data: `{"a":1,"a":2}`},
	}
}
