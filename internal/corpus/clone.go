package corpus

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitRunner executes git commands.
type GitRunner interface {
	Run(args []string) ([]byte, error)
}

type execGitRunner struct{}

func (execGitRunner) Run(args []string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

var defaultGitRunner GitRunner = execGitRunner{}

// ResolveSource ensures a source is available locally and returns the local root directory.
func ResolveSource(src SourceConfig, cacheDir string) (string, error) {
	return resolveSourceWithRunner(src, cacheDir, defaultGitRunner)
}

func resolveSourceWithRunner(src SourceConfig, cacheDir string, runner GitRunner) (string, error) {
	root := strings.TrimSpace(src.Root)
	if root == "" {
		return "", fmt.Errorf("source %s root is required", src.Name)
	}

	if filepath.IsAbs(root) {
		return validateLocalRoot(src.Name, root)
	}

	if err := validateRemoteSourceInputs(src, cacheDir); err != nil {
		return "", err
	}

	remote, err := normalizeRepository(src.Repository)
	if err != nil {
		return "", fmt.Errorf("source %s repository: %w", src.Name, err)
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create cache directory %s: %w", cacheDir, err)
	}

	repoDir := filepath.Join(cacheDir, cacheKey(remote))
	if err := ensureRepoCached(src, remote, repoDir, runner); err != nil {
		return "", err
	}

	if err := ensurePinnedCommit(src, repoDir, remote, runner); err != nil {
		return "", err
	}

	if _, err := runner.Run([]string{
		"-C", repoDir, "checkout", "--detach", "--force", src.CommitSHA,
	}); err != nil {
		return "", fmt.Errorf(
			"checkout source %s commit %s: %w",
			src.Name,
			src.CommitSHA,
			err,
		)
	}

	resolvedRoot := filepath.Join(repoDir, filepath.FromSlash(root))
	return validateRepoRoot(src, root, resolvedRoot)
}

func validateLocalRoot(sourceName string, root string) (string, error) {
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("source %s local root does not exist: %s", sourceName, root)
		}
		return "", fmt.Errorf("stat source %s local root: %w", sourceName, err)
	}
	return filepath.Clean(root), nil
}

func validateRemoteSourceInputs(src SourceConfig, cacheDir string) error {
	if strings.TrimSpace(src.Repository) == "" {
		return fmt.Errorf("source %s repository is required", src.Name)
	}
	if strings.TrimSpace(src.CommitSHA) == "" {
		return fmt.Errorf("source %s commit_sha is required", src.Name)
	}
	if strings.TrimSpace(cacheDir) == "" {
		return errors.New("cache directory is required")
	}
	return nil
}

func ensureRepoCached(
	src SourceConfig,
	remote string,
	repoDir string,
	runner GitRunner,
) error {
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("stat cached repo for %s: %w", src.Name, err)
		}
		if _, runErr := runner.Run([]string{
			"clone", "--no-checkout", remote, repoDir,
		}); runErr != nil {
			return fmt.Errorf(
				"clone source %s: %w",
				src.Name,
				classifyGitError(runErr, remote, src.CommitSHA),
			)
		}
	}
	return nil
}

func ensurePinnedCommit(
	src SourceConfig,
	repoDir string,
	remote string,
	runner GitRunner,
) error {
	hasCommit, err := cachedCommitExists(repoDir, src.CommitSHA, runner)
	if err != nil {
		return fmt.Errorf("check cached commit for %s: %w", src.Name, err)
	}
	if hasCommit {
		return nil
	}

	_, runErr := runner.Run([]string{
		"-C", repoDir, "fetch", "--depth", "1", "origin", src.CommitSHA,
	})
	if runErr != nil {
		return fmt.Errorf(
			"fetch source %s commit %s: %w",
			src.Name,
			src.CommitSHA,
			classifyGitError(runErr, remote, src.CommitSHA),
		)
	}
	return nil
}

func validateRepoRoot(src SourceConfig, root string, resolvedRoot string) (string, error) {
	if _, err := os.Stat(resolvedRoot); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf(
				"source %s root %q not found in commit %s",
				src.Name,
				root,
				src.CommitSHA,
			)
		}
		return "", fmt.Errorf("stat source %s root: %w", src.Name, err)
	}
	return filepath.Clean(resolvedRoot), nil
}

func cacheKey(remote string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(remote))))
	return hex.EncodeToString(sum[:8])
}

func normalizeRepository(repository string) (string, error) {
	repo := strings.TrimSpace(repository)
	if repo == "" {
		return "", errors.New("repository is required")
	}

	switch {
	case strings.HasPrefix(repo, "git@"):
		return repo, nil
	case strings.HasPrefix(repo, "ssh://"):
		return repo, nil
	case strings.HasPrefix(repo, "http://"), strings.HasPrefix(repo, "https://"):
		trimmed := strings.TrimRight(repo, "/")
		if strings.HasSuffix(trimmed, ".git") {
			return trimmed, nil
		}
		return trimmed + ".git", nil
	case strings.HasPrefix(repo, "github.com/"):
		return "https://" + strings.TrimRight(repo, "/") + ".git", nil
	default:
		if strings.Contains(repo, "/") && !filepath.IsAbs(repo) && !strings.HasPrefix(repo, ".") {
			return "https://github.com/" + strings.TrimLeft(strings.TrimRight(repo, "/"), "/") + ".git", nil
		}
		return repo, nil
	}
}

func cachedCommitExists(repoDir string, commitSHA string, runner GitRunner) (bool, error) {
	_, err := runner.Run([]string{"-C", repoDir, "cat-file", "-e", commitSHA + "^{commit}"})
	if err == nil {
		return true, nil
	}
	if strings.Contains(err.Error(), "Not a valid object name") || strings.Contains(err.Error(), "invalid object") {
		return false, nil
	}
	if strings.Contains(err.Error(), "unknown revision") {
		return false, nil
	}
	return false, nil
}

func classifyGitError(err error, remote string, commitSHA string) error {
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "repository not found"),
		strings.Contains(text, "could not read from remote repository"):
		return fmt.Errorf("repository not found or inaccessible: %s", remote)
	case strings.Contains(text, "couldn't find remote ref"),
		strings.Contains(text, "not our ref"):
		return fmt.Errorf("commit not found: %s", commitSHA)
	case strings.Contains(text, "failed to connect"),
		strings.Contains(text, "timed out"),
		strings.Contains(text, "could not resolve host"):
		return fmt.Errorf("network error while accessing %s", remote)
	default:
		return err
	}
}
