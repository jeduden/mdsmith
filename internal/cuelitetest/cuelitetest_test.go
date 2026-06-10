package cuelitetest

import (
	stderrors "errors"
	"fmt"
	"testing"

	"cuelang.org/go/cue/cuecontext"
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
	t.Run("same non-validate stage with matching paths is equal", func(t *testing.T) {
		// Paths are compared at every stage, so two StageAccepted outcomes
		// agree only when their paths agree too.
		a := Outcome{Stage: StageAccepted, Paths: [][]string{{"x"}}}
		b := Outcome{Stage: StageAccepted, Paths: [][]string{{"x"}}}
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
	t.Run("different paths at a non-validate stage differ", func(t *testing.T) {
		// A future payload at another stage (surface D's parsed segments at
		// StageAccepted) must be compared, not silently always-equal. Same
		// stage, different Paths → not equal.
		a := Outcome{Stage: StageAccepted, Paths: [][]string{{"a", "b"}}}
		b := Outcome{Stage: StageAccepted, Paths: [][]string{{"a", "c"}}}
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

// TestPaths pins the ABSOLUTE resolution stage of each path through both
// the in-house and the oracle Path, so the two stay symmetric stage for
// stage. It holds only the compile-stage rows the corpus run cannot
// assert — the corpus checks that the two paths AGREE, never which
// absolute stage they resolve at, so a bug that moved both arms to the
// wrong stage in lockstep would pass the corpus but fail here. The
// accept and validate-reject rows that were once here duplicated
// corpus() entries byte for byte and are covered by TestRun_corpus.
func TestPaths(t *testing.T) {
	paths := map[string]Path{"cuelite": CueLitePath, "oracle": OraclePath}
	cases := []struct {
		name  string
		c     Case
		stage Stage
	}{
		{"schema compile error",
			Case{Schema: `{status: =}`, Data: `{"status": "x"}`}, StageCompileSchema},
		{"data compile error",
			Case{Schema: `{status: string}`, Data: `{not json`}, StageCompileData},
		{"non-JSON data rejected at the data stage",
			Case{Schema: `{n: int}`, Data: `{n: 3}`}, StageCompileData},
		{"duplicate-key data rejected at the data stage",
			Case{Schema: `{a: int}`, Data: `{"a":1,"a":2}`}, StageCompileData},
		{"trailing top-level value rejected at the data stage",
			Case{Schema: `{x: int}`, Data: `{"x":1} {"a":1,"a":2}`}, StageCompileData},
	}
	for name, path := range paths {
		for _, tc := range cases {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				assert.Equal(t, tc.stage, path(tc.c).Stage)
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
// scanDuplicateJSONKeys tests: both implementations of the same
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
		// 1e999 overflows float64; UseNumber keeps the walk from misreading
		// it as malformed and deferring, so the duplicate beside it is caught.
		{"dup beside an overflowing number", `{"x":1e999,"a":1,"a":2}`, true},
		{"overflowing number without dup ok", `{"x":1e999}`, false},
		{"same key different objects ok", `{"x":{"a":1},"y":{"a":2}}`, false},
		{"sibling objects ok", `[{"a":1},{"a":2}]`, false},
		{"scalars ok", `{"a":1,"b":2}`, false},
		{"empty object ok", `{}`, false},
		{"empty array ok", `[]`, false},
		{"top-level scalar ok", `42`, false},
		{"malformed defers to extract", `{not json`, false},
		{"whitespace only defers", `   `, false},
		{"lone close brace defers", `}`, false},
		{"lone close bracket defers", `]`, false},
		{"non-string key defers", `{1:2}`, false},
		{"truncated object defers", `{"a":1`, false},
		{"truncated array defers", `["x"`, false},
		{"truncated after key defers", `{"a"`, false},
		// Invalid UTF-8 raw keys fold onto one U+FFFD; the walker must defer
		// to Extract (a utf8.Valid pre-check) rather than fabricate a dup.
		{"invalid UTF-8 keys defer", "{\"a\xff\":1,\"a\xfe\":2}", false},
		// Two lone-surrogate escaped keys decode to the same U+FFFD; the
		// walker must skip dup tracking for U+FFFD keys, not fabricate a dup.
		{"lone-surrogate keys not duplicates", `{"\ud800":1,"\udc00":2}`, false},
		// A U+FFFD key is skipped for dup tracking, but its VALUE is still
		// walked: a real duplicate nested under it must still be caught.
		{"duplicate nested under a U+FFFD key", `{"\ud800":{"a":1,"a":2}}`, true},
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

// TestOracleValidatePaths_emptyDecomposition mirrors cuelite's
// TestJoinValidationErrors_emptyDecomposition through the oracle's own
// seam: an empty errors.Errors decomposition (which a future CUE version
// or a malformed validation error could produce) must NOT yield a
// StageValidate Outcome with zero paths. cuelite's bottom path always
// surfaces ONE nil-path *PathError, so the oracle must surface ONE nil
// path too — otherwise the fail-safe path diverges by phantom (zero vs
// one path) even though both reject.
func TestOracleValidatePaths_emptyDecomposition(t *testing.T) {
	o := oracleValidate(nil)
	require.Equal(t, StageValidate, o.Stage)
	require.Len(t, o.Paths, 1, "an empty decomposition must yield one nil path, not zero")
	assert.Empty(t, o.Paths[0])
}

// TestOracleData pins oracleData's three error branches and its accept
// branch directly, apart from the corpus run: the duplicate-key scan, the
// Extract syntax check, and the BuildExpr bottom (a lone-surrogate value)
// must each surface a data-compile error, while strict JSON builds.
func TestOracleData(t *testing.T) {
	ctx := cuecontext.New()
	t.Run("duplicate key errors before the lift", func(t *testing.T) {
		_, err := oracleData(ctx, []byte(`{"a":1,"a":2}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"a"`)
	})
	t.Run("malformed JSON errors via Extract", func(t *testing.T) {
		_, err := oracleData(ctx, []byte(`{not json`))
		require.Error(t, err)
	})
	t.Run("lone-surrogate value errors at BuildExpr", func(t *testing.T) {
		// Grammar-valid, duplicate-free strict JSON that passes the scan and
		// Extract but builds to a bottom; oracleData must surface it.
		_, err := oracleData(ctx, []byte(`{"a": "\ud800"}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "surrogate")
	})
	t.Run("valid strict JSON builds", func(t *testing.T) {
		val, err := oracleData(ctx, []byte(`{"a": 1}`))
		require.NoError(t, err)
		assert.True(t, val.Exists())
	})
}

// TestValidatePaths pins validatePaths directly: it builds a StageValidate
// Outcome and sorts the paths deterministically in place.
func TestValidatePaths(t *testing.T) {
	o := validatePaths([][]string{{"b"}, {"a"}})
	assert.Equal(t, StageValidate, o.Stage)
	assert.Equal(t, [][]string{{"a"}, {"b"}}, o.Paths,
		"validatePaths sorts the rejecting leaves deterministically")
}

// TestSortedPaths pins sortedPaths directly: it returns a sorted copy and
// leaves its input untouched so Equal can normalize without mutating an
// operand.
func TestSortedPaths(t *testing.T) {
	in := [][]string{{"b"}, {"a"}}
	got := sortedPaths(in)
	assert.Equal(t, [][]string{{"a"}, {"b"}}, got, "the copy is sorted")
	assert.Equal(t, [][]string{{"b"}, {"a"}}, in, "the input is left untouched")
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
		// A lone-surrogate escape is grammar-valid, duplicate-free strict
		// JSON that passes the scanner and cuejson.Extract but builds to a
		// bottom ("invalid string: unmatched surrogate pair"). Both arms must
		// surface that bottom as a data-stage compile error (StageCompileData)
		// rather than one arm accepting a phantom value.
		{Name: "lone-surrogate value reject", Schema: `{a: string, b: int}`, Data: `{"a": "\ud800", "b": "x"}`},
		// 1e999 is valid JSON but overflows float64; without dec.UseNumber()
		// in both walkers, json.Decoder.Token errors on it mid-scan and the
		// mergeable duplicate "a" beside it slips past BOTH arms into a
		// phantom merged object that validates. Both arms must keep scanning
		// and reject the duplicate at the data stage.
		{
			Name:   "big-number duplicate reject",
			Schema: `{a: {b: int, c: int}}`,
			Data:   `{"x":1e999,"a":{"b":1},"a":{"c":2}}`,
		},
		// The same overflowing number with no duplicate key must still be
		// accepted end-to-end by both arms.
		{Name: "big-number no duplicate ok", Schema: `{x: number}`, Data: `{"x":1e999}`},
		// A second top-level value: the cuelite scanner must stop after the
		// first value closes (the oracle's walker already consumes only one),
		// so both arms defer to Extract's "after top-level value" error and
		// resolve at StageCompileData rather than fabricating a duplicate "a".
		{Name: "trailing top-level value reject", Schema: `{x: int}`, Data: `{"x":1} {"a":1,"a":2}`},
		// Two distinct invalid-byte raw keys both decode to U+FFFD; without a
		// utf8.Valid pre-check in both walkers, one arm fabricates a duplicate
		// while the other defers to Extract — a divergence. Both arms must
		// defer such input to Extract.
		{Name: "invalid-UTF-8 keys defer", Schema: `{a: _}`, Data: "{\"a\xff\":1,\"a\xfe\":2}"},
		// Two distinct lone-surrogate escaped keys decode to the same U+FFFD;
		// both walkers must skip dup tracking for U+FFFD keys, so neither
		// fabricates a duplicate. (The build then rejects the surrogate via
		// the val.Err() guard, so both arms resolve at StageCompileData.)
		{Name: "lone-surrogate keys not duplicates", Schema: `{a: _}`, Data: `{"\ud800":1,"\udc00":2}`},
		// A string VALUE that equals its own key. The seenKey parity guard
		// must not misread the value as a key; deleting it fabricates a
		// duplicate in the cuelite arm and the two arms diverge.
		{Name: "value equal to own key ok", Schema: `{a: string}`, Data: `{"a":"a"}`},
		// A string value that equals a LATER sibling key.
		{Name: "value equal to sibling key ok", Schema: `{x: string, y: int}`, Data: `{"x":"y","y":1}`},
		// A U+FFFD key is skipped for dup tracking, but its VALUE subtree must
		// still be walked in BOTH arms: a real duplicate nested under a
		// lone-surrogate key is caught at the data stage. An arm that skipped
		// the whole subtree after a lossy key would accept this and diverge.
		{Name: "duplicate nested under a lossy key reject", Schema: `{a: _}`, Data: `{"\ud800":{"a":1,"a":2}}`},
	}
}
