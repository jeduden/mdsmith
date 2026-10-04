// Package rootfstest holds the shared test helper that records the
// roots lint.OpenRootFS opens, so a package's tests check its roots
// are closed without declaring a seam of their own.
package rootfstest

import (
	"slices"
	"sync"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

// Record swaps lint.OpenRootFS's opener for one that opens through the
// opener it replaced and records each root, and returns a func that
// lists the roots opened so far in open order. Opens from several
// goroutines are each recorded. The opener is restored at t's cleanup.
// A test that calls Record must not run in parallel with others that
// open roots.
func Record(t testing.TB) func() []lint.RootFS {
	t.Helper()
	var (
		mu     sync.Mutex
		opened []lint.RootFS
	)
	t.Cleanup(lint.WrapOpenRootFS(func(prev func(string) lint.RootFS) func(string) lint.RootFS {
		return func(dir string) lint.RootFS {
			r := prev(dir)
			mu.Lock()
			opened = append(opened, r)
			mu.Unlock()
			return r
		}
	}))
	return func() []lint.RootFS {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(opened)
	}
}
