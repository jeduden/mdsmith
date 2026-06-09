package cuelite

import (
	stderrors "errors"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	cueerrors "cuelang.org/go/cue/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests pin the private helpers directly, so each ships with a
// dedicated unit test (plan 236) rather than relying only on coverage
// through the exported surface.

func TestBottom(t *testing.T) {
	sentinel := stderrors.New("boom")
	v := bottom(sentinel)
	reason, isB := v.isBottom()
	require.True(t, isB, "a bottom Value must report itself as bottom")
	assert.Same(t, sentinel, reason, "bottom carries the exact error it was built from")
}

func TestIsBottom(t *testing.T) {
	t.Run("error-carrying Value is a bottom", func(t *testing.T) {
		sentinel := stderrors.New("x")
		reason, isB := bottom(sentinel).isBottom()
		require.True(t, isB)
		assert.Same(t, sentinel, reason)
	})
	t.Run("zero Value is a bottom with errZeroValue", func(t *testing.T) {
		reason, isB := Value{}.isBottom()
		require.True(t, isB)
		assert.Same(t, errZeroValue, reason)
	})
	t.Run("compiled Value is not a bottom", func(t *testing.T) {
		v, err := Compile(`{a: string}`)
		require.NoError(t, err)
		reason, isB := v.isBottom()
		assert.False(t, isB)
		assert.NoError(t, reason)
	})
}

func TestBuildJSON(t *testing.T) {
	ctx := cuecontext.New()
	t.Run("valid strict JSON builds", func(t *testing.T) {
		val, err := buildJSON(ctx, []byte(`{"a": 1}`))
		require.NoError(t, err)
		assert.True(t, val.Exists())
	})
	t.Run("malformed JSON errors via Extract", func(t *testing.T) {
		_, err := buildJSON(ctx, []byte(`{not json`))
		require.Error(t, err)
	})
	t.Run("duplicate key errors before the lift", func(t *testing.T) {
		_, err := buildJSON(ctx, []byte(`{"a":1,"a":2}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"a"`)
	})
}

func TestRebuild(t *testing.T) {
	t.Run("operand already in ctx is returned directly", func(t *testing.T) {
		v, err := Compile(`{a: string}`)
		require.NoError(t, err)
		// rebuild into v's OWN context returns v.val unchanged, no recompile.
		got, ok := v.rebuild(v.val.Context())
		require.True(t, ok)
		assert.Equal(t, v.val, got)
	})
	t.Run("cross-context source-carrying operand recompiles", func(t *testing.T) {
		v, err := Compile(`{a: string}`)
		require.NoError(t, err)
		other, err := Compile(`{b: int}`)
		require.NoError(t, err)
		got, ok := v.rebuild(other.val.Context())
		require.True(t, ok, "a source-carrying Value rebuilds into a foreign context")
		assert.True(t, got.Exists())
		assert.Equal(t, other.val.Context(), got.Context())
	})
	t.Run("sourceless derived operand cannot rebuild", func(t *testing.T) {
		a, err := Compile(`{a: string}`)
		require.NoError(t, err)
		b, err := CompileJSON([]byte(`{"a": "x"}`))
		require.NoError(t, err)
		derived := a.Unify(b) // a Unify result retains no source
		require.False(t, derived.hasSrc)
		other, err := Compile(`{c: int}`)
		require.NoError(t, err)
		_, ok := derived.rebuild(other.val.Context())
		assert.False(t, ok, "a sourceless derived operand has no source to rebuild from")
	})
}

func TestPathErrorOf(t *testing.T) {
	// A real cue/errors.Error: pathErrorOf must use its path-free Msg (so
	// the path prints once) and wrap it (so errors.As reaches the CUE error).
	schema, err := Compile(`{status: "✅"}`)
	require.NoError(t, err)
	data, err := CompileJSON([]byte(`{"status": "🔲"}`))
	require.NoError(t, err)
	verr := schema.val.Unify(data.val).Validate()
	require.Error(t, verr)
	leaves := cueErrorsOf(verr)
	require.NotEmpty(t, leaves)

	pe := pathErrorOf(leaves[0])
	assert.Equal(t, leaves[0].Path(), pe.Path())
	var cueErr cueerrors.Error
	assert.True(t, stderrors.As(pe, &cueErr), "pathErrorOf must wrap the cue/errors error")
}

// cueErrorsOf decomposes a cue validation error into its leaves, used by
// TestPathErrorOf to obtain a real errors.Error to convert.
func cueErrorsOf(verr error) []cueerrors.Error {
	return cueerrors.Errors(verr)
}

func TestCollectPathErrors(t *testing.T) {
	t.Run("nil error contributes nothing", func(t *testing.T) {
		assert.Nil(t, collectPathErrors(nil, nil, map[error]struct{}{}))
	})
	t.Run("a foreign error contributes nothing", func(t *testing.T) {
		out := collectPathErrors(stderrors.New("foreign"), nil, map[error]struct{}{})
		assert.Nil(t, out)
	})
	t.Run("a bare PathError is appended as a leaf", func(t *testing.T) {
		pe := newPathError([]string{"a"}, "boom")
		out := collectPathErrors(pe, nil, map[error]struct{}{})
		require.Len(t, out, 1)
		assert.Same(t, pe, out[0])
	})
	t.Run("an already-visited node is skipped", func(t *testing.T) {
		pe := newPathError([]string{"a"}, "boom")
		visited := map[error]struct{}{pe: {}}
		out := collectPathErrors(pe, nil, visited)
		assert.Empty(t, out, "a node in visited must not be re-collected")
	})
}

func TestScanDuplicateJSONKeys_malformed(t *testing.T) {
	// A token error mid-stream (truncated document) leaves detection to
	// cuejson.Extract: the scan returns nil rather than a duplicate error.
	assert.NoError(t, checkDuplicateJSONKeys([]byte(`{"a":1,`)))
}
