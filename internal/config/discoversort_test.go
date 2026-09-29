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

// BenchmarkDiscoverKinds is a manual regression-detection tool for the
// removed sort.Slice call, not a CI-enforced gate: the win is a small,
// mostly-constant handful of allocations (reflect.Swapper's own
// overhead, not something that scales with entry count) against a much
// larger, YAML-parse-dominated total, so a hard per-op budget here would
// be too sensitive to unrelated allocation drift elsewhere in
// discoverKinds or its dependencies (yaml.v3, os.ReadDir) to stay
// meaningful — see TestDiscoverKinds_CollisionNamesSortedPair and the
// other tests in this file for the actual correctness/regression net on
// this change. Run manually with `-bench` and compare via benchstat
// before/after a change to discoverKinds. Measured locally with 20 kind
// files: ~1801 allocs/op before the sort.Slice removal, ~1798 after.
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
