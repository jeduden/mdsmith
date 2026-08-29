package schema

import (
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/fieldinterp"
)

// globMetaChars are the bytes a resolved `fmvar(...)` value must not
// contribute to the surrounding glob. Escaping them makes the
// frontmatter value match literally — the glob analogue of the
// `regex:` matcher's regexp.QuoteMeta.
//
// The set covers both matchers the resolved pattern feeds:
// doublestar (kind `path-pattern:`) and filepath.Match (schema
// `filename:`). `{` is doublestar-only — filepath.Match does not
// know brace alternatives — but escaping it is harmless there
// because `\{` still matches a literal `{`. `]` and `}` need no
// escape: neither is special until an unescaped `[` or `{` opens the
// construct, and both openers are escaped here.
//
// One platform caveat, confined to the `filename:` surface:
// filepath.Match disables escaping on Windows and reads `\` as a
// separator instead, so an escaped byte never matches there. Only
// `[` and `{` can trigger it — Windows filenames cannot contain
// `\`, `*`, or `?` at all. doublestar (the `path-pattern:` surface)
// always uses `/` as its separator and honours `\` escapes on every
// platform, so it is unaffected.
const globMetaChars = `\*?[{`

// PatternHasInterp reports whether pattern carries at least one
// `\#(...)` interpolation reference. Callers use it to skip the
// resolver — and the front-matter read it needs — on the overwhelming
// majority of patterns, which are plain globs.
func PatternHasInterp(pattern string) bool {
	return strings.Contains(pattern, interpMarker)
}

// ResolveGlobPattern substitutes every `\#(fmvar(name))` reference in
// a glob pattern with the named front-matter value, escaped so its
// glob metacharacters match literally. The result is a plain glob the
// caller feeds to doublestar (`path-pattern:`) or filepath.Match
// (`filename:`).
//
// `fmvar(name)` is the only helper in scope. `digits`, which the
// `regex:` matcher accepts, has no meaning for a glob — there is no
// capture to read back — so it is rejected rather than silently
// substituted.
//
// A reference whose field is absent from fm — or present but empty —
// returns an error instead of substituting an empty segment, which
// would otherwise let `.apm/skills//SKILL.md` quietly match nothing
// (or, for a trailing reference, match a degenerate name). The two
// cases get distinct messages because the fix differs: add the field
// versus give it a value. Callers surface the error as a diagnostic
// naming the field.
func ResolveGlobPattern(pattern string, fm map[string]any) (string, error) {
	if !PatternHasInterp(pattern) {
		return pattern, nil
	}
	return rewriteInterps(pattern, func(expr string) (string, error) {
		name, ok := parseFmvarCall(strings.TrimSpace(expr))
		if !ok {
			return "", unknownGlobHelperErr(expr)
		}
		val, found := fmvarLookup(fm, name)
		if !found {
			return "", fmt.Errorf(
				"`fmvar(%s)`: frontmatter value missing", name)
		}
		if val == "" {
			return "", fmt.Errorf(
				"`fmvar(%s)`: frontmatter value is empty", name)
		}
		return escapeGlobMeta(val), nil
	})
}

// ValidateGlobInterps checks a glob pattern's `\#(...)` references
// without a document in hand: every reference must be a syntactically
// valid `fmvar(<cue-path>)` call. It turns a schema typo into a
// config-load error instead of a per-file "frontmatter value missing"
// diagnostic that points at the document rather than the pattern.
// This is the glob counterpart of resolvePatternForCheck.
func ValidateGlobInterps(pattern string) error {
	if !PatternHasInterp(pattern) {
		return nil
	}
	return scanInterps(pattern, func(expr string, _, _ int) error {
		name, ok := parseFmvarCall(strings.TrimSpace(expr))
		if !ok {
			return unknownGlobHelperErr(expr)
		}
		if fieldinterp.ParseCUEPath(name) == nil {
			return fmt.Errorf(
				"`fmvar(%s)`: invalid frontmatter path "+
					"(non-identifier keys must be quoted, "+
					"e.g. `fmvar(\"my-key\")`)", name)
		}
		return nil
	})
}

// unknownGlobHelperErr is the shared message for a `\#(...)` body a
// glob pattern cannot resolve, so the resolver and the parse-time
// validator name the same supported form.
func unknownGlobHelperErr(expr string) error {
	return fmt.Errorf(
		"unknown helper %q in glob pattern; only `fmvar(name)` "+
			"is supported here", strings.TrimSpace(expr))
}

// InterpolatedGlobHint renders the diagnostic hint that shows what a
// `\#(fmvar(...))` pattern became once the document's front matter
// was applied — without it the reader sees only the unresolved
// pattern and has to do the substitution in their head. It returns
// the empty string (no hint) when nothing was interpolated, so plain
// globs keep their historical one-line diagnostic.
func InterpolatedGlobHint(interpolated bool, resolved ...string) string {
	if !interpolated || len(resolved) == 0 {
		return ""
	}
	return "with front matter applied: " + strings.Join(resolved, ", ")
}

// escapeGlobMeta backslash-escapes every glob metacharacter in s so
// the string matches itself literally inside a surrounding pattern.
// Values free of metacharacters — nearly all of them — are returned
// unchanged with no allocation.
func escapeGlobMeta(s string) string {
	if !strings.ContainsAny(s, globMetaChars) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 4)
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(globMetaChars, s[i]) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
