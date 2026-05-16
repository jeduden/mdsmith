package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeProfile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	return p
}

// The duplicate-block case the CI hit: unit profile records a hit
// for a line the e2e profile recorded as zero. Summing must keep
// it covered rather than letting the second file win.
func TestMergeCoverage_SumsDuplicateBlocks(t *testing.T) {
	dir := t.TempDir()
	unit := writeProfile(t, dir, "unit.cov",
		"mode: atomic\n"+
			"cmd/mdsmith/extract.go:26.31,30.3 3 5\n"+
			"cmd/mdsmith/extract.go:30.3,32.4 1 0\n")
	e2e := writeProfile(t, dir, "e2e.cov",
		"mode: atomic\n"+
			"cmd/mdsmith/extract.go:26.31,30.3 3 0\n"+
			"cmd/mdsmith/extract.go:30.3,32.4 1 2\n")
	out := filepath.Join(dir, "merged.cov")
	require.NoError(t, MergeCoverage([]string{unit, e2e}, out))

	got, err := os.ReadFile(out)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(got)), "\n")
	assert.Equal(t, "mode: atomic", lines[0])
	assert.Contains(t, lines, "cmd/mdsmith/extract.go:26.31,30.3 3 5")
	assert.Contains(t, lines, "cmd/mdsmith/extract.go:30.3,32.4 1 2")
}

func TestMergeCoverage_SetModeOrs(t *testing.T) {
	dir := t.TempDir()
	a := writeProfile(t, dir, "a.cov", "mode: set\nx.go:1.1,2.2 1 0\n")
	b := writeProfile(t, dir, "b.cov", "mode: set\nx.go:1.1,2.2 1 1\n")
	out := filepath.Join(dir, "m.cov")
	require.NoError(t, MergeCoverage([]string{a, b}, out))
	got, _ := os.ReadFile(out)
	assert.Contains(t, string(got), "x.go:1.1,2.2 1 1")
}

// Two blocks that share a file:line sort by their full key, not by
// Go map iteration order.
func TestMergeCoverage_StableTieBreak(t *testing.T) {
	dir := t.TempDir()
	p := writeProfile(t, dir, "p.cov",
		"mode: atomic\nx.go:1.5,1.9 1 1\nx.go:1.1,1.4 1 1\n")
	out := filepath.Join(dir, "m.cov")
	require.NoError(t, MergeCoverage([]string{p}, out))
	got, _ := os.ReadFile(out)
	lines := strings.Split(strings.TrimSpace(string(got)), "\n")
	// mode line, then the two blocks ordered by full key.
	assert.Equal(t, "x.go:1.1,1.4 1 1", lines[1])
	assert.Equal(t, "x.go:1.5,1.9 1 1", lines[2])
}

func TestLessBlock(t *testing.T) {
	a := covBlock{key: "x.go:1.1,1.4 1", startKey: "x.go:000000001"}
	b := covBlock{key: "x.go:1.5,1.9 1", startKey: "x.go:000000001"}
	c := covBlock{key: "y", startKey: "y:000000002"}
	// startKey differs.
	assert.True(t, lessBlock(a, c))
	assert.False(t, lessBlock(c, a))
	// startKey equal → key tiebreaker, both directions.
	assert.True(t, lessBlock(a, b))
	assert.False(t, lessBlock(b, a))
}

// In set mode, a repeated zero-hit key takes the "already seen"
// path the second time.
func TestMergeCoverage_SetModeRepeatedZero(t *testing.T) {
	dir := t.TempDir()
	p := writeProfile(t, dir, "p.cov",
		"mode: set\nx.go:1.1,2.2 1 0\nx.go:1.1,2.2 1 0\n")
	out := filepath.Join(dir, "m.cov")
	require.NoError(t, MergeCoverage([]string{p}, out))
	got, _ := os.ReadFile(out)
	assert.Contains(t, string(got), "x.go:1.1,2.2 1 0")
}

func TestMergeCoverage_ModeMismatch(t *testing.T) {
	dir := t.TempDir()
	a := writeProfile(t, dir, "a.cov", "mode: set\nx.go:1.1,2.2 1 1\n")
	b := writeProfile(t, dir, "b.cov", "mode: atomic\nx.go:1.1,2.2 1 1\n")
	err := MergeCoverage([]string{a, b}, filepath.Join(dir, "m.cov"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mode mismatch")
}

func TestParseCovLine(t *testing.T) {
	key, hits, err := parseCovLine("x.go:1.1,2.2 3 7")
	require.NoError(t, err)
	assert.Equal(t, "x.go:1.1,2.2 3", key)
	assert.Equal(t, 7, hits)

	_, _, err = parseCovLine("nospace")
	assert.Error(t, err)
	_, _, err = parseCovLine("x.go:1.1,2.2 3 notanint")
	assert.Error(t, err)
	// Has a trailing count but no statement-count field.
	_, _, err = parseCovLine("x.go:1.1,2.2 3")
	assert.Error(t, err)
}

func TestCovStartKey(t *testing.T) {
	assert.Equal(t, "x.go:000000012",
		covStartKey("x.go:12.4,13.2 1"))
	assert.Equal(t, "nocolon", covStartKey("nocolon"))
	assert.Equal(t, "x.go:12", covStartKey("x.go:12")) // no comma
	assert.Equal(t, "x.go:a.1,2.2 1",
		covStartKey("x.go:a.1,2.2 1")) // non-numeric start line
	// Comma but no dot must not panic on a -1 slice index.
	assert.Equal(t, "x.go:12,13 1", covStartKey("x.go:12,13 1"))
	// Dot but no comma is also rejected (comma < 0 branch).
	assert.Equal(t, "x.go:1.2 1", covStartKey("x.go:1.2 1"))
	// Dot after the comma is also rejected (degenerate coords).
	assert.Equal(t, "x.go:12,1.3 1", covStartKey("x.go:12,1.3 1"))
}

// A malformed record with a comma but no dot reaches covStartKey
// via MergeCoverage; it must degrade rather than crash the CI tool.
func TestMergeCoverage_MalformedCoordsNoPanic(t *testing.T) {
	dir := t.TempDir()
	p := writeProfile(t, dir, "p.cov",
		"mode: atomic\nx.go:12,13 1 1\n")
	require.NoError(t,
		MergeCoverage([]string{p}, filepath.Join(dir, "m.cov")))
}

func TestMergeCoverage_BlankLinesAndOddKeys(t *testing.T) {
	dir := t.TempDir()
	// Blank line tolerated; a record whose key has no colon/comma
	// still round-trips through covStartKey's fallbacks.
	p := writeProfile(t, dir, "p.cov",
		"mode: atomic\n\nx.go:1.1,2.2 1 1\nabc 1 0\n")
	out := filepath.Join(dir, "m.cov")
	require.NoError(t, MergeCoverage([]string{p}, out))
	got, _ := os.ReadFile(out)
	assert.Contains(t, string(got), "x.go:1.1,2.2 1 1")
	assert.Contains(t, string(got), "abc 1 0")
}

func TestMergeCoverage_ScannerError(t *testing.T) {
	dir := t.TempDir()
	// A single token larger than the 4 MiB scanner buffer makes
	// bufio.Scanner fail with ErrTooLong.
	huge := make([]byte, 5<<20)
	for i := range huge {
		huge[i] = 'a'
	}
	body := "mode: atomic\n" + string(huge) + "\n"
	p := writeProfile(t, dir, "huge.cov", body)
	err := MergeCoverage([]string{p}, filepath.Join(dir, "m.cov"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read")
}

func TestMergeCoverage_Errors(t *testing.T) {
	dir := t.TempDir()
	assert.Error(t, MergeCoverage(nil, filepath.Join(dir, "m.cov")))
	assert.Error(t, MergeCoverage([]string{filepath.Join(dir, "missing.cov")},
		filepath.Join(dir, "m.cov")))

	bad := writeProfile(t, dir, "bad.cov", "mode: atomic\ngarbage line\n")
	assert.Error(t, MergeCoverage([]string{bad}, filepath.Join(dir, "m.cov")))

	noMode := writeProfile(t, dir, "nm.cov", "x.go:1.1,2.2 1 1\n")
	assert.Error(t, MergeCoverage([]string{noMode}, filepath.Join(dir, "m.cov")))
}
