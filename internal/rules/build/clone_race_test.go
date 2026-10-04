package build

import (
	"sync"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/require"
)

// TestCloneInstanceDuringFixIsRaceFree pins that rule.CloneInstance may
// copy the rule while another goroutine runs Fix on it — the LSP runs
// Session.Fix on the shared rule set while a Check clones it per worker.
// A lazily built engine cached in the struct made Fix write the fields
// the clone reads; run with -race to see it.
func TestCloneInstanceDuringFixIsRaceFree(t *testing.T) {
	for range 20 {
		r := &Rule{}
		f, err := lint.NewFile("test.md", []byte("# A\n"))
		require.NoError(t, err)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); r.Fix(f) }()
		go func() { defer wg.Done(); _ = rule.CloneInstance(r) }()
		wg.Wait()
	}
}
