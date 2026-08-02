package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	buildexec "github.com/jeduden/mdsmith/internal/build"
)

// TestSnapshotOutputs_LargeFile_BoundedMemory pins
// docs/development/high-performance-go.md's "os.ReadFile on huge
// inputs — one giant alloc, all resident — use bufio.Reader with a
// tuned buffer" (Patterns to avoid). --build-verify's snapshotOutputs
// must not hold a declared build output's full content in memory: it
// only needs to know whether two runs produced the same bytes, so a
// streamed content hash serves that without an alloc proportional to
// the output's size.
func TestSnapshotOutputs_LargeFile_BoundedMemory(t *testing.T) {
	const size = 8 * 1024 * 1024 // 8 MiB
	root := t.TempDir()
	big := make([]byte, size)
	for i := range big {
		big[i] = byte(i)
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "out.bin"), big, 0o644))
	bt := buildTarget{
		target: buildexec.Target{Root: root, Outputs: []string{"out.bin"}},
	}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	snap := snapshotOutputs(bt)
	runtime.ReadMemStats(&after)

	require.Len(t, snap, 1)
	// The whole point of the fix: total heap growth must stay well under
	// the output's size, proving the content was streamed through a
	// hash rather than read whole into a []byte.
	grew := after.TotalAlloc - before.TotalAlloc
	if grew > size/4 {
		t.Fatalf("snapshotOutputs on an %d-byte output allocated %d bytes; "+
			"want well under the file size (no full-file buffering)", size, grew)
	}
}
