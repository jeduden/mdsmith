package log

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrintf_Enabled(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{Enabled: true, W: &buf}

	l.Printf("config: %s", ".mdsmith.yml")

	want := "config: .mdsmith.yml\n"
	assert.Equal(t, want, buf.String())
}

func TestPrintf_Disabled(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{Enabled: false, W: &buf}

	l.Printf("config: %s", ".mdsmith.yml")

	assert.Equal(t, "", buf.String(), "expected no output")
}

func TestPrintf_MultipleMessages(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{Enabled: true, W: &buf}

	l.Printf("file: %s", "README.md")
	l.Printf("rule: %s %s", "MDS001", "line-length")

	want := "file: README.md\nrule: MDS001 line-length\n"
	assert.Equal(t, want, buf.String())
}

// TestPrintf_ConcurrentWritesAreSerialized hammers Printf from many
// goroutines into one buffer. Without internal locking this is a data
// race on the unsynchronized writer (caught by `go test -race`) and
// can tear or drop lines; with the lock every line is intact and the
// count is exact.
func TestPrintf_ConcurrentWritesAreSerialized(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{Enabled: true, W: &buf}

	const goroutines = 16
	const perG = 64
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				l.Printf("g%02d-%03d", id, i)
			}
		}(g)
	}
	wg.Wait()

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	assert.Len(t, lines, goroutines*perG, "every Printf must produce exactly one line")
	for _, ln := range lines {
		assert.Regexp(t, `^g\d{2}-\d{3}$`, ln, "torn or interleaved write")
	}
}
