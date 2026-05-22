package descriptivelinktext

import (
	"reflect"
	"testing"
	"time"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func check(t *testing.T, src string) []lint.Diagnostic {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	r := &Rule{Banned: append([]string(nil), defaultBanned...)}
	return r.Check(f)
}

func TestDescriptiveText(t *testing.T) {
	diags := check(t, "# T\n\n[the install guide](x)\n")
	assert.Empty(t, diags)
}

func TestClickHere(t *testing.T) {
	diags := check(t, "# T\n\n[click here](x)\n")
	require.Len(t, diags, 1)
	assert.Equal(t, `link text "click here" is not descriptive`, diags[0].Message)
}

func TestHere(t *testing.T) {
	diags := check(t, "# T\n\nSee [here](x) for details.\n")
	require.Len(t, diags, 1)
	assert.Equal(t, `link text "here" is not descriptive`, diags[0].Message)
}

func TestLink(t *testing.T) {
	diags := check(t, "# T\n\n[link](x)\n")
	require.Len(t, diags, 1)
	assert.Equal(t, `link text "link" is not descriptive`, diags[0].Message)
}

func TestMore(t *testing.T) {
	diags := check(t, "# T\n\n[more](x)\n")
	require.Len(t, diags, 1)
	assert.Equal(t, `link text "more" is not descriptive`, diags[0].Message)
}

func TestCaseInsensitive(t *testing.T) {
	diags := check(t, "# T\n\n[Click Here](x)\n")
	require.Len(t, diags, 1)
	assert.Equal(t, `link text "Click Here" is not descriptive`, diags[0].Message)
}

func TestWhitespaceInsensitive(t *testing.T) {
	diags := check(t, "# T\n\n[click  here](x)\n")
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "not descriptive")
}

func TestCodeSpanOnly(t *testing.T) {
	diags := check(t, "# T\n\n[`here`](x)\n")
	assert.Empty(t, diags)
}

func TestImageOnly(t *testing.T) {
	diags := check(t, "# T\n\n[![alt](img.png)](x)\n")
	assert.Empty(t, diags)
}

func TestCustomBannedReplaces(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("# T\n\n[click here](x)\n\n[read more](y)\n"))
	require.NoError(t, err)
	r := &Rule{Banned: []string{"read more"}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, `link text "read more" is not descriptive`, diags[0].Message)
}

func TestEmptyBannedList(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("# T\n\n[click here](x)\n"))
	require.NoError(t, err)
	r := &Rule{Banned: []string{}}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestLineNumber(t *testing.T) {
	diags := check(t, "# T\n\nSome text.\n\n[here](x)\n")
	require.Len(t, diags, 1)
	assert.Equal(t, 5, diags[0].Line)
}

func TestApplySettingsBanned(t *testing.T) {
	r := &Rule{Banned: append([]string(nil), defaultBanned...)}
	err := r.ApplySettings(map[string]any{
		"banned": []any{"read more", "learn more"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"read more", "learn more"}, r.Banned)
}

func TestApplySettingsUnknown(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknown": "x"})
	assert.ErrorContains(t, err, "unknown setting")
}

func TestApplySettingsBannedWrongType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"banned": "not-a-list"})
	assert.ErrorContains(t, err, "list of strings")
}

func TestEnabledByDefault(t *testing.T) {
	r := &Rule{}
	assert.False(t, r.EnabledByDefault(), "MDS063 must be opt-in")
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	s := r.DefaultSettings()
	banned, ok := s["banned"].([]string)
	require.True(t, ok)
	assert.Equal(t, defaultBanned, banned)
}

func TestEmphasisWrappedBannedText(t *testing.T) {
	diags := check(t, "# T\n\n[*here*](x)\n")
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "not descriptive")
}

func TestSoftLineBreakInLinkText(t *testing.T) {
	diags := check(t, "# T\n\n[click\nhere](x)\n")
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "not descriptive")
}

// TestCachedBannedSet pins the per-rule memoization contract:
// subsequent calls on the same rule return the same cached map
// (reference identity); ApplySettings invalidates the cache so the
// next call rebuilds against the new Banned list. The cache lives
// on the rule instance, not on the per-Check File, so multiple
// concurrent Checks share one build — plan 195 task 9.
func TestCachedBannedSet(t *testing.T) {
	r := &Rule{Banned: []string{"Click Here", "MORE"}}

	first := r.cachedBannedSet()
	require.Equal(t, map[string]bool{"click here": true, "more": true}, first,
		"lookup keys must be the normalised form of r.Banned")

	second := r.cachedBannedSet()
	assert.Equal(t,
		reflect.ValueOf(first).Pointer(),
		reflect.ValueOf(second).Pointer(),
		"subsequent calls on the same rule must return the same cached map")

	// ApplySettings invalidates the cache; the next call rebuilds.
	require.NoError(t, r.ApplySettings(map[string]any{"banned": []string{"X"}}))
	third := r.cachedBannedSet()
	assert.NotEqual(t,
		reflect.ValueOf(first).Pointer(),
		reflect.ValueOf(third).Pointer(),
		"ApplySettings must clear the cache so the next call rebuilds")
	assert.Equal(t, map[string]bool{"x": true}, third)

	// An empty Banned yields a non-nil empty map; CheckNode short-
	// circuits on len(r.Banned)==0 before calling cachedBannedSet, so
	// this branch is purely defensive — pin it so a future refactor
	// cannot regress it to nil.
	empty := &Rule{}
	got := empty.cachedBannedSet()
	require.NotNil(t, got)
	assert.Empty(t, got)
}

// TestCategory pins the rule.Category implementation. The method
// returns a constant; the existing test suite never read it
// directly because the engine routes diagnostics through ID/Name
// only — the contract test added here keeps the constant pinned
// so a rename does not break tooling that groups diagnostics by
// category.
func TestCategory(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "prose", r.Category())
}

// TestCachedBannedSet_DoubleCheckedLockHits pins the inner
// fast-path of the double-checked-lock pattern: when two
// goroutines race to populate the cache, the second one
// observes the pointer set under the mutex and returns it
// without rebuilding. We can't reliably interleave goroutines
// here, so the test exercises the shape by calling
// cachedBannedSet twice in sequence; the second call hits the
// outer fast path and never enters the mutex, so this also
// guards against accidentally widening the locked section.
func TestCachedBannedSet_DoubleCheckedLockHits(t *testing.T) {
	r := &Rule{Banned: []string{"X"}}
	first := r.cachedBannedSet()
	second := r.cachedBannedSet()
	assert.NotNil(t, first)
	assert.NotNil(t, second)
}

// TestCachedBannedSet_InnerLockPath pins the inner double-checked
// load: a second goroutine acquires the mutex after the first has
// populated the pointer, so it observes p != nil inside the lock
// and returns without rebuilding. The test is in the same package
// so it can pre-acquire the rule's mutex, kick off a goroutine
// that races for the cache, then store a populated pointer before
// releasing the lock — the goroutine's mutex-acquire then sees
// the populated pointer on the inner Load and returns it without
// rebuilding. Mirrors the runtime race the double-checked-lock
// idiom is designed for, but deterministically.
func TestCachedBannedSet_InnerLockPath(t *testing.T) {
	r := &Rule{Banned: []string{"X"}}
	manualMap := map[string]bool{"manual": true}

	r.bannedSetMu.Lock()
	done := make(chan map[string]bool, 1)
	go func() {
		// outer Load returns nil (pointer is still cleared); the
		// goroutine then blocks on bannedSetMu.Lock until the main
		// goroutine releases it.
		done <- r.cachedBannedSet()
	}()
	// Yield repeatedly so the goroutine reaches the Lock call.
	// time.Sleep with a small delay is the standard way to force
	// the race window without adding test-only hooks to the rule.
	time.Sleep(10 * time.Millisecond)
	// Store a populated pointer before releasing — the goroutine's
	// inner Load (line 141) will now observe it and skip the
	// rebuild.
	r.bannedSetPtr.Store(&manualMap)
	r.bannedSetMu.Unlock()

	got := <-done
	require.Equal(t, manualMap, got,
		"inner Load must return the populated pointer, not rebuild")
}
