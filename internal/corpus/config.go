package corpus

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultNearDuplicateThreshold = 0.92
	defaultMaxReadmeShare         = 0.25
	defaultQASamplePerCategory    = 5
	defaultMinWords               = 30
	defaultMinChars               = 180
	defaultSeed                   = 62
)

// LoadConfig loads a build config from YAML and validates it.
func LoadConfig(path string) (BuildConfig, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return BuildConfig{}, fmt.Errorf("read config: %w", err)
	}

	var cfg BuildConfig
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return BuildConfig{}, fmt.Errorf("parse config yaml: %w", err)
	}

	cfg.applyDefaults(filepath.Dir(path))
	if err := cfg.validate(); err != nil {
		return BuildConfig{}, err
	}
	return cfg, nil
}

func (cfg *BuildConfig) applyDefaults(configDir string) {
	if cfg.DatasetVersion == "" {
		cfg.DatasetVersion = "v0"
	}
	if cfg.Seed == 0 {
		cfg.Seed = defaultSeed
	}
	if cfg.MinWords == 0 {
		cfg.MinWords = defaultMinWords
	}
	if cfg.MinChars == 0 {
		cfg.MinChars = defaultMinChars
	}
	if cfg.NearDuplicateThreshold == 0 {
		cfg.NearDuplicateThreshold = defaultNearDuplicateThreshold
	}
	if cfg.MaxReadmeShare == 0 {
		cfg.MaxReadmeShare = defaultMaxReadmeShare
	}
	if cfg.QASamplePerCategory == 0 {
		cfg.QASamplePerCategory = defaultQASamplePerCategory
	}

	for i := range cfg.Sources {
		if cfg.Sources[i].Root == "" {
			cfg.Sources[i].Root = "."
		}
		if !filepath.IsAbs(cfg.Sources[i].Root) {
			cfg.Sources[i].Root = filepath.Join(configDir, cfg.Sources[i].Root)
		}
		if len(cfg.Sources[i].Include) == 0 {
			cfg.Sources[i].Include = []string{"**/*.md", "**/*.markdown"}
		}
	}
}

func (cfg BuildConfig) validate() error {
	if cfg.CollectedAt == "" {
		return fmt.Errorf("collected_at is required")
	}
	if _, err := time.Parse(time.DateOnly, cfg.CollectedAt); err != nil {
		return fmt.Errorf("collected_at must use YYYY-MM-DD: %w", err)
	}
	if len(cfg.Sources) == 0 {
		return fmt.Errorf("at least one source is required")
	}
	if cfg.NearDuplicateThreshold < 0 || cfg.NearDuplicateThreshold > 1 {
		return fmt.Errorf("near_duplicate_threshold must be between 0 and 1")
	}
	if cfg.MaxReadmeShare < 0 || cfg.MaxReadmeShare > 1 {
		return fmt.Errorf("max_readme_share must be between 0 and 1")
	}
	if cfg.QASamplePerCategory < 1 {
		return fmt.Errorf("qa_sample_per_category must be >= 1")
	}

	allow := make(map[string]bool, len(cfg.LicenseAllowlist))
	for _, license := range cfg.LicenseAllowlist {
		allow[strings.ToUpper(strings.TrimSpace(license))] = true
	}
	if len(allow) == 0 {
		return fmt.Errorf("license_allowlist cannot be empty")
	}

	seen := make(map[string]bool, len(cfg.Sources))
	for _, source := range cfg.Sources {
		if source.Name == "" {
			return fmt.Errorf("source name is required")
		}
		if seen[source.Name] {
			return fmt.Errorf("duplicate source name: %s", source.Name)
		}
		seen[source.Name] = true
		if source.Repository == "" {
			return fmt.Errorf("source %s repository is required", source.Name)
		}
		if source.CommitSHA == "" {
			return fmt.Errorf("source %s commit_sha is required", source.Name)
		}
		if !allow[strings.ToUpper(strings.TrimSpace(source.License))] {
			return fmt.Errorf("source %s license %q is not allowlisted", source.Name, source.License)
		}
	}

	for category, rng := range cfg.Balance {
		if !isKnownCategory(category) {
			return fmt.Errorf("unknown balance category: %s", category)
		}
		if rng.Min < 0 || rng.Min > 1 || rng.Max < 0 || rng.Max > 1 || rng.Min > rng.Max {
			return fmt.Errorf("invalid balance range for %s", category)
		}
	}

	return nil
}

func isKnownCategory(category Category) bool {
	for _, known := range AllCategories() {
		if known == category {
			return true
		}
	}
	return false
}
