// Package rootfstest holds the shared test helper for packages that open
// workspace roots through a swappable lint.OpenRootFS seam, so each such
// package's tests record the opened handles the same way.
package rootfstest

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

// Record swaps *seam for one that opens through the seam it replaced and
// records each handle in open order, so a test can check the code under
// test closed it. The seam is restored at t's cleanup. A test that calls
// Record must not run in parallel with others that use the same seam.
func Record(t testing.TB, seam *func(string) lint.RootFS) *[]lint.RootFS {
	t.Helper()
	var opened []lint.RootFS
	prev := *seam
	*seam = func(dir string) lint.RootFS {
		r := prev(dir)
		opened = append(opened, r)
		return r
	}
	t.Cleanup(func() { *seam = prev })
	return &opened
}
