package cuelitetest

import (
	stderrors "errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
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
	t.Run("equal normalizes unsorted paths without mutating", func(t *testing.T) {
		// Equal must compare sorted copies so an Outcome built directly by a
		// later phase (not through validatePaths, which sorts) cannot produce
		// an order-sensitive comparison. The inputs must be left untouched.
		aPaths := [][]string{{"b"}, {"a"}}
		bPaths := [][]string{{"a"}, {"b"}}
		a := Outcome{Stage: StageValidate, Paths: aPaths}
		b := Outcome{Stage: StageValidate, Paths: bPaths}
		assert.True(t, a.Equal(b))
		// No mutation: the original (unsorted) slice order survives.
		assert.Equal(t, [][]string{{"b"}, {"a"}}, aPaths)
		assert.Equal(t, [][]string{{"a"}, {"b"}}, bPaths)
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

// TestRawDuplicateKeys exercises the oracle's independent duplicate-key
// walk directly, so its recursive descent is pinned apart from the
// corpus run. It is the oracle counterpart to cuelite's
// checkDuplicateJSONKeys tests: both implementations of the same
// strict-JSON contract are unit-tested on the same shapes.
func TestRawDuplicateKeys(t *testing.T) {
	cases := []struct {
		name string
		json string
		dup  bool
	}{
		{"conflicting dup", `{"a":1,"a":2}`, true},
		{"mergeable dup", `{"a":{"b":1},"a":{"c":2}}`, true},
		{"equal dup", `{"a":1,"a":1}`, true},
		{"nested dup", `{"x":{"a":1,"a":1}}`, true},
		{"dup in array element", `[{"a":1,"a":1}]`, true},
		{"deep array dup", `{"a":[[{"k":1,"k":2}]]}`, true},
		{"dup after nested object value", `{"a":{"x":1},"a":2}`, true},
		{"same key different objects ok", `{"x":{"a":1},"y":{"a":2}}`, false},
		{"sibling objects ok", `[{"a":1},{"a":2}]`, false},
		{"scalars ok", `{"a":1,"b":2}`, false},
		{"empty object ok", `{}`, false},
		{"empty array ok", `[]`, false},
		{"top-level scalar ok", `42`, false},
		{"malformed defers to extract", `{not json`, false},
		{"whitespace only defers", `   `, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := rawDuplicateKeys([]byte(tc.json))
			if tc.dup {
				require.Error(t, err, "expected duplicate error for %s", tc.json)
				assert.Contains(t, err.Error(), `duplicate JSON key`)
			} else {
				assert.NoError(t, err, "unexpected error for %s", tc.json)
			}
		})
	}
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
		require.Len(t, r.failures, 1)
		// The recorder renders the format with its args, so the captured
		// failure names the disagreeing case rather than a bare format string.
		assert.Contains(t, r.failures[0], `case "mismatch"`)
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
		{Name: "conflicting duplicate key reject", Schema: `{a: int}`, Data: `{"a":1,"a":2}`},
		// A MERGEABLE duplicate key. CUE's lift unifies same-named object
		// keys into a phantom merged object, so without the independent
		// duplicate-key check in oracleData the oracle would accept this while
		// the cuelite arm rejects it at StageCompileData — a phantom
		// divergence. Both arms must reject it at the data stage.
		{Name: "mergeable duplicate key reject", Schema: `{a: {b: int, c: int}}`, Data: `{"a":{"b":1},"a":{"c":2}}`},
	}
}
