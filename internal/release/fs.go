package release

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// FS is the small filesystem surface the release toolkit uses.
// Production paths use osFS (delegating to the real syscalls);
// tests inject a fault-injecting fake to exercise IO error
// branches that real filesystems don't reliably trigger
// (mid-pipeline mkdir failure, rename target on a non-directory,
// disk-full WriteFile, etc.).
//
// We intentionally keep this independent of stdlib's read-only
// `io/fs.FS`. The release toolkit needs Mkdir/Write/Rename
// alongside Read/Stat, and stdlib's interface only covers reads.
type FS interface {
	// Stat returns the FileInfo for the named file, mirroring
	// os.Stat.
	Stat(name string) (os.FileInfo, error)
	// ReadFile reads the named file, mirroring os.ReadFile.
	ReadFile(name string) ([]byte, error)
	// WriteFile writes data to the named file, mirroring
	// os.WriteFile.
	WriteFile(name string, data []byte, perm fs.FileMode) error
	// ReadDir reads the named directory, mirroring os.ReadDir.
	ReadDir(name string) ([]os.DirEntry, error)
	// MkdirAll creates name and any parents, mirroring
	// os.MkdirAll.
	MkdirAll(path string, perm fs.FileMode) error
	// MkdirTemp creates a new temporary directory, mirroring
	// os.MkdirTemp.
	MkdirTemp(dir, pattern string) (string, error)
	// Rename renames (moves) oldpath to newpath, mirroring
	// os.Rename.
	Rename(oldpath, newpath string) error
	// RemoveAll removes path and any children, mirroring
	// os.RemoveAll.
	RemoveAll(path string) error
}

// osFS is the production filesystem implementation. Each method
// delegates straight to the corresponding os.* call.
type osFS struct{}

func (osFS) Stat(name string) (os.FileInfo, error) { return os.Stat(name) }
func (osFS) ReadFile(name string) ([]byte, error)  { return os.ReadFile(name) }
func (osFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(name, data, perm)
}
func (osFS) ReadDir(name string) ([]os.DirEntry, error) { return os.ReadDir(name) }
func (osFS) MkdirAll(path string, perm fs.FileMode) error {
	return os.MkdirAll(path, perm)
}
func (osFS) MkdirTemp(dir, pattern string) (string, error) {
	return os.MkdirTemp(dir, pattern)
}
func (osFS) Rename(oldpath, newpath string) error { return os.Rename(oldpath, newpath) }
func (osFS) RemoveAll(path string) error          { return os.RemoveAll(path) }

// Runner runs an external command. The release toolkit shells
// out to `python -m build` and `python -m wheel tags` for the
// PyPI publish path; tests inject a fake Runner to cover those
// branches without putting python on PATH.
type Runner interface {
	// RunCommand executes name+args in dir, with stdout/stderr
	// inherited from the calling process. Mirrors exec.Cmd.Run
	// semantics: a non-zero exit returns *exec.ExitError; other
	// failures (binary not found, IO) return their underlying
	// error.
	RunCommand(dir, name string, args ...string) error
}

// osRunner is the production Runner. It builds an exec.Cmd,
// pipes stdout/stderr to the calling process, and delegates Run.
type osRunner struct{}

func (osRunner) RunCommand(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...) //nolint:gosec // CI-only invocation, args are constants
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// HTTPGetter is the one-call HTTP surface the release toolkit
// uses to fetch pinned benchmark tool tarballs and the published
// benchmark numbers / demo GIF from the orphan `assets` branch.
// Production uses osHTTPGetter (net/http); tests inject a fake so
// the assets-pull and download-integrity branches run without a
// network.
type HTTPGetter interface {
	// Get fetches url and returns the HTTP status code and the
	// fully-read body. A transport-level failure (DNS, refused,
	// timeout) returns a non-nil error with status 0; an HTTP
	// error response (404, 500, …) returns the status and the
	// body with a nil error, so callers decide per-asset whether
	// a miss is fatal or a keep-the-committed-copy fallback.
	Get(url string) (status int, body []byte, err error)
}

// osHTTPGetter is the production HTTPGetter. The client carries a
// generous timeout: the benchmark tool tarballs are a few MB and
// CI runners are occasionally slow, but an unbounded wait would
// hang a stuck publish/deploy indefinitely.
type osHTTPGetter struct{}

func (osHTTPGetter) Get(url string) (int, []byte, error) {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url) //nolint:gosec // CI-only fetch of a constant-derived URL
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read body: %w", err)
	}
	return resp.StatusCode, body, nil
}

// Toolkit owns a configured FS, Runner, and HTTPGetter and exposes
// the release helpers (Stamp, Check, BuildNpmPlatforms,
// BuildWheels, Bench, PullSiteAssets) as methods. `New()` returns
// a Toolkit backed by the real OS for all three; tests can use
// `NewWithFS(fakeFS)`, `NewWithDeps`, or `NewWithHTTP` to drive
// error paths that the OS does not expose.
type Toolkit struct {
	fs     FS
	runner Runner
	http   HTTPGetter
}

// New returns a Toolkit backed by the real OS filesystem, command
// runner, and HTTP client.
func New() *Toolkit {
	return &Toolkit{fs: osFS{}, runner: osRunner{}, http: osHTTPGetter{}}
}

// NewWithFS returns a Toolkit with a custom FS and the OS-backed
// Runner/HTTPGetter. Convenience helper for tests that only need
// IO faults.
func NewWithFS(fsys FS) *Toolkit {
	return &Toolkit{fs: fsys, runner: osRunner{}, http: osHTTPGetter{}}
}

// NewWithDeps returns a Toolkit with custom FS and Runner and the
// OS-backed HTTPGetter. Used by tests that exercise both IO and
// command-execution faults.
func NewWithDeps(fsys FS, runner Runner) *Toolkit {
	return &Toolkit{fs: fsys, runner: runner, http: osHTTPGetter{}}
}

// NewWithHTTP returns a Toolkit with a custom FS and HTTPGetter and
// the OS-backed Runner. Used by the PullSiteAssets tests to drive
// the build-time assets fetch without a network.
func NewWithHTTP(fsys FS, getter HTTPGetter) *Toolkit {
	return &Toolkit{fs: fsys, runner: osRunner{}, http: getter}
}
