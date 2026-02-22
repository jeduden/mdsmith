package corpus

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Collect gathers markdown records from configured sources.
func Collect(cfg *Config, cacheDir string) ([]Record, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	allow := makeAllowset(cfg.LicenseAllowlist)

	records := make([]Record, 0)
	for idx, source := range cfg.Sources {
		sourceRecords, err := collectSource(
			cfg,
			source,
			idx,
			len(cfg.Sources),
			allow,
			cacheDir,
		)
		if err != nil {
			return nil, err
		}
		records = append(records, sourceRecords...)
	}
	return records, nil
}

func makeAllowset(licenses []string) map[string]struct{} {
	allow := make(map[string]struct{}, len(licenses))
	for _, license := range licenses {
		normalized := strings.ToUpper(strings.TrimSpace(license))
		if normalized != "" {
			allow[normalized] = struct{}{}
		}
	}
	return allow
}

func collectSource(
	cfg *Config,
	source SourceConfig,
	index int,
	total int,
	allow map[string]struct{},
	cacheDir string,
) ([]Record, error) {
	reportProgress(
		cfg,
		fmt.Sprintf("source %d/%d: %s", index+1, total, source.Name),
	)

	if _, ok := allow[strings.ToUpper(strings.TrimSpace(source.License))]; !ok {
		reportProgress(
			cfg,
			fmt.Sprintf(
				"source %s skipped: license %s not allowlisted",
				source.Name,
				source.License,
			),
		)
		return nil, nil
	}

	resolvedRoot, err := resolveSourceWithRunnerAndProgress(
		source,
		cacheDir,
		defaultGitRunner,
		cfg.Progress,
	)
	if err != nil {
		return nil, err
	}
	reportProgress(cfg, fmt.Sprintf("source %s resolved to %s", source.Name, resolvedRoot))

	records, err := collectFromRoot(cfg, source, resolvedRoot)
	if err != nil {
		return nil, err
	}
	reportProgress(
		cfg,
		fmt.Sprintf("source %s collected %d records", source.Name, len(records)),
	)
	return records, nil
}

func collectFromRoot(cfg *Config, source SourceConfig, resolvedRoot string) ([]Record, error) {
	info, err := os.Stat(resolvedRoot)
	if err != nil {
		return nil, fmt.Errorf("stat source root %s: %w", resolvedRoot, err)
	}

	if !info.IsDir() {
		record, keep, err := collectFile(cfg, source, resolvedRoot, filepath.Base(resolvedRoot), resolvedRoot)
		if err != nil {
			return nil, err
		}
		if !keep {
			return nil, nil
		}
		return []Record{record}, nil
	}

	records := make([]Record, 0)
	err = filepath.WalkDir(resolvedRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !isMarkdown(path) {
			return nil
		}

		rel, err := filepath.Rel(resolvedRoot, path)
		if err != nil {
			return fmt.Errorf("relative path for %s: %w", path, err)
		}

		record, keep, err := collectFile(cfg, source, path, rel, resolvedRoot)
		if err != nil {
			return err
		}
		if keep {
			records = append(records, record)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk source %s: %w", source.Name, err)
	}
	return records, nil
}

func collectFile(
	cfg *Config,
	source SourceConfig,
	fullPath string,
	relPath string,
	resolvedRoot string,
) (Record, bool, error) {
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return Record{}, false, fmt.Errorf("read file %s: %w", fullPath, err)
	}
	raw := normalizeContent(string(content))
	words := countWords(raw)
	chars := utf8.RuneCountInString(raw)
	if words < cfg.MinWords || chars < cfg.MinChars {
		return Record{}, false, nil
	}

	rel := filepath.ToSlash(relPath)
	sourcePath := sourceRelativePath(source.Root, rel, resolvedRoot)
	contentHash := sha256Hex(raw)
	recordID := shortHash(source.Name + "|" + sourcePath + "|" + contentHash)

	return Record{
		RecordID:       recordID,
		Source:         source.Name,
		Repository:     source.Repository,
		CommitSHA:      source.CommitSHA,
		License:        source.License,
		Path:           sourcePath,
		Words:          words,
		Chars:          chars,
		ContentSHA256:  contentHash,
		RawContent:     raw,
		SourceResolved: resolvedRoot,
		CollectedAt:    cfg.CollectedAt,
	}, true, nil
}

func sourceRelativePath(configuredRoot string, relPath string, resolvedRoot string) string {
	trimmed := strings.TrimSpace(configuredRoot)
	if trimmed == "" || filepath.IsAbs(trimmed) {
		return relPath
	}
	joined := filepath.ToSlash(filepath.Join(trimmed, relPath))
	joined = strings.TrimPrefix(joined, "./")
	joined = strings.TrimPrefix(joined, "/")
	if joined == "" {
		return filepath.ToSlash(filepath.Base(resolvedRoot))
	}
	return joined
}

func isMarkdown(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".markdown"
}

func normalizeContent(input string) string {
	value := strings.ReplaceAll(input, "\r\n", "\n")
	lines := strings.Split(value, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func countWords(content string) int {
	return len(strings.Fields(content))
}

func sha256Hex(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func shortHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:16]
}

func reportProgress(cfg *Config, message string) {
	if cfg != nil && cfg.Progress != nil {
		cfg.Progress(message)
	}
}
