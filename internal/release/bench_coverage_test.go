package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// corruptTarGz returns a gzip stream whose decompressed payload is
// not a valid tar archive: the gzip layer reads fine, but the
// first tar.Next() fails. Drives extractTarGzBinary's read-tar
// error branch (distinct from the clean-EOF not-found branch the
// existing test covers).
func corruptTarGz(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, err := gz.Write(bytes.Repeat([]byte("not-a-valid-tar-header!!"), 100))
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

// truncatedMemberTarGz returns a gzip+tar stream whose single
// member's header declares a far larger Size than the bytes that
// actually follow, and whose tail is chopped so the member body is
// short. extractTarGzBinary's io.CopyN then trips on an unexpected
// EOF rather than a clean io.EOF.
func truncatedMemberTarGz(t *testing.T, binName string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: binName, Mode: 0o644, Size: 1 << 20, Typeflag: tar.TypeReg,
	}))
	_, err := tw.Write([]byte("short")) // 5 bytes for a 1 MiB-declared member
	require.NoError(t, err)
	require.NoError(t, gz.Flush())
	raw := buf.Bytes()
	return raw[:len(raw)-2] // chop the tail so the stream ends mid-member
}

// TestExtractTarGzBinary_ErrorBranches covers the read-tar and
// copy-extract failure paths that the happy-path tests in
// bench_test.go do not reach. The not-gzip and not-found branches
// are already covered there; these drive a corrupt tar header and a
// truncated member body.
func TestExtractTarGzBinary_ErrorBranches(t *testing.T) {
	t.Run("corrupt tar header errors", func(t *testing.T) {
		dst := filepath.Join(t.TempDir(), "mado")
		err := extractTarGzBinary(bytes.NewReader(corruptTarGz(t)), "mado", dst)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read tar")
	})

	t.Run("truncated member body errors during extract", func(t *testing.T) {
		dst := filepath.Join(t.TempDir(), "mado")
		err := extractTarGzBinary(bytes.NewReader(truncatedMemberTarGz(t, "mado")), "mado", dst)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "extract mado")
	})

	t.Run("unwritable destination errors on create", func(t *testing.T) {
		// A regular file as a path component makes os.OpenFile fail
		// with ENOTDIR even when the test runs as root, so the
		// create-output branch is reachable without chmod tricks.
		base := t.TempDir()
		regular := filepath.Join(base, "not-a-dir")
		require.NoError(t, os.WriteFile(regular, []byte("x"), 0o644))
		dst := filepath.Join(regular, "hyperfine") // parent is a file
		archive := makeTarGz(t, map[string]string{"hyperfine": "BIN"})
		err := extractTarGzBinary(bytes.NewReader(archive), "hyperfine", dst)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create "+dst)
	})
}

// TestPromoteBenchJSON_FSFaults drives the three filesystem error
// branches of promoteBenchJSON through the fault-injecting FS: a
// MkdirAll failure on the data dir, a non-NotExist read failure on
// a source export, and a WriteFile failure when promoting a
// successfully-read export.
func TestPromoteBenchJSON_FSFaults(t *testing.T) {
	t.Run("mkdir data dir failure", func(t *testing.T) {
		ff := newFakeFS()
		ff.failOnMkdirAllCall = 1
		err := NewWithFS(ff).promoteBenchJSON(t.TempDir(), filepath.Join(t.TempDir(), "data"))
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
		assert.Contains(t, err.Error(), "mkdir")
	})

	t.Run("non-NotExist read failure surfaces wrapped", func(t *testing.T) {
		// MkdirAll passes through to the real FS (call 1 ok); the
		// first ReadFile of corpus_repo.json fails with a generic
		// (non-IsNotExist) error, so promoteBenchJSON wraps it as
		// "read <src>" rather than the "not found" message.
		out := t.TempDir()
		ff := newFakeFS()
		ff.failOnReadFileCall = 1
		err := NewWithFS(ff).promoteBenchJSON(out, filepath.Join(t.TempDir(), "data"))
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
		assert.Contains(t, err.Error(), "read ")
		assert.NotContains(t, err.Error(), "not found")
	})

	t.Run("write failure on a read export", func(t *testing.T) {
		out := t.TempDir()
		// Stage every source export so ReadFile succeeds; the WriteFile
		// into the data dir then fails on the first promote.
		for _, n := range benchJSONNames {
			require.NoError(t, os.WriteFile(filepath.Join(out, n+".json"), []byte("{}"), 0o644))
		}
		ff := newFakeFS()
		ff.failOnWriteFileCall = 1
		err := NewWithFS(ff).promoteBenchJSON(out, filepath.Join(t.TempDir(), "data"))
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
		assert.Contains(t, err.Error(), "write ")
	})
}

// TestToolkit_exists covers the os.Stat wrapper both ways: an
// existing path reports true, a missing one reports false.
func TestToolkit_exists(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "here")
	require.NoError(t, os.WriteFile(present, []byte("x"), 0o644))

	tk := New()
	assert.True(t, tk.exists(present), "an existing file must report present")
	assert.True(t, tk.exists(dir), "an existing directory must report present")
	assert.False(t, tk.exists(filepath.Join(dir, "missing")),
		"a missing path must report absent")
}

// TestToolkit_copyInto copies one file verbatim, creating parent
// directories, and surfaces a read error on a missing source and a
// mkdir error when a parent path component is a regular file.
func TestToolkit_copyInto(t *testing.T) {
	tk := New()

	t.Run("copies content and creates parents", func(t *testing.T) {
		src := filepath.Join(t.TempDir(), "src.md")
		require.NoError(t, os.WriteFile(src, []byte("CONTENT"), 0o644))
		dst := filepath.Join(t.TempDir(), "nested", "deep", "out.md")
		require.NoError(t, tk.copyInto(src, dst))
		got, err := os.ReadFile(dst)
		require.NoError(t, err)
		assert.Equal(t, "CONTENT", string(got))
	})

	t.Run("missing source errors", func(t *testing.T) {
		err := tk.copyInto(filepath.Join(t.TempDir(), "nope.md"),
			filepath.Join(t.TempDir(), "out.md"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read ")
	})

	t.Run("mkdir failure on regular-file parent", func(t *testing.T) {
		src := filepath.Join(t.TempDir(), "src.md")
		require.NoError(t, os.WriteFile(src, []byte("x"), 0o644))
		base := t.TempDir()
		regular := filepath.Join(base, "file")
		require.NoError(t, os.WriteFile(regular, []byte("x"), 0o644))
		// dst's parent dir would be <regular>/sub, but <regular> is a
		// file, so MkdirAll fails with ENOTDIR regardless of euid.
		dst := filepath.Join(regular, "sub", "out.md")
		err := tk.copyInto(src, dst)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mkdir")
	})

	t.Run("write failure surfaces wrapped", func(t *testing.T) {
		// ReadFile and MkdirAll pass through to the real FS; only the
		// final WriteFile is faulted, so copyInto wraps it as "write".
		src := filepath.Join(t.TempDir(), "src.md")
		require.NoError(t, os.WriteFile(src, []byte("x"), 0o644))
		ff := newFakeFS()
		ff.failOnWriteFileCall = 1
		err := NewWithFS(ff).copyInto(src, filepath.Join(t.TempDir(), "out.md"))
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
		assert.Contains(t, err.Error(), "write ")
	})
}

// TestToolkit_copyMarkdownTree copies only *.md files under the
// source root, reproducing their relative layout, skips non-markdown
// files and directories, propagates a WalkDir error on a missing
// root, and propagates a copyInto failure.
func TestToolkit_copyMarkdownTree(t *testing.T) {
	tk := New()

	t.Run("copies markdown and skips the rest", func(t *testing.T) {
		src := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(src, "sub"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(src, "a.md"), []byte("A"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(src, "sub", "b.md"), []byte("B"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(src, "skip.txt"), []byte("T"), 0o644))

		dst := filepath.Join(t.TempDir(), "corpus")
		require.NoError(t, tk.copyMarkdownTree(src, dst))

		// The source path is reproduced under dst (leading separator
		// stripped), so each .md lands at dst/<abs-src-without-slash>.
		relA := filepath.Join(dst, src, "a.md")
		got, err := os.ReadFile(relA)
		require.NoError(t, err)
		assert.Equal(t, "A", string(got))
		gotB, err := os.ReadFile(filepath.Join(dst, src, "sub", "b.md"))
		require.NoError(t, err)
		assert.Equal(t, "B", string(gotB))
		// The .txt file must not have been copied anywhere under dst.
		_, err = os.Stat(filepath.Join(dst, src, "skip.txt"))
		assert.True(t, os.IsNotExist(err), "non-markdown files are skipped")
	})

	t.Run("missing source root surfaces a walk error", func(t *testing.T) {
		err := tk.copyMarkdownTree(filepath.Join(t.TempDir(), "absent"),
			filepath.Join(t.TempDir(), "corpus"))
		require.Error(t, err)
	})

	t.Run("copyInto failure propagates", func(t *testing.T) {
		// A markdown source exists, but the destination root is itself
		// a regular file, so copyInto's MkdirAll fails with ENOTDIR.
		src := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(src, "a.md"), []byte("A"), 0o644))
		base := t.TempDir()
		fileDst := filepath.Join(base, "dst-is-a-file")
		require.NoError(t, os.WriteFile(fileDst, []byte("x"), 0o644))
		err := tk.copyMarkdownTree(src, fileDst)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mkdir")
	})
}

// TestToolkit_Bench_EarlyErrors covers the two Bench failure
// branches reachable without a network, the hyperfine binary,
// npm, or python: the working-directory mkdir failure and the
// `go build` failure. The remaining orchestration (tool fetch,
// markdownlint install, corpora clone, hyperfine, fragment
// regeneration) shells out to external tooling and cannot be
// exercised tests-only — see the report.
func TestToolkit_Bench_EarlyErrors(t *testing.T) {
	t.Run("workdir mkdir failure", func(t *testing.T) {
		ff := newFakeFS()
		ff.failOnMkdirAllCall = 1 // first MkdirAll is binDir
		err := NewWithFS(ff).Bench(t.TempDir(), t.TempDir())
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
		assert.Contains(t, err.Error(), "mkdir")
	})

	t.Run("go build failure", func(t *testing.T) {
		// Real FS so binDir/outDir are created; the runner fails the
		// first command, which is `go build -o <bin> ./cmd/mdsmith`.
		tk := NewWithDeps(osFS{}, &fakeRunner{failOnCall: 1})
		err := tk.Bench(t.TempDir(), t.TempDir())
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
		assert.Contains(t, err.Error(), "build mdsmith")
	})

	t.Run("empty workdir defaults are applied without panicking", func(t *testing.T) {
		// An empty workdir must fall back to defaultBenchWorkdir; fail
		// the mkdir so the test never writes outside the temp area and
		// never reaches the network-bound steps.
		ff := newFakeFS()
		ff.failOnMkdirAllCall = 1
		err := NewWithFS(ff).Bench(t.TempDir(), "")
		require.Error(t, err)
		assert.ErrorIs(t, err, errInjected)
	})
}
