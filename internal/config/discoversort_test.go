package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// writeFilesReverseOrder creates n YAML files under dir named so their
// creation order is the reverse of their filename sort order — the
// worst case for "did we accidentally rely on filesystem creation
// order instead of os.ReadDir's filename sort" — and returns the
// basenames in filename-sorted order.
func writeFilesReverseOrder(t *testing.T, dir string, n int, body string) []string {
	t.Helper()
	names := make([]string, n)
	for i := 0; i < n; i++ {
		names[i] = fmt.Sprintf("kind-%02d", i)
	}
	for i := n - 1; i >= 0; i-- {
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, names[i]+".yaml"), []byte(body), 0o644))
	}
	return names
}

// TestDiscoverKinds_CollisionNamesSortedPair pins the behavior the
// removed sort.Slice in discoverKinds used to guarantee: a `.yaml`/
// `.yml` basename collision names the two colliding files in filename
// order in its error, regardless of filesystem creation order. Since
// os.ReadDir already returns entries "sorted by filename" (per its own
// doc), this must hold with no sort.Slice call in discoverKinds at all.
func TestDiscoverKinds_CollisionNamesSortedPair(t *testing.T) {
	dir := t.TempDir()
	kindsDir := filepath.Join(dir, ".mdsmith", "kinds")
	require.NoError(t, os.MkdirAll(kindsDir, 0o755))
	// Write the .yml (sorts after .yaml) first, so creation order is
	// the reverse of filename order.
	require.NoError(t, os.WriteFile(filepath.Join(kindsDir, "audit-log.yml"), []byte(`path-pattern: "*.md"`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(kindsDir, "audit-log.yaml"), []byte(`path-pattern: "*.md"`), 0o644))

	_, err := discoverKinds(dir)
	require.ErrorContains(t, err, "audit-log.yaml and audit-log.yml")
}

// TestDiscoverSchemas_ManyFilesReverseCreationOrderAllLoad is a basic
// coverage check that many schema files created in reverse filename
// order all still load — map membership never depends on iteration
// order, so this alone does not exercise the removed sort; see
// TestDiscoverSchemas_CollisionNamesSortedPair for the actual
// order-sensitive regression net.
func TestDiscoverSchemas_ManyFilesReverseCreationOrderAllLoad(t *testing.T) {
	dir := t.TempDir()
	schemasDir := filepath.Join(dir, ".mdsmith", "schemas")
	require.NoError(t, os.MkdirAll(schemasDir, 0o755))
	names := writeFilesReverseOrder(t, schemasDir, 10, "closed: false\n")

	got, err := discoverSchemas(dir)
	require.NoError(t, err)
	require.Len(t, got, len(names))
	for _, name := range names {
		require.Contains(t, got, name)
	}
}

// TestDiscoverSchemas_CollisionNamesSortedPair mirrors
// TestDiscoverKinds_CollisionNamesSortedPair for discoverSchemas: this
// is the test that actually depends on os.ReadDir's sorted-by-filename
// order, since the collision error names whichever file was processed
// first as "prior".
func TestDiscoverSchemas_CollisionNamesSortedPair(t *testing.T) {
	dir := t.TempDir()
	schemasDir := filepath.Join(dir, ".mdsmith", "schemas")
	require.NoError(t, os.MkdirAll(schemasDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(schemasDir, "rfc.yml"), []byte("closed: false\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(schemasDir, "rfc.yaml"), []byte("closed: false\n"), 0o644))

	_, err := discoverSchemas(dir)
	require.ErrorContains(t, err, "rfc.yaml and rfc.yml")
}

// TestDiscoverConventions_CollisionNamesSortedPair mirrors
// TestDiscoverKinds_CollisionNamesSortedPair for discoverConventions.
func TestDiscoverConventions_CollisionNamesSortedPair(t *testing.T) {
	dir := t.TempDir()
	conventionsDir := filepath.Join(dir, ".mdsmith", "conventions")
	require.NoError(t, os.MkdirAll(conventionsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(conventionsDir, "house.yml"), []byte("flavor: commonmark\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(conventionsDir, "house.yaml"), []byte("flavor: commonmark\n"), 0o644))

	_, err := discoverConventions(dir)
	require.ErrorContains(t, err, "house.yaml and house.yml")
}

// TestDiscoverWordlists_ManyFilesReverseCreationOrderAllLoad mirrors
// TestDiscoverSchemas_ManyFilesReverseCreationOrderAllLoad for
// discoverWordlists.
func TestDiscoverWordlists_ManyFilesReverseCreationOrderAllLoad(t *testing.T) {
	dir := t.TempDir()
	wordlistsDir := filepath.Join(dir, ".mdsmith", "wordlists")
	require.NoError(t, os.MkdirAll(wordlistsDir, 0o755))
	names := writeFilesReverseOrder(t, wordlistsDir, 10, "entries:\n  - foo\n")

	got, err := discoverWordlists(dir)
	require.NoError(t, err)
	require.Len(t, got, len(names))
	for _, name := range names {
		require.Contains(t, got, name)
	}
}

// TestDiscoverWordlists_CollisionNamesSortedPair mirrors
// TestDiscoverSchemas_CollisionNamesSortedPair for discoverWordlists.
func TestDiscoverWordlists_CollisionNamesSortedPair(t *testing.T) {
	dir := t.TempDir()
	wordlistsDir := filepath.Join(dir, ".mdsmith", "wordlists")
	require.NoError(t, os.MkdirAll(wordlistsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(wordlistsDir, "banned.yml"), []byte("entries:\n  - foo\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(wordlistsDir, "banned.yaml"), []byte("entries:\n  - foo\n"), 0o644))

	_, err := discoverWordlists(dir)
	require.ErrorContains(t, err, "banned.yaml and banned.yml")
}

// BenchmarkDiscoverKinds times discoverKinds over real kind files. It
// is a manual tool, not a CI gate: YAML parsing dominates its
// allocations, so the removed sort.Slice call barely shows (measured
// locally with 20 kind files: ~1801 allocs/op before the removal, ~1798
// after). TestDiscover_NoReSortOfReadDir is the CI gate; it isolates
// the sort by using files the extension filter skips. Run this with
// -bench and compare via benchstat before/after a change to
// discoverKinds.
func BenchmarkDiscoverKinds(b *testing.B) {
	dir := b.TempDir()
	kindsDir := filepath.Join(dir, ".mdsmith", "kinds")
	require.NoError(b, os.MkdirAll(kindsDir, 0o755))
	const n = 20
	for i := 0; i < n; i++ {
		require.NoError(b, os.WriteFile(
			filepath.Join(kindsDir, "kind-"+strconv.Itoa(i)+".yaml"),
			[]byte(`path-pattern: "*.md"`), 0o644))
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := discoverKinds(dir)
		require.NoError(b, err)
	}
}

// TestDiscover_NoReSortOfReadDir pins that no discover* function
// re-sorts os.ReadDir's entries, which already arrive sorted by
// filename. The removed re-sort was a sort.Slice call, which allocates
// for reflect.Swapper even when nothing moves. Every entry here is a
// .txt file that the extension filter skips, so all a discover* call
// allocates beyond os.ReadDir is its directory path and its two maps;
// a re-sort would add its own allocations on top.
func TestDiscover_NoReSortOfReadDir(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	cases := []struct {
		name     string
		dir      string
		discover func(string) error
	}{
		{"kinds", kindFilesDir, func(ws string) error { _, err := discoverKinds(ws); return err }},
		{"schemas", schemaFilesDir, func(ws string) error { _, err := discoverSchemas(ws); return err }},
		{"conventions", conventionFilesDir, func(ws string) error { _, err := discoverConventions(ws); return err }},
		{"wordlists", wordlistFilesDir, func(ws string) error { _, err := discoverWordlists(ws); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ws := t.TempDir()
			dir := filepath.Join(ws, filepath.FromSlash(tc.dir))
			require.NoError(t, os.MkdirAll(dir, 0o755))
			for i := 15; i >= 0; i-- {
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, fmt.Sprintf("note-%02d.txt", i)), nil, 0o644))
			}

			var err error
			readDir := testing.AllocsPerRun(50, func() {
				_, err = os.ReadDir(dir)
			})
			require.NoError(t, err)
			discover := testing.AllocsPerRun(50, func() {
				err = tc.discover(ws)
			})
			require.NoError(t, err)

			const allocBudget = 8 // measured: the path join plus two maps sized for 16 entries
			delta := discover - readDir
			t.Logf("discover allocs/op = %.0f, os.ReadDir allocs/op = %.0f, delta = %.0f",
				discover, readDir, delta)
			require.LessOrEqualf(t, delta, float64(allocBudget),
				"discover %s allocates %.0f more than os.ReadDir; did a re-sort come back?",
				tc.name, delta)
		})
	}
}
