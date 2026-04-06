package config

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseSize parses a human-readable size string into bytes.
// Accepted formats: "2MB", "500KB", "1GB", bare integer (bytes), "0" (unlimited).
// Case-insensitive. Uses binary units (1 KB = 1024, 1 MB = 1048576, 1 GB = 1073741824).
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size string")
	}

	upper := strings.ToUpper(s)

	// Try bare integer first.
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n < 0 {
			return 0, fmt.Errorf("negative size %q", s)
		}
		return n, nil
	}

	// Try suffix-based parsing.
	type suffix struct {
		label      string
		multiplier int64
	}
	suffixes := []suffix{
		{"GB", 1024 * 1024 * 1024},
		{"MB", 1024 * 1024},
		{"KB", 1024},
	}

	for _, sf := range suffixes {
		if strings.HasSuffix(upper, sf.label) {
			numStr := strings.TrimSpace(s[:len(s)-len(sf.label)])
			n, err := strconv.ParseInt(numStr, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid size %q: %w", s, err)
			}
			if n < 0 {
				return 0, fmt.Errorf("negative size %q", s)
			}
			return n * sf.multiplier, nil
		}
	}

	return 0, fmt.Errorf("invalid size %q: unrecognized unit (use KB, MB, or GB)", s)
}
