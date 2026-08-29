package schema

import (
	"fmt"
	"path/filepath"
	"strings"
)

// DecodeFilenameField normalizes a schema `filename:` value into the
// list of globs the document basename may match. It accepts a single
// glob string (the historical spelling, kept for backward
// compatibility) or a YAML sequence of glob strings, giving `filename:`
// the same OR semantics that `unique-frontmatter.include`,
// `overrides.glob`, `kind-assignment.glob`, and catalog `glob:` already
// carry elsewhere in the config. A nil value (absent key) or an empty
// string yields nil — no filename constraint. Every sequence entry must
// be a non-empty string.
func DecodeFilenameField(v any) ([]string, error) {
	switch t := v.(type) {
	case nil:
		return nil, nil
	case string:
		if t == "" {
			return nil, nil
		}
		if err := ValidateGlobInterps(t); err != nil {
			return nil, fmt.Errorf("filename %q: %w", t, err)
		}
		return []string{t}, nil
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			s, ok := e.(string)
			if !ok {
				return nil, fmt.Errorf(
					"filename must be a string or list of strings, "+
						"got %T in the list", e)
			}
			if s == "" {
				return nil, fmt.Errorf(
					"filename list entries must be non-empty globs")
			}
			if err := ValidateGlobInterps(s); err != nil {
				return nil, fmt.Errorf("filename %q: %w", s, err)
			}
			out = append(out, s)
		}
		if len(out) == 0 {
			return nil, nil
		}
		return out, nil
	case []string:
		out := make([]string, 0, len(t))
		for _, s := range t {
			if s == "" {
				continue
			}
			if err := ValidateGlobInterps(s); err != nil {
				return nil, fmt.Errorf("filename %q: %w", s, err)
			}
			out = append(out, s)
		}
		if len(out) == 0 {
			return nil, nil
		}
		return out, nil
	default:
		return nil, fmt.Errorf(
			"filename must be a string or list of strings, got %T", v)
	}
}

// MatchFilename reports whether base matches any of the schema filename
// globs. An empty patterns slice means "no constraint" and returns
// matched=true. A valid match wins regardless of glob order, so a
// malformed glob never masks a base that matches another glob; only
// when no glob matches is the first malformed glob (if any) surfaced,
// returning matched=false with that pattern and the filepath.Match
// error. On a clean non-match it returns matched=false with an empty
// badPattern and a nil error.
func MatchFilename(patterns []string, base string) (matched bool, badPattern string, err error) {
	if len(patterns) == 0 {
		return true, "", nil
	}
	var firstBad string
	var firstErr error
	for _, p := range patterns {
		ok, mErr := filepath.Match(p, base)
		if mErr != nil {
			if firstErr == nil {
				firstBad, firstErr = p, mErr
			}
			continue
		}
		if ok {
			return true, "", nil
		}
	}
	if firstErr != nil {
		return false, firstBad, firstErr
	}
	return false, "", nil
}

// ResolveFilenamePatterns applies ResolveGlobPattern to every entry
// in patterns, returning the resolved list plus a flag reporting
// whether any entry actually changed. A list with no `\#(...)`
// reference — every list authored before this feature — is returned
// as-is with interpolated=false and no allocation, so the common path
// pays nothing.
//
// The first unresolvable reference aborts with its error rather than
// dropping the entry: silently skipping a pattern whose front-matter
// field is missing would let the remaining globs decide the verdict,
// which is not what the author asked for.
func ResolveFilenamePatterns(
	patterns []string, fm map[string]any,
) (resolved []string, interpolated bool, err error) {
	for i, p := range patterns {
		if !PatternHasInterp(p) {
			continue
		}
		r, rErr := ResolveGlobPattern(p, fm)
		if rErr != nil {
			return nil, false, rErr
		}
		if resolved == nil {
			resolved = append([]string(nil), patterns...)
		}
		resolved[i] = r
	}
	if resolved == nil {
		return patterns, false, nil
	}
	return resolved, true, nil
}

// FilenameExpected renders the "expected" clause of a filename
// diagnostic. A single glob reads "filename matching glob <p>" — the
// historical wording — while several read
// "filename matching one of globs <p1>, <p2>" so the OR nature is
// explicit.
func FilenameExpected(patterns []string) string {
	if len(patterns) == 1 {
		return "filename matching glob " + patterns[0]
	}
	return "filename matching one of globs " + strings.Join(patterns, ", ")
}
