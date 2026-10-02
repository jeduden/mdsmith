package memo

import (
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jeduden/mdsmith/internal/structlayout"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEntry_GetBuildsOnce pins that build runs on the first Get only;
// later calls serve the cached value.
func TestEntry_GetBuildsOnce(t *testing.T) {
	var e Entry
	calls := 0
	build := func() any { calls++; return "v" }

	require.Equal(t, "v", e.Get(build))
	require.Equal(t, "v", e.Get(build))
	assert.Equal(t, 1, calls)
}

// TestEntry_GetConcurrentSingleBuild pins that racing goroutines still
// run build exactly once.
func TestEntry_GetConcurrentSingleBuild(t *testing.T) {
	var e Entry
	var calls int32
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.Equal(t, 7, e.Get(func() any {
				atomic.AddInt32(&calls, 1)
				return 7
			}))
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

// TestEntry_GetPanicMarksDone pins sync.Once semantics: a panicking
// build still marks the entry done and releases the mutex, so the next
// Get returns the zero value instead of deadlocking or rebuilding.
func TestEntry_GetPanicMarksDone(t *testing.T) {
	var e Entry
	assert.Panics(t, func() { e.Get(func() any { panic("boom") }) })

	calls := 0
	assert.Nil(t, e.Get(func() any { calls++; return "v" }))
	assert.Zero(t, calls)
}

// TestEntry_getSlow_SkipsBuildWhenDone pins the double-checked lock's
// second check: a caller that lost the race to the mutex finds the
// slot already done and returns the cached value without building.
func TestEntry_getSlow_SkipsBuildWhenDone(t *testing.T) {
	var e Entry
	e.val = "cached"
	e.done.Store(true)

	calls := 0
	assert.Equal(t, "cached", e.getSlow(func() any { calls++; return "v" }))
	assert.Zero(t, calls)
}

// TestMap_EntryReturnsSameEntryPerKey pins that one key maps to one
// Entry and distinct keys to distinct entries.
func TestMap_EntryReturnsSameEntryPerKey(t *testing.T) {
	var m Map
	a := m.Entry("a")
	assert.Same(t, a, m.Entry("a"))
	assert.NotSame(t, a, m.Entry("b"))
}

// TestMap_GetWarmPathAllocatesNothing pins the cache-hit cost of
// Map.Get at zero allocs: the Load-before-LoadOrStore check skips the
// throwaway &Entry{} that LoadOrStore's argument would construct.
func TestMap_GetWarmPathAllocatesNothing(t *testing.T) {
	var m Map
	build := func() any { return 42 }
	m.Get("k", build)

	allocs := testing.AllocsPerRun(200, func() {
		m.Get("k", build)
	})
	assert.Zero(t, allocs)
}

// TestMap_DeleteForcesRebuild pins that Delete drops the slot so the
// next Get runs build again, and leaves other keys cached.
func TestMap_DeleteForcesRebuild(t *testing.T) {
	var m Map
	var calls int32
	build := func() any { return atomic.AddInt32(&calls, 1) }
	m.Get("a", build)
	m.Get("b", build)
	m.Delete("a")
	assert.Equal(t, int32(3), m.Get("a", build))
	assert.Equal(t, int32(2), m.Get("b", build))
}

// TestMap_RangeVisitsEveryKeyAndStops pins Range's key iteration and
// its early stop when f returns false.
func TestMap_RangeVisitsEveryKeyAndStops(t *testing.T) {
	var m Map
	m.Entry("a")
	m.Entry("b")
	var seen []string
	m.Range(func(k string) bool {
		seen = append(seen, k)
		return true
	})
	assert.ElementsMatch(t, []string{"a", "b"}, seen)

	n := 0
	m.Range(func(string) bool {
		n++
		return false
	})
	assert.Equal(t, 1, n)
}

// TestMap_ClearDropsEveryKey pins that Clear empties the map.
func TestMap_ClearDropsEveryKey(t *testing.T) {
	var m Map
	m.Entry("a")
	m.Entry("b")
	m.Clear()
	m.Range(func(k string) bool {
		t.Errorf("unexpected key %q after Clear", k)
		return true
	})
}

// TestEntryFieldLayout_PointerFieldsLeading pins the val/done/mu
// order (pointer-bearing field first) from
// docs/development/high-performance-go.md "Struct layout".
func TestEntryFieldLayout_PointerFieldsLeading(t *testing.T) {
	structlayout.AssertPointerFieldsFirst(t, reflect.TypeOf(Entry{}))
}
