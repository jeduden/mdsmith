package fix

import (
	"bytes"
	"testing"

	vlog "github.com/jeduden/mdsmith/internal/log"
)

// With -v off, the per-file and per-pass log lines must not box their
// arguments; see docs/development/high-performance-go.md, "Allocations".
func TestFixerLogHelpers_DisabledDoNotAllocate(t *testing.T) {
	f := &Fixer{}
	path := "docs/a.md"
	allocs := testing.AllocsPerRun(100, func() {
		f.logFile(path)
		f.logPass(3, path)
		f.logStable(path, 3)
	})
	if allocs != 0 {
		t.Errorf("allocs = %v, want 0", allocs)
	}
}

func TestFixerLogHelpers_EnabledWriteLines(t *testing.T) {
	var buf bytes.Buffer
	f := &Fixer{Logger: &vlog.Logger{Enabled: true, W: &buf}}
	f.logFile("a.md")
	f.logPass(2, "a.md")
	f.logStable("a.md", 2)
	want := "file: a.md\nfix: pass 2 on a.md\nfix: a.md stable after 2 passes\n"
	if buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
}
