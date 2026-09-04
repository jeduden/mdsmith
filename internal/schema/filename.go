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
				return nil, fmt.Errorf(
					"filename list entries must be non-empty globs")
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
