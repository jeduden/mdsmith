package schema

import (
	"fmt"
	"path/filepath"
	"strconv"
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
		return decodeFilenameList(t)
	case []string:
		return decodeFilenameList(t)
	default:
		return nil, fmt.Errorf(
			"filename must be a string or list of strings, got %T", v)
	}
}

// decodeFilenameList validates a `filename:` sequence, whether it was
// decoded from YAML ([]any) or built in Go ([]string). Every entry must
// be a non-empty string; an empty list yields nil (no constraint).
func decodeFilenameList[T any](list []T) ([]string, error) {
	if len(list) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(list))
	for _, e := range list {
		s, ok := any(e).(string)
		if !ok {
			return nil, fmt.Errorf(
				"filename must be a string or list of strings, "+
					"got %T in the list", any(e))
		}
		if s == "" {
			return nil, fmt.Errorf(
				"filename list entries must be non-empty globs")
		}
		out = append(out, s)
	}
	return out, nil
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

// resolveFilenamePatterns resolves every entry's `\#(...)` references
// against fm, returning the matchable list and the first reference
// that could not be resolved. A list with no reference — every list
// authored before this feature — is returned as-is with
// unresolved=nil and no allocation, so the common path pays nothing.
//
// An entry whose reference cannot be resolved is DROPPED from the
// returned list rather than aborting the whole call: `filename:` is
// an OR list, so a basename that satisfies a sibling glob must still
// pass. The error rides along so the caller can surface it as the
// hint when nothing matched — including the all-entries-dropped case,
// which the caller detects as an empty resolved list.
//
// Each entry is scanned once. The output list is only allocated at
// the first entry that interpolates, seeded with the plain entries
// before it.
func resolveFilenamePatterns(
	patterns []string, fm map[string]any, fmIsCUE bool,
) (resolved []string, unresolved error) {
	var out []string
	for i, p := range patterns {
		r, interp, rErr := resolveFilenameEntry(p, fm, fmIsCUE)
		if !interp {
			if out != nil {
				out = append(out, p)
			}
			continue
		}
		if out == nil {
			out = make([]string, i, len(patterns))
			copy(out, patterns[:i])
		}
		if rErr != nil {
			if unresolved == nil {
				unresolved = rErr
			}
			continue
		}
		out = append(out, r)
	}
	if out == nil {
		return patterns, nil // no entry interpolates
	}
	return out, unresolved
}

// resolveFilenameEntry is resolveFilenamePatterns for one entry: the
// form MatchFilename matches, whether p carries a reference at all,
// and why a reference could not be resolved. A plain entry is
// returned unchanged with interp=false.
//
// When fmIsCUE is set the front-matter values are CUE constraints,
// not data, so every reference becomes a non-empty `?*` wildcard
// (WildcardGlobRefs).
func resolveFilenameEntry(
	p string, fm map[string]any, fmIsCUE bool,
) (resolved string, interp bool, err error) {
	if !PatternHasInterp(p) {
		return p, false, nil
	}
	if fmIsCUE {
		return WildcardGlobRefs(p), true, nil
	}
	// filenameMetaEscaper, not globMetaEscaper: MatchFilename runs
	// filepath.Match, which knows no brace alternatives and ignores
	// `\` escapes on Windows.
	r, rErr := resolveGlobPattern(p, fm, filenameMetaEscaper)
	return r, true, rErr
}

// authoredFilenamePattern maps badPattern, an entry of the resolved
// list that MatchFilename rejected as malformed, back to the entry
// the author wrote, so the diagnostic quotes the schema text rather
// than a substituted form. It also returns the hint that shows the
// front matter applied, empty for a plain entry. It runs only on the
// malformed-glob path.
func authoredFilenamePattern(
	patterns []string, badPattern string, fm map[string]any, fmIsCUE bool,
) (authored, hint string) {
	authored = badPattern
	for _, p := range patterns {
		r, interp, err := resolveFilenameEntry(p, fm, fmIsCUE)
		if err == nil && r == badPattern {
			authored = p
			if interp && !fmIsCUE {
				hint = InterpolatedGlobHint(GlobHintForm(p, fm))
			}
			break
		}
	}
	return authored, hint
}

// filenameHintForms returns, for the "with front matter applied"
// hint, each entry that carried a reference and resolved, with the
// values as the author wrote them (GlobHintForm). A plain sibling
// glob was never substituted, so listing it would claim a
// substitution that never happened; under fmIsCUE nothing was
// substituted at all. It runs only once a basename has missed, so
// the match path pays nothing for it.
func filenameHintForms(
	patterns []string, fm map[string]any, fmIsCUE bool,
) []string {
	if fmIsCUE {
		return nil
	}
	var out []string
	for _, p := range patterns {
		if !PatternHasInterp(p) {
			continue
		}
		if h, err := resolveGlobPattern(p, fm, nil); err == nil {
			out = append(out, h)
		}
	}
	return out
}

// FilenameDiagnostic reports the `filename:` verdict for base against
// patterns, resolving each entry's `\#(fmvar(name))` references
// against fm first so a schema can require the basename to agree with
// a front-matter value. It returns nil when the basename satisfies
// the constraint (including the "no constraint configured" case);
// otherwise it returns the diagnostic the caller emits with its own
// anchor and MakeDiag. ref names the schema source. fmIsCUE marks fm
// as CUE constraints (the `cue-frontmatter` placeholder); a reference
// then matches any non-empty text in one segment instead of a value.
//
// fmErr is why the document's front matter failed to parse, or nil.
// A failed parse leaves fm empty, so every reference is unresolved;
// the hint then names fmErr instead of claiming the field is missing,
// the same way the kind-level `path-pattern:` diagnostic does.
//
// Both `filename:` surfaces — the inline/composed schema path
// (validateFilename) and the legacy proto.md path
// (requiredstructure.checkFilenamePattern) — route through here so
// their wording, hint selection, and OR semantics cannot drift.
func FilenameDiagnostic(
	patterns []string, base string, fm map[string]any, fmErr error,
	fmIsCUE bool, ref string,
) *SchemaDiagnostic {
	if len(patterns) == 0 {
		return nil
	}
	resolved, unresolved := resolveFilenamePatterns(patterns, fm, fmIsCUE)
	matched, badPattern, err := MatchFilename(resolved, base)
	if err != nil {
		// Malformed glob in the schema. Surface it via the same
		// SchemaDiagnostic shape so the message carries the schema
		// reference and the user can jump to the offending pattern.
		// Actual quotes the entry as the author wrote it; when front
		// matter made it malformed, the hint shows the result.
		authored, applied := authoredFilenamePattern(
			patterns, badPattern, fm, fmIsCUE)
		hint := err.Error()
		if applied != "" {
			hint += "; " + applied
		}
		return &SchemaDiagnostic{
			Field:     "filename pattern",
			Actual:    strconv.Quote(authored),
			Expected:  "valid glob",
			Hint:      hint,
			SchemaRef: ref,
		}
	}
	// An empty resolved list means every entry was dropped as
	// unresolvable; MatchFilename reads that as "no constraint", so
	// the emptiness has to be checked here rather than trusting
	// matched.
	if matched && len(resolved) > 0 {
		return nil
	}
	// The parse failure is WHY a reference did not resolve, so it
	// takes the "missing" report's place.
	if unresolved != nil && fmErr != nil {
		unresolved = fmErr
	}
	// `glob` makes the constraint syntax explicit: users occasionally
	// read `string matching <pattern>` as a regex requirement, which
	// filepath.Match does not accept. The wording also lines up with
	// the kind-level `path-pattern` diagnostic ("path matching glob
	// ...") so the user vocabulary is consistent across both
	// surfaces. With several globs configured the "expected" clause
	// lists them all so the OR nature is visible.
	return &SchemaDiagnostic{
		Field:    "filename",
		Actual:   strconv.Quote(base),
		Expected: FilenameExpected(patterns),
		Hint: GlobMismatchHint(unresolved, patterns,
			filenameHintForms(patterns, fm, fmIsCUE)...),
		SchemaRef: ref,
	}
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
