package corpus

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSource_CloneAndCacheHit(t *testing.T) {
	t.Parallel()

	repoPath, commit := makeBareRepo(t)
	cacheDir := t.TempDir()
	runner := &recordingRunner{delegate: execGitRunner{}}
	source := SourceConfig{
		Name:       "seed",
		Repository: repoPath,
		Root:       "docs",
		CommitSHA:  commit,
	}

	first, err := resolveSourceWithRunner(source, cacheDir, runner)
	if err != nil {
		t.Fatalf("resolve first: %v", err)
	}
	if _, err := os.Stat(filepath.Join(first, "guide.md")); err != nil {
		t.Fatalf("expected collected file in resolved root: %v", err)
	}

	second, err := resolveSourceWithRunner(source, cacheDir, runner)
	if err != nil {
		t.Fatalf("resolve second: %v", err)
	}
	if first != second {
		t.Fatalf("resolved root mismatch: %q vs %q", first, second)
	}
	if got := runner.countCommand("clone"); got != 1 {
		t.Fatalf("clone command count = %d, want 1", got)
	}
}

func TestResolveSource_MissingCommit(t *testing.T) {
	t.Parallel()

	repoPath, _ := makeBareRepo(t)
	runner := &recordingRunner{delegate: execGitRunner{}}
	_, err := resolveSourceWithRunner(SourceConfig{
		Name:       "seed",
		Repository: repoPath,
		Root:       "docs",
		CommitSHA:  "0000000000000000000000000000000000000000",
	}, t.TempDir(), runner)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "commit") {
		t.Fatalf("expected missing commit error, got %v", err)
	}
}

func TestResolveSource_InvalidRepository(t *testing.T) {
	t.Parallel()

	_, err := resolveSourceWithRunner(SourceConfig{
		Name:       "seed",
		Repository: filepath.Join(t.TempDir(), "missing.git"),
		Root:       "docs",
		CommitSHA:  "abc123",
	}, t.TempDir(), &recordingRunner{delegate: execGitRunner{}})
	if err == nil {
		t.Fatal("expected invalid repository error")
	}
}

func TestResolveSource_LocalPathOverrideSkipsGit(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "local")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}

	runner := &recordingRunner{delegate: errRunner{}}
	resolved, err := resolveSourceWithRunner(SourceConfig{
		Name:       "seed",
		Repository: "github.com/acme/seed",
		Root:       root,
		CommitSHA:  "abc123",
	}, t.TempDir(), runner)
	if err != nil {
		t.Fatalf("resolve local override: %v", err)
	}
	if resolved != root {
		t.Fatalf("resolved root = %q, want %q", resolved, root)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("expected no git calls, got %d", len(runner.calls))
	}
}

func makeBareRepo(t *testing.T) (repoPath string, commit string) {
	t.Helper()

	root := t.TempDir()
	work := filepath.Join(root, "work")
	repo := filepath.Join(root, "repo.git")

	runGit(t, "init", work)
	runGitInDir(t, work, "config", "user.name", "Test User")
	runGitInDir(t, work, "config", "user.email", "test@example.com")
	runGitInDir(t, work, "config", "commit.gpgsign", "false")
	runGitInDir(t, work, "config", "tag.gpgsign", "false")
	runGitInDir(t, work, "config", "gpg.format", "openpgp")

	docsDir := filepath.Join(work, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	guidePath := filepath.Join(docsDir, "guide.md")
	guideContent := []byte("# Guide\n\nword word word word word\n")
	if err := os.WriteFile(guidePath, guideContent, 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}

	runGitInDir(t, work, "add", ".")
	runGitInDir(t, work, "commit", "-m", "seed")
	commit = strings.TrimSpace(runGitInDir(t, work, "rev-parse", "HEAD"))

	runGit(t, "clone", "--bare", work, repo)
	return repo, commit
}

func runGit(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
	return string(out)
}

func runGitInDir(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git -C %s %v failed: %v\n%s", dir, args, err, string(out))
	}
	return string(out)
}

type recordingRunner struct {
	delegate GitRunner
	calls    [][]string
}

func (r *recordingRunner) Run(args []string) ([]byte, error) {
	copied := append([]string(nil), args...)
	r.calls = append(r.calls, copied)
	return r.delegate.Run(args)
}

func (r *recordingRunner) countCommand(name string) int {
	count := 0
	for _, call := range r.calls {
		for _, token := range call {
			if token == name {
				count++
				break
			}
		}
	}
	return count
}

type errRunner struct{}

func (errRunner) Run(args []string) ([]byte, error) {
	return nil, exec.ErrNotFound
}

// --- classifyGitError ---

// TestClassifyGitError pins every branch of the error classifier:
// the four pattern groups (repo missing, commit missing, network)
// each rewrite the upstream message; the default branch passes it
// through unchanged. The ResolveSource end-to-end test only ever
// drives the "commit not found" path on a synthetic bare repo, so
// the other branches were uncovered.
func TestClassifyGitError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "repository not found",
			in:   "fatal: repository not found",
			want: "repository not found or inaccessible: origin"},
		{name: "could not read from remote repository",
			in:   "Could not read from remote repository",
			want: "repository not found or inaccessible: origin"},
		{name: "couldnt find remote ref",
			in:   "couldn't find remote ref deadbeef",
			want: "commit not found: deadbeef"},
		{name: "not our ref",
			in:   "fatal: not our ref deadbeef",
			want: "commit not found: deadbeef"},
		{name: "failed to connect",
			in:   "failed to connect to host",
			want: "network error while accessing origin"},
		{name: "timed out",
			in:   "operation timed out",
			want: "network error while accessing origin"},
		{name: "could not resolve host",
			in:   "could not resolve host: github.com",
			want: "network error while accessing origin"},
		{name: "default passthrough",
			in:   "permission denied",
			want: "permission denied"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := classifyGitError(
				&stringErr{c.in}, "origin", "deadbeef")
			if got := err.Error(); got != c.want {
				t.Errorf("classifyGitError(%q) = %q, want %q",
					c.in, got, c.want)
			}
		})
	}
}

type stringErr struct{ s string }

func (e *stringErr) Error() string { return e.s }

// --- reportSourceProgress ---

// TestReportSourceProgress pins both branches: nil callback is a
// no-op (must not panic); non-nil callback receives the formatted
// message. The Resolve path drives nil callbacks via tests that
// pass no progress function; this asserts the formatting too.
func TestReportSourceProgress(t *testing.T) {
	t.Parallel()
	t.Run("nil callback is no-op", func(t *testing.T) {
		// Must not panic.
		reportSourceProgress(nil, "ignored: %s", "x")
	})
	t.Run("non-nil callback receives formatted message", func(t *testing.T) {
		var got string
		reportSourceProgress(func(s string) { got = s }, "seed %s (%d)", "alpha", 7)
		if got != "seed alpha (7)" {
			t.Errorf("got %q, want %q", got, "seed alpha (7)")
		}
	})
}

// --- cachedCommitExists ---

// stubRunner returns the configured (data, err) on every Run call.
type stubRunner struct {
	out []byte
	err error
}

func (s stubRunner) Run([]string) ([]byte, error) { return s.out, s.err }

// TestCachedCommitExists pins every branch the helper takes: the
// happy path (commit found), the two "missing object" variants
// git emits ("Not a valid object name", "invalid object",
// "unknown revision"), and the fallthrough wrap for any other
// error string.
func TestCachedCommitExists(t *testing.T) {
	t.Parallel()
	t.Run("commit found", func(t *testing.T) {
		ok, err := cachedCommitExists("/repo", "abc", stubRunner{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Errorf("expected ok=true")
		}
	})
	t.Run("not a valid object name returns false", func(t *testing.T) {
		ok, err := cachedCommitExists("/repo", "abc",
			stubRunner{err: &stringErr{"fatal: Not a valid object name"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Errorf("expected ok=false")
		}
	})
	t.Run("invalid object returns false", func(t *testing.T) {
		ok, err := cachedCommitExists("/repo", "abc",
			stubRunner{err: &stringErr{"git: invalid object 123"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Errorf("expected ok=false")
		}
	})
	t.Run("unknown revision returns false", func(t *testing.T) {
		ok, err := cachedCommitExists("/repo", "abc",
			stubRunner{err: &stringErr{"fatal: unknown revision or path"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Errorf("expected ok=false")
		}
	})
	t.Run("other errors wrap with commit", func(t *testing.T) {
		_, err := cachedCommitExists("/repo", "deadbeef",
			stubRunner{err: &stringErr{"permission denied"}})
		if err == nil {
			t.Fatal("expected error to wrap")
		}
		if !strings.Contains(err.Error(), "deadbeef") {
			t.Errorf("err %q must name the commit", err)
		}
		if !strings.Contains(err.Error(), "permission denied") {
			t.Errorf("err %q must include upstream message", err)
		}
	})
}

// --- validateRepoRoot ---

// TestValidateRepoRoot pins the missing-root branch: when the
// resolved root does not exist, the helper must produce a
// commit-aware error string naming the configured root. The
// happy path is exercised by ResolveSource end-to-end; the
// negative path was not, and the error message format is part
// of the user-visible contract.
func TestValidateRepoRoot(t *testing.T) {
	t.Parallel()
	src := SourceConfig{
		Name:      "seed",
		CommitSHA: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
	}
	missing := filepath.Join(t.TempDir(), "no-such-root")
	got, err := validateRepoRoot(src, "configured/root", missing)
	if err == nil {
		t.Fatalf("expected error, got %q", got)
	}
	if !strings.Contains(err.Error(), "seed") {
		t.Errorf("error %q must mention source name", err)
	}
	if !strings.Contains(err.Error(), "configured/root") {
		t.Errorf("error %q must mention configured root", err)
	}
	if !strings.Contains(err.Error(), src.CommitSHA) {
		t.Errorf("error %q must mention commit", err)
	}
}

// TestValidateRepoRoot_HappyPath pins that an existing resolved
// root is returned verbatim.
func TestValidateRepoRoot_HappyPath(t *testing.T) {
	t.Parallel()
	src := SourceConfig{Name: "seed", CommitSHA: "abc"}
	dir := t.TempDir()
	got, err := validateRepoRoot(src, "docs", dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != dir {
		t.Errorf("validateRepoRoot = %q, want %q", got, dir)
	}
}

// TestNormalizeRepository pins every branch of the helper: the
// pass-through schemes (git@, ssh://), the http/https paths with
// trailing slash and existing .git suffix, the github.com short
// form, the owner/repo shorthand, and the rejection of empty
// input. Each case maps a documented input shape to its canonical
// clone URL.
func TestNormalizeRepository(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
		err  bool
	}{
		{name: "empty rejected", in: "", err: true},
		{name: "whitespace-only rejected", in: "   \t", err: true},
		{name: "git@ passthrough",
			in:   "git@github.com:owner/repo.git",
			want: "git@github.com:owner/repo.git"},
		{name: "ssh:// passthrough",
			in:   "ssh://git@github.com/owner/repo.git",
			want: "ssh://git@github.com/owner/repo.git"},
		{name: "https with .git stays",
			in:   "https://github.com/owner/repo.git",
			want: "https://github.com/owner/repo.git"},
		{name: "https without .git gets suffix",
			in:   "https://github.com/owner/repo",
			want: "https://github.com/owner/repo.git"},
		{name: "https trailing slash trimmed",
			in:   "https://github.com/owner/repo/",
			want: "https://github.com/owner/repo.git"},
		{name: "http upgraded with suffix",
			in:   "http://example.com/owner/repo",
			want: "http://example.com/owner/repo.git"},
		{name: "github.com short form",
			in:   "github.com/owner/repo",
			want: "https://github.com/owner/repo.git"},
		{name: "owner/repo shorthand",
			in:   "owner/repo",
			want: "https://github.com/owner/repo.git"},
		{name: "absolute path passthrough",
			in:   "/abs/local/repo",
			want: "/abs/local/repo"},
		{name: "relative dot path passthrough",
			in:   "./local/repo",
			want: "./local/repo"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := normalizeRepository(c.in)
			if c.err {
				if err == nil {
					t.Fatalf("expected error for %q, got %q", c.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("normalizeRepository(%q) = %q, want %q",
					c.in, got, c.want)
			}
		})
	}
}

// --- shortCommit ---

// TestShortCommit pins the truncation invariant: SHAs ≥ 8 chars are
// truncated, shorter inputs are returned verbatim. The helper is
// used for log lines, so misformatting would mostly hide elsewhere;
// a direct unit pin is cheap.
func TestShortCommit(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":         "",
		"abc":      "abc",
		"abcdefg":  "abcdefg",
		"abcdefgh": "abcdefgh",
		"abcdef1234567890abcdef1234567890abcdef12": "abcdef12",
		strings.Repeat("a", 40):                    "aaaaaaaa",
	}
	for in, want := range cases {
		if got := shortCommit(in); got != want {
			t.Errorf("shortCommit(%q) = %q, want %q", in, got, want)
		}
	}
}

// --- validateRemoteSourceInputs ---

// TestValidateRemoteSourceInputs pins the three negative paths the
// validator guards (missing repository, missing commit SHA, missing
// cache directory) plus the happy path. Each error string is
// substring-checked so a future copy edit still reads cleanly.
func TestValidateRemoteSourceInputs(t *testing.T) {
	t.Parallel()
	t.Run("missing repository", func(t *testing.T) {
		err := validateRemoteSourceInputs(SourceConfig{CommitSHA: "deadbeef"}, "/tmp")
		if err == nil {
			t.Fatal("expected error for missing repository")
		}
		if !strings.Contains(err.Error(), "repository") {
			t.Errorf("error %q must mention repository", err)
		}
	})
	t.Run("missing commit", func(t *testing.T) {
		err := validateRemoteSourceInputs(SourceConfig{
			Repository: "owner/repo",
		}, "/tmp")
		if err == nil {
			t.Fatal("expected error for missing commit")
		}
		if !strings.Contains(err.Error(), "commit") {
			t.Errorf("error %q must mention commit", err)
		}
	})
	t.Run("missing cache directory", func(t *testing.T) {
		err := validateRemoteSourceInputs(SourceConfig{
			Repository: "owner/repo",
			CommitSHA:  "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		}, "  ")
		if err == nil {
			t.Fatal("expected error for missing cache dir")
		}
		if !strings.Contains(err.Error(), "cache") {
			t.Errorf("error %q must mention cache", err)
		}
	})
	t.Run("happy path", func(t *testing.T) {
		err := validateRemoteSourceInputs(SourceConfig{
			Repository: "owner/repo",
			CommitSHA:  "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		}, "/tmp")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
