package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jeduden/mdsmith/internal/corpus"
)

var defaultFetchRoot = filepath.Join(os.TempDir(), "mdsmith-corpus-sources")

var runGit = execGit

func runMeasure(args []string) error {
	opts, err := parseMeasureArgs(args)
	if err != nil {
		return err
	}

	cfg, err := corpus.LoadConfig(opts.configPath)
	if err != nil {
		return err
	}
	if err := applyMeasureOverrides(&cfg, opts.datasetVersion, opts.collectedAt); err != nil {
		return err
	}
	fetchRoot := resolveFetchRoot(opts.fetchRoot, cfg.DatasetVersion)
	if err := fetchPinnedSources(fetchRoot, cfg.Sources); err != nil {
		return err
	}
	setSourceRootsToFetchRoot(&cfg, fetchRoot)

	result, err := corpus.Build(cfg)
	if err != nil {
		return err
	}
	return writeMeasureOutputs(opts.outDir, cfg, result)
}

type measureOptions struct {
	configPath     string
	outDir         string
	fetchRoot      string
	datasetVersion string
	collectedAt    string
}

func parseMeasureArgs(args []string) (measureOptions, error) {
	fs := flag.NewFlagSet("measure", flag.ContinueOnError)
	configPath := fs.String("config", "", "path to corpus build config yaml")
	outDir := fs.String("out", "", "output directory")
	fetchRoot := fs.String("fetch-root", defaultFetchRoot, "directory for fetched source checkouts")
	datasetVersion := fs.String("dataset-version", "", "override dataset_version from config")
	collectedAt := fs.String("collected-at", "", "override collected_at from config (YYYY-MM-DD)")
	if err := fs.Parse(args); err != nil {
		return measureOptions{}, err
	}
	if *configPath == "" || *outDir == "" {
		return measureOptions{}, errors.New("measure requires -config and -out")
	}

	return measureOptions{
		configPath:     *configPath,
		outDir:         *outDir,
		fetchRoot:      *fetchRoot,
		datasetVersion: *datasetVersion,
		collectedAt:    *collectedAt,
	}, nil
}

func applyMeasureOverrides(cfg *corpus.BuildConfig, datasetVersion string, collectedAt string) error {
	if datasetVersion != "" {
		cfg.DatasetVersion = datasetVersion
	}
	if collectedAt != "" {
		if _, err := time.Parse(time.DateOnly, collectedAt); err != nil {
			return fmt.Errorf("collected-at must use YYYY-MM-DD: %w", err)
		}
		cfg.CollectedAt = collectedAt
	}
	return nil
}

func setSourceRootsToFetchRoot(cfg *corpus.BuildConfig, fetchRoot string) {
	for i := range cfg.Sources {
		cfg.Sources[i].Root = filepath.Join(fetchRoot, cfg.Sources[i].Name)
	}
}

func resolveFetchRoot(fetchRoot string, datasetVersion string) string {
	if fetchRoot == defaultFetchRoot && datasetVersion != "" {
		return filepath.Join(fetchRoot, datasetVersion)
	}
	return fetchRoot
}

func writeMeasureOutputs(outDir string, cfg corpus.BuildConfig, result corpus.BuildOutput) error {
	manifestPath := filepath.Join(outDir, "manifest.jsonl")
	reportPath := filepath.Join(outDir, "report.json")
	samplePath := filepath.Join(outDir, "qa-sample.jsonl")
	configOutPath := filepath.Join(outDir, "config.generated.yml")

	if err := corpus.WriteManifest(manifestPath, result.Manifest); err != nil {
		return err
	}
	if err := corpus.WriteJSON(reportPath, result.Report); err != nil {
		return err
	}
	if err := corpus.WriteQASample(samplePath, result.QASample); err != nil {
		return err
	}
	if err := corpus.WriteConfig(configOutPath, cfg); err != nil {
		return err
	}

	fmt.Printf("manifest: %s\n", manifestPath)
	fmt.Printf("report:   %s\n", reportPath)
	fmt.Printf("qa:       %s\n", samplePath)
	fmt.Printf("config:   %s\n", configOutPath)
	return nil
}

func fetchPinnedSources(fetchRoot string, sources []corpus.SourceConfig) error {
	if err := os.MkdirAll(fetchRoot, 0o755); err != nil {
		return fmt.Errorf("create fetch root %s: %w", fetchRoot, err)
	}
	for _, source := range sources {
		if err := fetchPinnedSource(fetchRoot, source); err != nil {
			return err
		}
	}
	return nil
}

func fetchPinnedSource(fetchRoot string, source corpus.SourceConfig) error {
	if source.Name == "" {
		return errors.New("source name is required for fetch")
	}
	if source.CommitSHA == "" {
		return fmt.Errorf("source %s commit_sha is required for fetch", source.Name)
	}

	remoteURL, err := sourceRemoteURL(source)
	if err != nil {
		return fmt.Errorf("source %s: %w", source.Name, err)
	}

	dest := filepath.Join(fetchRoot, source.Name)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("create source directory %s: %w", dest, err)
	}

	if _, err := os.Stat(filepath.Join(dest, ".git")); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("stat git dir for %s: %w", dest, err)
		}
		if _, err := runGit("", "init", dest); err != nil {
			return fmt.Errorf("initialize git repo for %s: %w", source.Name, err)
		}
	}

	if _, err := runGit(dest, "remote", "get-url", "origin"); err == nil {
		if _, err := runGit(dest, "remote", "set-url", "origin", remoteURL); err != nil {
			return fmt.Errorf("set remote for %s: %w", source.Name, err)
		}
	} else {
		if _, err := runGit(dest, "remote", "add", "origin", remoteURL); err != nil {
			return fmt.Errorf("add remote for %s: %w", source.Name, err)
		}
	}

	if _, err := runGit(dest, "fetch", "--depth", "1", "origin", source.CommitSHA); err != nil {
		return fmt.Errorf("fetch %s@%s: %w", source.Name, source.CommitSHA, err)
	}
	if _, err := runGit(dest, "checkout", "--detach", "--force", "FETCH_HEAD"); err != nil {
		return fmt.Errorf("checkout %s@%s: %w", source.Name, source.CommitSHA, err)
	}

	got, err := runGit(dest, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("resolve fetched hash for %s: %w", source.Name, err)
	}
	if got != source.CommitSHA {
		return fmt.Errorf(
			"hash mismatch for %s: expected %s, got %s",
			source.Name,
			source.CommitSHA,
			got,
		)
	}

	fmt.Printf("fetched %s @ %s\n", source.Name, source.CommitSHA)
	return nil
}

func sourceRemoteURL(source corpus.SourceConfig) (string, error) {
	if source.RepositoryURL != "" {
		return normalizeRemoteURL(source.RepositoryURL), nil
	}

	repo := strings.TrimSpace(source.Repository)
	if repo == "" {
		return "", errors.New("repository or repository_url is required")
	}

	switch {
	case strings.HasPrefix(repo, "http://"), strings.HasPrefix(repo, "https://"):
		return normalizeRemoteURL(repo), nil
	case strings.HasPrefix(repo, "ssh://"), strings.HasPrefix(repo, "git@"):
		return repo, nil
	case strings.HasPrefix(repo, "github.com/"):
		return normalizeRemoteURL("https://" + repo), nil
	default:
		return normalizeRemoteURL("https://github.com/" + strings.TrimPrefix(repo, "/")), nil
	}
}

func normalizeRemoteURL(remote string) string {
	trimmed := strings.TrimSpace(remote)
	trimmed = strings.TrimRight(trimmed, "/")
	if strings.HasSuffix(trimmed, ".git") {
		return trimmed
	}
	return trimmed + ".git"
}

func execGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrText := strings.TrimSpace(stderr.String())
		if stderrText == "" {
			return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), stderrText, err)
	}
	return strings.TrimSpace(stdout.String()), nil
}
