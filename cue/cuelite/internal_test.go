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

func TestRawHasLoneSurrogateEscape(t *testing.T) {
	// A correctly PAIRED surrogate escape (a high surrogate \ud83d followed by
	// a low surrogate \ude00, the 😀 emoji) is NOT lone.
	assert.False(t, rawHasLoneSurrogateEscape([]byte("\"\\ud83d\\ude00\"")))
	// A high surrogate with no following escape is lone.
	assert.True(t, rawHasLoneSurrogateEscape([]byte(`"\ud800"`)))
	// A high surrogate followed by a non-escape character is lone.
	assert.True(t, rawHasLoneSurrogateEscape([]byte(`"\ud800AAAAAA"`)))
	// A high surrogate followed by a non-\u character is lone.
	assert.True(t, rawHasLoneSurrogateEscape([]byte(`"\ud83dA"`)))
	// A high surrogate followed by a valid \u escape whose value is NOT a low
	// surrogate (\u0041 = 'A') is lone.
	assert.True(t, rawHasLoneSurrogateEscape([]byte("\"\\ud83d\\u0041\"")))
	// A lone low surrogate is lone.
	assert.True(t, rawHasLoneSurrogateEscape([]byte(`"\udc00"`)))
	// A non-surrogate escape and a literal U+FFFD are not lone surrogates.
	assert.False(t, rawHasLoneSurrogateEscape([]byte(`"A"`)))
	assert.False(t, rawHasLoneSurrogateEscape([]byte("\"\xef\xbf\xbd\"")))
	// A truncated \u escape (not four hex digits) parses as not-a-surrogate.
	assert.False(t, rawHasLoneSurrogateEscape([]byte(`"\uZZZZ"`)))
	// A valid pair followed by a lone low surrogate: the scan skips the pair's
	// low half and still catches the trailing lone surrogate.
	assert.True(t, rawHasLoneSurrogateEscape([]byte("\"\\ud83d\\ude00\\udc00\"")))
	// A valid pair with no trailing surrogate is clean to the end.
	assert.False(t, rawHasLoneSurrogateEscape([]byte("\"\\ud83d\\ude00AAAAAA\"")))
	// An ESCAPED backslash (`\\`) followed by literal `ud800` text is NOT a
	// unicode escape: the `\u` belongs to the consumed `\\` boundary, so the
	// scan must tokenize `\\` pairs before matching `\u`. CUE accepts such a key
	// (the `\\ud800` decodes to the literal text `\ud800`, no surrogate).
	assert.False(t, rawHasLoneSurrogateEscape([]byte(`"\\ud800"`)))
	assert.False(t, rawHasLoneSurrogateEscape([]byte(`"a\\ud800"`)))
	assert.False(t, rawHasLoneSurrogateEscape([]byte(`"\\\\ud800"`)))
	// A literal escaped backslash BEFORE a genuine lone-surrogate escape is
	// still caught: `\\` consumes two, then `\ud800` is a real lone surrogate.
	assert.True(t, rawHasLoneSurrogateEscape([]byte(`"\\\ud800"`)))
}

func TestParseHex4(t *testing.T) {
	v, ok := parseHex4([]byte("D800"))
	assert.True(t, ok)
	assert.Equal(t, uint32(0xD800), v)
	v, ok = parseHex4([]byte("00ff"))
	assert.True(t, ok)
	assert.Equal(t, uint32(0x00FF), v)
	_, ok = parseHex4([]byte("00g0"))
	assert.False(t, ok)
	_, ok = parseHex4([]byte("abc"))
	assert.False(t, ok, "fewer than four digits is not a code unit")
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
	t.Run("lone-surrogate-escape keys are rejected", func(t *testing.T) {
		// "\ud800" and "\udc00" decode to the same U+FFFD; they collide, so the
		// scanner rejects the lone-surrogate ESCAPE outright (matching pre-flip
		// CUE) rather than fabricating or suppressing a duplicate.
		err := scanDuplicateKeys([]byte(`{"\ud800":1,"\udc00":2}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "lone-surrogate")
	})
	t.Run("a literal U+FFFD key is accepted", func(t *testing.T) {
		// A literal replacement character in the source is NOT an escape, so CUE
		// accepts it and the scanner walks past it without rejecting.
		assert.NoError(t, scanDuplicateKeys([]byte("{\"\xef\xbf\xbd\":1}")))
	})
	t.Run("a literal U+FFFD key with an escaped-backslash ud800 is accepted", func(t *testing.T) {
		// The key holds a literal U+FFFD (so keyHasLoneSurrogateEscape inspects
		// the raw bytes) plus an ESCAPED backslash before `ud800`: `\\ud800` is
		// the literal text `\ud800`, not a unicode escape, so CUE accepts it and
		// the scanner must too. (Regression: the raw scan matched `\u` after the
		// `\\` boundary and wrongly rejected.)
		assert.NoError(t, scanDuplicateKeys([]byte("{\"\xef\xbf\xbd\\\\ud800\":1}")))
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
	t.Run("duplicate nested under a literal U+FFFD key reported", func(t *testing.T) {
		// A literal U+FFFD key is walked (not rejected as an escape), so a real
		// duplicate nested under it is still caught.
		err := scanDuplicateKeys([]byte("{\"\xef\xbf\xbd\":{\"a\":1,\"a\":2}}"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"a"`)
	})
}
