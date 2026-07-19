package config

import (
	"fmt"
	"strings"
)

// EffectiveForeignRegions returns the foreign-region marker pairs that
// apply to filePath: the top-level `foreign-regions:` list followed by
// the pairs declared on every override whose glob matches the file.
// Override pairs append to (never replace) the base list, so a project
// can protect an extra marker pair in a subtree without restating the
// global ones. Returns nil when no pairs apply.
func EffectiveForeignRegions(cfg *Config, filePath string) []ForeignRegion {
	if cfg == nil {
		return nil
	}
	var out []ForeignRegion
	out = append(out, cfg.ForeignRegions...)
	for _, o := range cfg.Overrides {
		if len(o.ForeignRegions) == 0 {
			continue
		}
		if matchesAny(o.Patterns(), filePath) {
			out = append(out, o.ForeignRegions...)
		}
	}
	return out
}

// validateForeignRegions rejects malformed marker-pair declarations:
// a blank start or end marker, or a pair whose start equals its end
// (the scanner could never tell which line opens and which closes a
// region). It checks the top-level list and every override's list.
func validateForeignRegions(cfg *Config) error {
	if err := checkForeignRegionList(cfg.ForeignRegions); err != nil {
		return err
	}
	for i := range cfg.Overrides {
		if err := checkForeignRegionList(cfg.Overrides[i].ForeignRegions); err != nil {
			return err
		}
	}
	return nil
}

func checkForeignRegionList(regions []ForeignRegion) error {
	for i, r := range regions {
		if strings.TrimSpace(r.Start) == "" {
			return fmt.Errorf("foreign-regions[%d]: start marker must not be empty", i)
		}
		if strings.TrimSpace(r.End) == "" {
			return fmt.Errorf("foreign-regions[%d]: end marker must not be empty", i)
		}
		if strings.TrimSpace(r.Start) == strings.TrimSpace(r.End) {
			return fmt.Errorf(
				"foreign-regions[%d]: start and end markers must differ (both %q)",
				i, strings.TrimSpace(r.Start))
		}
	}
	return nil
}
