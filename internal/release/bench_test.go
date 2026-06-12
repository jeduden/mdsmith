package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
	assert.GreaterOrEqual(t, len(tools), 5,
		"gomarklint, hyperfine, mado, panache, rumdl are all pinned")
	names := make(map[string]bool, len(tools))
	for _, tool := range tools {
		names[tool.Name] = true
	}
	assert.True(t, names["gomarklint"],
		"gomarklint is pinned so the post-merge benchmark run measures it")

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

// TestRunHyperfine pins the exact hyperfine invocation: one
// comparison pass per corpus naming every pinned tool (mado,
// rumdl, panache, gomarklint) plus the two mdsmith rows, and a
// separate markdownlint-cli2 pass. A tool dropped from the
// command line — while still fetched via the manifest — would
// silently vanish from the published tables; this pins the
// wiring, not just the pin. The error subtests cover both
// per-pass failure branches.
func TestRunHyperfine(t *testing.T) {
	t.Run("commands pin the tool set", func(t *testing.T) {
		rec := &recordingRunner{}
		tk := NewWithDeps(osFS{}, rec)
		require.NoError(t, tk.runHyperfine(
			"/bin", "/out", "/work", "/bin/mdsmith", "/mdl/markdownlint-cli2", "/root"))

		require.Len(t, rec.calls, 4, "two hyperfine passes per corpus, two corpora")
		parity := "/root/docs/research/benchmarks/bench-parity.mdsmith.yml"
		for i, corpus := range []string{"corpus_repo", "corpus_neutral"} {
			cpath := "/work/" + corpus
			main := rec.calls[2*i]
			assert.Equal(t, "/bin/hyperfine", main.name)
			joined := strings.Join(main.args, " ")
			assert.Contains(t, joined, "--command-name mado /bin/mado check "+cpath)
			assert.Contains(t, joined,
				"--command-name rumdl /bin/rumdl check --no-cache "+cpath)
			assert.Contains(t, joined,
				"--command-name panache /bin/panache lint --no-cache "+cpath)
			assert.Contains(t, joined,
				"--command-name gomarklint /bin/gomarklint "+cpath)
			assert.Contains(t, joined,
				"--command-name mdsmith-parity /bin/mdsmith check -c "+parity+" "+cpath)
			assert.Contains(t, joined, "--command-name mdsmith /bin/mdsmith check "+cpath)
			assert.Contains(t, joined, "--export-json /out/"+corpus+".json")

			mdl := rec.calls[2*i+1]
			assert.Equal(t, "/bin/hyperfine", mdl.name)
			assert.Contains(t, strings.Join(mdl.args, " "),
				"--command-name markdownlint-cli2 /mdl/markdownlint-cli2 '"+cpath+"/**/*.md'")
		}
	})

	t.Run("comparison pass failure propagates", func(t *testing.T) {
		tk := NewWithDeps(osFS{}, &fakeRunner{failOnCall: 1})
		err := tk.runHyperfine("/bin", "/out", "/work", "/bin/mdsmith", "/mdl", "/root")
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
		assert.Contains(t, err.Error(), "hyperfine corpus_repo")
	})

	t.Run("markdownlint pass failure propagates", func(t *testing.T) {
		tk := NewWithDeps(osFS{}, &fakeRunner{failOnCall: 2})
		err := tk.runHyperfine("/bin", "/out", "/work", "/bin/mdsmith", "/mdl", "/root")
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
		assert.Contains(t, err.Error(), "markdownlint")
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

// touch writes an empty file at dir/rel, creating parents. Helper
// for staging a fake materialized corpus tree.
func touch(t *testing.T, dir, rel string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, nil, 0o644))
}

// TestCountMarkdownFiles checks that the corpus file count is read
// from the materialized tree: every nested *.md / *.markdown counts,
// non-Markdown is ignored, and a missing tree counts zero rather than
// erroring (a corpus may legitimately be empty before it is built).
func TestCountMarkdownFiles(t *testing.T) {
	root := t.TempDir()
	corpus := filepath.Join(root, "corpus")
	touch(t, corpus, "a.md")
	touch(t, corpus, "nested/deep/b.md")
	touch(t, corpus, "c.markdown")
	touch(t, corpus, "README.txt")  // not Markdown
	touch(t, corpus, "notes/d.rst") // not Markdown

	n, err := New().countMarkdownFiles(corpus)
	require.NoError(t, err)
	assert.Equal(t, 3, n, "two .md plus one .markdown, .txt/.rst ignored")

	// A path that was never built counts zero, not an error: the
	// neutral corpus does not exist until cloned.
	zero, err := New().countMarkdownFiles(filepath.Join(root, "absent"))
	require.NoError(t, err)
	assert.Equal(t, 0, zero)
}

// TestWriteCorpusSizes is the heart of the dynamic-count fix: the
// bench counts the Markdown actually placed in each corpus and writes
// corpus_sizes.json — the file gen_fragments.py reads instead of a
// hardcoded literal — with the {"repo","neutral"} keys and a stable,
// trailing-newline form so a no-op rerun produces no diff.
func TestWriteCorpusSizes(t *testing.T) {
	workdir := t.TempDir()
	touch(t, filepath.Join(workdir, "corpus_repo"), "docs/x.md")
	touch(t, filepath.Join(workdir, "corpus_repo"), "y.markdown")
	touch(t, filepath.Join(workdir, "corpus_repo"), "z.md")
	touch(t, filepath.Join(workdir, "corpus_neutral"), "rust-book/src/ch01.md")
	touch(t, filepath.Join(workdir, "corpus_neutral"), "rust-ref/src/types.md")

	dataDir := filepath.Join(t.TempDir(), "data")
	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	require.NoError(t, New().writeCorpusSizes(workdir, dataDir))

	raw, err := os.ReadFile(filepath.Join(dataDir, "corpus_sizes.json"))
	require.NoError(t, err)

	var got corpusSizes
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, 3, got.Repo, "three repo-corpus Markdown files")
	assert.Equal(t, 2, got.Neutral, "two neutral-corpus Markdown files")

	// The on-disk shape is exactly what gen_fragments.py reads: the
	// two keys it requires, 2-space indented, trailing newline, and
	// byte-identical across repeated writes (idempotent gate).
	want := "{\n  \"repo\": 3,\n  \"neutral\": 2\n}\n"
	assert.Equal(t, want, string(raw))
	require.NoError(t, New().writeCorpusSizes(workdir, dataDir))
	raw2, err := os.ReadFile(filepath.Join(dataDir, "corpus_sizes.json"))
	require.NoError(t, err)
	assert.Equal(t, raw, raw2, "a second write is byte-identical")
}

// TestNeutralCorpusPinned documents the reproducibility pin (task 3):
// the neutral corpus is checked out at fixed upstream commits, not the
// moving default-branch tip, so its content does not drift run to run.
// The SHAs are full 40-hex git object IDs; a stray edit that blanks or
// truncates one trips here.
func TestNeutralCorpusPinned(t *testing.T) {
	full := regexp.MustCompile(`^[0-9a-f]{40}$`)
	assert.Regexp(t, full, rustBookPinnedSHA, "rust-lang/book pin is a full SHA")
	assert.Regexp(t, full, rustRefPinnedSHA, "rust-lang/reference pin is a full SHA")
	assert.NotEqual(t, rustBookPinnedSHA, rustRefPinnedSHA,
		"the two corpora pin distinct commits")
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

// TestPullSiteAssets covers the build-time asset pull. Only the
// demo GIF is fetched now: the cross-tool benchmark numbers come
// from the committed in-repo snapshot (refreshed via run.sh,
// reviewed in a PR), so the noisy per-merge benchmark.yml run never
// moves the published figures. A 200 writes the GIF as a first-party
// asset; a required miss fails the deploy. (The transport-error and
// FS-fault branches are covered in siteassets_coverage_test.go.)
func TestPullSiteAssets(t *testing.T) {
	t.Run("200 writes the demo gif", func(t *testing.T) {
		root := t.TempDir()
		g := &fakeGetter{resp: map[string]struct {
			status int
			body   []byte
			err    error
		}{
			rawAssetsBase + "demo.gif": {200, []byte("GIF89a-bytes"), nil},
		}}
		require.NoError(t, NewWithHTTP(osFS{}, g).PullSiteAssets(root))

		gif, err := os.ReadFile(filepath.Join(root, "website", "static", "img", "demo.gif"))
		require.NoError(t, err)
		assert.Equal(t, "GIF89a-bytes", string(gif))
	})

	t.Run("benchmark fragments are never fetched", func(t *testing.T) {
		root := t.TempDir()
		g := &fakeGetter{resp: map[string]struct {
			status int
			body   []byte
			err    error
		}{
			rawAssetsBase + "demo.gif": {200, []byte("GIF"), nil},
		}}
		require.NoError(t, NewWithHTTP(osFS{}, g).PullSiteAssets(root))

		// The committed snapshot is the source of truth; the pull
		// must not even request the fragments off the assets branch.
		for _, u := range g.calls {
			assert.NotContains(t, u, "benchmarks/",
				"benchmark fragments come from the committed snapshot, not the assets branch")
		}
	})

	t.Run("required demo gif miss fails the deploy", func(t *testing.T) {
		root := t.TempDir()
		g := &fakeGetter{resp: map[string]struct {
			status int
			body   []byte
			err    error
		}{
			rawAssetsBase + "demo.gif": {404, []byte("nope"), nil},
		}}
		err := NewWithHTTP(osFS{}, g).PullSiteAssets(root)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "demo.gif")
		assert.Contains(t, err.Error(), "HTTP 404")
	})
}
