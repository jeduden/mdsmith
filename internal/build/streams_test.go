package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamCapture_RingBufferKeepsLast50Lines(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "action.log")
	sc, err := newStreamCapture(logPath, "tgt", nil)
	require.NoError(t, err)

	for i := 0; i < 70; i++ {
		_, _ = sc.stdout().Write([]byte("line\n"))
	}
	require.NoError(t, sc.Close())

	tail := sc.stdoutTail()
	assert.Len(t, tail, 50)
}

func TestStreamCapture_LogFilePrefixesStreams(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "action.log")
	sc, err := newStreamCapture(logPath, "tgt", nil)
	require.NoError(t, err)

	_, _ = sc.stdout().Write([]byte("out line\n"))
	_, _ = sc.stderr().Write([]byte("err line\n"))
	require.NoError(t, sc.Close())

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	text := string(data)
	assert.Contains(t, text, "[stdout] out line")
	assert.Contains(t, text, "[stderr] err line")
}

func TestStreamCapture_StderrTailRetainsLinesUnderCap(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "action.log")
	sc, err := newStreamCapture(logPath, "tgt", nil)
	require.NoError(t, err)

	for i := 0; i < 40; i++ {
		_, _ = sc.stderr().Write([]byte("e\n"))
	}
	require.NoError(t, sc.Close())

	// The ring buffer caps at ringLines (50); 40 lines all fit, and the
	// stderr tail returns them in arrival order.
	tail := sc.stderrTail()
	assert.Len(t, tail, 40)
}

func TestStreamCapture_StreamForwardsToWriter(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "action.log")
	var sink strings.Builder
	sc, err := newStreamCapture(logPath, "book.html", &sink)
	require.NoError(t, err)

	_, _ = sc.stdout().Write([]byte("reading chapter 1...\n"))
	_, _ = sc.stderr().Write([]byte("warn: slow\n"))
	require.NoError(t, sc.Close())

	out := sink.String()
	assert.Contains(t, out, "[book.html] reading chapter 1...")
	assert.Contains(t, out, "[book.html] warn: slow")
}

func TestStreamCapture_PartialLineFlushedOnClose(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "action.log")
	sc, err := newStreamCapture(logPath, "tgt", nil)
	require.NoError(t, err)

	_, _ = sc.stdout().Write([]byte("no newline here"))
	require.NoError(t, sc.Close())

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "[stdout] no newline here")
	assert.Equal(t, []string{"no newline here"}, sc.stdoutTail())
}

func TestNewStreamCapture_ErrorWhenLogPathIsDir(t *testing.T) {
	dir := t.TempDir()
	// Place a directory at the logPath so os.Create fails.
	logPath := filepath.Join(dir, "logs")
	require.NoError(t, os.MkdirAll(logPath, 0o755))
	_, err := newStreamCapture(logPath, "tgt", nil)
	require.Error(t, err)
}
