package schema

import (
	"fmt"
	"regexp"
	"regexp/syntax"
	"strconv"
	"strings"
	"sync"

	"github.com/jeduden/mdsmith/internal/fieldinterp"
)

// digitsCaptureName is the named group used by the `digits` helper.
// One per matcher; the validator inspects this group when
// `sequential: true` is set.
const digitsCaptureName = "n"

// digitsCaptureExpr is the literal string the `digits` helper
// substitutes into a `regex:` pattern.
const digitsCaptureExpr = `(?P<` + digitsCaptureName + `>[0-9]+)`

// interpMarker is the two-byte opener for a `\#(...)` interpolation
// reference inside a `regex:` body. mdsmith resolves the two
// supported expressions itself (`digits` and `fmvar(name)`) rather
// than going through the CUE evaluator for every pattern.
const interpMarker = `\#(`

// parseFmvarCall parses an `fmvar(<name>)` helper invocation
// inside an already-trimmed interpolation body. The argument is
// returned with surrounding whitespace stripped. The parser
// honours double-quoted segments so a CUE label whose key
// contains a literal `)` — written as `fmvar("release)date")` —
// reaches fmvarLookup intact instead of being truncated at the
// first close paren.
//
// Returns (name, true) on a syntactically valid invocation,
// or ("", false) when expr is not an fmvar call at all.
func parseFmvarCall(expr string) (string, bool) {
	rest := strings.TrimSpace(expr)
	if !strings.HasPrefix(rest, "fmvar") {
		return "", false
	}
	rest = strings.TrimSpace(rest[len("fmvar"):])
	if !strings.HasPrefix(rest, "(") {
		return "", false
	}
	rest = rest[1:]
	end := findCallArgEnd(rest)
	if end < 0 {
		return "", false
	}
	arg := strings.TrimSpace(rest[:end])
	tail := strings.TrimSpace(rest[end+1:])
	if tail != "" {
		return "", false
	}
	return arg, true
}

// findCallArgEnd returns the index of the closing `)` that ends
// the fmvar argument, scanning past double-quoted segments so
// embedded `)` characters do not terminate early. Returns -1
// when no close paren is found.
func findCallArgEnd(s string) int {
	i := 0
	for i < len(s) {
		switch s[i] {
		case '"':
			i = skipQuotedSegment(s, i)
		case ')':
			return i
		default:
			i++
		}
	}
	return -1
}

// scanInterps walks pattern, calling visit on each `\#(expr)`
// reference with the inner expression and the [start, end) byte
// offsets of the full reference. Nested parens inside expr are
// balanced; callers can replace the spans atomically.
func scanInterps(pattern string, visit func(expr string, start, end int) error) error {
	for i := 0; i < len(pattern); {
		idx := strings.Index(pattern[i:], interpMarker)
		if idx < 0 {
			return nil
		}
		start := i + idx
		exprStart := start + len(interpMarker)
		j, ok := findInterpEnd(pattern, exprStart)
		if !ok {
			return fmt.Errorf("unterminated interpolation in pattern")
		}
		expr := pattern[exprStart:j]
		if err := visit(expr, start, j+1); err != nil {
			return err
		}
		i = j + 1
	}
	return nil
}

// findInterpEnd walks pattern starting at exprStart (the byte
// after the opening `\#(`) and returns the index of the matching
// `)` plus true. It tracks paren depth across the body and skips
// over double-quoted segments so a quoted CUE path like
// `fmvar("release)date")` survives intact. Returns (0, false)
// when the interpolation is unterminated.
func findInterpEnd(pattern string, exprStart int) (int, bool) {
	depth := 1
	j := exprStart
	for j < len(pattern) && depth > 0 {
		c := pattern[j]
		if c == '"' {
			j = skipQuotedSegment(pattern, j)
			continue
		}
		switch c {
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth == 0 {
			return j, true
		}
		j++
	}
	return 0, false
}

// skipQuotedSegment advances past a double-quoted segment whose
// opening quote is at pattern[i]. Returns the index of the
// character after the closing quote (or len(pattern) when the
// segment is unterminated). Backslash escapes inside the segment
// consume the next byte verbatim so an escaped quote (`\"`) does
// not close the segment.
func skipQuotedSegment(pattern string, i int) int {
	i++ // step past the opening quote
	for i < len(pattern) {
		if pattern[i] == '\\' && i+1 < len(pattern) {
			i += 2
			continue
		}
		if pattern[i] == '"' {
			return i + 1
		}
		i++
	}
	return i
}

// resolvePattern substitutes the two supported `\#(expr)` helpers
// in a matcher's `regex:` body and returns the resulting RE2 source
// (anchored by callers with `^(?:...)$`). Frontmatter lookups use
// fm; a missing `fmvar` field returns an error so matchHeading
// turns it into a non-match instead of substituting an empty
// regex fragment that would otherwise let a degenerate heading
// match the literal-only remainder of the pattern.
//
// Returns (resolved, err) where err is non-nil when the input
// references an unknown helper or an unresolved `fmvar` field.
func resolvePattern(pattern string, fm map[string]any) (string, error) {
	return rewriteInterps(pattern, func(expr string) (string, error) {
		expr = strings.TrimSpace(expr)
		if expr == "digits" {
			return digitsCaptureExpr, nil
		}
		if name, ok := parseFmvarCall(expr); ok {
			val, found := fmvarLookup(fm, name)
			if !found {
				// Missing frontmatter value: fail the match
				// rather than substituting an empty regex
				// fragment, which would otherwise let a
				// heading like `## Topic ` match
				// `regex: 'Topic \#(fmvar(id))'` even with
				// `id` absent. matchHeading turns the error
				// into a non-match, surfacing the section as
				// missing instead of silently passing.
				return "", fmt.Errorf(
					"`fmvar(%s)`: frontmatter value missing", name)
			}
			return regexp.QuoteMeta(val), nil
		}
		return "", fmt.Errorf("unknown helper %q in `regex:` pattern", expr)
	})
}

// resolvePatternForCheck substitutes helpers using a sentinel for
// frontmatter lookups so the result is syntactically valid even
// when no document is in hand. Used at parse time to catch invalid
// RE2 in `regex:` early. Returns an error when the pattern uses an
// unsupported helper, an unterminated `\#(` reference, or an
// `fmvar(name)` whose argument is not a valid CUE path — turning
// a schema typo into a parse-time diagnostic instead of a
// confusing missing-section diagnostic at validate-time.
func resolvePatternForCheck(pattern string) (string, error) {
	return rewriteInterps(pattern, func(expr string) (string, error) {
		expr = strings.TrimSpace(expr)
		if expr == "digits" {
			return digitsCaptureExpr, nil
		}
		if name, ok := parseFmvarCall(expr); ok {
			if fieldinterp.ParseCUEPath(name) == nil {
				return "", fmt.Errorf(
					"`fmvar(%s)`: invalid frontmatter path "+
						"(non-identifier keys must be quoted, "+
						"e.g. `fmvar(\"my-key\")`)", name)
			}
			// Use a literal placeholder so the compiled regex is
			// syntactically valid. The validator will re-resolve
			// against the real frontmatter at validate-time.
			return "PROBE", nil
		}
		return "", fmt.Errorf("unknown helper %q in `regex:` pattern", expr)
	})
}

// rewriteInterps walks pattern and replaces each `\#(expr)`
// reference with the result of replace(expr).
func rewriteInterps(pattern string, replace func(expr string) (string, error)) (string, error) {
	var b strings.Builder
	cursor := 0
	err := scanInterps(pattern, func(expr string, start, end int) error {
		b.WriteString(pattern[cursor:start])
		rep, err := replace(expr)
		if err != nil {
			return err
		}
		b.WriteString(rep)
		cursor = end
		return nil
	})
	if err != nil {
		return "", err
	}
	b.WriteString(pattern[cursor:])
	return b.String(), nil
}

// fmvarLookup resolves a CUE-style field path against fm. The
// second return is true when the path resolves to a concrete
// value (including the empty string) and false when fm is nil,
// the path is unparseable, or the field is absent. resolvePattern
// uses the ok signal to fail the match for unresolved fmvar
// references, so a missing frontmatter field surfaces the
// section as missing rather than letting a partial regex match a
// degenerate heading.
func fmvarLookup(fm map[string]any, name string) (string, bool) {
	path := fieldinterp.ParseCUEPath(name)
	if len(path) == 0 || fm == nil {
		return "", false
	}
	val, err := fieldinterp.ResolvePath(fm, path)
	if err != nil {
		return "", false
	}
	return val, true
}

// patternHasDigits reports whether pattern references the `digits`
// helper via the `\#(digits)` interpolation. Used to validate
// `sequential: true` at parse time.
func patternHasDigits(pattern string) bool {
	return countDigitsHelpers(pattern) > 0
}

// hasNamedCapture reports whether pattern contains a named
// capture group with the given name. The check parses the regex
// via regexp/syntax so escape sequences (`\(\?P<n>`) and
// character-class contents that happen to spell `(?P<n>` are
// correctly excluded. A pattern that fails to parse is treated
// as having no captures — compileMatcher will surface the parse
// error later with full diagnostic context.
func hasNamedCapture(pattern, name string) bool {
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return false
	}
	return regexpHasNamedCapture(re, name)
}

// regexpHasNamedCapture walks the parsed regex AST looking for a
// capture group whose Name equals the target. Recurses into every
// sub-expression — alternations, concatenations, repeats, all
// nest captures inside their Sub slice.
func regexpHasNamedCapture(re *syntax.Regexp, name string) bool {
	if re == nil {
		return false
	}
	if re.Op == syntax.OpCapture && re.Name == name {
		return true
	}
	for _, sub := range re.Sub {
		if regexpHasNamedCapture(sub, name) {
			return true
		}
	}
	return false
}

// countDigitsHelpers returns the number of `\#(digits)` references
// in pattern. The matcher runtime only reads the first `n` capture,
// so the parser rejects patterns with more than one helper.
func countDigitsHelpers(pattern string) int {
	count := 0
	_ = scanInterps(pattern, func(expr string, _, _ int) error {
		if strings.TrimSpace(expr) == "digits" {
			count++
		}
		return nil
	})
	return count
}

// compiledMatcher carries a compiled RE2 pattern plus the index of
// the `digits` named capture (-1 when absent) so the validator can
// pull captured numbers out without scanning SubexpNames per match.
type compiledMatcher struct {
	re        *regexp.Regexp
	digitsIdx int
}

// compileMatcher resolves m's pattern against fm and compiles it as
// an anchored RE2 expression. The pattern is wrapped in `^(?:...)$`
// so callers do not have to anchor explicitly; the original body is
// what the docs and proto.md tables show.
func compileMatcher(m *Matcher, fm map[string]any) (*compiledMatcher, error) {
	if m == nil {
		return nil, fmt.Errorf("nil matcher")
	}
	resolved, err := resolvePattern(m.Regex, fm)
	if err != nil {
		return nil, err
	}
	re, err := regexp.Compile("^(?:" + resolved + ")$")
	if err != nil {
		return nil, err
	}
	digitsIdx := -1
	for i, name := range re.SubexpNames() {
		if name == digitsCaptureName {
			digitsIdx = i
			break
		}
	}
	return &compiledMatcher{re: re, digitsIdx: digitsIdx}, nil
}

// matcherCacheCap bounds the per-process compiled-matcher cache.
// A long-running LSP session edits front matter often, and each
// distinct `fmvar(...)` value produces a fresh cache key — without
// a bound the cache grows unboundedly. 1024 entries covers a busy
// workspace while keeping the working-set predictable; once full,
// the cache resets so old entries do not pin compiled regexps for
// the lifetime of the process.
const matcherCacheCap = 1024

// matcherCache memoises compiled matchers keyed on the raw pattern
// plus a serialised frontmatter fingerprint. Hot loops (one
// validator pass walks every heading × every scope) re-use the
// compiled regexp instead of recompiling per heading. The cache
// is sized-bounded by matcherCacheCap; see matcherCacheReset.
var (
	matcherCacheMu  sync.Mutex
	matcherCache    map[matcherCacheKey]*compiledMatcher = make(map[matcherCacheKey]*compiledMatcher, matcherCacheCap)
	matcherCacheLen int
)

type matcherCacheKey struct {
	regex string
	fmKey string
}

// cachedMatcher returns the compiled matcher for m under fm,
// compiling on cache miss. nil + err on a malformed pattern; the
// caller decides whether to surface that as a diagnostic.
//
// When the cache reaches matcherCacheCap, the entire map is
// dropped before inserting the new entry. A simple reset beats
// per-entry LRU bookkeeping for workloads where hot patterns are
// re-encountered on the next validator pass anyway.
func cachedMatcher(m *Matcher, fm map[string]any) (*compiledMatcher, error) {
	if m == nil {
		return nil, fmt.Errorf("nil matcher")
	}
	key := matcherCacheKey{regex: m.Regex, fmKey: fmFingerprint(m.Regex, fm)}
	matcherCacheMu.Lock()
	if v, ok := matcherCache[key]; ok {
		matcherCacheMu.Unlock()
		return v, nil
	}
	matcherCacheMu.Unlock()
	cm, err := compileMatcher(m, fm)
	if err != nil {
		return nil, err
	}
	matcherCacheMu.Lock()
	if matcherCacheLen >= matcherCacheCap {
		matcherCache = make(map[matcherCacheKey]*compiledMatcher, matcherCacheCap)
		matcherCacheLen = 0
	}
	if _, exists := matcherCache[key]; !exists {
		matcherCacheLen++
	}
	matcherCache[key] = cm
	matcherCacheMu.Unlock()
	return cm, nil
}

// fmFingerprint joins the fmvar values referenced by pattern into a
// stable cache key. Patterns that don't use `fmvar(...)` get an
// empty fingerprint so they share a cache entry across documents.
//
// Names and values are written through strconv.Quote so a value
// that contains a separator character (`;`, `=`) cannot collide
// with another field's name=value pair. Two documents whose
// `fmvar(...)` substitutions differ always produce different
// fingerprints.
func fmFingerprint(pattern string, fm map[string]any) string {
	var b strings.Builder
	_ = scanInterps(pattern, func(expr string, _, _ int) error {
		expr = strings.TrimSpace(expr)
		name, isCall := parseFmvarCall(expr)
		if !isCall {
			return nil
		}
		val, ok := fmvarLookup(fm, name)
		b.WriteString(strconv.Quote(name))
		b.WriteByte('=')
		if ok {
			b.WriteString(strconv.Quote(val))
		} else {
			b.WriteString("<missing>")
		}
		b.WriteByte('\n')
		return nil
	})
	return b.String()
}

// headingCaptures reports whether dh.Text matches m's pattern and,
// when it does, returns every non-empty named capture group keyed
// by its group name. The schema parser only ever names the `n`
// digits group, so today the map carries at most that one key;
// returning the full SubexpNames map keeps the seam open for
// future named placeholders without another matcher change.
func headingCaptures(m *Matcher, dh DocHeading, fm map[string]any) (bool, map[string]string) {
	if m == nil {
		return false, nil
	}
	cm, err := cachedMatcher(m, fm)
	if err != nil {
		return false, nil
	}
	sub := cm.re.FindStringSubmatch(dh.Text)
	if sub == nil {
		return false, nil
	}
	var out map[string]string
	// SubexpNames and FindStringSubmatch are both NumSubexp()+1
	// long, so an index from one always indexes the other.
	for i, name := range cm.re.SubexpNames() {
		if i == 0 || name == "" {
			continue
		}
		if out == nil {
			out = make(map[string]string, 1)
		}
		out[name] = sub[i]
	}
	return true, out
}

// matchHeading reports whether dh.Text matches m's pattern, and if
// so returns the captured `digits` value (when present) as the
// second return.
func matchHeading(m *Matcher, dh DocHeading, fm map[string]any) (bool, string) {
	if m == nil {
		return false, ""
	}
	cm, err := cachedMatcher(m, fm)
	if err != nil {
		return false, ""
	}
	sub := cm.re.FindStringSubmatch(dh.Text)
	if sub == nil {
		return false, ""
	}
	if cm.digitsIdx <= 0 || cm.digitsIdx >= len(sub) {
		return true, ""
	}
	return true, sub[cm.digitsIdx]
}
