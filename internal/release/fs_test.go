package release

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helperModeEnv selects the behaviour of TestHelperProcess when this
// test binary is re-exec'd as a stand-in command. RunCommand sets no
// cmd.Env (fs.go), so the child inherits the value a subtest sets with
// t.Setenv.
const helperModeEnv = "MDSMITH_RUNCOMMAND_HELPER"

// TestHelperProcess is not a real test: it is the executable the
// RunCommand subtests exec in place of `true`/`false`/`test -f`. It is a
// no-op during a normal `go test` run (helperModeEnv unset) and only
// acts when re-exec'd with that env set, mimicking one command's
// observable exit status / cwd behaviour. This keeps RunCommand's
// coverage cross-platform instead of depending on POSIX userland that
// is absent on Windows.
func TestHelperProcess(t *testing.T) {
	switch os.Getenv(helperModeEnv) {
	case "":
		return // normal run: not the helper child
	case "exit0":
		os.Exit(0)
	case "exit1":
		os.Exit(1)
	case "marker":
		// Succeed only when ./marker exists relative to the working
		// directory RunCommand set via cmd.Dir.
		if _, err := os.Stat("marker"); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	default:
		os.Exit(2)
	}
}

// TestOsRunner_RunCommand drives the production Runner via the
// helper-process pattern: it re-execs this test binary
// (TestHelperProcess) as a harmless stand-in command, so a zero exit
// returns nil, a non-zero exit returns *exec.ExitError, cmd.Dir is
// honoured, and a missing binary returns a not-found error — exercised
// identically on every platform, with no POSIX `true`/`false`/`test`.
func TestOsRunner_RunCommand(t *testing.T) {
	r := osRunner{}
	self := os.Args[0]
	runHelper := []string{"-test.run=^TestHelperProcess$"}

	t.Run("zero exit returns nil", func(t *testing.T) {
		t.Setenv(helperModeEnv, "exit0")
		require.NoError(t, r.RunCommand("", self, runHelper...))
	})

	t.Run("non-zero exit returns ExitError", func(t *testing.T) {
		t.Setenv(helperModeEnv, "exit1")
		err := r.RunCommand("", self, runHelper...)
		require.Error(t, err)
		var exitErr *exec.ExitError
		assert.True(t, errors.As(err, &exitErr),
			"a non-zero exit must surface as *exec.ExitError, got %T", err)
	})

	t.Run("runs in the requested directory", func(t *testing.T) {
		t.Setenv(helperModeEnv, "marker")
		// The marker exists only inside dir, so a success proves
		// cmd.Dir was honoured; a different cwd fails the same check.
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "marker"), []byte("x"), 0o644))
		assert.NoError(t, r.RunCommand(dir, self, runHelper...))
		other := t.TempDir()
		assert.Error(t, r.RunCommand(other, self, runHelper...))
	})

	t.Run("missing binary returns an error", func(t *testing.T) {
		err := r.RunCommand("", "mdsmith-no-such-binary-on-path-xyz")
		require.Error(t, err)
		assert.ErrorIs(t, err, exec.ErrNotFound)
	})
}

// TestOsHTTPGetter_Get stands up an in-process httptest server to
// cover every branch of the production HTTP surface: a 200 returns
// the status and the fully-read body, a non-2xx status is returned
// verbatim with a nil error (callers decide per-asset), a
// truncated body surfaces a wrapped read error, and a refused
// connection returns status 0 with a transport error.
func TestOsHTTPGetter_Get(t *testing.T) {
	g := osHTTPGetter{}

	t.Run("200 returns status and body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("HELLO-BODY"))
		}))
		defer srv.Close()
		status, body, err := g.Get(srv.URL)
		require.NoError(t, err)
		assert.Equal(t, 200, status)
		assert.Equal(t, "HELLO-BODY", string(body))
	})

	t.Run("non-200 returns status and body with nil error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "missing", http.StatusNotFound)
		}))
		defer srv.Close()
		status, body, err := g.Get(srv.URL)
		require.NoError(t, err)
		assert.Equal(t, http.StatusNotFound, status)
		assert.Contains(t, string(body), "missing")
	})

	t.Run("truncated body surfaces a read error", func(t *testing.T) {
		// Promise 100 bytes via Content-Length but write 5 and slam
		// the connection shut, so io.ReadAll trips on a short read.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(200)
			_, _ = w.Write([]byte("short"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			if hj, ok := w.(http.Hijacker); ok {
				if conn, _, err := hj.Hijack(); err == nil {
					_ = conn.Close()
				}
			}
		}))
		defer srv.Close()
		status, body, err := g.Get(srv.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read body")
		assert.Equal(t, 200, status, "the status is still surfaced on a read failure")
		assert.Nil(t, body)
	})

	t.Run("refused connection returns status 0 and a transport error", func(t *testing.T) {
		// Bind a port, learn its address, then close the listener so
		// the subsequent dial is refused without touching the network.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		url := "http://" + ln.Addr().String()
		require.NoError(t, ln.Close())
		status, body, err := g.Get(url)
		require.Error(t, err)
		assert.Equal(t, 0, status)
		assert.Nil(t, body)
	})
}
