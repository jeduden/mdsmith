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
	for _, r := range cfg.ForeignRegions {
		if !containsForeignRegion(out, r) {
			out = append(out, r)
		}
	}
	for _, o := range cfg.Overrides {
		if len(o.ForeignRegions) == 0 {
			continue
		}
		if !matchesAny(o.Patterns(), filePath) {
			continue
		}
		for _, r := range o.ForeignRegions {
			if !containsForeignRegion(out, r) {
				out = append(out, r)
			}
		}
	}
	return out
}

// containsForeignRegion reports whether list already holds an equivalent
// marker pair. EffectiveForeignRegions dedups with it so a pair declared
// both top-level and on a matching override (or repeated within a list)
// contributes one protected span and one MDS074 diagnostic, not two —
// the check path does not otherwise dedup per-file diagnostics.
//
// Equivalence is by trimmed markers, matching how the scanner compares a
// marker against a source line (whole-line equality after TrimSpace). Two
// pairs that differ only in surrounding whitespace — e.g. "<!-- x -->" and
// " <!-- x -->" — scan identically, so they must dedup to one span and one
// diagnostic; a raw `==` comparison would let the whitespace variant slip
// past and re-introduce the double report. The marker-pair lists are short
// (a handful of entries), so the linear scan is cheaper than allocating a
// set on this per-file hot path.
func containsForeignRegion(list []ForeignRegion, r ForeignRegion) bool {
	start := strings.TrimSpace(r.Start)
	end := strings.TrimSpace(r.End)
	for _, e := range list {
		if strings.TrimSpace(e.Start) == start && strings.TrimSpace(e.End) == end {
			return true
		}
	}
	return false
}

// validateConfigSemantics runs the post-parse structural checks that
// depend on the fully-decoded config: kind graph validity and
// foreign-region marker-pair well-formedness. Kept together so
// loadFromBytes carries one call site rather than one per check.
func validateConfigSemantics(cfg *Config) error {
	if err := ValidateKinds(cfg); err != nil {
		return err
	}
	return validateForeignRegions(cfg)
}

// validateForeignRegions rejects malformed marker-pair declarations:
// a blank start or end marker, or a pair whose start equals its end
// (the scanner could never tell which line opens and which closes a
// region). It checks the top-level list and every override's list.
func validateForeignRegions(cfg *Config) error {
	if err := checkForeignRegionList("foreign-regions", cfg.ForeignRegions); err != nil {
		return err
	}
	for i := range cfg.Overrides {
		label := fmt.Sprintf("overrides[%d].foreign-regions", i)
		if err := checkForeignRegionList(label, cfg.Overrides[i].ForeignRegions); err != nil {
			return err
		}
	}
	return nil
}

// checkForeignRegionList validates one marker-pair list. label names the
// list's location in the config ("foreign-regions" or
// "overrides[i].foreign-regions") so an error points at the offending
// override rather than an ambiguous top-level index.
func checkForeignRegionList(label string, regions []ForeignRegion) error {
	for i, r := range regions {
		start := strings.TrimSpace(r.Start)
		end := strings.TrimSpace(r.End)
		if start == "" {
			return fmt.Errorf("%s[%d]: start marker must not be empty", label, i)
		}
		if end == "" {
			return fmt.Errorf("%s[%d]: end marker must not be empty", label, i)
		}
		if start == end {
			return fmt.Errorf(
				"%s[%d]: start and end markers must differ (both %q)",
				label, i, start)
		}
	}
	return nil
}
