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
