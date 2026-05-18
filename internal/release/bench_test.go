package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBenchManifestInvariants pins the contract every manifest
// entry must hold so a future edit cannot land a half-specified
// or unpinned tool. The shipped manifest must pass; each
// individually-broken clone must fail with a message naming the
// offending field.
func TestBenchManifestInvariants(t *testing.T) {
	require.NoError(t, validateBenchManifest(benchTools()),
		"the shipped pinned manifest must satisfy its own invariants")

	tools := benchTools()
	require.NotEmpty(t, tools)
	assert.GreaterOrEqual(t, len(tools), 4,
		"hyperfine, mado, panache, rumdl are all pinned")

	assert.Error(t, validateBenchManifest(nil), "empty manifest")

	cases := []struct {
		name   string
		mutate func(bt *benchTool)
		want   string
	}{
		{"empty name", func(b *benchTool) { b.Name = "" }, "empty name"},
		{"empty version", func(b *benchTool) { b.Version = "" }, "empty version"},
		{"empty url", func(b *benchTool) { b.URL = "" }, "empty url"},
		{"empty sha", func(b *benchTool) { b.SHA256 = "" }, "empty sha256"},
		{"non-github", func(b *benchTool) {
			b.URL = "https://example.com/download/" + b.Version + "/x.tar.gz"
		}, "not a github.com release URL"},
		{"not tarball", func(b *benchTool) {
			b.URL = "https://github.com/o/r/releases/download/" + b.Version + "/x.zip"
		}, "not a .tar.gz"},
		{"version not pinned in url", func(b *benchTool) {
			b.URL = "https://github.com/o/r/releases/download/v9.9.9/x.tar.gz"
		}, "does not pin version"},
		{"sha not hex", func(b *benchTool) { b.SHA256 = "NOTHEX" }, "64 lowercase hex"},
		{"sha uppercase", func(b *benchTool) {
			b.SHA256 = "ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789"
		}, "64 lowercase hex"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			one := benchTools()[:1]
			one[0] = benchTools()[0]
			c.mutate(&one[0])
			err := validateBenchManifest(one)
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.want)
		})
	}

	t.Run("duplicate name", func(t *testing.T) {
		dup := []benchTool{benchTools()[0], benchTools()[0]}
		err := validateBenchManifest(dup)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate manifest entry")
	})
}

// TestVerifyChecksum covers the integrity gate the acceptance
// criteria call out: a tampered download (wrong SHA-256) must
// fail loudly, and the message must name both digests.
func TestVerifyChecksum(t *testing.T) {
	data := []byte("the quick brown fox\n")
	sum := sha256.Sum256(data)
	good := hex.EncodeToString(sum[:])

	require.NoError(t, verifyChecksum(data, good))

	err := verifyChecksum(data, "0000000000000000000000000000000000000000000000000000000000000000")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")
	assert.Contains(t, err.Error(), good, "message names the actual digest")

	// A single flipped byte must not pass.
	tampered := append([]byte{}, data...)
	tampered[0] ^= 0xff
	assert.Error(t, verifyChecksum(tampered, good))
}

// makeTarGz builds an in-memory gzip-compressed tar with the
// given members. Mirrors the three nesting shapes the real tool
// tarballs use (root, "./"-prefixed, versioned subdir).
func makeTarGz(t *testing.T, members map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range members {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg,
		}))
		_, err := tw.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func TestExtractTarGzBinary(t *testing.T) {
	t.Run("nested member is extracted 0755", func(t *testing.T) {
		archive := makeTarGz(t, map[string]string{
			"hyperfine-v1.20.0-x86_64-unknown-linux-musl/README.md": "docs",
			"hyperfine-v1.20.0-x86_64-unknown-linux-musl/hyperfine": "#!/bin/sh\necho hi\n",
		})
		dst := filepath.Join(t.TempDir(), "hyperfine")
		require.NoError(t, extractTarGzBinary(bytes.NewReader(archive), "hyperfine", dst))
		got, err := os.ReadFile(dst)
		require.NoError(t, err)
		assert.Equal(t, "#!/bin/sh\necho hi\n", string(got))
		info, err := os.Stat(dst)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	})

	t.Run("dot-slash prefixed member (panache shape)", func(t *testing.T) {
		archive := makeTarGz(t, map[string]string{"./panache": "PANACHE"})
		dst := filepath.Join(t.TempDir(), "panache")
		require.NoError(t, extractTarGzBinary(bytes.NewReader(archive), "panache", dst))
		got, err := os.ReadFile(dst)
		require.NoError(t, err)
		assert.Equal(t, "PANACHE", string(got))
	})

	t.Run("missing binary errors", func(t *testing.T) {
		archive := makeTarGz(t, map[string]string{"README.md": "x"})
		dst := filepath.Join(t.TempDir(), "mado")
		err := extractTarGzBinary(bytes.NewReader(archive), "mado", dst)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"mado" not found`)
	})

	t.Run("not gzip errors", func(t *testing.T) {
		err := extractTarGzBinary(bytes.NewReader([]byte("not a gzip stream")), "x", filepath.Join(t.TempDir(), "x"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "open gzip")
	})
}

// TestPromoteBenchJSON covers the copy and the missing-source
// failure: a run that did not produce one of the four exports
// must error (naming it) rather than silently promote stale JSON.
func TestPromoteBenchJSON(t *testing.T) {
	t.Run("copies all four", func(t *testing.T) {
		root := t.TempDir()
		out := filepath.Join(root, "out")
		require.NoError(t, os.MkdirAll(out, 0o755))
		for _, n := range benchJSONNames {
			require.NoError(t, os.WriteFile(filepath.Join(out, n+".json"),
				[]byte(`{"results":[{"command":"`+n+`"}]}`), 0o644))
		}
		data := filepath.Join(root, benchDirRel, "data")
		require.NoError(t, New().promoteBenchJSON(out, data))
		for _, n := range benchJSONNames {
			b, err := os.ReadFile(filepath.Join(data, n+".json"))
			require.NoError(t, err, "%s promoted", n)
			assert.Contains(t, string(b), n)
		}
	})

	t.Run("missing source errors and names it", func(t *testing.T) {
		root := t.TempDir()
		out := filepath.Join(root, "out")
		require.NoError(t, os.MkdirAll(out, 0o755))
		// Only the first three exist; corpus_neutral_mdl is absent.
		for _, n := range benchJSONNames[:3] {
			require.NoError(t, os.WriteFile(filepath.Join(out, n+".json"), []byte("{}"), 0o644))
		}
		err := New().promoteBenchJSON(out, filepath.Join(root, "data"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "corpus_neutral_mdl.json not found")
	})
}

// fakeGetter is an in-memory HTTPGetter keyed by URL.
type fakeGetter struct {
	resp map[string]struct {
		status int
		body   []byte
		err    error
	}
	calls []string
}

func (f *fakeGetter) Get(url string) (int, []byte, error) {
	f.calls = append(f.calls, url)
	r, ok := f.resp[url]
	if !ok {
		return 404, []byte("not found"), nil
	}
	return r.status, r.body, r.err
}

func TestPullSiteAssets(t *testing.T) {
	t.Run("200 writes every destination", func(t *testing.T) {
		root := t.TempDir()
		g := &fakeGetter{resp: map[string]struct {
			status int
			body   []byte
			err    error
		}{
			rawAssetsBase + "benchmarks/results.fragment.md":  {200, []byte("RESULTS-TABLE"), nil},
			rawAssetsBase + "benchmarks/headline.fragment.md": {200, []byte("HEADLINE"), nil},
			rawAssetsBase + "demo.gif":                        {200, []byte("GIF89a-bytes"), nil},
		}}
		require.NoError(t, NewWithHTTP(osFS{}, g).PullSiteAssets(root))

		res, err := os.ReadFile(filepath.Join(root, benchDirRel, "results.fragment.md"))
		require.NoError(t, err)
		assert.Equal(t, "RESULTS-TABLE", string(res))
		gif, err := os.ReadFile(filepath.Join(root, "website", "static", "img", "demo.gif"))
		require.NoError(t, err)
		assert.Equal(t, "GIF89a-bytes", string(gif))
	})

	t.Run("missing benchmark fragments keep the committed snapshot", func(t *testing.T) {
		root := t.TempDir()
		// Pre-seed a committed snapshot the deploy must not lose.
		snap := filepath.Join(root, benchDirRel, "results.fragment.md")
		require.NoError(t, os.MkdirAll(filepath.Dir(snap), 0o755))
		require.NoError(t, os.WriteFile(snap, []byte("COMMITTED-SNAPSHOT"), 0o644))

		g := &fakeGetter{resp: map[string]struct {
			status int
			body   []byte
			err    error
		}{
			// benchmarks/* absent (404 default); only the demo GIF
			// is published.
			rawAssetsBase + "demo.gif": {200, []byte("GIF"), nil},
		}}
		require.NoError(t, NewWithHTTP(osFS{}, g).PullSiteAssets(root))

		kept, err := os.ReadFile(snap)
		require.NoError(t, err)
		assert.Equal(t, "COMMITTED-SNAPSHOT", string(kept),
			"a non-required miss must not overwrite the committed fragment")
	})

	t.Run("required demo gif miss fails the deploy", func(t *testing.T) {
		root := t.TempDir()
		g := &fakeGetter{resp: map[string]struct {
			status int
			body   []byte
			err    error
		}{
			rawAssetsBase + "benchmarks/results.fragment.md":  {200, []byte("R"), nil},
			rawAssetsBase + "benchmarks/headline.fragment.md": {200, []byte("H"), nil},
			rawAssetsBase + "demo.gif":                        {404, []byte("nope"), nil},
		}}
		err := NewWithHTTP(osFS{}, g).PullSiteAssets(root)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "demo.gif")
		assert.Contains(t, err.Error(), "HTTP 404")
	})
}
