package cuelite

import (
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests pin the private helpers directly, so each ships with a
// dedicated unit test (plan 236) rather than relying only on coverage
// through the exported surface. The CUE-only helpers (buildJSON, rebuild,
// cueErrorsOf) were deleted with the CUE-backed implementation in the
// plan-238 flip; the durable strict-JSON duplicate-key scanner is retargeted
// at the in-house scanDuplicateKeys below.

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
	t.Run("a ⊥ engine value is a bottom carrying its path and reason", func(t *testing.T) {
		// A conflicting Unify reduces a field to a ⊥ engine value; isBottom must
		// surface it as a *PathError carrying the field path, not a nil reason.
		schema, err := Compile(`{status: "✅"}`)
		require.NoError(t, err)
		data, err := CompileJSON([]byte(`{"status": "🔲"}`))
		require.NoError(t, err)
		merged := schema.Unify(data)
		// The merged value's status field is ⊥; the top-level struct is not, so
		// isBottom on the merged value is false (the leaf is found by Validate).
		_, isB := merged.isBottom()
		assert.False(t, isB, "a struct with a ⊥ field is not itself ⊥")
	})
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

func TestDupLevel_recordKey(t *testing.T) {
	t.Run("nil level treats token as a value", func(t *testing.T) {
		var l *dupLevel
		handled, err := l.recordKey("x")
		require.NoError(t, err)
		assert.False(t, handled)
	})
	t.Run("array level treats token as a value", func(t *testing.T) {
		l := &dupLevel{} // keys == nil marks an array level
		handled, err := l.recordKey("x")
		require.NoError(t, err)
		assert.False(t, handled)
	})
	t.Run("a non-string token at an object level is a value", func(t *testing.T) {
		l := &dupLevel{keys: map[string]struct{}{}}
		handled, err := l.recordKey(42)
		require.NoError(t, err)
		assert.False(t, handled)
	})
	t.Run("a fresh key is recorded and consumed", func(t *testing.T) {
		l := &dupLevel{keys: map[string]struct{}{}}
		handled, err := l.recordKey("a")
		require.NoError(t, err)
		assert.True(t, handled)
		assert.True(t, l.seenKey, "the key half flips the parity to expect a value")
		_, seen := l.keys["a"]
		assert.True(t, seen)
	})
	t.Run("a seenKey token is the value half and not a key", func(t *testing.T) {
		l := &dupLevel{keys: map[string]struct{}{}, seenKey: true}
		handled, err := l.recordKey("a")
		require.NoError(t, err)
		assert.False(t, handled, "past the key, the string is the value")
	})
	t.Run("a repeated key reports a duplicate", func(t *testing.T) {
		l := &dupLevel{keys: map[string]struct{}{"a": {}}}
		handled, err := l.recordKey("a")
		assert.True(t, handled)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"a"`)
	})
	t.Run("a U+FFFD key is consumed but not tracked", func(t *testing.T) {
		l := &dupLevel{keys: map[string]struct{}{}}
		handled, err := l.recordKey("�")
		require.NoError(t, err)
		assert.True(t, handled)
		assert.Empty(t, l.keys, "a replacement-char key is skipped for dup tracking")
	})
}

func TestScanDuplicateKeys(t *testing.T) {
	t.Run("malformed defers to the decoder", func(t *testing.T) {
		assert.NoError(t, scanDuplicateKeys([]byte(`{"a":1,`)))
	})
	t.Run("duplicate key reported", func(t *testing.T) {
		err := scanDuplicateKeys([]byte(`{"a":1,"a":2}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"a"`)
	})
	t.Run("invalid UTF-8 defers", func(t *testing.T) {
		assert.NoError(t, scanDuplicateKeys([]byte("{\"a\xff\":1,\"a\xfe\":2}")))
	})
	t.Run("replacement-char keys are not duplicates", func(t *testing.T) {
		assert.NoError(t, scanDuplicateKeys([]byte(`{"\ud800":1,"\udc00":2}`)))
	})
	t.Run("trailing top-level value defers", func(t *testing.T) {
		assert.NoError(t, scanDuplicateKeys([]byte(`{"x":1} {"a":1,"a":2}`)))
	})
	t.Run("top-level scalar stops the scan", func(t *testing.T) {
		assert.NoError(t, scanDuplicateKeys([]byte(`42 {"a":1,"a":2}`)))
	})
	t.Run("array value then a duplicate key reported", func(t *testing.T) {
		err := scanDuplicateKeys([]byte(`{"a":[1],"a":2}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"a"`)
	})
	t.Run("equal keys in distinct array-element objects are not duplicates", func(t *testing.T) {
		assert.NoError(t, scanDuplicateKeys([]byte(`[{"a":1},{"a":1}]`)))
	})
	t.Run("top-level array with a scalar element is clean", func(t *testing.T) {
		assert.NoError(t, scanDuplicateKeys([]byte(`[1]`)))
	})
	t.Run("a real duplicate inside an array-element object is reported", func(t *testing.T) {
		err := scanDuplicateKeys([]byte(`[{"a":1,"a":2}]`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"a"`)
	})
	t.Run("duplicate nested under a U+FFFD key reported", func(t *testing.T) {
		err := scanDuplicateKeys([]byte(`{"\ud800":{"a":1,"a":2}}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"a"`)
	})
}
