package cuelite

import (
	"encoding/json"
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
		pe := newPathError([]string{"a"}, "boom", nil)
		out := collectPathErrors(pe, nil, map[error]struct{}{})
		require.Len(t, out, 1)
		assert.Same(t, pe, out[0])
	})
	t.Run("an already-visited node is skipped", func(t *testing.T) {
		pe := newPathError([]string{"a"}, "boom", nil)
		visited := map[error]struct{}{pe: {}}
		out := collectPathErrors(pe, nil, visited)
		assert.Empty(t, out, "a node in visited must not be re-collected")
	})
}

func TestJSONLevel_recordKey(t *testing.T) {
	t.Run("nil level treats token as a value", func(t *testing.T) {
		// A top-level token has no object level; it is never a key.
		var l *jsonLevel
		handled, err := l.recordKey("x")
		require.NoError(t, err)
		assert.False(t, handled)
	})
	t.Run("array level treats token as a value", func(t *testing.T) {
		l := &jsonLevel{} // keys == nil marks an array level
		handled, err := l.recordKey("x")
		require.NoError(t, err)
		assert.False(t, handled)
	})
	t.Run("a non-string token at an object level is a value", func(t *testing.T) {
		l := &jsonLevel{keys: map[string]struct{}{}}
		handled, err := l.recordKey(json.Delim('{'))
		require.NoError(t, err)
		assert.False(t, handled)
	})
	t.Run("a fresh key is recorded and consumed", func(t *testing.T) {
		l := &jsonLevel{keys: map[string]struct{}{}}
		handled, err := l.recordKey("a")
		require.NoError(t, err)
		assert.True(t, handled)
		assert.True(t, l.seenKey, "the key half flips the parity to expect a value")
		_, seen := l.keys["a"]
		assert.True(t, seen)
	})
	t.Run("a seenKey token is the value half and not a key", func(t *testing.T) {
		l := &jsonLevel{keys: map[string]struct{}{}, seenKey: true}
		handled, err := l.recordKey("a")
		require.NoError(t, err)
		assert.False(t, handled, "past the key, the string is the value")
	})
	t.Run("a repeated key reports a duplicate", func(t *testing.T) {
		l := &jsonLevel{keys: map[string]struct{}{"a": {}}}
		handled, err := l.recordKey("a")
		assert.True(t, handled)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"a"`)
	})
	t.Run("a U+FFFD key is consumed but not tracked", func(t *testing.T) {
		l := &jsonLevel{keys: map[string]struct{}{}}
		handled, err := l.recordKey("�")
		require.NoError(t, err)
		assert.True(t, handled)
		assert.Empty(t, l.keys, "a replacement-char key is skipped for dup tracking")
	})
}

func TestScanDuplicateJSONKeys(t *testing.T) {
	t.Run("malformed defers to Extract", func(t *testing.T) {
		// A token error mid-stream (truncated document) leaves detection to
		// cuejson.Extract: the scan returns nil rather than a duplicate error.
		assert.NoError(t, scanDuplicateJSONKeys([]byte(`{"a":1,`)))
	})
	t.Run("duplicate key reported", func(t *testing.T) {
		err := scanDuplicateJSONKeys([]byte(`{"a":1,"a":2}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"a"`)
	})
	t.Run("invalid UTF-8 defers", func(t *testing.T) {
		// Two distinct invalid-byte keys would fold onto one U+FFFD; the
		// utf8.Valid pre-check returns nil so Extract handles the input.
		assert.NoError(t, scanDuplicateJSONKeys([]byte("{\"a\xff\":1,\"a\xfe\":2}")))
	})
	t.Run("replacement-char keys are not duplicates", func(t *testing.T) {
		// Two lone-surrogate escapes decode to the same U+FFFD; the scanner
		// skips dup tracking for them rather than fabricating a duplicate.
		assert.NoError(t, scanDuplicateJSONKeys([]byte(`{"\ud800":1,"\udc00":2}`)))
	})
	t.Run("trailing top-level value defers", func(t *testing.T) {
		// The scan stops after the first value closes; the second top-level
		// object is left to Extract rather than fabricating a duplicate.
		assert.NoError(t, scanDuplicateJSONKeys([]byte(`{"x":1} {"a":1,"a":2}`)))
	})
	t.Run("top-level scalar stops the scan", func(t *testing.T) {
		// A bare top-level scalar is the whole first value with no open
		// container, so the scan returns immediately and leaves any trailing
		// data (a fabricated second value) to Extract.
		assert.NoError(t, scanDuplicateJSONKeys([]byte(`42 {"a":1,"a":2}`)))
	})
	t.Run("duplicate nested under a U+FFFD key reported", func(t *testing.T) {
		// A U+FFFD key is skipped for dup tracking, but its VALUE subtree must
		// still be walked: a real duplicate inside the object the lossy key
		// maps to is caught. Skipping the whole subtree after a lossy key —
		// rather than only the key's own dup tracking — would miss this, so
		// this pins that recordKey consumes only the key, not the value.
		err := scanDuplicateJSONKeys([]byte(`{"\ud800":{"a":1,"a":2}}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"a"`)
	})
}
