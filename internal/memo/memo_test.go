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

// TestGetWith_PassesArgAndBuildsOnce pins that GetWith hands arg to
// build and, like Get, builds once.
func TestGetWith_PassesArgAndBuildsOnce(t *testing.T) {
	var e Entry
	calls := 0
	build := func(n int) any { calls++; return n * 2 }

	require.Equal(t, 42, GetWith(&e, 21, build))
	require.Equal(t, 42, GetWith(&e, 99, build))
	assert.Equal(t, 1, calls)
}

// TestGetWith_PanicMarksDone pins GetWith's panic contract, which
// matches Get's.
func TestGetWith_PanicMarksDone(t *testing.T) {
	var e Entry
	assert.Panics(t, func() {
		GetWith(&e, 1, func(int) any { panic("boom") })
	})
	assert.Nil(t, GetWith(&e, 1, func(int) any { return "v" }))
}

// TestLoad_ReturnsSameEntryPerKey pins that one key maps to one Entry
// and distinct keys to distinct entries.
func TestLoad_ReturnsSameEntryPerKey(t *testing.T) {
	var m sync.Map
	a := Load(&m, "a")
	assert.Same(t, a, Load(&m, "a"))
	assert.NotSame(t, a, Load(&m, "b"))
}

// TestLoad_WarmPathAllocatesNothing pins the cache-hit cost of
// Load+Get at zero allocs: the Load-before-LoadOrStore check skips the
// throwaway &Entry{} that LoadOrStore's argument would construct.
func TestLoad_WarmPathAllocatesNothing(t *testing.T) {
	var m sync.Map
	build := func() any { return 42 }
	Load(&m, "k").Get(build)

	allocs := testing.AllocsPerRun(200, func() {
		Load(&m, "k").Get(build)
	})
	assert.Zero(t, allocs)
}

// TestGetWith_WarmPathAllocatesNothing pins GetWith's cache-hit cost
// at zero allocs for a pointer argument, the shape lint.File.MemoFile
// uses.
func TestGetWith_WarmPathAllocatesNothing(t *testing.T) {
	var m sync.Map
	arg := &struct{ n int }{n: 1}
	build := func(p *struct{ n int }) any { return p.n }
	GetWith(Load(&m, "k"), arg, build)

	allocs := testing.AllocsPerRun(200, func() {
		GetWith(Load(&m, "k"), arg, build)
	})
	assert.Zero(t, allocs)
}

// TestEntryFieldLayout_PointerFieldsLeading pins the val/done/mu
// order (pointer-bearing field first) from
// docs/development/high-performance-go.md "Struct layout".
func TestEntryFieldLayout_PointerFieldsLeading(t *testing.T) {
	structlayout.AssertPointerFieldsFirst(t, reflect.TypeOf(Entry{}))
}
