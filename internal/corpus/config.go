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
	defaultMinWords      = 30
	defaultMinChars      = 100
	defaultTestFraction  = 0.2
	defaultQASampleLimit = 50
)

type localOverrideConfig struct {
	Sources []struct {
		Name string `yaml:"name"`
		Root string `yaml:"root"`
	} `yaml:"sources"`
}

// LoadConfig loads config from YAML, applies defaults, and merges config.local.yml overrides.
func LoadConfig(path string) (*Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return nil, fmt.Errorf("parse config yaml: %w", err)
	}
	applyConfigDefaults(&cfg)

	if err := mergeLocalOverrides(path, &cfg); err != nil {
		return nil, err
	}
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func applyConfigDefaults(cfg *Config) {
	if cfg.MinWords == 0 {
		cfg.MinWords = defaultMinWords
	}
	if cfg.MinChars == 0 {
		cfg.MinChars = defaultMinChars
	}
	if cfg.TestFraction == 0 {
		cfg.TestFraction = defaultTestFraction
	}
	if cfg.QASampleLimit == 0 {
		cfg.QASampleLimit = defaultQASampleLimit
	}
	if cfg.DatasetVersion == "" && cfg.CollectedAt != "" {
		cfg.DatasetVersion = "v" + cfg.CollectedAt
	}
}

func mergeLocalOverrides(configPath string, cfg *Config) error {
	localPath := filepath.Join(filepath.Dir(configPath), "config.local.yml")
	content, err := os.ReadFile(localPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read local override config: %w", err)
	}

	var local localOverrideConfig
	if err := yaml.Unmarshal(content, &local); err != nil {
		return fmt.Errorf("parse local override config: %w", err)
	}

	byName := make(map[string]string, len(local.Sources))
	for _, source := range local.Sources {
		name := strings.TrimSpace(source.Name)
		root := strings.TrimSpace(source.Root)
		if name == "" || root == "" {
			continue
		}
		byName[name] = root
	}

	if len(byName) == 0 {
		return nil
	}

	for i := range cfg.Sources {
		if root, ok := byName[cfg.Sources[i].Name]; ok {
			cfg.Sources[i].Root = root
			cfg.ResolvedFromLocal = true
		}
	}
	return nil
}

func validateConfig(cfg Config) error {
	if err := validateConfigHeader(cfg); err != nil {
		return err
	}

	allow := normalizeAllowlist(cfg.LicenseAllowlist)
	seen := make(map[string]struct{}, len(cfg.Sources))
	for _, source := range cfg.Sources {
		if err := validateSource(source, allow, seen); err != nil {
			return err
		}
		seen[source.Name] = struct{}{}
	}

	return nil
}

func validateConfigHeader(cfg Config) error {
	if cfg.CollectedAt == "" {
		return fmt.Errorf("collected_at is required")
	}
	if _, err := time.Parse(time.DateOnly, cfg.CollectedAt); err != nil {
		return fmt.Errorf("collected_at must use YYYY-MM-DD: %w", err)
	}
	if cfg.MinWords < 1 {
		return fmt.Errorf("min_words must be >= 1")
	}
	if cfg.MinChars < 1 {
		return fmt.Errorf("min_chars must be >= 1")
	}
	if cfg.TestFraction <= 0 || cfg.TestFraction >= 1 {
		return fmt.Errorf("test_fraction must be between 0 and 1")
	}
	if cfg.QASampleLimit < 1 {
		return fmt.Errorf("qa_sample_limit must be >= 1")
	}
	if len(cfg.LicenseAllowlist) == 0 {
		return fmt.Errorf("license_allowlist cannot be empty")
	}
	if len(cfg.Sources) == 0 {
		return fmt.Errorf("at least one source is required")
	}
	return nil
}

func normalizeAllowlist(items []string) map[string]struct{} {
	allow := make(map[string]struct{}, len(items))
	for _, license := range items {
		norm := strings.ToUpper(strings.TrimSpace(license))
		if norm != "" {
			allow[norm] = struct{}{}
		}
	}
	return allow
}

func validateSource(
	source SourceConfig,
	allow map[string]struct{},
	seen map[string]struct{},
) error {
	if strings.TrimSpace(source.Name) == "" {
		return fmt.Errorf("source name is required")
	}
	if _, ok := seen[source.Name]; ok {
		return fmt.Errorf("duplicate source name: %s", source.Name)
	}
	if strings.TrimSpace(source.Repository) == "" {
		return fmt.Errorf("source %s repository is required", source.Name)
	}
	if strings.TrimSpace(source.Root) == "" {
		return fmt.Errorf("source %s root is required", source.Name)
	}
	if strings.TrimSpace(source.CommitSHA) == "" {
		return fmt.Errorf("source %s commit_sha is required", source.Name)
	}
	if strings.TrimSpace(source.License) == "" {
		return fmt.Errorf("source %s license is required", source.Name)
	}
	if _, ok := allow[strings.ToUpper(strings.TrimSpace(source.License))]; !ok {
		return fmt.Errorf("source %s license %q is not allowlisted", source.Name, source.License)
	}
	return nil
}
