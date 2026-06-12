package release

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheckoutPinned covers checkoutPinned end to end: the happy path
// issues git init/fetch/checkout against a fresh dir, and each of the
// four failure points (mkdir, init, fetch, checkout) surfaces a
// wrapped error. The runner is faked so no real git or network runs.
func TestCheckoutPinned(t *testing.T) {
	const url, sha = "https://example.test/repo.git", "deadbeef"

	t.Run("happy path runs init, fetch, checkout", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "clone")
		tk := NewWithDeps(osFS{}, &fakeRunner{})
		require.NoError(t, tk.checkoutPinned(dir, url, sha))
		assert.DirExists(t, dir, "the clone dir is created before git init")
	})

	t.Run("mkdir failure surfaces wrapped", func(t *testing.T) {
		ff := newFakeFS()
		ff.failOnMkdirAllCall = 1
		err := NewWithDeps(ff, &fakeRunner{}).checkoutPinned(t.TempDir(), url, sha)
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
		assert.Contains(t, err.Error(), "mkdir")
	})

	t.Run("git init failure surfaces wrapped", func(t *testing.T) {
		err := NewWithDeps(osFS{}, &fakeRunner{failOnCall: 1}).
			checkoutPinned(filepath.Join(t.TempDir(), "c"), url, sha)
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
		assert.Contains(t, err.Error(), "git init")
	})

	t.Run("fetch failure surfaces wrapped", func(t *testing.T) {
		err := NewWithDeps(osFS{}, &fakeRunner{failOnCall: 2}).
			checkoutPinned(filepath.Join(t.TempDir(), "c"), url, sha)
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
		assert.Contains(t, err.Error(), "fetch")
	})

	t.Run("checkout failure surfaces wrapped", func(t *testing.T) {
		err := NewWithDeps(osFS{}, &fakeRunner{failOnCall: 3}).
			checkoutPinned(filepath.Join(t.TempDir(), "c"), url, sha)
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
		assert.Contains(t, err.Error(), "checkout")
	})
}

// TestBuildCorpora_NeutralBlock drives the neutral-corpus branch of
// buildCorpora with the repo corpus pre-materialized (so the
// git-ls-files block is skipped) and the runner faked. It covers the
// checkoutPinned-error propagation and the already-cloned skip plus
// the copyMarkdownTree fan-in that populates corpus_neutral.
func TestBuildCorpora_NeutralBlock(t *testing.T) {
	t.Run("checkoutPinned failure propagates", func(t *testing.T) {
		workdir := t.TempDir()
		// corpus_repo present → the git ls-files block is skipped;
		// corpus_neutral absent and rust-book absent → the first clone
		// runs checkoutPinned, whose git init fails.
		require.NoError(t, os.MkdirAll(filepath.Join(workdir, "corpus_repo"), 0o755))
		tk := NewWithDeps(osFS{}, &fakeRunner{failOnCall: 1})
		err := tk.buildCorpora(t.TempDir(), workdir)
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
		assert.Contains(t, err.Error(), "git init")
	})

	t.Run("already-cloned sources are copied into corpus_neutral", func(t *testing.T) {
		workdir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(workdir, "corpus_repo"), 0o755))
		// Pre-stage both clone dirs with a src/*.md so t.exists(dir) is
		// true (checkoutPinned is skipped via continue) and
		// copyMarkdownTree has markdown to fan in.
		touch(t, workdir, "rust-book/src/ch01.md")
		touch(t, workdir, "rust-ref/src/types.md")
		// The runner must never be called on this path.
		tk := NewWithDeps(osFS{}, &fakeRunner{failOnCall: 1})
		require.NoError(t, tk.buildCorpora(t.TempDir(), workdir))

		neutral := filepath.Join(workdir, "corpus_neutral")
		got, err := tk.countMarkdownFiles(neutral)
		require.NoError(t, err)
		assert.Equal(t, 2, got, "both staged neutral sources are copied in")
	})

	t.Run("copyMarkdownTree failure propagates", func(t *testing.T) {
		workdir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(workdir, "corpus_repo"), 0o755))
		// Both clone dirs exist (so checkoutPinned is skipped via
		// continue) and rust-book/src has a markdown file to copy, but an
		// injected WriteFile fault makes copyMarkdownTree's copyInto fail,
		// and buildCorpora surfaces it.
		touch(t, workdir, "rust-book/src/ch01.md")
		require.NoError(t, os.MkdirAll(filepath.Join(workdir, "rust-ref"), 0o755))
		ff := newFakeFS()
		ff.failOnWriteFileCall = 1
		err := NewWithFS(ff).buildCorpora(t.TempDir(), workdir)
		require.Error(t, err)
	})
}

// TestFinalizeBenchData covers the data-finalization tail of a bench
// run — promote JSON, write corpus_sizes.json, regenerate fragments —
// with the runner (python3) and, where needed, the FS faked, so none
// of hyperfine, the network, or a real corpus build is required.
func TestFinalizeBenchData(t *testing.T) {
	// stage builds an outDir holding the four hyperfine JSON exports
	// promoteBenchJSON expects, plus a materialized corpus pair for the
	// size count. It returns the four paths finalizeBenchData takes.
	stage := func(t *testing.T) (root, workdir, outDir, dataDir string) {
		t.Helper()
		root, workdir, outDir = t.TempDir(), t.TempDir(), t.TempDir()
		dataDir = filepath.Join(root, "data")
		require.NoError(t, os.MkdirAll(dataDir, 0o755))
		for _, n := range benchJSONNames {
			require.NoError(t, os.WriteFile(filepath.Join(outDir, n+".json"), []byte("{}"), 0o644))
		}
		touch(t, filepath.Join(workdir, "corpus_repo"), "a.md")
		touch(t, filepath.Join(workdir, "corpus_neutral"), "b.md")
		return
	}

	t.Run("happy path promotes, counts, regenerates", func(t *testing.T) {
		root, workdir, outDir, dataDir := stage(t)
		// The runner stands in for the python3 gen_fragments call.
		require.NoError(t, NewWithDeps(osFS{}, &fakeRunner{}).
			finalizeBenchData(root, workdir, outDir, dataDir))
		b, err := os.ReadFile(filepath.Join(dataDir, "corpus_sizes.json"))
		require.NoError(t, err)
		assert.Equal(t, "{\n  \"repo\": 1,\n  \"neutral\": 1\n}\n", string(b))
	})

	t.Run("promote failure propagates", func(t *testing.T) {
		root, workdir, outDir, dataDir := stage(t)
		ff := newFakeFS()
		ff.failOnMkdirAllCall = 1 // promoteBenchJSON's data-dir mkdir
		err := NewWithDeps(ff, &fakeRunner{}).finalizeBenchData(root, workdir, outDir, dataDir)
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
	})

	t.Run("corpus-size count failure propagates", func(t *testing.T) {
		root, workdir, outDir, dataDir := stage(t)
		ff := newFakeFS()
		ff.failOnReadDirCall = 1 // writeCorpusSizes' first corpus count
		err := NewWithDeps(ff, &fakeRunner{}).finalizeBenchData(root, workdir, outDir, dataDir)
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
		assert.Contains(t, err.Error(), "count markdown under")
	})

	t.Run("gen_fragments failure propagates", func(t *testing.T) {
		root, workdir, outDir, dataDir := stage(t)
		err := NewWithDeps(osFS{}, &fakeRunner{failOnCall: 1}).
			finalizeBenchData(root, workdir, outDir, dataDir)
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
		assert.Contains(t, err.Error(), "gen_fragments.py")
	})
}

// TestCountMarkdownFiles_ReadError covers the FS read-error branches of
// the recursive count: a failure reading the top-level corpus dir and a
// failure reading a nested subdirectory both surface a wrapped error.
func TestCountMarkdownFiles_ReadError(t *testing.T) {
	t.Run("top-level read failure surfaces wrapped", func(t *testing.T) {
		corpus := filepath.Join(t.TempDir(), "corpus")
		require.NoError(t, os.MkdirAll(corpus, 0o755))
		ff := newFakeFS()
		ff.failOnReadDirCall = 1
		_, err := NewWithFS(ff).countMarkdownFiles(corpus)
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
		assert.Contains(t, err.Error(), "count markdown under")
	})

	t.Run("nested read failure surfaces wrapped", func(t *testing.T) {
		corpus := filepath.Join(t.TempDir(), "corpus")
		require.NoError(t, os.MkdirAll(filepath.Join(corpus, "sub"), 0o755))
		ff := newFakeFS()
		ff.failOnReadDirCall = 2 // top dir reads ok; the sub dir fails
		_, err := NewWithFS(ff).countMarkdownFiles(corpus)
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
	})
}

// TestWriteCorpusSizes_Errors covers writeCorpusSizes's two failure
// branches: a count failure on a corpus (read fault) and a write
// failure persisting corpus_sizes.json.
func TestWriteCorpusSizes_Errors(t *testing.T) {
	t.Run("count failure propagates", func(t *testing.T) {
		workdir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(workdir, "corpus_repo"), 0o755))
		ff := newFakeFS()
		ff.failOnReadDirCall = 1 // counting corpus_repo fails
		err := NewWithFS(ff).writeCorpusSizes(workdir, t.TempDir())
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
		assert.Contains(t, err.Error(), "count markdown under")
	})

	t.Run("neutral count failure propagates", func(t *testing.T) {
		workdir := t.TempDir()
		// Both corpora present and flat (one ReadDir each), so the
		// repo count reads ok (call 1) and the neutral count read
		// (call 2) is the one faulted.
		require.NoError(t, os.MkdirAll(filepath.Join(workdir, "corpus_repo"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(workdir, "corpus_neutral"), 0o755))
		ff := newFakeFS()
		ff.failOnReadDirCall = 2
		err := NewWithFS(ff).writeCorpusSizes(workdir, t.TempDir())
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
		assert.Contains(t, err.Error(), "count markdown under")
	})

	t.Run("write failure surfaces wrapped", func(t *testing.T) {
		workdir := t.TempDir()
		// Both corpora present so the counts succeed; only the final
		// WriteFile of corpus_sizes.json is faulted.
		touch(t, filepath.Join(workdir, "corpus_repo"), "a.md")
		touch(t, filepath.Join(workdir, "corpus_neutral"), "b.md")
		ff := newFakeFS()
		ff.failOnWriteFileCall = 1
		dataDir := t.TempDir()
		err := NewWithFS(ff).writeCorpusSizes(workdir, dataDir)
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
		assert.Contains(t, err.Error(), "write ")
	})
}
