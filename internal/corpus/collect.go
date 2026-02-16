package corpus

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gobwas/glob"
	"github.com/jeduden/mdsmith/internal/mdtext"
)

type collectedRecord struct {
	ManifestRecord
	normalized string
	tokenSet   map[string]struct{}
}

var tokenNormalizer = strings.NewReplacer(
	"\n", " ",
	"\t", " ",
	".", " ",
	",", " ",
	"!", " ",
	"?", " ",
	":", " ",
	";", " ",
)

type collectionStats struct {
	filteredByPolicy  int
	filteredGenerated int
	filteredLowSignal int
	filesScanned      int
	sourcesIncluded   int
}

func collect(cfg BuildConfig) ([]collectedRecord, collectionStats, error) {
	allow := makeAllowlist(cfg.LicenseAllowlist)

	records := make([]collectedRecord, 0)
	stats := collectionStats{}

	for _, source := range cfg.Sources {
		allowed, reason := sourceAllowed(cfg.Policy, allow, source)
		if !allowed {
			_ = reason
			stats.filteredByPolicy++
			continue
		}
		stats.sourcesIncluded++

		files, err := sourceMarkdownFiles(source)
		if err != nil {
			return nil, stats, err
		}
		for _, file := range files {
			stats.filesScanned++
			record, kept, reason, err := collectFile(cfg, source, file)
			if err != nil {
				return nil, stats, err
			}
			if !kept {
				switch reason {
				case "generated":
					stats.filteredGenerated++
				case "low-signal":
					stats.filteredLowSignal++
				}
				continue
			}
			records = append(records, record)
		}
	}

	sort.Slice(records, func(i int, j int) bool {
		if records[i].SourceName == records[j].SourceName {
			return records[i].Path < records[j].Path
		}
		return records[i].SourceName < records[j].SourceName
	})

	return records, stats, nil
}

func makeAllowlist(items []string) map[string]bool {
	allow := make(map[string]bool, len(items))
	for _, item := range items {
		allow[strings.ToUpper(strings.TrimSpace(item))] = true
	}
	return allow
}

func sourceAllowed(policy QualityPolicy, allow map[string]bool, source SourceConfig) (bool, string) {
	license := strings.ToUpper(strings.TrimSpace(source.License))
	if !allow[license] {
		return false, "license"
	}
	if source.Quality.Archived {
		return false, "archived"
	}
	if source.Quality.Stars < policy.MinStars {
		return false, "stars"
	}
	if source.Quality.RecentCommits90D < policy.MinRecentCommits90D {
		return false, "activity"
	}
	if policy.RequireCI && !source.Quality.HasCI {
		return false, "ci"
	}
	return true, ""
}

func sourceMarkdownFiles(source SourceConfig) ([]string, error) {
	include, err := compileGlobPatterns(source.Include)
	if err != nil {
		return nil, fmt.Errorf("compile include patterns for %s: %w", source.Name, err)
	}
	exclude, err := compileGlobPatterns(source.Exclude)
	if err != nil {
		return nil, fmt.Errorf("compile exclude patterns for %s: %w", source.Name, err)
	}

	files := make([]string, 0)
	err = filepath.WalkDir(source.Root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !isMarkdownFile(path) {
			return nil
		}

		rel, err := filepath.Rel(source.Root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if len(include) > 0 && !matchesAny(include, rel) {
			return nil
		}
		if len(exclude) > 0 && matchesAny(exclude, rel) {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk source %s: %w", source.Name, err)
	}

	sort.Strings(files)
	return files, nil
}

func compileGlobPatterns(patterns []string) ([]glob.Glob, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	compiled := make([]glob.Glob, 0, len(patterns))
	for _, pattern := range patterns {
		g, err := glob.Compile(pattern)
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, g)
	}
	return compiled, nil
}

func matchesAny(patterns []glob.Glob, value string) bool {
	for _, pattern := range patterns {
		if pattern.Match(value) {
			return true
		}
	}
	return false
}

func isMarkdownFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".markdown"
}

func collectFile(
	cfg BuildConfig,
	source SourceConfig,
	fullPath string,
) (collectedRecord, bool, string, error) {
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return collectedRecord{}, false, "", fmt.Errorf("read file %s: %w", fullPath, err)
	}

	relPath, err := filepath.Rel(source.Root, fullPath)
	if err != nil {
		return collectedRecord{}, false, "", fmt.Errorf("relative path for %s: %w", fullPath, err)
	}
	relPath = filepath.ToSlash(relPath)

	normalized := normalizeMarkdown(string(content))
	if isGenerated(relPath, normalized) {
		return collectedRecord{}, false, "generated", nil
	}
	if isLowSignal(normalized, cfg.MinWords, cfg.MinChars) {
		return collectedRecord{}, false, "low-signal", nil
	}

	contentHash := sha256Hex(normalized)
	category := Classify(relPath, normalized)
	recordID := makeRecordID(source.Name, relPath, contentHash)
	wordCount := mdtext.CountWords(normalized)

	record := collectedRecord{
		ManifestRecord: ManifestRecord{
			RecordID:      recordID,
			Category:      category,
			SourceName:    source.Name,
			Repository:    source.Repository,
			RepositoryURL: source.RepositoryURL,
			Path:          relPath,
			CommitSHA:     source.CommitSHA,
			License:       source.License,
			CollectedAt:   cfg.CollectedAt,
			WordCount:     wordCount,
			CharCount:     len(normalized),
			ContentSHA256: contentHash,
			IsReadme:      strings.EqualFold(filepath.Base(relPath), "README.md"),
		},
		normalized: normalized,
		tokenSet:   tokenSet(normalized),
	}
	return record, true, "", nil
}

func normalizeMarkdown(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	for idx, line := range lines {
		lines[idx] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

func isGenerated(relPath string, content string) bool {
	p := strings.ToLower(relPath)
	for _, token := range []string{"/vendor/", "/node_modules/", "/dist/", "/build/", "/generated/", "/gen/"} {
		if strings.Contains(p, token) {
			return true
		}
	}
	lower := strings.ToLower(content)
	if strings.Contains(lower, "code generated") && strings.Contains(lower, "do not edit") {
		return true
	}
	return strings.Contains(lower, "automatically generated") && strings.Contains(lower, "do not edit")
}

func isLowSignal(content string, minWords int, minChars int) bool {
	if len(strings.TrimSpace(content)) < minChars {
		return true
	}
	return mdtext.CountWords(content) < minWords
}

func sha256Hex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func makeRecordID(source string, relPath string, contentHash string) string {
	input := strings.Join([]string{source, relPath, contentHash}, "|")
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:16])
}

func tokenSet(content string) map[string]struct{} {
	normalized := strings.ToLower(content)
	normalized = tokenNormalizer.Replace(normalized)
	parts := strings.Fields(normalized)
	set := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		trimmed := strings.Trim(part, "`'\"()[]{}<>")
		if len(trimmed) < 3 {
			continue
		}
		set[trimmed] = struct{}{}
	}
	return set
}
