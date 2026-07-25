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

	"github.com/jeduden/mdsmith/internal/bytelimit"
)

// Collect gathers markdown records from configured sources.
func Collect(cfg *Config, cacheDir string) ([]Record, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	allow := makeAllowset(cfg.LicenseAllowlist)

	records := make([]Record, 0, len(cfg.Sources))
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
			rel, relErr := filepath.Rel(resolvedRoot, path)
			if relErr == nil && isGenerated(rel) {
				return fs.SkipDir
			}
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

// isGenerated returns true when the relative path indicates the file lives
// inside a generated or vendored directory tree.  Skipping these paths avoids
// ingesting machine-written markdown that adds noise to the corpus.
func isGenerated(p string) bool {
	normalizedPath := "/" + strings.Trim(filepath.ToSlash(p), "/") + "/"
	for _, token := range []string{"/vendor/", "/node_modules/", "/dist/", "/build/", "/generated/", "/gen/"} {
		if strings.Contains(normalizedPath, token) {
			return true
		}
	}
	return false
}

func collectFile(
	cfg *Config,
	source SourceConfig,
	fullPath string,
	relPath string,
	resolvedRoot string,
) (Record, bool, error) {
	// Check for generated/vendor paths before reading the file to skip
	// unnecessary I/O.
	if isGenerated(relPath) {
		return Record{}, false, nil
	}

	// Skip a file this function cannot read — oversized, vanished, or
	// permission-denied — rather than failing the whole source:
	// collectFromRoot's caller aborts the entire walk (and every record
	// already collected from every source before it, since Collect
	// returns on the first error) on any error from this function. A
	// cloned third-party repository can legitimately contain one
	// oversized or racy file (a large CHANGELOG, a vendored spec, a file
	// removed mid-walk) without that file being reason to discard the
	// rest of the corpus build.
	info, err := os.Stat(fullPath)
	if err != nil {
		reportProgress(cfg, fmt.Sprintf("skipping %s: %v", relPath, err))
		return Record{}, false, nil
	}
	if info.Size() > bytelimit.DefaultMaxInputBytes {
		reportProgress(cfg, fmt.Sprintf(
			"skipping %s: %d bytes exceeds the %d byte limit",
			relPath, info.Size(), bytelimit.DefaultMaxInputBytes))
		return Record{}, false, nil
	}

	// bytelimit.ReadFileLimited re-checks the size on the actual read,
	// so a file that grows past the cap in the window between the Stat
	// above and this call (or otherwise becomes unreadable) is caught
	// here too — skipped the same way, not treated as fatal.
	content, err := bytelimit.ReadFileLimited(fullPath, bytelimit.DefaultMaxInputBytes)
	if err != nil {
		reportProgress(cfg, fmt.Sprintf("skipping %s: %v", relPath, err))
		return Record{}, false, nil
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
